// Package rank contains commando's ordering logic, factored out so it can be
// tested and tuned in isolation from the UI and storage. It ranks two things:
//
//   - Entries: the main command list (Entries).
//   - Candidates: a parameter's suggested values (Candidates).
//
// It imports only the entry package, so it has no dependency on the TUI, the
// providers, or persistence.
package rank

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const noMatchScore = int(^uint(0) >> 1)

// Tier classifies how well a query matches a string. Lower values are better
// matches and sort first.
type Tier int

const (
	// TierStartsWith: the string begins with the query.
	TierStartsWith Tier = iota
	// TierContains: the string contains the query (but does not start with it).
	TierContains
	// TierFuzzy: the query's characters appear in order (a subsequence) but not
	// contiguously.
	TierFuzzy
	// TierNone: no match.
	TierNone
)

// matchTier returns the tier for a query treated as a single term against one
// haystack. Matching is case-insensitive. An empty query matches everything at
// TierStartsWith.
func matchTier(query, haystack string) Tier {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return TierStartsWith
	}
	return termTier(q, strings.ToLower(haystack))
}

// termTier scores a single, already-lowercased term against an already-lowercased
// haystack. It is the shared core of both single-haystack matching (matchTier)
// and multi-term matching (matchQuery).
func termTier(term, haystack string) Tier {
	switch {
	case strings.HasPrefix(haystack, term):
		return TierStartsWith
	case strings.Contains(haystack, term):
		return TierContains
	case isSubsequence(term, haystack):
		return TierFuzzy
	default:
		return TierNone
	}
}

// matchQuery scores a whitespace-tokenised query against several haystacks as an
// AND of its terms: every term must match at least one haystack, but different
// terms may match different haystacks. This lets a query mix words from separate
// columns — e.g. a tag ("deploy") plus a description word ("cluster") — and type
// them in any order, instead of requiring the whole string to appear verbatim in
// one column.
//
// The returned tier is the weakest (highest) per-term tier, so an entry ranks
// only as well as its loosest-matching term: an all-starts-with query outranks
// one where a term only matches mid-word. A term that matches nothing makes the
// whole query a non-match (TierNone). An empty query matches everything at
// TierStartsWith.
func matchQuery(query string, haystacks ...string) Tier {
	if strings.TrimSpace(query) == "" {
		return TierStartsWith
	}
	// Lowercase each haystack once, then test every term against all of them.
	lowered := make([]string, len(haystacks))
	for i, h := range haystacks {
		lowered[i] = strings.ToLower(h)
	}
	worst := TierStartsWith
	for _, term := range strings.Fields(strings.ToLower(query)) {
		best := TierNone
		for _, h := range lowered {
			if t := termTier(term, h); t < best {
				best = t
			}
		}
		if best == TierNone {
			return TierNone // this term matched no haystack → not a match
		}
		if best > worst {
			worst = best
		}
	}
	return worst
}

// isSubsequence reports whether every rune of needle appears in haystack in
// order (the definition of a fuzzy match). Both are expected pre-lowercased.
func isSubsequence(needle, haystack string) bool {
	if needle == "" {
		return true
	}
	ni := 0
	nr := []rune(needle)
	for _, hr := range haystack {
		if hr == nr[ni] {
			ni++
			if ni == len(nr) {
				return true
			}
		}
	}
	return false
}

// lowerTextScore is textScore for an already-lowercased, non-empty term and
// haystack. It does not allocate.
func lowerTextScore(term, haystack string) int {
	if len(term) > len(haystack) {
		return noMatchScore
	}
	if haystack == term {
		return 0
	}
	if strings.HasPrefix(haystack, term) {
		return 10
	}
	first := strings.Index(haystack, term)
	if first < 0 {
		span, ok := subsequenceSpan(term, haystack)
		if !ok {
			return noMatchScore
		}
		return 100 + span - utf8.RuneCountInString(term)
	}
	// Word-prefix: any occurrence that starts right after whitespace.
	for i := first; ; {
		if r, _ := utf8.DecodeLastRuneInString(haystack[:i]); unicode.IsSpace(r) {
			return 20
		}
		next := strings.Index(haystack[i+1:], term)
		if next < 0 {
			break
		}
		i += 1 + next
	}
	return 30 + min(first, 20)
}

// termsTextScore sums lowerTextScore over already-lowercased terms against
// haystack (lowercased once here). An empty term list scores 0.
func termsTextScore(terms []string, haystack string) int {
	if len(terms) == 0 {
		return 0
	}
	lower := strings.ToLower(haystack)
	total := 0
	for _, term := range terms {
		score := lowerTextScore(term, lower)
		if score == noMatchScore {
			return noMatchScore
		}
		total += score
	}
	return total
}

// subsequenceSpan returns the rune length of the greedy in-order match of
// needle in haystack (from the first matched rune to the last). It does not
// allocate.
func subsequenceSpan(needle, haystack string) (int, bool) {
	if needle == "" {
		return 0, true
	}
	if isASCII(needle) {
		// Fast path: jump between needle bytes with IndexByte. ASCII bytes
		// never occur inside multi-byte UTF-8 sequences, so this matches the
		// rune-wise greedy scan exactly.
		pos, first := 0, -1
		for j := 0; j < len(needle); j++ {
			k := strings.IndexByte(haystack[pos:], needle[j])
			if k < 0 {
				return 0, false
			}
			if j == 0 {
				first = pos + k
			}
			pos += k + 1
		}
		return utf8.RuneCountInString(haystack[first:pos]), true
	}
	ni := 0
	nr, nsize := utf8.DecodeRuneInString(needle)
	first, pos := -1, 0
	for _, r := range haystack {
		if r == nr {
			if first < 0 {
				first = pos
			}
			ni += nsize
			if ni >= len(needle) {
				return pos - first + 1, true
			}
			nr, nsize = utf8.DecodeRuneInString(needle[ni:])
		}
		pos++
	}
	return 0, false
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
