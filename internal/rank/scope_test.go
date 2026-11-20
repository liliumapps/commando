package rank

import (
	"reflect"
	"testing"

	"github.com/liliumapps/commando/internal/entry"
)

func TestParseScope(t *testing.T) {
	cases := []struct {
		in        string
		wantScope Scope
		wantRest  string
	}{
		{"", ScopeAll, ""},
		{"docker", ScopeAll, "docker"},
		{":h", ScopeHistory, ""},
		{":h git push", ScopeHistory, "git push"},
		{":c build", ScopeCheat, "build"},
		{":p test", ScopeProject, "test"},
		{":b", ScopeBookmark, ""},
		{":z foo", ScopeAll, ":z foo"},               // unknown scope letter → literal
		{":cd", ScopeAll, ":cd"},                     // letter not a bare token → literal
		{"ubuntu:latest", ScopeAll, "ubuntu:latest"}, // colon mid-string → literal
		{":", ScopeAll, ":"},                         // bare colon → literal
		{"  :h  logs", ScopeHistory, "logs"},         // leading + inner whitespace trimmed
	}
	for _, c := range cases {
		gs, gr := ParseScope(c.in)
		if gs != c.wantScope || gr != c.wantRest {
			t.Errorf("ParseScope(%q) = (%v, %q), want (%v, %q)", c.in, gs, gr, c.wantScope, c.wantRest)
		}
	}
}

func TestWithScopeRoundTrips(t *testing.T) {
	for _, s := range []Scope{ScopeAll, ScopeCheat, ScopeProject, ScopeHistory, ScopeBookmark} {
		q := WithScope(s, "foo")
		gs, gr := ParseScope(q)
		if gs != s || gr != "foo" {
			t.Errorf("round-trip %v: WithScope->%q ->(%v,%q)", s, q, gs, gr)
		}
	}
	// An empty search keeps a trailing space so the next keystroke lands after
	// the token (":c" + "d" must not fuse into ":cd").
	if got := WithScope(ScopeCheat, ""); got != ":c " {
		t.Errorf("WithScope(cheat, \"\") = %q, want %q", got, ":c ")
	}
	// ScopeAll has no prefix.
	if got := WithScope(ScopeAll, "foo"); got != "foo" {
		t.Errorf("WithScope(all, \"foo\") = %q, want %q", got, "foo")
	}
}

func TestScopeCycleOrder(t *testing.T) {
	// Forward: all → cheats → project → history → bookmark → all.
	got := []Scope{}
	s := ScopeAll
	for i := 0; i < 5; i++ {
		s = s.Next()
		got = append(got, s)
	}
	want := []Scope{ScopeCheat, ScopeProject, ScopeHistory, ScopeBookmark, ScopeAll}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Next cycle = %v, want %v", got, want)
	}
	// Prev is the exact reverse.
	if ScopeAll.Prev() != ScopeBookmark || ScopeCheat.Prev() != ScopeAll {
		t.Errorf("Prev cycle wrong: all.Prev=%v cheat.Prev=%v", ScopeAll.Prev(), ScopeCheat.Prev())
	}
}

func TestScopeIncludes(t *testing.T) {
	if !ScopeAll.Includes(entry.SourceHistory) || !ScopeAll.Includes(entry.SourceBookmark) {
		t.Error("ScopeAll should include every source")
	}
	if !ScopeCheat.Includes(entry.SourceCheat) || ScopeCheat.Includes(entry.SourceProject) {
		t.Error("ScopeCheat should include only cheats")
	}
	if !ScopeProject.Includes(entry.SourceProject) || ScopeProject.Includes(entry.SourceCheat) {
		t.Error("ScopeProject should include only project entries")
	}
	if !ScopeHistory.Includes(entry.SourceHistory) || ScopeHistory.Includes(entry.SourceCheat) {
		t.Error("ScopeHistory should include only history")
	}
	if !ScopeBookmark.Includes(entry.SourceBookmark) || ScopeBookmark.Includes(entry.SourceProject) {
		t.Error("ScopeBookmark should include only bookmark")
	}
}

func TestEntriesScopedFiltering(t *testing.T) {
	entries := []entry.Entry{
		{Desc: "cheat build", Template: "make build", Source: entry.SourceCheat},
		{Desc: "project test", Template: "npm test", Source: entry.SourceProject},
		{Desc: "history push", Template: "git push", Source: entry.SourceHistory},
		{Desc: "saved deploy", Template: "deploy", Source: entry.SourceBookmark},
	}

	// :c and :p isolate authored and project commands.
	cur := descs(Entries(entries, ":c"))
	if !reflect.DeepEqual(cur, []string{"cheat build"}) {
		t.Errorf(":c = %v, want cheat", cur)
	}
	if got := descs(Entries(entries, ":p")); !reflect.DeepEqual(got, []string{"project test"}) {
		t.Errorf(":p = %v, want project", got)
	}
	// :h and :b isolate their single source.
	if got := descs(Entries(entries, ":h")); !reflect.DeepEqual(got, []string{"history push"}) {
		t.Errorf(":h = %v", got)
	}
	if got := descs(Entries(entries, ":b")); !reflect.DeepEqual(got, []string{"saved deploy"}) {
		t.Errorf(":b = %v", got)
	}
	// No prefix keeps everything.
	if got := Entries(entries, ""); len(got) != 4 {
		t.Errorf("no scope = %d entries, want 4", len(got))
	}
	// Scope + search compose: :c with a term only matches within the scope.
	if got := descs(Entries(entries, ":c build")); !reflect.DeepEqual(got, []string{"cheat build"}) {
		t.Errorf(":c build = %v, want [cheat build]", got)
	}
	// A search term that only exists in history is dropped under :c.
	if got := Entries(entries, ":c push"); len(got) != 0 {
		t.Errorf(":c push = %v, want none (push is only in history)", descs(got))
	}
}
