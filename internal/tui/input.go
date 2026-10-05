package tui

// Keep pasted/editable text within the input field's rune limit.
func boundedInput(text string, limit int) string {
	runes := []rune(text)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}
