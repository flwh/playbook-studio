package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"playbookstudio/internal/playbook"
	"playbookstudio/internal/sevenzip"
)

// StatusDTO 应用/环境状态
type StatusDTO struct {
	SevenZip       string `json:"sevenZip"`
	SevenZipOK     bool   `json:"sevenZipOk"`
	SevenZipSource string `json:"sevenZipSource"` // builtin | system | custom
	HasBundle      bool   `json:"hasBundle"`
	Archive        string `json:"archive"`
	Dirty          bool   `json:"dirty"`
	LastOpenDir    string `json:"lastOpenDir"`
}

// ConfDTO 是 playbook.conf 的可编辑视图
type ConfDTO struct {
	Summary  playbook.ConfSummary      `json:"summary"`
	Fields   []playbook.ConfField      `json:"fields"`
	Attrs    []playbook.ConfAttrItem   `json:"attrs"`
	Pages    []playbook.ConfOptionPage `json:"pages"`
	Packages []playbook.ConfPackage    `json:"packages"`
	Options  []string                  `json:"options"`
	Raw      string                    `json:"raw"`
}

// BundleDTO 是打开后的 bundle 视图
type BundleDTO struct {
	Archive   string                `json:"archive"`
	WorkDir   string                `json:"workDir"`
	Main      string                `json:"main"`
	Summary   playbook.ConfSummary  `json:"summary"`
	Conf      *ConfDTO              `json:"conf"`
	Files     []playbook.FileInfo   `json:"files"`
	Tree      []*playbook.TreeNode  `json:"tree"`
	TaskPaths []string              `json:"taskPaths"`
	Dirty     []string              `json:"dirty"`
}

// SchemaDTO 是 action 类型定义
type SchemaDTO struct {
	Tags          []playbook.TagSpec   `json:"tags"`
	Common        []playbook.FieldSpec `json:"common"`
	Groups        []string             `json:"groups"`
	Privileges    []string             `json:"privileges"`
	RunAsOptions  []string             `json:"runAsOptions"`
	RegistryTypes []string             `json:"registryTypes"`
}

// SaveResult 保存结果
type SaveResult struct {
	Archive string `json:"archive"`
	Size    int64  `json:"size"`
	Files   int    `json:"files"`
}

// StartupDTO 命令行/文件关联传入的启动参数
type StartupDTO struct {
	Archive  string `json:"archive"`
	Password string `json:"password"`
}

// PlaybookService 暴露给前端的服务
type PlaybookService struct {
	mu           sync.Mutex
	sz           *sevenzip.Manager
	bundle       *playbook.Bundle
	lastOpenDir  string
	startArchive string
	startPass    string

	win            *application.WebviewWindow
	closeConfirmed bool
}

// NewPlaybookService 创建服务
func NewPlaybookService() *PlaybookService {
	return &PlaybookService{sz: sevenzip.New()}
}

// SetStartupArgs 记录命令行传入的归档路径与密码（文件关联/自动化用）
func (s *PlaybookService) SetStartupArgs(archive, password string) {
	s.startArchive = archive
	s.startPass = password
}

// ---------- 自绘窗口控制 ----------

// AttachWindow 绑定窗口并挂上关闭拦截（有未保存修改时交给前端确认）
func (s *PlaybookService) AttachWindow(w *application.WebviewWindow) {
	s.mu.Lock()
	s.win = w
	s.mu.Unlock()
	if w == nil {
		return
	}
	w.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		s.mu.Lock()
		force := s.closeConfirmed
		dirty := s.bundle != nil && s.bundle.IsDirty()
		s.mu.Unlock()
		log.Printf("WindowClosing: force=%v dirty=%v", force, dirty)
		if force || !dirty {
			return
		}
		// 取消本次关闭，交给前端弹出确认框
		e.Cancel()
		go application.Get().Event.Emit("app:close-request", true)
	})
}

// WindowCloseRequest 前端点击自绘关闭按钮
func (s *PlaybookService) WindowCloseRequest() string {
	s.mu.Lock()
	if s.bundle != nil && s.bundle.IsDirty() {
		s.mu.Unlock()
		return "dirty"
	}
	s.closeConfirmed = true
	w := s.win
	s.mu.Unlock()
	if w != nil {
		w.Close()
	}
	return "closed"
}

// WindowForceClose 确认丢弃修改后真正关闭
func (s *PlaybookService) WindowForceClose() {
	s.mu.Lock()
	s.closeConfirmed = true
	w := s.win
	s.mu.Unlock()
	if w != nil {
		w.Close()
	}
}

// WindowMinimise 最小化
func (s *PlaybookService) WindowMinimise() {
	s.mu.Lock()
	w := s.win
	s.mu.Unlock()
	if w != nil {
		w.Minimise()
	}
}

// WindowToggleMaximise 最大化/还原
func (s *PlaybookService) WindowToggleMaximise() bool {
	s.mu.Lock()
	w := s.win
	s.mu.Unlock()
	if w == nil {
		return false
	}
	if w.IsMaximised() {
		w.UnMaximise()
		return false
	}
	w.Maximise()
	return true
}

// WindowIsMaximised 当前是否最大化
func (s *PlaybookService) WindowIsMaximised() bool {
	s.mu.Lock()
	w := s.win
	s.mu.Unlock()
	if w == nil {
		return false
	}
	return w.IsMaximised()
}

// StartupFile 取出并清空启动参数
func (s *PlaybookService) StartupFile() StartupDTO {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := StartupDTO{Archive: s.startArchive, Password: s.startPass}
	s.startArchive, s.startPass = "", ""
	return d
}

func (s *PlaybookService) requireBundle() (*playbook.Bundle, error) {
	if s.bundle == nil {
		return nil, errors.New("尚未打开任何 playbook")
	}
	return s.bundle, nil
}

// ---------- 环境 ----------

// Status 返回当前状态
func (s *PlaybookService) Status() StatusDTO {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := StatusDTO{
		SevenZip:       s.sz.ExePath(),
		SevenZipOK:     s.sz.Available(),
		SevenZipSource: s.sz.Source(),
		LastOpenDir:    s.lastOpenDir,
	}
	if s.bundle != nil {
		st.HasBundle = true
		st.Archive = s.bundle.ArchivePath
		st.Dirty = s.bundle.IsDirty()
	}
	return st
}

// SetSevenZipPath 手动指定 7z 路径
func (s *PlaybookService) SetSevenZipPath(p string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sz.SetExePath(p)
}

// PickArchive 选择 .apbx 文件
func (s *PlaybookService) PickArchive() (string, error) {
	d := application.Get().Dialog.OpenFile().
		SetTitle("选择 Playbook 归档 (.apbx)")
	if s.lastOpenDir != "" {
		d = d.SetDirectory(s.lastOpenDir)
	}
	p, err := d.AddFilter("Playbook 归档", "*.apbx;*.7z").PromptForSingleSelection()
	if err != nil || p == "" {
		return "", err
	}
	s.mu.Lock()
	s.lastOpenDir = filepath.Dir(p)
	s.mu.Unlock()
	return p, nil
}

// PickSavePath 选择保存位置
func (s *PlaybookService) PickSavePath(defaultName string) (string, error) {
	if defaultName == "" {
		defaultName = "playbook.apbx"
	}
	p, err := application.Get().Dialog.SaveFile().
		SetMessage("导出 Playbook 归档").
		SetFilename(defaultName).
		AddFilter("Playbook 归档", "*.apbx").
		PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	if p != "" && !strings.HasSuffix(strings.ToLower(p), ".apbx") {
		p += ".apbx"
	}
	return p, nil
}

// ---------- 打开/关闭 ----------

// OpenBundle 解包并载入 playbook
func (s *PlaybookService) OpenBundle(archive, password string) (*BundleDTO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bundle != nil {
		s.bundle.Cleanup()
		s.bundle = nil
	}
	b, err := playbook.OpenBundle(s.sz, archive, password, "")
	if err != nil {
		return nil, err
	}
	s.bundle = b
	s.lastOpenDir = filepath.Dir(archive)
	return s.bundleDTO()
}

// CloseBundle 关闭并清理临时目录
func (s *PlaybookService) CloseBundle() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bundle != nil {
		s.bundle.Cleanup()
		s.bundle = nil
	}
	return nil
}

// Bundle 返回当前 bundle
func (s *PlaybookService) Bundle() (*BundleDTO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.requireBundle(); err != nil {
		return nil, err
	}
	return s.bundleDTO()
}

// Reload 丢弃修改并重新解包
func (s *PlaybookService) Reload() (*BundleDTO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	archive, password := b.ArchivePath, b.Password
	b.Cleanup()
	s.bundle = nil
	nb, err := playbook.OpenBundle(s.sz, archive, password, "")
	if err != nil {
		return nil, err
	}
	s.bundle = nb
	return s.bundleDTO()
}

func (s *PlaybookService) bundleDTO() (*BundleDTO, error) {
	b := s.bundle
	dto := &BundleDTO{
		Archive:   b.ArchivePath,
		WorkDir:   b.WorkDir,
		Main:      b.Main,
		Files:     b.Files,
		Tree:      b.Tree(),
		TaskPaths: make([]string, 0, len(b.Tasks)),
		Dirty:     b.DirtyFiles(),
	}
	for k := range b.Tasks {
		dto.TaskPaths = append(dto.TaskPaths, k)
	}
	sortStrings(dto.TaskPaths)
	if c, err := s.confDTO(); err == nil {
		dto.Conf = c
		dto.Summary = c.Summary
	}
	return dto, nil
}

func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

func (s *PlaybookService) confDTO() (*ConfDTO, error) {
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	if b.Conf == nil {
		return nil, errors.New("归档里没有 playbook.conf")
	}
	opts := make([]string, 0, 32)
	for k := range b.Conf.OptionNames() {
		opts = append(opts, k)
	}
	sortStrings(opts)
	return &ConfDTO{
		Summary:  b.Conf.Summary(),
		Fields:   b.Conf.Fields(),
		Attrs:    b.Conf.Attrs(),
		Pages:    b.Conf.OptionPages(),
		Packages: b.Conf.SoftwarePackages(),
		Options:  opts,
		Raw:      b.Conf.Text,
	}, nil
}

// ---------- 任务读取 ----------

// Task 读取一个任务文件
func (s *PlaybookService) Task(rel string) (*playbook.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	return b.GetTask(rel)
}

// ---------- 编辑 ----------

// UpdateField 修改/新增 action 字段
func (s *PlaybookService) UpdateField(rel string, index int, key, value, kind string) (*playbook.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	t, err := b.GetTask(rel)
	if err != nil {
		return nil, err
	}
	if kind == "" {
		kind = playbook.FieldKindOf(tagAt(t, index), key)
	}
	if err := t.SetFieldTyped(index, key, value, kind); err != nil {
		return nil, err
	}
	b.MarkDirty(rel)
	return t, nil
}

// RemoveField 删除 action 字段
func (s *PlaybookService) RemoveField(rel string, index int, key string) (*playbook.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	t, err := b.GetTask(rel)
	if err != nil {
		return nil, err
	}
	if err := t.DeleteField(index, key); err != nil {
		return nil, err
	}
	b.MarkDirty(rel)
	return t, nil
}

func tagAt(t *playbook.Task, index int) string {
	if index >= 0 && index < len(t.Actions) {
		return t.Actions[index].Tag
	}
	return ""
}

// AddAction 新增 action
func (s *PlaybookService) AddAction(rel string, index int, tag string, fields []playbook.KeyValue, flow bool) (*playbook.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	t, err := b.GetTask(rel)
	if err != nil {
		return nil, err
	}
	for i := range fields {
		if fields[i].Kind == "" {
			fields[i].Kind = playbook.FieldKindOf(tag, fields[i].Key)
		}
	}
	if err := t.InsertAction(index, tag, fields, flow); err != nil {
		return nil, err
	}
	b.MarkDirty(rel)
	return t, nil
}

// RemoveAction 删除 action
func (s *PlaybookService) RemoveAction(rel string, index int) (*playbook.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	t, err := b.GetTask(rel)
	if err != nil {
		return nil, err
	}
	if err := t.DeleteAction(index); err != nil {
		return nil, err
	}
	b.MarkDirty(rel)
	return t, nil
}

// DuplicateAction 复制 action
func (s *PlaybookService) DuplicateAction(rel string, index int) (*playbook.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	t, err := b.GetTask(rel)
	if err != nil {
		return nil, err
	}
	if err := t.DuplicateAction(index); err != nil {
		return nil, err
	}
	b.MarkDirty(rel)
	return t, nil
}

// MoveAction 移动 action
func (s *PlaybookService) MoveAction(rel string, from, to int) (*playbook.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	t, err := b.GetTask(rel)
	if err != nil {
		return nil, err
	}
	if err := t.MoveAction(from, to); err != nil {
		return nil, err
	}
	b.MarkDirty(rel)
	return t, nil
}

// SetMeta 修改任务元数据
func (s *PlaybookService) SetMeta(rel, key, value string) (*playbook.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	t, err := b.GetTask(rel)
	if err != nil {
		return nil, err
	}
	if err := t.SetMeta(key, value); err != nil {
		return nil, err
	}
	b.MarkDirty(rel)
	return t, nil
}

// SetTaskText 源码模式整体替换
func (s *PlaybookService) SetTaskText(rel, text string) (*playbook.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	t, err := b.GetTask(rel)
	if err != nil {
		return nil, err
	}
	if err := t.SetText(text); err != nil {
		return nil, err
	}
	b.MarkDirty(rel)
	return t, nil
}

// ---------- playbook.conf ----------

// SetConfText 修改 conf 元素文本
func (s *PlaybookService) SetConfText(tag string, occurrence int, value string) (*ConfDTO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	if b.Conf == nil {
		return nil, errors.New("归档里没有 playbook.conf")
	}
	if err := b.Conf.SetText(tag, occurrence, value); err != nil {
		return nil, err
	}
	b.MarkDirty("playbook.conf")
	return s.confDTO()
}

// SetConfAttr 修改 conf 元素属性
func (s *PlaybookService) SetConfAttr(tag string, occurrence int, attr, value string) (*ConfDTO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	if b.Conf == nil {
		return nil, errors.New("归档里没有 playbook.conf")
	}
	if err := b.Conf.SetAttr(tag, occurrence, attr, value); err != nil {
		return nil, err
	}
	b.MarkDirty("playbook.conf")
	return s.confDTO()
}

// SetConfRaw 源码模式替换 conf
func (s *PlaybookService) SetConfRaw(text string) (*ConfDTO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	if b.Conf == nil {
		return nil, errors.New("归档里没有 playbook.conf")
	}
	b.Conf.SetRaw(text)
	b.MarkDirty("playbook.conf")
	return s.confDTO()
}

// ---------- 其他文件 ----------

// ReadFile 读取 bundle 内任意文件
func (s *PlaybookService) ReadFile(rel string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return "", err
	}
	return b.ReadFile(rel)
}

// SaveFile 写入 bundle 内任意文件
func (s *PlaybookService) SaveFile(rel, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return err
	}
	if err := b.WriteFile(rel, content); err != nil {
		return err
	}
	b.MarkDirty(rel)
	return nil
}

// ---------- 校验 & 保存 ----------

// Validate 执行校验
func (s *PlaybookService) Validate() ([]playbook.Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	return b.Validate(), nil
}

// Schema 返回 action 类型定义
func (s *PlaybookService) Schema() SchemaDTO {
	return SchemaDTO{
		Tags:          playbook.TagSpecs(),
		Common:        playbook.CommonFields(),
		Groups:        playbook.GroupNames(),
		Privileges:    []string{"Admin", "TrustedInstaller", "System", "CurrentUser"},
		RunAsOptions:  []string{"currentUserElevated", "currentUser", "system", "trustedInstaller"},
		RegistryTypes: []string{"REG_DWORD", "REG_SZ", "REG_EXPAND_SZ", "REG_QWORD", "REG_BINARY", "REG_MULTI_SZ"},
	}
}

// Save 写回并重新打包；out 为空时覆盖原文件
func (s *PlaybookService) Save(out string) (*SaveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return nil, err
	}
	if out == "" {
		out = b.ArchivePath
	}
	if err := b.SaveAs(s.sz, out); err != nil {
		return nil, err
	}
	n := 0
	_ = filepath.Walk(b.WorkDir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return &SaveResult{Archive: out, Size: sevenzip.Size(out), Files: n}, nil
}

// ExportWorkspace 把解包内容导出到指定目录（便于用外部工具查看）
func (s *PlaybookService) ExportWorkspace(dir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.requireBundle()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return copyTree(b.WorkDir, dir)
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// RevealInExplorer 在资源管理器中打开
func (s *PlaybookService) RevealInExplorer(p string) error {
	if p == "" {
		return errors.New("路径为空")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	if st, err2 := os.Stat(abs); err2 == nil && !st.IsDir() {
		return exec.Command("explorer.exe", "/select,"+abs).Start()
	}
	return exec.Command("explorer.exe", abs).Start()
}

var _ = fmt.Sprintf
