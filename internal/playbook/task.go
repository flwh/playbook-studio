// Package playbook 实现 AME Playbook (.apbx) 的解析与编辑。
//
// 关键设计：对“已存在”的 action 一律走源码文本手术式修改，不做整体重新序列化，
// 因此编辑后只有被改动的部分发生变化，注释、空行、引号风格、长行都不会被打乱。
package playbook

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ---------- 行级工具 ----------

func isBlankLine(s string) bool   { return strings.TrimSpace(s) == "" }
func isCommentLine(s string) bool { return strings.HasPrefix(strings.TrimSpace(s), "#") }
func isSeqLine(s string) bool {
	t := strings.TrimLeft(s, " \t")
	return strings.HasPrefix(t, "- ") || t == "-"
}

func indentWidth(s string) int {
	n := 0
	for _, r := range s {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 4
		default:
			return n
		}
	}
	return n
}

func commentLines(c string) int {
	if c == "" {
		return 0
	}
	return strings.Count(strings.TrimRight(c, "\n"), "\n") + 1
}

// scanHeadUp 从 itemLine(1-based) 向上收集紧邻的注释块起始行（允许注释之间夹空行）
func scanHeadUp(lines []string, itemLine int) int {
	start := itemLine
	j := itemLine - 1
	for j >= 1 {
		if isCommentLine(lines[j-1]) {
			start = j
			j--
			continue
		}
		if isBlankLine(lines[j-1]) && j-1 >= 1 && isCommentLine(lines[j-2]) {
			start = j
			j--
			continue
		}
		break
	}
	return start
}

// blockEndDown 从 startLine 向下扫到块结束（最后一项用）
func blockEndDown(lines []string, startLine, itemIndent int) int {
	last := startLine
	for j := startLine; j <= len(lines); j++ {
		ln := lines[j-1]
		if isBlankLine(ln) {
			continue
		}
		if j > startLine && indentWidth(ln) <= itemIndent {
			break
		}
		last = j
	}
	return last
}

// ---------- 数据模型 ----------

type TaskMeta struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Privilege   string   `json:"privilege"`
	OnUpgrade   bool     `json:"onUpgrade"`
	DependsOn   []string `json:"dependsOn"`
}

// Field 是 action 的一个键值对
type Field struct {
	Key   string `json:"key"`
	Kind  string `json:"kind"` // scalar | list | map | block
	Value string `json:"value"`
	Tag   string `json:"tag"` // YAML 解析 tag，如 !!bool / !!int / !!str
	Items []string `json:"items,omitempty"`
	// 源位置（1-based）
	Line   int `json:"line"`
	Column int `json:"column"`
	KeyCol int `json:"keyCol"`
}

// LastLine 返回该字段在源码中占据的最后一行的行号(1-based)
func (f Field) LastLine() int {
	if f.Kind == "block" && f.Line > 0 {
		return f.Line + strings.Count(strings.TrimRight(f.Value, "\n"), "\n")
	}
	return f.Line
}

// Action 是 actions 序列中的一项，如 `- !registryValue: {...}`
type Action struct {
	Index  int     `json:"index"`
	Tag    string  `json:"tag"`  // 去掉 ! 与尾部 :
	Flow   bool    `json:"flow"` // 流式 {..}
	Fields []Field `json:"fields"`
	Line   int     `json:"line"`
	// 源码区间（1-based，含）
	SpanStart int    `json:"spanStart"`
	SpanEnd   int    `json:"spanEnd"`
	HeadFrom  int    `json:"headFrom"` // 注释块起始行
	Indent    int    `json:"indent"`   // '-' 所在列(0-based)
	Child     int    `json:"child"`    // 子键缩进
	Raw       string `json:"raw"`
	Note      string `json:"note"` // 上方注释文本
	Option    string `json:"option"`
	Options   []string `json:"options,omitempty"`
	Weight    int    `json:"weight"`
}

// Task 是一个 YAML 任务文件
type Task struct {
	Path    string   `json:"path"`
	Text    string   `json:"text"` // 当前文本（LF）
	CRLF    bool     `json:"-"`
	Meta    TaskMeta `json:"meta"`
	Actions []Action `json:"actions"`

	lines []string
	doc   *yaml.Node
	seq   *yaml.Node
}

// ---------- 解析 ----------

// ParseTask 解析任务 YAML。rel 为 bundle 内相对路径。
func ParseTask(rel string, data []byte) (*Task, error) {
	raw := string(data)
	crlf := strings.Contains(raw, "\r\n")
	text := strings.ReplaceAll(raw, "\r\n", "\n")

	t := &Task{Path: rel, Text: text, CRLF: crlf}
	if err := t.reparse(); err != nil {
		return nil, err
	}
	return t, nil
}

// reparse 重新解析当前文本并重建 action 视图
func (t *Task) reparse() error {
	t.lines = strings.Split(t.Text, "\n")
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(t.Text), &doc); err != nil {
		return fmt.Errorf("%s: %w", t.Path, err)
	}
	t.doc = &doc
	t.seq = nil
	if len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: 顶层不是映射", t.Path)
	}
	t.Meta = TaskMeta{}
	t.seq = findKey(root, "actions")
	t.readMeta(root)
	if t.seq == nil || t.seq.Kind != yaml.SequenceNode {
		t.Actions = nil
		return nil
	}
	return t.buildActions()
}

func findKey(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func (t *Task) readMeta(root *yaml.Node) {
	if n := findKey(root, "title"); n != nil {
		t.Meta.Title = n.Value
	}
	if n := findKey(root, "description"); n != nil {
		t.Meta.Description = n.Value
	}
	if n := findKey(root, "privilege"); n != nil {
		t.Meta.Privilege = n.Value
	}
	if n := findKey(root, "onUpgrade"); n != nil {
		t.Meta.OnUpgrade = n.Value == "true"
	}
	if n := findKey(root, "dependsOn"); n != nil && n.Kind == yaml.SequenceNode {
		for _, c := range n.Content {
			t.Meta.DependsOn = append(t.Meta.DependsOn, c.Value)
		}
	}
}

func (t *Task) buildActions() error {
	n := len(t.seq.Content)
	out := make([]Action, 0, n)
	for i, item := range t.seq.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		a := Action{Index: i, Line: item.Line}
		a.Tag = normalizeTag(item.Tag)
		a.Flow = (item.Style & yaml.FlowStyle) != 0

		lineTxt := t.line(item.Line)
		a.Indent = strings.Index(lineTxt, "-")
		if a.Indent < 0 {
			a.Indent = 0
		}
		a.Child = a.Indent + 2
		if item.Line < len(t.lines) {
			for j := item.Line; j < len(t.lines); j++ {
				if isBlankLine(t.lines[j]) {
					continue
				}
				if indentWidth(t.lines[j]) > a.Indent {
					a.Child = indentWidth(t.lines[j])
				}
				break
			}
		}

		for k := 0; k+1 < len(item.Content); k += 2 {
			kn, vn := item.Content[k], item.Content[k+1]
			f := Field{Key: kn.Value, Line: vn.Line, Column: vn.Column, KeyCol: kn.Column, Tag: vn.Tag}
			switch vn.Kind {
			case yaml.ScalarNode:
				f.Value = vn.Value
				if (vn.Style&yaml.LiteralStyle) != 0 || (vn.Style&yaml.FoldedStyle) != 0 {
					f.Kind = "block"
				} else {
					f.Kind = "scalar"
				}
			case yaml.SequenceNode:
				f.Kind = "list"
				for _, c := range vn.Content {
					f.Items = append(f.Items, c.Value)
				}
			case yaml.MappingNode:
				f.Kind = "map"
			default:
				f.Kind = "scalar"
				f.Value = vn.Value
			}
			a.Fields = append(a.Fields, f)
			switch f.Key {
			case "option":
				a.Option = f.Value
			case "options":
				a.Options = f.Items
			case "weight":
				a.Weight, _ = strconv.Atoi(f.Value)
			}
		}

		a.HeadFrom = scanHeadUp(t.lines, item.Line)
		a.SpanStart = a.HeadFrom
		if i+1 < n {
			a.SpanEnd = scanHeadUp(t.lines, t.seq.Content[i+1].Line) - 1
		} else {
			a.SpanEnd = blockEndDown(t.lines, item.Line, a.Indent)
		}
		for a.SpanEnd > item.Line && isBlankLine(t.line(a.SpanEnd)) {
			a.SpanEnd--
		}
		if a.SpanEnd < item.Line {
			a.SpanEnd = item.Line
		}
		if a.HeadFrom < item.Line {
			a.Note = strings.Join(t.lines[a.HeadFrom-1:item.Line-1], "\n")
		}
		a.Raw = strings.Join(t.lines[a.SpanStart-1:a.SpanEnd], "\n")
		out = append(out, a)
	}
	t.Actions = out
	return nil
}

func (t *Task) line(n int) string {
	if n >= 1 && n <= len(t.lines) {
		return t.lines[n-1]
	}
	return ""
}

func normalizeTag(tag string) string {
	t := strings.TrimPrefix(tag, "!")
	t = strings.TrimSuffix(t, ":")
	return t
}

// FindAction 按索引取 action
func (t *Task) FindAction(i int) (*Action, error) {
	if i < 0 || i >= len(t.Actions) {
		return nil, fmt.Errorf("action 索引越界: %d", i)
	}
	return &t.Actions[i], nil
}

// ActionByLine 返回包含指定源码行的 action（用于“跳转到源码行”）
func (t *Task) ActionByLine(line int) *Action {
	for i := range t.Actions {
		a := &t.Actions[i]
		if line >= a.SpanStart && line <= a.SpanEnd {
			return a
		}
	}
	return nil
}

// ---------- 序列化 ----------

// Bytes 返回当前文本（保留原始换行风格）
func (t *Task) Bytes() []byte {
	if t.CRLF {
		return []byte(strings.ReplaceAll(t.Text, "\n", "\r\n"))
	}
	return []byte(t.Text)
}

// SetText 整体替换文本（源码模式编辑）
func (t *Task) SetText(s string) error {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	t.Text = s
	return t.reparse()
}
