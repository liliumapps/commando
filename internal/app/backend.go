package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/liliumapps/commando/internal/bookmark"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/persist"
	"github.com/liliumapps/commando/internal/provider"
	"github.com/liliumapps/commando/internal/rank"
	"github.com/liliumapps/commando/internal/resolve"
	"github.com/liliumapps/commando/internal/shellcmd"
	"github.com/liliumapps/commando/internal/state"
	"github.com/liliumapps/commando/internal/tui"
)

// fileBackend implements tui.Backend using the file-based state store for memory
// and the shell for a variable's Src candidate command.
type fileBackend struct {
	state     *state.State
	bookmarks *bookmark.Store
	pctx      provider.Context
	dataDir   string // exposed to src commands as COMMANDO_DATA_DIR

	// previewCache memoizes Preview output by (field, candidate, command) so
	// moving the cursor over candidates does not re-run the preview command.
	previewCache map[string]tui.PreviewData

	// srcCache memoizes a variable's raw Src output for specs that set a Cache
	// duration, keyed by the resolved command, so a slow candidate command is
	// not re-run on every re-fetch within a session.
	srcCache map[string]srcCacheEntry
	// disk persists srcCache across launches, keyed by (command, cwd); nil
	// keeps caching in-memory only.
	disk    *persist.DiskCache
	now     func() time.Time
	cacheMu sync.Mutex

	// redact reports whether text looks like it carries a secret (the
	// history_ignore patterns plus value-shaped token detectors). Such values
	// are never remembered and such commands are never bookmarked from the
	// TUI. nil disables the check.
	redact func(string) bool

	// matchers caches the compiled history matcher per cheat (guarded by
	// cacheMu); a nil value records a template that cannot be matched.
	matchers map[string]*fillMatcher
}

// ErrSecretBookmark is returned when the TUI asks to bookmark a command that
// looks like it contains a secret.
var ErrSecretBookmark = errors.New("not bookmarked: the command looks like it contains a secret (see history_ignore)")

// srcCacheEntry is a cached raw Src output with its expiry.
type srcCacheEntry struct {
	raw       string
	expires   time.Time
	truncated bool
}

// Bookmark saves cmd as a bookmark via the backing store.
// A command that looks secret is refused with ErrSecretBookmark (shown by the
// TUI) rather than written to bookmarks.toml in plain text.
func (b *fileBackend) Bookmark(cmd string) error {
	if b.bookmarks == nil {
		return nil
	}
	if b.redact != nil && strings.TrimSpace(cmd) != "" && b.redact(cmd) {
		return ErrSecretBookmark
	}
	return b.bookmarks.Add(bookmark.Bookmark{Cmd: cmd, Created: time.Now().Unix()})
}

func (b *fileBackend) RemoveBookmark(cmd string) error {
	if b.bookmarks == nil {
		return nil
	}
	_, err := b.bookmarks.Remove(cmd)
	return err
}

// FieldData assembles the candidate list for a field: remembered values first
// (frecency-ranked), then values produced by the Src command, de-duplicated.
// The prefill is the top remembered value, falling back to the spec default.
// resolved holds values already chosen for other variables; any <name>
// placeholders in the Src command are substituted from it before the command
// runs, enabling cross-variable candidate lookups.
func (b *fileBackend) FieldData(ctx context.Context, e entry.Entry, f resolve.Field, resolved map[string]string) tui.FieldData {
	scope := b.scopeKey(f.Spec.Remember)

	// Remembered (previously used) values carry their usage data so the ranker
	// can order them most-recently-used first.
	var remembered []rank.Candidate
	if scope != "" && b.state != nil {
		for _, v := range b.state.Recall(e.ID(), f.Name, scope) {
			remembered = append(remembered, rank.Candidate{
				Value:    v.Value,
				LastUsed: v.LastN,
				UseCount: v.Freq,
			})
		}
	}

	srcCands, issue, truncated := b.srcCandidates(ctx, f.Spec, resolved)

	candidates := rank.BuildCandidates(remembered, srcCands)

	// Only an author-defined default prefills the input. Remembered values are
	// offered as candidates (most-recent first), not auto-typed, so the input
	// starts empty unless the cheat set a default.
	return tui.FieldData{
		Candidates: candidates,
		Prefill:    f.Spec.Default,
		Error:      issue,
		Truncated:  truncated,
	}
}

// Preview runs the field's Preview command for a highlighted candidate and
// returns its combined output. "{}" is replaced by the candidate value; other
// <vars> are substituted from resolved. Results are cached per (field, resolved
// deps, candidate) so repeated cursor movement does not re-run the command.
func (b *fileBackend) Preview(ctx context.Context, e entry.Entry, f resolve.Field, candidate string, resolved map[string]string) tui.PreviewData {
	if strings.TrimSpace(f.Spec.Preview) == "" || candidate == "" {
		return tui.PreviewData{}
	}
	// Substitute cross-variable refs, then the {} candidate placeholder.
	cmd := resolve.SubstituteShellRefs(f.Spec.Preview, resolved)
	cmd = strings.ReplaceAll(cmd, "{}", resolve.ShellQuote(candidate))

	key := f.Name + "\x00" + candidate + "\x00" + cmd
	b.cacheMu.Lock()
	if b.previewCache != nil {
		if v, ok := b.previewCache[key]; ok {
			b.cacheMu.Unlock()
			return v
		}
	}
	b.cacheMu.Unlock()

	res := b.execShell(ctx, cmd)
	data := tui.PreviewData{
		Text:      res.Stdout,
		Truncated: res.Truncated,
	}
	if res.Err != nil {
		data.Error = shellIssue(res)
	}
	if ctx.Err() != nil {
		return tui.PreviewData{}
	}

	b.cacheMu.Lock()
	if b.previewCache == nil {
		b.previewCache = make(map[string]tui.PreviewData)
	}
	b.previewCache[key] = data
	b.cacheMu.Unlock()
	return data
}

// Remember persists a chosen value at the field's scope. A value that looks
// like a secret — by itself, or as "<var>=<value>" so a variable named
// `password` or `token` is caught by the ignore patterns — is not stored.
func (b *fileBackend) Remember(e entry.Entry, f resolve.Field, value string) {
	if b.state == nil {
		return
	}
	if b.secretValue(f.Name, value) {
		return
	}
	scope := b.scopeKey(f.Spec.Remember)
	if scope == "" {
		return
	}
	_ = b.state.Remember(e.ID(), f.Name, scope, value)
}

// scopeKey converts a VarSpec scope into a concrete storage key incorporating
// the relevant path so that project/dir scopes are isolated correctly. An unset
// scope resolves to the default (global), so variables remember by default; an
// explicit "none" scope returns "" to disable memory.
func (b *fileBackend) scopeKey(s entry.Scope) string {
	switch s.Effective() {
	case entry.ScopeGlobal:
		return "global"
	case entry.ScopeProject:
		root := b.pctx.GitRoot
		if root == "" {
			root = b.pctx.Cwd
		}
		return "project:" + root
	case entry.ScopeDir:
		return "dir:" + b.pctx.Cwd
	default: // ScopeNone
		return ""
	}
}

// srcCandidates builds the ordered candidate list for a field from its inline
// Values (first) and its Src command output (second), applying the spec's
// extraction (header lines, columns/match, label/value) to each. resolved
// supplies values already chosen for other variables so <name> refs in Src are
// substituted before it runs.
func (b *fileBackend) srcCandidates(ctx context.Context, spec entry.VarSpec, resolved map[string]string) ([]rank.Candidate, string, bool) {
	var cands []rank.Candidate

	// Inline values are treated as source lines: extraction applies to them too,
	// but HeaderLines does not (they are already clean).
	if len(spec.Values) > 0 {
		noHeader := spec
		noHeader.HeaderLines = 0
		cands = append(cands, projectLines(spec.Values, noHeader)...)
	}

	src := resolve.SubstituteShellRefs(spec.Src, resolved)
	if strings.TrimSpace(src) != "" {
		raw, res, ok := b.runSrcRawContext(ctx, src, spec.Cache)
		if ok {
			cands = append(cands, extractCandidates(raw, spec)...)
		} else if ctx.Err() == nil {
			return cands, shellIssue(res), res.Truncated
		}
		return cands, "", res.Truncated
	}
	return cands, "", false
}

// runSrcRaw executes a Src command and returns its raw stdout, memoizing the
// result for the spec's Cache duration when set. A failing command yields ok
// false. The cache key is the fully-resolved command so distinct dependency
// values do not collide.
func (b *fileBackend) runSrcRaw(src, cache string) (string, bool) {
	raw, _, ok := b.runSrcRawContext(context.Background(), src, cache)
	return raw, ok
}

func (b *fileBackend) runSrcRawContext(ctx context.Context, src, cache string) (string, shellcmd.Result, bool) {
	ttl := parseCacheDuration(cache)
	if ttl > 0 && !tui.IsRefresh(ctx) {
		if ce, ok := b.cachedSrc(src, ttl); ok {
			res := shellcmd.Result{Stdout: ce.raw, Truncated: ce.truncated}
			return ce.raw, res, true
		}
	}

	res := b.execShell(ctx, src)
	if res.Err != nil {
		return "", res, false
	}
	if ttl > 0 {
		ce := srcCacheEntry{raw: res.Stdout, expires: b.clock().Add(ttl), truncated: res.Truncated}
		b.cacheMu.Lock()
		if b.srcCache == nil {
			b.srcCache = make(map[string]srcCacheEntry)
		}
		b.srcCache[src] = ce
		b.cacheMu.Unlock()
		// Best-effort: an unwritable cache only costs a re-run next launch.
		_ = b.disk.Put(src, b.pctx.Cwd, diskSrcRecord{Raw: ce.raw, Truncated: ce.truncated}, ttl)
	}
	return res.Stdout, res, true
}

// diskSrcRecord is the persisted form of a cached Src output.
type diskSrcRecord struct {
	Raw       string `json:"raw"`
	Truncated bool   `json:"truncated,omitempty"`
}

// cachedSrc looks up a cached Src output: the in-memory layer first, then the
// disk cache (keyed by the command and working directory), promoting a disk
// hit into memory.
func (b *fileBackend) cachedSrc(src string, ttl time.Duration) (srcCacheEntry, bool) {
	b.cacheMu.Lock()
	if ce, ok := b.srcCache[src]; ok && b.clock().Before(ce.expires) {
		b.cacheMu.Unlock()
		return ce, true
	}
	b.cacheMu.Unlock()

	var rec diskSrcRecord
	expires, ok := b.disk.Get(src, b.pctx.Cwd, ttl, &rec)
	if !ok {
		return srcCacheEntry{}, false
	}
	ce := srcCacheEntry{raw: rec.Raw, expires: expires, truncated: rec.Truncated}
	b.cacheMu.Lock()
	if b.srcCache == nil {
		b.srcCache = make(map[string]srcCacheEntry)
	}
	b.srcCache[src] = ce
	b.cacheMu.Unlock()
	return ce, true
}

// clock returns the backend's time source, defaulting to time.Now.
func (b *fileBackend) clock() time.Time {
	if b.now != nil {
		return b.now()
	}
	return time.Now()
}

// parseCacheDuration parses a Go duration string, returning 0 (no caching) for
// an empty or malformed value.
func parseCacheDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

// execShell runs a shell command in the provider's working directory with
// COMMANDO_DATA_DIR exported, returning its combined stdout.
func (b *fileBackend) execShell(ctx context.Context, command string) shellcmd.Result {
	var env []string
	if b.dataDir != "" {
		// Expose the cheat data directory so commands can read shipped data
		// files (e.g. `cat "$COMMANDO_DATA_DIR/hosts.txt"`).
		env = append(env, "COMMANDO_DATA_DIR="+b.dataDir)
	}
	return shellcmd.RunContext(ctx, command, shellcmd.Options{
		Dir: b.pctx.Cwd,
		Env: env,
	})
}

func shellIssue(res shellcmd.Result) string {
	if stderr := strings.TrimSpace(res.Stderr); stderr != "" {
		return stderr
	}
	if res.Err != nil {
		return res.Err.Error()
	}
	return "command failed"
}

// Compile-time check that fileBackend offers recent fills to the TUI.
var _ tui.FillsBackend = (*fileBackend)(nil)

// maxHistoryFills caps how many history-only fills are offered per form.
const maxHistoryFills = 20

// maxHistoryLines caps how many history lines are matched per form open.
const maxHistoryLines = 5000

// fillsEligible reports whether e records and offers recent fills: authored
// cheat templates only (history, bookmarks, and project commands are literal).
func fillsEligible(e entry.Entry) bool {
	return e.Source == entry.SourceCheat && e.Kind == entry.CommandTemplate
}

// fillPlan returns the entry's plan and the fields whose values take part in
// fills (not multi, not remember="none"), or nil when fills do not apply.
func fillPlan(e entry.Entry) (plan *resolve.Plan, fields map[string]resolve.Field) {
	if !fillsEligible(e) {
		return nil, nil
	}
	plan, err := resolve.NewPlan(e)
	if err != nil || !plan.HasVars() {
		return nil, nil
	}
	fields = make(map[string]resolve.Field)
	for _, f := range plan.Fields {
		if !f.Spec.Multi && f.Spec.Remember.Remembers() {
			fields[f.Name] = f
		}
	}
	if len(fields) == 0 {
		return nil, nil
	}
	return plan, fields
}

// fillScopeKey is the storage scope of an entry's fills: the narrowest scope
// among its remembered variables (dir, then project, then global).
func (b *fileBackend) fillScopeKey(plan *resolve.Plan) string {
	narrowest := entry.ScopeNone
	order := map[entry.Scope]int{entry.ScopeNone: 0, entry.ScopeGlobal: 1, entry.ScopeProject: 2, entry.ScopeDir: 3}
	for _, f := range plan.Fields {
		s := f.Spec.Remember.Effective()
		if order[s] > order[narrowest] {
			narrowest = s
		}
	}
	return b.scopeKey(narrowest)
}

// secretNameParts mark a variable whose values are secrets by name alone.
var secretNameParts = []string{"password", "passwd", "passphrase", "secret", "token", "apikey", "api_key", "api-key", "private_key", "privatekey", "credential"}

// secretValue reports whether a variable's value must not be stored: the value
// itself looks secret (denylist or built-in detectors), or the variable's name
// says it holds one. The name and value are not joined into "name=value"
// before matching, because history patterns such as `AWS_*=` would then flag
// ordinary variables like `aws_account`.
func (b *fileBackend) secretValue(name, value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	if b.redact != nil && b.redact(value) {
		return true
	}
	lower := strings.ToLower(name)
	for _, part := range secretNameParts {
		if strings.Contains(lower, part) {
			return true
		}
	}
	return false
}

// fillLooksSecret reports whether any value looks like a secret, by itself or
// as "<var>=<value>", the same test Remember applies to single values.
func (b *fileBackend) fillLooksSecret(values map[string]string) bool {
	for name, v := range values {
		if b.secretValue(name, v) {
			return true
		}
	}
	return false
}

// fillableValues keeps only the non-empty values of fill fields.
func fillableValues(values map[string]string, fields map[string]resolve.Field) map[string]string {
	out := make(map[string]string, len(values))
	for name, v := range values {
		if _, ok := fields[name]; ok && v != "" {
			out[name] = v
		}
	}
	return out
}

// RememberFill records a completed form as one fill. Values of multi and
// remember="none" variables are dropped; the whole record is skipped when any
// value looks like a secret.
func (b *fileBackend) RememberFill(e entry.Entry, values map[string]string) {
	if b.state == nil {
		return
	}
	plan, fields := fillPlan(e)
	if plan == nil {
		return
	}
	vals := fillableValues(values, fields)
	if len(vals) == 0 || b.fillLooksSecret(vals) {
		return
	}
	scope := b.fillScopeKey(plan)
	if scope == "" {
		return
	}
	_ = b.state.RememberFill(e.ID(), scope, vals)
}

// ForgetFill deletes a stored fill.
func (b *fileBackend) ForgetFill(e entry.Entry, values map[string]string) error {
	if b.state == nil {
		return nil
	}
	plan, _ := fillPlan(e)
	if plan == nil {
		return nil
	}
	scope := b.fillScopeKey(plan)
	if scope == "" {
		return nil
	}
	return b.state.ForgetFill(e.ID(), scope, values)
}

// RecentFills returns e's stored fills (frecency order) followed by fills
// found in history: lines (newest first, at most 5,000) that match the
// cheat's template, grouped by identical values. A history match equal to a
// stored fill is merged into it. Secret-looking lines and values are skipped.
func (b *fileBackend) RecentFills(ctx context.Context, e entry.Entry, history []entry.Entry) []tui.Fill {
	plan, fields := fillPlan(e)
	if plan == nil {
		return nil
	}
	var out []tui.Fill
	index := make(map[string]int)
	if scope := b.fillScopeKey(plan); scope != "" && b.state != nil {
		for _, f := range b.state.RecallFills(e.ID(), scope) {
			vals := fillableValues(f.Values, fields)
			if len(vals) == 0 || b.fillLooksSecret(vals) {
				continue
			}
			sig := state.FillSignature(vals)
			if _, dup := index[sig]; dup {
				continue
			}
			index[sig] = len(out)
			out = append(out, tui.Fill{Values: vals, Count: f.Count, LastUsed: f.LastN})
		}
	}

	fm := b.fillMatcherFor(e, plan, fields)
	if fm == nil {
		return out
	}
	histOnly := 0
	for i, h := range history {
		if i >= maxHistoryLines {
			break
		}
		if i%128 == 0 && ctx.Err() != nil {
			return nil
		}
		line := h.Template
		if b.redact != nil && b.redact(line) {
			continue
		}
		vals, ok := fm.match(line)
		if !ok || len(vals) == 0 || b.fillLooksSecret(vals) {
			continue
		}
		sig := state.FillSignature(vals)
		if at, seen := index[sig]; seen {
			if out[at].FromHistory {
				out[at].Count++
			}
			out[at].LastUsed = max(out[at].LastUsed, h.LastUsed)
			continue
		}
		if histOnly >= maxHistoryFills {
			continue
		}
		histOnly++
		index[sig] = len(out)
		out = append(out, tui.Fill{Values: vals, Count: 1, LastUsed: h.LastUsed, FromHistory: true})
	}
	return out
}

// fillMatcherFor returns the cached history matcher for e (nil when its
// template cannot be matched safely).
func (b *fileBackend) fillMatcherFor(e entry.Entry, plan *resolve.Plan, fields map[string]resolve.Field) *fillMatcher {
	key := e.ID() + "\x00" + e.Template
	b.cacheMu.Lock()
	defer b.cacheMu.Unlock()
	if fm, ok := b.matchers[key]; ok {
		return fm
	}
	multi := make(map[string]bool)
	for _, f := range plan.Fields {
		if f.Spec.Multi {
			multi[f.Name] = true
		}
	}
	fm := newFillMatcher(e.Template,
		func(name string) bool { _, ok := fields[name]; return ok },
		func(name string) bool { return multi[name] })
	if b.matchers == nil {
		b.matchers = make(map[string]*fillMatcher)
	}
	b.matchers[key] = fm
	return fm
}
