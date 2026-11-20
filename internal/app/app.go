package app

import (
	"fmt"
	"os"
	"sync"

	"github.com/liliumapps/commando/internal/bookmark"
	"github.com/liliumapps/commando/internal/config"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/persist"
	"github.com/liliumapps/commando/internal/provider"
	"github.com/liliumapps/commando/internal/provider/cheatfile"
	"github.com/liliumapps/commando/internal/provider/history"
	"github.com/liliumapps/commando/internal/provider/project"
	"github.com/liliumapps/commando/internal/state"
	"github.com/liliumapps/commando/internal/tui"
)

// App is commando's composition root. It owns the config, file-based state, and
// providers, and offers the high-level operations the CLI drives. All state is
// plain files — there is no database.
type App struct {
	Paths     config.Paths
	Config    *config.Config
	State     *state.State
	Bookmarks *bookmark.Store
	// Diagnostics contains non-fatal provider failures from the most recent
	// entry load.
	Diagnostics []Diagnostic

	providers []provider.Provider
	pctx      provider.Context
	// cache persists `cache`-enabled src and lister output across launches;
	// nil when no user cache directory is available.
	cache *persist.DiskCache
	// saveErrOnce limits best-effort save failures to one stderr report.
	saveErrOnce sync.Once
}

// Open initialises the application: resolves paths, seeds sample cheats on first
// run, loads config and file-based state, and constructs providers.
func Open() (*App, error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, err
	}
	if err := paths.EnsureDirs(); err != nil {
		return nil, err
	}
	// Seed sample cheats the first time, never clobbering user files.
	if _, err := config.SeedCheats(paths); err != nil {
		return nil, fmt.Errorf("seed cheats: %w", err)
	}

	cfg, err := config.Load(paths)
	if err != nil {
		return nil, err
	}

	st, err := state.Open(paths.DataDir)
	if err != nil {
		return nil, err
	}

	bookmarks := bookmark.Open(paths.BookmarksPath)
	pctx := buildContext()

	// Build the redaction predicate once so the history provider hides
	// secret-looking commands the same way capture used to.
	ignore := func(cmd string) bool { return config.ShouldIgnore(cfg.HistoryIgnore, cmd) }

	cache := persist.NewDiskCache(paths.CacheDir) // nil when unavailable
	projects := project.New(cfg.Providers)
	projects.SetDiskCache(cache)

	providers := []provider.Provider{
		bookmarks,
		cheatfile.New(paths),
		projects,
		history.New(
			history.WithIgnore(ignore),
			history.WithLimit(cfg.History.EffectiveMaxLoad()),
			history.WithMaxBytes(cfg.History.EffectiveMaxBytes()),
		),
	}

	return &App{
		Paths:     paths,
		Config:    cfg,
		State:     st,
		Bookmarks: bookmarks,
		providers: providers,
		pctx:      pctx,
		cache:     cache,
	}, nil
}

// Close flushes any pending batched state.
func (a *App) Close() error {
	if a.State == nil {
		return nil
	}
	return a.State.Close()
}

// Entries returns the aggregated, ranked entries from all providers.
func (a *App) Entries() ([]entry.Entry, error) {
	entries, diagnostics := aggregate(a.providers, a.pctx, a.State)
	a.Diagnostics = diagnostics
	return entries, nil
}

// Backend returns a TUI backend bound to this app's state and context.
func (a *App) Backend() tui.Backend {
	patterns := a.Config.HistoryIgnore
	return &fileBackend{
		state:     a.State,
		bookmarks: a.Bookmarks,
		pctx:      a.pctx,
		dataDir:   a.Paths.CheatDataDir,
		disk:      a.cache,
		redact:    func(s string) bool { return config.ShouldIgnore(patterns, s) },
	}
}

// reportSaveError prints the first best-effort save failure to stderr. Saves
// must never block returning the chosen command, but silently dropping them
// hides a broken data directory (or, on Windows, a locked file).
func (a *App) reportSaveError(err error) {
	if err == nil {
		return
	}
	a.saveErrOnce.Do(func() {
		fmt.Fprintf(os.Stderr, "commando: could not save state: %v\n", err)
	})
}

// Search runs the interactive TUI and returns the assembled command (empty when
// aborted). When the user commits a command, its entry usage is recorded so the
// entry ranks higher next time.
func (a *App) Search() (tui.Result, error) {
	entries, err := a.Entries()
	if err != nil {
		return tui.Result{Aborted: true}, err
	}
	a.State.BeginBatch()
	defer func() {
		a.reportSaveError(a.State.EndBatch())
	}()

	opts := tui.Options{HistoryListLimit: a.Config.History.EffectiveListLimit()}
	if len(a.Diagnostics) > 0 {
		opts.Status = fmt.Sprintf("%d source issue(s); run `commando doctor` for details", len(a.Diagnostics))
	}
	res, err := tui.Run(entries, a.Backend(), opts)
	if err == nil && !res.Aborted && res.EntryID != "" {
		// Usage tracking is best-effort; a write failure must not block the
		// chosen command from being returned, but it is reported once.
		a.reportSaveError(a.State.NoteUse(res.EntryID))
	}
	return res, err
}

// ProjectContext exposes the resolved provider context (cwd, git root).
func (a *App) ProjectContext() provider.Context { return a.pctx }

// HistoryEntries returns just the shell-history entries (newest first), for the
// `history` command. It reads the history file live via the history provider.
func (a *App) HistoryEntries() ([]entry.Entry, error) {
	ignore := func(cmd string) bool { return config.ShouldIgnore(a.Config.HistoryIgnore, cmd) }
	// The CLI applies its own result --limit after filtering, while the
	// configured load and byte bounds still protect startup on large histories.
	return history.New(
		history.WithIgnore(ignore),
		history.WithLimit(a.Config.History.EffectiveMaxLoad()),
		history.WithMaxBytes(a.Config.History.EffectiveMaxBytes()),
	).Entries(a.pctx)
}
