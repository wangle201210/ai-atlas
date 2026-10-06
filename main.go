package main

import (
	"ai-atlas/internal/atlas"
	"ai-atlas/internal/buildinfo"
	"ai-atlas/internal/updates"
	"embed"
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"log"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	updater.HandleHelperMode()
	service, err := atlas.New("", "")
	if err != nil {
		log.Fatal(err)
	}
	defer atlas.Close(service)
	updateService := updates.New(buildinfo.Version(), nil, nativeUpdatesSupported(), func() error {
		if service.Status().Running {
			return errors.New("请等待索引扫描结束后再更新")
		}
		return checkUpdateLocation()
	})
	defer updates.Close(updateService)
	var window *application.WebviewWindow
	app := application.New(application.Options{
		SingleInstance: instanceOptions(atlas.InstanceKey(service), func() {
			if window != nil {
				window.Show()
				window.Focus()
			}
		}),
		Name: "AI Atlas", Description: "AI 编程工具的用量、会话与存储分析",
		Services: []application.Service{application.NewService(service), application.NewService(updateService)},
		Assets:   application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:      application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	provider, err := updates.NewProvider()
	if err != nil {
		log.Fatal(err)
	}
	if err = app.Updater.Init(updater.Config{CurrentVersion: buildinfo.Version(), Providers: []updater.Provider{provider}, Window: updater.WindowNone}); err != nil {
		log.Fatal(err)
	}
	updates.Attach(updateService, app.Updater)
	app.Event.On(updater.EventDownloadProgress, func(e *application.CustomEvent) {
		if p, ok := e.Data.(updater.Progress); ok {
			updates.ReportProgress(updateService, p.Written, p.Total)
		}
	})
	app.Event.On(updater.EventVerifying, func(_ *application.CustomEvent) { updates.ReportPhase(updateService, "verifying") })
	app.Event.On(updater.EventInstalling, func(_ *application.CustomEvent) { updates.ReportPhase(updateService, "installing") })
	window = app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "AI Atlas", Width: 1320, Height: 880, MinWidth: 800, MinHeight: 620, BackgroundColour: application.NewRGB(248, 250, 252), URL: "/"})
	atlas.AttachPickers(service, func() (string, error) {
		return app.Dialog.OpenFile().CanChooseDirectories(true).CanChooseFiles(false).AttachToWindow(window).PromptForSingleSelection()
	}, func() (string, error) {
		return app.Dialog.OpenFile().CanChooseDirectories(false).CanChooseFiles(true).AttachToWindow(window).PromptForSingleSelection()
	})
	if err = app.Run(); err != nil {
		log.Print(err)
	}
}
