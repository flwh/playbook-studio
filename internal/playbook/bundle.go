package playbook

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"playbookstudio/internal/sevenzip"
)

// FileInfo 描述 bundle 内的一个文件
type FileInfo struct {
	Path  string `json:"path"` // bundle 内相对路径（反斜杠）
	Size  int64  `json:"size"`
	IsYml bool   `json:"isYml"`
}

// TreeNode 是展开后的任务树
type TreeNode struct {
	Path        string      `json:"path"`
	Include     string      `json:"include"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Privilege   string      `json:"privilege"`
	ActionCount int         `json:"actionCount"`
	Missing     bool        `json:"missing"`
	Cycle       bool        `json:"cycle"`
	Children    []*TreeNode `json:"children"`
}

// Bundle 是一次打开的 .apbx 会话
type Bundle struct {
	ArchivePath string
	Password    string
	WorkDir     string

	Conf  *Conf
	Tasks map[string]*Task // key: bundle 内相对路径（反斜杠）
	Files []FileInfo
	Main  string // 主任务文件（Configuration\main.yml）

	dirty map[string]bool
	order []string
}

// RelPath 把系统路径转换成 bundle 内的相对路径（反斜杠）
func RelPath(workDir, full string) string {
	r, err := filepath.Rel(workDir, full)
	if err != nil {
		return full
	}
	return strings.ReplaceAll(r, "/", "\\")
}

// IncludedPath 把 bundle 相对路径转成 DSL 里使用的写法（相对 Configuration，反斜杠）
func IncludedPath(bundleRel string) string {
	s := bundleRel
	if strings.HasPrefix(strings.ToLower(s), "configuration\\") {
		s = s[len("configuration\\"):]
	}
	return s
}

// OpenBundle 解包并载入
func OpenBundle(sz *sevenzip.Manager, archive, password, workDir string) (*Bundle, error) {
	if !sz.Available() {
		return nil, sevenzip.ErrNotFound
	}
	if _, err := os.Stat(archive); err != nil {
		return nil, fmt.Errorf("打不开归档: %w", err)
	}
	if workDir == "" {
		d, err := os.MkdirTemp("", "playbook-studio-")
		if err != nil {
			return nil, err
		}
		workDir = d
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, err
	}
	// 清理旧内容，避免残留文件被再次打包
	entries, _ := os.ReadDir(workDir)
	for _, e := range entries {
		os.RemoveAll(filepath.Join(workDir, e.Name()))
	}
	if err := sz.Extract(archive, workDir, password); err != nil {
		return nil, err
	}

	b := &Bundle{
		ArchivePath: archive,
		Password:    password,
		WorkDir:     workDir,
		Tasks:       map[string]*Task{},
		dirty:       map[string]bool{},
	}

	// playbook.conf
	confPath := filepath.Join(workDir, "playbook.conf")
	if data, err := os.ReadFile(confPath); err == nil {
		c, err := ParseConf("playbook.conf", data)
		if err != nil {
			return nil, err
		}
		b.Conf = c
	}

	// 收集文件 & 解析 YAML
	_ = filepath.Walk(workDir, func(p string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel := RelPath(workDir, p)
		lower := strings.ToLower(rel)
		isYml := strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")
		b.Files = append(b.Files, FileInfo{Path: rel, Size: info.Size(), IsYml: isYml})
		if isYml && strings.HasPrefix(lower, "configuration\\") {
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			t, err := ParseTask(rel, data)
			if err != nil {
				return nil
			}
			b.Tasks[rel] = t
		}
		return nil
	})
	sort.Slice(b.Files, func(i, j int) bool { return b.Files[i].Path < b.Files[j].Path })

	// 主任务
	for _, cand := range []string{"Configuration\\main.yml", "Configuration\\main.yaml"} {
		if _, ok := b.Tasks[cand]; ok {
			b.Main = cand
			break
		}
	}
	if b.Main == "" {
		for k := range b.Tasks {
			if strings.EqualFold(k, "configuration\\main.yml") {
				b.Main = k
			}
		}
	}
	return b, nil
}

// TaskByInclude 按 DSL 里的写法查找任务
func (b *Bundle) TaskByInclude(inc string) (*Task, bool) {
	norm := strings.ReplaceAll(strings.TrimSpace(inc), "/", "\\")
	norm = strings.TrimPrefix(norm, ".\\")
	key := "Configuration\\" + norm
	if t, ok := b.Tasks[key]; ok {
		return t, true
	}
	for k, t := range b.Tasks {
		if strings.EqualFold(k, key) {
			return t, true
		}
	}
	return nil, false
}

// Tree 从主任务展开任务树
func (b *Bundle) Tree() []*TreeNode {
	if b.Main == "" {
		return nil
	}
	seen := map[string]bool{}
	var walk func(rel string) *TreeNode
	walk = func(rel string) *TreeNode {
		t, ok := b.Tasks[rel]
		if !ok {
			return &TreeNode{Path: rel, Include: IncludedPath(rel), Missing: true, Title: filepath.Base(rel)}
		}
		node := &TreeNode{
			Path: rel, Include: IncludedPath(rel),
			Title: t.Meta.Title, Description: t.Meta.Description,
			Privilege: t.Meta.Privilege, ActionCount: len(t.Actions),
		}
		if seen[rel] {
			node.Cycle = true
			return node
		}
		seen[rel] = true
		defer delete(seen, rel)
		for i := range t.Actions {
			a := &t.Actions[i]
			if a.Tag != "task" {
				continue
			}
			inc, _ := a.FieldValue("path")
			if inc == "" {
				continue
			}
			child, ok := b.TaskByInclude(inc)
			if !ok {
				node.Children = append(node.Children, &TreeNode{Include: inc, Missing: true, Title: inc})
				continue
			}
			node.Children = append(node.Children, walk(child.Path))
		}
		return node
	}
	return []*TreeNode{walk(b.Main)}
}

// IsDirty 是否有未保存修改
func (b *Bundle) IsDirty() bool { return len(b.dirty) > 0 }

// DirtyFiles 未保存文件列表
func (b *Bundle) DirtyFiles() []string {
	out := make([]string, 0, len(b.dirty))
	for k := range b.dirty {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// MarkDirty 标记文件已修改
func (b *Bundle) MarkDirty(rel string) {
	if b.dirty == nil {
		b.dirty = map[string]bool{}
	}
	b.dirty[rel] = true
}

// GetTask 取任务（不存在时报错）
func (b *Bundle) GetTask(rel string) (*Task, error) {
	if t, ok := b.Tasks[rel]; ok {
		return t, nil
	}
	for k, t := range b.Tasks {
		if strings.EqualFold(k, rel) {
			return t, nil
		}
	}
	return nil, fmt.Errorf("任务不存在: %s", rel)
}

// WriteBack 把改动写回解包目录
func (b *Bundle) WriteBack() error {
	for rel := range b.dirty {
		if rel == "playbook.conf" {
			if b.Conf == nil {
				continue
			}
			if err := os.WriteFile(filepath.Join(b.WorkDir, "playbook.conf"), b.Conf.Bytes(), 0o644); err != nil {
				return err
			}
			continue
		}
		t, ok := b.Tasks[rel]
		if !ok {
			continue
		}
		full := filepath.Join(b.WorkDir, filepath.FromSlash(strings.ReplaceAll(rel, "\\", "/")))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, t.Bytes(), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// SaveAs 写回并重新打包
func (b *Bundle) SaveAs(sz *sevenzip.Manager, outArchive string) error {
	if err := b.WriteBack(); err != nil {
		return err
	}
	if outArchive == "" {
		outArchive = b.ArchivePath
	}
	if err := sz.Pack(b.WorkDir, outArchive, b.Password, true, 7); err != nil {
		return err
	}
	b.dirty = map[string]bool{}
	b.ArchivePath = outArchive
	return nil
}

// ReadFile 读取任意文本文件
func (b *Bundle) ReadFile(rel string) (string, error) {
	full := filepath.Join(b.WorkDir, filepath.FromSlash(strings.ReplaceAll(rel, "\\", "/")))
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteFile 写入任意文本文件并标记为已修改（保留原文件换行风格）
func (b *Bundle) WriteFile(rel, content string) error {
	full := filepath.Join(b.WorkDir, filepath.FromSlash(strings.ReplaceAll(rel, "\\", "/")))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	norm := strings.ReplaceAll(content, "\r\n", "\n")
	crlf := true
	if old, err := os.ReadFile(full); err == nil {
		crlf = strings.Contains(string(old), "\r\n")
	}
	out := norm
	if crlf {
		out = strings.ReplaceAll(norm, "\n", "\r\n")
	}
	if err := os.WriteFile(full, []byte(out), 0o644); err != nil {
		return err
	}
	if t, ok := b.Tasks[rel]; ok {
		t.CRLF = crlf
		return t.SetText(norm)
	}
	return nil
}

// Cleanup 删除临时解包目录
func (b *Bundle) Cleanup() {
	if b.WorkDir != "" && strings.Contains(path.Base(b.WorkDir), "playbook-studio-") {
		os.RemoveAll(b.WorkDir)
	}
}
