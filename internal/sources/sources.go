// Package sources manages cheat sources: git repositories (or one file in a
// repository) whose cheat files commando loads next to the user's own. The
// list lives in sources.toml in the config directory; each repository is
// shallow-cloned once under <DataDir>/repos and shared by every source that
// names it.
//
// Cheats can run shell code (a variable's src and preview), so a source is
// only loaded at the commit the user reviewed and trusted, which is recorded
// in sources.toml. A clone at any other commit is skipped with a diagnostic.
package sources

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/liliumapps/commando/internal/config"
	"github.com/liliumapps/commando/internal/persist"
)

// Source is one [[source]] entry in sources.toml.
type Source struct {
	// URL is the git remote: https://, ssh://, or user@host:path.
	URL string `toml:"url"`
	// File is a path relative to the repository root naming the one cheat
	// file to load. Empty loads every *.toml in the repository.
	File string `toml:"file,omitempty"`
	// Commit is the reviewed commit; only this commit is loaded. Empty means
	// the source has not been reviewed yet (run `commando cheats update`).
	Commit string `toml:"commit,omitempty"`
	// Updated is when the source was last added or updated (RFC 3339).
	Updated string `toml:"updated,omitempty"`
}

// Key identifies a source within its repository: the URL, plus "#file" for a
// single-file source. It prefixes the identity of every cheat it loads.
func (s Source) Key() string {
	if s.File == "" {
		return s.URL
	}
	return s.URL + "#" + s.File
}

// Label is a short human name: the repository name for a whole-repository
// source ("team-cheats"), or the repository name and file name for a single
// file ("commando/docker.toml").
func (s Source) Label() string {
	name := repoName(s.URL)
	if s.File == "" {
		return name
	}
	return name + "/" + path.Base(s.File)
}

// Scope describes what the source loads, for listings: the file, or "all".
func (s Source) Scope() string {
	if s.File == "" {
		return "all"
	}
	return s.File
}

func (s Source) same(o Source) bool { return s.URL == o.URL && s.File == o.File }

// validate checks the fields of a source read from or about to be written to
// sources.toml.
func (s Source) validate() error {
	if err := ValidateURL(s.URL); err != nil {
		return err
	}
	if s.File != "" {
		if err := ValidateFile(s.File); err != nil {
			return err
		}
	}
	if s.Commit != "" && !commitRE.MatchString(s.Commit) {
		return fmt.Errorf("commit %q is not a full hexadecimal commit id", s.Commit)
	}
	return nil
}

var commitRE = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// allowFileURLs lets tests use file:// remotes (local bare repositories). It
// is never set outside tests.
var allowFileURLs = false

var scpRE = regexp.MustCompile(`^[A-Za-z0-9._~-]+@[A-Za-z0-9][A-Za-z0-9.-]*:[^:]+$`)

// ValidateURL accepts https:// and ssh remotes (ssh://... or user@host:path)
// and rejects everything else: local paths, file://, git transport helpers
// such as ext::, plain http, and option-like or whitespace-containing text.
func ValidateURL(raw string) error {
	if raw == "" {
		return errors.New("a repository URL is required")
	}
	if strings.TrimSpace(raw) != raw || strings.HasPrefix(raw, "-") || strings.ContainsAny(raw, " \t\r\n\\") || hasControl(raw) {
		return fmt.Errorf("invalid repository URL %q", raw)
	}
	if strings.Contains(raw, "::") {
		return fmt.Errorf("unsupported repository URL %q: git transport helpers are not allowed", raw)
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "https://"), strings.HasPrefix(lower, "ssh://"):
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("invalid repository URL %q: %w", raw, err)
		}
		if u.Hostname() == "" || strings.HasPrefix(u.Hostname(), "-") || strings.HasPrefix(u.User.Username(), "-") {
			return fmt.Errorf("invalid repository URL %q: missing host", raw)
		}
		if strings.Trim(u.Path, "/") == "" {
			return fmt.Errorf("invalid repository URL %q: missing repository path", raw)
		}
		if _, hasPassword := u.User.Password(); hasPassword {
			return fmt.Errorf("repository URL must not contain a password; use a git credential helper or ssh key")
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("invalid repository URL %q: query and fragment are not allowed", raw)
		}
		return nil
	case scpRE.MatchString(raw):
		return nil
	case allowFileURLs && strings.HasPrefix(lower, "file://"):
		return nil
	}
	return fmt.Errorf("unsupported repository URL %q: use https://, ssh://, or user@host:path", raw)
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// ValidateFile checks a source's file: a clean, relative, slash-separated path
// inside the repository that ends in .toml and does not enter .git.
func ValidateFile(file string) error {
	switch {
	case file == "":
		return errors.New("file must not be empty")
	case strings.ContainsAny(file, "\\:") || hasControl(file):
		return fmt.Errorf("file %q: use a relative path with forward slashes", file)
	case strings.HasPrefix(file, "/") || filepath.IsAbs(file):
		return fmt.Errorf("file %q must be relative to the repository root", file)
	case path.Clean(file) != file:
		return fmt.Errorf("file %q is not a clean path", file)
	case !strings.EqualFold(path.Ext(file), ".toml"):
		return fmt.Errorf("file %q must end in .toml", file)
	}
	for _, part := range strings.Split(file, "/") {
		if part == ".." || part == "." {
			return fmt.Errorf("file %q must stay inside the repository", file)
		}
		if strings.EqualFold(part, ".git") {
			return fmt.Errorf("file %q must not be inside .git", file)
		}
	}
	return nil
}

// repoName is the last path element of a remote without a .git suffix.
func repoName(remote string) string {
	s := strings.TrimRight(remote, "/")
	s = strings.TrimSuffix(s, ".git")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		s = s[i+1:]
	}
	if s == "" {
		return "repo"
	}
	return s
}

// SourcesPath is the location of sources.toml.
func SourcesPath(p config.Paths) string { return filepath.Join(p.ConfigDir, "sources.toml") }

// ReposDir holds one clone per source URL.
func ReposDir(p config.Paths) string { return filepath.Join(p.DataDir, "repos") }

var unsafeNameRE = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// CloneDir is the clone location for a URL: a readable slug of the host and
// path followed by a hash of the full URL, so distinct URLs never collide.
func CloneDir(p config.Paths, remote string) string {
	slug := remote
	if i := strings.Index(slug, "://"); i >= 0 {
		slug = slug[i+3:]
	}
	if i := strings.LastIndex(slug, "@"); i >= 0 {
		slug = slug[i+1:]
	}
	slug = strings.Trim(unsafeNameRE.ReplaceAllString(slug, "-"), "-.")
	if len(slug) > 48 {
		slug = slug[:48]
	}
	sum := sha256.Sum256([]byte(remote))
	return filepath.Join(ReposDir(p), slug+"-"+hex.EncodeToString(sum[:6]))
}

type sourcesFile struct {
	Sources []Source `toml:"source"`
}

// Read loads sources.toml. A missing file is an empty list. Unknown fields
// and invalid entries are errors, so a typo is reported rather than ignored.
func Read(p config.Paths) ([]Source, error) {
	file := SourcesPath(p)
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	var sf sourcesFile
	md, err := toml.Decode(string(data), &sf)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}
	if unknown := md.Undecoded(); len(unknown) > 0 {
		names := make([]string, len(unknown))
		for i, k := range unknown {
			names[i] = k.String()
		}
		return nil, fmt.Errorf("parse %s: unknown field(s): %s", file, strings.Join(names, ", "))
	}
	for i, s := range sf.Sources {
		if err := s.validate(); err != nil {
			return nil, fmt.Errorf("%s: source #%d: %w", file, i+1, err)
		}
		for _, prev := range sf.Sources[:i] {
			if prev.same(s) {
				return nil, fmt.Errorf("%s: source #%d: %s is listed twice", file, i+1, s.Key())
			}
		}
	}
	return sf.Sources, nil
}

const fileHeader = `# Cheat sources for commando, managed by "commando cheats add|update|remove".
# Each source is a git repository (all of its *.toml files) or one file in it.
# Only the recorded commit is loaded; run "commando cheats update" to review
# and move to a newer one.

`

// Write saves the source list atomically.
func Write(p config.Paths, srcs []Source) error {
	var buf bytes.Buffer
	buf.WriteString(fileHeader)
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	if err := enc.Encode(sourcesFile{Sources: srcs}); err != nil {
		return fmt.Errorf("encode sources: %w", err)
	}
	return persist.WriteFile(SourcesPath(p), buf.Bytes(), 0o644)
}
