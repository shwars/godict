package ui

import (
	"context"
	"fmt"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"godict/internal/audio"
	"godict/internal/config"
	"godict/internal/llm"
	"godict/internal/speechkit"
	"godict/internal/workflow"
)

type Desktop struct {
	app      fyne.App
	window   fyne.Window
	config   *config.Config
	recorder *audio.Recorder

	modelSelect    *widget.Select
	templateSelect *widget.Select
	languageSelect *widget.Select
	button         *widget.Button
	status         *widget.Label
	recording      bool
	processing     bool
}

func New(app fyne.App, cfg *config.Config) *Desktop {
	d := &Desktop{app: app, config: cfg, recorder: &audio.Recorder{}}
	d.window = app.NewWindow("GoDict")
	d.window.Resize(fyne.NewSize(430, 250))

	modelNames := make([]string, 0, len(cfg.Models))
	for _, model := range cfg.Models {
		modelNames = append(modelNames, model.Name)
	}
	sort.Strings(modelNames)
	templateNames := make([]string, 0, len(cfg.Templates))
	for _, template := range cfg.Templates {
		templateNames = append(templateNames, template.Name)
	}
	sort.Strings(templateNames)

	d.modelSelect = widget.NewSelect(modelNames, nil)
	d.modelSelect.SetSelected(cfg.DefaultModel().Name)
	d.templateSelect = widget.NewSelect(templateNames, nil)
	d.templateSelect.SetSelected(templateNames[0])
	d.languageSelect = widget.NewSelect(cfg.LanguageNames(), nil)
	d.languageSelect.SetSelected(cfg.Speech.DefaultLanguage)
	d.button = widget.NewButton("Start recording", d.toggle)
	d.button.Importance = widget.HighImportance
	d.status = widget.NewLabel("Ready")
	d.status.Alignment = fyne.TextAlignCenter

	content := container.NewVBox(
		widget.NewLabel("Prompt template"), d.templateSelect,
		widget.NewLabel("LLM"), d.modelSelect,
		widget.NewLabel("Recognition language"), d.languageSelect,
		d.button, d.status,
	)
	d.window.SetContent(container.NewPadded(content))
	return d
}

func (d *Desktop) ShowAndRun() { d.window.ShowAndRun() }

func (d *Desktop) toggle() {
	if d.processing {
		return
	}
	if !d.recording {
		d.start()
		return
	}
	d.stopAndProcess()
}

func (d *Desktop) start() {
	if err := d.recorder.Start(); err != nil {
		d.setReadyError(err)
		return
	}
	d.recording = true
	d.button.SetText("Stop recording")
	d.status.SetText("Recording…")
	d.modelSelect.Disable()
	d.templateSelect.Disable()
	d.languageSelect.Disable()
}

func (d *Desktop) stopAndProcess() {
	wav, err := d.recorder.Stop()
	d.recording = false
	if err != nil {
		d.setReadyError(err)
		return
	}
	d.processing = true
	d.button.SetText("Processing…")
	d.button.Disable()
	d.status.SetText("Recognizing speech…")

	model := d.selectedModel()
	template := d.selectedTemplate().Text
	language := d.config.Speech.Languages[d.languageSelect.Selected]
	go func() {
		processor := workflow.Processor{
			Recognizer: speechkit.Client{APIKey: d.config.Speech.APIKey, FolderID: d.config.Speech.FolderID},
			Generator:  llm.Client{Model: model},
			Clipboard:  d.app.Clipboard(),
			OnGenerating: func() {
				fyne.Do(func() { d.status.SetText("Generating text…") })
			},
		}
		err := processor.Process(context.Background(), wav, language, template)
		fyne.Do(func() {
			d.processing = false
			if err != nil {
				d.setReadyError(err)
				return
			}
			d.button.Enable()
			d.button.SetText("Start recording")
			d.modelSelect.Enable()
			d.templateSelect.Enable()
			d.languageSelect.Enable()
			d.status.SetText("Copied to clipboard")
		})
	}()
}

func (d *Desktop) setReadyError(err error) {
	d.recording, d.processing = false, false
	d.button.Enable()
	d.button.SetText("Start recording")
	d.modelSelect.Enable()
	d.templateSelect.Enable()
	d.languageSelect.Enable()
	d.status.SetText("Error: " + friendlyError(err))
}

func (d *Desktop) selectedModel() config.Model {
	for _, model := range d.config.Models {
		if model.Name == d.modelSelect.Selected {
			return model
		}
	}
	return d.config.DefaultModel()
}
func (d *Desktop) selectedTemplate() config.Template {
	for _, template := range d.config.Templates {
		if template.Name == d.templateSelect.Selected {
			return template
		}
	}
	return d.config.Templates[0]
}
func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 180 {
		return fmt.Sprintf("%s…", message[:177])
	}
	return message
}
