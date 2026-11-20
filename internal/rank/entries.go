package rank

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/liliumapps/commando/internal/entry"
)

// sourcePriority orders entry sources when nothing else separates two entries.
// Curated commands rank above raw shell history; bookmarks (deliberately saved)
// rank first.
func sourcePriority(s entry.Source) int {
	switch s {
	case entry.SourceBookmark:
		return 0
	case entry.SourceCheat:
		return 1
	case entry.SourceProject:
		return 2
	case entry.SourceHistory:
		return 3
	default:
		return 4
	}
}

// Entries returns entries ordered for display.
//
// The query may carry a leading ":x" scope prefix (":c" cheats, ":p" project,
// ":h" history, ":b" bookmarks); it is parsed off first (see ParseScope), entries
// outside the scope are dropped, and the remaining text is used for matching. So
// ":h git" keeps only history entries and ranks them by "git". A bare ":h" keeps
// all history with no further filtering.
//
// With no search text the primary key is frecency (see Frecency: use count
// weighted by the age of the last use), then recency of use; never-used
// entries follow, ordered by source priority (bookmark > cheat > project >
// history), the provider's own order, and title.
//
// With search text, weighted match quality is the primary key: exact, prefix,
// word-prefix, contains, then fuzzy, with title matches preferred over tags,
// command text, and description. Close matches (scores in the same band of
// ten) are ordered by frecency first, then by exact score, then as above.
// Entries that do not match at all are dropped.
//
// Search text is matched against each entry's lowercased fields, precomputed
// once by entry.PrepareSearch when available, so a keystroke over a large
// history costs no per-entry lowercasing or splitting.
//
// The input slice is not mutated; a new sorted slice is returned.
func Entries(entries []entry.Entry, query string) []entry.Entry {
	scope, search := ParseScope(query)
	terms := strings.Fields(strings.ToLower(search))
	now := nowUnix()

	// scored packs every sort key into a few integers so the comparator does
	// four word compares instead of chasing the (large) entries. Keys are
	// arranged so that smaller sorts first:
	//
	//	k1: match band (score/10) | inverted frecency | score within band
	//	k2: inverted last-used time (most recent first)
	//	k3: inverted use count | source priority
	//	k4: inverted provider order (newest first)
	type scored struct {
		k1, k2, k3, k4 uint64
		idx            int32
	}

	scoredEntries := make([]scored, 0, len(entries))
	for i := range entries {
		e := &entries[i]
		if !scope.Includes(e.Source) {
			continue // outside the requested scope
		}
		score := 0
		if len(terms) > 0 {
			score = entryTermsScore(terms, e)
			if score == noMatchScore {
				continue // filter out non-matches when searching
			}
		}
		count := e.UseCount
		if count <= 0 && e.LastUsed > 0 {
			count = 1
		}
		// Frecency weights are multiples of 0.25, so ×4 is an exact integer.
		frec4 := uint64(min(Frecency(count, e.LastUsed, now)*4, math.MaxUint32))
		band := uint64(min(closeMatchBucket(score), 1<<27-1))
		scoredEntries = append(scoredEntries, scored{
			k1:  band<<36 | (math.MaxUint32-frec4)<<4 | uint64(score%10),
			k2:  ^uint64(max(e.LastUsed, 0)),
			k3:  uint64(math.MaxUint32-uint32(min(max(e.UseCount, 0), math.MaxUint32)))<<8 | uint64(sourcePriority(e.Source)),
			k4:  ^uint64(max(e.ProviderOrder, 0)),
			idx: int32(i),
		})
	}

	slices.SortFunc(scoredEntries, func(a, b scored) int {
		// Match quality band, then frecency (hot commands first, also among
		// close matches), then exact score.
		if a.k1 != b.k1 {
			return cmp.Compare(a.k1, b.k1)
		}
		// Recency of use: most recently used first; never used last.
		if a.k2 != b.k2 {
			return cmp.Compare(a.k2, b.k2)
		}
		// More uses first, then source priority (bookmark > cheat > project >
		// history).
		if a.k3 != b.k3 {
			return cmp.Compare(a.k3, b.k3)
		}
		// Preserve a provider's own ordering when all stronger signals tie.
		// History sets this to the command's position in time order, so
		// fresh, never-launched history remains newest-first.
		if a.k4 != b.k4 {
			return cmp.Compare(a.k4, b.k4)
		}
		// Finally, title/description then original order for determinism.
		ea, eb := &entries[a.idx], &entries[b.idx]
		if c := strings.Compare(ea.TitleText(), eb.TitleText()); c != 0 {
			return c
		}
		if c := strings.Compare(ea.Desc, eb.Desc); c != 0 {
			return c
		}
		return cmp.Compare(a.idx, b.idx)
	})

	out := make([]entry.Entry, len(scoredEntries))
	for i, s := range scoredEntries {
		out[i] = entries[s.idx]
	}
	return out
}

// field weights: title matches beat tags, then command text, then description.
const (
	weightTitle    = 0
	weightTags     = 6
	weightTemplate = 12
	weightDesc     = 16
)

// entryTermsScore sums, over already-lowercased terms, the best weighted field
// score. A term that matches no field makes the entry a non-match.
func entryTermsScore(terms []string, e *entry.Entry) int {
	f := e.LowerSearchFields()
	total := 0
	for _, term := range terms {
		best := noMatchScore
		if s := lowerTextScore(term, f.Title); s != noMatchScore {
			best = s + weightTitle
		}
		if best > weightTags {
			if s := lowerTextScore(term, f.Tags); s != noMatchScore {
				best = min(best, s+weightTags)
			}
		}
		if best > weightTemplate {
			if s := lowerTextScore(term, f.Template); s != noMatchScore {
				best = min(best, s+weightTemplate)
			}
		}
		if best > weightDesc {
			if s := lowerTextScore(term, f.Desc); s != noMatchScore {
				best = min(best, s+weightDesc)
			}
		}
		if best == noMatchScore {
			return noMatchScore
		}
		total += best
	}
	return total
}
