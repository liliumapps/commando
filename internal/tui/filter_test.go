package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
)

func TestFilterExactMatchBeatsSourcePriority(t *testing.T) {
	// Relevance is stronger than source priority: the exact history title beats
	// a longer cheat title that only starts with the query.
	entries := []entry.Entry{
		{Desc: "deploy", Template: "deploy", Source: entry.SourceHistory},
		{Desc: "deploy service to prod", Template: "deploy <svc>", Source: entry.SourceCheat},
	}
	got := filterEntries(entries, "deploy")
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(got))
	}
	if got[0].Source != entry.SourceHistory {
		t.Errorf("exact history match should rank first, got %v", got[0].Source)
	}
}

func TestFilterEmptyQueryMostRecentFirst(t *testing.T) {
	entries := []entry.Entry{
		{Desc: "older", Template: "b", Source: entry.SourceCheat, LastUsed: 100},
		{Desc: "newer", Template: "a", Source: entry.SourceCheat, LastUsed: 500},
	}
	got := filterEntries(entries, "")
	if got[0].Desc != "newer" {
		t.Errorf("empty query should put most-recently-used first, got %q", got[0].Desc)
	}
}

func TestCapHistory(t *testing.T) {
	entries := []entry.Entry{
		{Desc: "cheat", Source: entry.SourceCheat},
		{Desc: "h1", Source: entry.SourceHistory},
		{Desc: "h2", Source: entry.SourceHistory},
		{Desc: "h3", Source: entry.SourceHistory},
		{Desc: "bm", Source: entry.SourceBookmark},
	}
	got := capHistory(entries, 2)
	// All non-history kept; only 2 history kept, in order.
	var hist []string
	var curated int
	for _, e := range got {
		if e.Source == entry.SourceHistory {
			hist = append(hist, e.Desc)
		} else {
			curated++
		}
	}
	if curated != 2 {
		t.Errorf("curated entries dropped: kept %d, want 2", curated)
	}
	if len(hist) != 2 || hist[0] != "h1" || hist[1] != "h2" {
		t.Errorf("history cap wrong: %v, want [h1 h2]", hist)
	}
}

func scopeSampleEntries() []entry.Entry {
	return []entry.Entry{
		{Desc: "cheat one", Template: "make one", Source: entry.SourceCheat},
		{Desc: "project two", Template: "npm two", Source: entry.SourceProject},
		{Desc: "history three", Template: "git three", Source: entry.SourceHistory},
		{Desc: "saved four", Template: "deploy four", Source: entry.SourceBookmark},
	}
}

func TestTabCyclesScopeViaQueryPrefix(t *testing.T) {
	m := New(scopeSampleEntries(), newFakeBackend())

	// Tab rewrites the query prefix: all → :c → :p → :h → :b → all.
	m = send(m, key(tea.KeyTab))
	if m.query != ":c " {
		t.Fatalf("first Tab query = %q, want %q", m.query, ":c ")
	}
	if len(m.filtered) != 1 || m.filtered[0].Source != entry.SourceCheat {
		t.Errorf(":c filtered %d entries, want 1 cheat", len(m.filtered))
	}

	m = send(m, key(tea.KeyTab))
	if m.query != ":p " || len(m.filtered) != 1 || m.filtered[0].Source != entry.SourceProject {
		t.Errorf("second Tab: query=%q filtered=%d, want :p with 1 project", m.query, len(m.filtered))
	}

	m = send(m, key(tea.KeyTab))
	if m.query != ":h " || len(m.filtered) != 1 || m.filtered[0].Source != entry.SourceHistory {
		t.Errorf("third Tab: query=%q filtered=%d, want :h with 1 history", m.query, len(m.filtered))
	}

	m = send(m, key(tea.KeyTab))
	if m.query != ":b " || len(m.filtered) != 1 || m.filtered[0].Source != entry.SourceBookmark {
		t.Errorf("fourth Tab: query=%q filtered=%d, want :b with 1 bookmark", m.query, len(m.filtered))
	}

	m = send(m, key(tea.KeyTab))
	if m.query != "" || len(m.filtered) != 4 {
		t.Errorf("fifth Tab should wrap to all: query=%q filtered=%d, want \"\"/4", m.query, len(m.filtered))
	}
}

func TestTabPreservesSearchText(t *testing.T) {
	m := New(scopeSampleEntries(), newFakeBackend())
	m = send(m, runes("t")) // matches project "two"/"project two", history "three", etc.
	m = send(m, key(tea.KeyTab))
	if m.query != ":c t" {
		t.Fatalf("Tab should preserve the search text: query=%q, want %q", m.query, ":c t")
	}
	// Scope + search compose: only cheat entries matching "t".
	for _, e := range m.filtered {
		if e.Source != entry.SourceCheat {
			t.Errorf(":c t leaked a %v entry: %q", e.Source, e.Desc)
		}
	}
}

func TestShiftTabCyclesBackward(t *testing.T) {
	m := New(scopeSampleEntries(), newFakeBackend())
	m = send(m, key(tea.KeyShiftTab)) // all → :b (reverse)
	if m.query != ":b " {
		t.Errorf("Shift+Tab from all = %q, want %q", m.query, ":b ")
	}
}

func TestScopeExplicitBypassesHistoryCap(t *testing.T) {
	// With no scope and no search, the history cap applies; an explicit ":h"
	// scope shows all history regardless of the cap.
	entries := []entry.Entry{
		{Desc: "h1", Source: entry.SourceHistory},
		{Desc: "h2", Source: entry.SourceHistory},
		{Desc: "h3", Source: entry.SourceHistory},
	}
	m := NewWithOptions(entries, newFakeBackend(), Options{HistoryListLimit: 1})
	if len(m.filtered) != 1 {
		t.Fatalf("idle view should cap history to 1, got %d", len(m.filtered))
	}
	m = send(m, key(tea.KeyTab)) // :c — no history
	m = send(m, key(tea.KeyTab)) // :p — no history
	m = send(m, key(tea.KeyTab)) // :h — all history, cap bypassed
	if m.query != ":h " || len(m.filtered) != 3 {
		t.Errorf(":h should show all history uncapped: query=%q filtered=%d, want :h/3", m.query, len(m.filtered))
	}
}

func TestCapHistoryUnlimited(t *testing.T) {
	entries := []entry.Entry{
		{Desc: "h1", Source: entry.SourceHistory},
		{Desc: "h2", Source: entry.SourceHistory},
	}
	if got := capHistory(entries, 0); len(got) != 2 {
		t.Errorf("limit 0 should be unlimited, got %d", len(got))
	}
	if got := capHistory(entries, -1); len(got) != 2 {
		t.Errorf("negative limit should be unlimited, got %d", len(got))
	}
}
