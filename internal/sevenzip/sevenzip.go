// Package sevenzip 通过 7-Zip 命令行实现 .apbx(AES-256 加密 7z) 的解包与打包。
//
// 程序内置了 7-Zip 官方免安装的独立控制台版 7zr.exe（仅 7z 格式，支持 AES-256 与头加密），
// 首次运行时释放到用户缓存目录，因此目标机器无需安装 7-Zip。
// 7-Zip 遵循 LGPL 许可，许可证随程序一并分发（见 bin/LICENSE-7zip.txt）。
package sevenzip

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed bin/7zr.exe
var builtin7z []byte

//go:embed bin/LICENSE-7zip.txt
var builtinLicense []byte

const builtinName = "7zr.exe"

// ErrNotFound 表示找不到可用的 7z 命令行程序
var ErrNotFound = errors.New("未找到 7z 命令行程序")

// ErrWrongPassword 表示密码错误
var ErrWrongPassword = errors.New("密码错误")

// Manager 管理 7z 可执行文件
type Manager struct {
	exe    string
	source string // builtin | system | custom
}

// 来源标识
const (
	SourceBuiltin = "builtin"
	SourceSystem  = "system"
	SourceCustom  = "custom"
)

// New 创建 Manager 并定位 7z（优先使用内置版本）
func New() *Manager {
	m := &Manager{}
	m.locate()
	return m
}

// ensureBuiltin 把内置的 7zr.exe 释放到用户缓存目录
func ensureBuiltin() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "PlaybookStudio", "tools")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	exe := filepath.Join(dir, builtinName)
	license := filepath.Join(dir, "LICENSE-7zip.txt")
	needWrite := true
	if st, err := os.Stat(exe); err == nil && st.Size() == int64(len(builtin7z)) {
		needWrite = false
	}
	if needWrite {
		if err := os.WriteFile(exe, builtin7z, 0o755); err != nil {
			return "", err
		}
	}
	if _, err := os.Stat(license); err != nil {
		_ = os.WriteFile(license, builtinLicense, 0o644)
	}
	return exe, nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func (m *Manager) locate() {
	if v := os.Getenv("PLAYBOOK_STUDIO_7Z"); v != "" && fileExists(v) {
		m.exe, m.source = v, SourceCustom
		return
	}
	if p, err := ensureBuiltin(); err == nil && fileExists(p) {
		m.exe, m.source = p, SourceBuiltin
		return
	}
	for _, p := range systemCandidates() {
		if fileExists(p) {
			m.exe, m.source = p, SourceSystem
			return
		}
	}
}

// SetExePath 手动指定 7z 路径
func (m *Manager) SetExePath(p string) error {
	if p == "" {
		// 传空表示恢复自动定位
		m.locate()
		return nil
	}
	if !fileExists(p) {
		return fmt.Errorf("无效的 7z 路径: %s", p)
	}
	m.exe, m.source = p, SourceCustom
	return nil
}

// ExePath 返回当前使用的 7z 路径（可能为空）
func (m *Manager) ExePath() string { return m.exe }

// Source 返回当前 7z 的来源：builtin / system / custom
func (m *Manager) Source() string { return m.source }

// Available 是否可用
func (m *Manager) Available() bool { return m.exe != "" }

// BuiltinSize 返回内置 7zr.exe 的体积（用于界面展示）
func BuiltinSize() int { return len(builtin7z) }

func systemCandidates() []string {
	var out []string
	add := func(p string) {
		if p != "" {
			out = append(out, p)
		}
	}
	add(os.Getenv("PLAYBOOK_STUDIO_7Z"))
	add(os.Getenv("SEVENZIP"))
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		add(filepath.Join(dir, "tools", "7z.exe"))
		add(filepath.Join(dir, "7z.exe"))
	}
	pf := os.Getenv("ProgramFiles")
	pfx := os.Getenv("ProgramFiles(x86)")
	lad := os.Getenv("LOCALAPPDATA")
	for _, d := range []string{pf, pfx} {
		if d != "" {
			add(filepath.Join(d, "7-Zip", "7z.exe"))
		}
	}
	if lad != "" {
		add(filepath.Join(lad, "Programs", "7-Zip", "7z.exe"))
	}
	for _, name := range []string{"7z.exe", "7z", "7za.exe", "7za"} {
		if p, err := exec.LookPath(name); err == nil {
			add(p)
		}
	}
	if lad != "" {
		// Windows 商店版 7-Zip / NanaZip 的别名
		add(filepath.Join(lad, "Microsoft", "WindowsApps", "7z.exe"))
	}
	return out
}



type runOpts struct {
	password string
	level    int
	encHeader bool
	dir      string
}

func (m *Manager) run(args []string, dir string) error {
	if m.exe == "" {
		return ErrNotFound
	}
	cmd := exec.Command(m.exe, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		low := strings.ToLower(text)
		if strings.Contains(low, "wrong password") || strings.Contains(low, "password") && strings.Contains(low, "error") {
			return ErrWrongPassword
		}
		return fmt.Errorf("7z 执行失败: %v\n%s", err, strings.TrimSpace(text))
	}
	return nil
}

// Extract 解包到 destDir；password 为空时按空密码处理（不会阻塞等待输入）
func (m *Manager) Extract(archive, destDir, password string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	args := []string{"x", "-y", "-bso0", "-bsp0", "-bd", "-o" + destDir}
	args = append(args, "-p"+password)
	args = append(args, archive)
	return m.run(args, "")
}

// Pack 将 srcDir 目录内容打包为 archive（7z + AES-256，可选加密头）
func (m *Manager) Pack(srcDir, archive, password string, encryptHeader bool, level int) error {
	if password == "" && encryptHeader {
		return errors.New("启用头加密时必须提供密码")
	}
	if level <= 0 || level > 9 {
		level = 7
	}
	if abs, err := filepath.Abs(archive); err == nil {
		archive = abs
	}
	args := []string{"a", "-t7z", "-y", "-bso0", "-bsp0", "-bd",
		"-mx=" + fmt.Sprint(level), "-m0=LZMA2", "-ms=on"}
	if encryptHeader {
		args = append(args, "-mhe=on")
	}
	args = append(args, "-p"+password)
	args = append(args, archive, "*")
	return m.run(args, srcDir)
}

// Size 返回文件大小（不存在返回 -1）
func Size(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return -1
	}
	return st.Size()
}
