package history

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/provider"
)

// Ignorer decides whether a command should be excluded (e.g. secret-looking
// commands). The config package's ShouldIgnore satisfies this.
type Ignorer func(cmd string) bool

// Provider surfaces the user's shell command history as entries, read live from
// the shell history file each time Entries is called. Commands are de-duplicated
// (keeping the most recent occurrence) and ranked so recent, frequently used
// commands sort first.
type Provider struct {
	// path is the history file to read. When empty it is resolved at call time
	// from $HISTFILE and the usual fallbacks.
	path string
	// limit caps how many unique commands are surfaced (0 = no cap).
	limit int
	// maxBytes caps how much of the file tail is read (0 = no byte cap).
	maxBytes int64
	// ignore, when set, drops matching commands (redaction denylist).
	ignore Ignorer
	// readFile is the file reader; overridable in tests.
	readFile func(string) ([]byte, error)
	// lookupEnv resolves environment variables; overridable in tests.
	lookupEnv func(string) (string, bool)
	// homeDir returns the user home; overridable in tests.
	homeDir func() (string, error)
}

// Option configures a Provider.
type Option func(*Provider)

// WithPath forces a specific history file instead of auto-detection.
func WithPath(path string) Option { return func(p *Provider) { p.path = path } }

// WithLimit caps the number of unique commands surfaced.
func WithLimit(n int) Option { return func(p *Provider) { p.limit = n } }

// WithMaxBytes caps history I/O to the newest n bytes. Zero disables the cap.
func WithMaxBytes(n int64) Option { return func(p *Provider) { p.maxBytes = n } }

// WithIgnore sets the redaction predicate.
func WithIgnore(fn Ignorer) Option { return func(p *Provider) { p.ignore = fn } }

const defaultMaxReadBytes = 8 << 20

// New returns a history provider. By default it surfaces up to 500 unique
// commands from the newest 8 MiB of the auto-detected shell history file.
func New(opts ...Option) *Provider {
	p := &Provider{
		limit:     500,
		maxBytes:  defaultMaxReadBytes,
		lookupEnv: os.LookupEnv,
		homeDir:   os.UserHomeDir,
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Name implements provider.Provider.
func (p *Provider) Name() string { return "history" }

// Path returns the history file this provider would read.
func (p *Provider) Path() string { return p.resolvePath() }

// Entries implements provider.Provider. It reads and parses the history file,
// de-duplicates commands (newest wins), applies the ignore predicate, and
// returns them newest-first, capped at the configured limit.
func (p *Provider) Entries(_ provider.Context) ([]entry.Entry, error) {
	path := p.resolvePath()
	if path == "" {
		return nil, nil
	}
	var data []byte
	var err error
	if p.readFile != nil {
		data, err = p.readFile(path)
	} else {
		data, err = readTail(path, p.maxBytes)
	}
	if err != nil {
		// A missing/unreadable history file is not fatal — just no history.
		return nil, nil
	}

	var parsed []HistEntry
	if filepath.Base(path) == "fish_history" {
		parsed = parseFish(string(data))
	} else {
		if isZshHistory(path, data) {
			data = unmetafyZsh(data)
		}
		parsed = Parse(string(data))
	}
	orderByTimestamp(parsed)

	// Walk newest-first, de-duplicating as we go. Stop as soon as the configured
	// unique-command limit is satisfied instead of retaining the whole parsed
	// history in another map and slice.
	seen := make(map[string]bool)
	out := make([]entry.Entry, 0, min(len(parsed), max(p.limit, 0)))
	for i := len(parsed) - 1; i >= 0; i-- {
		cmd := parsed[i].Cmd
		if seen[cmd] {
			continue
		}
		seen[cmd] = true
		if p.ignore != nil && p.ignore(cmd) {
			continue
		}
		if p.limit > 0 && len(out) >= p.limit {
			break
		}
		out = append(out, entry.Entry{
			Title:         cmd,
			Desc:          cmd,
			Template:      cmd,
			Kind:          entry.CommandLiteral,
			Source:        entry.SourceHistory,
			ProviderOrder: int64(i + 1),
		})
	}
	return out, nil
}

// readTail returns at most maxBytes from the newest end of path. When the read
// starts mid-line, that partial line is discarded so it cannot become a bogus
// command. A non-positive cap intentionally reads the whole file.
func readTail(path string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return os.ReadFile(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() <= maxBytes {
		return io.ReadAll(io.LimitReader(f, maxBytes+1))
	}

	start := info.Size() - maxBytes
	// Include the byte immediately before the bounded range. If it is a newline,
	// start is already aligned; otherwise discard through the first newline.
	section := io.NewSectionReader(f, start-1, maxBytes+1)
	data, err := io.ReadAll(section)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return data, nil
	}
	if data[0] == '\n' {
		return data[1:], nil
	}
	for i, b := range data {
		if b == '\n' {
			return data[i+1:], nil
		}
	}
	return nil, nil
}

// resolvePath returns the history file to read: the explicit path if set, then
// $HISTFILE, then conventional files prioritized for the active shell.
func (p *Provider) resolvePath() string {
	if p.path != "" {
		return p.path
	}
	if hf, ok := p.lookupEnv("HISTFILE"); ok && hf != "" {
		return hf
	}
	home, err := p.homeDir()
	if err != nil {
		return ""
	}
	dataHome, ok := p.lookupEnv("XDG_DATA_HOME")
	if !ok || dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	fishPath := filepath.Join(dataHome, "fish", "fish_history")
	zshPath := filepath.Join(home, ".zsh_history")
	genericPath := filepath.Join(home, ".history")
	bashPath := filepath.Join(home, ".bash_history")

	var candidates []string
	shellPath, _ := p.lookupEnv("SHELL")
	switch filepath.Base(shellPath) {
	case "fish":
		candidates = []string{fishPath, zshPath, genericPath, bashPath}
	case "bash":
		candidates = []string{bashPath, genericPath, zshPath, fishPath}
	case "zsh":
		candidates = []string{zshPath, genericPath, bashPath, fishPath}
	default:
		candidates = []string{zshPath, fishPath, genericPath, bashPath}
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// isZshHistory reports whether a history file is zsh's, whose bytes are
// metafied: by name (.zsh_history, zsh_history, ...) or by the extended
// ": <ts>:<elapsed>;" format on its first non-blank line. Other shells' files
// are never unmetafied, since 0x83 is a common UTF-8 continuation byte.
func isZshHistory(path string, data []byte) bool {
	if strings.Contains(strings.ToLower(filepath.Base(path)), "zsh") {
		return true
	}
	for len(data) > 0 {
		line := data
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line, data = data[:i], data[i+1:]
		} else {
			data = nil
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		return zshExtendedRE.Match(line)
	}
	return false
}

var zshExtendedRE = regexp.MustCompile(`^: \d+:\d+;`)

// orderByTimestamp sorts history oldest-first by timestamp when the file
// carries timestamps, so files appended out of order (bash `histappend` writes
// each session at exit) still rank by when commands actually ran. Entries
// without a timestamp inherit the previous entry's, keeping their file
// position relative to their neighbours. Files without timestamps keep file
// order.
func orderByTimestamp(parsed []HistEntry) {
	hasTS := false
	for _, h := range parsed {
		if h.TS > 0 {
			hasTS = true
			break
		}
	}
	if !hasTS {
		return
	}
	keys := make([]int64, len(parsed))
	var last int64
	for i, h := range parsed {
		if h.TS > 0 {
			last = h.TS
		}
		keys[i] = last
	}
	idx := make([]int, len(parsed))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return keys[idx[a]] < keys[idx[b]] })
	sorted := make([]HistEntry, len(parsed))
	for i, j := range idx {
		sorted[i] = parsed[j]
	}
	copy(parsed, sorted)
}
