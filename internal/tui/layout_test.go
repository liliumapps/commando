package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/liliumapps/commando/internal/entry"
)

// stripANSI removes CSI escape sequences so layout assertions see plain text.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func lines(v string) []string { return strings.Split(stripANSI(v), "\n") }

func TestListSelectedCommandIsNearInput(t *testing.T) {
	m := New(sampleEntries(), newFakeBackend())
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	ls := lines(m.View())
	bottom := strings.Join(ls[len(ls)-6:], "\n")
	if !strings.Contains(bottom, "git log") || strings.Contains(bottom, "command:") {
		t.Errorf("selected command should be in the bottom control cluster; got:\n%s", bottom)
	}
}

func TestListFillsHeightInputAtBottom(t *testing.T) {
	m := New(sampleEntries(), newFakeBackend())
	const h = 30
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: h})

	ls := lines(m.View())
	if len(ls) != h {
		t.Errorf("list view has %d lines, want %d (should fill the screen)", len(ls), h)
	}
	// The search input prompt sits just above the help line in the bottom
	// control cluster.
	inputLine := ls[len(ls)-2]
	if !strings.HasPrefix(strings.TrimSpace(inputLine), "❯") {
		t.Errorf("input not pinned near the bottom; line[-2]=%q", inputLine)
	}
	if !strings.Contains(strings.Join(ls[len(ls)-6:], "\n"), "commando") {
		t.Error("expected palette status in the bottom cluster")
	}
}

func TestFormFillsHeightInputAtBottom(t *testing.T) {
	be := newFakeBackend()
	be.data["file"] = fd("a.go", "b.go")
	m := New(sampleEntries(), be)
	const h = 30
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: h})
	m = send(m, key(tea.KeyEnter)) // into the form

	ls := lines(m.View())
	if len(ls) != h {
		t.Errorf("form view has %d lines, want %d", len(ls), h)
	}
	// The active field's input line "❯ <name>: ..." sits in the bottom region.
	joinedBottom := strings.Join(ls[len(ls)-3:], "\n")
	if !strings.Contains(joinedBottom, "❯ author") {
		t.Errorf("form input not pinned to the bottom region; got:\n%s", joinedBottom)
	}
	// The command preview is pinned to the top, while the active input remains at
	// the bottom.
	top := strings.Join(ls[:min(10, len(ls))], "\n")
	if !strings.Contains(top, "git log") {
		t.Errorf("expected command preview at the top; got:\n%s", top)
	}
}

func TestListDescriptionRowDoesNotShiftFooter(t *testing.T) {
	entries := []entry.Entry{
		{Title: "same", Desc: "same", Template: "same", Kind: entry.CommandLiteral},
		{Title: "other", Desc: "extra detail", Template: "echo other", Kind: entry.CommandLiteral},
	}
	m := New(entries, newFakeBackend())
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})

	firstHeight := len(lines(m.listFooter()))
	firstDescription, _ := m.selectedEntryDetails()
	if strings.TrimSpace(stripANSI(firstDescription)) == "" {
		t.Fatal("description row should be present when description matches the title")
	}

	m = send(m, key(tea.KeyUp))
	if secondHeight := len(lines(m.listFooter())); secondHeight != firstHeight {
		t.Fatalf("footer shifted from %d to %d lines when selection changed", firstHeight, secondHeight)
	}
}

func TestFormCandidateListOmitsFieldNameHeader(t *testing.T) {
	be := newFakeBackend()
	be.data["force"] = fd("force  (--force-with-lease)")
	m := New([]entry.Entry{{
		Desc:     "git push",
		Template: "git push [<force>]",
		Vars:     []entry.VarSpec{{Name: "force", Src: "printf -- '--force-with-lease\n'"}},
		Source:   entry.SourceCheat,
	}}, be)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 18})
	m = send(m, key(tea.KeyEnter))

	view := stripANSI(m.View())
	if strings.Contains(view, "force candidates") {
		t.Errorf("candidate list should not repeat the field name as a header:\n%s", view)
	}
	if !strings.Contains(view, "force  (--force-with-lease)") {
		t.Errorf("candidate value should still be visible:\n%s", view)
	}
	if !strings.Contains(view, "force (opt)") {
		t.Errorf("input line should still identify the active field:\n%s", view)
	}
}

func TestListShorterTerminalStillBounded(t *testing.T) {
	// A very short terminal must not produce negative-sized regions or panic.
	m := New(sampleEntries(), newFakeBackend())
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 6})
	ls := lines(m.View())
	if len(ls) < 1 {
		t.Fatal("view should render at least one line")
	}
}

func TestViewsStayWithinTerminalWidth(t *testing.T) {
	for _, width := range []int{40, 80, 120} {
		t.Run(string(rune(width)), func(t *testing.T) {
			be := newFakeBackend()
			be.data["file"] = fd("a-very-long-candidate-name-that-must-not-wrap.go")
			m := New(sampleEntries(), be)
			m = send(m, tea.WindowSizeMsg{Width: width, Height: 24})
			assertViewWidth(t, m.View(), width)

			m = send(m, key(tea.KeyEnter))
			assertViewWidth(t, m.View(), width)
		})
	}
}

func assertViewWidth(t *testing.T, view string, width int) {
	t.Helper()
	for i, line := range strings.Split(view, "\n") {
		if got := ansi.StringWidth(line); got > width {
			t.Errorf("line %d width=%d exceeds terminal width=%d: %q", i, got, width, stripANSI(line))
		}
	}
}
