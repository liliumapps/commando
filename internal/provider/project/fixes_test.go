package project

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/liliumapps/commando/internal/config"
	"github.com/liliumapps/commando/internal/persist"
	"github.com/liliumapps/commando/internal/provider"
)

func TestParseMakefileMultiTargetAndAssignments(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Makefile"), "build test: deps ## Build and test\n\techo\n"+
		"FOO ::= x\nBAR := y:z\nBAZ ?= a:b\nQUX += c:d\nCMD != echo a:b\nPLAIN = a:b\n"+
		".PHONY: build test\n%.o: %.c\n\tcc\n$(OBJ): x\nclean::\n\trm\n"+
		"define TEMPLATE\nfake: target\nendef\nlint:\r\n")
	got := templateSet(parseMakefile(filepath.Join(dir, "Makefile")))
	for _, want := range []string{"make build", "make test", "make clean", "make lint"} {
		if !got[want] {
			t.Errorf("missing %q; got %v", want, got)
		}
	}
	for _, bad := range []string{"make FOO", "make BAR", "make BAZ", "make QUX", "make CMD", "make PLAIN", "make .PHONY", "make %.o", "make fake", "make deps"} {
		if got[bad] {
			t.Errorf("unexpected %q", bad)
		}
	}
	if e, ok := entryByTemplate(parseMakefile(filepath.Join(dir, "Makefile")), "make test"); !ok || e.Desc != "Build and test" {
		t.Errorf("help text should apply to every target on the line, got %#v", e)
	}
}

func TestDetectsAlternativeMarkers(t *testing.T) {
	for _, tc := range []struct{ file, content, want string }{
		{"GNUmakefile", "build:\n", "make build"},
		{"makefile", "build:\n", "make build"},
		{"compose.yaml", "services:  # the stack\n  web:\n    image: x\n", "docker compose logs -f web"},
		{"compose.yml", "services:\n  db:\n    image: x\n", "docker compose logs -f db"},
		{".justfile", "fmt:\n\tgo fmt\n", "just fmt"},
		{"Justfile", "fmt:\n\tgo fmt\n", "just fmt"},
	} {
		dir := t.TempDir()
		write(t, filepath.Join(dir, tc.file), tc.content)
		entries, err := New(nil).Entries(provider.Context{Cwd: dir})
		if err != nil {
			t.Fatal(err)
		}
		if !templateSet(entries)[tc.want] {
			t.Errorf("%s: missing %q; got %v", tc.file, tc.want, templateSet(entries))
		}
	}
}

func TestMakefileGroupUsesNearestOnly(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "GNUmakefile"), "root:\n")
	sub := filepath.Join(root, "sub")
	write(t, filepath.Join(sub, "Makefile"), "child:\n")
	entries, _ := New(nil).Entries(provider.Context{Cwd: sub, GitRoot: root})
	got := templateSet(entries)
	if !got["make child"] || got["make -C .. root"] {
		t.Errorf("nearest makefile should win, got %v", got)
	}
}

func TestParentMarkerCommandsWorkFromCwd(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "package.json"), `{"scripts":{"build":"tsc"}}`)
	write(t, filepath.Join(root, "pnpm-lock.yaml"), "")
	write(t, filepath.Join(root, "Cargo.toml"), "[package]\n")
	write(t, filepath.Join(root, "build.gradle"), "")
	write(t, filepath.Join(root, "gradlew"), "")
	write(t, filepath.Join(root, "justfile"), "fmt:\n\tx\n")
	write(t, filepath.Join(root, "compose.yaml"), "services:\n  web:\n    image: x\n")
	sub := filepath.Join(root, "src", "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := New(nil).Entries(provider.Context{Cwd: sub, GitRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	got := templateSet(entries)
	for _, want := range []string{
		"pnpm --dir ../.. run build",
		"cargo build --manifest-path ../../Cargo.toml",
		"../../gradlew -p ../.. build",
		"just --justfile ../../justfile fmt",
		"docker compose -f ../../compose.yaml up -d",
	} {
		if !got[want] {
			t.Errorf("missing %q; got %v", want, got)
		}
	}
}

func TestNodeRunnerFromMonorepoRoot(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "package.json"), `{"packageManager":"yarn@4.0.0","workspaces":["packages/*"]}`)
	pkg := filepath.Join(root, "packages", "app")
	write(t, filepath.Join(pkg, "package.json"), `{"scripts":{"dev":"vite"}}`)
	entries, _ := New(nil).Entries(provider.Context{Cwd: pkg, GitRoot: root})
	if !templateSet(entries)["yarn run dev"] {
		t.Errorf("workspace package should use the root's yarn, got %v", templateSet(entries))
	}

	root2 := t.TempDir()
	write(t, filepath.Join(root2, "pnpm-lock.yaml"), "")
	pkg2 := filepath.Join(root2, "apps", "web")
	write(t, filepath.Join(pkg2, "package.json"), `{"scripts":{"dev":"vite"}}`)
	entries, _ = New(nil).Entries(provider.Context{Cwd: pkg2, GitRoot: root2})
	if !templateSet(entries)["pnpm run dev"] {
		t.Errorf("workspace package should use the root lockfile, got %v", templateSet(entries))
	}
}

func TestFindMarkersNormalizesSymlinkedPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	base := t.TempDir()
	realDir := filepath.Join(base, "realDir")
	write(t, filepath.Join(realDir, "Makefile"), "build:\n")
	write(t, filepath.Join(base, "Makefile"), "outside:\n")
	link := filepath.Join(base, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(link, "a")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// cwd goes through the symlink while git reports the realDir path.
	realRoot, _ := filepath.EvalSymlinks(realDir)
	fs := findMarkers(sub, realRoot+string(filepath.Separator), map[string]bool{"Makefile": true, "nothere": true})
	if len(fs) != 1 {
		t.Fatalf("found %d markers, want 1: %+v", len(fs), fs)
	}
	if fs[0].dir != realRoot {
		t.Errorf("found Makefile in %q, want %q (must not climb above the git root)", fs[0].dir, realRoot)
	}
}

func TestFindMarkersDoesNotEscapeUnrelatedRoot(t *testing.T) {
	base := t.TempDir()
	write(t, filepath.Join(base, "Makefile"), "x:\n")
	cwd := filepath.Join(base, "a")
	other := filepath.Join(base, "b")
	for _, d := range []string{cwd, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if fs := findMarkers(cwd, other, map[string]bool{"Makefile": true}); len(fs) != 0 {
		t.Errorf("walked above an unrelated boundary: %+v", fs)
	}
}

func TestGitEntriesWithGitFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".git"), "gitdir: ../.git/worktrees/x\n")
	entries, _ := New(nil).Entries(provider.Context{Cwd: dir})
	if !templateSet(entries)["git status"] {
		t.Error("a .git file (worktree) should enable git actions")
	}
}

func TestUserProviderDiskCacheAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Taskfile.yml"), "tasks:\n")
	cacheDir := t.TempDir()
	clock := time.Unix(1000, 0)
	defs := []config.ProviderDef{{Name: "task", Detect: "Taskfile.yml", List: "task -l", Template: "task {}", Cache: "5m"}}

	calls := 0
	mk := func() *Provider {
		p := New(defs)
		p.now = func() time.Time { return clock }
		c := persist.NewDiskCache(cacheDir)
		c.SetClock(func() time.Time { return clock })
		p.SetDiskCache(c)
		p.runner = func(_, _ string) ([]string, error) { calls++; return []string{"lint"}, nil }
		return p
	}
	ctx := provider.Context{Cwd: dir}
	if _, err := mk().Entries(ctx); err != nil {
		t.Fatal(err)
	}
	e, _ := mk().Entries(ctx) // a fresh launch
	if calls != 1 || !templateSet(e)["task lint"] {
		t.Fatalf("second launch should hit the disk cache: calls=%d entries=%v", calls, templateSet(e))
	}
	clock = clock.Add(6 * time.Minute)
	mk().Entries(ctx)
	if calls != 2 {
		t.Errorf("expired disk cache should re-run the lister, calls=%d", calls)
	}

	// A corrupt record is a miss, not an error.
	matches, _ := filepath.Glob(filepath.Join(cacheDir, "*.json"))
	for _, m := range matches {
		os.WriteFile(m, []byte("{garbage"), 0o600)
	}
	clock = clock.Add(time.Second)
	if _, err := mk().Entries(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Errorf("corrupt cache should be a miss, calls=%d", calls)
	}
}
