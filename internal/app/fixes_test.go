package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liliumapps/commando/internal/bookmark"
	"github.com/liliumapps/commando/internal/config"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/persist"
	"github.com/liliumapps/commando/internal/provider"
	"github.com/liliumapps/commando/internal/resolve"
	"github.com/liliumapps/commando/internal/state"
)

func redactor() func(string) bool {
	patterns := config.Default().HistoryIgnore
	return func(s string) bool { return config.ShouldIgnore(patterns, s) }
}

func TestRememberSkipsSecretValues(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b := &fileBackend{state: st, redact: redactor()}
	e := entry.Entry{Template: "x <v>", Source: entry.SourceCheat}
	field := func(name string) resolve.Field { return resolve.Field{Name: name} }

	b.Remember(e, field("token"), "abc123")                              // name=value matches *token*
	b.Remember(e, field("key"), "ghp"+"_abcdefghijklmnopqrstuvwxyz0123") // token-shaped value
	b.Remember(e, field("branch"), "main")
	if got := st.Recall(e.ID(), "token", "global"); len(got) != 0 {
		t.Errorf("secret var remembered: %v", got)
	}
	if got := st.Recall(e.ID(), "key", "global"); len(got) != 0 {
		t.Errorf("token-shaped value remembered: %v", got)
	}
	if got := st.Recall(e.ID(), "branch", "global"); len(got) != 1 {
		t.Errorf("ordinary value not remembered: %v", got)
	}
}

func TestBookmarkRefusesSecretCommand(t *testing.T) {
	store := bookmark.Open(filepath.Join(t.TempDir(), "bookmarks.toml"))
	b := &fileBackend{bookmarks: store, redact: redactor()}
	if err := b.Bookmark("curl -H 'Authorization: Bearer abcdefghijklmnop' https://api"); !errors.Is(err, ErrSecretBookmark) {
		t.Fatalf("Bookmark err = %v, want ErrSecretBookmark", err)
	}
	if err := b.Bookmark("make build"); err != nil {
		t.Fatal(err)
	}
	bs, _ := store.List()
	if len(bs) != 1 || bs[0].Cmd != "make build" {
		t.Errorf("bookmarks = %+v", bs)
	}
}

func TestSrcCachePersistsAcrossBackends(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "count")
	src := "printf 'x\\n' >> " + counter + "; cat " + counter
	cache := persist.NewDiskCache(filepath.Join(dir, "cache"))
	pctx := provider.Context{Cwd: dir}

	for i := 0; i < 2; i++ { // two "launches"
		b := &fileBackend{pctx: pctx, disk: cache}
		if _, ok := b.runSrcRaw(src, "5m"); !ok {
			t.Fatal("src failed")
		}
	}
	data, _ := os.ReadFile(counter)
	if n := strings.Count(string(data), "x"); n != 1 {
		t.Errorf("src ran %d times across launches, want 1 (disk cache)", n)
	}

	// A different cwd is a different key.
	other := t.TempDir()
	b := &fileBackend{pctx: provider.Context{Cwd: other}, disk: cache}
	b.runSrcRaw(src, "5m")
	data, _ = os.ReadFile(counter)
	if n := strings.Count(string(data), "x"); n != 2 {
		t.Errorf("cwd should be part of the cache key, runs = %d", n)
	}
}

// slowProvider records how many providers are running at once.
type slowProvider struct {
	name    string
	running *int32
	peak    *int32
}

func (p slowProvider) Name() string { return p.name }
func (p slowProvider) Entries(provider.Context) ([]entry.Entry, error) {
	n := atomic.AddInt32(p.running, 1)
	for {
		old := atomic.LoadInt32(p.peak)
		if n <= old || atomic.CompareAndSwapInt32(p.peak, old, n) {
			break
		}
	}
	time.Sleep(30 * time.Millisecond)
	atomic.AddInt32(p.running, -1)
	return []entry.Entry{{Title: p.name, Template: p.name, Source: entry.SourceCheat}}, nil
}

func TestAggregateRunsProvidersConcurrentlyInOrder(t *testing.T) {
	var running, peak int32
	var provs []provider.Provider
	var want []string
	for _, n := range []string{"a", "b", "c", "d"} {
		provs = append(provs, slowProvider{name: n, running: &running, peak: &peak})
		want = append(want, n)
	}
	got, diags := aggregate(provs, provider.Context{}, nil)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	if !reflect.DeepEqual(templatesOf(got), want) {
		t.Errorf("order = %v, want %v", templatesOf(got), want)
	}
	if atomic.LoadInt32(&peak) < 2 {
		t.Errorf("providers ran sequentially (peak concurrency %d)", peak)
	}
}

func TestAggregateReportsRecoveredState(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "usage.toml"), []byte("garbage = ["), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, diags := aggregate(nil, provider.Context{}, st)
	if len(diags) != 1 || diags[0].Source != "state" || !strings.Contains(diags[0].Message, "corrupt") {
		t.Errorf("diagnostics = %+v", diags)
	}
}

func TestGitRootAcceptsGitFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /nonexistent/worktree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := gitRoot(sub); got != dir {
		t.Errorf("gitRoot = %q, want %q (.git file marks a worktree root)", got, dir)
	}
}

func TestStableIDKeepsMemoryAcrossEdits(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b := &fileBackend{state: st}
	before := entry.Entry{Title: "Deploy", Template: "deploy <env>", Source: entry.SourceCheat, StableID: "deploy"}
	after := entry.Entry{Title: "Deploy service", Template: "deploy --wait <env>", Source: entry.SourceCheat, StableID: "deploy"}
	b.Remember(before, resolve.Field{Name: "env"}, "prod")
	if got := st.Recall(after.ID(), "env", "global"); len(got) != 1 || got[0].Value != "prod" {
		t.Errorf("memory orphaned by edit: %v", got)
	}
}

func TestDoctorReportsQuarantinedState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("COMMANDO_HOME", home)
	dataDir := filepath.Join(home, "commando")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "memory.toml"), []byte("[[entry]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var recovered *Check
	checks := Doctor()
	for i := range checks {
		if checks[i].Name == "state" && checks[i].Status == CheckError {
			t.Fatalf("corrupt state should not be an error: %+v", checks[i])
		}
		if checks[i].Name == "recovered" {
			recovered = &checks[i]
		}
	}
	if recovered == nil || recovered.Status != CheckWarning || !strings.Contains(recovered.Detail, "memory.toml.corrupt-") {
		t.Errorf("recovered check = %+v", recovered)
	}
}
