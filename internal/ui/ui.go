package ui

import (
	"context"
	"fmt"
	"image/color"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
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
	webSearchCheck *widget.Check
	displayCheck   *widget.Check
	resultBox      *widget.Entry
	root           *fyne.Container
	button         *widget.Button
	status         *widget.Label
	statusDot      *canvas.Circle
	waveform       *fyne.Container
	stream         speechkit.Stream
	recording      bool
	processing     bool
}

var microphoneIcon = fyne.NewStaticResource("microphone.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#06111f" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="2" width="6" height="12" rx="3"/><path d="M6 11a6 6 0 0 0 12 0M12 17v4M8.5 21h7"/></svg>`))

func New(app fyne.App, cfg *config.Config) *Desktop {
	d := &Desktop{app: app, config: cfg, recorder: &audio.Recorder{}}
	app.Settings().SetTheme(goDictTheme{})
	d.window = app.NewWindow("GoDict")

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
	d.templateSelect.SetSelected(cfg.DefaultTemplate().Name)
	d.languageSelect = widget.NewSelect(cfg.LanguageNames(), nil)
	d.languageSelect.SetSelected(cfg.Speech.DefaultLanguage)
	d.webSearchCheck = widget.NewCheck("Web search", nil)
	d.displayCheck = widget.NewCheck("Display result", func(show bool) {
		if show {
			d.resultBox.Show()
		} else {
			d.resultBox.Hide()
		}
		d.window.Resize(d.root.MinSize())
	})
	d.resultBox = widget.NewMultiLineEntry()
	// Fyne 2.6 does not provide Entry.SetReadOnly; a disabled Entry is the
	// supported non-editable multiline display control in this release.
	d.resultBox.Disable()
	d.resultBox.SetMinRowsVisible(6)
	d.resultBox.Hide()
	selectWidth := widestSelectWidth(d.templateSelect.Options, d.modelSelect.Options, d.languageSelect.Options)
	d.button = widget.NewButtonWithIcon("Start recording", microphoneIcon, d.toggle)
	d.button.Importance = widget.HighImportance
	d.status = widget.NewLabel("Ready")
	d.status.Alignment = fyne.TextAlignCenter
	d.statusDot = canvas.NewCircle(color.NRGBA{R: 142, G: 160, B: 184, A: 255})
	statusDot := statusDotSlot(d.statusDot)

	divider := canvas.NewRectangle(color.NRGBA{R: 30, G: 42, B: 67, A: 255})
	divider.SetMinSize(fyne.NewSize(1, 1))
	fields := container.NewVBox(
		fieldRow("TEMPLATE", d.templateSelect, selectWidth),
		fieldRow("LLM", d.modelSelect, selectWidth),
		fieldRow("LANGUAGE", d.languageSelect, selectWidth),
	)
	options := container.NewVBox(d.webSearchCheck, d.displayCheck)
	recordArea := container.NewVBox(
		container.NewCenter(d.button),
		container.NewCenter(container.NewHBox(statusDot, d.status)),
	)
	top := container.NewVBox(fields, options, divider)
	// Border gives the visible result entry all spare vertical space on resize.
	content := container.NewBorder(top, recordArea, nil, nil, d.resultBox)
	d.root = container.NewStack(canvas.NewRectangle(color.NRGBA{R: 19, G: 27, B: 47, A: 255}), container.NewPadded(content))
	d.window.SetContent(d.root)
	d.window.Resize(d.root.MinSize())
	return d
}

func fieldRow(name string, selectWidget *widget.Select, selectWidth float32) fyne.CanvasObject {
	label := widget.NewLabel(name)
	label.TextStyle = fyne.TextStyle{Bold: true}
	label.Alignment = fyne.TextAlignLeading
	labelSlot := container.NewGridWrap(fyne.NewSize(78, 28), label)
	// The transparent spacer establishes an initial width from all loaded
	// options, while Stack and Border let Select consume added window width.
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(selectWidth, selectWidget.MinSize().Height))
	selectorSlot := container.NewStack(spacer, selectWidget)
	return container.NewBorder(nil, nil, labelSlot, nil, selectorSlot)
}

func widestSelectWidth(optionSets ...[]string) float32 {
	var width float32
	for _, options := range optionSets {
		for _, option := range options {
			probe := widget.NewSelect([]string{option}, nil)
			probe.SetSelected(option)
			if size := probe.MinSize(); size.Width > width {
				width = size.Width
			}
		}
	}
	return width
}

func statusDotSlot(dot *canvas.Circle) *fyne.Container {
	dot.Resize(fyne.NewSize(7, 7))
	dot.Move(fyne.NewPos(0, 7))
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(7, 21))
	return container.NewWithoutLayout(spacer, dot)
}

func waveform() *fyne.Container {
	bars := make([]fyne.CanvasObject, 0, 8)
	for _, height := range []float32{8, 16, 12, 20, 14, 22, 10, 18} {
		bar := canvas.NewRectangle(color.NRGBA{R: 96, G: 165, B: 250, A: 255})
		bar.SetMinSize(fyne.NewSize(3, height))
		bars = append(bars, bar)
	}
	return container.NewHBox(bars...)
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
	d.processing = true
	d.button.Disable()
	d.button.SetText("Connecting…")
	d.button.SetIcon(theme.ViewRefreshIcon())
	d.status.SetText("Connecting to SpeechKit…")
	d.setStatusColor(color.NRGBA{R: 251, G: 191, B: 36, A: 255})
	language := d.config.Speech.Languages[d.languageSelect.Selected]
	go func() {
		stream, err := (speechkit.Client{APIKey: d.config.Speech.APIKey, FolderID: d.config.Speech.FolderID}).Start(context.Background(), language)
		if err == nil {
			err = d.recorder.Start(stream.Send)
		}
		fyne.Do(func() {
			if err != nil {
				if stream != nil {
					stream.Cancel()
				}
				d.setReadyError(err)
				return
			}
			d.stream = stream
			d.processing, d.recording = false, true
			d.button.Enable()
			d.button.SetText("Stop recording")
			d.button.SetIcon(theme.MediaStopIcon())
			d.button.Importance = widget.DangerImportance
			d.status.SetText("Recording")
			d.setStatusColor(color.NRGBA{R: 34, G: 197, B: 94, A: 255})
			d.modelSelect.Disable()
			d.templateSelect.Disable()
			d.languageSelect.Disable()
			d.webSearchCheck.Disable()
			d.displayCheck.Disable()
		})
	}()
}

func (d *Desktop) stopAndProcess() {
	d.recording = false
	d.processing = true
	d.button.SetText("Processing…")
	d.button.SetIcon(theme.ViewRefreshIcon())
	d.button.Disable()
	d.status.SetText("Finalizing transcript…")
	d.button.Importance = widget.MediumImportance
	d.setStatusColor(color.NRGBA{R: 251, G: 191, B: 36, A: 255})

	model := d.selectedModel()
	template := d.selectedTemplate().Text
	language := d.config.Speech.Languages[d.languageSelect.Selected]
	stream := d.stream
	d.stream = nil
	go func() {
		if stream == nil {
			fyne.Do(func() { d.setReadyError(fmt.Errorf("SpeechKit stream is not active")) })
			return
		}
		if err := d.recorder.Stop(); err != nil {
			stream.Cancel()
			fyne.Do(func() { d.setReadyError(err) })
			return
		}
		recognized, err := stream.Close()
		if err != nil {
			fyne.Do(func() { d.setReadyError(err) })
			return
		}
		processor := workflow.Processor{
			Generator: llm.Client{Model: model, WebSearch: d.webSearchCheck.Checked},
			Clipboard: d.app.Clipboard(),
			OnResult: func(result string) {
				fyne.Do(func() {
					if d.displayCheck.Checked {
						d.resultBox.SetText(result)
					}
				})
			},
			OnGenerating: func() {
				fyne.Do(func() { d.status.SetText("Generating text…") })
			},
		}
		err = processor.ProcessRecognized(context.Background(), recognized, language, template)
		if err == nil && d.config.Settings.BeepOnCompletion {
			audio.Beep()
		}
		fyne.Do(func() {
			d.processing = false
			if err != nil {
				d.setReadyError(err)
				return
			}
			d.button.Enable()
			d.button.SetText("Start recording")
			d.button.SetIcon(microphoneIcon)
			d.button.Importance = widget.HighImportance
			d.modelSelect.Enable()
			d.templateSelect.Enable()
			d.languageSelect.Enable()
			d.webSearchCheck.Enable()
			d.displayCheck.Enable()
			d.status.SetText("Copied to clipboard")
			d.setStatusColor(color.NRGBA{R: 34, G: 197, B: 94, A: 255})
		})
	}()
}

func (d *Desktop) setReadyError(err error) {
	d.recording, d.processing = false, false
	d.button.Enable()
	d.button.SetText("Start recording")
	d.button.SetIcon(microphoneIcon)
	d.button.Importance = widget.HighImportance
	d.modelSelect.Enable()
	d.templateSelect.Enable()
	d.languageSelect.Enable()
	d.webSearchCheck.Enable()
	d.displayCheck.Enable()
	d.status.SetText("Error: " + friendlyError(err))
	d.setStatusColor(color.NRGBA{R: 251, G: 113, B: 133, A: 255})
}

func (d *Desktop) setStatusColor(c color.Color) {
	d.statusDot.FillColor = c
	d.statusDot.Refresh()
}

// goDictTheme translates the reference's navy surface, blue accent, and muted
// labels into Fyne's native controls while keeping platform accessibility.
type goDictTheme struct{}

func (goDictTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	palette := map[fyne.ThemeColorName]color.Color{
		theme.ColorNameBackground:          color.NRGBA{R: 11, G: 16, B: 32, A: 255},
		theme.ColorNameForeground:          color.NRGBA{R: 248, G: 250, B: 252, A: 255},
		theme.ColorNamePrimary:             color.NRGBA{R: 96, G: 165, B: 250, A: 255},
		theme.ColorNameButton:              color.NRGBA{R: 24, G: 35, B: 67, A: 255},
		theme.ColorNameDisabledButton:      color.NRGBA{R: 41, G: 54, B: 83, A: 255},
		theme.ColorNameInputBackground:     color.NRGBA{R: 11, G: 16, B: 32, A: 255},
		theme.ColorNameInputBorder:         color.NRGBA{R: 41, G: 54, B: 83, A: 255},
		theme.ColorNameMenuBackground:      color.NRGBA{R: 19, G: 27, B: 47, A: 255},
		theme.ColorNameOverlayBackground:   color.NRGBA{R: 19, G: 27, B: 47, A: 255},
		theme.ColorNameSelection:           color.NRGBA{R: 35, G: 66, B: 113, A: 255},
		theme.ColorNameHover:               color.NRGBA{R: 30, G: 57, B: 99, A: 255},
		theme.ColorNameDisabled:            color.NRGBA{R: 226, G: 232, B: 240, A: 255},
		theme.ColorNameSeparator:           color.NRGBA{R: 30, G: 42, B: 67, A: 255},
		theme.ColorNameError:               color.NRGBA{R: 251, G: 113, B: 133, A: 255},
		theme.ColorNameSuccess:             color.NRGBA{R: 34, G: 197, B: 94, A: 255},
		theme.ColorNameWarning:             color.NRGBA{R: 251, G: 191, B: 36, A: 255},
		theme.ColorNameForegroundOnPrimary: color.NRGBA{R: 6, G: 17, B: 31, A: 255},
		theme.ColorNameForegroundOnError:   color.NRGBA{R: 255, G: 255, B: 255, A: 255},
		theme.ColorNameForegroundOnSuccess: color.NRGBA{R: 6, G: 17, B: 31, A: 255},
		theme.ColorNameForegroundOnWarning: color.NRGBA{R: 6, G: 17, B: 31, A: 255},
	}
	if c, ok := palette[name]; ok {
		return c
	}
	return theme.DefaultTheme().Color(name, variant)
}
func (goDictTheme) Font(style fyne.TextStyle) fyne.Resource { return theme.DefaultTheme().Font(style) }
func (goDictTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}
func (goDictTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return 13
	case theme.SizeNameInnerPadding:
		return 4
	case theme.SizeNamePadding:
		return 3
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 8
	default:
		return theme.DefaultTheme().Size(name)
	}
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
