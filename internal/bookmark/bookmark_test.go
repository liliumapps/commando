package bookmark

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/provider"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	return Open(filepath.Join(t.TempDir(), "bookmarks.toml"))
}

func TestAddListRemove(t *testing.T) {
	s := newStore(t)
	if err := s.Add(Bookmark{Cmd: "kubectl get pods -A", Tags: []string{"k8s"}, Note: "all pods", Created: 100}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(Bookmark{Cmd: "git push --force", Created: 200}); err != nil {
		t.Fatal(err)
	}

	bs, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 2 {
		t.Fatalf("want 2 bookmarks, got %d", len(bs))
	}
	// Newest-first by Created.
	if bs[0].Cmd != "git push --force" {
		t.Errorf("newest = %q, want git push --force", bs[0].Cmd)
	}

	removed, err := s.Remove("git push --force")
	if err != nil || !removed {
		t.Fatalf("remove failed: removed=%v err=%v", removed, err)
	}
	bs, _ = s.List()
	if len(bs) != 1 || bs[0].Cmd != "kubectl get pods -A" {
		t.Errorf("after remove: %+v", bs)
	}
}

func TestAddRequiresCmd(t *testing.T) {
	s := newStore(t)
	if err := s.Add(Bookmark{Cmd: "  "}); err == nil {
		t.Error("expected error for empty command")
	}
}

func TestAddDedupUpdatesTagsNote(t *testing.T) {
	s := newStore(t)
	s.Add(Bookmark{Cmd: "make deploy", Created: 1})
	s.Add(Bookmark{Cmd: "make deploy", Tags: []string{"ci"}, Note: "prod", Created: 2})

	bs, _ := s.List()
	if len(bs) != 1 {
		t.Fatalf("dedup failed, got %d bookmarks", len(bs))
	}
	if len(bs[0].Tags) != 1 || bs[0].Tags[0] != "ci" || bs[0].Note != "prod" {
		t.Errorf("tags/note not updated on re-add: %+v", bs[0])
	}
}

func TestRemoveMissing(t *testing.T) {
	s := newStore(t)
	s.Add(Bookmark{Cmd: "a", Created: 1})
	removed, err := s.Remove("nonexistent")
	if err != nil || removed {
		t.Errorf("removing missing bookmark: removed=%v err=%v", removed, err)
	}
}

func TestEntriesSearchableAndLiteral(t *testing.T) {
	s := newStore(t)
	s.Add(Bookmark{Cmd: "jq '.[0].id' <file>", Tags: []string{"json"}, Note: "first id", Created: 1})

	entries, err := s.Entries(provider.Context{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Source != entry.SourceBookmark {
		t.Errorf("source = %v, want bookmark", e.Source)
	}
	if e.Kind != entry.CommandLiteral {
		t.Errorf("kind = %v, want literal", e.Kind)
	}
	// Note + tags are part of the searchable text.
	if !strings.Contains(e.SearchText(), "first id") || !strings.Contains(e.SearchText(), "json") {
		t.Errorf("search text missing note/tags: %q", e.SearchText())
	}
	// Literal commands stay byte-for-byte runnable; shell metacharacters are not
	// escaped into commando template syntax.
	if e.Template != "jq '.[0].id' <file>" {
		t.Errorf("template = %q, want literal command", e.Template)
	}
}

func TestFileIsReadableTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bookmarks.toml")
	s := Open(path)
	s.Add(Bookmark{Cmd: "echo hi", Tags: []string{"demo"}, Note: "greeting", Created: 42})

	data, _ := os.ReadFile(path)
	content := string(data)
	for _, want := range []string{`cmd = "echo hi"`, "demo", `note = "greeting"`} {
		if !strings.Contains(content, want) {
			t.Errorf("bookmarks.toml missing %q; got:\n%s", want, content)
		}
	}
}

func TestPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bookmarks.toml")
	Open(path).Add(Bookmark{Cmd: "persisted", Created: 1})

	bs, err := Open(path).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 1 || bs[0].Cmd != "persisted" {
		t.Errorf("did not persist: %+v", bs)
	}
}

func TestConcurrentStoresDoNotLoseBookmarks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bookmarks.toml")
	const workers = 20
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := "command-" + string(rune('a'+i))
			if err := Open(path).Add(Bookmark{Cmd: cmd, Created: int64(i)}); err != nil {
				t.Errorf("add %q: %v", cmd, err)
			}
		}(i)
	}
	wg.Wait()
	bookmarks, err := Open(path).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(bookmarks) != workers {
		t.Errorf("bookmarks = %d, want %d", len(bookmarks), workers)
	}
}

func TestCorruptBookmarksAreMovedAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bookmarks.toml")
	if err := os.WriteFile(path, []byte("[[bookmark]\ncmd = "), 0o644); err != nil {
		t.Fatal(err)
	}
	s := Open(path)
	entries, err := s.Entries(provider.Context{})
	if err == nil || !strings.Contains(err.Error(), "corrupt") || len(entries) != 0 {
		t.Fatalf("want a corrupt notice and no entries, got %v, %v", entries, err)
	}
	if m, _ := filepath.Glob(path + ".corrupt-*"); len(m) != 1 {
		t.Fatalf("corrupt copy not kept: %v", m)
	}
	if err := s.Add(Bookmark{Cmd: "ls"}); err != nil {
		t.Fatalf("Add after recovery: %v", err)
	}
	entries, err = s.Entries(provider.Context{})
	if err != nil || len(entries) != 1 {
		t.Fatalf("after recovery: %v, %v", entries, err)
	}
}
