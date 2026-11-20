package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/liliumapps/commando/internal/config"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/persist"
	"github.com/liliumapps/commando/internal/provider"
	"github.com/liliumapps/commando/internal/resolve"
	"github.com/liliumapps/commando/internal/shellcmd"
)

// builtinDetector parses entries from a located marker file. Detectors in the
// same group are alternatives for one tool (GNUmakefile/makefile/Makefile);
// only the nearest marker of a group is used, with ties going to the earlier
// detector (the tool's own lookup precedence).
type builtinDetector struct {
	marker string
	group  string
	parse  func(f found) []entry.Entry
}

// builtins are the built-in project detectors, evaluated in this order.
var builtins = []builtinDetector{
	{"GNUmakefile", "make", makeEntries},
	{"makefile", "make", makeEntries},
	{"Makefile", "make", makeEntries},
	{"package.json", "node", packageEntries},
	{"Cargo.toml", "cargo", cargoEntries},
	{"build.gradle", "gradle", gradleEntries},
	{"build.gradle.kts", "gradle", gradleEntries},
	{"justfile", "just", justEntries},
	{"Justfile", "just", justEntries},
	{".justfile", "just", justEntries},
	{"compose.yaml", "compose", composeEntries},
	{"compose.yml", "compose", composeEntries},
	{"docker-compose.yaml", "compose", composeEntries},
	{"docker-compose.yml", "compose", composeEntries},
}

// selectedBuiltin is a detector paired with the marker it will parse.
type selectedBuiltin struct {
	det builtinDetector
	f   found
}

// selectBuiltins picks, per detector group, the nearest found marker (deepest
// directory), breaking ties by detector order. The result follows the order of
// each group's first detector.
func selectBuiltins(byMarker map[string]found) []selectedBuiltin {
	best := make(map[string]selectedBuiltin)
	var groups []string
	for _, b := range builtins {
		f, ok := byMarker[b.marker]
		if !ok {
			continue
		}
		cur, have := best[b.group]
		if !have {
			groups = append(groups, b.group)
			best[b.group] = selectedBuiltin{b, f}
		} else if len(f.dir) > len(cur.f.dir) {
			best[b.group] = selectedBuiltin{b, f}
		}
	}
	out := make([]selectedBuiltin, 0, len(groups))
	for _, g := range groups {
		out = append(out, best[g])
	}
	return out
}

// Provider discovers project commands from the working directory. It combines
// built-in detectors with user-defined providers from config, and caches the
// output of user listers (which may be slow) for their configured duration.
type Provider struct {
	userDefs []config.ProviderDef

	mu    sync.Mutex
	cache map[string]cacheEntry
	// disk persists cached lister output across launches; nil disables it.
	disk *persist.DiskCache
	now  func() time.Time
	// runner executes a lister command in dir and returns stdout lines. It is a
	// field so tests can stub command execution.
	runner func(dir, command string) ([]string, error)
}

type cacheEntry struct {
	items   []string
	expires time.Time
}

// New returns a project provider using the given user-defined provider defs.
func New(userDefs []config.ProviderDef) *Provider {
	return &Provider{
		userDefs: userDefs,
		cache:    make(map[string]cacheEntry),
		now:      time.Now,
		runner:   runLister,
	}
}

// SetDiskCache enables the cross-launch cache layer for user listers that set
// a cache duration. The in-memory layer is always consulted first.
func (p *Provider) SetDiskCache(c *persist.DiskCache) { p.disk = c }

// Name implements provider.Provider.
func (p *Provider) Name() string { return "project" }

// Entries implements provider.Provider. It returns built-in and user-defined
// project commands discovered from ctx.Cwd upward.
func (p *Provider) Entries(ctx provider.Context) ([]entry.Entry, error) {
	if ctx.Cwd == "" {
		return nil, nil
	}

	// Collect the set of markers we care about (built-in + user-defined).
	markers := make(map[string]bool)
	for _, b := range builtins {
		markers[b.marker] = true
	}
	for _, ud := range p.userDefs {
		markers[ud.Detect] = true
	}

	foundMarkers := findMarkers(ctx.Cwd, ctx.GitRoot, markers)
	byMarker := make(map[string]found, len(foundMarkers))
	for _, f := range foundMarkers {
		byMarker[f.marker] = f
	}

	var out []entry.Entry
	var errs []error

	// Built-ins: the nearest marker per tool.
	for _, sel := range selectBuiltins(byMarker) {
		out = append(out, sel.det.parse(sel.f)...)
	}

	// User-defined providers.
	for _, ud := range p.userDefs {
		f, ok := byMarker[ud.Detect]
		if !ok {
			continue
		}
		entries, err := p.userEntries(ud, f)
		if err != nil {
			errs = append(errs, fmt.Errorf("provider %q: %w", ud.Name, err))
			continue
		}
		out = append(out, entries...)
	}

	// Git actions when inside a repository.
	if ctx.GitRoot != "" || pathExists(joinGit(ctx.Cwd)) {
		out = append(out, gitEntries()...)
	}

	return out, errors.Join(errs...)
}

// userEntries runs (or reads from cache) a user-defined lister and builds
// entries from its output.
func (p *Provider) userEntries(ud config.ProviderDef, f found) ([]entry.Entry, error) {
	items, err := p.listCached(ud, f.dir)
	if err != nil {
		return nil, err
	}
	entries := make([]entry.Entry, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if err := validateProviderItem(item); err != nil {
			return nil, err
		}
		cmd := strings.ReplaceAll(ud.Template, "{}", quoteProviderItem(item))
		entries = append(entries, entry.Entry{
			Title:    cmd,
			Tags:     []string{ud.Name, "project"},
			Template: cmd,
			Kind:     entry.CommandLiteral,
			Source:   entry.SourceProject,
		})
	}
	return entries, nil
}

const maxProviderItemBytes = 4096

func validateProviderItem(item string) error {
	if len(item) > maxProviderItemBytes {
		return fmt.Errorf("lister item exceeds %d bytes", maxProviderItemBytes)
	}
	if strings.IndexByte(item, 0) >= 0 {
		return fmt.Errorf("lister item contains a NUL byte")
	}
	return nil
}

func quoteProviderItem(item string) string {
	for _, r := range item {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || strings.ContainsRune("_@%+=:,./-", r) {
			continue
		}
		return resolve.ShellQuote(item)
	}
	return item
}

// listCached returns the lister output for a user provider, using the cache when
// a non-expired entry exists and the provider configured a cache duration. The
// in-memory layer is checked first, then the disk cache (keyed by the lister
// command and directory), so a slow lister is not re-run on every launch.
func (p *Provider) listCached(ud config.ProviderDef, dir string) ([]string, error) {
	ttl := parseDuration(ud.Cache)
	key := ud.Name + "\x00" + dir

	if ttl > 0 {
		p.mu.Lock()
		if ce, ok := p.cache[key]; ok && p.now().Before(ce.expires) {
			p.mu.Unlock()
			return ce.items, nil
		}
		p.mu.Unlock()

		if p.disk != nil {
			var items []string
			if expires, ok := p.disk.Get(ud.List, dir, ttl, &items); ok {
				p.mu.Lock()
				p.cache[key] = cacheEntry{items: items, expires: expires}
				p.mu.Unlock()
				return items, nil
			}
		}
	}

	items, err := p.runner(dir, ud.List)
	if err != nil {
		return nil, err
	}

	if ttl > 0 {
		p.mu.Lock()
		p.cache[key] = cacheEntry{items: items, expires: p.now().Add(ttl)}
		p.mu.Unlock()
		if p.disk != nil {
			_ = p.disk.Put(ud.List, dir, items, ttl) // best-effort
		}
	}
	return items, nil
}

// runLister executes command via the shell in dir and returns non-empty stdout
// lines.
func runLister(dir, command string) ([]string, error) {
	res := shellcmd.Run(command, shellcmd.Options{Dir: dir})
	if res.Err != nil {
		return nil, res.Err
	}
	var lines []string
	for _, l := range strings.Split(res.Stdout, "\n") {
		if s := strings.TrimSpace(l); s != "" {
			lines = append(lines, s)
		}
	}
	return lines, nil
}

func parseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

func joinGit(dir string) string { return filepath.Join(dir, ".git") }

// pathExists reports whether p exists as a file or directory (.git is a file
// in worktrees and submodules).
func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// gitEntries returns a small set of contextual git actions offered inside a
// repository.
func gitEntries() []entry.Entry {
	specs := []struct{ desc, tmpl string }{
		{"git status", "git status"},
		{"git pull --rebase", "git pull --rebase"},
		{"git push", "git push"},
		{"git fetch --all --prune", "git fetch --all --prune"},
	}
	out := make([]entry.Entry, 0, len(specs))
	for _, s := range specs {
		out = append(out, entry.Entry{
			Title:    s.desc,
			Tags:     []string{"git", "project"},
			Template: s.tmpl,
			Kind:     entry.CommandLiteral,
			Source:   entry.SourceProject,
		})
	}
	return out
}
