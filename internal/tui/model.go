package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/rank"
	"github.com/liliumapps/commando/internal/resolve"
)

// stage is the current screen of the TUI.
type stage int

const (
	stageList stage = iota // browsing/searching the entry list
	stageForm              // filling in a selected entry's variables
)

type formAction int

const (
	formActionRun formAction = iota
	formActionBookmark
)

type fieldDataMsg struct {
	formID   uint64
	field    int
	request  uint64
	resolved map[string]string
	data     FieldData
	// cancelled marks a fetch whose context was cancelled (the user moved on
	// or a newer fetch replaced it). Its data is not authoritative.
	cancelled bool
}

type previewDataMsg struct {
	formID  uint64
	field   int
	request uint64
	data    PreviewData
}

// Options configure a Model beyond its entries and backend.
type Options struct {
	// HistoryListLimit caps how many history entries appear when the query is
	// empty. Zero or negative means no cap. A search always shows all matching
	// history regardless of this.
	HistoryListLimit int
	// Status is an optional non-fatal startup notice, such as provider
	// diagnostics collected while entries were loaded.
	Status string
}

// Model is the root Bubble Tea model. After the program exits, Result holds the
// assembled command (empty if the user aborted) and Aborted reports whether the
// user quit without choosing.
type Model struct {
	backend Backend
	styles  styles
	opts    Options

	stage stage

	// list state
	allEntries []entry.Entry
	filtered   []entry.Entry
	query      string
	queryInput textinput.Model
	listCursor int

	// form state
	form    *formModel
	formErr string
	formAct formAction
	formID  uint64

	fieldCancel   context.CancelFunc
	previewCancel context.CancelFunc

	// terminal size
	width  int
	height int

	// status is a transient one-line message shown under the list (e.g. after
	// bookmarking an entry).
	status string

	// pendingDelete is the bookmark template armed for deletion by a first
	// Ctrl-D; a second Ctrl-D on the same row deletes it. Any other key
	// disarms it.
	pendingDelete string

	// clearBeforeRender forces a full terminal clear before the next rendered
	// view. This is used at stage boundaries where some terminals/recorders do
	// not discard rows from the previous stage reliably.
	clearBeforeRender bool

	// outputs
	Result   string
	ResultID string
	Aborted  bool
}

// New creates a Model over the given entries and backend with default options.
func New(entries []entry.Entry, backend Backend) *Model {
	return NewWithOptions(entries, backend, Options{})
}

// NewWithOptions creates a Model with explicit options.
func NewWithOptions(entries []entry.Entry, backend Backend, opts Options) *Model {
	if backend == nil {
		backend = nopBackend{}
	}
	m := &Model{
		backend:    backend,
		styles:     defaultStyles(),
		opts:       opts,
		stage:      stageList,
		allEntries: entries,
		queryInput: newTextInput(""),
		status:     opts.Status,
		Aborted:    true, // until a command is produced
	}
	m.refilter()
	return m
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case fieldDataMsg:
		return m.applyFieldData(msg)
	case previewDataMsg:
		return m.applyPreviewData(msg)
	case tea.KeyMsg:
		if m.stage == stageList {
			return m.updateList(msg)
		}
		return m.updateForm(msg)
	}
	return m, nil
}

// --- list stage ---

func (m *Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type != tea.KeyCtrlD && m.pendingDelete != "" {
		m.pendingDelete = ""
		m.status = ""
	}
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		m.Aborted = true
		return m, tea.Quit
	case tea.KeyUp, tea.KeyCtrlP:
		// The list is drawn reversed (best match at the bottom, next to the
		// input), so moving up the screen means a higher index.
		if m.listCursor < len(m.filtered)-1 {
			m.listCursor++
		}
		return m, nil
	case tea.KeyDown, tea.KeyCtrlN:
		// Moving down the screen goes toward the best match (index 0) by the
		// input.
		if m.listCursor > 0 {
			m.listCursor--
		}
		return m, nil
	case tea.KeyEnter:
		return m.selectEntry()
	case tea.KeyCtrlB:
		return m.bookmarkCurrent()
	case tea.KeyCtrlD:
		return m.removeCurrentBookmark()
	case tea.KeyTab:
		// Tab cycles the source scope by rewriting the ":x" prefix in the query
		// itself, so the query text stays the single source of truth. Shift+Tab
		// cycles the other way.
		m.cycleScope(rank.Scope.Next)
		return m, nil
	case tea.KeyShiftTab:
		m.cycleScope(rank.Scope.Prev)
		return m, nil
	}

	old := m.queryInput.Value()
	var cmd tea.Cmd
	m.queryInput, cmd = m.queryInput.Update(normalizeTextInputKey(msg))
	m.query = m.queryInput.Value()
	if m.query != old {
		// A new query re-ranks the list: start from its best match so Enter
		// never picks a stale row.
		m.listCursor = 0
		m.refilter()
	}
	return m, cmd
}

func (m *Model) removeCurrentBookmark() (tea.Model, tea.Cmd) {
	if len(m.filtered) == 0 {
		return m, nil
	}
	current := m.filtered[m.listCursor]
	if current.Source != entry.SourceBookmark {
		m.status = "delete is available for bookmarks"
		return m, nil
	}
	if m.pendingDelete != current.Template {
		m.pendingDelete = current.Template
		m.status = "press ^d again to delete bookmark: " + current.Template
		return m, nil
	}
	m.pendingDelete = ""
	if err := m.backend.RemoveBookmark(current.Template); err != nil {
		m.status = "remove failed: " + err.Error()
		return m, nil
	}
	kept := m.allEntries[:0]
	for _, e := range m.allEntries {
		if e.Source == entry.SourceBookmark && e.Template == current.Template {
			continue
		}
		kept = append(kept, e)
	}
	m.allEntries = kept
	m.refilter()
	m.status = "removed bookmark: " + current.Template
	return m, nil
}

// cycleScope advances the active scope using the given step (Scope.Next or
// Scope.Prev), preserving the current search text, and rewrites the query so its
// ":x" prefix reflects the new scope. Filtering then re-derives everything from
// the query text.
func (m *Model) cycleScope(step func(rank.Scope) rank.Scope) {
	scope, search := rank.ParseScope(m.query)
	m.query = rank.WithScope(step(scope), search)
	m.queryInput.SetValue(m.query)
	m.queryInput.CursorEnd()
	m.listCursor = 0
	m.refilter()
}

func (m *Model) refilter() {
	q := strings.TrimSpace(m.query)
	m.filtered = filterEntries(m.allEntries, q)
	// In the idle default view (no scope prefix and no search text) cap how many
	// history entries are shown so the list stays focused on curated commands.
	// Any query — a search or an explicit scope like ":h" — shows all matches.
	scope, search := rank.ParseScope(q)
	if scope == rank.ScopeAll && search == "" {
		m.filtered = capHistory(m.filtered, m.opts.HistoryListLimit)
	}
	if m.listCursor >= len(m.filtered) {
		m.listCursor = max(0, len(m.filtered)-1)
	}
}

// selectEntry transitions from the list to the form for the highlighted entry.
// bookmarkCurrent saves a literal command immediately. A parameterized command
// enters the form and bookmarks the resolved command, never the unusable raw
// template.
func (m *Model) bookmarkCurrent() (tea.Model, tea.Cmd) {
	if len(m.filtered) == 0 {
		return m, nil
	}
	e := m.filtered[m.listCursor]
	if e.Source == entry.SourceBookmark {
		m.status = "already a bookmark"
		return m, nil
	}

	if e.Kind == entry.CommandLiteral {
		return m.saveBookmark(e.Template)
	}

	plan, err := resolve.NewPlan(e)
	if err != nil {
		m.status = "bookmark failed: " + err.Error()
		return m, nil
	}
	if !plan.HasVars() {
		cmd, err := plan.Assemble(nil)
		if err != nil {
			m.status = "bookmark failed: " + err.Error()
			return m, nil
		}
		return m.saveBookmark(cmd)
	}

	m.form = newFormModel(e, plan)
	m.formErr = ""
	m.formAct = formActionBookmark
	m.formID++
	m.stage = stageForm
	m.clearBeforeRender = true
	cmd := m.loadCurrentField(false)
	return m, cmd
}

func (m *Model) saveBookmark(cmd string) (tea.Model, tea.Cmd) {
	if err := m.backend.Bookmark(cmd); err != nil {
		m.status = "bookmark failed: " + err.Error()
		return m, nil
	}
	m.addBookmarkEntry(cmd)
	m.status = "bookmarked: " + cmd
	return m, nil
}

func (m *Model) addBookmarkEntry(cmd string) {
	for _, e := range m.allEntries {
		if e.Source == entry.SourceBookmark && e.Template == cmd {
			return
		}
	}
	m.allEntries = append(m.allEntries, entry.Entry{
		Title:    cmd,
		Template: cmd,
		Kind:     entry.CommandLiteral,
		Source:   entry.SourceBookmark,
	})
	m.refilter()
}

// Entries with no variables are assembled and returned immediately.
func (m *Model) selectEntry() (tea.Model, tea.Cmd) {
	if len(m.filtered) == 0 {
		return m, nil
	}
	e := m.filtered[m.listCursor]
	if e.Kind == entry.CommandLiteral {
		m.Result = e.Template
		m.ResultID = e.ID()
		m.Aborted = false
		return m, tea.Quit
	}
	plan, err := resolve.NewPlan(e)
	if err != nil {
		// A malformed template should not have reached here (cheats are
		// validated), but degrade gracefully by returning the raw template.
		m.Result = e.Template
		m.ResultID = e.ID()
		m.Aborted = false
		return m, tea.Quit
	}
	if !plan.HasVars() {
		cmd, err := plan.Assemble(nil)
		if err != nil {
			cmd = e.Template
		}
		m.Result = cmd
		m.ResultID = e.ID()
		m.Aborted = false
		return m, tea.Quit
	}
	m.form = newFormModel(e, plan)
	m.formErr = ""
	m.formAct = formActionRun
	m.formID++
	m.stage = stageForm
	m.clearBeforeRender = true
	cmd := m.loadCurrentField(false)
	return m, cmd
}

// --- form stage ---

func (m *Model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	fs := m.form.cur()
	switch msg.Type {
	case tea.KeyCtrlC:
		m.cancelFormWork()
		m.Aborted = true
		return m, tea.Quit
	case tea.KeyEsc:
		// Esc returns to the list.
		m.cancelFormWork()
		m.stage = stageList
		m.form = nil
		m.clearBeforeRender = true
		return m, tea.ClearScreen
	case tea.KeyCtrlR:
		if fs.canRefresh() {
			cmd := m.loadCurrentField(true)
			return m, cmd
		}
		return m, nil
	case tea.KeyCtrlP:
		// Move to the previous parameter without using Ctrl+Left, which is a
		// macOS window/space shortcut on many setups.
		m.form.prevField()
		m.formErr = ""
		cmd := m.loadCurrentField(false)
		return m, cmd
	case tea.KeyCtrlN:
		// Move to the next parameter.
		m.form.nextField()
		m.formErr = ""
		cmd := m.loadCurrentField(false)
		return m, cmd
	case tea.KeyTab:
		// Tab jumps to the next required parameter (skipping optionals) — the
		// canonical "next required field" key.
		m.form.nextRequiredField()
		m.formErr = ""
		cmd := m.loadCurrentField(false)
		return m, cmd
	case tea.KeyShiftTab:
		// Shift+Tab jumps to the previous required parameter.
		m.form.prevRequiredField()
		m.formErr = ""
		cmd := m.loadCurrentField(false)
		return m, cmd
	case tea.KeyUp:
		// Candidates are drawn reversed (index 0 at the bottom, by the input),
		// so moving up the screen means a higher index. displayCandidates
		// includes the synthetic typed row (index 0) when present, so ↑ from a
		// free-typed input lands on the first real candidate. When nothing is
		// highlighted yet, the first press only highlights the bottom row
		// (index 0) instead of skipping past it.
		candidates := fs.displayCandidates()
		if len(candidates) > 0 {
			wasHighlighted := fs.cursorHighlighted()
			fs.navigated = true
			if wasHighlighted && fs.cursor < len(candidates)-1 {
				fs.cursor++
			}
		}
		cmd := m.loadPreview()
		return m, cmd
	case tea.KeyDown:
		// Moving down the screen goes toward the bottom-most candidate (index 0).
		if len(fs.displayCandidates()) > 0 {
			wasHighlighted := fs.cursorHighlighted()
			fs.navigated = true
			if wasHighlighted && fs.cursor > 0 {
				fs.cursor--
			}
		}
		cmd := m.loadPreview()
		return m, cmd
	case tea.KeySpace:
		// For a multi-select param, Space toggles the highlighted candidate
		// on/off (checkbox style). For a single-value param, Space is an ordinary
		// character typed into the value.
		if fs.field.Spec.Multi {
			vis := fs.displayCandidates()
			if len(vis) > 0 && fs.cursor >= 0 && fs.cursor < len(vis) {
				fs.touched = true
				fs.skipped = false
				fs.toggleSelected(vis[fs.cursor].Value)
			}
			cmd := m.loadPreview()
			return m, cmd
		}
		return m.updateFieldInput(fs, msg)
	case tea.KeyCtrlD:
		// Clear the current parameter's entire value.
		fs.clear()
		m.formErr = ""
		cmd := m.loadPreview()
		return m, cmd
	case tea.KeyCtrlS:
		if fs.canSkip() {
			fs.skipped = true
			fs.setInput("")
			m.formErr = ""
		}
		cmd := m.loadPreview()
		return m, cmd
	case tea.KeyEnter:
		// Enter commits this parameter's value (adopting the highlighted
		// candidate for a quick pick), then moves to the NEXT parameter. On the
		// last parameter it runs the command.
		fs.commit()
		return m.advanceOrRun()
	}
	return m.updateFieldInput(fs, msg)
}

func (m *Model) updateFieldInput(fs *fieldState, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	old := fs.input
	var cmd tea.Cmd
	fs.editor, cmd = fs.editor.Update(normalizeTextInputKey(msg))
	fs.syncInputFromEditor()
	if fs.input != old {
		fs.skipped = false
		fs.touched = true
		fs.adopted = false
		fs.cursor = 0
		fs.navigated = false
	}
	return m, tea.Batch(cmd, m.loadPreview())
}

func (m *Model) loadCurrentField(force bool) tea.Cmd {
	if m.form == nil {
		return nil
	}
	index := m.form.current
	fs := m.form.cur()
	resolved := m.form.rawValues()
	if !force && !fs.needsFetch(resolved) {
		return m.loadPreview()
	}
	if m.fieldCancel != nil {
		m.fieldCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.fieldCancel = cancel
	if force {
		ctx = WithRefresh(ctx)
	}

	fs.request++
	request := fs.request
	fs.loading = true
	fs.loadError = ""
	formID := m.formID
	e := m.form.entry
	field := fs.field
	snapshot := cloneStrings(resolved)
	return func() tea.Msg {
		data := m.backend.FieldData(ctx, e, field, snapshot)
		return fieldDataMsg{
			formID:    formID,
			field:     index,
			request:   request,
			resolved:  snapshot,
			data:      data,
			cancelled: ctx.Err() != nil,
		}
	}
}

func (m *Model) applyFieldData(msg fieldDataMsg) (tea.Model, tea.Cmd) {
	if m.form == nil || msg.formID != m.formID ||
		msg.field < 0 || msg.field >= len(m.form.fields) {
		return m, nil
	}
	fs := m.form.fields[msg.field]
	if msg.request != fs.request {
		return m, nil
	}
	if msg.cancelled {
		// A cancelled fetch may have returned partial or empty data. Leave the
		// field unloaded so it refetches when revisited.
		fs.loading = false
		return m, nil
	}
	fs.applyData(msg.data, msg.resolved)
	if msg.field != m.form.current {
		return m, nil
	}
	cmd := m.loadPreview()
	return m, cmd
}

func (m *Model) loadPreview() tea.Cmd {
	if m.form == nil {
		return nil
	}
	if m.previewCancel != nil {
		m.previewCancel()
		m.previewCancel = nil
	}
	m.form.previewRequest++
	request := m.form.previewRequest
	m.form.previewLoading = false

	fs := m.form.cur()
	candidates := fs.displayCandidates()
	if fs.field.Spec.Preview == "" || !fs.cursorHighlighted() ||
		fs.cursor < 0 || fs.cursor >= len(candidates) {
		m.form.previewData = PreviewData{}
		return nil
	}
	// The previous preview (if any) stays on screen until the new one arrives,
	// so the pane does not collapse to a one-line "loading" notice and back.

	ctx, cancel := context.WithCancel(context.Background())
	m.previewCancel = cancel
	m.form.previewLoading = true
	formID := m.formID
	fieldIndex := m.form.current
	e := m.form.entry
	field := fs.field
	candidate := candidates[fs.cursor].Value
	resolved := cloneStrings(m.form.rawValues())
	return func() tea.Msg {
		timer := time.NewTimer(75 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return previewDataMsg{formID: formID, field: fieldIndex, request: request}
		case <-timer.C:
		}
		data := m.backend.Preview(ctx, e, field, candidate, resolved)
		return previewDataMsg{
			formID:  formID,
			field:   fieldIndex,
			request: request,
			data:    data,
		}
	}
}

func (m *Model) applyPreviewData(msg previewDataMsg) (tea.Model, tea.Cmd) {
	if m.form == nil || msg.formID != m.formID ||
		msg.field != m.form.current || msg.request != m.form.previewRequest {
		return m, nil
	}
	m.form.previewData = msg.data
	m.form.previewLoading = false
	return m, nil
}

func (m *Model) cancelFormWork() {
	if m.fieldCancel != nil {
		m.fieldCancel()
		m.fieldCancel = nil
	}
	if m.previewCancel != nil {
		m.previewCancel()
		m.previewCancel = nil
	}
}

func cloneStrings(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// advanceOrRun handles Enter: validate the current parameter, then move to the
// next parameter linearly — or run the command when on the last parameter.
func (m *Model) advanceOrRun() (tea.Model, tea.Cmd) {
	fs := m.form.cur()
	if msg := fs.validationError(); msg != "" {
		m.formErr = msg
		return m, nil
	}
	if m.form.advanceField() {
		m.formErr = ""
		cmd := m.loadCurrentField(false)
		return m, cmd
	}
	return m.run()
}

// run assembles and executes the command. If a required parameter is still
// empty it does not run; instead it focuses the first unmet required parameter
// so the user can fill it in (with an explanatory error).
//
// Before running, every field is re-checked: a field whose candidates are
// stale (a dependency changed, or a strict field never loaded) is refetched
// first, and strict, pattern, and required rules are validated for all fields.
// Focus jumps to the first failing field with its error.
func (m *Model) run() (tea.Model, tea.Cmd) {
	if stale := m.form.firstStale(); stale >= 0 {
		m.form.cur().commitOnLeave()
		m.form.gotoField(stale)
		m.formErr = "re-checking " + m.form.fields[stale].field.Name + ": its candidates may have changed"
		cmd := m.loadCurrentField(false)
		return m, cmd
	}
	if idx, msg := m.form.firstInvalid(); idx >= 0 {
		if idx != m.form.current {
			m.form.cur().commitOnLeave()
			m.form.gotoField(idx)
		}
		m.formErr = msg
		cmd := m.loadCurrentField(false)
		return m, cmd
	}
	cmd, err := m.form.assemble()
	if err != nil {
		m.formErr = err.Error()
		return m, nil
	}
	m.formErr = ""

	// Bookmarking saves the assembled command only; it does not count as using
	// the values, so nothing is remembered.
	if m.formAct == formActionBookmark {
		if err := m.backend.Bookmark(cmd); err != nil {
			m.formErr = "bookmark failed: " + err.Error()
			return m, nil
		}
		m.addBookmarkEntry(cmd)
		m.status = "bookmarked: " + cmd
		m.cancelFormWork()
		m.stage = stageList
		m.form = nil
		m.clearBeforeRender = true
		return m, tea.ClearScreen
	}

	m.persist()
	m.Result = cmd
	m.ResultID = m.form.entry.ID()
	m.Aborted = false
	m.cancelFormWork()
	return m, tea.Quit
}

// persist stores each resolved field value through the backend for frecency.
func (m *Model) persist() {
	for _, fs := range m.form.fields {
		if fs.skipped {
			continue
		}
		// Variables remember by default; only an explicit remember="none" opts
		// out (Remembers() resolves the unset default to global).
		if !fs.field.Spec.Remember.Remembers() {
			continue
		}
		if fs.field.Spec.Multi {
			for _, v := range fs.selected {
				m.backend.Remember(m.form.entry, fs.field, v)
			}
			continue
		}
		if fs.input != "" {
			m.backend.Remember(m.form.entry, fs.field, fs.input)
		}
	}
}
