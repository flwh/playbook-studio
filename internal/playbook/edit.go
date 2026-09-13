package playbook

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ---------- 底层拼接 ----------

func insertLines(lines []string, at int, ins []string) []string {
	if at < 0 {
		at = 0
	}
	if at > len(lines) {
		at = len(lines)
	}
	out := make([]string, 0, len(lines)+len(ins))
	out = append(out, lines[:at]...)
	out = append(out, ins...)
	out = append(out, lines[at:]...)
	return out
}

// splice 用 repl 替换 [startLine, endLine]（1-based，含）
func (t *Task) splice(startLine, endLine int, repl []string) error {
	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(t.lines) {
		endLine = len(t.lines)
	}
	if endLine < startLine-1 {
		return fmt.Errorf("非法区间 %d..%d", startLine, endLine)
	}
	out := make([]string, 0, len(t.lines)+len(repl))
	out = append(out, t.lines[:startLine-1]...)
	out = append(out, repl...)
	if endLine < len(t.lines) {
		out = append(out, t.lines[endLine:]...)
	}
	t.Text = strings.Join(out, "\n")
	return t.reparse()
}

func (t *Task) commit(lines []string) error {
	t.Text = strings.Join(lines, "\n")
	return t.reparse()
}

func (t *Task) itemNode(idx int) (*yaml.Node, error) {
	if t.seq == nil {
		return nil, fmt.Errorf("文件中没有 actions 段")
	}
	if idx < 0 || idx >= len(t.seq.Content) {
		return nil, fmt.Errorf("action 索引越界: %d", idx)
	}
	return t.seq.Content[idx], nil
}

func fieldValueNode(item *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	for i := 0; i+1 < len(item.Content); i += 2 {
		if item.Content[i].Value == key {
			return item.Content[i], item.Content[i+1]
		}
	}
	return nil, nil
}

func isBlockScalar(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode &&
		((n.Style&yaml.LiteralStyle) != 0 || (n.Style&yaml.FoldedStyle) != 0)
}

// blockScalarEnd 返回块标量内容的最后一行
func (t *Task) blockScalarEnd(keyLine, keyIndent int) int {
	last := keyLine
	for j := keyLine + 1; j <= len(t.lines); j++ {
		ln := t.lines[j-1]
		if isBlankLine(ln) {
			continue
		}
		if indentWidth(ln) <= keyIndent {
			break
		}
		last = j
	}
	return last
}

// valueEndOnLine 返回某字段值在这一行上的结束列(0-based 不含)
func (t *Task) valueEndOnLine(line string, vn *yaml.Node) int {
	start := vn.Column - 1
	if start < 0 || start > len(line) {
		return len(line)
	}
	switch vn.Kind {
	case yaml.SequenceNode, yaml.MappingNode:
		return flowTokenEnd(line, start)
	default:
		return scalarTokenEnd(line, vn.Column)
	}
}

// flowTokenEnd 从 start(0-based) 找到流式集合的结束位置（'}' 或 ']' 之后），跳过行尾注释
func flowTokenEnd(line string, start int) int {
	depth := 0
	inS, inD := false, false
	for i := start; i < len(line); i++ {
		c := line[i]
		switch {
		case inS:
			if c == '\'' {
				if i+1 < len(line) && line[i+1] == '\'' {
					i++
					continue
				}
				inS = false
			}
		case inD:
			if c == '\\' {
				i++
				continue
			}
			if c == '"' {
				inD = false
			}
		default:
			switch c {
			case '\'':
				inS = true
			case '"':
				inD = true
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth <= 0 {
					return i + 1
				}
			case '#':
				if i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
					return i
				}
			}
		}
	}
	return len(line)
}

// ---------- 字段编辑 ----------

// SetField 修改或新增 action 的某个字段（源码手术，未改动的部分保持原样）
func (t *Task) SetField(idx int, key, value string) error {
	return t.SetFieldTyped(idx, key, value, "auto")
}

// SetFieldTyped 带类型信息的字段修改；kind 见 renderValueFor
func (t *Task) SetFieldTyped(idx int, key, value, kind string) error {
	item, err := t.itemNode(idx)
	if err != nil {
		return err
	}
	a := &t.Actions[idx]
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("字段名不能为空")
	}
	kn, vn := fieldValueNode(item, key)
	if kn == nil {
		return t.addFieldKind(a, key, value, kind)
	}
	// 多行块标量：整块替换
	if isBlockScalar(vn) {
		keyIndent := kn.Column - 1
		repl := renderBlockLiteral(key, value, keyIndent)
		// renderBlockLiteral 需要 key 缩进；kn.Column 指向 key 起始
		return t.splice(kn.Line, t.blockScalarEnd(kn.Line, keyIndent), repl)
	}
	// 列表
	if vn.Kind == yaml.SequenceNode {
		li := vn.Line - 1
		if li < 0 || li >= len(t.lines) {
			return fmt.Errorf("行号异常")
		}
		line := t.lines[li]
		start := vn.Column - 1
		if start < 0 || start > len(line) {
			return fmt.Errorf("列号异常")
		}
		end := flowTokenEnd(line, start)
		items := splitListInput(value)
		t.lines[li] = line[:start] + renderList(items) + line[end:]
		return t.commit(t.lines)
	}
	// 单行标量
	li := vn.Line - 1
	if li < 0 || li >= len(t.lines) {
		return fmt.Errorf("行号异常")
	}
	line := t.lines[li]
	start := vn.Column - 1
	if start < 0 || start > len(line) {
		return fmt.Errorf("列号异常")
	}
	end := scalarTokenEnd(line, vn.Column)
	tok := ""
	if start <= end {
		tok = line[start:end]
	}
	t.lines[li] = line[:start] + renderValueFor(kind, vn.Tag, tok, value) + line[end:]
	return t.commit(t.lines)
}

func splitListInput(v string) []string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "[")
	v = strings.TrimSuffix(v, "]")
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "'\"")
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// addField 新增字段
func (t *Task) addField(a *Action, key, value string) error {
	return t.addFieldKind(a, key, value, "auto")
}

func (t *Task) addFieldKind(a *Action, key, value, kind string) error {
	if a.Flow || len(a.Fields) == 0 && strings.Contains(t.line(a.Line), "{") {
		li := a.Line - 1
		if li < 0 || li >= len(t.lines) {
			return fmt.Errorf("行号异常")
		}
		line := t.lines[li]
		open := strings.Index(line, "{")
		if open < 0 {
			return t.addFieldBlock(a, key, value, kind)
		}
		closeIdx := flowTokenEnd(line, open)
		closing := closeIdx - 1
		for closing > open && (line[closing] == '}' || line[closing] == ' ') {
			if line[closing] == '}' {
				break
			}
			closing--
		}
		val := renderValueFor(kind, "", "", value)
		if strings.Contains(value, "\n") {
			val = quoteText('"', value)
		}
		seg := key + ": " + val
		inner := strings.TrimSpace(line[open+1 : closing])
		if inner == "" {
			t.lines[li] = line[:open+1] + seg + line[closing:]
		} else {
			t.lines[li] = line[:closing] + ", " + seg + line[closing:]
		}
		return t.commit(t.lines)
	}
	return t.addFieldBlock(a, key, value, kind)
}

func (t *Task) addFieldBlock(a *Action, key, value, kind string) error {
	last := a.Line
	for _, f := range a.Fields {
		if l := f.LastLine(); l > last {
			last = l
		}
	}
	var ins []string
	if strings.Contains(value, "\n") {
		ins = renderBlockLiteral(key, value, a.Child)
	} else {
		pad := strings.Repeat(" ", a.Child)
		ins = []string{pad + key + ": " + renderValueFor(kind, "", "", value)}
	}
	return t.splice(last+1, last, ins)
}

// DeleteField 删除字段
func (t *Task) DeleteField(idx int, key string) error {
	item, err := t.itemNode(idx)
	if err != nil {
		return err
	}
	a := &t.Actions[idx]
	kn, vn := fieldValueNode(item, key)
	if kn == nil || vn == nil {
		return nil
	}
	if a.Flow {
		// 流式 action：字段都在同一行，做 token 级删除
		li := kn.Line - 1
		if li < 0 || li >= len(t.lines) {
			return fmt.Errorf("行号异常")
		}
		line := t.lines[li]
		ks := kn.Column - 1
		ve := t.valueEndOnLine(line, vn)
		// 向前找逗号
		p := ks - 1
		for p >= 0 && (line[p] == ' ' || line[p] == '\t') {
			p--
		}
		if p >= 0 && line[p] == ',' {
			t.lines[li] = line[:p] + line[ve:]
		} else {
			q := ve
			for q < len(line) && (line[q] == ' ' || line[q] == '\t') {
				q++
			}
			if q < len(line) && line[q] == ',' {
				q++
				for q < len(line) && line[q] == ' ' {
					q++
				}
			}
			t.lines[li] = line[:ks] + line[q:]
		}
		return t.commit(t.lines)
	}
	// 块式 action：字段独占一行（块标量可能占多行）
	end := kn.Line
	if isBlockScalar(vn) {
		end = t.blockScalarEnd(kn.Line, kn.Column-1)
	}
	return t.splice(kn.Line, end, nil)
}

// ---------- action 编辑 ----------

func (t *Task) insertPosBefore(a *Action) int { return a.SpanStart - 1 }

// InsertAction 在 idx 处插入一个新 action（idx == len 表示追加）
func (t *Task) InsertAction(idx int, tag string, fields []KeyValue, flow bool) error {
	indent := 2
	if len(t.Actions) > 0 {
		indent = t.Actions[0].Indent
	}
	text := RenderAction(tag, fields, indent, flow)
	var pos int
	switch {
	case len(t.Actions) == 0:
		if t.seq == nil {
			return fmt.Errorf("文件中没有 actions 段")
		}
		l := t.seq.Line
		if l < 1 {
			l = 1
		}
		pos = l
	case idx >= len(t.Actions):
		pos = t.Actions[len(t.Actions)-1].SpanEnd
	default:
		if idx < 0 {
			idx = 0
		}
		pos = t.insertPosBefore(&t.Actions[idx])
	}
	return t.commit(insertLines(t.lines, pos, text))
}

// InsertActionText 以原始文本插入（用于复制/移动）
func (t *Task) InsertActionText(idx int, text []string) error {
	var pos int
	switch {
	case len(t.Actions) == 0:
		pos = t.seq.Line
	case idx >= len(t.Actions):
		pos = t.Actions[len(t.Actions)-1].SpanEnd
	default:
		if idx < 0 {
			idx = 0
		}
		pos = t.insertPosBefore(&t.Actions[idx])
	}
	return t.commit(insertLines(t.lines, pos, text))
}

// DeleteAction 删除 action（连同其上方注释块）
func (t *Task) DeleteAction(idx int) error {
	a, err := t.FindAction(idx)
	if err != nil {
		return err
	}
	start, end := a.SpanStart, a.SpanEnd
	// 避免留下连续空行
	if end+1 <= len(t.lines) && isBlankLine(t.lines[end]) &&
		start-2 >= 0 && isBlankLine(t.lines[start-2]) {
		end++
	}
	return t.splice(start, end, nil)
}

// DuplicateAction 复制 action（源码原样复制）
func (t *Task) DuplicateAction(idx int) error {
	a, err := t.FindAction(idx)
	if err != nil {
		return err
	}
	blk := append([]string{}, t.lines[a.SpanStart-1:a.SpanEnd]...)
	return t.commit(insertLines(t.lines, a.SpanEnd, blk))
}

// MoveAction 把 from 处的 action 移动到最终索引 to
func (t *Task) MoveAction(from, to int) error {
	a, err := t.FindAction(from)
	if err != nil {
		return err
	}
	blk := append([]string{}, t.lines[a.SpanStart-1:a.SpanEnd]...)
	if err := t.splice(a.SpanStart, a.SpanEnd, nil); err != nil {
		return err
	}
	n := len(t.Actions)
	var pos int
	switch {
	case n == 0:
		pos = len(t.lines)
	case to >= n:
		pos = t.Actions[n-1].SpanEnd
	default:
		if to < 0 {
			to = 0
		}
		pos = t.insertPosBefore(&t.Actions[to])
	}
	return t.commit(insertLines(t.lines, pos, blk))
}

// SetMeta 修改顶层元数据（title/description/privilege/onUpgrade）
func (t *Task) SetMeta(key, value string) error {
	if t.doc == nil || len(t.doc.Content) == 0 {
		return fmt.Errorf("空文档")
	}
	root := t.doc.Content[0]
	kn := findKeyNode(root, key)
	if kn != nil {
		li := kn.Line - 1
		if li < 0 || li >= len(t.lines) {
			return fmt.Errorf("行号异常")
		}
		vNode := findValueNode(root, key)
		line := t.lines[li]
		start := vNode.Column - 1
		if start < 0 || start > len(line) {
			return fmt.Errorf("列号异常")
		}
		end := scalarTokenEnd(line, vNode.Column)
		tok := ""
		if start <= end {
			tok = line[start:end]
		}
		t.lines[li] = line[:start] + renderScalarTokenTagged(tok, vNode.Tag, value) + line[end:]
		return t.commit(t.lines)
	}
	// 新增：插到 description/privilege/title 之后
	anchor := 0
	for _, k := range []string{"description", "privilege", "title"} {
		if kn := findKeyNode(root, k); kn != nil {
			anchor = kn.Line
			break
		}
	}
	if anchor == 0 {
		for _, k := range []string{"actions"} {
			if kn := findKeyNode(root, k); kn != nil {
				anchor = kn.Line - 1
			}
		}
	}
	ins := []string{key + ": " + quoteText('\'', value)}
	return t.commit(insertLines(t.lines, anchor, ins))
}

func findKeyNode(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i]
		}
	}
	return nil
}

func findValueNode(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// Weight 解析 weight（用于排序展示）
func (a *Action) WeightInt() int {
	if a.Weight != 0 {
		return a.Weight
	}
	if s, ok := a.FieldValue("weight"); ok {
		n, _ := strconv.Atoi(s)
		return n
	}
	return 0
}

// FieldValue 取字段字符串值
func (a *Action) FieldValue(key string) (string, bool) {
	for _, f := range a.Fields {
		if f.Key == key {
			if f.Kind == "list" {
				return strings.Join(f.Items, ","), true
			}
			return f.Value, true
		}
	}
	return "", false
}
