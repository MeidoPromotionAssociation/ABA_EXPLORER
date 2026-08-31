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
}

func main() {
	app := internal.NewApp()

	wailsApp := application.New(application.Options{
		Name:        "ABA_EXPLORER",
		Description: "Browser, unpacker and packer for KCES ABA containers",
		Services: []application.Service{
			application.NewService(app),
			application.NewService(internal.NewAbaExplorerService()),
			application.NewService(internal.NewCtExplorerService()),
			application.NewService(internal.NewConvertService()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	app.SetApplication(wailsApp)

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
