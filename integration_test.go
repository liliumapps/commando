package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBinaryLifecycle builds the commando binary and drives it through a
// realistic end-to-end lifecycle against an isolated COMMANDO_HOME and a fixture
// shell history file: version, shell init (no capture hook), live history search
// with secret redaction, and bookmark add/list. The interactive TUI is not
// exercised here (it needs a TTY); its logic is covered by internal/tui tests.
func TestBinaryLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary integration test in -short mode")
	}

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "commando")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	home := filepath.Join(tmp, "home")

	// A fixture shell history file, in zsh extended format, including a secret
	// that the redaction denylist must hide.
	histFile := filepath.Join(tmp, "hist")
	histContent := ": 1700000001:0;git status\n" +
		": 1700000002:0;npm run build\n" +
		": 1700000003:0;export API_TOKEN=deadbeef\n" +
		": 1700000004:0;kubectl get pods\n"
	if err := os.WriteFile(histFile, []byte(histContent), 0o644); err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(), "COMMANDO_HOME="+home, "HISTFILE="+histFile)

	run := func(args ...string) (string, string, int) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				code = ee.ExitCode()
			} else {
				t.Fatalf("run %v: %v", args, err)
			}
		}
		return stdout.String(), stderr.String(), code
	}

	// version
	if out, _, code := run("version"); code != 0 || !strings.Contains(out, "commando ") {
		t.Errorf("version: code=%d out=%q", code, out)
	}

	// init zsh emits the widget but NO capture hook.
	out, _, code := run("init", "zsh")
	if code != 0 || !strings.Contains(out, "__commando_widget") {
		t.Errorf("init zsh: code=%d", code)
	}
	if strings.Contains(out, "history record") {
		t.Error("init zsh should not install a capture hook")
	}
	for _, shellName := range []string{"bash", "fish"} {
		out, _, code = run("init", shellName)
		if code != 0 || !strings.Contains(out, "__commando_widget") {
			t.Errorf("init %s: code=%d", shellName, code)
		}
	}

	// history reads the fixture file live: git + npm + kubectl present, secret hidden.
	out, _, code = run("history")
	if code != 0 {
		t.Fatalf("history exit=%d", code)
	}
	for _, want := range []string{"git status", "npm run build", "kubectl get pods"} {
		if !strings.Contains(out, want) {
			t.Errorf("history missing %q; got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "API_TOKEN") {
		t.Errorf("secret command leaked into history:\n%s", out)
	}

	// substring search narrows.
	out, _, _ = run("history", "npm")
	if !strings.Contains(out, "npm run build") || strings.Contains(out, "git status") {
		t.Errorf("substring history search wrong:\n%s", out)
	}

	// bookmarks: add, then list shows it with tag + note.
	if _, errOut, code := run("bookmark", "add", "--tags", "deploy", "--note", "prod rollout", "kubectl rollout restart deploy/web"); code != 0 {
		t.Fatalf("bookmark add failed: code=%d err=%s", code, errOut)
	}
	out, _, _ = run("bookmark", "list")
	if !strings.Contains(out, "kubectl rollout restart deploy/web") || !strings.Contains(out, "deploy") || !strings.Contains(out, "prod rollout") {
		t.Errorf("bookmark list missing content:\n%s", out)
	}

	// --- everything is a plain file; nothing opaque ---
	if _, err := os.Stat(filepath.Join(home, "commando", "commando.db")); !os.IsNotExist(err) {
		t.Error("no SQLite database should exist under the data dir")
	}
	// bookmarks.toml lives in the config dir and is readable TOML.
	bmData, err := os.ReadFile(filepath.Join(home, "commando", "bookmarks.toml"))
	if err != nil {
		t.Fatalf("bookmarks.toml should exist: %v", err)
	}
	if !strings.Contains(string(bmData), "kubectl rollout restart deploy/web") {
		t.Errorf("bookmarks.toml missing the command:\n%s", bmData)
	}
	// Seeded cheats present.
	cheats, _ := filepath.Glob(filepath.Join(home, "commando", "cheats", "*.toml"))
	if len(cheats) < 4 {
		t.Errorf("expected seeded cheats, found %d", len(cheats))
	}
}
