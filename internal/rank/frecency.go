package rank

import "time"

// Frecency scores how "hot" something is from its use count and the age of its
// last use, zoxide-style: the count is weighted by how recently it was used.
//
//	used within the last hour  count × 4
//	within the last day        count × 2
//	within the last week       count × 0.5
//	older                      count × 0.25
//
// Never-used items (count 0 or lastUsed 0) score 0. A lastUsed in the future
// (clock skew) counts as "just now". now and lastUsed are unix seconds.
func Frecency(count int, lastUsed, now int64) float64 {
	if count <= 0 || lastUsed <= 0 {
		return 0
	}
	age := now - lastUsed
	var w float64
	switch {
	case age < int64(time.Hour/time.Second):
		w = 4
	case age < int64(24*time.Hour/time.Second):
		w = 2
	case age < int64(7*24*time.Hour/time.Second):
		w = 0.5
	default:
		w = 0.25
	}
	return float64(count) * w
}

// nowUnix is the ranking clock; overridable in tests.
var nowUnix = func() int64 { return time.Now().Unix() }

// closeMatchBucket groups query scores that are "close" so frecency can order
// them: scores are spaced 10 apart per tier (exact 0, prefix 10, word-prefix
// 20, contains 30+, fuzzy 100+) plus small field weights, so dividing by 10
// keeps distinct tiers apart while letting, e.g., a title prefix and a tag
// prefix compete on frecency.
func closeMatchBucket(score int) int { return score / 10 }
