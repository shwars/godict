package prompt

import "strings"

const RecognizedText = "{recognized_text}"
const Clipboard = "{clipboard}"
const Language = "{language}"

func NeedsClipboard(template string) bool { return strings.Contains(template, Clipboard) }

// Render substitutes the defined placeholders. Callers must only read clipboard text
// when NeedsClipboard returns true.
func Render(template, recognizedText, clipboardText, language string) string {
	rendered := strings.ReplaceAll(template, RecognizedText, recognizedText)
	rendered = strings.ReplaceAll(rendered, Clipboard, clipboardText)
	rendered = strings.ReplaceAll(rendered, Language, language)
	if !strings.Contains(template, RecognizedText) {
		rendered = strings.TrimRight(rendered, " \t\r\n") + "\n\n" + recognizedText
	}
	return rendered
}
