package app

import (
	"reflect"
	"testing"

	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/rank"
)

// values collects the inserted Value of each candidate.
func values(cands []rank.Candidate) []string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.Value
	}
	return out
}

// displays collects the Display text of each candidate.
func displays(cands []rank.Candidate) []string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.Display
	}
	return out
}

func TestExtractCandidatesPlain(t *testing.T) {
	raw := "a\nb\n\nc\n" // blank line and trailing newline
	got := extractCandidates(raw, entry.VarSpec{})
	// A blank line renders an empty value and is dropped; Display == Value.
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(values(got), want) {
		t.Errorf("values = %v, want %v", values(got), want)
	}
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(displays(got), want) {
		t.Errorf("displays = %v, want %v", displays(got), want)
	}
}

func TestExtractCandidatesHeaderLines(t *testing.T) {
	raw := "REPOSITORY TAG IMAGE_ID\nubuntu latest abc123\nalpine 3.18 def456\n"
	got := extractCandidates(raw, entry.VarSpec{HeaderLines: 1})
	want := []string{"ubuntu latest abc123", "alpine 3.18 def456"}
	if !reflect.DeepEqual(values(got), want) {
		t.Errorf("values = %v, want %v", values(got), want)
	}
}

// TestExtractCandidatesColumnsValue replaces the old `column` extraction: name
// the columns and insert only one of them, while Display stays the full line.
func TestExtractCandidatesColumnsValue(t *testing.T) {
	raw := "REPOSITORY TAG IMAGE_ID\nubuntu latest abc123\nalpine 3.18 def456\n"
	spec := entry.VarSpec{
		HeaderLines: 1,
		Columns:     []string{"repo", "tag", "id"},
		Value:       "<id>",
	}
	got := extractCandidates(raw, spec)
	if want := []string{"abc123", "def456"}; !reflect.DeepEqual(values(got), want) {
		t.Errorf("values = %v, want %v", values(got), want)
	}
	// Display defaults to the raw line so the list stays readable/searchable.
	if want := []string{"ubuntu latest abc123", "alpine 3.18 def456"}; !reflect.DeepEqual(displays(got), want) {
		t.Errorf("displays = %v, want %v", displays(got), want)
	}
}

// TestExtractCandidatesColumnsPositional checks positional <1>,<2>,... refs work
// without naming columns.
func TestExtractCandidatesColumnsPositional(t *testing.T) {
	raw := "alice:1000:/home/alice\nbob:1001:/home/bob\n"
	spec := entry.VarSpec{Delimiter: ":", Columns: []string{"user"}, Value: "<1>"}
	got := extractCandidates(raw, spec)
	if want := []string{"alice", "bob"}; !reflect.DeepEqual(values(got), want) {
		t.Errorf("values = %v, want %v", values(got), want)
	}
}

// TestExtractCandidatesComposedLabel is the "chrome (1234)" case: a composed
// display drawn from two columns while inserting one.
func TestExtractCandidatesComposedLabel(t *testing.T) {
	raw := "  1234 chrome\n  5678 postgres\n"
	spec := entry.VarSpec{
		Columns: []string{"pid", "comm"},
		Label:   "<comm> (<pid>)",
		Value:   "<pid>",
	}
	got := extractCandidates(raw, spec)
	if want := []string{"1234", "5678"}; !reflect.DeepEqual(values(got), want) {
		t.Errorf("values = %v, want %v", values(got), want)
	}
	if want := []string{"chrome (1234)", "postgres (5678)"}; !reflect.DeepEqual(displays(got), want) {
		t.Errorf("displays = %v, want %v", displays(got), want)
	}
}

// TestExtractCandidatesMatchNamedGroups covers the regex power path with named
// capture groups.
func TestExtractCandidatesMatchNamedGroups(t *testing.T) {
	raw := "  1234 chrome /Applications/Chrome\n  5678 postgres /usr/bin/postgres\n"
	spec := entry.VarSpec{
		Match: `^\s*(?P<pid>\d+)\s+(?P<comm>\S+)\s+(?P<args>.*)$`,
		Label: "<comm> (<pid>)",
		Value: "<pid>",
	}
	got := extractCandidates(raw, spec)
	if want := []string{"1234", "5678"}; !reflect.DeepEqual(values(got), want) {
		t.Errorf("values = %v, want %v", values(got), want)
	}
	if want := []string{"chrome (1234)", "postgres (5678)"}; !reflect.DeepEqual(displays(got), want) {
		t.Errorf("displays = %v, want %v", displays(got), want)
	}
}

// TestExtractCandidatesMatchDropsNonMatching checks that a line failing the
// Match regex is dropped rather than kept as a blank.
func TestExtractCandidatesMatchDropsNonMatching(t *testing.T) {
	raw := "1234 chrome\ngarbage line without digits at start\n7 k3\n"
	spec := entry.VarSpec{Match: `^(?P<pid>\d+)\s+(?P<comm>\S+)$`, Value: "<pid>"}
	got := extractCandidates(raw, spec)
	if want := []string{"1234", "7"}; !reflect.DeepEqual(values(got), want) {
		t.Errorf("values = %v, want %v", values(got), want)
	}
}

func TestExtractCandidatesMultiSpaceDelimiter(t *testing.T) {
	// docker-style output with variable spacing; delimiter of 2+ spaces keeps
	// multiword column values intact.
	raw := "NAME          STATUS\nweb server    running\ndb            exited\n"
	spec := entry.VarSpec{HeaderLines: 1, Delimiter: `\s\s+`, Columns: []string{"name"}, Value: "<name>"}
	got := extractCandidates(raw, spec)
	if want := []string{"web server", "db"}; !reflect.DeepEqual(values(got), want) {
		t.Errorf("values = %v, want %v", values(got), want)
	}
}

func TestExtractCandidatesHeaderLinesExceedsInput(t *testing.T) {
	if got := extractCandidates("only\n", entry.VarSpec{HeaderLines: 5}); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// TestExtractCandidatesEmptyValueDropped checks a line whose value template
// renders empty is dropped.
func TestExtractCandidatesEmptyValueDropped(t *testing.T) {
	raw := "a b\nc\nd e\n"
	// column 2 is absent on the "c" line, so <2> is empty and that line drops.
	spec := entry.VarSpec{Columns: []string{"x", "y"}, Value: "<2>"}
	got := extractCandidates(raw, spec)
	if want := []string{"b", "e"}; !reflect.DeepEqual(values(got), want) {
		t.Errorf("values = %v, want %v", values(got), want)
	}
}

func TestExtractCandidatesStripsANSI(t *testing.T) {
	raw := "\x1b[1;32mmain\x1b[0m\r\n\x1b[31mfeature/x\x1b[m\n\x1b]8;;https://x\x07link\x1b]8;;\x07\n"
	got := extractCandidates(raw, entry.VarSpec{Name: "b"})
	want := []string{"main", "feature/x", "link"}
	if len(got) != len(want) {
		t.Fatalf("got %d candidates %#v, want %v", len(got), got, want)
	}
	for i, c := range got {
		if c.Display != want[i] || c.Value != want[i] {
			t.Errorf("candidate %d = %q/%q, want %q", i, c.Display, c.Value, want[i])
		}
	}
}
