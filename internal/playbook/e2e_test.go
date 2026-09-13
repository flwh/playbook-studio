package playbook

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"playbookstudio/internal/sevenzip"
)

// 端到端：真实 .apbx → 解包 → 编辑 → 重新打包 → 再解包比对
// 需要设置环境变量 PLAYBOOK_APBX 与 PLAYBOOK_PASSWORD
func TestEndToEndRealBundle(t *testing.T) {
	archive := os.Getenv("PLAYBOOK_APBX")
	password := os.Getenv("PLAYBOOK_PASSWORD")
	if archive == "" {
		t.Skip("未设置 PLAYBOOK_APBX，跳过端到端测试")
	}
	sz := sevenzip.New()
	if !sz.Available() {
		t.Skip("未找到 7z")
	}

	t.Logf("7z: %s", sz.ExePath())

	dir1, err := os.MkdirTemp("", "pbs-e2e-a-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir1)
	b1, err := OpenBundle(sz, archive, password, dir1)
	if err != nil {
		t.Fatalf("解包失败: %v", err)
	}
	t.Logf("任务文件 %d 个, 文件 %d 个, 主任务 %s", len(b1.Tasks), len(b1.Files), b1.Main)
	if b1.Conf == nil {
		t.Fatal("没有解析到 playbook.conf")
	}
	s := b1.Conf.Summary()
	t.Logf("playbook: %s / %s v%s, 支持构建 %v", s.Name, s.Title, s.Version, s.SupportedBuilds)

	pages := b1.Conf.OptionPages()
	totalOpts := 0
	for _, p := range pages {
		totalOpts += len(p.Options)
	}
	t.Logf("FeaturePages %d 页 %d 个选项, 软件包 %d 个", len(pages), totalOpts, len(b1.Conf.SoftwarePackages()))

	tree := b1.Tree()
	var countNodes func(nodes []*TreeNode) int
	countNodes = func(nodes []*TreeNode) int {
		n := 0
		for _, x := range nodes {
			n++
			n += countNodes(x.Children)
		}
		return n
	}
	t.Logf("任务树节点 %d 个", countNodes(tree))

	issues := b1.Validate()
	byLevel := map[string]int{}
	for _, i := range issues {
		byLevel[i.Level]++
	}
	t.Logf("校验结果: error=%d warning=%d info=%d", byLevel["error"], byLevel["warning"], byLevel["info"])
	for i, it := range issues {
		if i >= 5 {
			break
		}
		t.Logf("   [%s] %s:%d !%s %s", it.Level, it.File, it.Line, it.Tag, it.Message)
	}

	// ---- 编辑 ----
	mainPath := b1.Main
	mainTask, err := b1.GetTask(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(mainTask.Actions) == 0 {
		t.Fatal("主任务没有动作")
	}

	// 1) 修改第一个 !task 引用的 option（无则新增一个字段）
	editedField := ""
	for i := range mainTask.Actions {
		a := &mainTask.Actions[i]
		if a.Tag != "task" {
			continue
		}
		if err := mainTask.SetFieldTyped(i, "option", "e2e-flag", "string"); err != nil {
			t.Fatalf("SetField 失败: %v", err)
		}
		v, ok := mainTask.Actions[i].FieldValue("option")
		if !ok || v != "e2e-flag" {
			t.Fatalf("字段写入后回读不一致: %q", v)
		}
		editedField = "option"
		break
	}
	if editedField == "" {
		t.Skip("主任务没有 !task 动作，跳过")
	}
	b1.MarkDirty(mainPath)

	// 2) 新增一个动作
	if err := mainTask.InsertAction(len(mainTask.Actions), "writeStatus",
		[]KeyValue{{Key: "status", Value: "E2E 测试"}}, false); err != nil {
		t.Fatalf("InsertAction 失败: %v", err)
	}
	// 3) 复制再删除，确保计数正确
	if err := mainTask.DuplicateAction(0); err != nil {
		t.Fatal(err)
	}
	if err := mainTask.DeleteAction(0); err != nil {
		t.Fatal(err)
	}

	// 4) 修改 conf 的 Version
	if err := b1.Conf.SetText("Version", 0, "99.99"); err != nil {
		t.Fatalf("SetText 失败: %v", err)
	}
	b1.MarkDirty("playbook.conf")

	// ---- 保存 ----
	out := filepath.Join(os.TempDir(), "pbs-e2e-out.apbx")
	defer os.Remove(out)
	if err := b1.SaveAs(sz, out); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		t.Fatalf("输出文件异常: %v", err)
	}
	t.Logf("重新打包完成: %s (%d 字节)", out, sevenzip.Size(out))

	// ---- 重新打开验证 ----
	dir2, err := os.MkdirTemp("", "pbs-e2e-b-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir2)
	b2, err := OpenBundle(sz, out, password, dir2)
	if err != nil {
		t.Fatalf("重新解包失败: %v", err)
	}
	if got := b2.Conf.Summary().Version; got != "99.99" {
		t.Errorf("conf 版本未生效: %q", got)
	}
	t2, err := b2.GetTask(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i := range t2.Actions {
		if v, ok := t2.Actions[i].FieldValue("option"); ok && v == "e2e-flag" {
			found = true
		}
	}
	if !found {
		t.Error("新增的 option 字段没有生效")
	}
	statusFound := false
	for i := range t2.Actions {
		if t2.Actions[i].Tag == "writeStatus" {
			if v, _ := t2.Actions[i].FieldValue("status"); v == "E2E 测试" {
				statusFound = true
			}
		}
	}
	if !statusFound {
		t.Error("新增的 writeStatus 动作没有生效")
	}
	if len(t2.Actions) != len(mainTask.Actions) {
		t.Errorf("动作数量不一致: %d vs %d", len(t2.Actions), len(mainTask.Actions))
	}

	// ---- 其他文件必须逐字节一致 ----
	changed := map[string]bool{mainPath: true, "playbook.conf": true}
	a := hashTree(t, b1.WorkDir)
	b := hashTree(t, b2.WorkDir)
	if len(a) != len(b) {
		t.Errorf("文件数量不一致: %d vs %d", len(a), len(b))
	}
	diffCount := 0
	for k, v := range a {
		if changed[k] {
			continue
		}
		w, ok := b[k]
		if !ok {
			t.Errorf("文件丢失: %s", k)
			continue
		}
		if v != w {
			diffCount++
			if diffCount <= 5 {
				t.Errorf("未编辑的文件被改动: %s", k)
			}
		}
	}
	if diffCount == 0 {
		t.Logf("✓ 除被编辑的 %d 个文件外，其余全部逐字节一致", len(changed))
	}

	// YAML 可被正常解析
	if err := t2.SetText(t2.Text); err != nil {
		t.Errorf("重新解析失败: %v", err)
	}
}

func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		out[filepath.ToSlash(rel)] = fmt.Sprintf("%x", md5.Sum(data))
		return nil
	})
	return out
}

func TestSortedKeysHelper(t *testing.T) {
	in := map[string]int{"b": 1, "a": 2}
	got := sortedKeys(in)
	if strings.Join(got, ",") != "a,b" {
		t.Errorf("排序异常: %v", got)
	}
	if !sort.StringsAreSorted(got) {
		t.Error("未排序")
	}
}
