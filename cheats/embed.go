// Package cheats holds commando's built-in cheat packs. They are embedded in
// the binary and seeded into the user's cheats directory on first run, and the
// same files can be added as a cheat source straight from the repository:
//
//	commando cheats add https://github.com/liliumapps/commando cheats/docker.toml
package cheats

import "embed"

// FS holds every *.toml pack in this directory, at the root of the FS.
//
//go:embed *.toml
var FS embed.FS
