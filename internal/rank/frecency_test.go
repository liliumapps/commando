package rank

import (
	"testing"

	"github.com/liliumapps/commando/internal/entry"
)

func withNow(t *testing.T, now int64) {
	t.Helper()
	prev := nowUnix
	nowUnix = func() int64 { return now }
	t.Cleanup(func() { nowUnix = prev })
}

func TestFrecencyWeights(t *testing.T) {
	const now = 1_700_000_000
	cases := []struct {
		count int
		age   int64
		want  float64
	}{
		{3, 60, 12},        // < 1h: ×4
		{3, 2 * 3600, 6},   // < 1d: ×2
		{4, 3 * 86400, 2},  // < 1w: ×0.5
		{4, 30 * 86400, 1}, // older: ×0.25
		{0, 60, 0},         // never used
		{5, -100, 20},      // clock skew counts as now
	}
	for _, c := range cases {
		if got := Frecency(c.count, now-c.age, now); got != c.want {
			t.Errorf("Frecency(%d, age %d) = %v, want %v", c.count, c.age, got, c.want)
		}
	}
}

func TestEntriesNoQueryUsesFrecency(t *testing.T) {
	const now = 10_000_000
	withNow(t, now)
	entries := []entry.Entry{
		// Used once a minute ago: 1×4 = 4.
		{Title: "once-recent", Template: "a", Source: entry.SourceCheat, LastUsed: now - 60, UseCount: 1},
		// Used 20 times, last two days ago: 20×0.5 = 10.
		{Title: "habit", Template: "b", Source: entry.SourceCheat, LastUsed: now - 2*86400, UseCount: 20},
	}
	got := Entries(entries, "")
	if got[0].Title != "habit" {
		t.Errorf("frecent entry should lead, got %q first", got[0].Title)
	}
}

func TestEntriesFrecencyBreaksCloseMatches(t *testing.T) {
	const now = 10_000_000
	withNow(t, now)
	entries := []entry.Entry{
		// Title prefix (score 10), never used.
		{Title: "deploy staging", Template: "x", Source: entry.SourceCheat},
		// Tag prefix (score 16): a close match, but hot.
		{Title: "rollout", Tags: []string{"deployment"}, Template: "y", Source: entry.SourceCheat, LastUsed: now - 10, UseCount: 5},
		// Fuzzy only: never beats the prefix band regardless of frecency.
		{Title: "dxexpxlxoxy", Template: "z", Source: entry.SourceCheat, LastUsed: now - 10, UseCount: 99},
	}
	got := Entries(entries, "deploy")
	if len(got) != 3 || got[0].Title != "rollout" || got[1].Title != "deploy staging" || got[2].Title != "dxexpxlxoxy" {
		var titles []string
		for _, e := range got {
			titles = append(titles, e.Title)
		}
		t.Errorf("order = %v", titles)
	}
	// An exact title match is a different band and still wins over frecency.
	entries = append(entries, entry.Entry{Title: "deploy", Template: "w", Source: entry.SourceCheat})
	if got := Entries(entries, "deploy"); got[0].Title != "deploy" {
		t.Errorf("exact match should lead, got %q", got[0].Title)
	}
}

func TestEntriesUsesPreparedFields(t *testing.T) {
	e := entry.Entry{Title: "Build Docs", Template: "make docs", Tags: []string{"Make"}, Source: entry.SourceProject}
	e.PrepareSearch()
	if got := Entries([]entry.Entry{e}, "DOCS make"); len(got) != 1 {
		t.Fatal("prepared entry should match case-insensitively across fields")
	}
}
