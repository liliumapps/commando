package rank

import (
	"reflect"
	"testing"

	"github.com/liliumapps/commando/internal/entry"
)

func TestMatchTier(t *testing.T) {
	cases := []struct {
		q, h string
		want Tier
	}{
		{"", "anything", TierStartsWith},
		{"git", "git status", TierStartsWith},
		{"status", "git status", TierContains},
		{"gts", "git status", TierFuzzy}, // g..t..s subsequence
		{"xyz", "git status", TierNone},
		{"GIT", "git status", TierStartsWith}, // case-insensitive
	}
	for _, c := range cases {
		if got := matchTier(c.q, c.h); got != c.want {
			t.Errorf("matchTier(%q,%q) = %v, want %v", c.q, c.h, got, c.want)
		}
	}
}

func TestMatchQueryMultiTerm(t *testing.T) {
	// haystacks model the (desc, template, tags) columns of one entry.
	desc, tmpl, tags := "Deploy all services related to one cluster",
		"kubectl --context <c> apply -k overlays/all", "infra deploy all"
	cases := []struct {
		q    string
		want Tier
	}{
		{"", TierStartsWith},
		{"deploy", TierStartsWith},            // desc starts with "Deploy"
		{"related", TierContains},             // mid-desc word
		{"deploy related", TierContains},      // both terms, worst tier wins
		{"related deploy", TierContains},      // order-independent
		{"infra related", TierContains},       // term in tags + term in desc
		{"infra deploy", TierStartsWith},      // each term starts a column
		{"related nonexistentword", TierNone}, // one term matches nothing
		{"DEPLOY RELATED", TierContains},      // case-insensitive
	}
	for _, c := range cases {
		if got := matchQuery(c.q, desc, tmpl, tags); got != c.want {
			t.Errorf("matchQuery(%q, ...) = %v, want %v", c.q, got, c.want)
		}
	}
}

func TestEntriesSearchMultiTermCrossColumn(t *testing.T) {
	// A query mixing a tag word and a description word, in either order, should
	// find the entry — the whole string need not appear in one column. This is
	// the regression guard for tag+desc combined search.
	entries := []entry.Entry{
		{Desc: "Deploy all services related to one cluster", Tags: []string{"deploy", "infra"},
			Template: "kubectl apply -k overlays/all", Source: entry.SourceCheat},
		{Desc: "Deploy debug", Tags: []string{"deploy", "infra"},
			Template: "kubectl debug -it <pod>", Source: entry.SourceCheat},
	}
	for _, q := range []string{"deploy related", "related deploy", "services one"} {
		got := descs(Entries(entries, q))
		if len(got) != 1 || got[0] != "Deploy all services related to one cluster" {
			t.Errorf("query %q = %v, want just the 'related' entry", q, got)
		}
	}
	// A term matching nothing drops the entry entirely.
	if got := Entries(entries, "deploy nonexistentword"); len(got) != 0 {
		t.Errorf("query with an unmatched term should return nothing, got %v", descs(got))
	}
}

func descs(entries []entry.Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Desc
	}
	return out
}

// srcCands turns plain source values into src Candidates (Display == Value), the
// shape BuildCandidates now expects for the src list.
func srcCands(values ...string) []Candidate {
	out := make([]Candidate, len(values))
	for i, v := range values {
		out[i] = Candidate{Display: v, Value: v}
	}
	return out
}

// vals extracts the Value of each candidate for order assertions.
func vals(cands []Candidate) []string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.Value
	}
	return out
}

func TestEntriesIdleMostRecentFirst(t *testing.T) {
	entries := []entry.Entry{
		{Desc: "cheat-unused", Template: "a", Source: entry.SourceCheat},
		{Desc: "used-old", Template: "b", Source: entry.SourceHistory, LastUsed: 100},
		{Desc: "used-recent", Template: "c", Source: entry.SourceCheat, LastUsed: 500},
		{Desc: "project-unused", Template: "d", Source: entry.SourceProject},
	}
	got := descs(Entries(entries, ""))
	// Used ones first by recency, then unused by source priority (cheat<project).
	want := []string{"used-recent", "used-old", "cheat-unused", "project-unused"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("idle order = %v, want %v", got, want)
	}
}

func TestEntriesPreserveHistoryProviderRecency(t *testing.T) {
	entries := []entry.Entry{
		{Title: "alphabetically first", Source: entry.SourceHistory, ProviderOrder: 1},
		{Title: "newest command", Source: entry.SourceHistory, ProviderOrder: 3},
		{Title: "middle command", Source: entry.SourceHistory, ProviderOrder: 2},
	}
	got := Entries(entries, "")
	if got[0].Title != "newest command" || got[1].Title != "middle command" {
		t.Errorf("history order = %q, %q, want newest then middle", got[0].Title, got[1].Title)
	}
}

func TestEntriesWeightTitleAboveCommandText(t *testing.T) {
	entries := []entry.Entry{
		{Title: "Deploy service", Template: "run-service", Source: entry.SourceCheat},
		{Title: "Other action", Template: "deploy", Source: entry.SourceCheat},
	}
	got := Entries(entries, "deploy")
	if got[0].Title != "Deploy service" {
		t.Fatalf("weighted ranking put %q first, want title match", got[0].Title)
	}
}

func TestEntriesIdleUnusedBySourceThenDesc(t *testing.T) {
	entries := []entry.Entry{
		{Desc: "zzz", Template: "z", Source: entry.SourceCheat},
		{Desc: "aaa", Template: "a", Source: entry.SourceCheat},
		{Desc: "hist", Template: "h", Source: entry.SourceHistory},
	}
	got := descs(Entries(entries, ""))
	want := []string{"aaa", "zzz", "hist"} // cheats (alpha) before history
	if !reflect.DeepEqual(got, want) {
		t.Errorf("idle unused order = %v, want %v", got, want)
	}
}

func TestEntriesSearchTierWins(t *testing.T) {
	entries := []entry.Entry{
		// A "contains" cheat match vs a "starts-with" history match.
		{Desc: "run deploy script", Template: "x", Source: entry.SourceCheat},
		{Desc: "deploy now", Template: "y", Source: entry.SourceHistory},
	}
	got := descs(Entries(entries, "deploy"))
	// starts-with (history "deploy now") beats contains (cheat) — tier wins.
	want := []string{"deploy now", "run deploy script"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("search tier order = %v, want %v", got, want)
	}
}

func TestEntriesSearchDropsNonMatches(t *testing.T) {
	entries := []entry.Entry{
		{Desc: "git status", Template: "git status", Source: entry.SourceCheat},
		{Desc: "npm build", Template: "npm build", Source: entry.SourceProject},
	}
	got := descs(Entries(entries, "git"))
	if len(got) != 1 || got[0] != "git status" {
		t.Errorf("search should drop non-matches, got %v", got)
	}
}

func TestEntriesSearchRecencyBreaksTierTie(t *testing.T) {
	entries := []entry.Entry{
		{Desc: "deploy a", Template: "a", Source: entry.SourceCheat, LastUsed: 100},
		{Desc: "deploy b", Template: "b", Source: entry.SourceCheat, LastUsed: 900},
	}
	got := descs(Entries(entries, "deploy")) // both starts-with
	want := []string{"deploy b", "deploy a"} // more recent first
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tier-tie recency order = %v, want %v", got, want)
	}
}

func TestCandidatesIdleUsedFirstMostRecent(t *testing.T) {
	cands := BuildCandidates(
		[]Candidate{
			{Value: "main", LastUsed: 100, UseCount: 5},
			{Value: "feature", LastUsed: 900, UseCount: 1},
		},
		srcCands("develop", "release", "main"), // "main" dup ignored
	)
	got := vals(Candidates(cands, ""))
	want := []string{"feature", "main", "develop", "release"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidate idle order = %v, want %v", got, want)
	}
}

func TestCandidatesSearchTier(t *testing.T) {
	cands := BuildCandidates(nil, srcCands("release", "prerelease", "dev"))
	got := vals(Candidates(cands, "rel"))
	// starts-with "release" before contains "prerelease"; "dev" dropped.
	want := []string{"release", "prerelease"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidate search order = %v, want %v", got, want)
	}
}

func TestCandidatesUsedValueMatchesKeepRecency(t *testing.T) {
	cands := BuildCandidates(
		[]Candidate{
			{Value: "deploy-prod", LastUsed: 100},
			{Value: "deploy-beta", LastUsed: 900},
		},
		nil,
	)
	got := vals(Candidates(cands, "deploy"))
	want := []string{"deploy-beta", "deploy-prod"} // both starts-with, recent first
	if !reflect.DeepEqual(got, want) {
		t.Errorf("used-value search order = %v, want %v", got, want)
	}
}

func TestBuildCandidatesDedupPrefersRemembered(t *testing.T) {
	cands := BuildCandidates(
		[]Candidate{{Value: "main", LastUsed: 100, UseCount: 3}},
		srcCands("main", "dev"),
	)
	// "main" should appear once, keeping its usage data.
	if len(cands) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(cands))
	}
	if cands[0].Value != "main" || cands[0].UseCount != 3 {
		t.Errorf("remembered 'main' data lost: %+v", cands[0])
	}
}

// TestBuildCandidatesRememberedAdoptsSrcDisplay checks that a remembered value
// which also appears in the src list adopts the src candidate's readable
// Display (so a recalled value still shows its label when the list has it).
func TestBuildCandidatesRememberedAdoptsSrcDisplay(t *testing.T) {
	cands := BuildCandidates(
		[]Candidate{{Value: "1234", LastUsed: 100, UseCount: 2}},
		[]Candidate{{Display: "chrome (1234)", Value: "1234"}},
	)
	if len(cands) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(cands))
	}
	if cands[0].Display != "chrome (1234)" {
		t.Errorf("remembered value did not adopt src Display: %+v", cands[0])
	}
	if cands[0].UseCount != 2 {
		t.Errorf("usage data lost when adopting Display: %+v", cands[0])
	}
}

// TestBuildCandidatesRememberedDisplayFallsBackToValue checks that a remembered
// value with no matching src line shows its bare value as Display.
func TestBuildCandidatesRememberedDisplayFallsBackToValue(t *testing.T) {
	cands := BuildCandidates([]Candidate{{Value: "9999", LastUsed: 100}}, nil)
	if len(cands) != 1 || cands[0].Display != "9999" {
		t.Fatalf("want Display fallback to value, got %+v", cands)
	}
}
