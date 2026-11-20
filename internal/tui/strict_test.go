package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
)

func TestBookmarkLiteralImmediately(t *testing.T) {
	be := newFakeBackend()
	const command = "npm run build"
	m := New([]entry.Entry{{
		Title:    command,
		Template: command,
		Kind:     entry.CommandLiteral,
		Source:   entry.SourceProject,
	}}, be)
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = send(m, key(tea.KeyCtrlB))
	if len(be.bookmarked) != 1 || be.bookmarked[0] != command {
		t.Errorf("bookmarked = %v, want [%q]", be.bookmarked, command)
	}
	if m.status == "" {
		t.Error("expected a status message after bookmarking")
	}
	if m.Result != "" {
		t.Error("bookmarking should not produce a result")
	}
}

func TestBookmarkTemplateStoresResolvedCommand(t *testing.T) {
	be := newFakeBackend()
	be.data["env"] = fd("prod")
	m := New([]entry.Entry{strictEntry()}, be)

	m = send(m, key(tea.KeyCtrlB))
	if m.stage != stageForm || m.formAct != formActionBookmark {
		t.Fatal("bookmarking a template should open the resolution form")
	}
	m = send(m, key(tea.KeyEnter))

	const want = "deploy --env prod"
	if len(be.bookmarked) != 1 || be.bookmarked[0] != want {
		t.Errorf("bookmarked = %v, want [%q]", be.bookmarked, want)
	}
	if m.stage != stageList || m.Result != "" {
		t.Error("resolved bookmark should return to the list without selecting")
	}
	found := false
	for _, e := range m.allEntries {
		if e.Source == entry.SourceBookmark && e.Template == want {
			found = true
		}
	}
	if !found {
		t.Error("new bookmark should be available in the current TUI session")
	}
}

func TestDeleteBookmarkFromList(t *testing.T) {
	be := newFakeBackend()
	const command = "kubectl get pods"
	entries := []entry.Entry{
		{Title: command, Template: command, Kind: entry.CommandLiteral, Source: entry.SourceBookmark},
		{Title: "other", Template: "echo other", Kind: entry.CommandLiteral, Source: entry.SourceHistory},
	}
	m := New(entries, be)
	m = send(m, key(tea.KeyCtrlD)) // arms the delete
	if len(be.removed) != 0 {
		t.Fatalf("first ^d must only ask for confirmation, removed = %v", be.removed)
	}
	m = send(m, key(tea.KeyCtrlD)) // confirms
	if len(be.removed) != 1 || be.removed[0] != command {
		t.Fatalf("removed = %v, want %q", be.removed, command)
	}
	for _, e := range m.allEntries {
		if e.Source == entry.SourceBookmark && e.Template == command {
			t.Fatal("deleted bookmark remained in current list")
		}
	}
}

func strictEntry() entry.Entry {
	return entry.Entry{
		Desc:     "deploy to an environment",
		Template: "deploy --env <env>",
		Vars: []entry.VarSpec{
			{Name: "env", Src: "printf 'dev\\nstaging\\nprod\\n'", Strict: true},
		},
		Source: entry.SourceCheat,
	}
}

func TestStrictRejectsNonCandidate(t *testing.T) {
	be := newFakeBackend()
	be.data["env"] = fd("prod", "beta")
	m := New([]entry.Entry{strictEntry()}, be)
	m = send(m, key(tea.KeyEnter)) // into form, env field

	// Type a value that is not a candidate, then Enter -> blocked.
	m = send(m, runes("staging"))
	m = send(m, key(tea.KeyEnter))
	if !m.Aborted {
		t.Error("strict field should not finalize with a non-candidate value")
	}
	if m.formErr == "" {
		t.Error("expected a strict-violation error")
	}
}

func TestStrictAcceptsCandidate(t *testing.T) {
	be := newFakeBackend()
	be.data["env"] = fd("prod", "beta")
	m := New([]entry.Entry{strictEntry()}, be)
	m = send(m, key(tea.KeyEnter)) // into form

	// Enter with an empty input adopts the highlighted candidate ("prod") and,
	// since it's the only required param, finalizes.
	m = send(m, key(tea.KeyEnter))
	if m.Aborted {
		t.Fatalf("strict field should finalize with a candidate; err=%q", m.formErr)
	}
	if m.Result != "deploy --env prod" {
		t.Errorf("Result = %q, want deploy --env prod", m.Result)
	}
}

func TestStrictAcceptsExactTypedMatch(t *testing.T) {
	be := newFakeBackend()
	be.data["env"] = fd("prod", "beta")
	m := New([]entry.Entry{strictEntry()}, be)
	m = send(m, key(tea.KeyEnter))

	// Typing the full candidate text is accepted (it matches a candidate).
	m = send(m, runes("beta"))
	m = send(m, key(tea.KeyEnter))
	if m.Aborted {
		t.Fatalf("typing an exact candidate should be accepted; err=%q", m.formErr)
	}
	if m.Result != "deploy --env beta" {
		t.Errorf("Result = %q, want deploy --env beta", m.Result)
	}
}

func TestStrictFilterHighlightsTopMatch(t *testing.T) {
	// Regression: on a strict field, typing a filter must highlight the top
	// matching candidate (index 0) from the first keystroke, so ↑ lands on the
	// SECOND match rather than appearing to skip the first.
	be := newFakeBackend()
	be.data["env"] = fd("prod", "preprod", "beta")
	m := New([]entry.Entry{strictEntry()}, be)
	m = send(m, key(tea.KeyEnter)) // into form, env field

	// Type "pr" -> matches prod, preprod (starts-with). Top match is highlighted.
	m = send(m, runes("pr"))
	fs := m.form.cur()
	if !fs.cursorHighlighted() {
		t.Error("strict field filtering should highlight the top candidate")
	}
	if got := highlightedValue(fs); got != "prod" {
		t.Errorf("highlighted = %q, want top match 'prod'", got)
	}

	// ↑ moves to the second match, not skipping the first.
	m = send(m, key(tea.KeyUp))
	if got := highlightedValue(m.form.cur()); got != "preprod" {
		t.Errorf("↑ should land on second match 'preprod', got %q", got)
	}
}

func TestStrictFilterEnterAdoptsTopMatch(t *testing.T) {
	// With a strict filter highlighting the top match, Enter adopts it (rather
	// than committing the partial typed text, which strict would reject).
	be := newFakeBackend()
	be.data["env"] = fd("prod", "beta")
	m := New([]entry.Entry{strictEntry()}, be)
	m = send(m, key(tea.KeyEnter))

	m = send(m, runes("pr"))       // partial, matches prod only
	m = send(m, key(tea.KeyEnter)) // adopt the highlighted top match + run
	if m.Aborted {
		t.Fatalf("strict Enter on a filtered match should adopt it; err=%q", m.formErr)
	}
	if m.Result != "deploy --env prod" {
		t.Errorf("Result = %q, want deploy --env prod (top match adopted)", m.Result)
	}
}

func TestStrictFilterNoMatchStillRejects(t *testing.T) {
	// A strict filter with no matching candidate must not fabricate an adopt:
	// Enter is still blocked (there is nothing valid to pick).
	be := newFakeBackend()
	be.data["env"] = fd("prod", "beta")
	m := New([]entry.Entry{strictEntry()}, be)
	m = send(m, key(tea.KeyEnter))

	m = send(m, runes("xyz")) // matches nothing
	if m.form.cur().cursorHighlighted() {
		t.Error("strict filter with no matches must not highlight anything")
	}
	m = send(m, key(tea.KeyEnter))
	if !m.Aborted {
		t.Error("strict field with an unmatched filter should not finalize")
	}
}

func TestNonStrictAllowsFreeText(t *testing.T) {
	// Same entry but without Strict: free text is accepted.
	e := strictEntry()
	e.Vars[0].Strict = false
	be := newFakeBackend()
	be.data["env"] = fd("prod", "beta")
	m := New([]entry.Entry{e}, be)
	m = send(m, key(tea.KeyEnter))
	m = send(m, runes("staging"))
	m = send(m, key(tea.KeyEnter))
	if m.Aborted {
		t.Fatalf("non-strict field should accept free text; err=%q", m.formErr)
	}
	if m.Result != "deploy --env staging" {
		t.Errorf("Result = %q, want deploy --env staging", m.Result)
	}
}
