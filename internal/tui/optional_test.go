package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/liliumapps/commando/internal/entry"
)

// optSelectEntry has an optional single-value list field followed by a required
// plain field, so we can Enter past the optional and reach a runnable state.
func optSelectEntry() []entry.Entry {
	return []entry.Entry{{
		Desc:     "deploy",
		Template: "deploy [--env <env>] <app>",
		Vars: []entry.VarSpec{
			{Name: "env", Src: "printf 'dev\\nstaging\\nprod\\n'"}, // optional (in [ ]), list field
			{Name: "app"}, // required plain field
		},
		Source: entry.SourceCheat,
	}}
}

// TestOptionalSelectEmptyLeavesUnset verifies that an optional single-value list
// field does NOT highlight or auto-adopt the top candidate: pressing Enter on an
// empty input leaves it unset and advances, so the parameter can be omitted.
func TestOptionalSelectEmptyLeavesUnset(t *testing.T) {
	be := newFakeBackend()
	be.data["env"] = fd("prod", "beta")
	m := New(optSelectEntry(), be)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = send(m, key(tea.KeyEnter)) // into form, focus env (optional)

	fs := m.form.cur()
	if fs.field.Name != "env" || !fs.field.Optional {
		t.Fatalf("expected optional env first, got %q optional=%v", fs.field.Name, fs.field.Optional)
	}
	// Nothing highlighted on arrival.
	if fs.cursorHighlighted() {
		t.Error("optional select with empty input must not highlight a candidate")
	}
	if strings.Contains(stripANSI(m.View()), "❯ prod") {
		t.Errorf("optional select must not render the top candidate highlighted; view:\n%s", stripANSI(m.View()))
	}

	// Enter leaves env empty and advances to app (does not adopt prod).
	m = send(m, key(tea.KeyEnter))
	if m.form.cur().field.Name != "app" {
		t.Fatalf("Enter should advance to app; now on %q", m.form.cur().field.Name)
	}
	if v, ok := m.form.fields[0].value(); ok {
		t.Errorf("optional env should be unset after Enter, got %q", v)
	}

	// Fill the required app and run: env must be omitted from the command.
	m = send(m, runes("web"))
	m = send(m, key(tea.KeyEnter))
	if m.Result != "deploy web" {
		t.Errorf("Result = %q, want 'deploy web' (env omitted)", m.Result)
	}
}

// TestOptionalSelectStillAdoptsWhenNavigated confirms the escape hatch: an
// optional select can still pick a candidate, just deliberately (via arrows).
func TestOptionalSelectStillAdoptsWhenNavigated(t *testing.T) {
	be := newFakeBackend()
	be.data["env"] = fd("prod", "beta")
	m := New(optSelectEntry(), be)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = send(m, key(tea.KeyEnter)) // env

	m = send(m, key(tea.KeyUp)) // first Up highlights the bottom candidate (prod)
	if !m.form.cur().cursorHighlighted() {
		t.Error("navigating an optional select should highlight the candidate")
	}
	if got := highlightedValue(m.form.cur()); got != "prod" {
		t.Errorf("first Up from an unhighlighted list should highlight index 0 (prod), got %q", got)
	}
	m = send(m, key(tea.KeyUp))    // second Up moves up the screen to beta
	m = send(m, key(tea.KeyEnter)) // adopt navigated env, advance to app
	m = send(m, runes("web"))
	m = send(m, key(tea.KeyEnter))
	if m.Result != "deploy --env beta web" {
		t.Errorf("Result = %q, want env=beta adopted after navigating", m.Result)
	}
}

// TestRequiredSelectStillFastPicks guards that the empty-input fast-pick is
// preserved for REQUIRED list fields (only optional ones changed).
func TestRequiredSelectStillFastPicks(t *testing.T) {
	be := newFakeBackend()
	be.data["branch"] = fd("main", "dev")
	m := New([]entry.Entry{{
		Desc:     "switch",
		Template: "git switch <branch>", // required list field
		Vars:     []entry.VarSpec{{Name: "branch", Src: "git branch"}},
		Source:   entry.SourceCheat,
	}}, be)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = send(m, key(tea.KeyEnter)) // branch
	if !m.form.cur().cursorHighlighted() {
		t.Error("required select should highlight the top candidate on empty input")
	}
	m = send(m, key(tea.KeyEnter)) // fast-pick main + run
	if m.Result != "git switch main" {
		t.Errorf("Result = %q, want 'git switch main' (required fast-pick)", m.Result)
	}
}

// optMultiEntry has an optional multi-select field followed by a required plain
// field.
func optMultiEntry() []entry.Entry {
	return []entry.Entry{{
		Desc:     "add",
		Template: "add [<file>...] <msg>",
		Vars: []entry.VarSpec{
			{Name: "file", Multi: true, Src: "ls", Each: `"{}"`},
			{Name: "msg"},
		},
		Source: entry.SourceCheat,
	}}
}

// TestOptionalMultiHidesCursorUntilEngaged verifies the confirmed multi
// behavior: on arrival the ❯ toggle cursor is hidden and Enter omits the param;
// each engagement (arrow, or Space selection) reveals the cursor.
func TestOptionalMultiHidesCursorUntilEngaged(t *testing.T) {
	be := newFakeBackend()
	be.data["file"] = fd("a.go", "b.go", "c.go")
	m := New(optMultiEntry(), be)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = send(m, key(tea.KeyEnter)) // focus file (optional multi)

	fs := m.form.cur()
	if fs.field.Name != "file" || !fs.field.Optional || !fs.field.Spec.Multi {
		t.Fatalf("expected optional multi file, got %q opt=%v multi=%v", fs.field.Name, fs.field.Optional, fs.field.Spec.Multi)
	}
	// Untouched: no highlight, and the ❯ marker is absent from the candidate rows.
	if fs.cursorHighlighted() {
		t.Error("untouched optional multi must not highlight the cursor")
	}
	if strings.Contains(stripANSI(m.View()), "❯ ◯") {
		t.Errorf("untouched optional multi must not draw the ❯ toggle cursor; view:\n%s", stripANSI(m.View()))
	}

	// Arrow engages: cursor becomes visible, but nothing is selected yet.
	m = send(m, key(tea.KeyUp))
	if !m.form.cur().cursorHighlighted() {
		t.Error("arrowing should reveal the optional multi cursor")
	}
	if len(m.form.cur().selected) != 0 {
		t.Errorf("arrowing must not select anything, selected=%v", m.form.cur().selected)
	}
}

// TestOptionalMultiEmptyEnterOmits confirms an untouched optional multi field is
// omitted from the command when the user just presses Enter.
func TestOptionalMultiEmptyEnterOmits(t *testing.T) {
	be := newFakeBackend()
	be.data["file"] = fd("a.go", "b.go")
	m := New(optMultiEntry(), be)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = send(m, key(tea.KeyEnter)) // file
	m = send(m, key(tea.KeyEnter)) // omit file, advance to msg
	if m.form.cur().field.Name != "msg" {
		t.Fatalf("Enter should advance to msg; now on %q", m.form.cur().field.Name)
	}
	m = send(m, runes("init"))
	m = send(m, key(tea.KeyEnter))
	if m.Result != "add init" {
		t.Errorf("Result = %q, want 'add init' (file omitted)", m.Result)
	}
}

// TestOptionalMultiSpaceEngagesAndSelects confirms Space toggles the bottom item
// and reveals the cursor from an untouched optional multi.
func TestOptionalMultiSpaceEngagesAndSelects(t *testing.T) {
	be := newFakeBackend()
	be.data["file"] = fd("a.go", "b.go", "c.go")
	m := New(optMultiEntry(), be)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = send(m, key(tea.KeyEnter)) // file

	m = send(m, key(tea.KeySpace)) // toggle bottom candidate (index 0 = a.go)
	fs := m.form.cur()
	if !fs.cursorHighlighted() {
		t.Error("Space should engage and reveal the optional multi cursor")
	}
	if len(fs.selected) != 1 || fs.selected[0] != "a.go" {
		t.Errorf("Space should select a.go, selected=%v", fs.selected)
	}
	m = send(m, key(tea.KeyEnter)) // advance to msg
	m = send(m, runes("init"))
	m = send(m, key(tea.KeyEnter))
	// The template's literal "..." follows the <file> placeholder, so it remains
	// after substitution (a template detail, not part of the value).
	if m.Result != `add "a.go"... init` {
		t.Errorf("Result = %q, want `add \"a.go\"... init`", m.Result)
	}
}

// TestRequiredMultiKeepsCursorVisible guards that a REQUIRED multi still shows
// the ❯ toggle cursor from arrival (only optional multi changed).
func TestRequiredMultiKeepsCursorVisible(t *testing.T) {
	be := newFakeBackend()
	be.data["file"] = fd("a.go", "b.go")
	m := New([]entry.Entry{{
		Desc:     "add",
		Template: "add <file>...", // required multi (no [ ])
		Vars:     []entry.VarSpec{{Name: "file", Multi: true, Src: "ls", Each: `"{}"`}},
		Source:   entry.SourceCheat,
	}}, be)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = send(m, key(tea.KeyEnter)) // file
	if m.form.cur().field.Optional {
		t.Fatal("file should be required in this entry")
	}
	if !m.form.cur().cursorHighlighted() {
		t.Error("required multi should show the ❯ toggle cursor from arrival")
	}
}
