package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/liliumapps/commando/internal/entry"
)

// maxRecentRows caps how many recent fills are visible at once; more scroll.
const maxRecentRows = 5

// maxHistoryScan caps how many history commands are offered to RecentFills
// per form open (the newest ones).
const maxHistoryScan = 5000

// Fill is one complete set of values previously used for a cheat's form: a
// stored record of a run through commando, or a shell-history line that
// matched the cheat's template.
type Fill struct {
	// Values maps variable names to their values. Variables absent from the
	// map were skipped (optional) or are never remembered.
	Values map[string]string
	// Count is how often the set was used (stored runs, or matching history
	// lines for a history-only fill).
	Count int
	// LastUsed is the unix time of the latest use, or 0 when unknown.
	LastUsed int64
	// FromHistory marks a fill seen only in shell history. It is not stored,
	// so it cannot be deleted.
	FromHistory bool
}

// FillsBackend is an optional Backend extension for recent fills: whole
// completed forms offered back when a cheat's form opens. A Backend that does
// not implement it simply shows no Recent block.
type FillsBackend interface {
	// RecentFills returns the fills for e, best first: stored fills merged
	// with matches found in history (shell-history entries, newest first).
	// It runs off the UI goroutine and should honor ctx cancellation.
	RecentFills(ctx context.Context, e entry.Entry, history []entry.Entry) []Fill
	// RememberFill records a completed form. It is called once when the user
	// runs a command (not when bookmarking), with the remembered values of
	// every filled, non-multi variable.
	RememberFill(e entry.Entry, values map[string]string)
	// ForgetFill deletes a stored fill with exactly these values.
	ForgetFill(e entry.Entry, values map[string]string) error
}

type recentFillsMsg struct {
	formID uint64
	fills  []Fill
}

// recentBlock is the Recent row block shown above the first field.
type recentBlock struct {
	fills []Fill
	// cursor indexes fills; 0 (the best) is drawn at the bottom, by the input.
	cursor int
	// active means the block is shown and owns the arrow and Enter keys.
	active bool
	// dismissed is set once the block is hidden; it does not come back for
	// this form.
	dismissed bool
	// pendingDelete is the signature armed by a first Ctrl-D.
	pendingDelete string
	// hint is a transient note shown in the block header.
	hint string
}

// historyForFills returns the shell-history entries newest first, capped.
func historyForFills(entries []entry.Entry) []entry.Entry {
	var hist []entry.Entry
	for _, e := range entries {
		if e.Source == entry.SourceHistory {
			hist = append(hist, e)
		}
	}
	sort.SliceStable(hist, func(i, j int) bool { return hist[i].ProviderOrder > hist[j].ProviderOrder })
	if len(hist) > maxHistoryScan {
		hist = hist[:maxHistoryScan]
	}
	return hist
}

func fillSignature(values map[string]string) string {
	names := make([]string, 0, len(values))
	for k := range values {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, k := range names {
		b.WriteString(k + "=" + values[k] + "\x00")
	}
	return b.String()
}

// loadRecentFills fetches the current form's recent fills asynchronously.
func (m *Model) loadRecentFills() tea.Cmd {
	fb, ok := m.backend.(FillsBackend)
	if !ok || m.form == nil {
		return nil
	}
	if m.recentCancel != nil {
		m.recentCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.recentCancel = cancel
	formID := m.formID
	e := m.form.entry
	hist := m.history
	return func() tea.Msg {
		fills := fb.RecentFills(ctx, e, hist)
		if ctx.Err() != nil {
			return recentFillsMsg{formID: formID}
		}
		return recentFillsMsg{formID: formID, fills: fills}
	}
}

func (m *Model) applyRecentFills(msg recentFillsMsg) (tea.Model, tea.Cmd) {
	if m.form == nil || msg.formID != m.formID || len(msg.fills) == 0 {
		return m, nil
	}
	r := &m.form.recent
	r.fills = msg.fills
	r.cursor = 0
	// Only offer the block while the user has not started on the form.
	if r.dismissed || m.form.current != 0 || m.form.fields[0].touched {
		r.dismissed = true
		return m, nil
	}
	r.active = true
	return m, nil
}

// recentShown reports whether the Recent block is active and fits on screen.
func (m *Model) recentShown() bool {
	return m.form != nil && m.form.recent.active && len(m.form.recent.fills) > 0 &&
		m.recentRowBudget() > 0
}

func (m *Model) dismissRecent() {
	r := &m.form.recent
	r.active = false
	r.dismissed = true
	r.pendingDelete = ""
	r.hint = ""
}

// updateRecent handles a key while the Recent block is shown. handled is false
// when the key should continue to the normal form handling.
func (m *Model) updateRecent(msg tea.KeyMsg) (handled bool, next tea.Model, cmd tea.Cmd) {
	r := &m.form.recent
	if msg.Type != tea.KeyCtrlD {
		r.pendingDelete = ""
		r.hint = ""
	}
	switch {
	case msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc:
		return false, m, nil
	case msg.Type == tea.KeyUp && !msg.Alt:
		if r.cursor < len(r.fills)-1 {
			r.cursor++
		}
		return true, m, nil
	case msg.Type == tea.KeyDown && !msg.Alt:
		if r.cursor > 0 {
			r.cursor--
			return true, m, nil
		}
		m.dismissRecent()
		return true, m, nil
	case (msg.Type == tea.KeyEnter && msg.Alt) || msg.Type == tea.KeyCtrlJ:
		m.applyFill(r.fills[r.cursor])
		m.dismissRecent()
		next, cmd := m.run()
		return true, next, cmd
	case msg.Type == tea.KeyEnter:
		m.applyFill(r.fills[r.cursor])
		m.dismissRecent()
		// Land where input is still needed: the first field the fill left empty
		// or invalid, or else the last field, so a further Enter runs it.
		target := m.form.fillTarget()
		if target < 0 {
			target = len(m.form.fields) - 1
		}
		m.form.gotoField(target)
		m.formErr = ""
		cmd := m.loadCurrentField(false)
		return true, m, cmd
	case msg.Type == tea.KeyCtrlD:
		cmd := m.deleteRecent()
		return true, m, cmd
	}
	// Anything else (typing, field navigation) hides the block and behaves as
	// it would without it.
	m.dismissRecent()
	return false, m, nil
}

func (m *Model) deleteRecent() tea.Cmd {
	r := &m.form.recent
	f := r.fills[r.cursor]
	if f.FromHistory {
		r.pendingDelete = ""
		r.hint = "history rows can't be deleted"
		return nil
	}
	sig := fillSignature(f.Values)
	if r.pendingDelete != sig {
		r.pendingDelete = sig
		r.hint = "press ^d again to delete this fill"
		return nil
	}
	r.pendingDelete = ""
	fb, ok := m.backend.(FillsBackend)
	if !ok {
		return nil
	}
	if err := fb.ForgetFill(m.form.entry, f.Values); err != nil {
		r.hint = "delete failed: " + err.Error()
		return nil
	}
	r.fills = append(r.fills[:r.cursor:r.cursor], r.fills[r.cursor+1:]...)
	if r.cursor >= len(r.fills) {
		r.cursor = max(0, len(r.fills)-1)
	}
	r.hint = "deleted"
	if len(r.fills) == 0 {
		m.dismissRecent()
	}
	return nil
}

// fillable reports whether a field takes part in recent fills: multi fields
// and remember="none" fields are never stored, so a fill leaves them alone.
func fillable(fs *fieldState) bool {
	return !fs.field.Spec.Multi && fs.field.Spec.Remember.Remembers()
}

// applyFill puts a fill's values into every field. A fillable optional field
// absent from the fill is skipped (it was skipped then too); a required one
// absent from it is left as it is, for the user or validation to deal with.
func (m *Model) applyFill(f Fill) {
	for _, fs := range m.form.fields {
		if !fillable(fs) {
			continue
		}
		v, ok := f.Values[fs.field.Name]
		switch {
		case ok:
			fs.setInput(v)
			fs.skipped = false
			fs.touched = true
			// A strict value is kept even when it no longer matches, so the
			// normal validation reports it instead of silently dropping it.
			fs.adopted = !fs.field.Spec.Strict
		case fs.field.Optional:
			fs.setInput("")
			fs.skipped = true
			fs.touched = true
			fs.adopted = false
		default:
			continue
		}
		fs.cursor = 0
		fs.navigated = false
	}
}

// fillValues collects the values to record as a fill when a form is run.
func (m *formModel) fillValues() map[string]string {
	vals := make(map[string]string)
	for _, fs := range m.fields {
		if fs.skipped || !fillable(fs) || fs.input == "" {
			continue
		}
		vals[fs.field.Name] = fs.input
	}
	return vals
}

// recentRowBudget is how many Recent rows fit (0 hides the block). The block
// yields first: the command box and the candidate minimum keep their room.
func (m *Model) recentRowBudget() int {
	if m.form == nil || !m.form.recent.active {
		return 0
	}
	footerH := lipgloss.Height(m.formFooter(false))
	candLines := len(strings.Split(strings.TrimRight(m.candidatesView(maxCandidateRows), "\n"), "\n"))
	minMid := min(minCandidateRows, candLines)
	box := m.fittedCommandBox(m.viewHeight() - footerH - minMid)
	free := m.viewHeight() - lipgloss.Height(box) - footerH - minMid
	n := len(m.form.recent.fills)
	rows := min(maxRecentRows, n, free-1) // 1 header line
	if n > rows {
		rows = min(rows, free-2) // and the "… more" line
	}
	return max(0, rows)
}

// recentView renders the Recent block with at most rows fill rows: a header,
// an overflow note, and the rows bottom-up (best by the input).
func (m *Model) recentView(rows int) []string {
	st := m.styles
	r := &m.form.recent
	if rows <= 0 || len(r.fills) == 0 {
		return nil
	}
	header := "  " + st.paramName.Render("recent")
	if r.hint != "" {
		header += st.dim.Render("  " + r.hint)
	}
	out := []string{header}
	start, end := windowBounds(r.cursor, len(r.fills), rows)
	if len(r.fills) > rows {
		out = append(out, "  "+st.dim.Render(fmt.Sprintf("… %d more", len(r.fills)-rows)))
	}
	now := m.clock().Unix()
	var body []string
	for i := start; i < end; i++ {
		prefix := "    "
		lineStyle := st.rowDesc
		if i == r.cursor {
			prefix = "  " + st.cursor.Render("❯") + " "
			lineStyle = st.fieldActive
		}
		text, meta := m.fillRowText(r.fills[i], now)
		width := max(1, m.viewWidth()-lipgloss.Width(prefix))
		metaW := lipgloss.Width(meta)
		var line string
		if metaW+4 < width {
			line = lineStyle.Render(truncate(text, width-metaW-3)) + "   " + st.dim.Render(meta)
		} else {
			line = lineStyle.Render(truncate(text, width))
		}
		body = append(body, prefix+line)
	}
	reverseLines(body)
	return append(out, body...)
}

// fillRowText formats a fill as "name=val  name=val" plus its "(4×, 2h ago)"
// usage, with a "from history" marker for history-only fills.
func (m *Model) fillRowText(f Fill, now int64) (text, meta string) {
	var parts []string
	for _, fs := range m.form.fields {
		if v, ok := f.Values[fs.field.Name]; ok {
			parts = append(parts, fs.field.Name+"="+flattenOneLine(v))
		}
	}
	text = strings.Join(parts, "  ")
	meta = fmt.Sprintf("(%d×", max(f.Count, 1))
	if f.LastUsed > 0 {
		meta += ", " + ago(now-f.LastUsed)
	}
	meta += ")"
	if f.FromHistory {
		meta += " from history"
	}
	return text, meta
}

// ago formats an age in seconds compactly ("just now", "5m ago", "2h ago").
func ago(secs int64) string {
	switch {
	case secs < 60:
		return "just now"
	case secs < 3600:
		return fmt.Sprintf("%dm ago", secs/60)
	case secs < 86400:
		return fmt.Sprintf("%dh ago", secs/3600)
	case secs < 30*86400:
		return fmt.Sprintf("%dd ago", secs/86400)
	case secs < 365*86400:
		return fmt.Sprintf("%dmo ago", secs/(30*86400))
	default:
		return fmt.Sprintf("%dy ago", secs/(365*86400))
	}
}

// recentHints are the key hints while the Recent block owns the keys.
func (m *Model) recentHints() [][2]string {
	run := "run"
	if m.formAct == formActionBookmark {
		run = "bookmark"
	}
	hints := [][2]string{{"↑↓", "recent"}, {"⏎", "fill"}, {"alt+⏎", run}}
	if !m.form.recent.fills[m.form.recent.cursor].FromHistory {
		hints = append(hints, [2]string{"^d", "delete"})
	}
	return append(hints, [2]string{"type", "skip"}, [2]string{"esc", "back"})
}

func (m *Model) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

// fillTarget is the first field that still needs the user after a fill: a
// required field left empty, or a value that fails validation. A strict field
// whose candidates have not loaded yet is not judged here (its list is fetched
// on demand, so "not in the list" would be a false alarm); run() refetches and
// validates it before running.
func (m *formModel) fillTarget() int {
	for i, fs := range m.fields {
		if fs.field.Spec.Strict && !fs.loaded {
			if _, bad := fs.patternViolation(); bad {
				return i
			}
		} else if fs.validationError() != "" {
			return i
		}
		if fs.field.Optional {
			continue
		}
		if _, ok := fs.value(); !ok {
			return i
		}
	}
	return -1
}
