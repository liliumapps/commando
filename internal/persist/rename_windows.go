//go:build windows

package persist

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// renameRetries bounds how long a rename waits for a transient sharing
// violation to clear.
const renameRetries = 10

// renameFile replaces newpath with oldpath. On Windows a concurrent reader that
// opened the destination without FILE_SHARE_DELETE (os.ReadFile, an editor, an
// indexer or antivirus scan) makes MoveFileEx fail with ERROR_ACCESS_DENIED or
// ERROR_SHARING_VIOLATION until it closes the file, so retry briefly with
// backoff before giving up.
func renameFile(oldpath, newpath string) error {
	var err error
	delay := 5 * time.Millisecond
	for attempt := 0; attempt < renameRetries; attempt++ {
		if err = os.Rename(oldpath, newpath); err == nil {
			return nil
		}
		if !os.IsPermission(err) && !isSharingViolation(err) {
			return err
		}
		time.Sleep(delay)
		if delay < 100*time.Millisecond {
			delay *= 2
		}
	}
	return err
}

func isSharingViolation(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
