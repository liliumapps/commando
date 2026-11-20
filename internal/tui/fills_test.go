package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
)

// fakeFillsBackend adds recent fills to fakeBackend.
type fakeFillsBackend struct {
	*fakeBackend
	fills       []Fill
	history     []entry.Entry
	rememberedF []map[string]string
	forgotten   []map[string]string
}

func (b *fakeFillsBackend) RecentFills(_ context.Context, _ entry.Entry, history []entry.Entry) []Fill {
	b.history = history
	return append([]Fill(nil), b.fills...)
}

func (b *fakeFillsBackend) RememberFill(_ entry.Entry, values map[string]string) {
	b.rememberedF = append(b.rememberedF, values)
}

func (b *fakeFillsBackend) ForgetFill(_ entry.Entry, values map[string]string) error {
	b.forgotten = append(b.forgotten, values)
	return nil
}

func logsCheat() entry.Entry {
	return entry.Entry{
		Title:    "pod logs",
		Template: "kubectl logs <pod> -n <ns> [--since <since>]",
		Vars:     []entry.VarSpec{{Name: "pod", Src: "pods"}, {Name: "ns"}, {Name: "since"}},
		Source:   entry.SourceCheat,
	}
}

var fillNow = time.Unix(1_763_640_000, 0)

func newFillsModel(fills ...Fill) (*Model, *fakeFillsBackend) {
	be := &fakeFillsBackend{fakeBackend: newFakeBackend(), fills: fills}
	be.data["pod"] = fd("api-1", "api-2", "web-1", "web-2", "db-1", "db-2", "cache-1", "cache-2")
	entries := []entry.Entry{logsCheat(), {Template: "kubectl get pods", Kind: entry.CommandLiteral, Source: entry.SourceHistory, ProviderOrder: 3}}
	m := New(entries, be)
	m.now = func() time.Time { return fillNow }
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = send(m, key(tea.KeyEnter)) // open the form
	return m, be
}

func twoFills() []Fill {
	return []Fill{
		{Values: map[string]string{"pod": "api-1", "ns": "payments"}, Count: 4, LastUsed: fillNow.Add(-2 * time.Hour).Unix()},
		{Values: map[string]string{"pod": "web-1", "ns": "shop", "since": "1h"}, Count: 1, FromHistory: true},
	}
}

func TestRecentBlockShownWithFills(t *testing.T) {
	m, be := newFillsModel(twoFills()...)
	if !m.recentShown() {
		t.Fatal("Recent block should be shown when fills exist")
	}
	view := stripANSI(m.View())
	for _, want := range []string{"recent", "pod=api-1  ns=payments   (4×, 2h ago)", "pod=web-1  ns=shop  since=1h   (1×) from history", "alt+⏎ run"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	if len(be.history) != 1 || be.history[0].Template != "kubectl get pods" {
		t.Errorf("history passed to backend = %+v", be.history)
	}
}

func TestRecentBlockAbsentWithoutFills(t *testing.T) {
	m, _ := newFillsModel()
	if m.recentShown() || strings.Contains(stripANSI(m.View()), "recent") {
		t.Fatal("no fills: no Recent block")
	}
	// A plain backend (no FillsBackend) behaves as before.
	plain := New([]entry.Entry{logsCheat()}, newFakeBackend())
	plain = send(plain, key(tea.KeyEnter))
	if plain.recentShown() {
		t.Fatal("plain backend shows a Recent block")
	}
}

func TestRecentEnterFillsAllFields(t *testing.T) {
	m, _ := newFillsModel(twoFills()...)
	m = send(m, key(tea.KeyUp)) // to the history row
	m = send(m, key(tea.KeyEnter))
	// Every field is filled, so focus lands on the last one: Enter runs.
	if last := len(m.form.fields) - 1; m.recentShown() || m.stage != stageForm || m.form.current != last {
		t.Fatalf("after Enter: shown=%v stage=%v current=%d, want %d", m.recentShown(), m.stage, m.form.current, last)
	}
	if got := m.form.preview(); got != "kubectl logs web-1 -n shop --since 1h" {
		t.Fatalf("preview = %q", got)
	}
	m = send(m, key(tea.KeyEnter))
	if m.Result != "kubectl logs web-1 -n shop --since 1h" {
		t.Fatalf("Result = %q", m.Result)
	}
}

func TestRecentFillSkipsAbsentOptional(t *testing.T) {
	m, _ := newFillsModel(twoFills()...)
	m.form.fields[2].setInput("old")
	m = send(m, key(tea.KeyEnter)) // the stored fill has no "since"
	if !m.form.fields[2].skipped {
		t.Fatal("optional field absent from the fill should be skipped")
	}
	if got := m.form.preview(); got != "kubectl logs api-1 -n payments" {
		t.Fatalf("preview = %q", got)
	}
}

func TestRecentAltEnterRuns(t *testing.T) {
	for _, k := range []tea.KeyMsg{{Type: tea.KeyEnter, Alt: true}, {Type: tea.KeyCtrlJ}} {
		m, be := newFillsModel(twoFills()...)
		m = send(m, k)
		if m.Aborted || m.Result != "kubectl logs api-1 -n payments" {
			t.Fatalf("%v: Result=%q aborted=%v", k, m.Result, m.Aborted)
		}
		if len(be.rememberedF) != 1 || be.rememberedF[0]["ns"] != "payments" {
			t.Fatalf("%v: fill not remembered: %+v", k, be.rememberedF)
		}
	}
}

func TestRecentAltEnterValidates(t *testing.T) {
	be := &fakeFillsBackend{fakeBackend: newFakeBackend(), fills: []Fill{{Values: map[string]string{"pod": "gone", "ns": "x"}, Count: 1}}}
	be.data["pod"] = fd("api-1")
	e := logsCheat()
	e.Vars[0].Strict = true
	m := New([]entry.Entry{e}, be)
	m = send(m, key(tea.KeyEnter))
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	if !m.Aborted || m.Result != "" || !strings.Contains(m.formErr, "not one of the allowed values") {
		t.Fatalf("strict violation should block the run: result=%q err=%q", m.Result, m.formErr)
	}
}

func TestRecentCtrlDConfirmsDelete(t *testing.T) {
	m, be := newFillsModel(twoFills()...)
	m = send(m, key(tea.KeyCtrlD))
	if len(be.forgotten) != 0 || !strings.Contains(stripANSI(m.View()), "press ^d again") {
		t.Fatalf("first ^d should only arm: forgotten=%v", be.forgotten)
	}
	m = send(m, key(tea.KeyCtrlD))
	if len(be.forgotten) != 1 || be.forgotten[0]["pod"] != "api-1" {
		t.Fatalf("second ^d should delete: %v", be.forgotten)
	}
	if len(m.form.recent.fills) != 1 || !m.recentShown() {
		t.Fatalf("remaining fills = %+v", m.form.recent.fills)
	}
	// History-only rows cannot be deleted.
	m = send(m, key(tea.KeyCtrlD))
	send(m, key(tea.KeyCtrlD))
	if len(be.forgotten) != 1 || !strings.Contains(stripANSI(m.View()), "history rows can't be deleted") {
		t.Fatalf("history row deleted: %v", be.forgotten)
	}
}

func TestRecentCtrlDDisarmedByOtherKey(t *testing.T) {
	m, be := newFillsModel(twoFills()...)
	m = send(m, key(tea.KeyCtrlD))
	m = send(m, key(tea.KeyUp))
	m = send(m, key(tea.KeyDown))
	send(m, key(tea.KeyCtrlD))
	if len(be.forgotten) != 0 {
		t.Fatalf("a key between presses should disarm: %v", be.forgotten)
	}
}

func TestRecentTypingHidesBlock(t *testing.T) {
	m, _ := newFillsModel(twoFills()...)
	m = send(m, runes("a"))
	if m.recentShown() {
		t.Fatal("typing should hide the block")
	}
	if m.form.cur().input != "a" {
		t.Fatalf("typed character lost: %q", m.form.cur().input)
	}
	m = send(m, key(tea.KeyUp)) // now the field's candidates
	if m.recentShown() || !m.form.cur().navigated {
		t.Fatal("arrows should drive candidates once the block is hidden")
	}
}

func TestRecentDownPastBlockHides(t *testing.T) {
	m, _ := newFillsModel(twoFills()...)
	m = send(m, key(tea.KeyUp))
	m = send(m, key(tea.KeyDown))
	if !m.recentShown() || m.form.recent.cursor != 0 {
		t.Fatal("↓ inside the block moves the cursor")
	}
	m = send(m, key(tea.KeyDown))
	if m.recentShown() {
		t.Fatal("↓ past the block hides it")
	}
	if m.form.cur().input != "" || m.form.cur().navigated {
		t.Fatal("hiding the block should not touch the field")
	}
}

func TestRecentEscReturnsToList(t *testing.T) {
	m, _ := newFillsModel(twoFills()...)
	m = send(m, key(tea.KeyEsc))
	if m.stage != stageList {
		t.Fatal("Esc should return to the list")
	}
}

func TestRecentNotShownAfterUserStarted(t *testing.T) {
	be := &fakeFillsBackend{fakeBackend: newFakeBackend(), fills: twoFills()}
	m := New([]entry.Entry{logsCheat()}, be)
	next, cmd := m.Update(key(tea.KeyEnter))
	m = next.(*Model)
	m = send(m, runes("x")) // typed before fills arrive
	m = runAll(m, cmd)
	if m.recentShown() {
		t.Fatal("late fills must not appear after the user started typing")
	}
}

// runAll executes cmd (and any batch) through the model.
func runAll(m *Model, cmd tea.Cmd) *Model {
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = runAll(m, c)
		}
		return m
	}
	return send(m, msg)
}

func TestBookmarkDoesNotRememberFill(t *testing.T) {
	be := &fakeFillsBackend{fakeBackend: newFakeBackend(), fills: twoFills()}
	m := New([]entry.Entry{logsCheat()}, be)
	m = send(m, key(tea.KeyCtrlB))
	send(m, tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	if len(be.bookmarked) != 1 || len(be.rememberedF) != 0 {
		t.Fatalf("bookmarked=%v rememberedFills=%v", be.bookmarked, be.rememberedF)
	}
}

func TestRecentLayoutAt80x24KeepsCandidates(t *testing.T) {
	var many []Fill
	for _, p := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		many = append(many, Fill{Values: map[string]string{"pod": p, "ns": "n"}, Count: 1})
	}
	m, _ := newFillsModel(many...)
	ls := lines(m.View())
	if len(ls) != 24 {
		t.Fatalf("form view has %d lines, want 24", len(ls))
	}
	cands := 0
	recentRows := 0
	for _, l := range ls {
		for _, c := range []string{"api-1", "api-2", "web-1", "web-2", "db-1", "db-2", "cache-1", "cache-2"} {
			if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "❯")) == c {
				cands++
			}
		}
		if strings.Contains(l, "  ns=n") {
			recentRows++
		}
	}
	if cands < minCandidateRows {
		t.Errorf("only %d candidate rows at 80x24:\n%s", cands, strings.Join(ls, "\n"))
	}
	if recentRows == 0 || recentRows > maxRecentRows {
		t.Errorf("recent rows = %d:\n%s", recentRows, strings.Join(ls, "\n"))
	}
	// A very short terminal hides the block, and keys work as usual.
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 12})
	if m.recentShown() {
		t.Errorf("block should yield at 80x12:\n%s", stripANSI(m.View()))
	}
}

func TestRecentScrolls(t *testing.T) {
	var many []Fill
	for _, p := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		many = append(many, Fill{Values: map[string]string{"pod": p, "ns": "n"}, Count: 1})
	}
	m, _ := newFillsModel(many...)
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	if !strings.Contains(stripANSI(m.View()), "… 2 more") {
		t.Fatalf("expected an overflow note:\n%s", stripANSI(m.View()))
	}
	for i := 0; i < 6; i++ {
		m = send(m, key(tea.KeyUp))
	}
	if !strings.Contains(stripANSI(m.View()), "pod=g  ns=n") {
		t.Fatalf("scrolling should reveal the last fill:\n%s", stripANSI(m.View()))
	}
}

// A fill that leaves a required field empty lands focus on that field.
func TestRecentEnterFocusesFirstUnfilledRequired(t *testing.T) {
	m, _ := newFillsModel(Fill{Values: map[string]string{"pod": "api-1"}, Count: 1})
	m = send(m, key(tea.KeyEnter))
	if fs := m.form.cur(); fs.field.Name != "ns" {
		t.Fatalf("focus on %q, want the empty required ns", fs.field.Name)
	}
}

// A strict field later in the form has not loaded its candidates when a fill
// is picked; that must not count as invalid, or focus would stop on it.
func TestRecentEnterSkipsUnloadedStrictField(t *testing.T) {
	be := &fakeFillsBackend{fakeBackend: newFakeBackend(), fills: []Fill{
		{Values: map[string]string{"account": "billing", "region": "NA", "role": "Dev"}, Count: 1},
	}}
	be.data["region"] = fd("NA", "EU")
	be.data["role"] = fd("Dev", "Ops")
	e := entry.Entry{
		Title:    "ssh",
		Template: "ssh-host --account <account> --region <region> --role <role>",
		Vars: []entry.VarSpec{
			{Name: "account"},
			{Name: "region", Values: []string{"NA", "EU"}, Strict: true},
			{Name: "role", Values: []string{"Dev", "Ops"}, Strict: true},
		},
		Source: entry.SourceCheat,
	}
	m := New([]entry.Entry{e}, be)
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = send(m, key(tea.KeyEnter))
	m = send(m, key(tea.KeyEnter)) // pick the fill
	if got, last := m.form.current, len(m.form.fields)-1; got != last {
		t.Fatalf("focus on field %d (%s), want the last field %d", got, m.form.cur().field.Name, last)
	}
	m = send(m, key(tea.KeyEnter))
	if m.Result != "ssh-host --account billing --region NA --role Dev" {
		t.Fatalf("Result = %q; err=%q on %s", m.Result, m.formErr, m.form.cur().field.Name)
	}
}
