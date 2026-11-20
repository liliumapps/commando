package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"version"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), "commando ") {
		t.Errorf("version output = %q", out.String())
	}
}

func TestHelp(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"help"}, &out, &errb); code != 0 {
		t.Fatalf("help exit = %d", code)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Errorf("help output missing usage: %q", out.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"bogus"}, &out, &errb)
	if code != 2 {
		t.Errorf("unknown command exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "unknown command") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestInitZsh(t *testing.T) {
	var out, errb bytes.Buffer
	code := cmdInit([]string{"zsh"}, &out, &errb)
	if code != 0 {
		t.Fatalf("init zsh exit = %d", code)
	}
	s := out.String()
	for _, want := range []string{"__commando_widget", "bindkey", "commando search --print"} {
		if !strings.Contains(s, want) {
			t.Errorf("init zsh output missing %q", want)
		}
	}
	// The capture hook is gone (history is read live from the shell file).
	if strings.Contains(s, "history record") {
		t.Error("init zsh should no longer install a history capture hook")
	}
}

func TestInitBashAndFish(t *testing.T) {
	t.Setenv("COMMANDO_HOME", t.TempDir())
	cases := []struct {
		shell string
		want  []string
	}{
		{"bash", []string{"READLINE_LINE", "bind -m vi-insert -x", "commando search --print"}},
		{"fish", []string{"commandline --replace", "bind \\cg", "commando search --print"}},
	}
	for _, tc := range cases {
		t.Run(tc.shell, func(t *testing.T) {
			var out, errb bytes.Buffer
			if code := cmdInit([]string{tc.shell}, &out, &errb); code != 0 {
				t.Fatalf("init %s exit = %d: %s", tc.shell, code, errb.String())
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("init %s output missing %q", tc.shell, want)
				}
			}
		})
	}
}

func TestReorderFlagsFirst(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"deploy", "--limit", "3"}, []string{"--limit", "3", "deploy"}},
		{[]string{"--limit", "3", "deploy"}, []string{"--limit", "3", "deploy"}},
		{[]string{"git", "status"}, []string{"git", "status"}},
		{[]string{"--limit=5", "npm"}, []string{"--limit=5", "npm"}},
		{[]string{"npm", "run", "--limit", "10"}, []string{"--limit", "10", "npm", "run"}},
		{nil, nil},
	}
	for _, tc := range cases {
		got := reorderFlagsFirst(tc.in)
		if strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Errorf("reorderFlagsFirst(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestInitUnsupportedShell(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdInit([]string{"powershell"}, &out, &errb); code != 2 {
		t.Errorf("unsupported shell exit = %d, want 2", code)
	}
}

func TestInitNoShell(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdInit(nil, &out, &errb); code != 2 {
		t.Errorf("init with no shell exit = %d, want 2", code)
	}
}

func TestAddWritesExplicitNoMemoryDefault(t *testing.T) {
	t.Setenv("COMMANDO_HOME", t.TempDir())
	input, output, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() {
		os.Stdin = oldStdin
		input.Close()
	})

	answers := "Deploy\nProduction deploy\ndeploy <token>\nops\n\n\nn\n\n"
	if _, err := output.WriteString(answers); err != nil {
		t.Fatal(err)
	}
	output.Close()

	var stdout, stderr bytes.Buffer
	if code := cmdAdd(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("add exit=%d stderr=%s", code, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(os.Getenv("COMMANDO_HOME"), "commando", "cheats", "user.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `remember = "none"`) {
		t.Fatalf("privacy default was not persisted explicitly:\n%s", data)
	}
}

func TestAppendCheatRejectsInvalidExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.toml")
	original := []byte("[[cmd")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	fragment := []byte("\n[[cmd]]\ntitle = \"ok\"\ntmpl = \"echo ok\"\n")
	if err := appendCheat(path, fragment); err == nil {
		t.Fatal("expected invalid existing file to block append")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("invalid file was modified: %q", got)
	}
}

func TestDoctorReportsChecks(t *testing.T) {
	t.Setenv("COMMANDO_HOME", t.TempDir())
	history := filepath.Join(t.TempDir(), "history")
	if err := os.WriteFile(history, []byte("git status\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HISTFILE", history)

	var stdout, stderr bytes.Buffer
	if code := cmdDoctor(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("doctor exit=%d stderr=%s", code, stderr.String())
	}
	for _, want := range []string{"config", "cheats", "state", "history", "providers", "shell"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("doctor output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestPromoteWritesParameterizedCheat(t *testing.T) {
	t.Setenv("COMMANDO_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	code := cmdPromote([]string{
		"--title", "Pod logs",
		"--tags", "k8s,logs",
		"kubectl logs <pod>",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("promote exit=%d stderr=%s", code, stderr.String())
	}
	path := filepath.Join(os.Getenv("COMMANDO_HOME"), "commando", "cheats", "user.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`title = "Pod logs"`, `tmpl = "kubectl logs <pod>"`, `remember = "none"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("promoted cheat missing %q:\n%s", want, data)
		}
	}
}

func TestBookmarkAddRefusesSecretUnlessForced(t *testing.T) {
	t.Setenv("COMMANDO_HOME", t.TempDir())
	secret := "curl -H 'Authorization: Bearer abc123' https://api.example.com"
	var out, errOut bytes.Buffer
	if code := run([]string{"bookmark", "add", secret}, &out, &errOut); code == 0 {
		t.Fatalf("secret-looking bookmark was saved; stderr=%q", errOut.String())
	}
	out.Reset()
	if code := run([]string{"bookmark", "add", "--force", secret}, &out, &errOut); code != 0 {
		t.Fatalf("--force should save; code=%d stderr=%q", code, errOut.String())
	}
}

func TestCheatsCommand(t *testing.T) {
	t.Setenv("COMMANDO_HOME", t.TempDir())
	var out, errb bytes.Buffer
	if code := runCheats([]string{"list"}, &out, &errb, strings.NewReader(""), false); code != 0 || !strings.Contains(out.String(), "No cheat sources") {
		t.Fatalf("list = %d %q %q", code, out.String(), errb.String())
	}
	for _, args := range [][]string{nil, {"bogus"}, {"add"}, {"add", "a", "b", "c"}, {"add", "--name", "x", "https://h/x"}, {"update", "x"}, {"remove"}} {
		out.Reset()
		errb.Reset()
		if code := runCheats(args, &out, &errb, strings.NewReader(""), false); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
	errb.Reset()
	if code := runCheats([]string{"add", "/tmp/local/repo"}, &out, &errb, strings.NewReader(""), false); code != 1 || !strings.Contains(errb.String(), "unsupported repository URL") {
		t.Errorf("local path: %d %q", code, errb.String())
	}
	errb.Reset()
	if code := runCheats([]string{"add", "https://h/x", "../x.toml"}, &out, &errb, strings.NewReader(""), false); code != 1 || !strings.Contains(errb.String(), "inside the repository") {
		t.Errorf("bad file: %d %q", code, errb.String())
	}
	errb.Reset()
	if code := runCheats([]string{"remove", "https://h/x"}, &out, &errb, strings.NewReader(""), false); code != 1 || !strings.Contains(errb.String(), "not a source") {
		t.Errorf("remove missing: %d %q", code, errb.String())
	}
}

func TestCheatsConfirmer(t *testing.T) {
	var out, errb bytes.Buffer
	if confirmer(&out, &errb, strings.NewReader("y\n"), true, false)() != true {
		t.Error("y on a tty should trust")
	}
	if confirmer(&out, &errb, strings.NewReader("\n"), true, false)() {
		t.Error("the default is no")
	}
	if confirmer(&out, &errb, strings.NewReader("y\n"), false, false)() {
		t.Error("without a terminal nothing is trusted")
	}
	if !confirmer(&out, &errb, strings.NewReader(""), false, true)() {
		t.Error("--yes trusts")
	}
}
