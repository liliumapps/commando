package rank

import (
	"strings"

	"github.com/liliumapps/commando/internal/entry"
)

// Scope narrows the entry list to a subset of sources. It is driven entirely by
// a leading `:x` token in the query text (e.g. ":h docker"), so the query string
// remains the single source of truth: the TUI never tracks a hidden scope, it
// just rewrites the prefix (see WithScope) and re-parses (see ParseScope).
type Scope int

const (
	// ScopeAll matches every source (the default, no prefix).
	ScopeAll Scope = iota
	// ScopeCheat matches authored cheat commands. Prefix ":c".
	ScopeCheat
	// ScopeProject matches project-detected and custom provider commands.
	// Prefix ":p".
	ScopeProject
	// ScopeHistory matches shell-history commands. Prefix ":h".
	ScopeHistory
	// ScopeBookmark matches saved bookmarks. Prefix ":b".
	ScopeBookmark

	// scopeCount is the number of scopes, used for wrap-around cycling.
	scopeCount
)

// letter returns the single character used in the ":x" prefix, or 0 for ScopeAll
// (which has no prefix).
func (s Scope) letter() byte {
	switch s {
	case ScopeCheat:
		return 'c'
	case ScopeProject:
		return 'p'
	case ScopeHistory:
		return 'h'
	case ScopeBookmark:
		return 'b'
	default:
		return 0
	}
}

// Name returns a short human label for the scope, suitable for a status chip.
func (s Scope) Name() string {
	switch s {
	case ScopeCheat:
		return "cheats"
	case ScopeProject:
		return "project"
	case ScopeHistory:
		return "history"
	case ScopeBookmark:
		return "bookmarks"
	default:
		return "all"
	}
}

// Includes reports whether an entry of the given source belongs to this scope.
func (s Scope) Includes(src entry.Source) bool {
	switch s {
	case ScopeAll:
		return true
	case ScopeCheat:
		return src == entry.SourceCheat
	case ScopeProject:
		return src == entry.SourceProject
	case ScopeHistory:
		return src == entry.SourceHistory
	case ScopeBookmark:
		return src == entry.SourceBookmark
	default:
		return true
	}
}

// Next returns the following scope in the cycle order all → cheats → project →
// history → bookmarks → all. Shift-cycling uses Prev.
func (s Scope) Next() Scope { return (s + 1) % scopeCount }

// Prev returns the previous scope in the cycle order (the reverse of Next).
func (s Scope) Prev() Scope { return (s + scopeCount - 1) % scopeCount }

// ParseScope splits a raw query into its scope and the remaining search text. A
// leading ":x" token — a colon, a known scope letter, then end-of-string or
// whitespace — sets the scope and is stripped from the search text. Anything
// else (a bare ":", an unknown letter like ":z", or a colon mid-string like
// "ubuntu:latest") is left untouched and searched literally, so the prefix never
// hijacks a genuine query.
//
// The returned search text is left-trimmed of the separating whitespace but is
// otherwise the raw remainder; callers trim/tokenise it as they see fit.
func ParseScope(query string) (scope Scope, search string) {
	q := strings.TrimLeft(query, " \t")
	// Need at least ":" + a letter, and that letter must be followed by a word
	// boundary (end of string or whitespace) to count as a scope token.
	if len(q) < 2 || q[0] != ':' {
		return ScopeAll, query
	}
	if len(q) > 2 && q[2] != ' ' && q[2] != '\t' {
		return ScopeAll, query // e.g. ":cd" — not a bare ":c" token
	}
	var sc Scope
	switch q[1] {
	case 'c':
		sc = ScopeCheat
	case 'p':
		sc = ScopeProject
	case 'h':
		sc = ScopeHistory
	case 'b':
		sc = ScopeBookmark
	default:
		return ScopeAll, query // unknown letter → literal
	}
	rest := ""
	if len(q) > 2 {
		rest = strings.TrimLeft(q[2:], " \t")
	}
	return sc, rest
}

// WithScope rebuilds a query string for the given scope and search text. It is
// the inverse of ParseScope and the mechanism behind Tab cycling: the TUI parses
// the current query, advances the scope, and writes the result back.
//
// A non-All scope always keeps a trailing space after its prefix (even with no
// search text) so that characters typed next land after the token rather than
// fusing into it (":c" + "d" must become ":c d", not ":cd").
func WithScope(s Scope, rest string) string {
	if s == ScopeAll {
		return rest
	}
	prefix := ":" + string(s.letter())
	if rest == "" {
		return prefix + " "
	}
	return prefix + " " + rest
}
