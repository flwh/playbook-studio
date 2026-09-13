package playbook

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleDir(t *testing.T) string {
	t.Helper()
	d := os.Getenv("PLAYBOOK_SAMPLE_DIR")
	if d == "" {
		t.Skip("未设置 PLAYBOOK_SAMPLE_DIR，跳过基于样例的测试")
	}
	if _, err := os.Stat(d); err != nil {
		t.Skipf("样例目录不存在: %s", d)
	}
	return d
}

func collectYaml(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func load(t *testing.T, path string) (*Task, []byte) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rel, _ := filepath.Rel(sampleDir(t), path)
	tk, err := ParseTask(filepath.ToSlash(rel), b)
	if err != nil {
		t.Fatalf("解析失败 %s: %v", path, err)
	}
	return tk, b
}

// 无操作往返必须字节一致（因为已存在的 action 从不整体重新序列化）
func TestRoundTripBytesIdentical(t *testing.T) {
	root := sampleDir(t)
	files := collectYaml(t, root)
	if len(files) == 0 {
		t.Fatal("没有找到 YAML 文件")
	}
	for _, f := range files {
		tk, orig := load(t, f)
		if !bytes.Equal(tk.Bytes(), orig) {
			t.Errorf("字节不一致: %s", f)
		}
	}
	t.Logf("共验证 %d 个文件", len(files))
}

func diffLines(a, b string) (changed int, firstA, firstB string) {
	al := strings.Split(a, "\n")
	bl := strings.Split(b, "\n")
	n := len(al)
	if len(bl) > n {
		n = len(bl)
	}
	for i := 0; i < n; i++ {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			if changed == 0 {
				firstA, firstB = x, y
			}
			changed++
		}
	}
	return
}

// 修改单个字段：只允许一行发生变化，且改动后重新解析能得到新值
func TestSetFieldTouchesSingleLine(t *testing.T) {
	root := sampleDir(t)
	for _, f := range collectYaml(t, root) {
		tk, orig := load(t, f)
		if len(tk.Actions) == 0 {
			continue
		}
		for i := range tk.Actions {
			a := tk.Actions[i]
			// 找一个单行标量字段
			var key, val string
			for _, fl := range a.Fields {
				// 仅针对字符串标量做“只改一行”的验证
				if fl.Kind == "scalar" && fl.Tag == "!!str" && fl.Value != "" && !strings.Contains(fl.Value, "\n") {
					key, val = fl.Key, fl.Value
					break
				}
			}
			if key == "" {
				continue
			}
			newVal := val + "-X"
			if err := tk.SetField(i, key, newVal); err != nil {
				t.Fatalf("%s action[%d] SetField(%s) 失败: %v", f, i, key, err)
			}
			changed, la, lb := diffLines(strings.ReplaceAll(string(orig), "\r\n", "\n"), tk.Text)
			if changed != 1 {
				t.Errorf("%s action[%d] 字段 %s 改动影响了 %d 行\n  原: %q\n  新: %q", f, i, key, changed, la, lb)
			}
			got, ok := tk.Actions[i].FieldValue(key)
			if !ok || got != newVal {
				t.Errorf("%s action[%d] 字段 %s 回读值错误: 期望 %q 实际 %q", f, i, key, newVal, got)
			}
			// 恢复
			if err := tk.SetField(i, key, val); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(tk.Bytes(), orig) {
				t.Errorf("%s action[%d] 字段 %s 改回原值后未能完全还原", f, i, key)
			}
		}
	}
}

// 带类型设置：bool / int 字段改值后必须保持裸标量，且能字节级还原
func TestSetFieldTyped(t *testing.T) {
	root := sampleDir(t)
	for _, f := range collectYaml(t, root) {
		tk, orig := load(t, f)
		for i := range tk.Actions {
			a := tk.Actions[i]
			for _, fl := range a.Fields {
				var kind, newVal string
				switch {
				case fl.Kind == "scalar" && fl.Tag == "!!bool":
					kind, newVal = "bool", "false"
					if fl.Value == "false" {
						newVal = "true"
					}
				case fl.Kind == "scalar" && fl.Tag == "!!int":
					kind, newVal = "int", "123"
				default:
					continue
				}
				if err := tk.SetFieldTyped(i, fl.Key, newVal, kind); err != nil {
					t.Fatalf("%s action[%d] SetFieldTyped(%s): %v", f, i, fl.Key, err)
				}
				got, _ := tk.Actions[i].FieldValue(fl.Key)
				if got != newVal {
					t.Errorf("%s action[%d] %s 期望 %q 实际 %q", f, i, fl.Key, newVal, got)
				}
				if strings.Contains(tk.Actions[i].Raw, "'"+newVal+"'") {
					t.Errorf("%s action[%d] %s 被错误地加了引号: %s", f, i, fl.Key, tk.Actions[i].Raw)
				}
				if err := tk.SetFieldTyped(i, fl.Key, fl.Value, kind); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(tk.Bytes(), orig) {
					t.Errorf("%s action[%d] %s 改回原值后未能完全还原", f, i, fl.Key)
				}
			}
		}
	}
}

// 删除字段后必须能还原
func TestDeleteFieldRestores(t *testing.T) {
	root := sampleDir(t)
	for _, f := range collectYaml(t, root) {
		tk, orig := load(t, f)
		if len(tk.Actions) == 0 {
			continue
		}
		for i := range tk.Actions {
			a := tk.Actions[i]
			for _, fl := range a.Fields {
				if fl.Key == "path" || fl.Key == "exe" || fl.Key == "name" {
					continue // 关键字段跳过
				}
				snapshot := tk.Text
				if err := tk.DeleteField(i, fl.Key); err != nil {
					t.Fatalf("%s action[%d] DeleteField(%s): %v", f, i, fl.Key, err)
				}
				if _, ok := tk.Actions[i].FieldValue(fl.Key); ok {
					t.Errorf("%s action[%d] 删除字段 %s 后仍然存在", f, i, fl.Key)
				}
				// 还原快照继续下一个
				if err := tk.SetText(snapshot); err != nil {
					t.Fatal(err)
				}
			}
		}
		if !bytes.Equal(tk.Bytes(), orig) {
			t.Errorf("%s 多次删除/还原后未能完全还原", f)
		}
	}
}

// action 的增删/复制/移动
func TestActionOps(t *testing.T) {
	root := sampleDir(t)
	for _, f := range collectYaml(t, root) {
		tk, orig := load(t, f)
		n := len(tk.Actions)
		if n == 0 {
			continue
		}
		// 删除第一个
		if err := tk.DeleteAction(0); err != nil {
			t.Fatalf("%s DeleteAction: %v", f, err)
		}
		if len(tk.Actions) != n-1 {
			t.Errorf("%s 删除后数量错误: %d -> %d", f, n, len(tk.Actions))
		}
		if err := tk.SetText(string(orig)); err != nil {
			t.Fatal(err)
		}
		// 复制第一个
		if err := tk.DuplicateAction(0); err != nil {
			t.Fatalf("%s DuplicateAction: %v", f, err)
		}
		if len(tk.Actions) != n+1 {
			t.Errorf("%s 复制后数量错误: %d -> %d", f, n, len(tk.Actions))
		}
		if tk.Actions[0].Raw != tk.Actions[1].Raw {
			t.Errorf("%s 复制结果不一致:\n%q\n%q", f, tk.Actions[0].Raw, tk.Actions[1].Raw)
		}
		if err := tk.SetText(string(orig)); err != nil {
			t.Fatal(err)
		}
		// 移动首项到末尾
		if err := tk.MoveAction(0, n-1); err != nil {
			t.Fatalf("%s MoveAction: %v", f, err)
		}
		if len(tk.Actions) != n {
			t.Errorf("%s 移动后数量错误: %d", f, len(tk.Actions))
		}
		if err := tk.SetText(string(orig)); err != nil {
			t.Fatal(err)
		}
		// 插入新 action
		err := tk.InsertAction(0, "writeStatus", []KeyValue{{Key: "status", Value: "hello"}}, false)
		if err != nil {
			t.Fatalf("%s InsertAction: %v", f, err)
		}
		if len(tk.Actions) != n+1 || tk.Actions[0].Tag != "writeStatus" {
			t.Errorf("%s 插入失败: %d tag=%q", f, len(tk.Actions), tk.Actions[0].Tag)
		}
		v, _ := tk.Actions[0].FieldValue("status")
		if v != "hello" {
			t.Errorf("%s 插入字段值错误: %q", f, v)
		}
		if err := tk.SetText(string(orig)); err != nil {
			t.Fatal(err)
		}
	}
}

// 元数据编辑
func TestSetMeta(t *testing.T) {
	root := sampleDir(t)
	f := filepath.Join(root, "Tasks", "start.yml")
	if _, err := os.Stat(f); err != nil {
		t.Skip("样例不完整")
	}
	tk, _ := load(t, f)
	if err := tk.SetMeta("title", "初始化的"); err != nil {
		t.Fatal(err)
	}
	if tk.Meta.Title != "初始化的" {
		t.Errorf("title 未更新: %q", tk.Meta.Title)
	}
	if err := tk.SetMeta("dependsOn", "x"); err != nil {
		t.Fatal(err)
	}
}
