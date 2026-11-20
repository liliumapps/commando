// Package tui implements commando's interactive terminal UI: a fuzzy-searchable
// list of entries followed by a fill-in form that resolves each variable with
// memory-ranked candidates, optional-field skipping, multi-select, and a live
// preview of the assembling command.
//
// The UI is deliberately decoupled from persistence and execution through the
// Backend interface, which supplies the dynamic data (candidate values, memory)
// the form needs. This keeps the TUI unit-testable with a fake backend.
package tui

import (
	"context"

	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/rank"
	"github.com/liliumapps/commando/internal/resolve"
)

type refreshKey struct{}

// WithRefresh marks ctx as a user-requested refresh (Ctrl-R): backends should
// bypass any cached candidate output and re-run the source command.
func WithRefresh(ctx context.Context) context.Context {
	return context.WithValue(ctx, refreshKey{}, true)
}

// IsRefresh reports whether ctx was marked by WithRefresh.
func IsRefresh(ctx context.Context) bool {
	v, _ := ctx.Value(refreshKey{}).(bool)
	return v
}

// FieldData is everything the form needs to render one variable field.
type FieldData struct {
	// Candidates are the selectable values with their usage data, de-duplicated
	// (remembered values first, then src values). Ordering per query is applied
	// by the ranker at render time, so this list need not be pre-sorted.
	Candidates []rank.Candidate
	// Prefill is the initial input value. Only an author-defined default
	// prefills; remembered values are offered as candidates, not auto-typed.
	Prefill string
	// Error is a non-fatal candidate-source failure. Remembered and inline
	// candidates may still be present.
	Error string
	// Truncated reports that the source exceeded the configured output limit.
	Truncated bool
}

// PreviewData is the asynchronous result of a candidate preview.
type PreviewData struct {
	Text      string
	Error     string
	Truncated bool
}

// Backend supplies the dynamic data the TUI needs during resolution. The engine
// implements it against the store and shell; tests provide a fake.
type Backend interface {
	// FieldData returns candidates and a prefill for a field of an entry.
	// resolved holds the raw values already chosen for other variables, keyed by
	// name, so a field's Src command can reference them (e.g. a src of
	// "kubectl get pods -n <namespace>" is run with <namespace> substituted). It is called
	// when a field is entered, and again if one of its referenced variables
	// changes.
	FieldData(ctx context.Context, e entry.Entry, f resolve.Field, resolved map[string]string) FieldData
	// Remember persists a chosen value so it can be recalled next time. It is
	// called once per resolved field when the user confirms a command.
	Remember(e entry.Entry, f resolve.Field, value string)
	// Preview runs the field's Preview command for the highlighted candidate and
	// returns its output for display. "{}" in the command is replaced by
	// candidate, and other <vars> are substituted from resolved. Implementations
	// should honor cancellation and cache by candidate.
	Preview(ctx context.Context, e entry.Entry, f resolve.Field, candidate string, resolved map[string]string) PreviewData
	// Bookmark saves the given command as a bookmark. It is invoked when the
	// user presses the bookmark key on a list entry. A nil error means saved.
	Bookmark(cmd string) error
	// RemoveBookmark deletes an exact bookmarked command.
	RemoveBookmark(cmd string) error
}

// nopBackend is a Backend that offers no candidates and remembers nothing. It is
// used when the caller has no store available (e.g. a dry run).
type nopBackend struct{}

func (nopBackend) FieldData(_ context.Context, _ entry.Entry, f resolve.Field, _ map[string]string) FieldData {
	return FieldData{Prefill: f.Spec.Default}
}
func (nopBackend) Remember(_ entry.Entry, _ resolve.Field, _ string) {}
func (nopBackend) Preview(_ context.Context, _ entry.Entry, _ resolve.Field, _ string, _ map[string]string) PreviewData {
	return PreviewData{}
}
func (nopBackend) Bookmark(_ string) error       { return nil }
func (nopBackend) RemoveBookmark(_ string) error { return nil }
