package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFormBoxedAndInputAtBottom(t *testing.T) {
	be := newFakeBackend()
	be.data["file"] = fd("a.go", "b.go")
	m := New(sampleEntries(), be)
	const h = 24
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: h})
	m = send(m, key(tea.KeyEnter)) // into form

	ls := lines(m.View())
	if len(ls) != h {
		t.Errorf("form view has %d lines, want %d (fills screen)", len(ls), h)
	}
	// The command summary remains boxed and anchored at the top.
	joined := strings.Join(ls, "\n")
	if !strings.Contains(joined, "╭") && !strings.Contains(joined, "┌") {
		t.Errorf("expected a boxed command summary, got:\n%s", joined)
	}
	top := strings.Join(ls[:min(10, len(ls))], "\n")
	if !strings.Contains(top, "git log") {
		t.Errorf("command summary should be at the top; got:\n%s", top)
	}
	// The active param's input line is in the bottom region (last two lines).
	bottom := strings.Join(ls[len(ls)-2:], "\n")
	if !strings.Contains(bottom, "author") {
		t.Errorf("active param input should be pinned to the bottom; got:\n%s", bottom)
	}
}

func TestFormShowsAllParamsIncludingUnset(t *testing.T) {
	m := New(sampleEntries(), newFakeBackend())
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = send(m, key(tea.KeyEnter)) // into form

	view := stripANSI(m.View())
	// Every parameter appears in the box assignment block, even unset ones.
	for _, name := range []string{"author", "count", "file"} {
		if !strings.Contains(view, name) {
			t.Errorf("param %q should be listed in the form", name)
		}
	}
	// An unset required param shows the unset marker.
	if !strings.Contains(view, "‹unset›") {
		t.Errorf("unset param should show the ‹unset› marker; view:\n%s", view)
	}
}

func TestFormParamAppearsAtMostTwice(t *testing.T) {
	// A single param must not be shown three times: it appears once as an
	// overview assignment line and once as the bottom editor — not as a
	// separate field-list row.
	m := New(sampleEntries(), newFakeBackend())
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = send(m, key(tea.KeyEnter)) // focus is on "author"

	view := stripANSI(m.View())
	// Count occurrences of the active param name "author" as a standalone token
	// at the start of trimmed lines (assignment line and input line).
	count := 0
	for _, ln := range strings.Split(view, "\n") {
		t := strings.TrimLeft(ln, " ❯")
		if strings.HasPrefix(t, "author") {
			count++
		}
	}
	if count > 2 {
		t.Errorf("param 'author' appears on %d lines, want at most 2 (overview + editor)", count)
	}
}
