package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/rank"
)

// entryOneParam builds an entry with a single required <x> param.
func entryOneParam() []entry.Entry {
	return []entry.Entry{{
		Desc:     "run",
		Template: "run <x>",
		Vars:     []entry.VarSpec{{Name: "x"}},
		Source:   entry.SourceCheat,
	}}
}

func TestParamInputEmptyWithRememberedNoDefault(t *testing.T) {
	be := newFakeBackend()
	// A remembered value exists, but the param has no author default.
	be.data["x"] = FieldData{Candidates: rank.BuildCandidates(
		[]rank.Candidate{{Value: "last-used-val", LastUsed: 500, UseCount: 3}}, nil,
	)}
	m := New(entryOneParam(), be)
	m = send(m, key(tea.KeyEnter)) // into form, param x

	// Input must NOT be pre-populated with the remembered value.
	if m.form.cur().input != "" {
		t.Errorf("input should be empty (remembered values are candidates, not typed), got %q", m.form.cur().input)
	}
	// The remembered value is the first candidate.
	vis := m.form.cur().visibleCandidates()
	if len(vis) == 0 || vis[0].Value != "last-used-val" {
		t.Errorf("remembered value should be the first candidate, got %v", vis)
	}
}

func TestParamInputKeepsAuthorDefault(t *testing.T) {
	be := newFakeBackend()
	m := New([]entry.Entry{{
		Desc:     "sleep",
		Template: "sleep <secs>",
		Vars:     []entry.VarSpec{{Name: "secs", Default: "5"}},
		Source:   entry.SourceCheat,
	}}, be)
	// Backend must return the author default as prefill; the fake returns
	// f.Spec.Default by default.
	m = send(m, key(tea.KeyEnter))
	if m.form.cur().input != "5" {
		t.Errorf("author default should prefill the input, got %q", m.form.cur().input)
	}
}

func TestParamInputShowsOptLabel(t *testing.T) {
	// An optional param shows "(opt)" in the bottom input line.
	m := New([]entry.Entry{{
		Desc:     "log",
		Template: "log [-n <count>] <file>",
		Vars:     []entry.VarSpec{{Name: "count"}, {Name: "file"}},
		Source:   entry.SourceCheat,
	}}, newFakeBackend())
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 20})
	m = send(m, key(tea.KeyEnter)) // focus first param (count, optional)
	if m.form.cur().field.Name != "count" || !m.form.cur().field.Optional {
		t.Fatalf("expected optional 'count' first, got %q optional=%v", m.form.cur().field.Name, m.form.cur().field.Optional)
	}
	view := stripANSI(m.View())
	// The bottom input line should contain "count (opt)".
	if !strings.Contains(view, "count (opt)") {
		t.Errorf("input line should show the (opt) label; view:\n%s", view)
	}
}
