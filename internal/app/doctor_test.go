package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorReportsSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("COMMANDO_HOME", home)
	find := func() Check {
		for _, c := range Doctor() {
			if c.Name == "sources" {
				return c
			}
		}
		t.Fatal("no sources check")
		return Check{}
	}
	if c := find(); c.Status != CheckOK || !strings.Contains(c.Detail, "none") {
		t.Errorf("no sources: %+v", c)
	}
	body := "[[source]]\nurl = \"https://example.com/acme/team-cheats\"\n"
	if err := os.WriteFile(filepath.Join(home, "commando", "sources.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if c := find(); c.Status != CheckWarning || !strings.Contains(c.Detail, "1 source(s)") || !strings.Contains(c.Detail, "not reviewed") {
		t.Errorf("unreviewed source: %+v", c)
	}
}
