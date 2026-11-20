package sources

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/liliumapps/commando/internal/config"
	"github.com/liliumapps/commando/internal/entry"
)

// maxCheatFileSize bounds how much of a repository file is read.
const maxCheatFileSize = 1 << 20

// loaded is one cheat from a source, with its author-declared id (empty when
// the cheat has none) kept so a user cheat with the same id can override it.
type loaded struct {
	entry    entry.Entry
	authorID string
}

// LoadCheats returns the user's own cheats (config.LoadCheats, identities
// unchanged) followed by the cheats of every source in sources.toml. Problems
// with one source or file are reported in the joined error while everything
// else still loads.
func LoadCheats(p config.Paths) ([]entry.Entry, error) {
	entries, userErr := config.LoadCheats(p)
	errs := []error{userErr}
	userIDs := make(map[string]bool)
	for _, e := range entries {
		if e.StableID != "" {
			userIDs[e.StableID] = true
		}
	}

	srcs, err := Read(p)
	if err != nil {
		return entries, errors.Join(userErr, err)
	}
	for _, s := range srcs {
		got, err := loadTrusted(p, s)
		if err != nil {
			errs = append(errs, err)
		}
		for _, l := range got {
			if l.authorID != "" && userIDs[l.authorID] {
				continue // the user's cheat with this id wins
			}
			entries = append(entries, l.entry)
		}
	}
	return entries, errors.Join(errs...)
}

// loadTrusted loads a source from its clone, but only when the clone is at
// the commit recorded in sources.toml.
func loadTrusted(p config.Paths, s Source) ([]loaded, error) {
	if s.Commit == "" {
		return nil, fmt.Errorf("source %s: not reviewed yet; run `commando cheats update`", s.Key())
	}
	dir := CloneDir(p, s.URL)
	if !isClone(dir) {
		return nil, fmt.Errorf("source %s: clone is missing; run `commando cheats update`", s.Key())
	}
	head, err := gitHead(dir)
	if err != nil {
		return nil, fmt.Errorf("source %s: %w", s.Key(), err)
	}
	if head != s.Commit {
		return nil, fmt.Errorf("source %s: clone is at %s but %s was reviewed; run `commando cheats update` to review it", s.Key(), short(head), short(s.Commit))
	}
	got, errs := loadScope(dir, s)
	return got, errors.Join(errs...)
}

// loadScope loads the cheats a source names from a checkout in dir, without
// any commit check. Each file that fails (parse error, symlink, a duplicate
// id) is reported and skipped; the rest load.
func loadScope(dir string, s Source) ([]loaded, []error) {
	files, errs := scopeFiles(dir, s)
	var out []loaded
	ids := make(map[string]string) // author id -> file that declared it
	for _, rel := range files {
		data, err := readInside(dir, rel)
		if err != nil {
			errs = append(errs, fmt.Errorf("source %s: %s: %w", s.Key(), rel, err))
			continue
		}
		es, err := config.LoadCheatData(s.Label()+":"+rel, data)
		if err != nil {
			errs = append(errs, fmt.Errorf("source %s: %w", s.Key(), err))
			continue
		}
		var dup error
		for _, e := range es {
			if prev, ok := ids[e.StableID]; ok && e.StableID != "" {
				dup = fmt.Errorf("source %s: %s: id %q is already used in %s; file skipped", s.Key(), rel, e.StableID, prev)
				break
			}
		}
		if dup != nil {
			errs = append(errs, dup)
			continue
		}
		for _, e := range es {
			authorID := e.StableID
			if authorID != "" {
				ids[authorID] = rel
			}
			out = append(out, loaded{entry: prefixed(s, e), authorID: authorID})
		}
	}
	return out, errs
}

// prefixed gives a source cheat an identity of its own,
// "src:<url>[#<file>]/<id or hash of title and template>", so its memory and
// usage never collide with the user's cheats or another source's.
func prefixed(s Source, e entry.Entry) entry.Entry {
	local := e.StableID
	if local == "" {
		local = e.ID() // hash of the title and template
	}
	e.StableID = "src:" + s.Key() + "/" + local
	e.Origin = s.Label()
	return e
}

// scopeFiles lists the slash-separated paths of the cheat files a source
// loads: its one file, or every *.toml in the repository outside dot
// directories. Symlinks are never followed.
func scopeFiles(dir string, s Source) ([]string, []error) {
	if s.File != "" {
		return []string{s.File}, nil
	}
	var files []string
	var errs []error
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if d.IsDir() {
			if p != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".toml") {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}
	sort.Strings(files)
	return files, errs
}

// readInside reads rel from the clone rooted at dir, refusing anything that is
// not a regular file at exactly that location: a symlink, or a path that
// resolves outside the clone through a symlinked directory.
func readInside(dir, rel string) ([]byte, error) {
	if err := ValidateFile(rel); err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	full := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("file not found in the repository")
		}
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("is a symlink; refusing to load it")
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("is not a regular file")
	}
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return nil, err
	}
	if resolved != full {
		return nil, errors.New("resolves outside the repository through a symlink; refusing to load it")
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxCheatFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxCheatFileSize {
		return nil, fmt.Errorf("is larger than %d bytes", maxCheatFileSize)
	}
	return data, nil
}

// Status summarizes the configured sources for `commando doctor`: how many
// there are, how many commands they load, and every problem found (missing
// clone, unreviewed or mismatched commit, load errors).
func Status(p config.Paths) (count, commands int, problems []error) {
	srcs, err := Read(p)
	if err != nil {
		return 0, 0, []error{err}
	}
	for _, s := range srcs {
		got, err := loadTrusted(p, s)
		commands += len(got)
		if err != nil {
			problems = append(problems, err)
		}
	}
	return len(srcs), commands, problems
}
