package prompt

import "testing"

func TestRenderReplacesRecognizedText(t *testing.T) {
	got := Render("Edit: {recognized_text}", "hello", "", "auto")
	if got != "Edit: hello" {
		t.Fatalf("Render() = %q", got)
	}
}

func TestRenderAppendsRecognizedTextWithoutPlaceholder(t *testing.T) {
	got := Render("Edit this", "hello", "", "auto")
	if got != "Edit this\n\nhello" {
		t.Fatalf("Render() = %q", got)
	}
}

func TestClipboardPlaceholder(t *testing.T) {
	if !NeedsClipboard("Use {clipboard}") {
		t.Fatal("placeholder was not detected")
	}
	if NeedsClipboard("No clipboard") {
		t.Fatal("clipboard access should not be requested")
	}
	if got := Render("{clipboard} / {recognized_text}", "speech", "copied", "auto"); got != "copied / speech" {
		t.Fatalf("Render() = %q", got)
	}
}

func TestLanguagePlaceholder(t *testing.T) {
	if got := Render("Write in {language}: {recognized_text}", "hello", "", "ru-RU"); got != "Write in ru-RU: hello" {
		t.Fatalf("Render() = %q", got)
	}
}
