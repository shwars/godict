package workflow

import (
	"context"
	"errors"
	"testing"
)

type fakeRecognizer struct {
	text string
	err  error
}

func (f fakeRecognizer) Recognize(context.Context, []byte, string) (string, error) {
	return f.text, f.err
}

type fakeGenerator struct {
	input  string
	output string
	err    error
}

func (f *fakeGenerator) Generate(_ context.Context, input string) (string, error) {
	f.input = input
	return f.output, f.err
}

type fakeClipboard struct {
	content       string
	reads, writes int
}

func (f *fakeClipboard) Content() string           { f.reads++; return f.content }
func (f *fakeClipboard) SetContent(content string) { f.writes++; f.content = content }

func TestProcessorDoesNotReadClipboardWithoutPlaceholder(t *testing.T) {
	clip := &fakeClipboard{content: "private"}
	generator := &fakeGenerator{output: "result"}
	err := (Processor{Recognizer: fakeRecognizer{text: "speech"}, Generator: generator, Clipboard: clip}).Process(context.Background(), []byte("audio"), "auto", "Clean {recognized_text}")
	if err != nil {
		t.Fatal(err)
	}
	if clip.reads != 0 {
		t.Fatalf("clipboard was read %d times", clip.reads)
	}
	if generator.input != "Clean speech" || clip.content != "result" {
		t.Fatalf("input=%q clipboard=%q", generator.input, clip.content)
	}
}

func TestProcessorReadsClipboardWhenPlaceholderExists(t *testing.T) {
	clip := &fakeClipboard{content: "context"}
	generator := &fakeGenerator{output: "result"}
	err := (Processor{Recognizer: fakeRecognizer{text: "speech"}, Generator: generator, Clipboard: clip}).Process(context.Background(), []byte("audio"), "auto", "Use {clipboard}: {recognized_text}")
	if err != nil {
		t.Fatal(err)
	}
	if clip.reads != 1 || generator.input != "Use context: speech" {
		t.Fatalf("reads=%d input=%q", clip.reads, generator.input)
	}
}

func TestProcessorDoesNotOverwriteClipboardOnFailure(t *testing.T) {
	clip := &fakeClipboard{content: "keep"}
	generator := &fakeGenerator{err: errors.New("unavailable")}
	err := (Processor{Recognizer: fakeRecognizer{text: "speech"}, Generator: generator, Clipboard: clip}).Process(context.Background(), []byte("audio"), "auto", "{recognized_text}")
	if err == nil || clip.writes != 0 || clip.content != "keep" {
		t.Fatalf("err=%v writes=%d content=%q", err, clip.writes, clip.content)
	}
}
