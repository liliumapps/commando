package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
)

func patternEntry() entry.Entry {
	return entry.Entry{
		Desc:     "kill a pid",
		Template: "kill -9 <pid>",
		Vars: []entry.VarSpec{
			{Name: "pid", Pattern: `[0-9]+`},
		},
		Source: entry.SourceCheat,
	}
}

func TestPatternRejectsMalformedValue(t *testing.T) {
	be := newFakeBackend()
	m := New([]entry.Entry{patternEntry()}, be)
	m = send(m, key(tea.KeyEnter)) // into form, pid field

	m = send(m, runes("notapid"))
	m = send(m, key(tea.KeyEnter))
	if !m.Aborted {
		t.Error("pattern field should not finalize with a non-matching value")
	}
	if m.formErr == "" {
		t.Error("expected a pattern-violation error")
	}
}

func TestPatternAcceptsMatchingValue(t *testing.T) {
	be := newFakeBackend()
	m := New([]entry.Entry{patternEntry()}, be)
	m = send(m, key(tea.KeyEnter))

	m = send(m, runes("4321"))
	m = send(m, key(tea.KeyEnter))
	if m.Aborted {
		t.Fatalf("matching value should be accepted; err=%q", m.formErr)
	}
	if m.Result != "kill -9 4321" {
		t.Errorf("Result = %q, want kill -9 4321", m.Result)
	}
}

// TestPatternIsAnchored ensures the pattern must match the whole value, not just
// a substring (so "12ab" is rejected by `[0-9]+`).
func TestPatternIsAnchored(t *testing.T) {
	be := newFakeBackend()
	m := New([]entry.Entry{patternEntry()}, be)
	m = send(m, key(tea.KeyEnter))

	m = send(m, runes("12ab"))
	m = send(m, key(tea.KeyEnter))
	if !m.Aborted {
		t.Error("partial match should be rejected by an anchored pattern")
	}
}
