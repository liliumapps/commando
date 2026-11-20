// Package cheatfile provides a Provider backed by the cheat files in the user's
// cheats directory. Loading and validation live in the config package; this
// provider simply surfaces the loaded entries.
package cheatfile

import (
	"github.com/liliumapps/commando/internal/config"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/provider"
)

// Provider surfaces cheat-file entries. It reloads on each call so that edits to
// cheat files take effect without restarting the tool.
type Provider struct {
	paths config.Paths
}

// New returns a cheatfile provider reading from the given paths.
func New(paths config.Paths) *Provider {
	return &Provider{paths: paths}
}

// Name implements provider.Provider.
func (p *Provider) Name() string { return "cheatfile" }

// Entries implements provider.Provider. The Context is unused: cheat entries are
// independent of the working directory.
func (p *Provider) Entries(_ provider.Context) ([]entry.Entry, error) {
	return config.LoadCheats(p.paths)
}
