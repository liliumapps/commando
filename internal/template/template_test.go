package template

import (
	"reflect"
	"testing"
)

func TestParseErrors(t *testing.T) {
	cases := []string{
		"git log <file",      // unterminated <
		"git log <>",         // empty var name
		"git log <bad name>", // space in name
		"git <a b>",          // invalid char
	}
	for _, raw := range cases {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q): expected error, got nil", raw)
		}
	}
}

func TestParseValid(t *testing.T) {
	cases := []string{
		"git status",
		"git log <file>",
		"git log [--author=<author>] [-n <count>] -- <file>",
		`echo \<literal\>`,
		"kubectl logs <pod> [-c <container>]",
		"arr[0] = 1", // stray ] treated as literal
	}
	for _, raw := range cases {
		if _, err := Parse(raw); err != nil {
			t.Errorf("Parse(%q): unexpected error: %v", raw, err)
		}
	}
}

func TestVars(t *testing.T) {
	tpl, err := Parse("git log [--author=<author>] [-n <count>] -- <file>")
	if err != nil {
		t.Fatal(err)
	}
	got := tpl.Vars()
	want := []VarRef{
		{Name: "author", Optional: true},
		{Name: "count", Optional: true},
		{Name: "file", Optional: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Vars() = %+v, want %+v", got, want)
	}
}

func TestVarsRequiredOverridesOptional(t *testing.T) {
	// <x> appears optional first, then required: it must be reported required.
	tpl, err := Parse("cmd [--flag=<x>] <x>")
	if err != nil {
		t.Fatal(err)
	}
	got := tpl.Vars()
	if len(got) != 1 || got[0].Name != "x" || got[0].Optional {
		t.Errorf("Vars() = %+v, want single required x", got)
	}
}

func TestAssembleOptionalSegments(t *testing.T) {
	tpl, err := Parse("git log [--author=<author>] [-n <count>] -- <file>")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{
			name:   "only required",
			values: map[string]string{"file": `"README.md"`},
			want:   `git log -- "README.md"`,
		},
		{
			name:   "count only",
			values: map[string]string{"count": "20", "file": `"README.md"`},
			want:   `git log -n 20 -- "README.md"`,
		},
		{
			name:   "author only",
			values: map[string]string{"author": "alice", "file": "a.go"},
			want:   `git log --author=alice -- a.go`,
		},
		{
			name:   "all provided",
			values: map[string]string{"author": "alice", "count": "5", "file": "a.go"},
			want:   `git log --author=alice -n 5 -- a.go`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tpl.Assemble(tc.values)
			if err != nil {
				t.Fatalf("Assemble: %v", err)
			}
			if got != tc.want {
				t.Errorf("Assemble = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAssembleMissingRequired(t *testing.T) {
	tpl, err := Parse("git log -- <file>")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tpl.Assemble(map[string]string{}); err == nil {
		t.Error("expected error for missing required variable")
	}
}

func TestAssemblePreservesValueSpacing(t *testing.T) {
	// A multi value with internal spaces must survive whitespace collapsing.
	tpl, err := Parse("git log -- <files>")
	if err != nil {
		t.Fatal(err)
	}
	got, err := tpl.Assemble(map[string]string{"files": `"a b.go" "c.go"`})
	if err != nil {
		t.Fatal(err)
	}
	want := `git log -- "a b.go" "c.go"`
	if got != want {
		t.Errorf("Assemble = %q, want %q", got, want)
	}
}

func TestAssembleNestedOptional(t *testing.T) {
	tpl, err := Parse("cmd [--a=<a> [--b=<b>]] end")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		values map[string]string
		want   string
	}{
		{map[string]string{}, "cmd end"},
		{map[string]string{"a": "1"}, "cmd --a=1 end"},
		{map[string]string{"a": "1", "b": "2"}, "cmd --a=1 --b=2 end"},
		// b provided but a missing: outer segment drops, so b is gone too.
		{map[string]string{"b": "2"}, "cmd end"},
	}
	for _, tc := range cases {
		got, err := tpl.Assemble(tc.values)
		if err != nil {
			t.Fatalf("Assemble(%v): %v", tc.values, err)
		}
		if got != tc.want {
			t.Errorf("Assemble(%v) = %q, want %q", tc.values, got, tc.want)
		}
	}
}

func TestAssembleEscapes(t *testing.T) {
	tpl, err := Parse(`echo \<not a var\> and \[not opt\]`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tpl.Assemble(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	want := "echo <not a var> and [not opt]"
	if got != want {
		t.Errorf("Assemble = %q, want %q", got, want)
	}
}

func TestFormatMulti(t *testing.T) {
	cases := []struct {
		items []string
		sep   string
		each  string
		want  string
	}{
		{[]string{"a.go", "b.go"}, " ", `"{}"`, `"a.go" "b.go"`},
		{[]string{"x", "y", "z"}, ",", "", "x,y,z"},
		{[]string{"one"}, " ", "-f {}", "-f one"},
		{nil, " ", `"{}"`, ""},
		{[]string{"a", "b"}, "", "", "a b"}, // empty sep defaults to space
	}
	for _, tc := range cases {
		got := FormatMulti(tc.items, tc.sep, tc.each)
		if got != tc.want {
			t.Errorf("FormatMulti(%v, %q, %q) = %q, want %q", tc.items, tc.sep, tc.each, got, tc.want)
		}
	}
}

func TestAssembleNoDoubleSpaces(t *testing.T) {
	// Multiple adjacent optional segments, all skipped, must not leave gaps.
	tpl, err := Parse("cmd [<a>] [<b>] [<c>] tail")
	if err != nil {
		t.Fatal(err)
	}
	got, err := tpl.Assemble(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "cmd tail" {
		t.Errorf("Assemble = %q, want %q", got, "cmd tail")
	}
}

func TestShellSyntaxKeptVerbatim(t *testing.T) {
	cases := []struct{ tmpl, want string }{
		{`grep -E '[0-9]+' <file>`, `grep -E '[0-9]+' x.txt`},
		{`printf 'a\tb\n' > <out>`, `printf 'a\tb\n' > o`},
		{`find . -name \*.go`, `find . -name \*.go`},
		{`[[ -f <f> ]] && echo ok`, `[[ -f a ]] && echo ok`},
		{`sort < <file>`, `sort < x.txt`},
		{`cat <<EOF`, `cat <<EOF`},
		{`diff <(ls) <(ls -a)`, `diff <(ls) <(ls -a)`},
		{`echo "a   b" [-n <count>]`, `echo "a   b"`},
		{`arr[0]=1; echo ${arr[0]} <file>`, `arr[0]=1; echo ${arr[0]} x.txt`},
		{`grep '[' <file>`, `grep '[' x.txt`},
		{`echo \<lit\> \\ <file>`, `echo <lit> \ x.txt`},
		{"cmd \\\n  --flag <file>", "cmd \\\n  --flag x.txt"},
	}
	vals := map[string]string{"file": "x.txt", "out": "o", "f": "a"}
	for _, tc := range cases {
		tpl, err := Parse(tc.tmpl)
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.tmpl, err)
			continue
		}
		got, err := tpl.Assemble(vals)
		if err != nil {
			t.Errorf("Assemble(%q): %v", tc.tmpl, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Assemble(%q) = %q, want %q", tc.tmpl, got, tc.want)
		}
	}
}
