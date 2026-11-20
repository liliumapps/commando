package config

import (
	"os"
	"path/filepath"
)

// appDir is the name of commando's directory under the user config/data roots.
const appDir = "commando"

// Paths holds the resolved filesystem locations commando uses. They are derived
// from XDG environment variables when set, falling back to the conventional
// ~/.config and ~/.local/share locations.
type Paths struct {
	// ConfigDir holds config.toml and the cheats/ subdirectory.
	ConfigDir string
	// CheatsDir holds *.toml cheat files.
	CheatsDir string
	// DataDir holds mutable state such as memory.toml, usage.toml, and data/.
	DataDir string
	// CheatDataDir holds auxiliary data files that cheat `src` commands read
	// (candidate lists, etc.). It is exposed to those commands as the
	// COMMANDO_DATA_DIR environment variable.
	CheatDataDir string
	// BookmarksPath is the bookmarks.toml file (in the config dir, since users
	// author and edit it like cheats).
	BookmarksPath string
	// CacheDir holds disposable caches (memoized `cache`-enabled src and
	// provider lister output): <user cache dir>/commando, or
	// $COMMANDO_HOME/cache/commando. It is created on first use, and empty
	// when the platform has no user cache directory.
	CacheDir string
}

// DefaultPaths resolves the standard paths for the current user. It does not
// create any directories; call EnsureDirs for that.
func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}

	configRoot := os.Getenv("XDG_CONFIG_HOME")
	if configRoot == "" {
		configRoot = filepath.Join(home, ".config")
	}
	dataRoot := os.Getenv("XDG_DATA_HOME")
	if dataRoot == "" {
		dataRoot = filepath.Join(home, ".local", "share")
	}

	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		cacheRoot = ""
	}

	// Allow a single override to relocate everything, which is convenient for
	// tests and for users who want an isolated setup.
	if root := os.Getenv("COMMANDO_HOME"); root != "" {
		configRoot = root
		dataRoot = root
		cacheRoot = filepath.Join(root, "cache")
	}
	cacheDir := ""
	if cacheRoot != "" {
		cacheDir = filepath.Join(cacheRoot, appDir)
	}

	configDir := filepath.Join(configRoot, appDir)
	dataDir := filepath.Join(dataRoot, appDir)
	return Paths{
		ConfigDir:     configDir,
		CheatsDir:     filepath.Join(configDir, "cheats"),
		DataDir:       dataDir,
		CheatDataDir:  filepath.Join(dataDir, "data"),
		BookmarksPath: filepath.Join(configDir, "bookmarks.toml"),
		CacheDir:      cacheDir,
	}, nil
}

// EnsureDirs creates the config, cheats, and data directories if they do not
// already exist.
func (p Paths) EnsureDirs() error {
	for _, d := range []string{p.ConfigDir, p.CheatsDir, p.DataDir, p.CheatDataDir} {
		if d == "" {
			continue
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
