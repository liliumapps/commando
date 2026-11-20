package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
)

func countSource(entries []entry.Entry, s entry.Source) int {
	n := 0
	for _, e := range entries {
		if e.Source == s {
			n++
		}
	}
	return n
}

func TestHistoryCappedWhenIdleAllWhenSearching(t *testing.T) {
	var entries []entry.Entry
	entries = append(entries, entry.Entry{Desc: "deploy cheat", Template: "deploy <x>", Source: entry.SourceCheat})
	// 20 history commands, all containing "deploy".
	for i := 0; i < 20; i++ {
		entries = append(entries, entry.Entry{
			Desc:     "deploy variant " + string(rune('a'+i)),
			Template: "deploy variant " + string(rune('a'+i)),
			Source:   entry.SourceHistory,
		})
	}

	m := NewWithOptions(entries, newFakeBackend(), Options{HistoryListLimit: 10})

	// Idle (no query): history capped to 10.
	if got := countSource(m.filtered, entry.SourceHistory); got != 10 {
		t.Errorf("idle history count = %d, want 10 (capped)", got)
	}

	// Typing a query reveals ALL matching history (20), plus the cheat.
	m = send(m, runes("deploy"))
	if got := countSource(m.filtered, entry.SourceHistory); got != 20 {
		t.Errorf("searching history count = %d, want 20 (uncapped)", got)
	}

	// Clearing the query re-applies the cap.
	m = send(m, key(tea.KeyBackspace))
	m = send(m, key(tea.KeyBackspace))
	m = send(m, key(tea.KeyBackspace))
	m = send(m, key(tea.KeyBackspace))
	m = send(m, key(tea.KeyBackspace))
	m = send(m, key(tea.KeyBackspace))
	if m.query != "" {
		t.Fatalf("query should be empty, got %q", m.query)
	}
	if got := countSource(m.filtered, entry.SourceHistory); got != 10 {
		t.Errorf("after clearing query, history count = %d, want 10 (capped again)", got)
	}
}

func TestHistoryCapDefaultUnlimitedWithoutOption(t *testing.T) {
	var entries []entry.Entry
	for i := 0; i < 15; i++ {
		entries = append(entries, entry.Entry{Desc: string(rune('a' + i)), Template: string(rune('a' + i)), Source: entry.SourceHistory})
	}
	// New (no options) => HistoryListLimit 0 => no cap.
	m := New(entries, newFakeBackend())
	if got := countSource(m.filtered, entry.SourceHistory); got != 15 {
		t.Errorf("without a limit option, history count = %d, want 15 (uncapped)", got)
	}
}
