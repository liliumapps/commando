package config

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/liliumapps/commando/internal/persist"
)

// sampleCheats embeds the built-in cheat files shipped with commando so that a
// fresh installation has useful content without any manual setup.
//
//go:embed cheats/*.toml
var sampleCheats embed.FS

type sampleManifest struct {
	Files []string `toml:"file"`
}

// SampleFiles returns the base names of the embedded sample cheat files.
func SampleFiles() ([]string, error) {
	entries, err := fs.ReadDir(sampleCheats, "cheats")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

// SampleData returns the raw bytes of an embedded sample cheat file by base name.
func SampleData(name string) ([]byte, error) {
	return sampleCheats.ReadFile(filepath.Join("cheats", name))
}

// SeedCheats offers each embedded sample file once. A manifest records samples
// that have already been seen, so deleting a sample is respected while newly
// shipped samples can still be added on upgrade.
func SeedCheats(p Paths) ([]string, error) {
	if err := p.EnsureDirs(); err != nil {
		return nil, err
	}
	names, err := SampleFiles()
	if err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(p.ConfigDir, ".sample-manifest.toml")
	var manifest sampleManifest
	if err := persist.ReadTOML(manifestPath, &manifest); err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(manifest.Files))
	for _, name := range manifest.Files {
		seen[name] = true
	}

	var written []string
	for _, name := range names {
		if seen[name] {
			continue
		}
		dst := filepath.Join(p.CheatsDir, name)
		if _, err := os.Stat(dst); err == nil {
			// Existing installations may predate the manifest. Adopt the file
			// without replacing it.
		} else if !os.IsNotExist(err) {
			return written, err
		} else {
			data, err := SampleData(name)
			if err != nil {
				return written, err
			}
			if err := persist.WriteFile(dst, data, 0o644); err != nil {
				return written, err
			}
			written = append(written, name)
		}
		seen[name] = true
	}

	manifest.Files = manifest.Files[:0]
	for name := range seen {
		manifest.Files = append(manifest.Files, name)
	}
	sort.Strings(manifest.Files)
	if err := persist.WriteTOML(manifestPath, manifest); err != nil {
		return written, err
	}
	return written, nil
}
