package prompt

import "strings"

const RecognizedText = "{recognized_text}"
const Clipboard = "{clipboard}"

func NeedsClipboard(template string) bool { return strings.Contains(template, Clipboard) }

// Render substitutes the defined placeholders. Callers must only read clipboard text
// when NeedsClipboard returns true.
func Render(template, recognizedText, clipboardText string) string {
	rendered := strings.ReplaceAll(template, RecognizedText, recognizedText)
	rendered = strings.ReplaceAll(rendered, Clipboard, clipboardText)
	if !strings.Contains(template, RecognizedText) {
		rendered = strings.TrimRight(rendered, " \t\r\n") + "\n\n" + recognizedText
	}
	return rendered
}
