package tui

import (
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func newTextInput(value string) textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.SetValue(value)
	input.CursorEnd()
	input.Focus()
	input.Cursor.SetMode(cursor.CursorStatic)
	return input
}

func normalizeTextInputKey(msg tea.KeyMsg) tea.KeyMsg {
	if msg.Type == tea.KeySpace && len(msg.Runes) == 0 {
		msg.Runes = []rune{' '}
	}
	return msg
}

// styledTextInputView renders input with a static caret, scrolled so the caret
// stays visible within width terminal cells. The window is measured in display
// cells, not runes, so wide (e.g. CJK) characters never overflow the line.
func styledTextInputView(input textinput.Model, textStyle, cursorStyle lipgloss.Style, width int) string {
	if width < 1 {
		width = 1
	}
	runes := []rune(input.Value())
	pos := input.Position()
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	widths := make([]int, len(runes))
	total := 0
	for i, r := range runes {
		widths[i] = ansi.StringWidth(string(r))
		total += widths[i]
	}

	maxText := max(0, width-1) // one cell for the caret
	start, end := 0, len(runes)
	if total > maxText {
		// earliest start that keeps the caret at or before the right edge
		startCaret, w := pos, 0
		for startCaret > 0 && w+widths[startCaret-1] <= maxText {
			startCaret--
			w += widths[startCaret]
		}
		// earliest start that still fills the window up to the end of the text
		startTail, w := len(runes), 0
		for startTail > 0 && w+widths[startTail-1] <= maxText {
			startTail--
			w += widths[startTail]
		}
		start = min(startCaret, startTail)
		end, w = start, 0
		for end < len(runes) && w+widths[end] <= maxText {
			w += widths[end]
			end++
		}
	}
	cursorPos := min(max(pos-start, 0), end-start)

	visible := runes[start:end]
	before := textStyle.Render(string(visible[:cursorPos]))
	after := textStyle.Render(string(visible[cursorPos:]))
	return before + cursorStyle.Render("▏") + after
}
