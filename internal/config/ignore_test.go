package config

import (
	"path/filepath"
	"testing"
)

func TestShouldIgnore(t *testing.T) {
	patterns := Default().HistoryIgnore
	cases := []struct {
		cmd    string
		ignore bool
	}{
		{"git status", false},
		{"npm run build", false},
		{"export SECRET_KEY=abc", true},
		{"mysql --password=hunter2", true},
		{"echo $API_TOKEN", true},
		{"aws configure set apikey xyz", true},
		{"ls -la", false},
		{"", true}, // empty is always ignored
	}
	for _, tc := range cases {
		if got := ShouldIgnore(patterns, tc.cmd); got != tc.ignore {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", tc.cmd, got, tc.ignore)
		}
	}
}

func TestShouldIgnoreSubstringPattern(t *testing.T) {
	// A pattern without wildcards is a substring match.
	if !ShouldIgnore([]string{"password"}, "mysql -p password123") {
		t.Error("substring pattern should match")
	}
	if ShouldIgnore([]string{"password"}, "git status") {
		t.Error("unrelated command should not match")
	}
}

func TestShouldIgnoreGlobCrossesSlash(t *testing.T) {
	patterns := Default().HistoryIgnore
	for _, cmd := range []string{
		"mysql --password=x/y",
		"curl https://user:secret@host/x",
		"export KEY=abc/def",
		"export API_KEY=a/b/c",
	} {
		if !ShouldIgnore(patterns, cmd) {
			t.Errorf("ShouldIgnore(%q) = false, want true", cmd)
		}
	}
}

func TestShouldIgnoreExtendedDefaults(t *testing.T) {
	patterns := Default().HistoryIgnore
	leak := []string{
		`curl -H "Authorization: Basic Zm9vOmJhcg==" https://api`,
		`curl -H 'X: y' --oauth2-bearer abcdefghijkl https://api`,
		"mysql -u root -phunter2 db",
		"AWS_PROFILE=prod aws s3 ls",
		"GITLAB_TOKEN=abc glab mr list",
		"CLIENT_SECRET=abc ./run",
		"MY_PAT=abc ./run",
		"DB_PASSWORD=abc ./run",
		"git clone https://bob:pw@example.com/r.git",
		"echo AKIA" + "ABCDEFGHIJKLMNOP",
		"gh auth login --with-token ghp" + "_abcdefghijklmnopqrstuvwxyz0123",
		"echo github_pat_11ABCDEFG0123456789_abcdefghijklmnop",
		"curl -d key=sk" + "-abcdefghijklmnopqrstuvwx https://x",
		"curl -d t=xox" + "b-1234567890-abcdef https://slack",
		"curl -d eyJ" + "hbGciOiJIUzI1NiJ9.eyJ" + "zdWIiOiIxMjM0NTYifQ.abcdefghijk https://x",
	}
	for _, cmd := range leak {
		if !ShouldIgnore(patterns, cmd) {
			t.Errorf("ShouldIgnore(%q) = false, want true", cmd)
		}
	}
	safe := []string{
		"mysql -P 3306 -h db",
		"mysql -p db",
		"git clone https://example.com/r.git",
		"curl http://host:8080/path",
		"make build",
		"ls ~/sketches",
	}
	for _, cmd := range safe {
		if ShouldIgnore(patterns, cmd) {
			t.Errorf("ShouldIgnore(%q) = true, want false", cmd)
		}
	}
}

func TestLooksSecretAppliesWithUserPatterns(t *testing.T) {
	// Value-shaped detectors apply even when the user replaced the glob list.
	if !ShouldIgnore([]string{"nothing"}, "echo AKIA"+"ABCDEFGHIJKLMNOP") {
		t.Error("AWS key id should always be redacted")
	}
}

func TestCompileGlobClasses(t *testing.T) {
	if !ShouldIgnore([]string{"foo[0-9]*"}, "foo1/bar") {
		t.Error("class + star should match across slash")
	}
	if ShouldIgnore([]string{"foo[!0-9]"}, "foo1") {
		t.Error("negated class should not match")
	}
	if ShouldIgnore([]string{"[unterminated"}, "x") {
		t.Error("malformed pattern should not match")
	}
}

func TestDefaultPathsCacheDirHonorsCommandoHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("COMMANDO_HOME", home)
	p, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "cache", "commando"); filepath.ToSlash(p.CacheDir) != filepath.ToSlash(want) {
		t.Errorf("CacheDir = %q, want %q", p.CacheDir, want)
	}
}
