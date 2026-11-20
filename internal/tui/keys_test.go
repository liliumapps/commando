package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
)

// singleParam builds an entry with one required single-value <x> param.
func singleParamEntry() []entry.Entry {
	return []entry.Entry{{
		Desc:     "run",
		Template: "run <x>",
		Vars:     []entry.VarSpec{{Name: "x", Src: "list"}},
		Source:   entry.SourceCheat,
	}}
}

func TestEnterAdoptsHighlightedCandidateOnEmptyInput(t *testing.T) {
	be := newFakeBackend()
	be.data["x"] = fd("alpha", "beta") // alpha is index 0 (bottom, highlighted)
	m := New(singleParamEntry(), be)
	m = send(m, key(tea.KeyEnter)) // into form, x

	// Input empty; Enter should pick the highlighted candidate (alpha) and, as
	// it's the only required param, run.
	m = send(m, key(tea.KeyEnter))
	if m.Aborted {
		t.Fatalf("Enter should pick+run; err=%q", m.formErr)
	}
	if m.Result != "run alpha" {
		t.Errorf("Result = %q, want 'run alpha'", m.Result)
	}
}

func TestEnterAdoptsNavigatedCandidate(t *testing.T) {
	be := newFakeBackend()
	be.data["x"] = fd("alpha", "beta")
	m := New(singleParamEntry(), be)
	m = send(m, key(tea.KeyEnter)) // form
	m = send(m, key(tea.KeyUp))    // navigate to index 1 (beta)
	m = send(m, key(tea.KeyEnter)) // adopt beta + run
	if m.Result != "run beta" {
		t.Errorf("Result = %q, want 'run beta' (navigated candidate)", m.Result)
	}
}

func TestPlainFieldEnterDoesNotAdoptRemembered(t *testing.T) {
	// A plain field (no Src) with remembered values shows them as candidates but
	// must NOT auto-adopt on Enter with an empty input.
	be := newFakeBackend()
	be.data["msg"] = FieldData{Candidates: rankRemembered("recent-msg", "older-msg")}
	m := New([]entry.Entry{{
		Desc:     "commit",
		Template: "git commit -m <msg>",
		Vars:     []entry.VarSpec{{Name: "msg"}}, // no Src => plain field
		Source:   entry.SourceCheat,
	}}, be)
	m = send(m, key(tea.KeyEnter)) // into form, msg
	// Enter on empty plain field: nothing adopted; it's the only (required)
	// param, so run() bounces back with an error rather than running with a
	// silently-filled value.
	m = send(m, key(tea.KeyEnter))
	if !m.Aborted {
		t.Error("plain field should not run by silently adopting a remembered value")
	}
	if m.form.cur().input != "" {
		t.Errorf("plain field Enter must not adopt a remembered value, got %q", m.form.cur().input)
	}
	// Deliberate pick: ↑ onto the top candidate, then Enter takes it.
	m = send(m, key(tea.KeyUp))    // navigate to recent-msg (index 0 highlighted -> up to 1?)
	m = send(m, key(tea.KeyDown))  // back to index 0 (recent-msg), navigated=true
	m = send(m, key(tea.KeyEnter)) // adopt + run
	if m.Result != "git commit -m recent-msg" {
		t.Errorf("↑+Enter should pick the navigated value; Result=%q", m.Result)
	}
}

func TestUnsetScopeRemembersByDefault(t *testing.T) {
	// A var with NO remember scope should still be persisted (memory defaults
	// to on). This is the bug: previously nothing was stored for such fields.
	be := newFakeBackend()
	m := New([]entry.Entry{{
		Desc:     "echo",
		Template: "echo <msg>",
		Vars:     []entry.VarSpec{{Name: "msg"}}, // no remember set
		Source:   entry.SourceCheat,
	}}, be)
	m = send(m, key(tea.KeyEnter)) // form, msg
	m = send(m, runes("hello"))
	send(m, key(tea.KeyEnter)) // last param -> run, which persists
	if got := be.remembered["msg"]; len(got) != 1 || got[0] != "hello" {
		t.Errorf("unset-scope var should be remembered by default, got %v", got)
	}
}

func TestNoneScopeDoesNotRemember(t *testing.T) {
	be := newFakeBackend()
	m := New([]entry.Entry{{
		Desc:     "echo",
		Template: "echo <secret>",
		Vars:     []entry.VarSpec{{Name: "secret", Remember: entry.ScopeNone}},
		Source:   entry.SourceCheat,
	}}, be)
	m = send(m, key(tea.KeyEnter))
	m = send(m, runes("hunter2"))
	send(m, key(tea.KeyEnter)) // run
	if got := be.remembered["secret"]; len(got) != 0 {
		t.Errorf("remember=none var must not be remembered, got %v", got)
	}
}

func TestListFieldEnterStillAdoptsTopOnEmpty(t *testing.T) {
	// A list field (has Src) still adopts the highlighted candidate on Enter
	// with an empty input — the fast pick is intended there.
	be := newFakeBackend()
	be.data["branch"] = fd("main", "dev")
	m := New([]entry.Entry{{
		Desc:     "switch",
		Template: "git switch <branch>",
		Vars:     []entry.VarSpec{{Name: "branch", Src: "git branch"}}, // Src => list field
		Source:   entry.SourceCheat,
	}}, be)
	m = send(m, key(tea.KeyEnter)) // form, branch
	m = send(m, key(tea.KeyEnter)) // empty input + list field => adopt "main" + run
	if m.Result != "git switch main" {
		t.Errorf("list field Enter on empty should adopt top candidate; Result=%q", m.Result)
	}
}

func TestEnterKeepsTypedTextOverCandidate(t *testing.T) {
	be := newFakeBackend()
	be.data["x"] = fd("alpha", "beta")
	m := New(singleParamEntry(), be)
	m = send(m, key(tea.KeyEnter)) // form
	m = send(m, runes("custom"))   // type free text (not navigated)
	m = send(m, key(tea.KeyEnter)) // keep typed text + run
	if m.Result != "run custom" {
		t.Errorf("Result = %q, want 'run custom' (typed text preserved)", m.Result)
	}
}

func TestEnterLabelReflectsState(t *testing.T) {
	m := New(sampleEntries(), newFakeBackend())
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 20})
	m = send(m, key(tea.KeyEnter)) // form, focus author (param 0 of 3)

	// Not on the last param: Enter hint reads "next".
	if v := stripANSI(m.View()); !strings.Contains(v, "⏎ next") {
		t.Errorf("expected '⏎ next' hint on a non-last param; view:\n%s", lastLine(v))
	}

	// Move to the last param (file): Enter hint reads "run".
	m = send(m, key(tea.KeyCtrlN))
	m = send(m, key(tea.KeyCtrlN))
	if m.form.cur().field.Name != "file" {
		t.Fatalf("expected file (last param), got %q", m.form.cur().field.Name)
	}
	if v := stripANSI(m.View()); !strings.Contains(v, "⏎ run") {
		t.Errorf("expected '⏎ run' hint on the last param; view:\n%s", lastLine(v))
	}
}

func lastLine(v string) string {
	ls := strings.Split(strings.TrimRight(v, "\n"), "\n")
	return ls[len(ls)-1]
}

func TestSpaceTogglesMultiNotAdvance(t *testing.T) {
	be := newFakeBackend()
	be.data["file"] = fd("a.go", "b.go")
	m := New(sampleEntries(), be) // file is the multi param
	m = send(m, key(tea.KeyEnter))
	// Navigate to file.
	m = send(m, key(tea.KeyCtrlN))
	m = send(m, key(tea.KeyCtrlN))
	if m.form.cur().field.Name != "file" {
		t.Fatalf("expected file param, got %q", m.form.cur().field.Name)
	}
	// Space toggles the highlighted candidate and stays on the field.
	m = send(m, key(tea.KeySpace))
	if m.form.cur().field.Name != "file" {
		t.Errorf("Space should not advance; now on %q", m.form.cur().field.Name)
	}
	if len(m.form.cur().selected) != 1 || m.form.cur().selected[0] != "a.go" {
		t.Errorf("Space should toggle a.go; selected = %v", m.form.cur().selected)
	}
	// Space again toggles it off.
	m = send(m, key(tea.KeySpace))
	if len(m.form.cur().selected) != 0 {
		t.Errorf("second Space should deselect; selected = %v", m.form.cur().selected)
	}
}

func TestSpaceTypesIntoSingleValue(t *testing.T) {
	be := newFakeBackend()
	m := New([]entry.Entry{{
		Desc:     "echo",
		Template: "echo <msg>",
		Vars:     []entry.VarSpec{{Name: "msg"}},
		Source:   entry.SourceCheat,
	}}, be)
	m = send(m, key(tea.KeyEnter))
	m = send(m, runes("hello"))
	m = send(m, key(tea.KeySpace)) // single-value: Space is a literal space
	m = send(m, runes("world"))
	if m.form.cur().input != "hello world" {
		t.Errorf("Space should type into a single value, got %q", m.form.cur().input)
	}
}

func TestListInputCursorEditsInsideQuery(t *testing.T) {
	m := New(sampleEntries(), newFakeBackend())
	m = send(m, runes("abc"))
	m = send(m, key(tea.KeyLeft))
	m = send(m, key(tea.KeyLeft))
	m = send(m, runes("X"))
	if m.query != "aXbc" {
		t.Fatalf("query = %q, want aXbc", m.query)
	}
	m = send(m, key(tea.KeyBackspace))
	if m.query != "abc" {
		t.Errorf("query after backspace = %q, want abc", m.query)
	}
}

func TestFormInputCursorEditsInsideValue(t *testing.T) {
	be := newFakeBackend()
	m := New([]entry.Entry{{
		Desc:     "echo",
		Template: "echo <msg>",
		Vars:     []entry.VarSpec{{Name: "msg"}},
		Source:   entry.SourceCheat,
	}}, be)
	m = send(m, key(tea.KeyEnter))
	m = send(m, runes("abc"))
	m = send(m, key(tea.KeyLeft))
	m = send(m, key(tea.KeyLeft))
	m = send(m, runes("X"))
	if m.form.cur().input != "aXbc" {
		t.Fatalf("field input = %q, want aXbc", m.form.cur().input)
	}
	m = send(m, key(tea.KeyEnter))
	if m.Result != "echo aXbc" {
		t.Errorf("Result = %q, want echo aXbc", m.Result)
	}
}

func TestParamNavCommitsNavigatedSingle(t *testing.T) {
	be := newFakeBackend()
	be.data["file"] = fd("x.go")
	m := New([]entry.Entry{{
		Desc:     "two",
		Template: "cmd <a> <b>",
		Vars:     []entry.VarSpec{{Name: "a", Src: "list"}, {Name: "b"}},
		Source:   entry.SourceCheat,
	}}, be)
	be.data["a"] = fd("aval", "aval2")
	m = send(m, key(tea.KeyEnter)) // form, a
	// Navigate a's candidates, then leave via Ctrl-N: navigated -> commit aval.
	m = send(m, key(tea.KeyUp))    // index 1
	m = send(m, key(tea.KeyDown))  // back to index 0 (aval), navigated
	m = send(m, key(tea.KeyCtrlN)) // leave -> commit aval
	// Go back to a and confirm its value stuck.
	m = send(m, key(tea.KeyCtrlP))
	if m.form.cur().input != "aval" {
		t.Errorf("leaving a navigated field should commit the candidate, got %q", m.form.cur().input)
	}
}

func TestParamNavDoesNotCommitUntouchedOptional(t *testing.T) {
	be := newFakeBackend()
	be.data["opt"] = fd("candidate-val")
	m := New([]entry.Entry{{
		Desc:     "opt entry",
		Template: "cmd [--x <opt>] <req>",
		Vars:     []entry.VarSpec{{Name: "opt", Src: "list"}, {Name: "req"}},
		Source:   entry.SourceCheat,
	}}, be)
	m = send(m, key(tea.KeyEnter)) // form, opt (optional, has candidates)
	if m.form.cur().field.Name != "opt" {
		t.Fatalf("expected opt first, got %q", m.form.cur().field.Name)
	}
	// Move past without navigating candidates: must NOT auto-fill the optional.
	m = send(m, key(tea.KeyCtrlN)) // -> req
	m = send(m, key(tea.KeyCtrlP)) // back to opt
	if _, ok := m.form.cur().value(); ok {
		t.Errorf("moving past an untouched optional should not fill it; got %q", m.form.cur().input)
	}
}
