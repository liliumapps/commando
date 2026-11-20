package sources

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// safetyConfig is passed to every git invocation. Hooks are disabled, only the
// https and ssh transports are allowed (no file://, ext::, or other helpers),
// symlinks are checked out as plain files, and nothing recurses into
// submodules. Normal authentication (ssh keys, credential helpers) still works.
var safetyConfig = []string{
	"-c", "core.hooksPath=/dev/null",
	"-c", "core.symlinks=false",
	"-c", "core.fsmonitor=false",
	"-c", "submodule.recurse=false",
	"-c", "protocol.allow=never",
	"-c", "protocol.https.allow=always",
	"-c", "protocol.ssh.allow=always",
}

// repoEnv lists variables that would point git at a different repository than
// the one commando names; they are dropped from the inherited environment.
var repoEnv = []string{
	"GIT_DIR=", "GIT_WORK_TREE=", "GIT_INDEX_FILE=", "GIT_OBJECT_DIRECTORY=",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES=", "GIT_NAMESPACE=", "GIT_COMMON_DIR=",
}

// git runs one git command. When repo is set, the command is pinned to that
// clone (GIT_DIR and GIT_WORK_TREE), so it can never act on an enclosing
// repository such as a dotfiles checkout of the home directory.
func git(repo string, args ...string) (string, error) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return "", errors.New("git is required for cheat sources but was not found on PATH")
	}
	full := append([]string{}, safetyConfig...)
	if allowFileURLs {
		full = append(full, "-c", "protocol.file.allow=always")
	}
	full = append(full, args...)
	cmd := exec.Command(bin, full...)
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		drop := false
		for _, prefix := range repoEnv {
			if strings.HasPrefix(kv, prefix) {
				drop = true
				break
			}
		}
		if !drop {
			env = append(env, kv)
		}
	}
	if repo != "" {
		if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
			return "", fmt.Errorf("%s is not a git clone", repo)
		}
		cmd.Dir = repo
		env = append(env, "GIT_DIR="+filepath.Join(repo, ".git"), "GIT_WORK_TREE="+repo)
	}
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// gitClone makes a shallow clone of the remote's default branch into dir and
// detaches HEAD, so the checked-out commit is recorded plainly in .git/HEAD.
func gitClone(remote, dir string) (string, error) {
	if _, err := git("", "clone", "--quiet", "--depth", "1", "--no-tags", "--no-recurse-submodules", "--", remote, dir); err != nil {
		return "", err
	}
	if _, err := git(dir, "checkout", "--quiet", "--detach"); err != nil {
		return "", err
	}
	return gitHead(dir)
}

// gitFetch fetches the remote's default branch (depth 1) and returns its
// commit without changing the checkout.
func gitFetch(dir string) (string, error) {
	if _, err := git(dir, "fetch", "--quiet", "--depth", "1", "--no-tags", "--no-recurse-submodules", "origin", "HEAD"); err != nil {
		return "", err
	}
	return git(dir, "rev-parse", "--verify", "FETCH_HEAD^{commit}")
}

// gitReset hard-resets the clone to commit with HEAD detached, discarding
// any local edits to tracked files.
func gitReset(dir, commit string) error {
	_, err := git(dir, "checkout", "--quiet", "--force", "--detach", commit)
	return err
}

// gitHead returns the commit checked out in a clone. A detached HEAD (how
// commando leaves its clones) is read straight from .git/HEAD, so loading
// cheats does not start a git process; anything else asks git.
func gitHead(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
	if err != nil {
		return "", err
	}
	if head := strings.TrimSpace(string(data)); commitRE.MatchString(head) {
		return head, nil
	}
	return git(dir, "rev-parse", "--verify", "HEAD^{commit}")
}

// isClone reports whether dir holds a clone.
func isClone(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	if commit == "" {
		return "-"
	}
	return commit
}
