// Package entry defines the core data model shared across the whole
// application: an Entry (one selectable command) and its VarSpecs (how each
// placeholder in the command template is resolved). Every provider produces
// Entry values and the resolver and TUI consume them, so this package is the
// central contract and must not import any other internal package.
package entry

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
)

// Source identifies where an Entry came from. It is used for display grouping
// and for deciding how an Entry participates in features such as frecency.
type Source int

const (
	// SourceCheat is an entry authored in a cheat file.
	SourceCheat Source = iota
	// SourceProject is an entry discovered from a project marker (Makefile,
	// package.json, ...).
	SourceProject
	// SourceHistory is an entry read from the user's shell history file.
	SourceHistory
	// SourceBookmark is a user-saved command with optional searchable tags/note.
	SourceBookmark
)

// String returns a short, stable label for a Source, suitable for display in
// the TUI (e.g. "cheat", "project", "history").
func (s Source) String() string {
	switch s {
	case SourceCheat:
		return "cheat"
	case SourceProject:
		return "project"
	case SourceHistory:
		return "history"
	case SourceBookmark:
		return "bookmark"
	default:
		return "unknown"
	}
}

// CommandKind identifies how Entry.Template should be interpreted. Template
// commands use commando's <var>/[optional] grammar; literal commands are shell
// text that must be returned exactly as stored.
type CommandKind int

const (
	// CommandTemplate is the default for authored cheat entries.
	CommandTemplate CommandKind = iota
	// CommandLiteral is used for history, bookmarks, and project-detected
	// commands. Their shell syntax is not parsed as commando template syntax.
	CommandLiteral
)

// String returns a stable label for display/debugging.
func (k CommandKind) String() string {
	switch k {
	case CommandLiteral:
		return "literal"
	default:
		return "template"
	}
}

// Scope controls how far a remembered variable value is shared. It is the
// domain type behind the VarSpec.Remember string.
type Scope string

const (
	// ScopeDefault is the unset value (no `remember` in the cheat). It resolves
	// to the built-in default, ScopeGlobal, so variables remember their values
	// out of the box.
	ScopeDefault Scope = ""
	// ScopeNone explicitly disables remembering for a variable (opt-out).
	ScopeNone Scope = "none"
	// ScopeGlobal remembers a value across every directory.
	ScopeGlobal Scope = "global"
	// ScopeProject remembers a value per git project root.
	ScopeProject Scope = "project"
	// ScopeDir remembers a value per working directory.
	ScopeDir Scope = "dir"
)

// Valid reports whether s is one of the recognised scopes.
func (s Scope) Valid() bool {
	switch s {
	case ScopeDefault, ScopeNone, ScopeGlobal, ScopeProject, ScopeDir:
		return true
	default:
		return false
	}
}

// Effective resolves the unset default: an unset scope means global, so
// variables remember by default. Every other scope is returned unchanged.
func (s Scope) Effective() Scope {
	if s == ScopeDefault {
		return ScopeGlobal
	}
	return s
}

// Remembers reports whether values for this scope should be stored at all.
func (s Scope) Remembers() bool {
	return s.Effective() != ScopeNone
}

// VarSpec describes how a single <name> placeholder in a template is resolved.
//
// A candidate has two projections that need not be equal: the Display string
// (what the user sees and fuzzy-searches) and the Value string (what is inserted
// into the command, remembered, and substituted into dependent Src commands).
// The Label and Value templates below derive each from the columns of a source
// line, so a variable can, e.g., show "chrome (1234)" while inserting only
// "1234". When neither template is set, Display and Value are both the raw line.
type VarSpec struct {
	// Name matches the placeholder token, e.g. Name "file" matches <file>.
	Name string
	// Src is an optional shell command whose stdout lines become the candidate
	// values offered to the user.
	Src string
	// Values is an inline list of candidate lines, offered in addition to (and
	// before) any produced by Src. It lets a cheat provide a fixed choice list
	// without a shell command. Each entry is processed like a Src line (so Match
	// / Columns / Label / Value apply to it too).
	Values []string
	// Default is an optional value pre-filled in the input field.
	Default string
	// Multi allows the user to select more than one candidate value.
	Multi bool
	// Sep is the separator used to join multiple selected values. When empty a
	// single space is used.
	Sep string
	// Each is an optional per-item format string applied to every selected
	// value; the literal "{}" is replaced by the value. For example `"{}"`
	// wraps each value in double quotes.
	Each string
	// Remember sets the persistence scope for the chosen value. Unset
	// (ScopeDefault) means remember globally — variables remember by default.
	// Use "none" to opt out, or "project"/"dir" to scope the memory.
	Remember Scope

	// --- candidate extraction (applied to each Src/Values line) ---

	// HeaderLines drops this many leading lines from the Src output before
	// candidates are extracted (e.g. to skip a `docker images` header row). It
	// does not apply to Values, which are already clean.
	HeaderLines int
	// Delimiter is a regular expression used to split each candidate line into
	// columns for Columns naming. When empty, runs of whitespace are used.
	Delimiter string
	// Columns names the fields of each line after splitting on Delimiter, so the
	// Label and Value templates can reference them as <name>. Positional names
	// <1>, <2>, ... and <0> (the whole line) are always available too. Mutually
	// exclusive with Match.
	Columns []string
	// Match is a regular expression applied to each line; its capture groups
	// populate the names referenced by Label and Value. Named groups
	// (?P<pid>...) are addressable as <pid>; numbered groups as <1>, <2>, ...;
	// <0> is the whole match. A line that does not match is dropped. Mutually
	// exclusive with Columns.
	Match string
	// Label is a template rendered per line into the candidate's Display text
	// (shown and fuzzy-searched). <name> tokens reference Columns names or Match
	// groups. When empty, Display is the raw line.
	Label string
	// Value is a template rendered per line into the candidate's inserted value
	// (also remembered and substituted into dependent Src commands). <name>
	// tokens reference Columns names or Match groups. When empty, the inserted
	// value is the raw line.
	Value string

	// --- interaction ---

	// Strict, when true, restricts the field to the offered candidates: a value
	// the user types that is not among the candidate values is rejected instead
	// of accepted as free text.
	Strict bool
	// Pattern is an optional regular expression the resolved value must match in
	// full; a value that does not match is rejected. Unlike Strict (which limits
	// input to the candidate list), Pattern validates the shape of any value,
	// typed or selected (e.g. `^[0-9]+$` for a PID).
	Pattern string
	// Preview is an optional command run for the highlighted candidate; the
	// literal "{}" is replaced by the candidate value and its output is shown in
	// a preview pane. Other <vars> are substituted like Src.
	Preview string
	// Cache, when set to a Go duration (e.g. "5m"), memoizes the Src command's
	// output for that long, so a slow candidate command is not re-run on every
	// re-fetch within a session.
	Cache string
}

// SepOrDefault returns the configured separator, defaulting to a single space.
func (v VarSpec) SepOrDefault() string {
	if v.Sep == "" {
		return " "
	}
	return v.Sep
}

// Entry is a single selectable command: a short title, optional longer
// description, searchable tags, command text, and (for template entries) the
// specs describing how to resolve each placeholder.
type Entry struct {
	// Title is the short name shown in the result list. Older configs/providers
	// may leave it empty; TitleText falls back to Desc, then Template.
	Title string
	// Desc is optional detail shown for the selected entry. If empty, DescText
	// falls back to TitleText.
	Desc     string
	Tags     []string
	Template string
	Kind     CommandKind
	Vars     []VarSpec
	Source   Source
	// LastUsed is the unix timestamp when this entry was last launched, or 0 if
	// never used. The aggregator fills it from recorded usage; the ranker uses
	// it to put the most recently used commands first.
	LastUsed int64
	// UseCount is how many times this entry has been launched (0 if never). Used
	// as a secondary ranking signal.
	UseCount int
	// ProviderOrder preserves source-native recency when no Commando usage data
	// exists. Larger values are newer. History uses the command's last position
	// in the history file; other providers leave it zero.
	ProviderOrder int64
	// StableID is an optional author-assigned identity (the cheat `id` field).
	// When set, ID uses it instead of hashing the template and title, so a cheat
	// can be retitled or its template edited without orphaning its remembered
	// values and usage.
	StableID string
	// Origin labels the cheat source an entry was loaded from (e.g.
	// "team-cheats" or "commando/docker.toml"). It is empty for the user's own
	// cheats and for every non-cheat entry.
	Origin string

	// search caches the lowercased searchable fields; see PrepareSearch.
	search *SearchFields
}

// SearchFields are an entry's searchable fields, lowercased once so ranking a
// large list per keystroke does not re-lowercase every entry. A field equal to
// a higher-priority one (Title > Tags > Template > Desc) is left empty, since
// matching it again could never score better; history entries, whose title,
// command, and description are the same text, are then scanned once.
type SearchFields struct {
	Title    string
	Tags     string
	Template string
	Desc     string
}

// PrepareSearch precomputes the lowercased search fields. Call it after the
// entry's text fields are final; a later edit to Title/Desc/Tags/Template
// must be followed by another PrepareSearch (or the cache goes stale).
func (e *Entry) PrepareSearch() {
	f := e.computeSearchFields()
	e.search = &f
}

// LowerSearchFields returns the lowercased search fields, from the cache when
// PrepareSearch was called and computed on the fly otherwise.
func (e *Entry) LowerSearchFields() SearchFields {
	if e.search != nil {
		return *e.search
	}
	return e.computeSearchFields()
}

func (e *Entry) computeSearchFields() SearchFields {
	f := SearchFields{
		Title:    strings.ToLower(e.TitleText()),
		Tags:     strings.ToLower(strings.Join(e.Tags, " ")),
		Template: strings.ToLower(e.Template),
		Desc:     strings.ToLower(e.Desc),
	}
	if f.Template == f.Title || f.Template == f.Tags {
		f.Template = ""
	}
	if f.Desc == f.Title || f.Desc == f.Tags || f.Desc == strings.ToLower(e.Template) {
		f.Desc = ""
	}
	return f
}

// TitleText returns the primary display title, with compatibility fallbacks for
// entries created before Title existed.
func (e Entry) TitleText() string {
	if strings.TrimSpace(e.Title) != "" {
		return e.Title
	}
	if strings.TrimSpace(e.Desc) != "" {
		return e.Desc
	}
	return e.Template
}

// DescText returns the selected-entry detail. A command without a separate
// description uses its title so the footer always has useful text.
func (e Entry) DescText() string {
	if strings.TrimSpace(e.Desc) != "" && strings.TrimSpace(e.Desc) != strings.TrimSpace(e.TitleText()) {
		return e.Desc
	}
	return e.TitleText()
}

// Var returns the VarSpec with the given name and whether it was found.
func (e Entry) Var(name string) (VarSpec, bool) {
	for _, v := range e.Vars {
		if v.Name == name {
			return v, true
		}
	}
	return VarSpec{}, false
}

// ID returns a stable identifier for the entry, derived from its source,
// template, and title. It is used as the key for history and memory so that the
// same logical command keeps its remembered values across runs even as ordering
// changes. Desc is intentionally excluded so adding optional explanatory detail
// does not reset memory.
//
// A non-empty StableID takes precedence: the identity is then
// "<source>:<StableID>", independent of the template and title.
func (e Entry) ID() string {
	if id := strings.TrimSpace(e.StableID); id != "" {
		return e.Source.String() + ":" + id
	}
	h := sha1.New()
	h.Write([]byte(e.Source.String()))
	h.Write([]byte{0})
	h.Write([]byte(e.Template))
	h.Write([]byte{0})
	h.Write([]byte(e.TitleText()))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// SearchText returns the text used for fuzzy matching: title, description,
// command, and tags. Keeping this in one place ensures list rendering and
// filtering agree on what is searchable.
func (e Entry) SearchText() string {
	parts := []string{e.TitleText(), e.Desc, e.Template}
	parts = append(parts, e.Tags...)
	var out []string
	seen := make(map[string]bool, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return strings.Join(out, " ")
}
