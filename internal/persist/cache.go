package persist

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// DiskCache memoizes command output across launches. Each record lives in its
// own JSON file named by sha256(command, dir) under the cache directory and
// carries an absolute expiry. Writes are atomic; an unreadable, corrupt, or
// expired record is simply a miss. Records are private to the user (0600),
// since command output can be sensitive.
type DiskCache struct {
	dir string
	now func() time.Time
}

// NewDiskCache returns a cache rooted at dir. A nil *DiskCache is valid and
// never hits, so callers can treat the disk layer as optional.
func NewDiskCache(dir string) *DiskCache {
	if dir == "" {
		return nil
	}
	return &DiskCache{dir: dir, now: time.Now}
}

// SetClock overrides the time source; intended for tests.
func (c *DiskCache) SetClock(now func() time.Time) {
	if c != nil {
		c.now = now
	}
}

type cacheRecord struct {
	Stored  int64           `json:"stored"`  // unix nanoseconds
	Expires int64           `json:"expires"` // unix nanoseconds
	Value   json.RawMessage `json:"value"`
}

// Key derives the record name for a (command, dir) pair.
func (c *DiskCache) Key(command, dir string) string {
	h := sha256.New()
	h.Write([]byte(command))
	h.Write([]byte{0})
	h.Write([]byte(dir))
	return hex.EncodeToString(h.Sum(nil))
}

func (c *DiskCache) path(command, dir string) string {
	return filepath.Join(c.dir, c.Key(command, dir)+".json")
}

// Get decodes the unexpired record for (command, dir) into v and reports its
// expiry and whether it was found. A record also misses when it is older than
// ttl (when ttl > 0), so shortening a configured cache duration takes effect
// immediately. Expired or corrupt records are removed best-effort.
func (c *DiskCache) Get(command, dir string, ttl time.Duration, v any) (time.Time, bool) {
	if c == nil {
		return time.Time{}, false
	}
	p := c.path(command, dir)
	data, err := os.ReadFile(p)
	if err != nil {
		return time.Time{}, false
	}
	var rec cacheRecord
	if err := json.Unmarshal(data, &rec); err != nil || len(rec.Value) == 0 {
		_ = os.Remove(p)
		return time.Time{}, false
	}
	now := c.now().UnixNano()
	expires := rec.Expires
	if ttl > 0 && rec.Stored+int64(ttl) < expires {
		expires = rec.Stored + int64(ttl)
	}
	if now >= expires || now < rec.Stored-int64(time.Minute) {
		_ = os.Remove(p)
		return time.Time{}, false
	}
	if err := json.Unmarshal(rec.Value, v); err != nil {
		_ = os.Remove(p)
		return time.Time{}, false
	}
	return time.Unix(0, expires), true
}

// Put stores v for (command, dir) until ttl elapses. A non-positive ttl is a
// no-op.
func (c *DiskCache) Put(command, dir string, v any, ttl time.Duration) error {
	if c == nil || ttl <= 0 {
		return nil
	}
	value, err := json.Marshal(v)
	if err != nil {
		return err
	}
	now := c.now()
	data, err := json.Marshal(cacheRecord{Stored: now.UnixNano(), Expires: now.Add(ttl).UnixNano(), Value: value})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	return WriteFile(c.path(command, dir), data, 0o600)
}
