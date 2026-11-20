package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liliumapps/commando/internal/config"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/provider"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func templateSet(entries []entry.Entry) map[string]bool {
	m := make(map[string]bool, len(entries))
	for _, e := range entries {
		m[e.Template] = true
	}
	return m
}

func entryByTemplate(entries []entry.Entry, tmpl string) (entry.Entry, bool) {
	for _, e := range entries {
		if e.Template == tmpl {
			return e, true
		}
	}
	return entry.Entry{}, false
}

func TestParseMakefile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Makefile"), "build: deps ## Build everything\n\tgo build ./...\n\ntest:\n\tgo test ./...\n\n.PHONY: build test\n\nVAR = value\ndeps:\n\techo deps\n")
	entries := parseMakefile(filepath.Join(dir, "Makefile"))
	got := templateSet(entries)
	for _, want := range []string{"make build", "make test", "make deps"} {
		if !got[want] {
			t.Errorf("missing target %q; got %v", want, got)
		}
	}
	if got["make VAR"] {
		t.Error("variable assignment should not be a target")
	}
	if got["make .PHONY"] {
		t.Error(".PHONY should not be a target")
	}
	build, ok := entryByTemplate(entries, "make build")
	if !ok || !strings.Contains(build.Desc, "Build everything") {
		t.Errorf("make help text should appear in description, got %#v", build)
	}
	if build.Kind != entry.CommandLiteral {
		t.Errorf("make target kind = %v, want literal", build.Kind)
	}
}

func TestParsePackageJSON(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "package.json"), `{"scripts": {"build": "tsc", "test": "jest", "dev": "vite"}}`)
	entries := parsePackageJSON(filepath.Join(dir, "package.json"), dir)
	if len(entries) != 3 {
		t.Fatalf("expected 3 scripts, got %d", len(entries))
	}
	if !strings.HasPrefix(entries[0].Template, "npm run ") {
		t.Errorf("expected npm runner, got %q", entries[0].Template)
	}

	// A pnpm lockfile switches the runner.
	write(t, filepath.Join(dir, "pnpm-lock.yaml"), "lockfileVersion: 6.0")
	entries = parsePackageJSON(filepath.Join(dir, "package.json"), dir)
	if !strings.HasPrefix(entries[0].Template, "pnpm run ") {
		t.Errorf("expected pnpm runner, got %q", entries[0].Template)
	}

	write(t, filepath.Join(dir, "package.json"), `{"packageManager": "yarn@4.1.0", "scripts": {"build": "tsc"}}`)
	entries = parsePackageJSON(filepath.Join(dir, "package.json"), dir)
	if got := entries[0].Template; got != "yarn run build" {
		t.Errorf("expected packageManager runner, got %q", got)
	}
	if entries[0].Kind != entry.CommandLiteral {
		t.Errorf("package script kind = %v, want literal", entries[0].Kind)
	}
}

func TestParsePackageJSONQuotesRepositoryControlledNames(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "package.json"), `{
		"packageManager": "npm; echo unsafe@1.0.0",
		"scripts": {"release prod": "deploy", "test; echo unsafe": "test"}
	}`)
	entries := parsePackageJSON(filepath.Join(dir, "package.json"), dir)
	got := templateSet(entries)
	for _, want := range []string{
		"npm run 'release prod'",
		"npm run 'test; echo unsafe'",
	} {
		if !got[want] {
			t.Errorf("missing safe script command %q; got %v", want, got)
		}
	}
}

func TestParseJustfile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "justfile"), "alias b := build\n\nbuild:\n\tcargo build\n\ntest: build\n\tcargo test\n\nx := \"not a recipe\"\n")
	got := templateSet(parseJustfile(filepath.Join(dir, "justfile")))
	if !got["just build"] || !got["just test"] || !got["just b"] {
		t.Errorf("expected build+test recipes and b alias, got %v", got)
	}
	if got["just x"] {
		t.Error("assignment should not be a recipe")
	}
}

func TestParseCompose(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "docker-compose.yml"), "version: \"3\"\nservices:\n    web:\n      image: nginx\n    db:\n      image: postgres\nvolumes:\n  data:\n")
	got := templateSet(parseCompose(filepath.Join(dir, "docker-compose.yml")))
	if !got["docker compose logs -f web"] || !got["docker compose logs -f db"] {
		t.Errorf("expected per-service logs, got %v", got)
	}
	if !got["docker compose up -d"] {
		t.Error("expected whole-stack up action")
	}
}

func TestProviderWalksUpToGitRoot(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "Makefile"), "build:\n\tgo build\n")
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	p := New(nil)
	entries, err := p.Entries(provider.Context{Cwd: sub, GitRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	// The Makefile lives two levels up, so the command must address it from
	// the working directory.
	if !templateSet(entries)["make -C ../.. build"] {
		t.Errorf("expected to find Makefile target by walking up to git root, got %v", templateSet(entries))
	}
}

func TestUserProviderWithCache(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Taskfile.yml"), "tasks:\n")

	calls := 0
	p := New([]config.ProviderDef{{
		Name:     "task",
		Detect:   "Taskfile.yml",
		List:     "echo ignored",
		Template: "task {}",
		Cache:    "5m",
	}})
	p.runner = func(_, _ string) ([]string, error) {
		calls++
		return []string{"lint", "build"}, nil
	}
	clock := time.Unix(1000, 0)
	p.now = func() time.Time { return clock }

	ctx := provider.Context{Cwd: dir}
	e1, _ := p.Entries(ctx)
	e2, _ := p.Entries(ctx) // should hit cache

	if calls != 1 {
		t.Errorf("lister called %d times, want 1 (cache)", calls)
	}
	if !templateSet(e1)["task lint"] || !templateSet(e2)["task build"] {
		t.Error("expected task entries from user provider")
	}
}

func TestUserProviderCacheExpiry(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Taskfile.yml"), "tasks:\n")
	calls := 0
	p := New([]config.ProviderDef{{Name: "task", Detect: "Taskfile.yml", List: "x", Template: "task {}", Cache: "1m"}})
	p.runner = func(_, _ string) ([]string, error) { calls++; return []string{"a"}, nil }
	clock := time.Unix(1000, 0)
	p.now = func() time.Time { return clock }

	ctx := provider.Context{Cwd: dir}
	p.Entries(ctx)
	clock = clock.Add(2 * time.Minute) // expire
	p.Entries(ctx)
	if calls != 2 {
		t.Errorf("after expiry lister called %d times, want 2", calls)
	}
}

func TestUserProviderQuotesListerItems(t *testing.T) {
	p := New([]config.ProviderDef{{
		Name:     "task",
		Template: "task {}",
	}})
	p.runner = func(_, _ string) ([]string, error) {
		return []string{"lint", "release prod", "$(touch /tmp/unsafe)", "it's"}, nil
	}
	entries, err := p.userEntries(p.userDefs[0], found{dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	got := templateSet(entries)
	for _, want := range []string{
		"task lint",
		"task 'release prod'",
		"task '$(touch /tmp/unsafe)'",
		`task 'it'"'"'s'`,
	} {
		if !got[want] {
			t.Errorf("missing quoted command %q; got %v", want, got)
		}
	}
}

func TestUserProviderRejectsInvalidItems(t *testing.T) {
	p := New([]config.ProviderDef{{Name: "task", Template: "task {}"}})
	p.runner = func(_, _ string) ([]string, error) {
		return []string{"bad\x00item"}, nil
	}
	if _, err := p.userEntries(p.userDefs[0], found{dir: t.TempDir()}); err == nil {
		t.Fatal("expected NUL item to fail")
	}

	p.runner = func(_, _ string) ([]string, error) {
		return []string{strings.Repeat("x", maxProviderItemBytes+1)}, nil
	}
	if _, err := p.userEntries(p.userDefs[0], found{dir: t.TempDir()}); err == nil {
		t.Fatal("expected oversized item to fail")
	}
}

func TestGitEntriesWhenRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := New(nil)
	entries, err := p.Entries(provider.Context{Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !templateSet(entries)["git status"] {
		t.Error("expected git actions inside a repo")
	}
	// All git entries must be SourceProject.
	for _, e := range entries {
		if e.Source != entry.SourceProject {
			t.Errorf("entry %q has non-project source", e.Desc)
		}
	}
}
