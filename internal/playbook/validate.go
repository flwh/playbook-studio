package playbook

import (
	"fmt"
	"sort"
	"strings"
)

// Issue 是一条校验结果
type Issue struct {
	Level   string `json:"level"` // error | warning | info
	File    string `json:"file"`
	Line    int    `json:"line"`
	Index   int    `json:"index"`
	Tag     string `json:"tag"`
	Message string `json:"message"`
}

func (b *Bundle) addIssue(list *[]Issue, level, file string, line, index int, tag, format string, args ...any) {
	*list = append(*list, Issue{
		Level: level, File: file, Line: line, Index: index, Tag: tag,
		Message: fmt.Sprintf(format, args...),
	})
}

// Validate 对整个 bundle 做静态校验
func (b *Bundle) Validate() []Issue {
	var issues []Issue

	options := map[string]bool{}
	if b.Conf != nil {
		options = b.Conf.OptionNames()
	}

	// 任务树中缺失的引用
	if b.Main != "" {
		var walk func(node *TreeNode)
		walk = func(node *TreeNode) {
			if node == nil {
				return
			}
			if node.Missing {
				b.addIssue(&issues, "error", node.Path, 0, -1, "task",
					"引用的任务文件不存在: %s", node.Include)
			}
			for _, c := range node.Children {
				walk(c)
			}
		}
		for _, n := range b.Tree() {
			walk(n)
		}
	}

	// 未被主任务引用的任务文件（孤立文件）
	reachable := map[string]bool{}
	var mark func(node *TreeNode)
	mark = func(node *TreeNode) {
		if node == nil || node.Missing || node.Cycle {
			return
		}
		reachable[node.Path] = true
		for _, c := range node.Children {
			mark(c)
		}
	}
	for _, n := range b.Tree() {
		mark(n)
	}

	rel := func(t *Task) string { return t.Path }

	for _, key := range sortedKeys(b.Tasks) {
		t := b.Tasks[key]
		if b.Main != "" && key != b.Main && !reachable[key] {
			b.addIssue(&issues, "info", key, 0, -1, "",
				"该任务文件没有被主任务引用")
		}
		if t.Meta.Privilege == "" {
			b.addIssue(&issues, "warning", key, 0, -1, "", "缺少 privilege 字段")
		} else if !validPrivilege(t.Meta.Privilege) {
			b.addIssue(&issues, "warning", key, 0, -1, "",
				"未知的 privilege: %s", t.Meta.Privilege)
		}
		for i := range t.Actions {
			a := &t.Actions[i]
			spec, known := TagSpecOf(a.Tag)
			if !known {
				b.addIssue(&issues, "warning", key, a.Line, i, a.Tag,
					"未知的 action 类型: !%s", a.Tag)
			} else {
				for _, f := range spec.Fields {
					if !f.Required {
						continue
					}
					if v, ok := a.FieldValue(f.Key); !ok || strings.TrimSpace(v) == "" {
						b.addIssue(&issues, "error", key, a.Line, i, a.Tag,
							"缺少必填字段 %s", f.Key)
					}
				}
			}
			// option / options 同时出现
			if a.Option != "" && len(a.Options) > 0 {
				b.addIssue(&issues, "warning", key, a.Line, i, a.Tag,
					"同时使用了 option 与 options，行为可能不符合预期")
			}
			// option 是否在 playbook.conf 中定义过
			for _, opt := range append([]string{a.Option}, a.Options...) {
				o := strings.TrimPrefix(strings.TrimSpace(opt), "!")
				if o == "" {
					continue
				}
				if b.Conf != nil && !options[o] {
					b.addIssue(&issues, "warning", key, a.Line, i, a.Tag,
						"option %q 未在 playbook.conf 的 FeaturePages 中定义", o)
				}
			}
			// 各类型的细节校验
			switch a.Tag {
			case "registryValue":
				op, _ := a.FieldValue("operation")
				data, hasData := a.FieldValue("data")
				typ, _ := a.FieldValue("type")
				if op != "delete" {
					if !hasData || data == "" {
						b.addIssue(&issues, "warning", key, a.Line, i, a.Tag,
							"没有 data，将写入空值")
					}
					if typ == "REG_DWORD" && data != "" && !isNumeric(data) {
						b.addIssue(&issues, "warning", key, a.Line, i, a.Tag,
							"REG_DWORD 的 data 不是数字: %s", data)
					}
				}
				if op == "delete" && !hasData && a.Option == "" {
					b.addIssue(&issues, "info", key, a.Line, i, a.Tag,
						"删除整个注册表值")
				}
			case "registryKey":
				op, _ := a.FieldValue("operation")
				if op == "" {
					b.addIssue(&issues, "warning", key, a.Line, i, a.Tag,
						"缺少 operation(add/delete)")
				}
			case "powerShell", "cmd":
				if v, _ := a.FieldValue("command"); strings.TrimSpace(v) == "" {
					b.addIssue(&issues, "error", key, a.Line, i, a.Tag, "命令为空")
				}
			case "run":
				if v, _ := a.FieldValue("exe"); strings.TrimSpace(v) == "" {
					b.addIssue(&issues, "error", key, a.Line, i, a.Tag, "缺少 exe")
				}
			case "appx":
				if v, _ := a.FieldValue("operation"); v == "" {
					b.addIssue(&issues, "error", key, a.Line, i, a.Tag, "缺少 operation")
				}
			case "service":
				st, hasSt := a.FieldValue("startup")
				if hasSt && st != "" && !isNumeric(st) {
					b.addIssue(&issues, "warning", key, a.Line, i, a.Tag,
						"startup 应为 0-4 的数字")
				}
			}
		}
		_ = rel
	}

	sort.Slice(issues, func(i, j int) bool {
		rank := func(l string) int {
			switch l {
			case "error":
				return 0
			case "warning":
				return 1
			}
			return 2
		}
		if rank(issues[i].Level) != rank(issues[j].Level) {
			return rank(issues[i].Level) < rank(issues[j].Level)
		}
		if issues[i].File != issues[j].File {
			return issues[i].File < issues[j].File
		}
		return issues[i].Line < issues[j].Line
	})
	return issues
}

func validPrivilege(p string) bool {
	switch strings.ToLower(p) {
	case "admin", "trustedinstaller", "system", "currentuser", "user":
		return true
	}
	return false
}

func isNumeric(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		_, err := fmt.Sscanf(s, "%x", new(uint64))
		return err == nil
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
