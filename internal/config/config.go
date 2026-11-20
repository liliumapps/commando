// Package config loads commando's configuration and cheat files from disk. It
// defines the on-disk TOML schema and translates it into the domain types used
// elsewhere (entry.Entry, provider definitions). Validation happens here so the
// rest of the code base can assume well-formed data.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/resolve"
	commandtemplate "github.com/liliumapps/commando/internal/template"
)

// Config is the top-level configuration loaded from config.toml.
type Config struct {
	// KeyBinding is the portable control-key sequence bound to the summon widget
	// (e.g. "^G").
	KeyBinding string `toml:"key_binding"`
	// HistoryIgnore is a list of glob patterns; commands matching any of them
	// are hidden from history, and remembered values/bookmarks matching them
	// are not stored. `*` matches any run of characters (including `/`), the
	// match is case-insensitive against the whole line and each token, and a
	// pattern without wildcards is a substring match. Built-in secret-shaped
	// value detectors (see LooksSecret) always apply as well.
	HistoryIgnore []string `toml:"history_ignore"`
	// History tunes how shell history participates in the list.
	History HistoryConfig `toml:"history"`
	// Providers holds user-defined project providers.
	Providers []ProviderDef `toml:"provider"`
}

// HistoryConfig controls how many shell-history commands are surfaced.
type HistoryConfig struct {
	// ListLimit caps how many history commands appear in the list when the user
	// is NOT searching. A query shows ALL matching history regardless of this.
	// Default 10; a negative value means unlimited.
	ListLimit int `toml:"list_limit"`
	// MaxLoad caps how many unique commands are loaded from the history tail.
	// Default 5000; a negative value means unlimited.
	MaxLoad int `toml:"max_load"`
	// MaxBytes caps history file I/O to the newest bytes. Default 8 MiB; a
	// negative value reads the whole file.
	MaxBytes int64 `toml:"max_bytes"`
}

// EffectiveListLimit returns the non-search display cap for history, resolving
// the "negative means unlimited" convention to 0 (which callers treat as no
// cap) and a zero value to the default.
func (h HistoryConfig) EffectiveListLimit() int {
	if h.ListLimit < 0 {
		return 0 // unlimited
	}
	if h.ListLimit == 0 {
		return defaultHistoryListLimit
	}
	return h.ListLimit
}

// EffectiveMaxLoad returns the unique-command cap. Zero resolves to the safe
// default and a negative value means unlimited.
func (h HistoryConfig) EffectiveMaxLoad() int {
	if h.MaxLoad < 0 {
		return 0
	}
	if h.MaxLoad == 0 {
		return defaultHistoryMaxLoad
	}
	return h.MaxLoad
}

// EffectiveMaxBytes returns the history tail byte cap. Zero resolves to the
// safe default and a negative value means unlimited.
func (h HistoryConfig) EffectiveMaxBytes() int64 {
	if h.MaxBytes < 0 {
		return 0
	}
	if h.MaxBytes == 0 {
		return defaultHistoryMaxBytes
	}
	return h.MaxBytes
}

// ProviderDef is a user-defined project provider from config.toml.
type ProviderDef struct {
	Name     string `toml:"name"`
	Detect   string `toml:"detect"`   // marker filename that must exist
	List     string `toml:"list"`     // shell command producing candidate items (one per line)
	Template string `toml:"template"` // template with {} replaced by each item
	Cache    string `toml:"cache"`    // optional cache duration, e.g. "5m"
}

// defaultHistoryListLimit is how many history commands show without a query.
const (
	defaultHistoryListLimit = 10
	defaultHistoryMaxLoad   = 5000
	defaultHistoryMaxBytes  = 8 << 20
)

// Default returns the built-in configuration used when no config.toml exists.
func Default() *Config {
	return &Config{
		KeyBinding: "^G",
		HistoryIgnore: []string{
			"*password*", "*passwd*", "*secret*", "*token*",
			"*apikey*", "*api_key*", "export *KEY*", "*credential*",
			"*authorization:*", "*bearer *",
			"aws_*=*", "*_token=*", "*_secret=*", "*_pat=*",
		},
		History: HistoryConfig{
			ListLimit: defaultHistoryListLimit,
			MaxLoad:   defaultHistoryMaxLoad,
			MaxBytes:  defaultHistoryMaxBytes,
		},
	}
}

// Load reads config.toml from p.ConfigDir. A missing file is not an error: the
// default configuration is returned. User settings are merged over the
// defaults so that omitted fields keep sensible values.
func Load(p Paths) (*Config, error) {
	cfg := Default()
	path := filepath.Join(p.ConfigDir, "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var user Config
	if err := decodeStrict(path, data, &user); err != nil {
		return nil, err
	}
	if user.KeyBinding != "" {
		cfg.KeyBinding = user.KeyBinding
	}
	if user.HistoryIgnore != nil {
		cfg.HistoryIgnore = user.HistoryIgnore
	}
	// Merge history tuning field-by-field so a user can override just one.
	// A negative value explicitly means "unlimited" and is preserved; only the
	// zero value falls back to the default.
	if user.History.ListLimit != 0 {
		cfg.History.ListLimit = user.History.ListLimit
	}
	if user.History.MaxLoad != 0 {
		cfg.History.MaxLoad = user.History.MaxLoad
	}
	if user.History.MaxBytes != 0 {
		cfg.History.MaxBytes = user.History.MaxBytes
	}
	cfg.Providers = append(cfg.Providers, user.Providers...)

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if !validKeyBinding(c.KeyBinding) {
		return fmt.Errorf("key_binding must use ^X control-key notation")
	}
	seenNames := make(map[string]bool, len(c.Providers))
	for i := range c.Providers {
		pd := &c.Providers[i]
		pd.Name = strings.TrimSpace(pd.Name)
		pd.Detect = strings.TrimSpace(pd.Detect)
		pd.List = strings.TrimSpace(pd.List)
		pd.Template = strings.TrimSpace(pd.Template)
		pd.Cache = strings.TrimSpace(pd.Cache)

		if pd.Name == "" {
			return fmt.Errorf("provider #%d: name is required", i+1)
		}
		nameKey := strings.ToLower(pd.Name)
		if seenNames[nameKey] {
			return fmt.Errorf("provider %q: name is duplicated", pd.Name)
		}
		seenNames[nameKey] = true
		if pd.Detect == "" {
			return fmt.Errorf("provider %q: detect is required", pd.Name)
		}
		cleanDetect := filepath.Clean(pd.Detect)
		if cleanDetect == "." || filepath.IsAbs(pd.Detect) ||
			cleanDetect == ".." || strings.HasPrefix(cleanDetect, ".."+string(filepath.Separator)) {
			return fmt.Errorf("provider %q: detect must be a relative path within the project", pd.Name)
		}
		if pd.List == "" {
			return fmt.Errorf("provider %q: list is required", pd.Name)
		}
		if pd.Template == "" {
			return fmt.Errorf("provider %q: template is required", pd.Name)
		}
		if !strings.Contains(pd.Template, "{}") {
			return fmt.Errorf("provider %q: template must contain {}", pd.Name)
		}
		if pd.Cache != "" {
			duration, err := time.ParseDuration(pd.Cache)
			if err != nil {
				return fmt.Errorf("provider %q: invalid cache duration %q: %w", pd.Name, pd.Cache, err)
			}
			if duration < 0 {
				return fmt.Errorf("provider %q: cache duration must not be negative", pd.Name)
			}
		}
	}
	return nil
}

func validKeyBinding(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 2 || value[0] != '^' {
		return false
	}
	return (value[1] >= 'A' && value[1] <= 'Z') || (value[1] >= 'a' && value[1] <= 'z')
}

func decodeStrict(name string, data []byte, dst any) error {
	metadata, err := toml.Decode(string(data), dst)
	if err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	unknown := metadata.Undecoded()
	if len(unknown) == 0 {
		return nil
	}
	names := make([]string, len(unknown))
	for i, key := range unknown {
		names[i] = key.String()
	}
	sort.Strings(names)
	return fmt.Errorf("parse %s: unknown field(s): %s", name, strings.Join(names, ", "))
}

// --- Cheat file schema ---

// cheatFile is the TOML structure of a cheats/*.toml file.
//
// Vars declares reusable variable groups shared across commands in the file,
// written as `[[vars.GROUP]]` array-of-tables. A command opts into one or more
// groups with `use = ["GROUP", ...]`; the group's specs are merged in, and a
// command-local `[[cmd.var]]` with the same name overrides the shared one. This
// lets common variables be defined once and reused across many commands.
type cheatFile struct {
	Vars map[string][]cheatVar `toml:"vars"`
	Cmds []cheatCmd            `toml:"cmd"`
}

type cheatCmd struct {
	// ID is an optional stable identity. When set, remembered values and usage
	// are keyed by it, so editing the title or template keeps them.
	ID    string     `toml:"id"`
	Title string     `toml:"title"`
	Desc  string     `toml:"desc"`
	Tmpl  string     `toml:"tmpl"`
	Tags  []string   `toml:"tags"`
	Use   []string   `toml:"use"`
	Vars  []cheatVar `toml:"var"`
}

type cheatVar struct {
	Name        string   `toml:"name"`
	Src         string   `toml:"src"`
	Values      []string `toml:"values"`
	Default     string   `toml:"default"`
	Multi       bool     `toml:"multi"`
	Sep         string   `toml:"sep"`
	Each        string   `toml:"each"`
	Remember    string   `toml:"remember"`
	HeaderLines int      `toml:"header_lines"`
	Delimiter   string   `toml:"delimiter"`
	Columns     []string `toml:"columns"`
	Match       string   `toml:"match"`
	Label       string   `toml:"label"`
	Value       string   `toml:"value"`
	Strict      bool     `toml:"strict"`
	Pattern     string   `toml:"pattern"`
	Preview     string   `toml:"preview"`
	Cache       string   `toml:"cache"`
}

// LoadCheats reads every *.toml file in p.CheatsDir and returns the entries they
// define. A missing cheats directory yields no entries and no error. Individual
// files are validated; a malformed file produces an error naming the file.
func LoadCheats(p Paths) ([]entry.Entry, error) {
	matches, err := filepath.Glob(filepath.Join(p.CheatsDir, "*.toml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)

	var entries []entry.Entry
	var errs []error
	ids := make(map[string]string) // stable id -> file that declared it
	for _, path := range matches {
		es, err := loadCheatFile(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, e := range es {
			if e.StableID != "" {
				if prev, dup := ids[e.StableID]; dup {
					errs = append(errs, fmt.Errorf("%s: id %q is already used in %s", filepath.Base(path), e.StableID, prev))
					continue
				}
				ids[e.StableID] = filepath.Base(path)
			}
			entries = append(entries, e)
		}
	}
	return entries, errors.Join(errs...)
}

// LoadCheatData parses cheat entries from raw TOML bytes. It is exported to make
// the sample-cheat validation and unit tests straightforward.
func LoadCheatData(name string, data []byte) ([]entry.Entry, error) {
	var cf cheatFile
	if err := decodeStrict(name, data, &cf); err != nil {
		return nil, err
	}
	entries := make([]entry.Entry, 0, len(cf.Cmds))
	ids := make(map[string]int)
	for i, c := range cf.Cmds {
		e, err := c.toEntry(cf.Vars)
		if err != nil {
			return nil, fmt.Errorf("%s: cmd #%d: %w", name, i+1, err)
		}
		if e.StableID != "" {
			if prev, dup := ids[e.StableID]; dup {
				return nil, fmt.Errorf("%s: cmd #%d: id %q duplicates cmd #%d", name, i+1, e.StableID, prev)
			}
			ids[e.StableID] = i + 1
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func loadCheatFile(path string) ([]entry.Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return LoadCheatData(filepath.Base(path), data)
}

// toEntry builds an entry from a command, merging in any shared variable groups
// named in its `use` list. Shared specs are added first, in the order the groups
// are listed; a command-local var with the same name overrides the shared one
// (and keeps the command's position for it). groups may be nil.
func (c cheatCmd) toEntry(groups map[string][]cheatVar) (entry.Entry, error) {
	title, desc := c.entryTitleDesc()
	if title == "" {
		return entry.Entry{}, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(c.Tmpl) == "" {
		return entry.Entry{}, fmt.Errorf("tmpl is required")
	}
	if _, err := commandtemplate.Parse(c.Tmpl); err != nil {
		return entry.Entry{}, fmt.Errorf("invalid tmpl: %w", err)
	}
	id := strings.TrimSpace(c.ID)
	if c.ID != "" && !validCheatID(id) {
		return entry.Entry{}, fmt.Errorf("invalid id %q: use letters, digits, '.', '_', '-', or '/'", c.ID)
	}

	// Collect variable definitions: shared groups first (in `use` order), then
	// command-local vars. A later definition for the same name replaces an
	// earlier one, so command-local vars win over shared ones.
	byName := make(map[string]entry.VarSpec)
	var order []string
	add := func(v cheatVar) error {
		spec, err := v.toSpec()
		if err != nil {
			return err
		}
		if _, seen := byName[spec.Name]; !seen {
			order = append(order, spec.Name)
		}
		byName[spec.Name] = spec
		return nil
	}

	for _, g := range c.Use {
		shared, ok := groups[g]
		if !ok {
			return entry.Entry{}, fmt.Errorf("unknown shared variable group %q", g)
		}
		for _, v := range shared {
			if err := add(v); err != nil {
				return entry.Entry{}, err
			}
		}
	}
	for _, v := range c.Vars {
		if err := add(v); err != nil {
			return entry.Entry{}, err
		}
	}

	vars := make([]entry.VarSpec, len(order))
	for i, name := range order {
		vars[i] = byName[name]
	}
	return entry.Entry{
		Title:    title,
		Desc:     desc,
		Tags:     c.Tags,
		Template: c.Tmpl,
		Vars:     vars,
		Source:   entry.SourceCheat,
		StableID: id,
	}, nil
}

// cheatIDRE restricts stable ids to a readable, grep-friendly alphabet.
var cheatIDRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

func validCheatID(id string) bool { return len(id) <= 128 && cheatIDRE.MatchString(id) }

func (c cheatCmd) entryTitleDesc() (title, desc string) {
	title = strings.TrimSpace(c.Title)
	desc = strings.TrimSpace(c.Desc)
	// Backward compatibility: existing cheats use `desc` as the short list
	// label. When `title` is omitted, keep that behavior and treat desc as the
	// title, not as a separate detail line.
	if title == "" {
		title = desc
		desc = ""
	}
	return title, desc
}

// toSpec converts a parsed cheat variable into an entry.VarSpec, validating its
// fields. Extraction is validated eagerly so a malformed cheat fails at load
// with a clear message rather than silently yielding blank or missing
// candidates at runtime.
func (v cheatVar) toSpec() (entry.VarSpec, error) {
	if v.Name == "" {
		return entry.VarSpec{}, fmt.Errorf("var name is required")
	}
	scope := entry.Scope(v.Remember)
	if !scope.Valid() {
		return entry.VarSpec{}, fmt.Errorf("var %q: invalid remember scope %q", v.Name, v.Remember)
	}
	if v.HeaderLines < 0 {
		return entry.VarSpec{}, fmt.Errorf("var %q: header_lines must be >= 0", v.Name)
	}

	names, err := v.extractionNames()
	if err != nil {
		return entry.VarSpec{}, err
	}
	if err := checkTemplateRefs(v.Name, "label", v.Label, names); err != nil {
		return entry.VarSpec{}, err
	}
	if err := checkTemplateRefs(v.Name, "value", v.Value, names); err != nil {
		return entry.VarSpec{}, err
	}

	if v.Pattern != "" {
		if _, err := regexp.Compile(v.Pattern); err != nil {
			return entry.VarSpec{}, fmt.Errorf("var %q: invalid pattern regex: %w", v.Name, err)
		}
	}
	if v.Cache != "" {
		duration, err := time.ParseDuration(v.Cache)
		if err != nil {
			return entry.VarSpec{}, fmt.Errorf("var %q: invalid cache duration %q: %w", v.Name, v.Cache, err)
		}
		if duration < 0 {
			return entry.VarSpec{}, fmt.Errorf("var %q: cache duration must not be negative", v.Name)
		}
	}

	return entry.VarSpec{
		Name:        v.Name,
		Src:         v.Src,
		Values:      v.Values,
		Default:     v.Default,
		Multi:       v.Multi,
		Sep:         v.Sep,
		Each:        v.Each,
		Remember:    scope,
		HeaderLines: v.HeaderLines,
		Delimiter:   v.Delimiter,
		Columns:     v.Columns,
		Match:       v.Match,
		Label:       v.Label,
		Value:       v.Value,
		Strict:      v.Strict,
		Pattern:     v.Pattern,
		Preview:     v.Preview,
		Cache:       v.Cache,
	}, nil
}

// extractionNames validates the columns/match configuration and returns the set
// of <name> tokens the label/value templates may reference. Positional names
// (<0>, <1>, ...) are always allowed and so are not enumerated here; the
// returned set covers only the named columns or capture groups.
func (v cheatVar) extractionNames() (map[string]bool, error) {
	if v.Match != "" && len(v.Columns) > 0 {
		return nil, fmt.Errorf("var %q: `match` and `columns` are mutually exclusive", v.Name)
	}
	names := make(map[string]bool)

	if v.Delimiter != "" {
		if _, err := regexp.Compile(v.Delimiter); err != nil {
			return nil, fmt.Errorf("var %q: invalid delimiter regex: %w", v.Name, err)
		}
	}
	for _, c := range v.Columns {
		if c != "" {
			names[c] = true
		}
	}

	if v.Match != "" {
		re, err := regexp.Compile(v.Match)
		if err != nil {
			return nil, fmt.Errorf("var %q: invalid match regex: %w", v.Name, err)
		}
		for _, n := range re.SubexpNames() {
			if n != "" {
				names[n] = true
			}
		}
	}
	return names, nil
}

// checkTemplateRefs verifies that every <name> placeholder in a label/value
// template refers to a known column/group name or a positional index. It keeps
// authoring errors (typos, missing columns) at load time.
func checkTemplateRefs(varName, field, tmpl string, names map[string]bool) error {
	for _, ref := range resolve.SrcRefs(tmpl) {
		if isPositionalRef(ref) || names[ref] {
			continue
		}
		return fmt.Errorf("var %q: %s template references unknown name <%s>", varName, field, ref)
	}
	return nil
}

// isPositionalRef reports whether ref is an all-digit positional column name
// (<0> = whole line/match, <1>, <2>, ... = columns/groups by position), which
// is always available regardless of columns/match naming.
func isPositionalRef(ref string) bool {
	if ref == "" {
		return false
	}
	for _, r := range ref {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
