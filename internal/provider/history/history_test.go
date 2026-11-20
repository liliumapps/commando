package history

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/provider"
)

func cmds(entries []HistEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Cmd
	}
	return out
}

func TestParseExtended(t *testing.T) {
	content := ": 1700841477:0;git status\n: 1700841500:2;npm run build\n"
	got := Parse(content)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	if got[0].Cmd != "git status" || got[0].TS != 1700841477 {
		t.Errorf("entry 0 = %+v", got[0])
	}
	if got[1].Cmd != "npm run build" || got[1].TS != 1700841500 {
		t.Errorf("entry 1 = %+v", got[1])
	}
}

func TestParsePlain(t *testing.T) {
	content := "git status\nls -la\n"
	got := cmds(Parse(content))
	want := []string{"git status", "ls -la"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseMultilineContinuation(t *testing.T) {
	// A trailing backslash continues onto the next line.
	content := ": 1700000000:0;echo one \\\n&& echo two\nls\n"
	got := Parse(content)
	if len(got) != 2 {
		t.Fatalf("want 2 commands, got %d: %+v", len(got), got)
	}
	if got[0].Cmd != "echo one \n&& echo two" {
		t.Errorf("continuation not joined: %q", got[0].Cmd)
	}
	if got[1].Cmd != "ls" {
		t.Errorf("entry 1 = %q", got[1].Cmd)
	}
}

func TestParseEscapedBackslashNotContinuation(t *testing.T) {
	// An even number of trailing backslashes is a literal, not a continuation.
	content := `printf "a\\\\"` + "\nls\n"
	got := Parse(content)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d: %+v", len(got), cmds(got))
	}
}

func TestParseSkipsBlank(t *testing.T) {
	content := "git status\n\n\n: 1700000000:0;\nls\n"
	got := cmds(Parse(content))
	want := []string{"git status", "ls"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseFishHistory(t *testing.T) {
	content := `- cmd: git status
  when: 1700000001
- cmd: printf hello\nworld
  when: 1700000002
  paths:
    - /tmp
`
	got := parseFish(content)
	if len(got) != 2 {
		t.Fatalf("fish entries = %d, want 2: %+v", len(got), got)
	}
	if got[0].Cmd != "git status" || got[0].TS != 1700000001 {
		t.Errorf("first fish entry = %+v", got[0])
	}
	if got[1].Cmd != "printf hello\nworld" || got[1].TS != 1700000002 {
		t.Errorf("second fish entry = %+v", got[1])
	}
}

func TestProviderDedupNewestFirst(t *testing.T) {
	// git status appears twice; it should surface once, and ordering is
	// newest-first by last occurrence.
	content := "git status\nnpm build\ngit status\nls\n"
	p := New(WithPath("/hist"))
	p.readFile = func(string) ([]byte, error) { return []byte(content), nil }

	entries, err := p.Entries(provider.Context{})
	if err != nil {
		t.Fatal(err)
	}
	var descs []string
	for _, e := range entries {
		descs = append(descs, e.Desc)
		if e.Source != entry.SourceHistory {
			t.Errorf("entry %q not SourceHistory", e.Desc)
		}
	}
	// Newest-first by last occurrence: ls (idx3), git status (idx2), npm build (idx1).
	want := []string{"ls", "git status", "npm build"}
	if !reflect.DeepEqual(descs, want) {
		t.Errorf("order = %v, want %v", descs, want)
	}
	if entries[0].ProviderOrder <= entries[1].ProviderOrder ||
		entries[1].ProviderOrder <= entries[2].ProviderOrder {
		t.Errorf("provider order does not preserve recency: %+v", entries)
	}
}

func TestProviderAppliesIgnore(t *testing.T) {
	content := "git status\nexport SECRET_TOKEN=abc\nls\n"
	p := New(WithPath("/hist"), WithIgnore(func(cmd string) bool {
		return contains(cmd, "SECRET")
	}))
	p.readFile = func(string) ([]byte, error) { return []byte(content), nil }

	entries, _ := p.Entries(provider.Context{})
	for _, e := range entries {
		if contains(e.Desc, "SECRET") {
			t.Errorf("ignored command leaked: %q", e.Desc)
		}
	}
	if len(entries) != 2 {
		t.Errorf("want 2 entries after ignore, got %d", len(entries))
	}
}

func TestProviderLimit(t *testing.T) {
	content := "a\nb\nc\nd\ne\n"
	p := New(WithPath("/h"), WithLimit(2))
	p.readFile = func(string) ([]byte, error) { return []byte(content), nil }
	entries, _ := p.Entries(provider.Context{})
	if len(entries) != 2 {
		t.Errorf("limit not applied: got %d, want 2", len(entries))
	}
	// Newest-first: e, d.
	if entries[0].Desc != "e" || entries[1].Desc != "d" {
		t.Errorf("limited entries = %q, %q, want e, d", entries[0].Desc, entries[1].Desc)
	}
}

func TestProviderKeepsLiteralMetachars(t *testing.T) {
	content := "echo a[0] <x>\n"
	p := New(WithPath("/h"))
	p.readFile = func(string) ([]byte, error) { return []byte(content), nil }
	entries, _ := p.Entries(provider.Context{})
	if len(entries) != 1 {
		t.Fatal("want 1 entry")
	}
	if entries[0].Kind != entry.CommandLiteral {
		t.Errorf("kind = %v, want literal", entries[0].Kind)
	}
	if entries[0].Template != `echo a[0] <x>` {
		t.Errorf("template = %q, want literal command", entries[0].Template)
	}
}

func TestResolvePathPrefersHistfile(t *testing.T) {
	p := New()
	p.lookupEnv = func(k string) (string, bool) {
		if k == "HISTFILE" {
			return "/custom/hist", true
		}
		return "", false
	}
	if got := p.resolvePath(); got != "/custom/hist" {
		t.Errorf("resolvePath = %q, want /custom/hist", got)
	}
}

func TestResolvePathUsesFishHistoryForFishShell(t *testing.T) {
	home := t.TempDir()
	dataHome := filepath.Join(home, "xdg-data")
	fishPath := filepath.Join(dataHome, "fish", "fish_history")
	if err := os.MkdirAll(filepath.Dir(fishPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fishPath, []byte("- cmd: git status\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := New()
	p.homeDir = func() (string, error) { return home, nil }
	p.lookupEnv = func(key string) (string, bool) {
		switch key {
		case "SHELL":
			return "/opt/homebrew/bin/fish", true
		case "XDG_DATA_HOME":
			return dataHome, true
		default:
			return "", false
		}
	}
	if got := p.resolvePath(); got != fishPath {
		t.Errorf("fish history path = %q, want %q", got, fishPath)
	}
}

func TestProviderMissingFileIsEmpty(t *testing.T) {
	p := New(WithPath("/does/not/exist"))
	entries, err := p.Entries(provider.Context{})
	if err != nil {
		t.Errorf("missing file should not error, got %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("missing file should yield no entries, got %d", len(entries))
	}
}

func TestReadTailIsBoundedAndLineAligned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	content := "old-command-with-a-long-prefix\nnew-one\nnew-two\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := readTail(path, int64(len("prefix\nnew-one\nnew-two\n")))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "new-one\nnew-two\n"; got != want {
		t.Errorf("tail = %q, want %q", got, want)
	}
}

func TestProviderReadsOnlyConfiguredTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	content := "old-one\nold-two\nnew-one\nnew-two\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	p := New(WithPath(path), WithLimit(0), WithMaxBytes(int64(len("new-one\nnew-two\n"))))
	entries, err := p.Entries(provider.Context{})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(entries))
	for i, e := range entries {
		got[i] = e.Template
	}
	want := []string{"new-two", "new-one"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("entries = %v, want %v", got, want)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
