package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/resolve"
	"github.com/liliumapps/commando/internal/tui"
)

// TestFieldDataInlineValues checks that inline `values` become candidates
// (projected through label/value like src lines) without any shell command.
func TestFieldDataInlineValues(t *testing.T) {
	b := &fileBackend{}
	spec := entry.VarSpec{
		Name:   "env",
		Values: []string{"dev", "staging", "prod"},
	}
	fd := b.FieldData(context.Background(), entry.Entry{Template: "deploy <env>"}, resolve.Field{Name: "env", Spec: spec}, nil)
	got := candValues(fd)
	want := []string{"dev", "staging", "prod"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("inline values = %v, want %v", got, want)
	}
}

// TestFieldDataValuesWithExtraction checks that extraction (columns/label/value)
// applies to inline values too, and that HeaderLines does NOT drop an inline
// value (inline lists are already clean).
func TestFieldDataValuesWithExtraction(t *testing.T) {
	b := &fileBackend{}
	spec := entry.VarSpec{
		Name:        "pid",
		Values:      []string{"1234 chrome", "5678 postgres"},
		HeaderLines: 1, // must be ignored for inline values
		Columns:     []string{"pid", "comm"},
		Label:       "<comm> (<pid>)",
		Value:       "<pid>",
	}
	fd := b.FieldData(context.Background(), entry.Entry{Template: "kill <pid>"}, resolve.Field{Name: "pid", Spec: spec}, nil)
	if got, want := candValues(fd), []string{"1234", "5678"}; !reflect.DeepEqual(got, want) {
		t.Errorf("values = %v, want %v", got, want)
	}
	if got, want := candDisplays(fd), []string{"chrome (1234)", "postgres (5678)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("displays = %v, want %v", got, want)
	}
}

// TestRunSrcRawCaches checks that a Src command is executed once within its
// cache TTL and re-executed after it expires.
func TestRunSrcRawCaches(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "count")
	// Each run appends a line to the counter file, so its line count == runs.
	src := "printf 'x\\n' >> " + counter + "; cat " + counter

	now := time.Unix(1000, 0)
	b := &fileBackend{now: func() time.Time { return now }}

	runs := func() int {
		data, err := os.ReadFile(counter)
		if err != nil {
			return 0
		}
		n := 0
		for _, c := range data {
			if c == '\n' {
				n++
			}
		}
		return n
	}

	// First call populates the cache.
	if _, ok := b.runSrcRaw(src, "5m"); !ok {
		t.Fatal("first runSrcRaw failed")
	}
	if runs() != 1 {
		t.Fatalf("after first call, runs = %d, want 1", runs())
	}
	// Second call within TTL must hit the cache (no new run).
	if _, ok := b.runSrcRaw(src, "5m"); !ok {
		t.Fatal("second runSrcRaw failed")
	}
	if runs() != 1 {
		t.Fatalf("cached call re-ran command: runs = %d, want 1", runs())
	}
	// Advance past the TTL: the command runs again.
	now = now.Add(6 * time.Minute)
	if _, ok := b.runSrcRaw(src, "5m"); !ok {
		t.Fatal("third runSrcRaw failed")
	}
	if runs() != 2 {
		t.Fatalf("after TTL expiry, runs = %d, want 2", runs())
	}
	// A Ctrl-R refresh bypasses the still-valid cache entry.
	if _, _, ok := b.runSrcRawContext(tui.WithRefresh(context.Background()), src, "5m"); !ok {
		t.Fatal("refresh runSrcRaw failed")
	}
	if runs() != 3 {
		t.Fatalf("refresh served the cache: runs = %d, want 3", runs())
	}
}

// TestRunSrcRawNoCache checks that with no cache duration the command runs every
// time.
func TestRunSrcRawNoCache(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "count")
	src := "printf 'x\\n' >> " + counter + "; cat " + counter
	b := &fileBackend{}
	for i := 0; i < 3; i++ {
		if _, ok := b.runSrcRaw(src, ""); !ok {
			t.Fatalf("run %d failed", i)
		}
	}
	data, _ := os.ReadFile(counter)
	if got := len(data); got != 6 { // "x\n" * 3
		t.Errorf("uncached command ran wrong number of times, file = %q", data)
	}
}

func candValues(fd tui.FieldData) []string {
	out := make([]string, len(fd.Candidates))
	for i, c := range fd.Candidates {
		out[i] = c.Value
	}
	return out
}

func candDisplays(fd tui.FieldData) []string {
	out := make([]string, len(fd.Candidates))
	for i, c := range fd.Candidates {
		out[i] = c.Display
	}
	return out
}
