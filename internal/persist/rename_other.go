//go:build !windows

package persist

import "os"

// renameFile atomically replaces newpath with oldpath.
func renameFile(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }
