package history

import (
	"reflect"
	"testing"

	"github.com/liliumapps/commando/internal/provider"
)

func TestParseBashTimestamps(t *testing.T) {
	content := "#1700000000\nls -la\n#1700000100\nfor f in *; do\n  echo $f\ndone\n#1700000200\r\ngit status\r\n"
	got := Parse(content)
	want := []HistEntry{
		{Cmd: "ls -la", TS: 1700000000},
		{Cmd: "for f in *; do\n  echo $f\ndone", TS: 1700000100},
		{Cmd: "git status", TS: 1700000200},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse = %#v\nwant %#v", got, want)
	}
}

func TestParseCRLFContinuation(t *testing.T) {
	got := Parse("echo a \\\r\nb\r\nls\r\n")
	want := []HistEntry{{Cmd: "echo a \nb"}, {Cmd: "ls"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse = %#v, want %#v", got, want)
	}
}

func TestUnmetafyZsh(t *testing.T) {
	// zsh metafies special bytes: "ă" (C4 83) is stored as C4 83 A3, i.e. the
	// 0x83 byte becomes the marker 0x83 followed by 0x83^0x20.
	meta := []byte(": 1700000000:0;echo \xc4\x83\xa3\n")
	p := New(WithPath("/home/u/.zsh_history"))
	p.readFile = func(string) ([]byte, error) { return meta, nil }
	entries, _ := p.Entries(provider.Context{})
	if len(entries) != 1 || entries[0].Template != "echo ă" {
		t.Fatalf("entries = %#v", entries)
	}

	// Bash history is never unmetafied: 0x83 is a normal UTF-8 byte there.
	p = New(WithPath("/home/u/.bash_history"))
	p.readFile = func(string) ([]byte, error) { return []byte("echo \xc4\x83\n"), nil }
	entries, _ = p.Entries(provider.Context{})
	if len(entries) != 1 || entries[0].Template != "echo ă" {
		t.Fatalf("bash entries = %#v", entries)
	}
}

func TestProviderOrdersByTimestamp(t *testing.T) {
	// Two bash sessions appended out of order (histappend writes at exit).
	content := "#300\nnewest\n#100\noldest\n#200\nmiddle\n"
	p := New(WithPath("/hist"))
	p.readFile = func(string) ([]byte, error) { return []byte(content), nil }
	entries, _ := p.Entries(provider.Context{})
	var got []string
	for _, e := range entries {
		got = append(got, e.Template)
	}
	want := []string{"newest", "middle", "oldest"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if entries[0].ProviderOrder <= entries[1].ProviderOrder {
		t.Errorf("ProviderOrder should follow timestamps: %+v", entries)
	}
}
