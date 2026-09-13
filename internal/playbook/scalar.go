package playbook

import (
	"strconv"
	"strings"
)

// scalarTokenEnd 返回单行中标量 token 的结束列(1-based, 不含)，
// col 为 token 起始列(1-based)；支持单引号/双引号/裸标量。
func scalarTokenEnd(line string, col int) int {
	i := col - 1
	if i < 0 || i >= len(line) {
		return i
	}
	switch line[i] {
	case '\'':
		j := i + 1
		for j < len(line) {
			if line[j] == '\'' {
				if j+1 < len(line) && line[j+1] == '\'' {
					j += 2
					continue
				}
				return j + 1
			}
			j++
		}
		return len(line)
	case '"':
		j := i + 1
		for j < len(line) {
			if line[j] == '\\' {
				j += 2
				continue
			}
			if line[j] == '"' {
				return j + 1
			}
			j++
		}
		return len(line)
	}
	// 裸标量：遇到 , } ] 或 " #" 结束
	j := i
	end := len(line)
	for j < len(line) {
		c := line[j]
		if c == ',' || c == '}' || c == ']' {
			end = j
			break
		}
		if c == '#' && j > i && (line[j-1] == ' ' || line[j-1] == '\t') {
			end = j
			break
		}
		j++
	}
	for end > i && (line[end-1] == ' ' || line[end-1] == '\t') {
		end--
	}
	return end
}

// quoteStyleOf 返回 token 的引号风格：0 裸, '\'' 单引号, '"' 双引号
func quoteStyleOf(tok string) byte {
	if len(tok) >= 2 && tok[0] == '\'' {
		return '\''
	}
	if len(tok) >= 2 && tok[0] == '"' {
		return '"'
	}
	return 0
}

// quoteText 按引号风格包装文本
func quoteText(style byte, v string) string {
	switch style {
	case '\'':
		return "'" + strings.ReplaceAll(v, "'", "''") + "'"
	case '"':
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range v {
			switch r {
			case '\\':
				b.WriteString(`\\`)
			case '"':
				b.WriteString(`\"`)
			case '\n':
				b.WriteString(`\n`)
			case '\t':
				b.WriteString(`\t`)
			default:
				b.WriteRune(r)
			}
		}
		b.WriteByte('"')
		return b.String()
	}
	if plainSafe(v) {
		return v
	}
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

// plainSafe 判断裸标量是否安全（不需要引号）
func plainSafe(v string) bool {
	if v == "" || strings.TrimSpace(v) != v {
		return false
	}
	if strings.ContainsAny(v, "\n\r\t") {
		return false
	}
	switch strings.ToLower(v) {
	case "true", "false", "null", "yes", "no", "on", "off", "~":
		return false // 必须引号，否则会被解析成布尔/空值
	}
	if _, err := strconv.ParseFloat(v, 64); err == nil {
		return false // 数字形态必须引号，否则会被解析成数字
	}
	if strings.ContainsRune("-?:,[]{}#&*!|>'\"%@`", rune(v[0])) {
		return false
	}
	if strings.ContainsAny(v, ",[]{}#&*!|>'\"%@`:") {
		return false
	}
	return true
}

// renderQuoted 渲染值：
//   - 原本就带引号的，沿用同一种引号（保证改回原值时字节一致）
//   - 原本是裸标量的，仅在必要时加单引号（避免 true/0 被误写成字符串）
func renderQuoted(origToken, v string) string {
	if s := quoteStyleOf(origToken); s != 0 {
		return quoteText(s, v)
	}
	return quoteText(0, v)
}

// renderScalarToken 依据原 token 的引号风格渲染新值
func renderScalarToken(origToken, v string) string {
	return renderQuoted(origToken, v)
}

// renderScalarTokenTagged 依据原 token 的引号风格 + YAML 解析出的 tag 渲染新值。
// tag 为 !!bool / !!int 等时保持裸标量，避免把 true 写成 'true'。
func renderScalarTokenTagged(origToken, tag, v string) string {
	switch tag {
	case "!!bool":
		lv := strings.ToLower(strings.TrimSpace(v))
		if lv == "true" || lv == "false" {
			return lv
		}
	case "!!int", "!!float":
		if _, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return strings.TrimSpace(v)
		}
	case "!!null":
		if strings.TrimSpace(v) == "" || strings.EqualFold(v, "null") {
			return "null"
		}
	}
	return renderQuoted(origToken, v)
}

// renderList 渲染流式列表
func renderList(items []string) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, quoteText('\'', it))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// renderBlockLiteral 生成字面量块（用于多行命令），保证值可精确还原
func renderBlockLiteral(key, value string, keyIndent int) []string {
	chomp := ""
	switch {
	case !strings.HasSuffix(value, "\n"):
		chomp = "-"
	case strings.HasSuffix(value, "\n\n"):
		chomp = "+"
	}
	body := value
	if chomp == "" {
		body = strings.TrimSuffix(body, "\n")
	}
	if chomp == "-" {
		body = strings.TrimSuffix(body, "\n")
	}
	content := strings.Split(body, "\n")

	// 首行以空白开头时需要显式缩进指示符
	indicator := ""
	for _, l := range content {
		if l == "" {
			continue
		}
		if l[0] == ' ' || l[0] == '\t' {
			indicator = "2"
		}
		break
	}

	pad := strings.Repeat(" ", keyIndent)
	top := pad + key + ": |" + indicator + chomp
	out := []string{top}
	cpad := strings.Repeat(" ", keyIndent+2)
	for _, l := range content {
		if l == "" {
			out = append(out, "")
			continue
		}
		out = append(out, cpad+l)
	}
	return out
}

// renderValueFor 依据字段的 schema 类型渲染值。
// kind: auto | string | int | bool | list | block
func renderValueFor(kind, tag, origTok, v string) string {
	switch kind {
	case "bool":
		lv := strings.ToLower(strings.TrimSpace(v))
		if lv == "true" || lv == "false" {
			return lv
		}
		return quoteText('\'', v)
	case "int":
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
		return "0"
	case "list":
		return renderList(splitListInput(v))
	case "string":
		return renderQuoted(origTok, v)
	}
	return renderScalarTokenTagged(origTok, tag, v)
}

// KeyValue 是新增 action 时的字段
type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// Kind: auto | string | int | bool | list；为空按 auto
	Kind string `json:"kind"`
}

// RenderAction 生成一个新 action 的源码行（indent 为 '-' 所在列）
func RenderAction(tag string, fields []KeyValue, indent int, flow bool) []string {
	pad := strings.Repeat(" ", indent)
	cpad := strings.Repeat(" ", indent+2)
	if flow {
		var b strings.Builder
		b.WriteString(pad)
		b.WriteString("- !" + tag + ": ")
		if len(fields) == 0 {
			b.WriteString("{}")
		} else {
			b.WriteString("{")
			for i, f := range fields {
				if i > 0 {
					b.WriteString(", ")
				}
				v := renderValueFor(f.Kind, "", "", f.Value)
				if strings.Contains(f.Value, "\n") {
					v = quoteText('"', f.Value)
				}
				b.WriteString(f.Key + ": " + v)
			}
			b.WriteString("}")
		}
		return []string{b.String()}
	}
	out := []string{pad + "- !" + tag + ":"}
	for _, f := range fields {
		if strings.Contains(f.Value, "\n") {
			out = append(out, renderBlockLiteral(f.Key, f.Value, indent+2)...)
			continue
		}
		out = append(out, cpad+f.Key+": "+renderValueFor(f.Kind, "", "", f.Value))
	}
	return out
}
