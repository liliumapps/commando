// Package app wires the pieces of commando together: it builds the provider
// context, aggregates entries from all providers, applies frecency ranking, and
// adapts the store into the TUI's Backend. It is the composition root used by
// the CLI.
package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/liliumapps/commando/internal/provider"
)

// buildContext determines the working directory, git root, and environment for
// providers.
func buildContext() provider.Context {
	cwd, _ := os.Getwd()
	return provider.Context{
		Cwd:     cwd,
		GitRoot: gitRoot(cwd),
		Env:     envMap(),
	}
}

// gitRoot returns the git repository root containing dir, or "" if dir is not in
// a repository. It first tries `git rev-parse` and falls back to walking up for
// a .git directory so detection works even without the git binary on PATH.
func gitRoot(dir string) string {
	if dir == "" {
		return ""
	}
	if out, err := runGitTopLevel(dir); err == nil && out != "" {
		return out
	}
	d := dir
	for {
		// .git is a directory in a normal clone but a file ("gitdir: ...") in
		// worktrees and submodules; either marks the repository root.
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// gitTimeout bounds `git rev-parse` so a hung git (network filesystem, lock
// contention, credential prompt) cannot stall startup.
const gitTimeout = 2 * time.Second

func runGitTopLevel(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.WaitDelay = 100 * time.Millisecond
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func envMap() map[string]string {
	env := os.Environ()
	m := make(map[string]string, len(env))
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	return m
}
