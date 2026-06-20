// Package workflow contains the service sequence independent from the desktop UI.
package workflow

import (
	"context"
	"fmt"

	"godict/internal/prompt"
)

type Recognizer interface {
	Recognize(context.Context, []byte, string) (string, error)
}

type Generator interface {
	Generate(context.Context, string) (string, error)
}

type Clipboard interface {
	Content() string
	SetContent(string)
}

type Processor struct {
	Recognizer Recognizer
	Generator  Generator
	Clipboard  Clipboard
	// OnGenerating runs after speech recognition and immediately before the LLM request.
	OnGenerating func()
}

// Process recognizes recorded audio, conditionally reads the clipboard, generates
// output, and writes it to the clipboard only after every preceding step succeeds.
func (p Processor) Process(ctx context.Context, audio []byte, language, template string) error {
	if p.Recognizer == nil || p.Generator == nil || p.Clipboard == nil {
		return fmt.Errorf("workflow is not fully configured")
	}
	recognized, err := p.Recognizer.Recognize(ctx, audio, language)
	if err != nil {
		return err
	}
	return p.ProcessRecognized(ctx, recognized, template)
}

// ProcessRecognized runs the template, LLM, and clipboard portion of the
// workflow after a live recognizer has already finalized its transcript.
func (p Processor) ProcessRecognized(ctx context.Context, recognized, template string) error {
	if p.Generator == nil || p.Clipboard == nil {
		return fmt.Errorf("workflow is not fully configured")
	}
	clipboard := ""
	if prompt.NeedsClipboard(template) {
		clipboard = p.Clipboard.Content()
	}
	if p.OnGenerating != nil {
		p.OnGenerating()
	}
	result, err := p.Generator.Generate(ctx, prompt.Render(template, recognized, clipboard))
	if err != nil {
		return err
	}
	p.Clipboard.SetContent(result)
	return nil
}
