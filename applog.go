package main

import (
	"log"
	"os"
	"path/filepath"
)

// setupLog 把日志写到用户缓存目录，便于排查现场问题。
// 日志文件：%LOCALAPPDATA%\PlaybookStudio\logs\playbook-studio.log
func setupLog() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(base, "PlaybookStudio", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	p := filepath.Join(dir, "playbook-studio.log")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return ""
	}
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return p
}
