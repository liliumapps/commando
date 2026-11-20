package rank

import (
	"sort"
	"strings"
)

// Candidate is a suggested value for a parameter. It carries two projections of
// the same choice: Display is what the user sees and fuzzy-searches; Value is
// what gets inserted into the command, remembered, and substituted into
// dependent Src commands. For a plain candidate the two are identical; an
// extraction spec (label/value templates) can make them differ, e.g. Display
// "chrome (1234)" with Value "1234".
//
// Remembered (previously used) values carry a non-zero LastUsed; values that
// come only from the parameter's `src` command carry LastUsed 0.
type Candidate struct {
	// Display is shown in the list and used as the fuzzy-match key.
	Display string
	// Value is inserted, remembered, and fed to dependent Src commands.
	Value string
	// LastUsed is the unix timestamp this value was last chosen, or 0 if it was
	// never chosen (e.g. a fresh value from the src command).
	LastUsed int64
	// UseCount is how many times this value was chosen (0 if never).
	UseCount int
	// srcOrder preserves the original ordering of never-used src values so their
	// relative order (as produced by the src command) is kept.
	srcOrder int
}

// Candidates returns candidates ordered for display, matched by the query
// against each candidate's Display text.
//
// With an empty query, previously-used values come first, most-recently-used
// first; then never-used values from the src command, in their original order.
//
// With a query, match quality is primary (exact, prefix, word-prefix, contains,
// then fuzzy); within equal scores, used values come first (most recent first),
// then never-used src values in original order. Non-matching candidates are
// dropped.
//
// Returns the ranked candidates (Display + Value preserved) so callers can show
// Display while committing Value.
func Candidates(cands []Candidate, query string) []Candidate {
	type scored struct {
		c     Candidate
		score int
	}
	terms := strings.Fields(strings.ToLower(query))
	scoredCands := make([]scored, 0, len(cands))
	for _, c := range cands {
		score := termsTextScore(terms, c.Display)
		if query != "" && score == noMatchScore {
			continue
		}
		scoredCands = append(scoredCands, scored{c: c, score: score})
	}

	sort.SliceStable(scoredCands, func(i, j int) bool {
		a, b := scoredCands[i], scoredCands[j]

		if query != "" && a.score != b.score {
			return a.score < b.score
		}

		aUsed, bUsed := a.c.LastUsed > 0, b.c.LastUsed > 0
		if aUsed != bUsed {
			return aUsed // used values before never-used ones
		}
		if aUsed { // both used: most recent first, then most frequent
			if a.c.LastUsed != b.c.LastUsed {
				return a.c.LastUsed > b.c.LastUsed
			}
			if a.c.UseCount != b.c.UseCount {
				return a.c.UseCount > b.c.UseCount
			}
		}
		// Both never used: keep original src order.
		return a.c.srcOrder < b.c.srcOrder
	})

	out := make([]Candidate, len(scoredCands))
	for i, s := range scoredCands {
		out[i] = s.c
	}
	return out
}

// BuildCandidates merges remembered values (with their usage data) and fresh
// src candidates into a single de-duplicated list, keyed by Value. Remembered
// values take precedence when a value appears in both, but adopt the src
// candidate's Display so the readable label is shown even for a recalled value
// present in the current list. srcOrder is assigned in input order so never-used
// src values keep their sequence.
func BuildCandidates(remembered, srcCands []Candidate) []Candidate {
	seen := make(map[string]int, len(remembered)+len(srcCands))
	out := make([]Candidate, 0, len(remembered)+len(srcCands))

	order := 0
	for _, c := range remembered {
		if c.Value == "" {
			continue
		}
		if _, ok := seen[c.Value]; ok {
			continue
		}
		// A remembered value has no source line, so its Display is just the
		// value unless a matching src candidate later supplies a richer label.
		if c.Display == "" {
			c.Display = c.Value
		}
		c.srcOrder = order
		order++
		seen[c.Value] = len(out)
		out = append(out, c)
	}
	for _, sc := range srcCands {
		if sc.Value == "" {
			continue
		}
		if idx, ok := seen[sc.Value]; ok {
			// Already present as a remembered value: keep its usage data but
			// adopt the src candidate's readable Display.
			if sc.Display != "" {
				out[idx].Display = sc.Display
			}
			continue
		}
		if sc.Display == "" {
			sc.Display = sc.Value
		}
		sc.srcOrder = order
		seen[sc.Value] = len(out)
		out = append(out, sc)
		order++
	}
	return out
}
