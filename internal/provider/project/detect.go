// Package project provides a Provider that discovers runnable commands from the
// project surrounding the current directory: Makefile targets, package.json
// scripts, Cargo/Gradle/just tasks, docker-compose services, and git actions.
//
// Detection walks up from the working directory to the git root (or filesystem
// root) looking for marker files. Built-in detectors parse well-known files
// directly in Go rather than shelling out, so they work even when tools like
// grep or jq are unavailable and cannot be broken by unusual file contents.
package project

import (
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// found is a marker file located during the upward walk.
type found struct {
	dir    string // directory containing the marker (normalized)
	marker string // marker file name
	path   string // full path to the marker
	cwd    string // normalized directory the walk started from
	stop   string // normalized walk boundary (git root), "" for none
}

// dirRef returns how to refer to f.dir from the working directory: "" when the
// marker is in the working directory itself, otherwise the shorter of the
// relative and absolute paths (slash-separated, unquoted).
func (f found) dirRef() string {
	if f.cwd == "" || samePath(f.dir, f.cwd) {
		return ""
	}
	p := f.dir
	if rel, err := filepath.Rel(f.cwd, f.dir); err == nil && len(rel) <= len(f.dir) {
		p = rel
	}
	return filepath.ToSlash(p)
}

// relDir is dirRef shell-quoted for use in a command line.
func (f found) relDir() string {
	if ref := f.dirRef(); ref != "" {
		return quoteProviderItem(ref)
	}
	return ""
}

// relPath returns the shell-quoted path of a file in the marker's directory as
// seen from the working directory ("name" when they are the same).
func (f found) relPath(name string) string {
	return quoteProviderItem(path.Join(f.dirRef(), name))
}

// normalizeDir cleans p and resolves symlinks so paths from different sources
// compare equal: git reports real paths (/private/tmp on macOS) while the
// shell's cwd may go through a symlink (/tmp), and Windows paths may differ in
// separator style (C:/x vs C:\x). When resolution fails the cleaned path is
// used.
func normalizeDir(p string) string {
	if p == "" {
		return ""
	}
	clean := filepath.Clean(filepath.FromSlash(p))
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		return filepath.Clean(resolved)
	}
	return clean
}

// samePath compares normalized paths, case-insensitively on Windows.
func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// within reports whether dir is root or a descendant of it (both normalized).
func within(dir, root string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// findMarkers walks upward from startDir collecting the first occurrence of each
// distinct marker name. The walk stops after the directory that is (or contains)
// stopAt, so detection is scoped to the enclosing project rather than climbing
// into unrelated parent directories; it never climbs above stopAt. When
// stopAt is empty the walk climbs to the filesystem root. Both paths are
// normalized (Clean + EvalSymlinks) before comparing.
func findMarkers(startDir, stopAt string, markers map[string]bool) []found {
	start := normalizeDir(startDir)
	stop := normalizeDir(stopAt)
	if stop != "" && !within(start, stop) && !samePath(start, stop) {
		// The boundary does not contain the start directory (inconsistent
		// context); scan only the start directory rather than risk walking into
		// unrelated parents.
		stop = start
	}
	seen := make(map[string]found)
	dir := start
	for {
		for name := range markers {
			if _, ok := seen[name]; ok {
				continue
			}
			p := filepath.Join(dir, name)
			if fileExists(p) {
				seen[name] = found{dir: dir, marker: name, path: p, cwd: start, stop: stop}
			}
		}
		if stop != "" && samePath(dir, stop) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}

	out := make([]found, 0, len(seen))
	for _, f := range seen {
		out = append(out, f)
	}
	return out
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
