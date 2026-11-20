// Package bookmark manages user-saved commands stored in a plain TOML file.
// A bookmark is a command plus optional tags and a note, all of which are
// fuzzy-searchable in the TUI, so a user can save a hard-to-remember command
// and find it later by words that describe it rather than its exact text.
package bookmark

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/persist"
	"github.com/liliumapps/commando/internal/provider"
)

// Bookmark is one saved command.
type Bookmark struct {
	Cmd     string   `toml:"cmd"`
	Tags    []string `toml:"tags,omitempty"`
	Note    string   `toml:"note,omitempty"`
	Created int64    `toml:"created,omitempty"`
}

// file is the on-disk structure of bookmarks.toml.
type file struct {
	Bookmarks []Bookmark `toml:"bookmark"`
}

// Store is a file-backed collection of bookmarks. It is also a Provider, so
// bookmarks appear in the entry list alongside cheats, project commands, and
// history.
type Store struct {
	path string
	mu   sync.Mutex
	// recovered holds corrupt-file notices not yet reported by Entries.
	recovered []string
}

// Open returns a bookmark store backed by path (bookmarks.toml). The file need
// not exist yet; it is created on the first Add.
func Open(path string) *Store { return &Store{path: path} }

// Name implements provider.Provider.
func (s *Store) Name() string { return "bookmark" }

// Entries implements provider.Provider, surfacing each bookmark as a literal
// command entry. The command is the title; note and tags remain searchable.
//
// When bookmarks.toml was corrupt it is moved aside (see load) and the notice
// is returned once as a non-fatal error alongside the (empty) entries, so the
// launcher still opens and the user learns what happened.
func (s *Store) Entries(_ provider.Context) ([]entry.Entry, error) {
	bs, err := s.List()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	notices := s.recovered
	s.recovered = nil
	s.mu.Unlock()
	out := make([]entry.Entry, 0, len(bs))
	for _, b := range bs {
		out = append(out, entry.Entry{
			Title:    b.Cmd,
			Desc:     b.Note,
			Tags:     b.Tags,
			Template: b.Cmd,
			Kind:     entry.CommandLiteral,
			Source:   entry.SourceBookmark,
		})
	}
	if len(notices) > 0 {
		return out, errors.New(strings.Join(notices, "; "))
	}
	return out, nil
}

// List returns all bookmarks, newest first. A missing file yields none.
func (s *Store) List() ([]Bookmark, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load()
	if err != nil {
		return nil, err
	}
	bs := f.Bookmarks
	sort.SliceStable(bs, func(i, j int) bool { return bs[i].Created > bs[j].Created })
	return bs, nil
}

// Add appends a bookmark and persists. A bookmark with the same command as an
// existing one updates that one's tags/note instead of duplicating.
func (s *Store) Add(b Bookmark) error {
	if strings.TrimSpace(b.Cmd) == "" {
		return fmt.Errorf("bookmark: command is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return persist.WithFileLock(s.path, func() error {
		f, err := s.load()
		if err != nil {
			return err
		}
		for i := range f.Bookmarks {
			if f.Bookmarks[i].Cmd == b.Cmd {
				if len(b.Tags) > 0 {
					f.Bookmarks[i].Tags = b.Tags
				}
				if b.Note != "" {
					f.Bookmarks[i].Note = b.Note
				}
				return s.save(f)
			}
		}
		f.Bookmarks = append(f.Bookmarks, b)
		return s.save(f)
	})
}

// Remove deletes the bookmark whose command exactly matches cmd. It reports
// whether a bookmark was removed.
func (s *Store) Remove(cmd string) (bool, error) {
	removed := false
	s.mu.Lock()
	defer s.mu.Unlock()
	err := persist.WithFileLock(s.path, func() error {
		f, err := s.load()
		if err != nil {
			return err
		}
		kept := f.Bookmarks[:0]
		for _, b := range f.Bookmarks {
			if b.Cmd == cmd {
				removed = true
				continue
			}
			kept = append(kept, b)
		}
		if !removed {
			return nil
		}
		f.Bookmarks = kept
		return s.save(f)
	})
	return removed, err
}

// load reads bookmarks.toml. A file that exists but does not parse is moved
// aside to "bookmarks.toml.corrupt-<unix>" and treated as empty, so a bad edit
// cannot make every later Add fail; the move is reported through Entries.
// Callers hold s.mu.
func (s *Store) load() (*file, error) {
	f, q, err := persist.ReadTOMLOrQuarantine[file](s.path, time.Now())
	if err != nil {
		return nil, err
	}
	if q != nil {
		s.recovered = append(s.recovered, q.String())
	}
	return &f, nil
}

func (s *Store) save(f *file) error {
	return persist.WriteTOML(s.path, f)
}
