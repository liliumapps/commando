//go:build !unix && !windows

package persist

import "sync"

var fallbackLocks sync.Map

func acquireFileLock(path string) (func(), error) {
	value, _ := fallbackLocks.LoadOrStore(path, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock, nil
}
