package app

import (
	"fmt"
	"sync"

	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/provider"
	"github.com/liliumapps/commando/internal/state"
)

// Diagnostic describes a non-fatal provider failure. Healthy providers still
// contribute entries, while callers can surface the degraded source.
type Diagnostic struct {
	Source  string
	Message string
}

// aggregate collects entries from all providers (bookmarks, cheats, project,
// history), de-duplicates them, and annotates each with its usage data
// (last-used time + count) so the ranker can order them. Ordering itself is
// deferred to the rank package, applied in the TUI per query.
//
// Providers run concurrently (history parsing, project detection, and user
// listers are independent), but results are combined in provider order so the
// output is deterministic.
func aggregate(provs []provider.Provider, pctx provider.Context, st *state.State) ([]entry.Entry, []Diagnostic) {
	type result struct {
		entries []entry.Entry
		err     error
	}
	results := make([]result, len(provs))
	var wg sync.WaitGroup
	for i, p := range provs {
		wg.Add(1)
		go func(i int, p provider.Provider) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[i] = result{err: fmt.Errorf("provider panicked: %v", r)}
				}
			}()
			es, err := p.Entries(pctx)
			results[i] = result{entries: es, err: err}
		}(i, p)
	}
	wg.Wait()

	var all []entry.Entry
	var diagnostics []Diagnostic
	for i, p := range provs {
		all = append(all, results[i].entries...)
		if err := results[i].err; err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Source:  p.Name(),
				Message: fmt.Sprintf("%v", err),
			})
		}
	}

	deduped := dedupe(all)

	for i := range deduped {
		if st != nil {
			last, count := st.EntryUsage(deduped[i].ID())
			deduped[i].LastUsed = last
			deduped[i].UseCount = count
		}
		// Lowercase the searchable text once, not on every keystroke.
		deduped[i].PrepareSearch()
	}
	if st != nil {
		for _, note := range st.Recovered() {
			diagnostics = append(diagnostics, Diagnostic{Source: "state", Message: note})
		}
	}
	return deduped, diagnostics
}

// dedupe removes duplicate entries so curated commands always win over raw
// history. Non-history entries (bookmarks, cheats, project) are kept first,
// deduped by (template, title); a history entry is then added only if its
// command is not already offered by a non-history entry, deduped among history.
func dedupe(entries []entry.Entry) []entry.Entry {
	type descKey struct{ tmpl, title string }
	var out []entry.Entry

	seenNonHist := make(map[descKey]bool)
	offeredTmpl := make(map[string]bool)
	for _, e := range entries {
		if e.Source == entry.SourceHistory {
			continue
		}
		k := descKey{e.Template, e.TitleText()}
		if seenNonHist[k] {
			continue
		}
		seenNonHist[k] = true
		offeredTmpl[e.Template] = true
		out = append(out, e)
	}

	seenHist := make(map[string]bool)
	for _, e := range entries {
		if e.Source != entry.SourceHistory {
			continue
		}
		if offeredTmpl[e.Template] || seenHist[e.Template] {
			continue
		}
		seenHist[e.Template] = true
		out = append(out, e)
	}
	return out
}
