package main

import (
	"embed"
	"log"
	"os"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	logPath := setupLog()
	log.Println("=== start ===", "log:", logPath)
	svc := NewPlaybookService()

	// 支持从命令行 / 文件关联直接打开：playbook-studio.exe <path.apbx> [password]
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(a, "-") {
			continue
		}
		if svc.startArchive == "" {
			svc.startArchive = a
		} else if svc.startPass == "" {
			svc.startPass = a
		}
	}

	app := application.New(application.Options{
		Name:        "Playbook Studio",
		Description: "AME Playbook (.apbx) 工作流编辑器",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		// server 模式下暴露 HTTP 服务（桌面构建会忽略）
		Server: application.ServerOptions{
			Host: "127.0.0.1",
			Port: 34115,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Playbook Studio",
		// 无边框：标题栏与窗口按钮全部由前端 Vue 绘制
		Frameless:        true,
		Width:            1480,
		Height:           940,
		MinWidth:         1120,
		MinHeight:        680,
		BackgroundColour: application.NewRGB(12, 13, 18),
		// 注意：不要开启 Windows.WebView2CompositionHosting。
		// 该模式（配合自定义非客户区）会让进程在空闲/关闭时触发 __fastfail，
		// 表现为 Windows 弹出“快速异常检测失败”。标题栏拖拽改用运行时的
		// --wails-draggable: drag 机制（见 frontend/public/style.css）。
		Windows: application.WindowsWindow{
			WebView2CompositionHosting: false,
		},
		URL: "/",
	})
	svc.AttachWindow(win)

	if err := app.Run(); err != nil {
		log.Println("app.Run error:", err)
		log.Fatal(err)
	}
	log.Println("=== app.Run returned, exiting ===")
}
