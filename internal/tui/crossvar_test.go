package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
)

// crossVarEntry has a <namespace> field and a <pod> field whose Src
// references <namespace>, mirroring a kubectl cheat that lists pods per
// namespace.
func crossVarEntry() entry.Entry {
	return entry.Entry{
		Desc:     "tail logs of a pod in a namespace",
		Template: "kubectl logs <pod> --tail 100",
		Vars: []entry.VarSpec{
			{Name: "pod", Src: "kubectl get pods -n <namespace> -o name"},
			{Name: "namespace", Src: "kubectl get namespaces -o name"},
		},
		Source: entry.SourceCheat,
	}
}

func TestCrossVariableSrcReceivesResolvedValue(t *testing.T) {
	be := newFakeBackend()
	be.data["namespace"] = fd("kube-system", "default")
	// pod candidates depend on the chosen namespace.
	be.dynamic["pod"] = func(resolved map[string]string) FieldData {
		namespace := resolved["namespace"]
		if namespace == "" {
			return FieldData{} // nothing until a namespace is chosen
		}
		d := fd("pod-in-" + namespace)
		d.Prefill = "pod-in-" + namespace
		return d
	}

	m := New([]entry.Entry{crossVarEntry()}, be)
	m = send(m, key(tea.KeyEnter)) // into form

	// The plan expands <namespace> (referenced only by pod's Src) into a
	// field and orders dependencies first, so <namespace> is resolved before
	// <pod>.
	if m.form.cur().field.Name != "namespace" {
		t.Fatalf("first field = %q, want namespace (dependency ordered first)", m.form.cur().field.Name)
	}
	// Enter commits the highlighted namespace candidate (kube-system) and, since
	// pod is still an unmet required param, advances focus to it.
	m = send(m, key(tea.KeyEnter))

	// pod's Src must have seen namespace=kube-system and produced its
	// dependent candidates/prefill.
	if m.form.cur().field.Name != "pod" {
		t.Fatalf("second field = %q, want pod", m.form.cur().field.Name)
	}
	got := be.lastResolved["pod"]
	if got["namespace"] != "kube-system" {
		t.Errorf("pod Src saw namespace=%q, want kube-system", got["namespace"])
	}
	if fs := m.form.cur(); len(fs.data.Candidates) != 1 || fs.data.Candidates[0].Value != "pod-in-kube-system" {
		t.Errorf("pod candidates = %v, want [pod-in-kube-system]", fs.data.Candidates)
	}
	// Since the user hasn't touched pod, it adopts the refreshed prefill.
	if m.form.cur().input != "pod-in-kube-system" {
		t.Errorf("pod prefill = %q, want pod-in-kube-system", m.form.cur().input)
	}
}

func TestCrossVariableRefetchOnDependencyChange(t *testing.T) {
	be := newFakeBackend()
	be.data["namespace"] = fd("ns1", "ns2")
	fetches := 0
	be.dynamic["pod"] = func(resolved map[string]string) FieldData {
		fetches++
		return fd("pod-" + resolved["namespace"])
	}

	m := New([]entry.Entry{crossVarEntry()}, be)
	m = send(m, key(tea.KeyEnter)) // form; param 0 = namespace (dependency)
	if m.form.cur().field.Name != "namespace" {
		t.Fatalf("first param = %q, want namespace", m.form.cur().field.Name)
	}

	// Select ns1 (the bottom candidate, index 0): Up then Down leaves the cursor
	// at 0 but marks the field navigated, so moving Ctrl-N commits ns1.
	m = send(m, key(tea.KeyUp))    // -> index 1 (ns2), navigated
	m = send(m, key(tea.KeyDown))  // -> index 0 (ns1), still navigated
	m = send(m, key(tea.KeyCtrlN)) // commit namespace=ns1, move to pod -> fetch
	firstFetches := fetches
	if firstFetches == 0 {
		t.Fatal("pod should have fetched on entry")
	}
	if m.form.cur().data.Candidates[0].Value != "pod-ns1" {
		t.Errorf("pod candidates = %v, want [pod-ns1]", m.form.cur().data.Candidates)
	}

	// Go back to namespace, change it to ns2, return: pod must refetch.
	m = send(m, key(tea.KeyCtrlP)) // back to namespace
	m = send(m, key(tea.KeyUp))    // nothing highlighted yet: first Up highlights index 0 (ns1)
	m = send(m, key(tea.KeyUp))    // reversed candidates: Up -> index 1 (ns2)
	m = send(m, key(tea.KeyCtrlN)) // commit namespace=ns2, to pod -> refetch
	if fetches <= firstFetches {
		t.Errorf("pod should refetch after namespace changed (%d -> %d)", firstFetches, fetches)
	}
	if m.form.cur().data.Candidates[0].Value != "pod-ns2" {
		t.Errorf("pod candidates = %v, want [pod-ns2] after namespace change", m.form.cur().data.Candidates)
	}
	steady := fetches

	// Return to pod without changing namespace: no refetch.
	m = send(m, key(tea.KeyCtrlP)) // back to namespace
	send(m, key(tea.KeyCtrlN))     // to pod, namespace unchanged
	if fetches != steady {
		t.Errorf("pod refetched despite unchanged namespace (%d -> %d)", steady, fetches)
	}
}
