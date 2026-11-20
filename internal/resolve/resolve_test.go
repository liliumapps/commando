package resolve

import (
	"testing"

	"github.com/liliumapps/commando/internal/entry"
)

func sampleEntry() entry.Entry {
	return entry.Entry{
		Desc:     "Search git log by author for a file",
		Template: "git log [--author=<author>] [-n <count>] -- <file>",
		Vars: []entry.VarSpec{
			{Name: "author", Src: "git log --format='%an' | sort -u"},
			{Name: "count", Default: "20"},
			{Name: "file", Multi: true, Each: `"{}"`, Remember: entry.ScopeProject},
		},
	}
}

func TestNewPlanFields(t *testing.T) {
	p, err := NewPlan(sampleEntry())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(p.Fields))
	}
	if !p.Fields[0].Optional || !p.Fields[1].Optional {
		t.Error("author and count should be optional")
	}
	if p.Fields[2].Optional {
		t.Error("file should be required")
	}
	if p.Fields[1].Spec.Default != "20" {
		t.Errorf("count default = %q, want 20", p.Fields[1].Spec.Default)
	}
	if !p.Fields[2].Spec.Multi {
		t.Error("file should be multi")
	}
}

func TestPlanMissingSpecGetsDefault(t *testing.T) {
	// Template references <x> but the entry has no VarSpec for it.
	e := entry.Entry{Template: "echo <x>"}
	p, err := NewPlan(e)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Fields) != 1 || p.Fields[0].Name != "x" {
		t.Fatalf("unexpected fields: %+v", p.Fields)
	}
}

func TestPlanAssemble(t *testing.T) {
	p, err := NewPlan(sampleEntry())
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Assemble(map[string]string{"count": "5", "file": `"a.go" "b.go"`})
	if err != nil {
		t.Fatal(err)
	}
	want := `git log -n 5 -- "a.go" "b.go"`
	if got != want {
		t.Errorf("Assemble = %q, want %q", got, want)
	}
}

func TestPlanPreviewShowsPlaceholder(t *testing.T) {
	p, err := NewPlan(sampleEntry())
	if err != nil {
		t.Fatal(err)
	}
	// Nothing filled: required <file> should render as a placeholder, optional
	// segments dropped.
	got := p.Preview(map[string]string{})
	want := "git log -- <file>"
	if got != want {
		t.Errorf("Preview = %q, want %q", got, want)
	}
}

func TestPlanHasVars(t *testing.T) {
	p, _ := NewPlan(entry.Entry{Template: "make build"})
	if p.HasVars() {
		t.Error("template with no vars should report HasVars=false")
	}
	p2, _ := NewPlan(entry.Entry{Template: "make <target>"})
	if !p2.HasVars() {
		t.Error("template with a var should report HasVars=true")
	}
}

func TestNewPlanBadTemplate(t *testing.T) {
	if _, err := NewPlan(entry.Entry{Template: "git log <file"}); err == nil {
		t.Error("expected error for malformed template")
	}
}
