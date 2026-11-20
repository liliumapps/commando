package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

func TestRememberFillDedupesIdenticalSets(t *testing.T) {
	s, clk := newTestState(t)
	a := map[string]string{"ns": "payments", "pod": "api-7d9f"}
	for i := 0; i < 3; i++ {
		if err := s.RememberFill("e1", "global", a); err != nil {
			t.Fatal(err)
		}
		clk.advance(time.Minute)
	}
	if err := s.RememberFill("e1", "global", map[string]string{"ns": "payments", "pod": "web-1"}); err != nil {
		t.Fatal(err)
	}
	got := s.RecallFills("e1", "global")
	if len(got) != 2 {
		t.Fatalf("want 2 fills, got %+v", got)
	}
	// Three uses beat one, both within the hour.
	if got[0].Values["pod"] != "api-7d9f" || got[0].Count != 3 {
		t.Errorf("hottest fill = %+v", got[0])
	}
	if got[0].LastN != clk.t.Add(-time.Minute).Unix() {
		t.Errorf("last = %d", got[0].LastN)
	}
}

func TestRememberFillIgnoresEmptyAndScopesSeparately(t *testing.T) {
	s, _ := newTestState(t)
	if err := s.RememberFill("e1", "global", map[string]string{"a": ""}); err != nil {
		t.Fatal(err)
	}
	if got := s.RecallFills("e1", "global"); len(got) != 0 {
		t.Fatalf("empty set stored: %+v", got)
	}
	_ = s.RememberFill("e1", "project:/r", map[string]string{"a": "1", "b": ""})
	if got := s.RecallFills("e1", "global"); len(got) != 0 {
		t.Fatalf("project fill leaked into global: %+v", got)
	}
	got := s.RecallFills("e1", "project:/r")
	if len(got) != 1 || len(got[0].Values) != 1 || got[0].Values["a"] != "1" {
		t.Fatalf("project fill = %+v", got)
	}
}

func TestFillsEvictLowestFrecency(t *testing.T) {
	s, clk := newTestState(t)
	hot := map[string]string{"v": "hot"}
	for i := 0; i < 5; i++ {
		_ = s.RememberFill("e", "global", hot)
	}
	clk.advance(2 * time.Hour)
	for i := 0; i < maxFillsPerBucket+5; i++ {
		clk.advance(time.Second)
		if err := s.RememberFill("e", "global", map[string]string{"v": "one-" + string(rune('a'+i))}); err != nil {
			t.Fatal(err)
		}
	}
	got := s.RecallFills("e", "global")
	if len(got) != maxFillsPerBucket {
		t.Fatalf("kept %d fills, want %d", len(got), maxFillsPerBucket)
	}
	found := false
	for _, f := range got {
		if f.Values["v"] == "hot" {
			found = true
		}
	}
	if !found {
		t.Error("frequently used fill was evicted by one-offs")
	}
	// The newest one-off always survives.
	last := "one-" + string(rune('a'+maxFillsPerBucket+4))
	found = false
	for _, f := range got {
		if f.Values["v"] == last {
			found = true
		}
	}
	if !found {
		t.Error("just-recorded fill was evicted")
	}
}

func TestForgetFill(t *testing.T) {
	s, _ := newTestState(t)
	_ = s.RememberFill("e", "global", map[string]string{"a": "1"})
	_ = s.RememberFill("e", "global", map[string]string{"a": "2"})
	if err := s.ForgetFill("e", "global", map[string]string{"a": "1"}); err != nil {
		t.Fatal(err)
	}
	got := s.RecallFills("e", "global")
	if len(got) != 1 || got[0].Values["a"] != "2" {
		t.Fatalf("after forget: %+v", got)
	}
	reopened, err := Open(filepath.Dir(s.fillsPath))
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.RecallFills("e", "global"); len(got) != 1 || got[0].Values["a"] != "2" {
		t.Fatalf("forget not persisted: %+v", got)
	}
}

func TestFillsFileIsReadableTOML(t *testing.T) {
	s, _ := newTestState(t)
	vals := map[string]string{"namespace": "pay\"ments", "pod.name": "api 7"}
	if err := s.RememberFill("id1", "global", vals); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.fillsPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"[[fill]]", `id = "id1"`, `scope = "global"`, `values = { namespace = "pay\"ments", "pod.name" = "api 7" }`, "count = 1"} {
		if !strings.Contains(text, want) {
			t.Errorf("fills.toml missing %q:\n%s", want, text)
		}
	}
	var ff fillsFile
	if err := toml.Unmarshal(data, &ff); err != nil {
		t.Fatalf("fills.toml does not parse: %v", err)
	}
	reopened, err := Open(filepath.Dir(s.fillsPath))
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.RecallFills("id1", "global")
	if len(got) != 1 || FillSignature(got[0].Values) != FillSignature(vals) {
		t.Fatalf("round trip = %+v", got)
	}
}

func TestClearMemoryClearsFills(t *testing.T) {
	s, _ := newTestState(t)
	_ = s.RememberFill("e", "global", map[string]string{"a": "1"})
	_ = s.Remember("e", "a", "global", "1")
	if err := s.ClearMemory(); err != nil {
		t.Fatal(err)
	}
	if got := s.RecallFills("e", "global"); len(got) != 0 {
		t.Fatalf("fills survived ClearMemory: %+v", got)
	}
	reopened, err := Open(filepath.Dir(s.fillsPath))
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.RecallFills("e", "global"); len(got) != 0 {
		t.Fatalf("fills survived ClearMemory on disk: %+v", got)
	}
}

func TestOpenQuarantinesCorruptFills(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fills.toml"), []byte("[[fill]\nbroken = "), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("corrupt fills must not abort startup: %v", err)
	}
	if got := s.Recovered(); len(got) != 1 {
		t.Fatalf("Recovered = %v", got)
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "fills.toml.corrupt-*")); len(m) != 1 {
		t.Errorf("want one .corrupt-* copy, got %v", m)
	}
	if err := s.RememberFill("e", "global", map[string]string{"a": "1"}); err != nil {
		t.Fatal(err)
	}
}

func TestBatchDefersFills(t *testing.T) {
	s, _ := newTestState(t)
	s.BeginBatch()
	_ = s.RememberFill("e", "global", map[string]string{"a": "1"})
	if _, err := os.Stat(s.fillsPath); !os.IsNotExist(err) {
		t.Fatal("fills.toml written inside a batch")
	}
	if len(s.RecallFills("e", "global")) != 1 {
		t.Fatal("batched fill not visible")
	}
	if err := s.EndBatch(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.fillsPath); err != nil {
		t.Fatalf("fills.toml not flushed: %v", err)
	}
}
