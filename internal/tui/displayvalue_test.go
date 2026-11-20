package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/rank"
)

// dvEntry is a single-param entry whose field draws from a src list.
func dvEntry() entry.Entry {
	return entry.Entry{
		Desc:     "kill a process",
		Template: "kill -9 <pid>",
		Vars:     []entry.VarSpec{{Name: "pid", Src: "ps"}},
		Source:   entry.SourceCheat,
	}
}

// labeledFieldData builds FieldData with Display != Value candidates.
func labeledFieldData(pairs ...[2]string) FieldData {
	src := make([]rank.Candidate, len(pairs))
	for i, p := range pairs {
		src[i] = rank.Candidate{Display: p[0], Value: p[1]}
	}
	return FieldData{Candidates: rank.BuildCandidates(nil, src)}
}

// TestDisplayShownValueCommitted verifies that a candidate renders its Display
// label in the candidate list, but committing it inserts the Value.
func TestDisplayShownValueCommitted(t *testing.T) {
	be := newFakeBackend()
	be.data["pid"] = labeledFieldData(
		[2]string{"chrome (1234)", "1234"},
		[2]string{"postgres (5678)", "5678"},
	)
	m := New([]entry.Entry{dvEntry()}, be)
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = send(m, key(tea.KeyEnter)) // into form, pid field

	// The candidate list should show the readable labels, not the bare pids.
	view := m.View()
	if !strings.Contains(view, "chrome (1234)") {
		t.Errorf("candidate list should render the Display label; view:\n%s", view)
	}

	// Enter on the empty input adopts the highlighted candidate (a list field),
	// which is the top one, and — being the only required param — runs.
	m = send(m, key(tea.KeyEnter))
	if m.Aborted {
		t.Fatalf("should finalize by adopting the highlighted candidate; err=%q", m.formErr)
	}
	if m.Result != "kill -9 1234" {
		t.Errorf("Result = %q, want kill -9 1234 (Value inserted, not Display)", m.Result)
	}
}

// TestStrictMatchesValueNotDisplay ensures strictness is checked against the
// Value: typing the inserted value is accepted, typing the display label is not.
func TestStrictMatchesValueNotDisplay(t *testing.T) {
	e := dvEntry()
	e.Vars[0].Strict = true
	be := newFakeBackend()
	be.data["pid"] = labeledFieldData([2]string{"chrome (1234)", "1234"})

	// Typing the Value "1234" is accepted.
	m := New([]entry.Entry{e}, be)
	m = send(m, key(tea.KeyEnter))
	m = send(m, runes("1234"))
	m = send(m, key(tea.KeyEnter))
	if m.Aborted {
		t.Fatalf("typing the candidate Value should be accepted; err=%q", m.formErr)
	}
	if m.Result != "kill -9 1234" {
		t.Errorf("Result = %q, want kill -9 1234", m.Result)
	}
}
