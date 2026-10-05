package main

import (
	"ai-atlas/internal/atlas"
	"embed"
	"github.com/wailsapp/wails/v3/pkg/application"
	"log"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	service, err := atlas.New("", "")
	if err != nil {
		log.Fatal(err)
	}
	defer atlas.Close(service)
	app := application.New(application.Options{
		Name: "AI Atlas", Description: "AI 编程工具的用量、会话与存储分析",
		Services: []application.Service{application.NewService(service)},
		Assets:   application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:      application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "AI Atlas", Width: 1320, Height: 880, MinWidth: 800, MinHeight: 620, BackgroundColour: application.NewRGB(248, 250, 252), URL: "/"})
	if err = app.Run(); err != nil {
		log.Print(err)
	}
}
