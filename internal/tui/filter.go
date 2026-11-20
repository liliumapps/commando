package tui

import (
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/rank"
)

// capHistory returns entries with at most limit history entries retained, in
// order. Non-history entries are always kept. A limit <= 0 means no cap. It is
// used for the no-query list so a long shell history does not drown out curated
// commands; searching bypasses this to surface every match.
func capHistory(entries []entry.Entry, limit int) []entry.Entry {
	if limit <= 0 {
		return entries
	}
	out := make([]entry.Entry, 0, len(entries))
	histSeen := 0
	for _, e := range entries {
		if e.Source == entry.SourceHistory {
			if histSeen >= limit {
				continue
			}
			histSeen++
		}
		out = append(out, e)
	}
	return out
}

// filterEntries ranks entries for the given query via the rank package: with an
// empty query, most-recently-used first then unused by source; with a query,
// by match tier (starts-with, contains, fuzzy) then recency. Non-matches are
// dropped when searching.
func filterEntries(entries []entry.Entry, query string) []entry.Entry {
	return rank.Entries(entries, query)
}
