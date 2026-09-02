package main

import (
	"embed"
	"log"

	"github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// 注册拖放事件，绑定生成器据此产出强类型的前端 API
	// Registering the drop event lets the binding generator emit a strongly typed frontend API
	application.RegisterEvent[string]("explorer:file-dropped")
	// 协议唤起：外部工具用 aba-explorer://open?path=... 请求打开容器或内容表
	// Protocol invocation: external tools ask to open a container or catalog via aba-explorer://open?path=...
	application.RegisterEvent[string](internal.ProtocolOpenEvent)
	// 索引进度事件，全局搜索建索引时按文件推送
	// The index-progress event pushed per file while the global search builds its index
	application.RegisterEvent[internal.IndexProgress](internal.IndexProgressEvent)
}

func main() {
	app := internal.NewApp()
	search := internal.NewSearchService()

	// 单实例回调要用到应用与窗口，两者都在下面才创建，因此先声明变量由闭包捕获
	// The single-instance callback needs the app and window created below, so closures capture these declarations
	var wailsApp *application.App
	var mainWindow *application.WebviewWindow

	// 单实例是设置项，关掉后每次唤起都会开新窗口，目标文件仍会在冷启动路径上打开
	// Single instance is a setting; with it off every invocation opens another window and the target file still
	// opens on the cold-start path
	var singleInstance *application.SingleInstanceOptions
	if internal.LoadSettings().SingleInstance {
		singleInstance = &application.SingleInstanceOptions{
			UniqueID: "Github.MeidoPromotionAssociation.ABA_EXPLORER",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				// 已有窗口时把它带到前台，否则协议唤起看起来像什么都没发生
				// Bringing the existing window forward, otherwise an invocation looks like nothing happened
				if mainWindow != nil {
					mainWindow.Show()
					mainWindow.Focus()
				}
				if path := internal.SecondInstanceTarget(data.Args); path != "" && wailsApp != nil {
					wailsApp.Event.Emit(internal.ProtocolOpenEvent, path)
				}
			},
		}
	}

	wailsApp = application.New(application.Options{
		Name:        "ABA_EXPLORER",
		Description: "Browser, unpacker and packer for KCES ABA containers",
		// 单实例：开启后协议唤起与文件关联双击都转交给已在运行的实例，不会堆积窗口
		// Single instance: once on, protocol invocations and association double-clicks are handed to the instance
		// already running instead of piling up windows
		SingleInstance: singleInstance,
		Services: []application.Service{
			application.NewService(app),
			application.NewService(internal.NewAbaExplorerService()),
			application.NewService(internal.NewCtExplorerService()),
			application.NewService(internal.NewConvertService()),
			application.NewService(search),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	app.SetApplication(wailsApp)
	search.SetEmitter(func(name string, data any) {
		wailsApp.Event.Emit(name, data)
	})

	window := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "ABA EXPLORER by 90135",
		Width:            1360,
		Height:           860,
		MinWidth:         1024,
		MinHeight:        640,
		EnableFileDrop:   true,
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
	})
	mainWindow = window
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		if len(files) > 0 {
			wailsApp.Event.Emit("explorer:file-dropped", files[0])
		}
	})

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
