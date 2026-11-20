// Package persist holds small helpers for reading and writing commando's
// file-backed state as plain TOML. Writes are atomic: a temp file in the same
// directory is written and renamed into place, so a crash mid-write never
// corrupts the existing file. Both the state and bookmark stores build on these.
package persist

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/BurntSushi/toml"
)

// ReadTOML decodes the TOML file at path into v. A missing file leaves v at its
// zero value and returns nil, so an absent file is simply empty state.
func ReadTOML(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := toml.Unmarshal(data, v); err != nil {
		return &CorruptError{Path: path, Err: err}
	}
	return nil
}

// CorruptError reports a state file that exists but cannot be decoded.
type CorruptError struct {
	Path string
	Err  error
}

func (e *CorruptError) Error() string { return fmt.Sprintf("parse %s: %v", e.Path, e.Err) }
func (e *CorruptError) Unwrap() error { return e.Err }

// Quarantine describes a corrupt file that was moved aside.
type Quarantine struct {
	Path    string // original location
	MovedTo string // "<path>.corrupt-<unix>", or "" when the move failed
	Err     error  // the decode error (and move error, if any)
}

func (q Quarantine) String() string {
	if q.MovedTo == "" {
		return fmt.Sprintf("%s is corrupt and could not be moved aside: %v", q.Path, q.Err)
	}
	return fmt.Sprintf("%s was corrupt and was moved to %s (%v)", q.Path, q.MovedTo, q.Err)
}

// ReadTOMLOrQuarantine is ReadTOML for state that can be rebuilt. When the
// file exists but does not decode, it is renamed to "<path>.corrupt-<unix>" so
// the data is kept for inspection, the zero value is returned (never a
// partially decoded one), and a non-nil Quarantine describes what happened.
// Read errors other than corruption are returned as errors; a missing file is
// empty state.
func ReadTOMLOrQuarantine[T any](path string, now time.Time) (T, *Quarantine, error) {
	var v T
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return v, nil, nil
		}
		return v, nil, fmt.Errorf("read %s: %w", path, err)
	}
	decodeErr := toml.Unmarshal(data, &v)
	if decodeErr == nil {
		return v, nil, nil
	}
	var zero T
	q := &Quarantine{Path: path, Err: decodeErr}
	dst := fmt.Sprintf("%s.corrupt-%d", path, now.Unix())
	if _, statErr := os.Stat(dst); statErr == nil {
		dst = fmt.Sprintf("%s.corrupt-%d", path, now.UnixNano())
	}
	if mvErr := renameFile(path, dst); mvErr != nil {
		q.Err = errors.Join(decodeErr, mvErr)
	} else {
		q.MovedTo = dst
	}
	return zero, q, nil
}

// QuarantinedFiles lists files previously moved aside for path
// ("<path>.corrupt-*"), oldest first.
func QuarantinedFiles(path string) []string {
	matches, _ := filepath.Glob(path + ".corrupt-*")
	sort.Strings(matches)
	return matches
}

// IsCorrupt reports whether err came from a file that exists but does not
// decode.
func IsCorrupt(err error) bool {
	var ce *CorruptError
	return errors.As(err, &ce)
}

// WriteTOML encodes v to path atomically: it creates the parent directory if
// needed, writes a temp file alongside it, then renames it into place, so a
// crash mid-write never corrupts the existing file.
func WriteTOML(path string, v any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	if err := toml.NewEncoder(tmp).Encode(v); err != nil {
		tmp.Close()
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := renameFile(tmpName, path); err != nil {
		return fmt.Errorf("finalize %s: %w", path, err)
	}
	syncDir(dir)
	return nil
}

// WriteFile writes data to path atomically with the requested permissions.
// The replacement is assembled beside the destination so the final rename
// cannot cross filesystems.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := renameFile(tmpName, path); err != nil {
		return fmt.Errorf("finalize %s: %w", path, err)
	}
	syncDir(dir)
	return nil
}

// WithFileLock serializes a read-modify-write operation across processes.
// Locking a stable sidecar rather than the destination itself keeps the lock
// valid while the destination is atomically replaced by rename.
func WithFileLock(path string, fn func() error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("lock %s: %w", path, err)
	}
	unlock, err := acquireFileLock(path + ".lock")
	if err != nil {
		return fmt.Errorf("lock %s: %w", path, err)
	}
	defer unlock()
	return fn()
}

// syncDir makes the rename durable on filesystems that support directory
// fsync. Some platforms reject directory Sync, so this remains best-effort.
func syncDir(dir string) {
	f, err := os.Open(dir)
	if err != nil {
		return
	}
	defer f.Close()
	_ = f.Sync()
}
