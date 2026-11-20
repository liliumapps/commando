package resolve

import (
	"reflect"
	"testing"
)

func TestSrcRefs(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"", nil},
		{"echo hi", nil},
		{"kubectl get pods -n <namespace> -o json | jq -r '.items[0].metadata.name'", []string{"namespace"}},
		{"kubectl logs <pod> -c <container> <flags>", []string{"pod", "container", "flags"}},
		{"echo <a> <a> <b>", []string{"a", "b"}}, // de-duped, order preserved
		// Shell metacharacters that must NOT be treated as template syntax:
		{"jq -r '.[0].id'", nil},
		{`sed 's/\x1b\[[0-9;]*m//g'`, nil},
		{"awk '{print $0}'", nil},
		{"cat a[b].txt", nil},
		{"echo <>", nil}, // empty name is not a ref
	}
	for _, tc := range cases {
		got := SrcRefs(tc.src)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("SrcRefs(%q) = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestSubstituteRefs(t *testing.T) {
	cases := []struct {
		src  string
		vals map[string]string
		want string
	}{
		{
			src:  "kubectl get pods -n <namespace> <flags>",
			vals: map[string]string{"namespace": "kube-system", "flags": "-o"},
			want: "kubectl get pods -n kube-system -o",
		},
		{
			// Missing value -> empty substitution.
			src:  "kubectl get pods -n <namespace> <flags>",
			vals: map[string]string{"namespace": "abc"},
			want: "kubectl get pods -n abc ",
		},
		{
			// Shell metacharacters preserved verbatim around a placeholder.
			src:  `kubectl get pods -n <namespace> -o json | jq -r '.items[0].metadata.name'`,
			vals: map[string]string{"namespace": "ns1"},
			want: `kubectl get pods -n ns1 -o json | jq -r '.items[0].metadata.name'`,
		},
		{
			// No placeholders -> unchanged, brackets/backslashes intact.
			src:  `sed 's/\x1b\[[0-9;]*m//g'`,
			vals: map[string]string{"x": "y"},
			want: `sed 's/\x1b\[[0-9;]*m//g'`,
		},
	}
	for _, tc := range cases {
		got := SubstituteRefs(tc.src, tc.vals)
		if got != tc.want {
			t.Errorf("SubstituteRefs(%q, %v) = %q, want %q", tc.src, tc.vals, got, tc.want)
		}
	}
}

func TestSubstituteShellRefsQuotesByDefault(t *testing.T) {
	got := SubstituteShellRefs("git commit -m <msg> <flag:raw>", map[string]string{
		"msg":  "fix typo",
		"flag": "--amend",
	})
	want := "git commit -m 'fix typo' --amend"
	if got != want {
		t.Errorf("SubstituteShellRefs = %q, want %q", got, want)
	}
}

func TestShellQuote(t *testing.T) {
	if got, want := ShellQuote("it's fine"), `'it'"'"'s fine'`; got != want {
		t.Errorf("ShellQuote = %q, want %q", got, want)
	}
	if got := ShellQuote(""); got != "''" {
		t.Errorf("empty ShellQuote = %q", got)
	}
}
