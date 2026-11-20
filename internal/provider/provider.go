// Package provider defines the Provider interface and the Context passed to
// providers. A Provider is any source of entries: cheat files, project markers,
// or shell history. Keeping the interface tiny is deliberate — the whole design
// rests on every source reducing to a stream of entry.Entry values.
package provider

import "github.com/liliumapps/commando/internal/entry"

// Context carries the ambient information a provider may need to produce
// entries: the current working directory, the enclosing git repository root (if
// any), and a snapshot of environment variables.
type Context struct {
	Cwd     string
	GitRoot string
	Env     map[string]string
}

// Provider is a source of entries. Implementations must be safe to call with a
// zero-value Context field (e.g. GitRoot may be empty outside a repository).
type Provider interface {
	// Name is a short, stable identifier used in logs and diagnostics.
	Name() string
	// Entries returns the entries this provider currently offers for ctx. A
	// provider that finds nothing relevant (e.g. no Makefile present) returns a
	// nil slice and a nil error rather than an error.
	Entries(ctx Context) ([]entry.Entry, error)
}
