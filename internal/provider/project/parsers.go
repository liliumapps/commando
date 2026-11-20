package project

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/liliumapps/commando/internal/entry"
)

// makeNameRE matches a plain make target name. It rejects special targets
// (.PHONY, .DEFAULT, ...), pattern rules (%), and variable references ($).
var makeNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/+-]*$`)
var makeHelpRE = regexp.MustCompile(`##\s*(.+)$`)

// localFound describes a marker in the working directory itself, so commands
// need no directory flags. It backs the path-only parser helpers.
func localFound(path string) found {
	return found{dir: filepath.Dir(path), path: path, marker: filepath.Base(path)}
}

// parseMakefile extracts target names from a Makefile in the working
// directory.
func parseMakefile(path string) []entry.Entry { return makeEntries(localFound(path)) }

// makeEntries builds `make` commands for the targets in f. A Makefile found in
// a parent directory is addressed with `make -C <dir>`.
func makeEntries(f found) []entry.Entry {
	names, help := makeTargets(f.path)
	cmd := "make"
	if rel := f.relDir(); rel != "" {
		cmd += " -C " + rel
	}
	entries := make([]entry.Entry, 0, len(names))
	for _, n := range names {
		tmpl := cmd + " " + quoteProviderItem(n)
		entries = append(entries, entry.Entry{
			Title:    tmpl,
			Desc:     help[n],
			Tags:     []string{"make", "project"},
			Template: tmpl,
			Kind:     entry.CommandLiteral,
			Source:   entry.SourceProject,
		})
	}
	return entries
}

// makeTargets returns the sorted target names in a makefile and any `## help`
// text attached to them. Rule lines may declare several targets
// (`build test: deps`) and double-colon rules; variable assignments of every
// flavour (`=`, `:=`, `::=`, `:::=`, `?=`, `+=`, `!=`), `define` blocks,
// recipe lines, special targets, and pattern rules are skipped.
func makeTargets(path string) (targets []string, descs map[string]string) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, nil
	}
	defer fh.Close()

	seen := make(map[string]bool)
	var names []string
	help := make(map[string]string)
	inDefine := false
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if strings.HasPrefix(line, "\t") {
			continue // recipe line, not a target
		}
		trimmed := strings.TrimSpace(line)
		if inDefine {
			if trimmed == "endef" || strings.HasPrefix(trimmed, "endef ") {
				inDefine = false
			}
			continue
		}
		if isDefineLine(trimmed) {
			inDefine = true
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		code := line
		if i := strings.IndexByte(code, '#'); i >= 0 {
			code = code[:i]
		}
		colon := strings.IndexByte(code, ':')
		if colon < 0 {
			continue
		}
		if eq := strings.IndexByte(code, '='); eq >= 0 && eq < colon {
			continue // FOO = a:b, FOO ?= x, FOO += x, FOO != cmd
		}
		rest := code[colon:]
		if strings.HasPrefix(rest, ":=") || strings.HasPrefix(rest, "::=") || strings.HasPrefix(rest, ":::=") {
			continue // simply-expanded / POSIX assignments
		}
		text := ""
		if hm := makeHelpRE.FindStringSubmatch(line); hm != nil {
			text = strings.TrimSpace(hm[1])
		}
		for _, name := range strings.Fields(code[:colon]) {
			if !makeNameRE.MatchString(name) || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
			if text != "" {
				help[name] = text
			}
		}
	}
	sort.Strings(names)
	return names, help
}

func isDefineLine(trimmed string) bool {
	for _, prefix := range []string{"define", "override define", "export define", "private define"} {
		if trimmed == prefix || strings.HasPrefix(trimmed, prefix+" ") || strings.HasPrefix(trimmed, prefix+"\t") {
			return true
		}
	}
	return false
}

// packageJSON is the subset of package.json we read.
type packageJSON struct {
	Scripts        map[string]string `json:"scripts"`
	PackageManager string            `json:"packageManager"`
}

// parsePackageJSON extracts npm/yarn/pnpm scripts from a package.json in the
// working directory.
func parsePackageJSON(path, dir string) []entry.Entry {
	f := localFound(path)
	f.dir = dir
	return packageEntries(f)
}

// packageEntries extracts npm/yarn/pnpm/bun scripts. The runner comes from the
// packageManager field or a lockfile, searched from the package directory up
// to the git root so workspace packages in a monorepo use the root's manager.
// A package.json in a parent directory is addressed with the runner's
// directory flag.
func packageEntries(f found) []entry.Entry {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}
	runner := detectNodeRunner(f.dir, f.stop, pkg.PackageManager)

	cmd := runner
	if rel := f.relDir(); rel != "" {
		switch runner {
		case "npm":
			cmd += " --prefix " + rel
		case "pnpm":
			cmd += " --dir " + rel
		default: // yarn, bun
			cmd += " --cwd " + rel
		}
	}

	names := make([]string, 0, len(pkg.Scripts))
	for name := range pkg.Scripts {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]entry.Entry, 0, len(names))
	for _, n := range names {
		tmpl := cmd + " run " + quoteProviderItem(n)
		entries = append(entries, entry.Entry{
			Title:    tmpl,
			Desc:     pkg.Scripts[n],
			Tags:     []string{runner, "npm", "script", "project"},
			Template: tmpl,
			Kind:     entry.CommandLiteral,
			Source:   entry.SourceProject,
		})
	}
	return entries
}

// nodeLockfiles maps lockfiles to their package manager, in precedence order.
var nodeLockfiles = []struct{ file, runner string }{
	{"pnpm-lock.yaml", "pnpm"},
	{"yarn.lock", "yarn"},
	{"bun.lock", "bun"},
	{"bun.lockb", "bun"},
	{"package-lock.json", "npm"},
	{"npm-shrinkwrap.json", "npm"},
}

// detectNodeRunner picks the package manager for a package in dir: its own
// packageManager field first, then — walking from dir up to stop (the git
// root; only dir itself when stop is empty) — an ancestor package.json's
// packageManager or a lockfile. The default is npm.
func detectNodeRunner(dir, stop, packageManager string) string {
	if r := runnerFromPackageManager(packageManager); r != "" {
		return r
	}
	d := dir
	for {
		if d != dir {
			if data, err := os.ReadFile(filepath.Join(d, "package.json")); err == nil {
				var pkg packageJSON
				if json.Unmarshal(data, &pkg) == nil {
					if r := runnerFromPackageManager(pkg.PackageManager); r != "" {
						return r
					}
				}
			}
		}
		for _, lf := range nodeLockfiles {
			if fileExists(filepath.Join(d, lf.file)) {
				return lf.runner
			}
		}
		if stop == "" || samePath(d, stop) || !within(d, stop) {
			break
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return "npm"
}

func runnerFromPackageManager(packageManager string) string {
	if i := strings.IndexByte(packageManager, '@'); i > 0 {
		packageManager = packageManager[:i]
	}
	switch packageManager {
	case "npm", "pnpm", "yarn", "bun":
		return packageManager
	}
	return ""
}

// cargoEntries emits the common cargo subcommands. Cargo.toml does not
// enumerate tasks, so we offer the standard set; a manifest in a parent
// directory is addressed with --manifest-path.
func cargoEntries(f found) []entry.Entry {
	suffix := ""
	if f.relDir() != "" {
		suffix = " --manifest-path " + f.relPath(filepath.Base(f.path))
	}
	subs := []string{"build", "run", "test", "check", "clippy", "fmt", "bench", "update"}
	entries := make([]entry.Entry, 0, len(subs))
	for _, s := range subs {
		cmd := "cargo " + s + suffix
		entries = append(entries, entry.Entry{
			Title:    cmd,
			Tags:     []string{"cargo", "rust", "project"},
			Template: cmd,
			Kind:     entry.CommandLiteral,
			Source:   entry.SourceProject,
		})
	}
	return entries
}

// gradleEntries emits common gradle tasks. The wrapper is searched from the
// build's directory up to the git root (multi-project builds keep it at the
// root) and invoked by its path from the working directory; without a wrapper
// a build in another directory is addressed with `-p <dir>`.
func gradleEntries(f found) []entry.Entry {
	cmd := "gradle"
	if wrapper, ok := findGradleWrapper(f); ok {
		cmd = wrapper
	}
	if rel := f.relDir(); rel != "" {
		cmd += " -p " + rel
	}
	tasks := []string{"build", "test", "clean", "assemble", "check", "tasks"}
	entries := make([]entry.Entry, 0, len(tasks))
	for _, tk := range tasks {
		tmpl := cmd + " " + tk
		entries = append(entries, entry.Entry{
			Title:    tmpl,
			Tags:     []string{"gradle", "java", "project"},
			Template: tmpl,
			Kind:     entry.CommandLiteral,
			Source:   entry.SourceProject,
		})
	}
	return entries
}

// wrapperDir returns the directory holding the nearest gradlew at or above
// f.dir (bounded by f.stop), or "" when there is none.
func wrapperDir(f found) string {
	d := f.dir
	for {
		if fileExists(filepath.Join(d, "gradlew")) {
			return d
		}
		if f.stop == "" || samePath(d, f.stop) || !within(d, f.stop) {
			return ""
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// findGradleWrapper returns the command that invokes the nearest wrapper from
// the working directory: "./gradlew" when it sits in the working directory,
// otherwise its relative (or absolute) path.
func findGradleWrapper(f found) (string, bool) {
	wd := wrapperDir(f)
	if wd == "" {
		return "", false
	}
	wf := found{dir: wd, cwd: f.cwd, stop: f.stop}
	if wf.dirRef() == "" {
		return "./gradlew", true
	}
	ref := wf.relPath("gradlew")
	if !strings.Contains(ref, "/") {
		ref = "./" + ref
	}
	return ref, true
}

// justRecipeRE matches a recipe name at the start of a line in a justfile.
var justRecipeRE = regexp.MustCompile(`^([a-zA-Z0-9][a-zA-Z0-9._-]*)(?:\s+[^:]*)?:`)
var justAliasRE = regexp.MustCompile(`^alias\s+([a-zA-Z0-9][a-zA-Z0-9._-]*)\s*:=\s*([a-zA-Z0-9][a-zA-Z0-9._-]*)`)

// parseJustfile extracts recipe names from a justfile in the working
// directory.
func parseJustfile(path string) []entry.Entry { return justEntries(localFound(path)) }

// justEntries extracts recipe names from a justfile. A justfile in a parent
// directory is addressed with --justfile (just then runs recipes from the
// justfile's directory, as it would when found by its own upward search).
func justEntries(f found) []entry.Entry {
	fh, err := os.Open(f.path)
	if err != nil {
		return nil
	}
	defer fh.Close()

	var names []string
	seen := make(map[string]bool)
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "#") {
			continue
		}
		if m := justAliasRE.FindStringSubmatch(line); m != nil {
			name := m[1]
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
			continue
		}
		line = strings.TrimPrefix(line, "@")
		m := justRecipeRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		// Skip assignments like `foo := bar`.
		if strings.Contains(line, ":=") {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}

	prefix := "just"
	if f.relDir() != "" {
		prefix += " --justfile " + f.relPath(filepath.Base(f.path))
	}
	entries := make([]entry.Entry, 0, len(names))
	for _, n := range names {
		cmd := prefix + " " + n
		entries = append(entries, entry.Entry{
			Title:    cmd,
			Tags:     []string{"just", "project"},
			Template: cmd,
			Kind:     entry.CommandLiteral,
			Source:   entry.SourceProject,
		})
	}
	return entries
}

// composeKeyRE matches a YAML mapping key with indentation. It is intentionally
// conservative but handles common compose files with two or four spaces.
var composeKeyRE = regexp.MustCompile(`^(\s*)([a-zA-Z0-9][a-zA-Z0-9._-]*):(?:\s|$)`)

// composeServicesRE matches the top-level `services:` key, allowing a
// trailing comment.
var composeServicesRE = regexp.MustCompile(`^\s*services:\s*(?:#.*)?$`)

// parseCompose extracts services from a compose file in the working directory.
func parseCompose(path string) []entry.Entry { return composeEntries(localFound(path)) }

// composeEntries extracts service names from a compose file and offers up and
// logs actions for each, plus whole-stack actions. A compose file in a parent
// directory is addressed with -f.
func composeEntries(f found) []entry.Entry {
	fh, err := os.Open(f.path)
	if err != nil {
		return nil
	}
	defer fh.Close()

	var services []string
	inServices := false
	servicesIndent := -1
	serviceIndent := -1
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		trimmed := strings.TrimRight(line, " \t")
		if composeServicesRE.MatchString(trimmed) {
			inServices = true
			servicesIndent = len(line) - len(strings.TrimLeft(line, " "))
			serviceIndent = -1
			continue
		}
		if inServices {
			if strings.TrimSpace(trimmed) == "" || strings.HasPrefix(strings.TrimSpace(trimmed), "#") {
				continue
			}
			indent := len(line) - len(strings.TrimLeft(line, " "))
			if indent <= servicesIndent {
				inServices = false
				continue
			}
			if m := composeKeyRE.FindStringSubmatch(line); m != nil {
				keyIndent := len(m[1])
				if serviceIndent < 0 {
					serviceIndent = keyIndent
				}
				if keyIndent == serviceIndent {
					services = append(services, m[2])
				}
			}
		}
	}

	base := "docker compose"
	if f.relDir() != "" {
		base += " -f " + f.relPath(filepath.Base(f.path))
	}
	mk := func(cmd string, tags ...string) entry.Entry {
		return entry.Entry{Title: cmd, Tags: tags, Template: cmd, Kind: entry.CommandLiteral, Source: entry.SourceProject}
	}
	entries := []entry.Entry{
		mk(base+" up -d", "docker", "compose", "project"),
		mk(base+" down", "docker", "compose", "project"),
	}
	for _, svc := range services {
		entries = append(entries,
			mk(base+" logs -f "+svc, "docker", "compose", "logs", "project"),
			mk(base+" restart "+svc, "docker", "compose", "project"),
		)
	}
	return entries
}
