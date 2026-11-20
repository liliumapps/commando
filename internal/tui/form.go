package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/rank"
	"github.com/liliumapps/commando/internal/resolve"
	"github.com/liliumapps/commando/internal/template"
)

// fieldState holds the in-progress input for a single variable field.
type fieldState struct {
	field resolve.Field
	data  FieldData

	// deps are the names of variables referenced by this field's Src command.
	// When non-empty, candidates are fetched lazily (on entry) and re-fetched
	// when a referenced value changes.
	deps []string
	// loaded is true once FieldData has been fetched at least once.
	loaded bool
	// loading and request track asynchronous candidate loads. request is
	// incremented for each fetch so stale responses can be ignored.
	loading   bool
	loadError string
	truncated bool
	request   uint64
	// depSnapshot records the referenced values at the last fetch, used to
	// detect when a re-fetch is needed.
	depSnapshot map[string]string

	// input is the free-text value being typed (used for single-value fields
	// and as a filter/custom entry for candidate lists).
	input  string
	editor textinput.Model
	// touched is set once the user edits this field, so a dependency-triggered
	// re-fetch does not clobber a value they chose.
	touched bool
	// navigated is set when the user moves the candidate cursor with the arrow
	// keys. It tells commit() to adopt the highlighted candidate on Enter (vs.
	// keeping typed text). It resets on typing and when the field loses focus.
	navigated bool
	// cursor is the index of the highlighted candidate in the visible list.
	cursor int
	// selected holds the chosen candidate values for a multi field, in
	// selection order.
	selected []string
	// selSet mirrors selected for O(1) membership checks.
	selSet map[string]bool
	// skipped marks an optional field the user chose not to fill.
	skipped bool
	// adopted is set when the current input was placed by the form rather than
	// typed: a candidate adopted via Enter/arrow-leave, or a prefill. While it
	// holds and the input equals a candidate, the full list is shown so the user
	// can browse to another value. Typing clears it, so a typed value that
	// happens to equal a candidate keeps the list filtered.
	adopted bool

	// dataVersion increments whenever data changes; it keys the memoized
	// ranking below so candidates are ranked once per (input, data) rather than
	// several times per frame.
	dataVersion uint64
	valueSet    map[string]struct{}
	memo        candidateMemo

	// pattern is the compiled Spec.Pattern (anchored to a full-string match), or
	// nil when the field defines no pattern. Compiled once here rather than per
	// keystroke. The regex is validated at config load, so compilation is not
	// expected to fail; if it somehow does, the field simply has no pattern.
	pattern *regexp.Regexp
}

// candidateMemo caches the ranked candidate lists for one (input, adopted,
// dataVersion) key.
type candidateMemo struct {
	valid   bool
	input   string
	adopted bool
	version uint64
	vis     []rank.Candidate
	display []rank.Candidate
}

func newFieldState(field resolve.Field) *fieldState {
	fs := &fieldState{
		field:   field,
		deps:    resolve.SrcRefs(field.Spec.Src),
		input:   field.Spec.Default, // provisional until data is loaded
		editor:  newTextInput(field.Spec.Default),
		selSet:  make(map[string]bool),
		adopted: field.Spec.Default != "",
	}
	if p := field.Spec.Pattern; p != "" {
		if re, err := regexp.Compile("^(?:" + p + ")$"); err == nil {
			fs.pattern = re
		}
	}
	return fs
}

// applyData stores freshly fetched candidate data and, unless the user has
// already edited the field, adopts the suggested prefill. cur is the map of
// referenced values used for the fetch, snapshotted for change detection.
func (fs *fieldState) applyData(data FieldData, cur map[string]string) {
	fs.setData(data)
	fs.loaded = true
	fs.loading = false
	fs.loadError = data.Error
	fs.truncated = data.Truncated
	snap := make(map[string]string, len(fs.deps))
	for _, d := range fs.deps {
		snap[d] = cur[d]
	}
	fs.depSnapshot = snap
	if !fs.touched {
		fs.setInput(data.Prefill)
		fs.adopted = data.Prefill != ""
	}
	fs.dropStaleStrict()
}

// setData replaces the candidate data and invalidates derived caches.
func (fs *fieldState) setData(data FieldData) {
	fs.data = data
	fs.dataVersion++
	fs.memo.valid = false
	fs.valueSet = make(map[string]struct{}, len(data.Candidates))
	for _, c := range data.Candidates {
		fs.valueSet[c.Value] = struct{}{}
	}
}

// dropStaleStrict invalidates a strict field's value when freshly loaded
// candidates (e.g. after a dependency changed) no longer contain it. A failed
// source leaves the value alone: its candidate list is not authoritative.
func (fs *fieldState) dropStaleStrict() {
	if !fs.field.Spec.Strict || fs.data.Error != "" {
		return
	}
	if len(fs.selected) > 0 {
		kept := fs.selected[:0]
		for _, v := range fs.selected {
			if fs.matchesCandidate(v) {
				kept = append(kept, v)
			} else {
				delete(fs.selSet, v)
			}
		}
		fs.selected = kept
	}
	if fs.input != "" && !fs.matchesCandidate(fs.input) && fs.adopted {
		fs.setInput("")
		fs.adopted = false
		fs.cursor = 0
		fs.navigated = false
	}
}

// needsFetch reports whether candidate data must be (re)loaded for the given
// current referenced values: either it was never loaded, or a referenced value
// changed since the last fetch.
func (fs *fieldState) needsFetch(cur map[string]string) bool {
	if !fs.loaded {
		return true
	}
	for _, d := range fs.deps {
		if fs.depSnapshot[d] != cur[d] {
			return true
		}
	}
	return false
}

// rawValue returns the unformatted chosen value for this field, suitable for
// substitution into another field's Src command. Unlike value(), it does not
// apply the multi Each/Sep formatting.
func (fs *fieldState) rawValue() (string, bool) {
	if fs.skipped {
		return "", false
	}
	if fs.field.Spec.Multi {
		if len(fs.selected) > 0 {
			return strings.Join(fs.selected, " "), true
		}
		if fs.input != "" {
			return fs.input, true
		}
		return "", false
	}
	if fs.input == "" {
		return "", false
	}
	return fs.input, true
}

// visibleCandidates returns the candidates ranked for the current input,
// delegating ordering to the rank package: with no input all candidates show in
// idle order (used values most-recent-first, then src values); otherwise they
// are ranked by match tier (starts-with, contains, fuzzy) against each
// candidate's Display.
//
// Right after a candidate was adopted (or prefilled) the input equals that
// candidate's value; then the full list is shown so the user can still browse
// to a different value instead of the list collapsing to one. A typed value
// keeps the list filtered even when it exactly equals a candidate.
//
// The result is memoized per (input, adopted, data version); callers must not
// modify it.
func (fs *fieldState) visibleCandidates() []rank.Candidate {
	fs.ensureMemo()
	return fs.memo.vis
}

func (fs *fieldState) ensureMemo() {
	m := &fs.memo
	if m.valid && m.input == fs.input && m.adopted == fs.adopted && m.version == fs.dataVersion {
		return
	}
	query := fs.input
	if fs.adopted && fs.matchesCandidate(query) {
		query = ""
	}
	vis := rank.Candidates(fs.data.Candidates, query)
	display := vis
	typedRow := !fs.field.Spec.Multi && !fs.field.Spec.Strict &&
		fs.input != "" && !fs.matchesCandidate(fs.input) && len(vis) > 0
	if typedRow {
		row := rank.Candidate{Display: fs.input, Value: fs.input}
		display = append([]rank.Candidate{row}, vis...)
	}
	*m = candidateMemo{
		valid:   true,
		input:   fs.input,
		adopted: fs.adopted,
		version: fs.dataVersion,
		vis:     vis,
		display: display,
	}
}

// matchesCandidate reports whether v exactly equals one of the candidate values
// (the inserted value, not the display label — that is what commit stores and
// what strictness is checked against).
func (fs *fieldState) matchesCandidate(v string) bool {
	if fs.valueSet == nil {
		fs.setData(fs.data)
	}
	_, ok := fs.valueSet[v]
	return ok
}

// isListField reports whether this field draws its candidates from a `src`
// command (a "list" field). For such fields the candidates are the canonical
// choices, so Enter on an empty input adopts the highlighted one. A plain field
// (no `src`) is free text whose candidates are merely remembered-value hints —
// there, Enter never auto-adopts; the user must navigate (↑) to pick one, which
// keeps remembered values non-intrusive.
func (fs *fieldState) isListField() bool {
	return fs.field.Spec.Src != ""
}

// filtering reports whether a single-value field is actively narrowing its
// candidate list with typed text that is not yet an exact candidate, while at
// least one candidate still matches. This is the shared trigger for the
// highlight and for Enter's adopt: in this state the row at index 0 is the
// committable default — the typed text itself for a non-strict field (shown as a
// synthetic row) or the top matching candidate for a strict field. Because the
// default sits at the cursor's home from the first keystroke, ↑ moves off it to
// index 1 rather than appearing to skip the first item. An empty input or an
// exact match is not "filtering": the full idle list is shown and no default is
// forced.
func (fs *fieldState) filtering() bool {
	return !fs.field.Spec.Multi &&
		fs.input != "" &&
		!fs.matchesCandidate(fs.input) &&
		len(fs.visibleCandidates()) > 0
}

// hasTypedRow reports whether displayCandidates prepends a synthetic typed-value
// row: a non-strict field that is filtering. A strict field never gets one (its
// typed text is not committable), so its top match stands as the default
// instead.
func (fs *fieldState) hasTypedRow() bool {
	return fs.filtering() && !fs.field.Spec.Strict
}

// displayCandidates is the list actually shown and navigated: the ranked real
// candidates, with a synthetic row carrying the typed text prepended at index 0
// when hasTypedRow. Because that synthetic row sits at the cursor's home position
// and is highlighted, the highlighted row is always exactly what Enter commits —
// the typed value while typing, or a real candidate once navigated to. Navigation
// bounds, rendering, commit, and preview all read this list so they never
// disagree about which row is current.
func (fs *fieldState) displayCandidates() []rank.Candidate {
	fs.ensureMemo()
	return fs.memo.display
}

// adoptCursor sets the field's value to the highlighted display candidate. It is
// a no-op for a multi field or when the cursor points nowhere. Both Enter
// (commit) and arrow-leave (commitOnLeave) route through it once they have
// decided that adopting is appropriate.
func (fs *fieldState) adoptCursor() {
	if fs.field.Spec.Multi {
		return
	}
	vis := fs.displayCandidates()
	if len(vis) == 0 || fs.cursor < 0 || fs.cursor >= len(vis) {
		return
	}
	fs.setInput(vis[fs.cursor].Value)
	fs.adopted = true
	fs.touched = true
	fs.skipped = false
}

// commit finalizes this single-value field's value when the user presses Enter.
// It adopts the highlighted display candidate whenever cursorAdopts is true;
// otherwise the typed input stands as-is. Adopting is a no-op for a non-strict
// synthetic typed row (index 0 already carries the typed text) and pulls in the
// real value for a navigated, filtered, or quick-picked candidate. For a plain
// field with an empty, un-navigated input nothing is adopted, so a remembered
// value is never pulled in silently.
//
// For a required multi field with nothing selected, Enter selects the
// highlighted candidate (it is drawn highlighted, so that is what Enter takes)
// unless free text that will be used verbatim was typed.
func (fs *fieldState) commit() {
	if fs.field.Spec.Multi {
		fs.commitMulti()
		return
	}
	if fs.cursorAdopts() {
		fs.adoptCursor()
	}
}

func (fs *fieldState) commitMulti() {
	if fs.field.Optional || fs.skipped || len(fs.selected) > 0 {
		return
	}
	if fs.input != "" && !(fs.field.Spec.Strict && !fs.matchesCandidate(fs.input)) {
		return
	}
	vis := fs.displayCandidates()
	if fs.cursor < 0 || fs.cursor >= len(vis) {
		return
	}
	fs.toggleSelected(vis[fs.cursor].Value)
	fs.setInput("")
	fs.touched = true
}

// cursorAdopts reports whether pressing Enter adopts the highlighted display
// candidate for this single-value field. True in three cases:
//   - the user navigated the list (they chose the highlighted row);
//   - the field is filtering (index 0 is the committable default — the typed
//     text for a non-strict field, or the top match for a strict one);
//   - the input is empty on a REQUIRED list field (a quick pick of the top
//     candidate). An optional list field is excluded here so an empty,
//     un-navigated input leaves it unset instead of forcing the top candidate.
//
// commit() uses this to decide what Enter picks, and cursorHighlighted() uses it
// to draw the highlight, so the two never disagree.
func (fs *fieldState) cursorAdopts() bool {
	emptyRequiredPick := fs.input == "" && fs.isListField() && !fs.field.Optional
	return fs.navigated || fs.filtering() || emptyRequiredPick
}

// cursorEngaged reports whether the user has interacted with the candidate list
// enough that its cursor should be shown: they navigated it (arrows), edited the
// field (typed a filter), or toggled a selection. Used to keep an optional multi
// field's ❯ toggle cursor hidden until the user engages, so an untouched optional
// multi field shows nothing highlighted and Enter omits it.
func (fs *fieldState) cursorEngaged() bool {
	return fs.navigated || fs.touched || len(fs.selected) > 0
}

// cursorHighlighted reports whether the candidate under fs.cursor should be
// drawn as highlighted. For a multi field the cursor is the Space toggle target:
// a required multi always shows it, but an optional multi hides it until the user
// engages (cursorEngaged), so the field can be left empty without a preselection.
// For a single-value field it is highlighted only when Enter would actually adopt
// it (cursorAdopts) — while free-typing that is the synthetic typed row, so the
// highlight tracks the typed value rather than falsely marking an unrelated
// candidate as selected.
func (fs *fieldState) cursorHighlighted() bool {
	if fs.field.Spec.Multi {
		return !fs.field.Optional || fs.cursorEngaged()
	}
	return fs.cursorAdopts()
}

// commitOnLeave finalizes a single-value field when focus moves away via the
// arrow keys. Unlike commit(), it adopts the highlighted candidate ONLY when the
// user actually navigated the list — never on an untouched pass-through, so
// moving past an optional param does not silently fill it, and a filtered-but-
// un-navigated default is not committed just by leaving. Typed input is left
// as-is either way (it already lives in fs.input).
func (fs *fieldState) commitOnLeave() {
	if fs.navigated {
		fs.adoptCursor()
	}
}

// clear resets this field to empty: no typed input, no multi-selections, and
// un-skipped. Used by the clear-value key.
func (fs *fieldState) clear() {
	fs.setInput("")
	fs.adopted = false
	fs.selected = nil
	fs.selSet = make(map[string]bool)
	fs.skipped = false
	fs.touched = true
	fs.cursor = 0
	fs.navigated = false
}

func (fs *fieldState) setInput(value string) {
	fs.input = value
	fs.editor.SetValue(value)
	fs.editor.CursorEnd()
}

func (fs *fieldState) syncInputFromEditor() {
	fs.input = fs.editor.Value()
}

// toggleSelected adds or removes value from the multi-selection.
func (fs *fieldState) toggleSelected(value string) {
	if fs.selSet[value] {
		delete(fs.selSet, value)
		for i, v := range fs.selected {
			if v == value {
				fs.selected = append(fs.selected[:i], fs.selected[i+1:]...)
				break
			}
		}
		return
	}
	fs.selSet[value] = true
	fs.selected = append(fs.selected, value)
}

// value returns the resolved substitution string for this field and whether it
// is present (i.e. not skipped / not empty). For multi fields the selected
// values are formatted via the spec's Each/Sep; if none are explicitly selected
// but there is free-text input, the input is used verbatim.
func (fs *fieldState) value() (string, bool) {
	if fs.skipped {
		return "", false
	}
	if fs.field.Spec.Multi {
		if len(fs.selected) > 0 {
			return template.FormatMulti(fs.selected, fs.field.Spec.Sep, fs.field.Spec.Each), true
		}
		if fs.input != "" {
			return template.FormatMulti([]string{fs.input}, fs.field.Spec.Sep, fs.field.Spec.Each), true
		}
		return "", false
	}
	if fs.input == "" {
		return "", false
	}
	return fs.input, true
}

// canSkip reports whether this field may be skipped (optional fields only).
func (fs *fieldState) canSkip() bool { return fs.field.Optional }

// strictViolation reports whether a strict field currently holds a value that is
// not among its candidates. A skipped field, an empty field, or a non-strict
// field never violates. For multi fields every selected value must be a
// candidate; free-typed input on a strict field is only accepted if it exactly
// matches a candidate.
func (fs *fieldState) strictViolation() bool {
	if !fs.field.Spec.Strict || fs.skipped {
		return false
	}
	if fs.field.Spec.Multi {
		if len(fs.selected) > 0 {
			return false // selections are candidates by construction
		}
		// No explicit selection: any typed input must match a candidate.
		return fs.input != "" && !fs.matchesCandidate(fs.input)
	}
	if fs.input == "" {
		return false // empty is handled by required/optional rules, not strict
	}
	return !fs.matchesCandidate(fs.input)
}

// validationError returns the strict or pattern error for this field's current
// value, or "" when it is acceptable. Required presence is checked separately.
func (fs *fieldState) validationError() string {
	if fs.strictViolation() {
		bad := fs.input
		return "\"" + bad + "\" is not one of the allowed values for " + fs.field.Name
	}
	if bad, ok := fs.patternViolation(); ok {
		return "\"" + bad + "\" does not match the required pattern for " + fs.field.Name
	}
	return ""
}

// canRefresh reports whether Ctrl-R may refetch this field's candidates: it has
// a src and no fetch is in flight.
func (fs *fieldState) canRefresh() bool {
	return fs.isListField() && !fs.loading
}

// patternViolation reports whether a field with a Pattern holds a value that
// does not match it. A skipped or empty field never violates (emptiness is an
// optional/required concern). For a multi field every selected value — or the
// typed input when nothing is selected — must match. The pattern is anchored to
// the full value (see entry validation), so a partial match is a violation.
func (fs *fieldState) patternViolation() (string, bool) {
	re := fs.pattern
	if re == nil || fs.skipped {
		return "", false
	}
	check := func(v string) (string, bool) {
		if v != "" && !re.MatchString(v) {
			return v, true
		}
		return "", false
	}
	if fs.field.Spec.Multi {
		if len(fs.selected) > 0 {
			for _, v := range fs.selected {
				if bad, ok := check(v); ok {
					return bad, true
				}
			}
			return "", false
		}
		return check(fs.input)
	}
	return check(fs.input)
}

// formModel manages the sequence of field states for an entry and assembles the
// live preview. It is pure with respect to rendering: Bubble Tea message
// handling in model.go drives it, and tests can drive it directly.
type formModel struct {
	entry   entry.Entry
	plan    *resolve.Plan
	fields  []*fieldState
	current int

	previewData    PreviewData
	previewLoading bool
	previewRequest uint64
}

func newFormModel(e entry.Entry, plan *resolve.Plan) *formModel {
	fields := make([]*fieldState, len(plan.Fields))
	for i, f := range plan.Fields {
		fields[i] = newFieldState(f)
	}
	return &formModel{entry: e, plan: plan, fields: fields}
}

// rawValues returns the map of unformatted chosen values, used to substitute
// into dependent fields' Src commands.
func (m *formModel) rawValues() map[string]string {
	vals := make(map[string]string)
	for _, fs := range m.fields {
		if v, ok := fs.rawValue(); ok {
			vals[fs.field.Name] = v
		}
	}
	return vals
}

// values returns the current substitution map from all non-skipped, non-empty
// fields, with multi fields formatted via Each/Sep for command assembly.
func (m *formModel) values() map[string]string {
	vals := make(map[string]string)
	for _, fs := range m.fields {
		if v, ok := fs.value(); ok {
			vals[fs.field.Name] = v
		}
	}
	return vals
}

// preview renders the best-effort assembled command for display.
func (m *formModel) preview() string {
	return m.plan.Preview(m.values())
}

// candidatePreview returns the last asynchronously loaded preview.
func (m *formModel) candidatePreview() string {
	return m.previewData.Text
}

// assemble produces the final command, erroring if a required field is unmet.
func (m *formModel) assemble() (string, error) {
	return m.plan.Assemble(m.values())
}

// cur returns the currently focused field state.
func (m *formModel) cur() *fieldState { return m.fields[m.current] }

// gotoField focuses the field at index i (clamped) and loads its candidates.
func (m *formModel) gotoField(i int) {
	if i < 0 {
		i = 0
	}
	if i > len(m.fields)-1 {
		i = len(m.fields) - 1
	}
	m.current = i
	// Reset the candidate cursor and navigation state so a freshly focused
	// param starts at its best candidate and Enter keeps any typed value.
	m.fields[i].cursor = 0
	m.fields[i].navigated = false
	m.previewData = PreviewData{}
	m.previewLoading = false
}

// prevField moves focus to the previous parameter (left arrow), wrapping to the
// last one from the first. Returns false when there is only one field.
func (m *formModel) prevField() bool {
	if len(m.fields) < 2 {
		return false
	}
	m.cur().commitOnLeave()
	i := m.current - 1
	if i < 0 {
		i = len(m.fields) - 1
	}
	m.gotoField(i)
	return true
}

// nextField moves focus to the next parameter (right arrow), wrapping to the
// first from the last. Returns false when there is only one field.
func (m *formModel) nextField() bool {
	if len(m.fields) < 2 {
		return false
	}
	m.cur().commitOnLeave()
	i := m.current + 1
	if i >= len(m.fields) {
		i = 0
	}
	m.gotoField(i)
	return true
}

// advanceField moves focus to the next parameter without wrapping, committing
// the current single-value field on leave. It returns false when already on the
// last parameter (the caller then runs the command). Unlike prevField/nextField
// (the wrapping ←/→ navigation), this is the linear Enter progression.
func (m *formModel) advanceField() bool {
	if m.current >= len(m.fields)-1 {
		return false
	}
	m.cur().commitOnLeave()
	m.gotoField(m.current + 1)
	return true
}

// onLastField reports whether focus is on the final parameter.
func (m *formModel) onLastField() bool {
	return m.current == len(m.fields)-1
}

// requiredIndices returns the indices of the required parameters, in order.
func (m *formModel) requiredIndices() []int {
	var idx []int
	for i, fs := range m.fields {
		if !fs.field.Optional {
			idx = append(idx, i)
		}
	}
	return idx
}

// nextRequiredField jumps focus to the next required parameter after the
// current one (wrapping), committing the current single-value field on leave.
// Returns false when there are fewer than two required params to move between.
func (m *formModel) nextRequiredField() bool {
	req := m.requiredIndices()
	if len(req) < 2 {
		return false
	}
	m.cur().commitOnLeave()
	for _, i := range req {
		if i > m.current {
			m.gotoField(i)
			return true
		}
	}
	m.gotoField(req[0]) // wrap to the first required param
	return true
}

// prevRequiredField jumps focus to the previous required parameter before the
// current one (wrapping), committing the current single-value field on leave.
func (m *formModel) prevRequiredField() bool {
	req := m.requiredIndices()
	if len(req) < 2 {
		return false
	}
	m.cur().commitOnLeave()
	for k := len(req) - 1; k >= 0; k-- {
		if req[k] < m.current {
			m.gotoField(req[k])
			return true
		}
	}
	m.gotoField(req[len(req)-1]) // wrap to the last required param
	return true
}

// firstInvalid returns the index of the first field that fails validation
// (strict, pattern, or required presence) with its error message, or -1 when
// every field is valid.
func (m *formModel) firstInvalid() (index int, msg string) {
	for i, fs := range m.fields {
		if msg := fs.validationError(); msg != "" {
			return i, msg
		}
		if fs.field.Optional {
			continue
		}
		if _, ok := fs.value(); !ok {
			return i, "fill in required parameter: " + fs.field.Name
		}
	}
	return -1, ""
}

// firstStale returns the index of the first field whose candidates must be
// refetched before the command can be validated, or -1. That is a list field
// whose candidates are missing or out of date for the current values (a
// dependency changed since the fetch) and whose value depends on them: a
// strict field holding a value, or an untouched dependent field whose prefill
// came from the stale fetch.
func (m *formModel) firstStale() int {
	resolved := m.rawValues()
	for i, fs := range m.fields {
		if fs.skipped || !fs.isListField() || !fs.needsFetch(resolved) {
			continue
		}
		_, hasValue := fs.rawValue()
		if fs.field.Spec.Strict && hasValue {
			return i
		}
		if len(fs.deps) > 0 && fs.loaded && !fs.touched {
			return i
		}
	}
	return -1
}
