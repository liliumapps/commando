package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
)

func TestFieldDataLoadsOnlyFocusedField(t *testing.T) {
	be := newFakeBackend()
	m := New(sampleEntries(), be)
	m = send(m, key(tea.KeyEnter))

	if got := strings.Join(be.fieldCalls, ","); got != "author" {
		t.Fatalf("initial field loads = %q, want author only", got)
	}
	send(m, key(tea.KeyCtrlN))
	if got := strings.Join(be.fieldCalls, ","); got != "author,count" {
		t.Fatalf("loads after navigation = %q, want author,count", got)
	}
}

func TestCandidateSourceFailureIsVisibleAndRetryable(t *testing.T) {
	be := newFakeBackend()
	be.data["env"] = FieldData{Error: "lookup failed"}
	e := entry.Entry{
		Title:    "deploy",
		Template: "deploy <env>",
		Vars:     []entry.VarSpec{{Name: "env", Src: "printf 'dev\\nstaging\\nprod\\n'"}},
		Source:   entry.SourceCheat,
	}
	m := New([]entry.Entry{e}, be)
	m = send(m, key(tea.KeyEnter))
	view := stripANSI(m.View())
	if !strings.Contains(view, "candidate source failed: lookup failed") ||
		!strings.Contains(view, "^r retry") {
		t.Fatalf("candidate error not actionable:\n%s", view)
	}

	send(m, key(tea.KeyCtrlR))
	if len(be.fieldCalls) != 2 {
		t.Fatalf("retry field calls = %d, want 2", len(be.fieldCalls))
	}
}

func TestOptionalPreviewWaitsForCandidateEngagement(t *testing.T) {
	be := newFakeBackend()
	be.data["pod"] = fd("web-1")
	be.previews["pod"] = func(candidate string, _ map[string]string) string {
		return candidate
	}
	e := entry.Entry{
		Title:    "logs",
		Template: "logs [<pod>]",
		Vars:     []entry.VarSpec{{Name: "pod", Src: "pods", Preview: "show {}"}},
		Source:   entry.SourceCheat,
	}
	m := New([]entry.Entry{e}, be)
	m = send(m, key(tea.KeyEnter))
	if len(be.previewCalls) != 0 {
		t.Fatalf("untouched optional field ran preview: %v", be.previewCalls)
	}
	send(m, key(tea.KeyUp))
	if len(be.previewCalls) != 1 || be.previewCalls[0] != "pod:web-1" {
		t.Fatalf("engaged preview calls = %v", be.previewCalls)
	}
}
