package entry

import "testing"

func TestIDStableAndDistinct(t *testing.T) {
	a := Entry{Desc: "list pods", Template: "kubectl get pods", Source: SourceCheat}
	b := Entry{Desc: "list pods", Template: "kubectl get pods", Source: SourceCheat}
	c := Entry{Desc: "list pods", Template: "kubectl get pods -A", Source: SourceCheat}

	if a.ID() != b.ID() {
		t.Error("identical entries should share an ID")
	}
	if a.ID() == c.ID() {
		t.Error("entries with different templates should have different IDs")
	}
	if len(a.ID()) != 16 {
		t.Errorf("ID length = %d, want 16", len(a.ID()))
	}
}

func TestSourceString(t *testing.T) {
	cases := map[Source]string{
		SourceCheat:   "cheat",
		SourceProject: "project",
		SourceHistory: "history",
		Source(99):    "unknown",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("Source(%d).String() = %q, want %q", s, got, want)
		}
	}
}

func TestSearchText(t *testing.T) {
	e := Entry{Desc: "commit all", Tags: []string{"git", "commit"}}
	if got, want := e.SearchText(), "commit all git commit"; got != want {
		t.Errorf("SearchText = %q, want %q", got, want)
	}
	e2 := Entry{Title: "short", Desc: "long detail", Template: "git status", Tags: []string{"git"}}
	if got, want := e2.SearchText(), "short long detail git status git"; got != want {
		t.Errorf("SearchText = %q, want %q", got, want)
	}
}

func TestTitleAndDescFallbacks(t *testing.T) {
	legacy := Entry{Desc: "legacy desc", Template: "cmd"}
	if got := legacy.TitleText(); got != "legacy desc" {
		t.Errorf("legacy TitleText = %q", got)
	}
	if got := legacy.DescText(); got != "legacy desc" {
		t.Errorf("legacy DescText = %q", got)
	}
	titled := Entry{Title: "title", Template: "cmd"}
	if got := titled.DescText(); got != "title" {
		t.Errorf("empty desc should fall back to title, got %q", got)
	}
}

func TestVarLookup(t *testing.T) {
	e := Entry{Vars: []VarSpec{{Name: "a"}, {Name: "b", Default: "x"}}}
	if v, ok := e.Var("b"); !ok || v.Default != "x" {
		t.Errorf("Var(b) = %+v, %v", v, ok)
	}
	if _, ok := e.Var("z"); ok {
		t.Error("Var(z) should not be found")
	}
}

func TestScopeValid(t *testing.T) {
	for _, s := range []Scope{ScopeDefault, ScopeNone, ScopeGlobal, ScopeProject, ScopeDir} {
		if !s.Valid() {
			t.Errorf("scope %q should be valid", s)
		}
	}
	if Scope("bogus").Valid() {
		t.Error("bogus scope should be invalid")
	}
}

func TestScopeDefaultsToGlobal(t *testing.T) {
	// An unset scope means remember globally (memory on by default).
	if ScopeDefault.Effective() != ScopeGlobal {
		t.Errorf("unset scope should resolve to global, got %q", ScopeDefault.Effective())
	}
	if !ScopeDefault.Remembers() {
		t.Error("unset scope should remember by default")
	}
	// Explicit "none" opts out.
	if ScopeNone.Remembers() {
		t.Error("scope 'none' should not remember")
	}
	// Named scopes pass through unchanged and remember.
	for _, s := range []Scope{ScopeGlobal, ScopeProject, ScopeDir} {
		if s.Effective() != s || !s.Remembers() {
			t.Errorf("scope %q should pass through and remember", s)
		}
	}
}

func TestSepOrDefault(t *testing.T) {
	if got := (VarSpec{}).SepOrDefault(); got != " " {
		t.Errorf("empty sep = %q, want space", got)
	}
	if got := (VarSpec{Sep: ","}).SepOrDefault(); got != "," {
		t.Errorf("sep = %q, want comma", got)
	}
}

func TestStableIDOverridesHash(t *testing.T) {
	a := Entry{Title: "a", Template: "x", Source: SourceCheat, StableID: "my.id"}
	b := Entry{Title: "b", Template: "y", Source: SourceCheat, StableID: "my.id"}
	if a.ID() != b.ID() || a.ID() != "cheat:my.id" {
		t.Errorf("IDs = %q, %q", a.ID(), b.ID())
	}
	if (Entry{Title: "a", Template: "x"}).ID() == a.ID() {
		t.Error("entries without StableID keep the hashed identity")
	}
}

func TestPrepareSearchLowercasesAndSkipsDuplicates(t *testing.T) {
	e := Entry{Title: "Git Status", Desc: "git status", Template: "git status", Tags: []string{"VCS"}}
	e.PrepareSearch()
	f := e.LowerSearchFields()
	if f.Title != "git status" || f.Tags != "vcs" || f.Template != "" || f.Desc != "" {
		t.Errorf("fields = %+v", f)
	}
	if (&Entry{Title: "X"}).LowerSearchFields().Title != "x" {
		t.Error("unprepared entries compute fields on the fly")
	}
}
