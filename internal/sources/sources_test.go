package sources

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/liliumapps/commando/cheats"
	"github.com/liliumapps/commando/internal/config"
)

func testPaths(t *testing.T) config.Paths {
	t.Helper()
	root := t.TempDir()
	return config.Paths{
		ConfigDir: filepath.Join(root, "cfg"),
		CheatsDir: filepath.Join(root, "cfg", "cheats"),
		DataDir:   filepath.Join(root, "data"),
	}
}

func TestSourcesRoundTrip(t *testing.T) {
	p := testPaths(t)
	want := []Source{
		{URL: "https://github.com/acme/team-cheats"},
		{URL: "git@github.com:liliumapps/commando.git", File: "cheats/docker.toml", Commit: strings.Repeat("a", 40), Updated: "2026-11-20T10:00:00Z"},
	}
	if err := Write(p, want); err != nil {
		t.Fatal(err)
	}
	got, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip:\n got %+v\nwant %+v", got, want)
	}
	data, _ := os.ReadFile(SourcesPath(p))
	if !strings.Contains(string(data), "[[source]]\nurl = \"https://github.com/acme/team-cheats\"") {
		t.Errorf("unexpected layout:\n%s", data)
	}

	// Hand-edited: the minimal form from the docs loads.
	if err := os.WriteFile(SourcesPath(p), []byte("[[source]]\nurl = \"https://github.com/acme/team-cheats\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := Read(p); err != nil || len(got) != 1 {
		t.Fatalf("minimal file: %v %+v", err, got)
	}
	for name, body := range map[string]string{
		"unknown field": "[[source]]\nurl = \"https://h/x\"\nref = \"main\"\n",
		"duplicate":     "[[source]]\nurl = \"https://h/x\"\n[[source]]\nurl = \"https://h/x\"\n",
		"bad url":       "[[source]]\nurl = \"/tmp/repo\"\n",
		"bad file":      "[[source]]\nurl = \"https://h/x\"\nfile = \"../x.toml\"\n",
		"bad commit":    "[[source]]\nurl = \"https://h/x\"\ncommit = \"main\"\n",
	} {
		if err := os.WriteFile(SourcesPath(p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Read(p); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestValidateURL(t *testing.T) {
	good := []string{
		"https://github.com/acme/team-cheats",
		"https://github.com/acme/team-cheats.git",
		"https://user@git.example.com:8443/team/cheats",
		"ssh://git@github.com/acme/cheats.git",
		"ssh://git@example.com:2222/acme/cheats",
		"git@github.com:acme/cheats.git",
	}
	bad := []string{
		"", " https://github.com/a/b", "http://github.com/a/b",
		"file:///tmp/repo", "FILE:///tmp/repo", "/tmp/repo", "./repo", "../repo", `C:\repo`,
		"ext::sh -c touch% /tmp/pwned", "fd::17", "git://github.com/a/b",
		"https://github.com", "https://github.com/", "https://user:token@github.com/a/b",
		"ssh://-oProxyCommand=evil/x", "ssh://-oProxyCommand=x@host/x", "-uhttps://x/y",
		"github.com:acme/cheats", "git@-host:acme/cheats", "https://github.com/a/b c",
		"https://github.com/a/b?x=1",
	}
	for _, u := range good {
		if err := ValidateURL(u); err != nil {
			t.Errorf("ValidateURL(%q) = %v, want ok", u, err)
		}
	}
	for _, u := range bad {
		if err := ValidateURL(u); err == nil {
			t.Errorf("ValidateURL(%q) accepted", u)
		}
	}
}

func TestValidateFile(t *testing.T) {
	for _, f := range []string{"docker.toml", "cheats/docker.toml", "a/b/c.TOML", ".hidden/x.toml"} {
		if err := ValidateFile(f); err != nil {
			t.Errorf("ValidateFile(%q) = %v", f, err)
		}
	}
	for _, f := range []string{"", "/etc/x.toml", "../x.toml", "a/../../x.toml", "a/./x.toml", "a//x.toml",
		"x.yaml", "cheats", `a\x.toml`, "C:x.toml", ".git/config.toml", "a/.GIT/x.toml"} {
		if err := ValidateFile(f); err == nil {
			t.Errorf("ValidateFile(%q) accepted", f)
		}
	}
}

func TestCloneDirAndLabel(t *testing.T) {
	p := testPaths(t)
	a := CloneDir(p, "https://github.com/acme/team-cheats")
	b := CloneDir(p, "git@github.com:acme/team-cheats")
	if a == b || filepath.Dir(a) != ReposDir(p) || !strings.HasPrefix(filepath.Base(a), "github.com-acme-team-cheats-") {
		t.Errorf("clone dirs %q %q", a, b)
	}
	if l := (Source{URL: "https://github.com/acme/team-cheats.git"}).Label(); l != "team-cheats" {
		t.Errorf("label %q", l)
	}
	if l := (Source{URL: "git@github.com:liliumapps/commando", File: "cheats/docker.toml"}).Label(); l != "commando/docker.toml" {
		t.Errorf("label %q", l)
	}
}

// fakeClone lays out files as a clone at commit without running git.
func fakeClone(t *testing.T, p config.Paths, remote, commit string, files map[string]string) string {
	t.Helper()
	dir := CloneDir(p, remote)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte(commit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadPrefixesOverridesAndDuplicates(t *testing.T) {
	p := testPaths(t)
	const remote = "https://example.com/acme/team-cheats"
	commit := strings.Repeat("b", 40)
	fakeClone(t, p, remote, commit, map[string]string{
		"a.toml":           "[[cmd]]\nid = \"shared\"\ntitle = \"From source\"\ntmpl = \"echo src\"\n\n[[cmd]]\nid = \"dup\"\ntitle = \"A dup\"\ntmpl = \"echo a\"\n\n[[cmd]]\ntitle = \"No id\"\ntmpl = \"echo n\"\n",
		"z/b.toml":         "[[cmd]]\nid = \"dup\"\ntitle = \"B dup\"\ntmpl = \"echo b\"\n",
		"c.toml":           "[[cmd]]\ntitle = \"C\"\ntmpl = \"echo c\"\n",
		"broken.toml":      "[[cmd",
		".github/x.toml":   "[[cmd]]\ntitle = \"hidden\"\ntmpl = \"echo h\"\n",
		"notes/readme.txt": "not a cheat",
	})
	if err := os.MkdirAll(p.CheatsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.CheatsDir, "mine.toml"), []byte("[[cmd]]\nid = \"shared\"\ntitle = \"Mine\"\ntmpl = \"echo mine\"\n\n[[cmd]]\ntitle = \"Plain\"\ntmpl = \"echo plain\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(p, []Source{{URL: remote, Commit: commit}}); err != nil {
		t.Fatal(err)
	}
	userOnly, err := config.LoadCheats(p)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := LoadCheats(p)
	if err == nil || !strings.Contains(err.Error(), `id "dup" is already used in a.toml`) || !strings.Contains(err.Error(), "broken.toml") {
		t.Fatalf("want duplicate-id and parse diagnostics, got %v", err)
	}
	byTitle := map[string]string{}
	for _, e := range entries {
		byTitle[e.TitleText()] = e.ID()
		if e.TitleText() == "No id" && e.Origin != "team-cheats" {
			t.Errorf("origin = %q", e.Origin)
		}
	}
	// The user's cheats keep their identities exactly.
	for _, e := range userOnly {
		if byTitle[e.TitleText()] != e.ID() {
			t.Errorf("user cheat %q changed identity: %q vs %q", e.TitleText(), byTitle[e.TitleText()], e.ID())
		}
	}
	if byTitle["Mine"] != "cheat:shared" {
		t.Errorf("user id = %q", byTitle["Mine"])
	}
	if _, ok := byTitle["From source"]; ok {
		t.Error("a user cheat with the same id should override the source cheat")
	}
	if byTitle["A dup"] != "cheat:src:"+remote+"/dup" {
		t.Errorf("prefixed id = %q", byTitle["A dup"])
	}
	if _, ok := byTitle["B dup"]; ok {
		t.Error("a file with a duplicate id should be skipped")
	}
	if id := byTitle["No id"]; !strings.HasPrefix(id, "cheat:src:"+remote+"/") || len(id) <= len("cheat:src:"+remote+"/") {
		t.Errorf("hashed id = %q", id)
	}
	if _, ok := byTitle["C"]; !ok {
		t.Error("valid files must still load")
	}
	if _, ok := byTitle["hidden"]; ok {
		t.Error("dot directories must be skipped")
	}

	// A single-file source gets its own identity namespace.
	if err := Write(p, []Source{{URL: remote, File: "c.toml", Commit: commit}}); err != nil {
		t.Fatal(err)
	}
	entries, err = LoadCheats(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.TitleText() == "C" && (!strings.HasPrefix(e.ID(), "cheat:src:"+remote+"#c.toml/") || e.Origin != "team-cheats/c.toml") {
			t.Errorf("single-file id %q origin %q", e.ID(), e.Origin)
		}
	}
}

func TestLoadRefusesUnreviewedCommit(t *testing.T) {
	p := testPaths(t)
	const remote = "https://example.com/acme/cheats"
	fakeClone(t, p, remote, strings.Repeat("c", 40), map[string]string{"a.toml": "[[cmd]]\ntitle = \"A\"\ntmpl = \"echo a\"\n"})
	for _, s := range []Source{
		{URL: remote, Commit: strings.Repeat("d", 40)},
		{URL: remote},
		{URL: "https://example.com/other/missing", Commit: strings.Repeat("c", 40)},
	} {
		if err := Write(p, []Source{s}); err != nil {
			t.Fatal(err)
		}
		entries, err := LoadCheats(p)
		if err == nil || len(entries) != 0 {
			t.Errorf("%+v: want a diagnostic and no entries, got %d, %v", s, len(entries), err)
		}
		if n, _, problems := Status(p); n != 1 || len(problems) != 1 {
			t.Errorf("Status = %d, %v", n, problems)
		}
	}
}

func TestLoadRejectsSymlinks(t *testing.T) {
	p := testPaths(t)
	const remote = "https://example.com/acme/links"
	commit := strings.Repeat("e", 40)
	dir := fakeClone(t, p, remote, commit, map[string]string{"ok.toml": "[[cmd]]\ntitle = \"OK\"\ntmpl = \"echo ok\"\n"})
	outside := t.TempDir()
	evil := "[[cmd]]\ntitle = \"Evil\"\ntmpl = \"echo evil\"\n"
	if err := os.WriteFile(filepath.Join(outside, "evil.toml"), []byte(evil), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "evil.toml"), filepath.Join(dir, "link.toml")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks unavailable:", err)
		}
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "linkdir")); err != nil {
		t.Fatal(err)
	}
	for _, s := range []Source{{URL: remote, Commit: commit}, {URL: remote, File: "link.toml", Commit: commit}, {URL: remote, File: "linkdir/evil.toml", Commit: commit}} {
		if err := Write(p, []Source{s}); err != nil {
			t.Fatal(err)
		}
		entries, err := LoadCheats(p)
		for _, e := range entries {
			if e.TitleText() == "Evil" {
				t.Errorf("%+v: loaded a cheat through a symlink", s)
			}
		}
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Errorf("%+v: want a symlink diagnostic, got %v", s, err)
		}
	}
}

// TestRepoPacksLoadAsSources checks this repository works as a source: every
// pack in cheats/ loads as a single-file source.
func TestRepoPacksLoadAsSources(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	names, err := config.SampleFiles()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cheats.FS.Open("docker.toml"); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		s := Source{URL: "https://github.com/liliumapps/commando", File: "cheats/" + name}
		got, errs := loadScope(root, s)
		if len(errs) > 0 || len(got) == 0 {
			t.Errorf("%s: %d cheats, %v", name, len(got), errs)
		}
	}
}

// --- git-backed tests against a local bare repository ---

type remoteRepo struct {
	t    *testing.T
	work string
	url  string
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newRemote(t *testing.T, files map[string]string) *remoteRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	allowFileURLs = true
	t.Cleanup(func() { allowFileURLs = false })

	root := t.TempDir()
	work := filepath.Join(root, "work")
	bare := filepath.Join(root, "remote.git")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, work, "init", "-q")
	r := &remoteRepo{t: t, work: work}
	r.commit(files)
	runGit(t, root, "clone", "-q", "--bare", work, bare)
	runGit(t, work, "remote", "add", "origin", bare)
	slashed := filepath.ToSlash(bare)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	r.url = "file://" + slashed
	return r
}

func (r *remoteRepo) commit(files map[string]string) {
	r.t.Helper()
	for name, body := range files {
		full := filepath.Join(r.work, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	runGit(r.t, r.work, "add", "-A")
	runGit(r.t, r.work, "commit", "-q", "-m", "update")
}

func (r *remoteRepo) push(files map[string]string) string {
	r.commit(files)
	runGit(r.t, r.work, "push", "-q", "origin", "HEAD:main")
	return runGit(r.t, r.work, "rev-parse", "HEAD")
}

func manager(p config.Paths, confirm func() bool) (*Manager, *bytes.Buffer) {
	var out bytes.Buffer
	return &Manager{Paths: p, Out: &out, Confirm: confirm}, &out
}

func yes() bool { return true }

const dockerCheat = "[[cmd]]\ntitle = \"Logs\"\ntmpl = \"docker logs <c>\"\n\n  [[cmd.var]]\n  name = \"c\"\n  src  = \"docker ps --format '{{.Names}}'\"\n"

func TestAddUpdateRemove(t *testing.T) {
	r := newRemote(t, map[string]string{
		"docker.toml":    dockerCheat,
		"git/git.toml":   "[[cmd]]\ntitle = \"Status\"\ntmpl = \"git status\"\n",
		".hidden/x.toml": "[[cmd]]\ntitle = \"Hidden\"\ntmpl = \"echo h\"\n",
	})
	p := testPaths(t)

	m, out := manager(p, yes)
	if err := m.Add(r.url, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "docker ps --format '{{.Names}}'") || !strings.Contains(out.String(), "Logs") {
		t.Errorf("review did not list the src command:\n%s", out)
	}
	srcs, _ := Read(p)
	if len(srcs) != 1 || !commitRE.MatchString(srcs[0].Commit) || srcs[0].Updated == "" {
		t.Fatalf("sources after add: %+v", srcs)
	}
	first := srcs[0].Commit
	entries, err := LoadCheats(p)
	if err != nil || len(entries) != 2 {
		t.Fatalf("load: %d, %v", len(entries), err)
	}
	if err := m.Add(r.url, ""); err == nil {
		t.Error("adding the same source twice should fail")
	}

	// A second source for the same repository shares the clone.
	if err := m.Add(r.url, "git/git.toml"); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(r.url, "missing.toml"); err == nil {
		t.Error("a missing file should fail")
	}
	srcs, _ = Read(p)
	if len(srcs) != 2 || srcs[1].Commit != first {
		t.Fatalf("sources: %+v", srcs)
	}

	// A change that leaves the src commands alone updates without review.
	second := r.push(map[string]string{"git/git.toml": "[[cmd]]\ntitle = \"Short status\"\ntmpl = \"git status -s\"\n"})
	m.Confirm = func() bool { t.Error("unchanged commands should not be reviewed"); return false }
	if err := m.Update(); err != nil {
		t.Fatal(err)
	}
	srcs, _ = Read(p)
	if srcs[0].Commit != second || srcs[1].Commit != second {
		t.Fatalf("update did not advance: %+v", srcs)
	}

	// A changed src is reviewed; declining keeps the reviewed commit.
	r.push(map[string]string{"docker.toml": strings.Replace(dockerCheat, "docker ps", "docker ps -a", 1)})
	asked := false
	out.Reset()
	m.Confirm = func() bool { asked = true; return false }
	if err := m.Update(); !IsDeclined(err) {
		t.Fatalf("want a declined error, got %v", err)
	}
	if !asked || !strings.Contains(out.String(), "* src      docker ps -a") {
		t.Errorf("changed src not marked in review:\n%s", out)
	}
	srcs, _ = Read(p)
	if srcs[0].Commit != second {
		t.Fatalf("declined update moved the commit: %+v", srcs)
	}
	if head, _ := gitHead(CloneDir(p, r.url)); head != second {
		t.Errorf("declined update left the clone at %s", head)
	}
	if _, err := LoadCheats(p); err != nil {
		t.Errorf("declined update broke loading: %v", err)
	}

	m.Confirm = yes
	if err := m.Update(); err != nil {
		t.Fatal(err)
	}
	srcs, _ = Read(p)
	if srcs[0].Commit == second {
		t.Fatal("accepted update did not advance")
	}

	// A manual checkout of another commit is refused until reviewed.
	dir := CloneDir(p, r.url)
	if err := gitReset(dir, second); err != nil {
		t.Fatal(err)
	}
	if entries, err := LoadCheats(p); err == nil || len(entries) != 0 {
		t.Errorf("mismatched commit loaded: %d, %v", len(entries), err)
	}
	if err := m.Update(); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCheats(p); err != nil {
		t.Errorf("update did not restore the clone: %v", err)
	}

	out.Reset()
	if err := m.List(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "git/git.toml") || !strings.Contains(out.String(), "all") {
		t.Errorf("list:\n%s", out)
	}

	if err := m.Remove(r.url, "git/git.toml"); err != nil {
		t.Fatal(err)
	}
	if !isClone(dir) {
		t.Error("clone removed while another source uses it")
	}
	if err := m.Remove(r.url, "git/git.toml"); err == nil {
		t.Error("removing a missing source should fail")
	}
	if err := m.Remove(r.url, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("clone not removed: %v", err)
	}
}

func TestAddDeclinedLeavesNothing(t *testing.T) {
	r := newRemote(t, map[string]string{"docker.toml": dockerCheat})
	p := testPaths(t)
	m, _ := manager(p, func() bool { return false })
	if err := m.Add(r.url, "docker.toml"); !IsDeclined(err) {
		t.Fatalf("want declined, got %v", err)
	}
	if _, err := os.Stat(SourcesPath(p)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("sources.toml written: %v", err)
	}
	left, _ := os.ReadDir(ReposDir(p))
	if len(left) != 0 {
		t.Errorf("repos dir not empty: %v", left)
	}
}

func TestUpdateClonesHandAddedSource(t *testing.T) {
	r := newRemote(t, map[string]string{"docker.toml": dockerCheat})
	p := testPaths(t)
	if err := Write(p, []Source{{URL: r.url}}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCheats(p); err == nil {
		t.Error("an unreviewed source should not load")
	}
	asked := false
	m, _ := manager(p, func() bool { asked = true; return true })
	if err := m.Update(); err != nil {
		t.Fatal(err)
	}
	if !asked {
		t.Error("a never-reviewed source must be reviewed")
	}
	entries, err := LoadCheats(p)
	if err != nil || len(entries) != 1 {
		t.Fatalf("load: %d, %v", len(entries), err)
	}
}

func TestFileURLsRejectedOutsideTests(t *testing.T) {
	if allowFileURLs {
		t.Fatal("test hook leaked")
	}
	p := testPaths(t)
	m, _ := manager(p, yes)
	if err := m.Add("file:///tmp/repo", ""); err == nil {
		t.Error("file:// accepted")
	}
}

// TestGitBlocksFileTransport checks the git safety flags on their own: with
// the test hook off, git refuses a file:// remote even if validation is
// bypassed.
func TestGitBlocksFileTransport(t *testing.T) {
	r := newRemote(t, map[string]string{"docker.toml": dockerCheat})
	allowFileURLs = false
	if _, err := gitClone(r.url, filepath.Join(t.TempDir(), "c")); err == nil {
		t.Error("git cloned a file:// remote without the test hook")
	}
}
