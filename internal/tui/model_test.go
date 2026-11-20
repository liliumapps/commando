package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/rank"
	"github.com/liliumapps/commando/internal/resolve"
)

// fakeBackend supplies canned candidate data and records remembered values.
type fakeBackend struct {
	data       map[string]FieldData
	remembered map[string][]string
	// dynamic, when set for a field name, computes FieldData from the resolved
	// values passed in — used to exercise cross-variable dependencies.
	dynamic map[string]func(resolved map[string]string) FieldData
	// lastResolved records the resolved map most recently passed to FieldData
	// for each field, so tests can assert what a dependent field saw.
	lastResolved map[string]map[string]string
	// previews, when set for a field name, computes preview text from the
	// highlighted candidate and resolved values.
	previews map[string]func(candidate string, resolved map[string]string) string
	// bookmarked records commands bookmarked via the star key.
	bookmarked   []string
	removed      []string
	fieldCalls   []string
	previewCalls []string
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		data:         map[string]FieldData{},
		remembered:   map[string][]string{},
		dynamic:      map[string]func(map[string]string) FieldData{},
		lastResolved: map[string]map[string]string{},
		previews:     map[string]func(string, map[string]string) string{},
	}
}

func (b *fakeBackend) FieldData(_ context.Context, _ entry.Entry, f resolve.Field, resolved map[string]string) FieldData {
	b.fieldCalls = append(b.fieldCalls, f.Name)
	b.lastResolved[f.Name] = resolved
	if fn, ok := b.dynamic[f.Name]; ok {
		return fn(resolved)
	}
	if d, ok := b.data[f.Name]; ok {
		return d
	}
	return FieldData{Prefill: f.Spec.Default}
}

func (b *fakeBackend) Remember(_ entry.Entry, f resolve.Field, value string) {
	b.remembered[f.Name] = append(b.remembered[f.Name], value)
}

func (b *fakeBackend) Preview(_ context.Context, _ entry.Entry, f resolve.Field, candidate string, resolved map[string]string) PreviewData {
	b.previewCalls = append(b.previewCalls, f.Name+":"+candidate)
	if fn, ok := b.previews[f.Name]; ok {
		return PreviewData{Text: fn(candidate, resolved)}
	}
	return PreviewData{}
}

// fd builds FieldData from plain string candidates (never-used src values), a
// convenience for tests that don't care about usage data. Each string becomes a
// src Candidate with Display == Value.
func fd(candidates ...string) FieldData {
	src := make([]rank.Candidate, len(candidates))
	for i, c := range candidates {
		src[i] = rank.Candidate{Display: c, Value: c}
	}
	return FieldData{Candidates: rank.BuildCandidates(nil, src)}
}

// rankRemembered builds candidates that look like previously-used (remembered)
// values, in most-recent-first order, for testing plain fields.
func rankRemembered(values ...string) []rank.Candidate {
	rem := make([]rank.Candidate, len(values))
	ts := int64(1000)
	for i, v := range values {
		rem[i] = rank.Candidate{Value: v, LastUsed: ts, UseCount: 1}
		ts-- // earlier in the list = more recent
	}
	return rank.BuildCandidates(rem, nil)
}

func (b *fakeBackend) Bookmark(cmd string) error {
	b.bookmarked = append(b.bookmarked, cmd)
	return nil
}

func (b *fakeBackend) RemoveBookmark(cmd string) error {
	b.removed = append(b.removed, cmd)
	return nil
}

// send routes a key message through the model and returns it for chaining.
func send(m *Model, msg tea.Msg) *Model {
	next, cmd := m.Update(msg)
	m = next.(*Model)
	for cmd != nil {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, child := range batch {
				if child != nil {
					m = send(m, child())
				}
			}
			return m
		}
		next, cmd = m.Update(msg)
		m = next.(*Model)
	}
	return m
}

func runes(s string) tea.Msg    { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }

func sampleEntries() []entry.Entry {
	return []entry.Entry{
		{
			Desc:     "Search git log by author for a file",
			Tags:     []string{"git", "log"},
			Template: "git log [--author=<author>] [-n <count>] -- <file>",
			Vars: []entry.VarSpec{
				{Name: "author"},
				{Name: "count", Default: "20"},
				{Name: "file", Multi: true, Each: `"{}"`, Remember: entry.ScopeProject},
			},
			Source: entry.SourceCheat,
		},
		{
			Desc:     "npm run build",
			Template: "npm run build",
			Kind:     entry.CommandLiteral,
			Source:   entry.SourceProject,
		},
	}
}

func TestListFilterAndPlainSelect(t *testing.T) {
	m := New(sampleEntries(), newFakeBackend())
	// Filter to the project command, which has no vars, and select it.
	m = send(m, runes("npm"))
	if len(m.filtered) != 1 || m.filtered[0].Desc != "npm run build" {
		t.Fatalf("filter failed: %+v", m.filtered)
	}
	m = send(m, key(tea.KeyEnter))
	if m.Aborted {
		t.Fatal("selecting a no-var command should not abort")
	}
	if m.Result != "npm run build" {
		t.Errorf("Result = %q, want npm run build", m.Result)
	}
}

func TestFormFillOptionalMultiWithNavigation(t *testing.T) {
	be := newFakeBackend()
	be.data["file"] = fd("a.go", "b.go", "c.go")
	m := New(sampleEntries(), be)

	// Select the git entry (first, default cursor).
	m = send(m, key(tea.KeyEnter))
	if m.stage != stageForm {
		t.Fatal("should be on form stage")
	}

	// Params: author(opt), count(opt, default 20), file(required, multi).
	// Focus starts on author. Navigate right to the file param.
	if m.form.cur().field.Name != "author" {
		t.Fatalf("first param = %q, want author", m.form.cur().field.Name)
	}
	m = send(m, key(tea.KeyCtrlN)) // -> count
	if m.form.cur().field.Name != "count" || m.form.cur().input != "20" {
		t.Fatalf("second param = %q input %q, want count/20", m.form.cur().field.Name, m.form.cur().input)
	}
	m = send(m, key(tea.KeyCtrlN)) // -> file
	if m.form.cur().field.Name != "file" {
		t.Fatalf("third param = %q, want file", m.form.cur().field.Name)
	}

	// Multi-select a.go and c.go via Space (Space toggles, does not advance).
	// Candidates are reversed: Up moves to a higher index.
	m = send(m, key(tea.KeySpace)) // toggle a.go (cursor 0)
	m = send(m, key(tea.KeyUp))
	m = send(m, key(tea.KeyUp))
	m = send(m, key(tea.KeySpace)) // toggle c.go (cursor 2)

	// Enter now: author/count optional (count has default 20), file satisfied.
	m = send(m, key(tea.KeyEnter))

	if m.Aborted {
		t.Fatalf("form should have produced a command; err=%q", m.formErr)
	}
	want := `git log -n 20 -- "a.go" "c.go"`
	if m.Result != want {
		t.Errorf("Result = %q, want %q", m.Result, want)
	}
	if got := be.remembered["file"]; len(got) != 2 || got[0] != "a.go" || got[1] != "c.go" {
		t.Errorf("remembered file = %v, want [a.go c.go]", got)
	}
}

func TestEnterAdvancesLinearly(t *testing.T) {
	m := New(sampleEntries(), newFakeBackend())
	m = send(m, key(tea.KeyEnter)) // into form, focus author (param 0)

	// Enter moves to the NEXT param (count), not the unmet required (file).
	m = send(m, key(tea.KeyEnter))
	if m.form.cur().field.Name != "count" {
		t.Errorf("Enter should move to the next param (count), got %q", m.form.cur().field.Name)
	}
	if !m.Aborted {
		t.Error("Enter mid-form should not run the command")
	}
}

// multiReqEntry: params a(req), b(opt), c(req) — for required-jump navigation.
func multiReqEntry() []entry.Entry {
	return []entry.Entry{{
		Desc:     "three",
		Template: "cmd <a> [--b <b>] <c>",
		Vars:     []entry.VarSpec{{Name: "a"}, {Name: "b"}, {Name: "c"}},
		Source:   entry.SourceCheat,
	}}
}

func TestTabJumpsToNextRequired(t *testing.T) {
	m := New(multiReqEntry(), newFakeBackend())
	m = send(m, key(tea.KeyEnter)) // form, focus a (required, index 0)
	if m.form.cur().field.Name != "a" {
		t.Fatalf("first param = %q, want a", m.form.cur().field.Name)
	}
	// Tab skips the optional b and lands on the next required, c.
	m = send(m, key(tea.KeyTab))
	if m.form.cur().field.Name != "c" {
		t.Errorf("Tab should jump to next required (c), got %q", m.form.cur().field.Name)
	}
	// Tab again wraps back to the first required, a.
	m = send(m, key(tea.KeyTab))
	if m.form.cur().field.Name != "a" {
		t.Errorf("Tab should wrap to first required (a), got %q", m.form.cur().field.Name)
	}
}

func TestShiftTabJumpsToPrevRequired(t *testing.T) {
	m := New(multiReqEntry(), newFakeBackend())
	m = send(m, key(tea.KeyEnter))    // focus a
	m = send(m, key(tea.KeyTab))      // -> c
	m = send(m, key(tea.KeyShiftTab)) // back to a (prev required)
	if m.form.cur().field.Name != "a" {
		t.Errorf("Shift+Tab should jump to prev required (a), got %q", m.form.cur().field.Name)
	}
}

func TestFormClearValue(t *testing.T) {
	be := newFakeBackend()
	be.data["file"] = fd("a.go", "b.go")
	m := New(sampleEntries(), be)
	m = send(m, key(tea.KeyEnter)) // form, author
	m = send(m, key(tea.KeyCtrlN)) // -> count (has default "20")
	if m.form.cur().input != "20" {
		t.Fatalf("count should prefill 20, got %q", m.form.cur().input)
	}
	m = send(m, key(tea.KeyCtrlD)) // clear count
	if m.form.cur().input != "" {
		t.Errorf("Ctrl+D should clear the value, got %q", m.form.cur().input)
	}
}

func TestEscReturnsToList(t *testing.T) {
	m := New(sampleEntries(), newFakeBackend())
	m = send(m, key(tea.KeyEnter)) // into form
	if m.stage != stageForm {
		t.Fatal("expected form stage")
	}
	m = send(m, key(tea.KeyEsc)) // back to list
	if m.stage != stageList {
		t.Errorf("Esc should return to list, stage=%v", m.stage)
	}
}

func TestPreviewUpdatesLive(t *testing.T) {
	m := New(sampleEntries(), newFakeBackend())
	m = send(m, key(tea.KeyEnter)) // into form (author param)
	// Type an author value; preview reflects it. count carries a default of 20
	// (a default means "use this unless skipped"), so its optional segment is
	// present; the required <file> shows as a placeholder.
	m = send(m, runes("alice"))
	got := m.form.preview()
	want := `git log --author=alice -n 20 -- <file>`
	if got != want {
		t.Errorf("preview = %q, want %q", got, want)
	}

	// Navigate right to count and skip it (^s) — its segment drops from preview.
	m = send(m, key(tea.KeyCtrlN)) // -> count
	if m.form.cur().field.Name != "count" {
		t.Fatalf("expected count, got %q", m.form.cur().field.Name)
	}
	m = send(m, key(tea.KeyCtrlS)) // skip count
	got = m.form.preview()
	want = `git log --author=alice -- <file>`
	if got != want {
		t.Errorf("preview after skipping count = %q, want %q", got, want)
	}
}

func TestAbortWithCtrlC(t *testing.T) {
	m := New(sampleEntries(), newFakeBackend())
	m = send(m, key(tea.KeyCtrlC))
	if !m.Aborted {
		t.Error("Ctrl+C should abort")
	}
}

func TestViewsRenderWithoutPanic(t *testing.T) {
	m := New(sampleEntries(), newFakeBackend())
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.View() == "" {
		t.Error("list view should render non-empty")
	}
	m = send(m, key(tea.KeyEnter))
	if m.View() == "" {
		t.Error("form view should render non-empty")
	}
}
