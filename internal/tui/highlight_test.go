package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/liliumapps/commando/internal/entry"
)

// branchEntry is a single command with one single-value list field ("branch")
// whose candidates come from a Src command, so isListField() is true.
func branchEntry() []entry.Entry {
	return []entry.Entry{{
		Desc:     "checkout a branch",
		Template: "git checkout <branch>",
		Vars:     []entry.VarSpec{{Name: "branch", Src: "git branch"}},
		Source:   entry.SourceCheat,
	}}
}

// highlightedValue returns the Value of the candidate under the cursor — i.e.
// the row drawn highlighted and the one Enter would commit — or "" if the cursor
// points nowhere.
func highlightedValue(fs *fieldState) string {
	vis := fs.displayCandidates()
	if fs.cursor < 0 || fs.cursor >= len(vis) {
		return ""
	}
	return vis[fs.cursor].Value
}

// TestTypedInputBecomesHighlightedRow verifies the core invariant: on a
// non-strict single-value field, the typed text appears as a synthetic
// highlighted candidate row at the cursor's home (index 0), so the highlighted
// row always equals what Enter commits — while typing it is the typed value.
func TestTypedInputBecomesHighlightedRow(t *testing.T) {
	be := newFakeBackend()
	be.data["branch"] = fd("main", "master", "maint")
	m := New(branchEntry(), be)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = send(m, key(tea.KeyEnter)) // into form, focus "branch"

	// Empty input on a list field: the top real candidate is the adopt target.
	if got := highlightedValue(m.form.cur()); got != "main" {
		t.Fatalf("empty list field should highlight top candidate; highlighted=%q", got)
	}

	// Type free text that is not an exact candidate ("ma" matches several).
	m = send(m, runes("ma"))
	fs := m.form.cur()
	if fs.input != "ma" {
		t.Fatalf("input = %q, want ma", fs.input)
	}
	// A synthetic typed row now sits at index 0 and is highlighted.
	if !fs.hasTypedRow() {
		t.Error("free-typed input should produce a synthetic typed row")
	}
	if got := highlightedValue(fs); got != "ma" {
		t.Errorf("highlighted row = %q, want the typed value %q", got, "ma")
	}
	// It is rendered too (the ❯ marker appears on a candidate row showing "ma").
	if !strings.Contains(stripANSI(m.View()), "❯ ma") {
		t.Errorf("typed row should render highlighted; view:\n%s", stripANSI(m.View()))
	}
}

// TestUpArrowLandsOnFirstRealCandidate is the regression for the reported bug:
// with the typed row at index 0, pressing ↑ once moves to the FIRST real
// candidate instead of skipping it.
func TestUpArrowLandsOnFirstRealCandidate(t *testing.T) {
	be := newFakeBackend()
	be.data["branch"] = fd("main", "master", "maint")
	m := New(branchEntry(), be)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = send(m, key(tea.KeyEnter))
	m = send(m, runes("ma")) // typed row at index 0; main/master/maint above

	m = send(m, key(tea.KeyUp)) // -> index 1, the first real candidate
	if got := highlightedValue(m.form.cur()); got != "main" {
		t.Errorf("↑ from typed row should land on first real candidate 'main', got %q", got)
	}

	// Enter now adopts that real candidate, not the typed text.
	m = send(m, key(tea.KeyEnter))
	if m.Result != "git checkout main" {
		t.Errorf("Result = %q, want 'git checkout main' after ↑+Enter", m.Result)
	}
}

// TestTypedRowEnterKeepsTypedValue confirms Enter on the (un-navigated) typed
// row commits the typed text verbatim — the highlight and the commit agree.
func TestTypedRowEnterKeepsTypedValue(t *testing.T) {
	be := newFakeBackend()
	be.data["branch"] = fd("main", "master", "maint")
	m := New(branchEntry(), be)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = send(m, key(tea.KeyEnter))

	m = send(m, runes("feature/x")) // free text, not a candidate, not navigated
	m = send(m, key(tea.KeyEnter))  // last (required) param -> run
	if m.Result != "git checkout feature/x" {
		t.Errorf("Result = %q, want the free-typed value committed", m.Result)
	}
}

// TestExactMatchTypedHasNoSyntheticRow checks that typing a value that exactly
// equals a candidate does NOT add a duplicate synthetic row — the real
// candidate stands in for it.
func TestExactMatchTypedHasNoSyntheticRow(t *testing.T) {
	be := newFakeBackend()
	be.data["branch"] = fd("main", "master", "maint")
	m := New(branchEntry(), be)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = send(m, key(tea.KeyEnter))

	m = send(m, runes("main")) // exact candidate
	if m.form.cur().hasTypedRow() {
		t.Error("an exact-match input must not create a duplicate synthetic row")
	}
}

// TestStrictFieldHasNoTypedRow confirms strict fields opt out of the synthetic
// row entirely: they cannot commit a non-candidate, so no typed row is shown and
// the pre-existing strict behavior is preserved.
func TestStrictFieldHasNoTypedRow(t *testing.T) {
	e := branchEntry()
	e[0].Vars[0].Strict = true
	be := newFakeBackend()
	be.data["branch"] = fd("main", "master", "maint")
	m := New(e, be)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = send(m, key(tea.KeyEnter))

	m = send(m, runes("ma")) // non-candidate prefix
	if m.form.cur().hasTypedRow() {
		t.Error("strict field must not create a synthetic typed row")
	}
}
