package persist

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiskCacheRoundTripAndExpiry(t *testing.T) {
	c := NewDiskCache(t.TempDir())
	clock := time.Unix(1000, 0)
	c.SetClock(func() time.Time { return clock })

	if err := c.Put("ls", "/a", []string{"x", "y"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	var got []string
	if _, ok := c.Get("ls", "/a", time.Minute, &got); !ok || len(got) != 2 {
		t.Fatalf("hit = %v, %v", ok, got)
	}
	if _, ok := c.Get("ls", "/b", time.Minute, &got); ok {
		t.Error("different dir must miss")
	}
	// Shortening the configured ttl takes effect for existing records.
	clock = clock.Add(30 * time.Second)
	if _, ok := c.Get("ls", "/a", 10*time.Second, &got); ok {
		t.Error("record older than the current ttl must miss")
	}
	clock = clock.Add(time.Minute)
	if _, ok := c.Get("ls", "/a", time.Minute, &got); ok {
		t.Error("expired record must miss")
	}
	if m, _ := filepath.Glob(filepath.Join(c.dir, "*.json")); len(m) != 0 {
		t.Errorf("expired record not removed: %v", m)
	}
}

func TestDiskCacheCorruptIsMiss(t *testing.T) {
	c := NewDiskCache(t.TempDir())
	if err := c.Put("ls", "/a", "v", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.path("ls", "/a"), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	var v string
	if _, ok := c.Get("ls", "/a", time.Hour, &v); ok {
		t.Error("corrupt record must miss")
	}
	var nilCache *DiskCache
	if _, ok := nilCache.Get("ls", "/a", time.Hour, &v); ok || nilCache.Put("ls", "/a", "v", time.Hour) != nil {
		t.Error("nil cache must be a no-op")
	}
}

func TestReadTOMLOrQuarantine(t *testing.T) {
	type doc struct {
		Name string `toml:"name"`
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "x.toml")
	if v, q, err := ReadTOMLOrQuarantine[doc](path, time.Unix(5, 0)); err != nil || q != nil || v.Name != "" {
		t.Fatalf("missing file: %v %v %v", v, q, err)
	}
	os.WriteFile(path, []byte(`name = "ok"`), 0o644)
	if v, q, err := ReadTOMLOrQuarantine[doc](path, time.Unix(5, 0)); err != nil || q != nil || v.Name != "ok" {
		t.Fatalf("valid file: %v %v %v", v, q, err)
	}
	os.WriteFile(path, []byte(`name = "unterminated`), 0o644)
	v, q, err := ReadTOMLOrQuarantine[doc](path, time.Unix(5, 0))
	if err != nil || q == nil || v.Name != "" {
		t.Fatalf("corrupt file: %v %v %v", v, q, err)
	}
	if q.MovedTo != path+".corrupt-5" {
		t.Errorf("MovedTo = %q", q.MovedTo)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("corrupt file should be moved away")
	}
	if got := QuarantinedFiles(path); len(got) != 1 {
		t.Errorf("QuarantinedFiles = %v", got)
	}
	if err := ReadTOML(q.MovedTo, &doc{}); !IsCorrupt(err) {
		t.Errorf("ReadTOML on corrupt file: IsCorrupt(%v) = false", err)
	}
}
