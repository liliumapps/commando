package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
)

func previewEntry() entry.Entry {
	return entry.Entry{
		Desc:     "describe a pod",
		Template: "kubectl describe pod <pod>",
		Vars: []entry.VarSpec{
			{Name: "pod", Src: "get-pods", Preview: "kubectl describe pod {}"},
		},
		Source: entry.SourceCheat,
	}
}

func TestCandidatePreviewFollowsCursor(t *testing.T) {
	be := newFakeBackend()
	be.data["pod"] = fd("web-1", "web-2", "db-0")
	be.previews["pod"] = func(candidate string, _ map[string]string) string {
		return "details for " + candidate
	}
	m := New([]entry.Entry{previewEntry()}, be)
	m = send(m, key(tea.KeyEnter)) // into form, pod field, cursor at 0

	if got := m.form.candidatePreview(); got != "details for web-1" {
		t.Errorf("preview at cursor 0 = %q, want details for web-1", got)
	}
	m = send(m, key(tea.KeyUp)) // reversed list: Up moves to a higher index -> web-2
	if got := m.form.candidatePreview(); got != "details for web-2" {
		t.Errorf("preview at cursor 1 = %q, want details for web-2", got)
	}

	// The rendered form should include the preview text.
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	if !strings.Contains(m.View(), "details for web-2") {
		t.Error("form view should render the candidate preview")
	}
}

func TestCandidatePreviewReceivesResolvedRefs(t *testing.T) {
	// Both <ns> and <pod> are template variables; ns is resolved first, and the
	// pod field's preview references the chosen ns via the resolved map.
	e := entry.Entry{
		Desc:     "logs",
		Template: "kubectl logs -n <ns> <pod>",
		Vars: []entry.VarSpec{
			{Name: "ns", Src: "get-ns"},
			{Name: "pod", Src: "get-pods", Preview: "kubectl logs <pod> -n <ns>"},
		},
		Source: entry.SourceCheat,
	}
	be := newFakeBackend()
	be.data["pod"] = fd("p1")
	be.data["ns"] = fd("kube-system")
	var sawNS string
	be.previews["pod"] = func(candidate string, resolved map[string]string) string {
		sawNS = resolved["ns"]
		return "ok"
	}
	m := New([]entry.Entry{e}, be)
	m = send(m, key(tea.KeyEnter)) // field 0 = ns (template order)
	if m.form.cur().field.Name != "ns" {
		t.Fatalf("first field = %q, want ns", m.form.cur().field.Name)
	}
	// Enter adopts the highlighted ns candidate (kube-system) and advances to pod.
	m = send(m, key(tea.KeyEnter))

	if m.form.cur().field.Name != "pod" {
		t.Fatalf("second field = %q, want pod", m.form.cur().field.Name)
	}
	_ = m.form.candidatePreview()
	if sawNS != "kube-system" {
		t.Errorf("preview saw ns=%q, want kube-system", sawNS)
	}
}

func TestNoPreviewWhenUnset(t *testing.T) {
	be := newFakeBackend()
	be.data["file"] = fd("a.go")
	// sampleEntries()[0] file field has no Preview.
	m := New(sampleEntries(), be)
	m = send(m, key(tea.KeyEnter))
	if got := m.form.candidatePreview(); got != "" {
		t.Errorf("candidatePreview without a Preview command = %q, want empty", got)
	}
}
