package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/resolve"
)

// ctxBackend wraps fakeBackend and returns empty data (without an error) for a
// fetch whose context was cancelled, like a killed src command would.
type ctxBackend struct{ *fakeBackend }

func (b ctxBackend) FieldData(ctx context.Context, e entry.Entry, f resolve.Field, resolved map[string]string) FieldData {
	d := b.fakeBackend.FieldData(ctx, e, f, resolved)
	if ctx.Err() != nil {
		return FieldData{}
	}
	return d
}

// Fix 1: a plain field with remembered values and an empty input shows nothing
// highlighted; the first ↑ highlights index 0 instead of skipping to index 1.
func TestFirstArrowHighlightsBottomCandidate(t *testing.T) {
	be := newFakeBackend()
	be.data["name"] = FieldData{Candidates: rankRemembered("one", "two", "three")}
	m := New([]entry.Entry{{
		Desc: "greet", Template: "greet <name>",
		Vars: []entry.VarSpec{{Name: "name"}}, Source: entry.SourceCheat,
	}}, be)
	m = send(m, key(tea.KeyEnter))
	fs := m.form.cur()
	if fs.cursorHighlighted() {
		t.Fatal("plain field with empty input should not highlight a remembered value")
	}
	m = send(m, key(tea.KeyUp))
	if got := highlightedValue(m.form.cur()); got != "one" || !m.form.cur().cursorHighlighted() {
		t.Fatalf("first ↑ should highlight index 0 (one), got %q", got)
	}
	m = send(m, key(tea.KeyUp))
	if got := highlightedValue(m.form.cur()); got != "two" {
		t.Fatalf("second ↑ should move to index 1 (two), got %q", got)
	}

	// ↓ from an unhighlighted state likewise only highlights index 0.
	m2 := New(optSelectEntry(), func() *fakeBackend { b := newFakeBackend(); b.data["env"] = fd("prod", "beta"); return b }())
	m2 = send(m2, key(tea.KeyEnter))
	m2 = send(m2, key(tea.KeyDown))
	if got := highlightedValue(m2.form.cur()); got != "prod" || !m2.form.cur().cursorHighlighted() {
		t.Fatalf("first ↓ should highlight index 0 (prod), got %q", got)
	}
}

// Fix 2: typing a value that equals a candidate keeps the list filtered; the
// full list returns only after the value was adopted.
func TestExactTypedMatchKeepsListFiltered(t *testing.T) {
	be := newFakeBackend()
	be.data["env"] = fd("dev", "devops", "staging")
	m := New([]entry.Entry{{
		Desc: "deploy", Template: "deploy <env> <app>",
		Vars:   []entry.VarSpec{{Name: "env", Src: "envs"}, {Name: "app"}},
		Source: entry.SourceCheat,
	}}, be)
	m = send(m, key(tea.KeyEnter))
	m = send(m, runes("de"))
	if n := len(m.form.cur().visibleCandidates()); n != 2 {
		t.Fatalf("typing de: %d visible, want 2", n)
	}
	m = send(m, runes("v"))
	if n := len(m.form.cur().visibleCandidates()); n != 2 {
		t.Fatalf("typing dev (exact candidate): %d visible, want still 2", n)
	}

	// Adopt via ↑ + Enter, then come back: the full list is browsable.
	m = send(m, key(tea.KeyCtrlD)) // clear
	m = send(m, key(tea.KeyEnter)) // quick-pick dev, advance to app
	m = send(m, key(tea.KeyCtrlP)) // back to env
	if fs := m.form.cur(); fs.input != "dev" || len(fs.visibleCandidates()) != 3 {
		t.Fatalf("after adopting, input=%q visible=%d, want dev and the full list of 3", fs.input, len(fs.visibleCandidates()))
	}
}

// Fix 3: a fetch cancelled by moving on is not stored; revisiting refetches.
// A src that returned nothing offers ^r refresh.
func TestCancelledFetchIsNotStored(t *testing.T) {
	fb := newFakeBackend()
	fb.data["a"] = fd("a1", "a2")
	fb.data["b"] = fd("b1")
	be := ctxBackend{fb}
	m := New([]entry.Entry{{
		Desc: "ab", Template: "ab <a> <b>",
		Vars:   []entry.VarSpec{{Name: "a", Src: "slow"}, {Name: "b", Src: "fast"}},
		Source: entry.SourceCheat,
	}}, be)
	next, loadA := m.Update(key(tea.KeyEnter)) // form, fetch of a pending
	m = next.(*Model)
	m = send(m, key(tea.KeyCtrlN)) // to b: cancels a's fetch
	m = send(m, loadA())           // a's late, cancelled response
	if a := m.form.fields[0]; a.loaded || a.loading {
		t.Fatalf("cancelled fetch stored: loaded=%v loading=%v", a.loaded, a.loading)
	}
	m = send(m, key(tea.KeyCtrlP)) // back to a: refetch
	if n := len(m.form.cur().data.Candidates); n != 2 {
		t.Fatalf("revisit should refetch a; candidates=%d", n)
	}
}

func TestEmptySrcOffersRefresh(t *testing.T) {
	be := newFakeBackend()
	be.data["env"] = FieldData{}
	m := New([]entry.Entry{{
		Desc: "deploy", Template: "deploy <env>",
		Vars: []entry.VarSpec{{Name: "env", Src: "envs"}}, Source: entry.SourceCheat,
	}}, be)
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 24})
	m = send(m, key(tea.KeyEnter))
	if !strings.Contains(stripANSI(m.View()), "^r refresh") {
		t.Fatalf("empty src should offer ^r refresh:\n%s", stripANSI(m.View()))
	}
	be.data["env"] = fd("prod")
	m = send(m, key(tea.KeyCtrlR))
	if len(be.fieldCalls) != 2 || len(m.form.cur().data.Candidates) != 1 {
		t.Fatalf("^r should refetch: calls=%v", be.fieldCalls)
	}
}

// Fix 4: a new query resets the list cursor to the best match.
func TestListQueryChangeResetsCursor(t *testing.T) {
	m := New(revEntries("alpha", "beta", "gamma"), newFakeBackend())
	m = send(m, key(tea.KeyUp))
	m = send(m, key(tea.KeyUp))
	if m.listCursor != 2 {
		t.Fatalf("cursor = %d, want 2", m.listCursor)
	}
	m = send(m, runes("a"))
	if m.listCursor != 0 {
		t.Fatalf("cursor after query change = %d, want 0", m.listCursor)
	}
}

// Fix 5: run validates every field, not only the current one.
func TestRunValidatesAllFields(t *testing.T) {
	be := newFakeBackend()
	be.data["env"] = fd("prod", "beta")
	m := New([]entry.Entry{{
		Desc: "deploy", Template: "deploy <env> <count> <app>",
		Vars: []entry.VarSpec{
			{Name: "env", Src: "envs", Strict: true},
			{Name: "count", Pattern: `[0-9]+`},
			{Name: "app"},
		},
		Source: entry.SourceCheat,
	}}, be)
	m = send(m, key(tea.KeyEnter))
	m = send(m, runes("nope"))     // invalid strict value
	m = send(m, key(tea.KeyCtrlN)) // leave without validation
	m = send(m, runes("abc"))      // invalid pattern
	m = send(m, key(tea.KeyCtrlN))
	m = send(m, runes("web"))
	m = send(m, key(tea.KeyEnter)) // run
	if m.Result != "" || m.form.current != 0 || !strings.Contains(m.formErr, "allowed values") {
		t.Fatalf("run should stop at env: result=%q current=%d err=%q", m.Result, m.form.current, m.formErr)
	}
	m = send(m, key(tea.KeyCtrlD))
	m = send(m, runes("prod"))
	m = send(m, key(tea.KeyCtrlP)) // wraps to app (last)
	m = send(m, key(tea.KeyEnter))
	if m.Result != "" || m.form.current != 1 || !strings.Contains(m.formErr, "pattern") {
		t.Fatalf("run should stop at count: result=%q current=%d err=%q", m.Result, m.form.current, m.formErr)
	}
}

// eightParamPreviewEntry has eight params; the last has candidates and an
// eight-line preview.
func eightParamPreviewEntry() entry.Entry {
	var vars []entry.VarSpec
	var tpl []string
	for i := 1; i <= 7; i++ {
		name := fmt.Sprintf("p%d", i)
		vars = append(vars, entry.VarSpec{Name: name, Default: "v"})
		tpl = append(tpl, "<"+name+">")
	}
	vars = append(vars, entry.VarSpec{Name: "pick", Src: "list", Preview: "show {}"})
	tpl = append(tpl, "<pick>")
	return entry.Entry{Desc: "many", Template: "run " + strings.Join(tpl, " "), Vars: vars, Source: entry.SourceCheat}
}

// Fix 6: at 80x24 with 8 params and an 8-line preview the candidate area keeps
// at least 5 rows and the box top stays on screen.
func TestPreviewDoesNotSqueezeCandidates(t *testing.T) {
	be := newFakeBackend()
	be.data["pick"] = fd("cand-1", "cand-2", "cand-3", "cand-4", "cand-5", "cand-6", "cand-7")
	be.previews["pick"] = func(c string, _ map[string]string) string {
		return strings.Repeat(c+" line\n", 8)
	}
	m := New([]entry.Entry{eightParamPreviewEntry()}, be)
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = send(m, key(tea.KeyEnter))
	for range 7 {
		m = send(m, key(tea.KeyCtrlN))
	}
	if m.form.cur().field.Name != "pick" {
		t.Fatalf("on %q, want pick", m.form.cur().field.Name)
	}
	ls := lines(m.View())
	if len(ls) > 24 {
		t.Fatalf("view is %d lines, want <= 24", len(ls))
	}
	if !strings.HasPrefix(ls[0], "╭") {
		t.Fatalf("box top clipped; first line %q", ls[0])
	}
	rows := 0
	for _, l := range ls {
		if strings.Contains(l, "cand-") && !strings.Contains(l, "line") && !strings.HasPrefix(l, "│") {
			rows++
		}
	}
	if rows < 5 {
		t.Fatalf("candidate rows = %d, want >= 5:\n%s", rows, strings.Join(ls, "\n"))
	}
}

// Fix 6 (height jump) and 7: while the next preview loads the previous one
// stays; every line of the preview box is indented alike.
func TestPreviewKeptWhileLoadingAndAligned(t *testing.T) {
	be := newFakeBackend()
	be.data["pod"] = fd("web-1", "web-2")
	be.previews["pod"] = func(c string, _ map[string]string) string { return "info " + c + "\nmore" }
	m := New([]entry.Entry{previewEntry()}, be)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 30})
	m = send(m, key(tea.KeyEnter))
	if !strings.Contains(stripANSI(m.View()), "info web-1") {
		t.Fatalf("preview missing:\n%s", stripANSI(m.View()))
	}
	next, _ := m.Update(key(tea.KeyUp)) // new preview requested, not delivered
	m = next.(*Model)
	view := stripANSI(m.View())
	if strings.Contains(view, "loading preview") || !strings.Contains(view, "info web-1") {
		t.Fatalf("previous preview should stay while loading:\n%s", view)
	}

	ls := lines(m.View())
	top := -1
	col := -1
	for i, l := range ls {
		if i > 0 && strings.Contains(l, "╭") {
			top, col = i, runeIndex(l, '╭')
			break
		}
	}
	if top < 0 {
		t.Fatalf("no preview box:\n%s", view)
	}
	for i := top + 1; i < len(ls); i++ {
		r := []rune(ls[i])
		if col >= len(r) {
			t.Fatalf("line %d too short for box column %d: %q", i, col, ls[i])
		}
		if r[col] == '╰' {
			return
		}
		if r[col] != '│' {
			t.Fatalf("preview box line %d misaligned (col %d): %q", i, col, ls[i])
		}
	}
	t.Fatal("preview box bottom not found")
}

func runeIndex(s string, want rune) int {
	for i, r := range []rune(s) {
		if r == want {
			return i
		}
	}
	return -1
}

// Fix 8: wide characters never overflow the input width.
func TestTextInputWideCharsFitWidth(t *testing.T) {
	in := newTextInput(strings.Repeat("日本語", 5))
	st := defaultStyles()
	for _, pos := range []int{0, 3, 8, 15} {
		in.SetCursor(pos)
		out := styledTextInputView(in, st.valueText, st.caret, 10)
		if w := ansi.StringWidth(out); w > 10 {
			t.Fatalf("pos %d: width %d > 10: %q", pos, w, stripANSI(out))
		}
		if !strings.Contains(out, "▏") {
			t.Fatalf("pos %d: caret missing", pos)
		}
	}
}

// Fix 9: ranking is memoized per (input, data) and refreshed on new data.
func TestCandidateRankingMemoized(t *testing.T) {
	fs := newFieldState(resolve.Field{Name: "x", Spec: entry.VarSpec{Name: "x", Src: "s"}})
	fs.applyData(fd("a", "b"), nil)
	v1 := fs.visibleCandidates()
	v2 := fs.visibleCandidates()
	if &v1[0] != &v2[0] {
		t.Error("visibleCandidates should be memoized between calls")
	}
	fs.applyData(fd("c"), nil)
	if v := fs.visibleCandidates(); len(v) != 1 || v[0].Value != "c" || !fs.matchesCandidate("c") || fs.matchesCandidate("a") {
		t.Errorf("memo not invalidated on new data: %v", v)
	}
}

// Fix 10: a single ^d asks for confirmation; moving away disarms it.
func TestDeleteBookmarkNeedsConfirmation(t *testing.T) {
	be := newFakeBackend()
	entries := []entry.Entry{
		{Title: "b1", Template: "echo b1", Kind: entry.CommandLiteral, Source: entry.SourceBookmark},
		{Title: "b2", Template: "echo b2", Kind: entry.CommandLiteral, Source: entry.SourceBookmark},
	}
	m := New(entries, be)
	m = send(m, key(tea.KeyCtrlD))
	if len(be.removed) != 0 || !strings.Contains(m.status, "again") {
		t.Fatalf("first ^d should ask: removed=%v status=%q", be.removed, m.status)
	}
	m = send(m, key(tea.KeyUp)) // move: disarm
	m = send(m, key(tea.KeyCtrlD))
	if len(be.removed) != 0 {
		t.Fatalf("^d on a new row must ask again, removed=%v", be.removed)
	}
	send(m, key(tea.KeyCtrlD))
	if len(be.removed) != 1 {
		t.Fatalf("second ^d should delete, removed=%v", be.removed)
	}
}

// Fix 11: bookmarking from the form remembers nothing.
func TestBookmarkFromFormDoesNotRemember(t *testing.T) {
	be := newFakeBackend()
	be.data["env"] = fd("prod")
	m := New([]entry.Entry{strictEntry()}, be)
	m = send(m, key(tea.KeyCtrlB))
	send(m, key(tea.KeyEnter))
	if len(be.bookmarked) != 1 || len(be.remembered) != 0 {
		t.Fatalf("bookmarked=%v remembered=%v", be.bookmarked, be.remembered)
	}
}

// Fix 12: Enter on a required multi field with nothing selected selects the
// highlighted candidate.
func TestRequiredMultiEnterSelectsHighlighted(t *testing.T) {
	be := newFakeBackend()
	be.data["file"] = fd("a.txt", "b.txt")
	m := New([]entry.Entry{{
		Desc: "cat", Template: "cat <file>",
		Vars:   []entry.VarSpec{{Name: "file", Src: "ls", Multi: true}},
		Source: entry.SourceCheat,
	}}, be)
	m = send(m, key(tea.KeyEnter))
	m = send(m, key(tea.KeyUp)) // highlight b.txt
	m = send(m, key(tea.KeyEnter))
	if m.Result != "cat b.txt" {
		t.Fatalf("Result = %q, want cat b.txt", m.Result)
	}
}

// Fix 13: preview text has ANSI stripped and tabs expanded.
func TestSanitizePreviewText(t *testing.T) {
	got := sanitizePreviewText("\x1b[31mred\x1b[0m\tx\r\nab\tc\a")
	if want := "red     x\nab      c"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Fix 14: a strict field whose dependency changed after it was filled is
// refetched before running, and its stale value is dropped.
func TestRunRefetchesStaleStrictDependent(t *testing.T) {
	be := newFakeBackend()
	be.data["namespace"] = fd("ns1", "ns2")
	be.dynamic["pod"] = func(resolved map[string]string) FieldData {
		return fd("pod-" + resolved["namespace"])
	}
	m := New([]entry.Entry{{
		Desc: "q", Template: "kubectl logs <pod> <note>",
		Vars: []entry.VarSpec{
			{Name: "pod", Src: "kubectl get pods -n <namespace> -o name", Strict: true},
			{Name: "namespace", Src: "kubectl get namespaces -o name"},
			{Name: "note"},
		},
		Source: entry.SourceCheat,
	}}, be)
	m = send(m, key(tea.KeyEnter))
	if names := []string{m.form.fields[0].field.Name, m.form.fields[1].field.Name, m.form.fields[2].field.Name}; strings.Join(names, ",") != "namespace,pod,note" {
		t.Fatalf("field order = %v", names)
	}
	m = send(m, key(tea.KeyEnter)) // pick namespace ns1
	m = send(m, key(tea.KeyEnter)) // picks pod-ns1 for pod
	m = send(m, runes("x"))        // note
	m = send(m, key(tea.KeyCtrlN)) // wrap to namespace
	m = send(m, key(tea.KeyUp))
	m = send(m, key(tea.KeyUp))    // ns2
	m = send(m, key(tea.KeyCtrlP)) // wrap back to note, committing ns2
	m = send(m, key(tea.KeyEnter)) // run: pod is stale
	if m.Result != "" || m.form.cur().field.Name != "pod" {
		t.Fatalf("run should re-check pod first; result=%q on %q", m.Result, m.form.cur().field.Name)
	}
	if fs := m.form.cur(); fs.input != "" || be.lastResolved["pod"]["namespace"] != "ns2" {
		t.Fatalf("stale strict value kept: input=%q fetched with %v", fs.input, be.lastResolved["pod"])
	}
	m = send(m, key(tea.KeyEnter)) // quick-pick pod-ns2, to note
	m = send(m, key(tea.KeyEnter)) // run
	if m.Result != "kubectl logs pod-ns2 x" {
		t.Fatalf("Result = %q, want kubectl logs pod-ns2 x", m.Result)
	}
}
