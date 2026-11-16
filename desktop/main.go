package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	service := NewDesktopService()
	app := application.New(application.Options{
		Name:        "Croc Desktop",
		Description: "Private peer-to-peer file transfers",
		Services:    []application.Service{application.NewService(service)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
	})
	service.attach(app)
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "Croc Desktop",
		Width:          1180,
		Height:         820,
		MinWidth:       940,
		MinHeight:      680,
		EnableFileDrop: true,
	})
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		app.Event.Emit("files-dropped", event.Context().DroppedFiles())
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
