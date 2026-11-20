package app

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/provider"
	"github.com/liliumapps/commando/internal/state"
	"github.com/liliumapps/commando/internal/tui"
)

func matcherFor(t *testing.T, tmpl string) *fillMatcher {
	t.Helper()
	fm := newFillMatcher(tmpl, func(string) bool { return true }, func(string) bool { return false })
	if fm == nil {
		t.Fatalf("no matcher for %q", tmpl)
	}
	return fm
}

func TestFillMatcher(t *testing.T) {
	cases := []struct {
		tmpl, line string
		want       map[string]string // nil = no match
	}{
		{"kubectl logs <pod> -n <ns>", "kubectl logs api-7d9f -n payments", map[string]string{"pod": "api-7d9f", "ns": "payments"}},
		{"kubectl logs <pod> -n <ns>", "  kubectl   logs\tapi -n  pay  ", map[string]string{"pod": "api", "ns": "pay"}},
		// Quoted values: plain words are unquoted, values needing quotes keep them.
		{"kubectl logs <pod> -n <ns>", `kubectl logs 'api' -n "payments"`, map[string]string{"pod": "api", "ns": "payments"}},
		{"git commit -m <msg>", `git commit -m "fix login bug"`, map[string]string{"msg": `"fix login bug"`}},
		{"git commit -m <msg>", `git commit -m 'it''s'`, map[string]string{"msg": `'it''s'`}},
		// Optional segments may be absent or present.
		{"git log [--author=<a>] [-n <count>] -- <file>", "git log -- main.go", map[string]string{"file": "main.go"}},
		{"git log [--author=<a>] [-n <count>] -- <file>", "git log -n 5 -- main.go", map[string]string{"count": "5", "file": "main.go"}},
		{"git log [--author=<a>] [-n <count>] -- <file>", "git log --author=bob -n 5 -- x", map[string]string{"a": "bob", "count": "5", "file": "x"}},
		{"deploy [<env>]", "deploy", map[string]string{}},
		// A repeated variable must capture the same value.
		{"cp <f> <f>.bak", "cp a.txt a.txt.bak", map[string]string{"f": "a.txt"}},
		{"cp <f> <f>.bak", "cp a.txt b.txt.bak", nil},
		// No false positives on prefixes, extensions, or other commands.
		{"git push <remote> <branch>", "git push origin", nil},
		{"git push <remote> <branch>", "git push origin main --force", nil},
		{"git push <remote> <branch>", "git pushx origin main", nil},
		{"git push <remote> <branch>", "echo git push origin main", nil},
		// Regex metacharacters in literal text are literal.
		{"grep -E 'a.b' <file>", "grep -E 'axb' f", nil},
		{"grep -E 'a.b' <file>", "grep -E 'a.b' f", map[string]string{"file": "f"}},
	}
	for _, c := range cases {
		got, ok := matcherFor(t, c.tmpl).match(c.line)
		if c.want == nil {
			if ok {
				t.Errorf("%q ~ %q: unexpected match %v", c.tmpl, c.line, got)
			}
			continue
		}
		if !ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q ~ %q = %v, %v; want %v", c.tmpl, c.line, got, ok, c.want)
		}
	}
}

func TestFillMatcherRejectsUnsafeTemplates(t *testing.T) {
	all := func(string) bool { return true }
	none := func(string) bool { return false }
	for _, tmpl := range []string{"<cmd>", "[sudo <u>] <cmd> <arg>", "ls -la", "bad <unterminated"} {
		if newFillMatcher(tmpl, all, none) != nil {
			t.Errorf("%q should not produce a matcher", tmpl)
		}
	}
}

func TestFillMatcherSkipsUncapturedVars(t *testing.T) {
	fm := newFillMatcher("login <user> <token> <files>",
		func(n string) bool { return n == "user" },
		func(n string) bool { return n == "files" })
	got, ok := fm.match("login bob s3cr3t a.go b.go")
	if !ok || !reflect.DeepEqual(got, map[string]string{"user": "bob"}) {
		t.Fatalf("got %v %v", got, ok)
	}
}

func newFillsBackend(t *testing.T) *fileBackend {
	t.Helper()
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &fileBackend{
		state:  st,
		pctx:   provider.Context{Cwd: "/work", GitRoot: "/repo"},
		redact: func(s string) bool { return strings.Contains(s, "secret") },
	}
}

func kubeCheat() entry.Entry {
	return entry.Entry{
		Title:    "logs",
		Template: "kubectl logs <pod> -n <ns> [--since <since>]",
		Vars:     []entry.VarSpec{{Name: "pod"}, {Name: "ns"}, {Name: "token", Remember: entry.ScopeNone}},
		Source:   entry.SourceCheat,
	}
}

func hist(order int64, cmd string) entry.Entry {
	return entry.Entry{Template: cmd, Kind: entry.CommandLiteral, Source: entry.SourceHistory, ProviderOrder: order}
}

func TestRememberFillPrivacyAndEligibility(t *testing.T) {
	b := newFillsBackend(t)
	e := kubeCheat()
	e.Template = "kubectl logs <pod> -n <ns> --token <token>"

	// remember="none" values are dropped from the record.
	b.RememberFill(e, map[string]string{"pod": "api", "ns": "pay", "token": "t0k"})
	got := b.RecentFills(context.Background(), e, nil)
	if len(got) != 1 || !reflect.DeepEqual(got[0].Values, map[string]string{"pod": "api", "ns": "pay"}) {
		t.Fatalf("fills = %+v", got)
	}
	// One secret-looking value skips the whole record.
	b.RememberFill(e, map[string]string{"pod": "my-secret-pod", "ns": "pay"})
	if got := b.RecentFills(context.Background(), e, nil); len(got) != 1 {
		t.Fatalf("secret record stored: %+v", got)
	}
	// Literal entries never record fills.
	lit := entry.Entry{Template: "ls <x>", Kind: entry.CommandLiteral, Source: entry.SourceHistory}
	b.RememberFill(lit, map[string]string{"x": "1"})
	if got := b.RecentFills(context.Background(), lit, nil); len(got) != 0 {
		t.Fatalf("literal entry has fills: %+v", got)
	}
}

func TestFillScopeIsNarrowest(t *testing.T) {
	b := newFillsBackend(t)
	e := kubeCheat()
	e.Vars = []entry.VarSpec{{Name: "pod", Remember: entry.ScopeDir}, {Name: "ns", Remember: entry.ScopeProject}}
	b.RememberFill(e, map[string]string{"pod": "api", "ns": "pay"})
	if got := b.state.RecallFills(e.ID(), "dir:/work"); len(got) != 1 {
		t.Fatalf("dir-scoped fill missing: %+v", got)
	}
	if got := b.state.RecallFills(e.ID(), "global"); len(got) != 0 {
		t.Fatalf("fill leaked into global: %+v", got)
	}
}

func TestRecentFillsMergesStoredAndHistory(t *testing.T) {
	b := newFillsBackend(t)
	e := kubeCheat()
	b.RememberFill(e, map[string]string{"pod": "api", "ns": "pay"})
	b.RememberFill(e, map[string]string{"pod": "api", "ns": "pay"})

	history := []entry.Entry{
		hist(9, "kubectl logs web -n shop --since 1h"),
		hist(8, "kubectl logs api -n pay"),              // same as stored: merged
		hist(7, "kubectl logs web  -n shop --since 1h"), // same values as the newest
		hist(6, "kubectl logs db -n secret-ns"),         // secret-looking: skipped
		hist(5, "kubectl logs"),                         // prefix only
		hist(4, "kubectl get pods"),
	}
	got := b.RecentFills(context.Background(), e, history)
	want := []tui.Fill{
		{Values: map[string]string{"pod": "api", "ns": "pay"}, Count: 2},
		{Values: map[string]string{"pod": "web", "ns": "shop", "since": "1h"}, Count: 2, FromHistory: true},
	}
	for i := range got {
		got[i].LastUsed = 0
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RecentFills =\n%+v\nwant\n%+v", got, want)
	}

	// Cancelled work returns nothing rather than a partial list.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := b.RecentFills(ctx, e, history); got != nil {
		t.Fatalf("cancelled RecentFills = %+v", got)
	}
}

func TestForgetFillRemovesStored(t *testing.T) {
	b := newFillsBackend(t)
	e := kubeCheat()
	b.RememberFill(e, map[string]string{"pod": "api", "ns": "pay"})
	if err := b.ForgetFill(e, map[string]string{"pod": "api", "ns": "pay"}); err != nil {
		t.Fatal(err)
	}
	if got := b.RecentFills(context.Background(), e, nil); len(got) != 0 {
		t.Fatalf("fill not forgotten: %+v", got)
	}
}
