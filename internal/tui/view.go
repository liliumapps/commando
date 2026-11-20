package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/rank"
)

// maxCandidateRows caps how many candidates are shown per form field.
const maxCandidateRows = 10

// maxPreviewRows caps how many lines of a candidate preview are shown.
const maxPreviewRows = 8

// minCandidateRows is the candidate area the form keeps (or all candidate
// lines, when fewer) before the preview and parameter block shrink.
const minCandidateRows = 5

// View implements tea.Model.
func (m *Model) View() string {
	const clearScreen = "\x1b[2J\x1b[H"
	var out string
	if m.stage == stageList {
		out = m.viewList()
	} else {
		out = m.viewForm()
	}
	if m.clearBeforeRender {
		m.clearBeforeRender = false
		return clearScreen + out
	}
	return out
}

// --- list stage ---

// viewList composes the search palette. Results are single-line rows; the
// selected command text appears once in the bottom control cluster.
func (m *Model) viewList() string {
	footer := m.listFooter()
	footerH := lipgloss.Height(footer)
	listH := m.viewHeight() - footerH
	if listH < 1 {
		listH = 1
	}

	rows := m.listRows(listH)
	for len(rows) < listH {
		rows = append([]string{""}, rows...)
	}
	return strings.Join(rows, "\n") + "\n" + footer
}

func (m *Model) listRows(height int) []string {
	st := m.styles
	if len(m.filtered) == 0 {
		return []string{"  " + st.dim.Render("no matches")}
	}

	start, end := windowBounds(m.listCursor, len(m.filtered), height)

	var rows []string
	for i := start; i < end; i++ {
		rows = append(rows, m.renderEntryRow(m.filtered[i], i == m.listCursor))
	}
	reverseLines(rows)
	return rows
}

func (m *Model) renderEntryRow(e entry.Entry, selected bool) string {
	st := m.styles
	width := m.viewWidth()

	cursor := "  "
	titleStyle := st.rowDesc
	if selected {
		cursor = st.cursor.Render("❯ ")
		titleStyle = st.rowDescSel
	}

	source := st.sourceStyle(e.Source.String()).Render(sourceCode(e.Source))
	sourceGap := "  "
	if width < 64 {
		available := max(1, width-lipgloss.Width(cursor)-lipgloss.Width(source)-lipgloss.Width(sourceGap))
		text := flattenOneLine(e.TitleText())
		if command := flattenOneLine(e.Template); command != text {
			text += "  " + command
		}
		text = pad(truncate(text, available), available)
		if selected {
			return cursor + source + sourceGap + titleStyle.Render(text)
		}
		return cursor + source + sourceGap + titleStyle.Render(text)
	}

	columnGap := "    "
	available := width - lipgloss.Width(cursor) - lipgloss.Width(source) - lipgloss.Width(sourceGap) - lipgloss.Width(columnGap)
	available = max(1, available)
	titleW := max(1, available*40/100)
	cmdW := max(1, available-titleW)

	title := pad(truncate(flattenOneLine(e.TitleText()), titleW), titleW)
	cmd := truncate(flattenOneLine(e.Template), cmdW)
	if selected {
		return cursor + source + sourceGap + titleStyle.Render(title) + columnGap + st.rowDescSel.Render(pad(cmd, cmdW))
	}
	return cursor + source + sourceGap + titleStyle.Render(title) + columnGap + m.highlightEntryCommand(e, cmd)
}

func sourceCode(src entry.Source) string {
	switch src {
	case entry.SourceCheat:
		return "c"
	case entry.SourceProject:
		return "p"
	case entry.SourceHistory:
		return "h"
	case entry.SourceBookmark:
		return "b"
	default:
		return "?"
	}
}

// reverseLines reverses a slice of rendered lines in place.
func reverseLines(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func (m *Model) highlightEntryCommand(e entry.Entry, cmd string) string {
	if e.Kind == entry.CommandLiteral {
		return m.styles.cmdLiteral.Render(cmd)
	}
	return m.styles.highlightTemplate(cmd)
}

// listFooter renders the selected command, query, and key hints together so the
// user can inspect and refine without jumping between top and bottom regions.
func (m *Model) listFooter() string {
	st := m.styles
	scope, _ := rank.ParseScope(m.query)
	scopeName := scope.Name()
	if scope == rank.ScopeAll {
		scopeName = "all"
	}
	info := m.listStatusLine(scopeName)
	desc, cmd := m.selectedEntryDetails()
	promptMarker := st.prompt.Render("❯ ")
	promptWidth := max(1, m.viewWidth()-lipgloss.Width(promptMarker))
	prompt := promptMarker + styledTextInputView(m.queryInput, st.query, st.caret, promptWidth)
	help := m.renderHelp([][2]string{
		{"↑↓", "move"}, {"←→", "edit"}, {"⏎", "select"}, {"tab", "scope"}, {"^b", "bookmark"}, {"esc", "quit"},
	})
	if len(m.filtered) > 0 && m.filtered[m.listCursor].Source == entry.SourceBookmark {
		help = m.renderHelp([][2]string{
			{"↑↓", "move"}, {"←→", "edit"}, {"⏎", "select"}, {"tab", "scope"},
			{"^d", "delete"}, {"esc", "quit"},
		})
	}
	rule := st.dim.Render(strings.Repeat("─", max(0, m.viewWidth()-1)))
	var lines []string
	lines = append(lines, info)
	if m.viewHeight() >= 8 {
		lines = append(lines, desc, cmd, rule)
	}
	lines = append(lines, prompt, truncate(help, m.viewWidth()))
	if m.status != "" {
		lines = append(lines, st.helpKey.Render(truncate(flattenOneLine(m.status), m.viewWidth())))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) listStatusLine(scopeName string) string {
	st := m.styles
	left := st.dim.Render("scope: " + scopeName)
	right := st.counter.Render(fmt.Sprintf("%d/%d", len(m.filtered), len(m.allEntries))) + st.dim.Render(" commando")
	fillW := m.viewWidth() - lipgloss.Width(left) - lipgloss.Width(right)
	if fillW < 0 {
		fillW = 0
	}
	return left + st.dim.Render(strings.Repeat("─", fillW)) + right
}

func (m *Model) selectedEntryDetails() (description, command string) {
	st := m.styles
	if len(m.filtered) == 0 {
		return st.dim.Render("none"), st.dim.Render("")
	}
	e := m.filtered[m.listCursor]
	desc := st.dim.Render(truncate(flattenOneLine(e.DescText()), m.viewWidth()))
	cmd := m.highlightEntryCommand(e, truncate(flattenOneLine(e.Template), m.viewWidth()))
	return desc, cmd
}

// --- form stage ---

// viewForm composes the fill-in form full-screen:
//   - a bordered command summary pinned to the top;
//   - the active parameter's candidates bottom-aligned in the middle;
//   - the active parameter's input, errors, and key hints pinned to the bottom.
//
// Each parameter appears exactly once as an editor (the bottom input) and once
// as an overview line (in the box) — never as a duplicated field list.
//
// The candidate area keeps at least minCandidateRows rows (or all candidate
// lines when fewer): the box gives up preview lines and then parameter lines
// before the candidates are squeezed.
func (m *Model) viewForm() string {
	showRecent := m.recentShown()
	footer := m.formFooter(showRecent)
	footerH := lipgloss.Height(footer)

	candLines := len(strings.Split(strings.TrimRight(m.candidatesView(maxCandidateRows), "\n"), "\n"))
	minMid := min(minCandidateRows, candLines)
	box := m.fittedCommandBox(m.viewHeight() - footerH - minMid)

	// The Recent block sits between the candidates and the input. It yields
	// first: it only takes room the box and the candidate minimum leave free.
	var recent []string
	if showRecent {
		recent = m.recentView(m.recentRowBudget())
	}

	boxH := lipgloss.Height(box)
	middleH := max(0, m.viewHeight()-boxH-footerH-len(recent))
	midLimit := min(maxCandidateRows, max(1, middleH))
	// While the Recent block owns the arrows, no candidate is drawn as the
	// highlighted pick.
	m.dimCandidates = showRecent
	mid := strings.Split(strings.TrimRight(m.candidatesView(midLimit), "\n"), "\n")
	m.dimCandidates = false
	mid = clampLines(mid, middleH)
	for len(mid) < middleH {
		mid = append([]string{""}, mid...)
	}

	parts := []string{box}
	if middleH > 0 {
		parts = append(parts, strings.Join(mid, "\n"))
	}
	if len(recent) > 0 {
		parts = append(parts, strings.Join(recent, "\n"))
	}
	parts = append(parts, footer)
	workspace := strings.Join(parts, "\n")
	if lipgloss.Height(workspace) > m.viewHeight() {
		lines := strings.Split(workspace, "\n")
		workspace = strings.Join(clampLines(lines, m.viewHeight()), "\n")
	}
	return workspace
}

// fittedCommandBox renders the command box within budget lines, giving up
// preview lines and then parameter lines until it fits (or cannot shrink).
func (m *Model) fittedCommandBox(budget int) string {
	paramLimit := 6
	if m.viewHeight() < 16 {
		paramLimit = 2
	}
	previewRows := maxPreviewRows
	box := m.commandBoxView(paramLimit, previewRows)
	for lipgloss.Height(box) > budget {
		switch {
		case previewRows > 3:
			previewRows--
		case paramLimit > 2:
			paramLimit--
		case previewRows > 0:
			previewRows--
		case paramLimit > 1:
			paramLimit--
		default:
			previewRows = -1 // nothing left to shrink
		}
		if previewRows < 0 {
			break
		}
		box = m.commandBoxView(paramLimit, previewRows)
	}
	return box
}

// commandBoxView renders the bordered command box: description, highlighted
// command template, the parameter assignment block (at most paramLimit rows),
// and (if any) the preview (at most previewRows lines; 0 hides it).
func (m *Model) commandBoxView(paramLimit, previewRows int) string {
	st := m.styles
	f := m.form

	var b strings.Builder
	b.WriteString(st.rowDesc.Render(flattenOneLine(f.entry.TitleText())))
	b.WriteString("\n")
	// The live preview of the assembled command: filled values are substituted,
	// unfilled required params show as <name> placeholders.
	b.WriteString(st.dim.Render("command:"))
	b.WriteString("\n  ")
	commandWidth := max(1, m.viewWidth()-8)
	b.WriteString(st.highlightAssembled(truncate(flattenOneLine(f.preview()), commandWidth)))
	if m.viewHeight() < 10 {
		return b.String()
	}
	b.WriteString("\n\n")
	b.WriteString(st.dim.Render("params:"))
	b.WriteString("\n")
	b.WriteString(m.assignmentsBlock(paramLimit))

	if f.cur().field.Spec.Preview != "" && previewRows > 0 {
		if pane := m.previewPane(previewRows); pane != "" {
			b.WriteString("\n\n")
			b.WriteString(strings.TrimRight(pane, "\n"))
		}
	}
	return st.commandBox.Width(max(1, m.viewWidth()-2)).Render(b.String())
}

// assignmentsBlock renders one line per parameter: "name = value" for filled
// params, "name =" for unfilled ones, and a "[skipped]" marker for skipped
// optionals. The active parameter's name is highlighted and marked with a
// cursor so the user can see which one the bottom input edits.
func (m *Model) assignmentsBlock(limit int) string {
	st := m.styles
	var lines []string
	limit = max(1, limit)
	start, end := assignmentBounds(m.form.current, len(m.form.fields), limit)
	if start > 0 {
		lines = append(lines, "  "+st.dim.Render(fmt.Sprintf("… %d previous", start)))
	}
	for i := start; i < end; i++ {
		fs := m.form.fields[i]
		active := i == m.form.current

		marker := "  "
		nameStyle := st.paramName
		if active {
			marker = st.cursor.Render("❯ ")
			nameStyle = st.paramNameCur
		}
		name := nameStyle.Render(fs.field.Name)
		status := paramStatus(fs)

		var val string
		switch {
		case fs.skipped:
			val = st.skipTag.Render("(skipped)")
		case fs.field.Spec.Multi && len(fs.selected) > 0:
			selected := truncate(flattenOneLine(strings.Join(fs.selected, ", ")), max(8, m.viewWidth()/2))
			val = st.valueText.Render(fmt.Sprintf("%d selected", len(fs.selected))) +
				st.dim.Render("  "+selected)
		default:
			if v, ok := fs.value(); ok {
				val = st.valueText.Render(truncate(flattenOneLine(v), max(8, m.viewWidth()/2)))
			} else {
				val = st.placeholder.Render("‹unset›")
			}
		}
		lines = append(lines, marker+name+" "+st.skipTag.Render(status)+" "+st.equals.Render("=")+" "+val)
	}
	if end < len(m.form.fields) {
		lines = append(lines, "  "+st.dim.Render(fmt.Sprintf("… %d more", len(m.form.fields)-end)))
	}
	return strings.Join(lines, "\n")
}

func assignmentBounds(current, total, limit int) (start, end int) {
	if total <= limit {
		return 0, total
	}
	start = current - limit/2
	if start < 0 {
		start = 0
	}
	end = min(total, start+limit)
	start = max(0, end-limit)
	return start, end
}

func paramStatus(fs *fieldState) string {
	var parts []string
	if fs.field.Optional {
		parts = append(parts, "optional")
	} else {
		parts = append(parts, "required")
	}
	if fs.field.Spec.Multi {
		parts = append(parts, "multi")
	}
	if fs.field.Spec.Strict {
		parts = append(parts, "strict")
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// candidatesView renders the candidate list for the active parameter only.
func (m *Model) candidatesView(limit int) string {
	st := m.styles
	fs := m.form.cur()

	vis := fs.displayCandidates()
	var notices []string
	if fs.loading {
		notices = append(notices, "  "+st.dim.Render("loading candidates..."))
	}
	if fs.loadError != "" {
		notices = append(notices, "  "+st.errText.Render("candidate source failed: "+flattenOneLine(fs.loadError)))
	}
	if fs.truncated {
		notices = append(notices, "  "+st.dim.Render("candidate output truncated"))
	}
	if len(vis) == 0 {
		if !fs.loading {
			if fs.field.Spec.Src != "" {
				notices = append(notices, "  "+st.dim.Render("no candidates; type a value"))
			} else {
				notices = append(notices, "  "+st.dim.Render("type a value"))
			}
		}
		return strings.Join(notices, "\n")
	}

	// Reversed, bottom-anchored to match the list view: the highlighted
	// candidate (fs.cursor) sits at the bottom, right above the input, and other
	// candidates stack upward.
	start, end := windowBounds(fs.cursor, len(vis), limit)
	highlight := fs.cursorHighlighted() && !m.dimCandidates
	var rows []string
	for i := start; i < end; i++ {
		c := vis[i]
		prefix := "    "
		lineStyle := st.rowDesc
		if i == fs.cursor && highlight {
			prefix = "  " + st.cursor.Render("❯") + " "
			lineStyle = st.fieldActive
		}
		check := ""
		if fs.field.Spec.Multi {
			if fs.selSet[c.Value] {
				check = st.checkOn.Render("◉ ")
			} else {
				check = st.checkOff.Render("◯ ")
			}
		}
		contentWidth := max(1, m.viewWidth()-lipgloss.Width(prefix)-lipgloss.Width(check))
		rows = append(rows, prefix+check+lineStyle.Render(truncate(flattenOneLine(c.Display), contentWidth)))
	}
	reverseLines(rows)
	// The overflow indicator goes on top (the truncated tail of the list is the
	// part furthest from the cursor).
	if len(vis) > limit {
		rows = append([]string{"  " + st.dim.Render(fmt.Sprintf("… %d more", len(vis)-limit))}, rows...)
	}
	rows = append(notices, rows...)
	return strings.Join(rows, "\n") + "\n"
}

// formFooter renders the bottom-pinned region: the active parameter's input
// line, an optional error, and key hints. All input happens here at the bottom.
// recent selects the Recent block's key hints; the height is the same either
// way.
func (m *Model) formFooter(recent bool) string {
	st := m.styles
	fs := m.form.cur()

	var b strings.Builder
	// Active parameter input line, mirroring the assignment block's labels:
	// show (opt) for optional params and (multi) for multi-select ones.
	label := st.paramNameCur.Render(fs.field.Name)
	if fs.field.Optional {
		label += st.skipTag.Render(" (opt)")
	}
	if fs.field.Spec.Multi {
		label += st.skipTag.Render(" (multi)")
	}
	if fs.field.Spec.Multi && len(fs.selected) > 0 {
		summary := fmt.Sprintf("%d selected: %s", len(fs.selected), strings.Join(fs.selected, ", "))
		b.WriteString(st.dim.Render(truncate(summary, m.viewWidth())) + "\n")
	}
	inputPrefix := st.prompt.Render("❯ ") + label + st.equals.Render(" = ")
	inputWidth := max(1, m.viewWidth()-lipgloss.Width(inputPrefix))
	b.WriteString(inputPrefix + styledTextInputView(fs.editor, st.valueText, st.caret, inputWidth) + "\n")

	if m.formErr != "" {
		b.WriteString(st.errText.Render("✗ "+m.formErr) + "\n")
	}

	hints := m.formHints(fs)
	if recent {
		hints = m.recentHints()
	}
	b.WriteString(truncate(m.renderHelp(hints), m.viewWidth()))
	return b.String()
}

// formHints returns the key hints for the form, tailored to the active field.
// The Enter hint reflects its dynamic action: "run" on the last parameter,
// "next" otherwise.
func (m *Model) formHints(fs *fieldState) [][2]string {
	hints := [][2]string{{"←→", "edit"}, {"^p/^n", "params"}}
	if len(fs.visibleCandidates()) > 0 {
		// Candidates are drawn bottom-up (best by the input): ↑ moves the
		// highlight up the screen, ↓ back toward the input.
		hints = append(hints, [2]string{"↑↓", "candidates"})
		if fs.field.Spec.Multi {
			hints = append(hints, [2]string{"space", "toggle"})
		}
	}
	enterLabel := "next"
	if m.form.onLastField() {
		if m.formAct == formActionBookmark {
			enterLabel = "bookmark"
		} else {
			enterLabel = "run"
		}
	}
	hints = append(hints, [2]string{"⏎", enterLabel})
	// Tab/Shift+Tab jump between required params — only worth showing when there
	// is more than one required param to move between.
	if len(m.form.requiredIndices()) > 1 {
		hints = append(hints, [2]string{"tab", "req params"})
	}
	hints = append(hints, [2]string{"^d", "clear"})
	if fs.canSkip() {
		hints = append(hints, [2]string{"^s", "skip"})
	}
	switch {
	case fs.loadError != "":
		hints = append(hints, [2]string{"^r", "retry"})
	case fs.canRefresh() && fs.loaded && len(fs.data.Candidates) == 0:
		hints = append(hints, [2]string{"^r", "refresh"})
	}
	hints = append(hints, [2]string{"esc", "back"})
	return hints
}

// clampLines keeps at most n lines, preferring to drop from the top so the tail
// (the active field's candidates) stays visible.
func clampLines(lines []string, n int) []string {
	if n < 0 {
		n = 0
	}
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

// previewPane renders the highlighted candidate's preview output in a bordered
// box, at most rows lines tall (plus the border) so a long preview does not
// push the form off screen. While a new preview loads, the previous one stays
// in place so the layout does not jump.
func (m *Model) previewPane(rows int) string {
	st := m.styles
	rows = max(1, rows)
	text := sanitizePreviewText(m.form.candidatePreview())
	if m.form.previewLoading && strings.TrimSpace(text) == "" && m.form.previewData.Error == "" {
		return "    " + st.dim.Render("loading preview...")
	}
	if strings.TrimSpace(text) == "" && m.form.previewData.Error == "" {
		return ""
	}
	if m.form.previewData.Error != "" {
		if text != "" {
			text += "\n"
		}
		text += "preview failed: " + m.form.previewData.Error
	}
	if m.form.previewData.Truncated {
		text += "\n(preview output truncated)"
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > rows {
		lines = append(lines[:rows-1], "…")
	}
	// Truncate each line to the content width so the box stays intact.
	w := max(1, m.viewWidth()-8)
	for i, l := range lines {
		lines[i] = truncate(l, w)
	}
	body := st.dim.Render(strings.Join(lines, "\n"))
	boxLines := strings.Split(st.previewBox.Render(body), "\n")
	for i, l := range boxLines {
		boxLines[i] = "    " + l
	}
	return strings.Join(boxLines, "\n") + "\n"
}

// sanitizePreviewText makes command output safe to measure and draw: ANSI
// escape sequences are stripped, CRLF becomes LF, tabs expand to 8-column
// stops, and other control characters are dropped. Truncation then matches
// what the terminal actually shows.
func sanitizePreviewText(text string) string {
	if text == "" {
		return ""
	}
	text = ansi.Strip(text)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	const tabStop = 8
	var b strings.Builder
	b.Grow(len(text))
	col := 0
	for _, r := range text {
		switch {
		case r == '\n':
			b.WriteByte('\n')
			col = 0
		case r == '\t':
			n := tabStop - col%tabStop
			b.WriteString(strings.Repeat(" ", n))
			col += n
		case r < 0x20 || r == 0x7f:
			// drop stray control characters (e.g. a lone \r or bell)
		default:
			b.WriteRune(r)
			col += ansi.StringWidth(string(r))
		}
	}
	return b.String()
}

// highlightAssembled colours an already-assembled command line: <placeholders>
// that remain (unfilled required vars) are tinted, everything else is command
// colour.
func (s styles) highlightAssembled(cmd string) string {
	var b strings.Builder
	i := 0
	for i < len(cmd) {
		if cmd[i] == '<' {
			if j := strings.IndexByte(cmd[i:], '>'); j >= 0 {
				b.WriteString(s.highlightVar(cmd[i+1 : i+j]))
				i += j + 1
				continue
			}
		}
		// Emit a run of literal text up to the next '<'.
		next := strings.IndexByte(cmd[i:], '<')
		if next < 0 {
			b.WriteString(s.cmdLiteral.Render(cmd[i:]))
			break
		}
		b.WriteString(s.cmdLiteral.Render(cmd[i : i+next]))
		i += next
	}
	return b.String()
}

// --- helpers ---

func (m *Model) renderHelp(pairs [][2]string) string {
	st := m.styles
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = st.helpKey.Render(p[0]) + " " + st.help.Render(p[1])
	}
	return st.help.Render("  ") + strings.Join(parts, st.help.Render(" · "))
}

// viewWidth returns a sane content width, defaulting when the size is unknown.
func (m *Model) viewWidth() int {
	if m.width <= 0 {
		return 100
	}
	return m.width
}

// viewHeight returns a sane content height, defaulting when the size is unknown.
func (m *Model) viewHeight() int {
	if m.height <= 0 {
		return 24
	}
	return m.height
}

// flattenOneLine collapses any embedded newlines, carriage returns, and tabs in
// s into single spaces, and collapses runs of whitespace, so a multi-line shell
// command (joined from a backslash continuation in history) renders on a single
// row instead of breaking the layout. Only used for display; the underlying
// command string is left untouched for execution.
func flattenOneLine(s string) string {
	if !strings.ContainsAny(s, "\n\r\t") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		switch r {
		case '\n', '\r', '\t', ' ':
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		default:
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// windowBounds returns a [start,end) window of size at most limit that keeps
// cursor visible within a list of length total.
func windowBounds(cursor, total, limit int) (start, end int) {
	if total <= limit {
		return 0, total
	}
	start = cursor - limit/2
	if start < 0 {
		start = 0
	}
	end = start + limit
	if end > total {
		end = total
		start = end - limit
	}
	return start, end
}

// truncate shortens s to at most n runes, appending an ellipsis when cut.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= n {
		return s
	}
	return ansi.Truncate(s, n, "…")
}

// pad right-pads s to n terminal cells so wide glyphs stay aligned.
func pad(s string, n int) string {
	w := ansi.StringWidth(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}
