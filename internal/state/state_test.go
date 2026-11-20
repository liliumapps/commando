package state

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestState(t *testing.T) (*State, *fakeClock) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	s.SetClock(clk.Now)
	return s, clk
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestRememberAndRecall(t *testing.T) {
	s, clk := newTestState(t)

	for i := 0; i < 3; i++ {
		if err := s.Remember("e1", "branch", "project", "main"); err != nil {
			t.Fatal(err)
		}
	}
	tsMain := clk.t.Unix()
	clk.advance(14 * 24 * time.Hour)
	if err := s.Remember("e1", "branch", "project", "feature"); err != nil {
		t.Fatal(err)
	}
	tsFeature := clk.t.Unix()

	// Recall returns both values with their usage data; ordering is the ranker's
	// job, so here we just verify the data is correct.
	vals := s.Recall("e1", "branch", "project")
	if len(vals) != 2 {
		t.Fatalf("expected 2 remembered values, got %d", len(vals))
	}
	byVal := map[string]RememberedValue{}
	for _, v := range vals {
		byVal[v.Value] = v
	}
	if byVal["main"].Freq != 3 || byVal["main"].LastN != tsMain {
		t.Errorf("main usage wrong: %+v", byVal["main"])
	}
	if byVal["feature"].Freq != 1 || byVal["feature"].LastN != tsFeature {
		t.Errorf("feature usage wrong: %+v", byVal["feature"])
	}
}

func TestRememberIgnoresEmpty(t *testing.T) {
	s, _ := newTestState(t)
	if err := s.Remember("e1", "v", "global", ""); err != nil {
		t.Fatal(err)
	}
	if got := s.Recall("e1", "v", "global"); len(got) != 0 {
		t.Errorf("empty value should not be remembered, got %+v", got)
	}
}

func TestMemoryPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	s, _ := mustOpen(t, dir)
	if err := s.Remember("e1", "branch", "project", "main"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remember("e1", "branch", "project", "main"); err != nil {
		t.Fatal(err)
	}

	// Reopen: the value and its freq must survive.
	s2, _ := mustOpen(t, dir)
	vals := s2.Recall("e1", "branch", "project")
	if len(vals) != 1 || vals[0].Value != "main" || vals[0].Freq != 2 {
		t.Errorf("after reopen got %+v, want main freq=2", vals)
	}
}

func TestMemoryFileIsReadableTOML(t *testing.T) {
	dir := t.TempDir()
	s, _ := mustOpen(t, dir)
	s.Remember("e1", "branch", "project", "main")

	data, err := os.ReadFile(filepath.Join(dir, "memory.toml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{`id = "e1"`, `var = "branch"`, `value = "main"`, "freq = 1"} {
		if !contains(content, want) {
			t.Errorf("memory.toml missing %q; got:\n%s", want, content)
		}
	}
}

func TestUsageAndPersistence(t *testing.T) {
	dir := t.TempDir()
	s, clk := mustOpen(t, dir)

	if last, count := s.EntryUsage("hot"); last != 0 || count != 0 {
		t.Errorf("unused entry should have zero usage, got last=%d count=%d", last, count)
	}
	for i := 0; i < 5; i++ {
		if err := s.NoteUse("hot"); err != nil {
			t.Fatal(err)
		}
	}
	last, count := s.EntryUsage("hot")
	if count != 5 {
		t.Errorf("use count = %d, want 5", count)
	}
	if last != clk.t.Unix() {
		t.Errorf("last-used = %d, want %d", last, clk.t.Unix())
	}

	// Persistence across reopen.
	s2, _ := mustOpen(t, dir)
	if _, c := s2.EntryUsage("hot"); c != 5 {
		t.Errorf("usage should persist across reopen, count = %d", c)
	}
}

func TestClearMemoryAndUsage(t *testing.T) {
	s, _ := mustOpen(t, t.TempDir())
	if err := s.Remember("entry", "env", "global", "prod"); err != nil {
		t.Fatal(err)
	}
	if err := s.NoteUse("entry"); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearMemory(); err != nil {
		t.Fatal(err)
	}
	if got := s.Recall("entry", "env", "global"); len(got) != 0 {
		t.Fatalf("memory after clear = %+v", got)
	}
	if err := s.ClearUsage(); err != nil {
		t.Fatal(err)
	}
	if last, count := s.EntryUsage("entry"); last != 0 || count != 0 {
		t.Fatalf("usage after clear = (%d,%d)", last, count)
	}
}

func TestBatchDefersAndFlushesBothFiles(t *testing.T) {
	dir := t.TempDir()
	s, _ := mustOpen(t, dir)
	s.BeginBatch()
	if err := s.Remember("entry", "env", "global", "prod"); err != nil {
		t.Fatal(err)
	}
	if err := s.NoteUse("entry"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"memory.toml", "usage.toml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s written before batch ended: %v", name, err)
		}
	}
	if err := s.EndBatch(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"memory.toml", "usage.toml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s not flushed: %v", name, err)
		}
	}
}

func TestConcurrentStatesMergeMutations(t *testing.T) {
	dir := t.TempDir()
	const workers = 12
	states := make([]*State, workers)
	for i := range states {
		var err error
		states[i], err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	for i, s := range states {
		wg.Add(1)
		go func(i int, s *State) {
			defer wg.Done()
			if err := s.Remember("entry", "branch", "global", "shared"); err != nil {
				t.Errorf("remember shared: %v", err)
			}
			if err := s.Remember("entry", "branch", "global", string(rune('a'+i))); err != nil {
				t.Errorf("remember distinct: %v", err)
			}
			if err := s.NoteUse("entry"); err != nil {
				t.Errorf("note use: %v", err)
			}
		}(i, s)
	}
	wg.Wait()

	got, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	values := got.Recall("entry", "branch", "global")
	byValue := make(map[string]RememberedValue, len(values))
	for _, value := range values {
		byValue[value.Value] = value
	}
	if byValue["shared"].Freq != workers {
		t.Errorf("shared frequency = %d, want %d", byValue["shared"].Freq, workers)
	}
	if len(values) != workers+1 {
		t.Errorf("remembered values = %d, want %d", len(values), workers+1)
	}
	if _, count := got.EntryUsage("entry"); count != workers {
		t.Errorf("usage count = %d, want %d", count, workers)
	}
}

func TestStateRetentionCapsGrowth(t *testing.T) {
	s, clk := newTestState(t)
	s.BeginBatch()
	for i := 0; i < maxRememberedValuesPerBucket+5; i++ {
		value := string(rune('a' + i))
		if err := s.Remember("entry", "value", "global", value); err != nil {
			t.Fatal(err)
		}
		clk.advance(time.Second)
	}
	for i := 0; i < maxUsageEntries+5; i++ {
		if err := s.NoteUse("entry-" + string(rune(i))); err != nil {
			t.Fatal(err)
		}
		clk.advance(time.Second)
	}
	if err := s.EndBatch(); err != nil {
		t.Fatal(err)
	}
	if got := len(s.Recall("entry", "value", "global")); got != maxRememberedValuesPerBucket {
		t.Errorf("memory retained %d values, want %d", got, maxRememberedValuesPerBucket)
	}
	s.mu.RLock()
	usageCount := len(s.usage)
	s.mu.RUnlock()
	if usageCount != maxUsageEntries {
		t.Errorf("usage retained %d entries, want %d", usageCount, maxUsageEntries)
	}
}

func mustOpen(t *testing.T, dir string) (*State, *fakeClock) {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	s.SetClock(clk.Now)
	return s, clk
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestOpenQuarantinesCorruptFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"memory.toml", "usage.toml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("[[entry]\nbroken = "), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("corrupt state must not abort startup: %v", err)
	}
	s.SetClock(func() time.Time { return time.Unix(100, 0) })
	if got := s.Recovered(); len(got) != 2 {
		t.Fatalf("Recovered = %v, want 2 notes", got)
	}
	for _, name := range []string{"memory.toml", "usage.toml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s should have been moved aside", name)
		}
		if m, _ := filepath.Glob(filepath.Join(dir, name+".corrupt-*")); len(m) != 1 {
			t.Errorf("%s: want one .corrupt-* copy, got %v", name, m)
		}
	}
	// State starts empty and is writable again.
	if err := s.Remember("e", "v", "global", "x"); err != nil {
		t.Fatal(err)
	}
	if err := s.NoteUse("e"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil || len(reopened.Recall("e", "v", "global")) != 1 {
		t.Fatalf("reopen after recovery: %v %v", err, reopened.Recall("e", "v", "global"))
	}
}

func TestFlushQuarantinesFileCorruptedByAnotherProcess(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "memory.toml"), []byte("not = [toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Remember("e", "v", "global", "x"); err != nil {
		t.Fatalf("flush over a corrupt file should recover, got %v", err)
	}
	if len(s.Recovered()) != 1 {
		t.Errorf("Recovered = %v", s.Recovered())
	}
}

func TestPruneMemoryEvictsByFrecency(t *testing.T) {
	const now = int64(1_700_000_000)
	key := memoryKey{"e", "v", "global"}
	var vals []memoryValue
	// A habitual value: used 50 times, last a few days ago (50×0.5 = 25).
	vals = append(vals, memoryValue{Value: "habit", Freq: 50, LastTS: now - 3*86400})
	// 25 one-off values used more recently than the habit (1×0.5 each).
	for i := 0; i < maxRememberedValuesPerBucket; i++ {
		vals = append(vals, memoryValue{Value: "once" + string(rune('a'+i)), Freq: 1, LastTS: now - 2*86400 + int64(i)})
	}
	memory := map[memoryKey][]memoryValue{key: vals}
	pruneMemoryBucket(memory, key, now)
	kept := memory[key]
	if len(kept) != maxRememberedValuesPerBucket {
		t.Fatalf("kept %d values", len(kept))
	}
	found := false
	for _, v := range kept {
		if v.Value == "habit" {
			found = true
		}
	}
	if !found {
		t.Error("frecent value was evicted by recency-only pruning")
	}

	// The value being remembered right now always survives.
	applyMemoryMutation(memory, memoryMutation{Key: key, Value: "fresh", LastTS: now})
	fresh := false
	for _, v := range memory[key] {
		fresh = fresh || v.Value == "fresh"
	}
	if !fresh {
		t.Error("newly remembered value was pruned immediately")
	}
}
