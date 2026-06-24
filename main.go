package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"godict/internal/config"
	"godict/internal/ui"
)

// applicationIcon is embedded so the window and task switcher retain the
// GoDict microphone icon even when the executable is copied elsewhere.
//
//go:embed Icon.png
var applicationIcon []byte

func main() {
	application := app.NewWithID("net.godict.desktop")
	application.SetIcon(fyne.NewStaticResource("Icon.png", applicationIcon))
	configPath, err := configPath()
	if err != nil {
		showStartupError(application, err)
		return
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		showStartupError(application, fmt.Errorf("load %s: %w", configPath, err))
		return
	}
	ui.New(application, cfg).ShowAndRun()
}

func configPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find executable: %w", err)
	}
	path := filepath.Join(filepath.Dir(executable), "godict.config")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	// go run places its temporary executable elsewhere; keeping this fallback makes
	// local development convenient while release builds still use their sidecar file.
	if workingDirectory, err := os.Getwd(); err == nil {
		return filepath.Join(workingDirectory, "godict.config"), nil
	}
	return path, nil
}

func showStartupError(application fyne.App, err error) {
	window := application.NewWindow("GoDict configuration error")
	window.SetContent(container.NewPadded(container.NewVBox(
		widget.NewLabel("GoDict could not start."),
		widget.NewLabel(err.Error()),
	)))
	window.Resize(fyne.NewSize(520, 160))
	window.ShowAndRun()
}
