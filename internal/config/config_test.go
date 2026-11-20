package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/template"
)

func TestLoadDefaultWhenMissing(t *testing.T) {
	p := Paths{ConfigDir: t.TempDir()}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KeyBinding != "^G" {
		t.Errorf("default key binding = %q, want ^G", cfg.KeyBinding)
	}
	if len(cfg.HistoryIgnore) == 0 {
		t.Error("expected default history_ignore patterns")
	}
	if cfg.History.EffectiveListLimit() != 10 {
		t.Errorf("default history list limit = %d, want 10", cfg.History.EffectiveListLimit())
	}
	if cfg.History.EffectiveMaxLoad() != 5000 {
		t.Errorf("default history max load = %d, want 5000", cfg.History.EffectiveMaxLoad())
	}
	if cfg.History.EffectiveMaxBytes() != 8<<20 {
		t.Errorf("default history max bytes = %d, want 8 MiB", cfg.History.EffectiveMaxBytes())
	}
}

func TestHistoryConfigMergeAndSemantics(t *testing.T) {
	dir := t.TempDir()
	content := `
[history]
list_limit = 25
max_load = 2000
max_bytes = 1048576
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(Paths{ConfigDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.History.EffectiveListLimit() != 25 {
		t.Errorf("list limit = %d, want 25", cfg.History.EffectiveListLimit())
	}
	if cfg.History.EffectiveMaxLoad() != 2000 {
		t.Errorf("max load = %d, want 2000", cfg.History.EffectiveMaxLoad())
	}
	if cfg.History.EffectiveMaxBytes() != 1048576 {
		t.Errorf("max bytes = %d, want 1048576", cfg.History.EffectiveMaxBytes())
	}
}

func TestHistoryNegativeMeansUnlimited(t *testing.T) {
	h := HistoryConfig{ListLimit: -1, MaxLoad: -1, MaxBytes: -1}
	if h.EffectiveListLimit() != 0 {
		t.Errorf("negative list limit should be unlimited (0), got %d", h.EffectiveListLimit())
	}
	if h.EffectiveMaxLoad() != 0 {
		t.Errorf("negative max load should be unlimited (0), got %d", h.EffectiveMaxLoad())
	}
	if h.EffectiveMaxBytes() != 0 {
		t.Errorf("negative max bytes should be unlimited (0), got %d", h.EffectiveMaxBytes())
	}
	// A partial override keeps the bounded defaults.
	def := Default()
	if def.History.EffectiveListLimit() != 10 || def.History.EffectiveMaxLoad() != 5000 {
		t.Errorf("defaults wrong: list=%d load=%d", def.History.EffectiveListLimit(), def.History.EffectiveMaxLoad())
	}
}

func TestLoadUserConfigMerges(t *testing.T) {
	dir := t.TempDir()
	content := `
key_binding = "^T"

[[provider]]
name = "make"
detect = "Makefile"
list = "grep -E '^[a-z]+:' Makefile | cut -d: -f1"
template = "make {}"
cache = "5m"
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(Paths{ConfigDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KeyBinding != "^T" {
		t.Errorf("key binding = %q, want ^T", cfg.KeyBinding)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].Name != "make" {
		t.Fatalf("providers = %+v", cfg.Providers)
	}
}

func TestLoadRejectsInvalidKeyBinding(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "config.toml"),
		[]byte(`key_binding = "ctrl-g"`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(Paths{ConfigDir: dir}); err == nil || !strings.Contains(err.Error(), "key_binding") {
		t.Fatalf("error = %v, want key_binding validation", err)
	}
}

func TestLoadInvalidProvider(t *testing.T) {
	dir := t.TempDir()
	content := `
[[provider]]
name = "broken"
detect = "Makefile"
`
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o644)
	if _, err := Load(Paths{ConfigDir: dir}); err == nil {
		t.Error("expected validation error for provider missing list/template")
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	cases := []string{
		`key_bindng = "^T"`,
		"[history]\nmax_lod = 10",
		`[[provider]]
name = "task"
detect = "Taskfile.yml"
list = "task --list"
template = "task {}"
typo = true`,
	}
	for i, content := range cases {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(Paths{ConfigDir: dir}); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Errorf("case %d: error = %v, want unknown field", i, err)
		}
	}
}

func TestExampleConfigMatchesStrictSchema(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "config.example.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := decodeStrict("config.example.toml", data, &cfg); err != nil {
		t.Fatal(err)
	}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadValidatesProviderDefinitions(t *testing.T) {
	cases := []struct {
		name      string
		providers string
	}{
		{"blank name", `[[provider]]
name = " "
detect = "Taskfile.yml"
list = "task --list"
template = "task {}"`},
		{"duplicate name", `[[provider]]
name = "task"
detect = "Taskfile.yml"
list = "task --list"
template = "task {}"
[[provider]]
name = "TASK"
detect = "tasks.toml"
list = "task --list"
template = "task {}"`},
		{"escaping marker", `[[provider]]
name = "task"
detect = "../Taskfile.yml"
list = "task --list"
template = "task {}"`},
		{"missing placeholder", `[[provider]]
name = "task"
detect = "Taskfile.yml"
list = "task --list"
template = "task"`},
		{"invalid cache", `[[provider]]
name = "task"
detect = "Taskfile.yml"
list = "task --list"
template = "task {}"
cache = "later"`},
		{"negative cache", `[[provider]]
name = "task"
detect = "Taskfile.yml"
list = "task --list"
template = "task {}"
cache = "-1m"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(tc.providers), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(Paths{ConfigDir: dir}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoadCheatDataValidation(t *testing.T) {
	cases := []struct {
		name    string
		toml    string
		wantErr bool
	}{
		{"missing desc", `[[cmd]]` + "\n" + `tmpl = "ls"`, true},
		{"missing tmpl", `[[cmd]]` + "\n" + `desc = "x"`, true},
		{"bad scope", `[[cmd]]
desc = "x"
tmpl = "ls <a>"
[[cmd.var]]
name = "a"
remember = "planet"`, true},
		{"valid", `[[cmd]]
desc = "list"
tmpl = "ls [-l] <path>"
[[cmd.var]]
name = "path"
default = "."`, false},
		{"bad delimiter regex", `[[cmd]]
desc = "x"
tmpl = "cmd <a>"
[[cmd.var]]
name = "a"
delimiter = "["
columns = ["c1"]`, true},
		{"negative header_lines", `[[cmd]]
desc = "x"
tmpl = "cmd <a>"
[[cmd.var]]
name = "a"
header_lines = -1`, true},
		{"match and columns mutually exclusive", `[[cmd]]
desc = "x"
tmpl = "cmd <a>"
[[cmd.var]]
name = "a"
match = '(?P<x>\d+)'
columns = ["y"]`, true},
		{"bad match regex", `[[cmd]]
desc = "x"
tmpl = "cmd <a>"
[[cmd.var]]
name = "a"
match = '(?P<x>'`, true},
		{"label references unknown name", `[[cmd]]
desc = "x"
tmpl = "cmd <a>"
[[cmd.var]]
name = "a"
columns = ["pid", "comm"]
label = "<nope> (<pid>)"`, true},
		{"value references unknown group", `[[cmd]]
desc = "x"
tmpl = "cmd <a>"
[[cmd.var]]
name = "a"
match = '^(?P<pid>\d+)'
value = "<comm>"`, true},
		{"bad pattern regex", `[[cmd]]
desc = "x"
tmpl = "cmd <a>"
[[cmd.var]]
name = "a"
pattern = "("`, true},
		{"bad cache duration", `[[cmd]]
desc = "x"
tmpl = "cmd <a>"
[[cmd.var]]
name = "a"
cache = "soon"`, true},
		{"negative cache duration", `[[cmd]]
desc = "x"
tmpl = "cmd <a>"
[[cmd.var]]
name = "a"
cache = "-1m"`, true},
		{"invalid command template", `[[cmd]]
desc = "x"
tmpl = "cmd <a"`, true},
		{"unknown command field", `[[cmd]]
desc = "x"
tmpl = "cmd"
descc = "typo"`, true},
		{"unknown variable field", `[[cmd]]
desc = "x"
tmpl = "cmd <a>"
[[cmd.var]]
name = "a"
strct = true`, true},
		{"valid extraction — composed label from columns", `[[cmd]]
desc = "kill"
tmpl = "kill -9 <pid>"
[[cmd.var]]
name = "pid"
src = "ps -eo pid,comm"
header_lines = 1
columns = ["pid", "comm"]
label = "<comm> (<pid>)"
value = "<pid>"
pattern = '^[0-9]+$'
remember = "none"`, false},
		{"valid extraction — match with named groups", `[[cmd]]
desc = "rmi"
tmpl = "docker rmi <image>"
[[cmd.var]]
name = "image"
src = "docker images"
header_lines = 1
match = '^(?P<repo>\S+)\s+(?P<tag>\S+)\s+(?P<id>\S+)'
label = "<repo>:<tag>"
value = "<id>"
cache = "5m"
strict = true
preview = "docker inspect {}"`, false},
		{"valid extraction — positional refs", `[[cmd]]
desc = "user"
tmpl = "id <u>"
[[cmd.var]]
name = "u"
src = "getent passwd"
delimiter = ":"
columns = ["name"]
value = "<1>"`, false},
		{"valid inline values", `[[cmd]]
desc = "env"
tmpl = "deploy <env>"
[[cmd.var]]
name = "env"
values = ["dev", "staging", "prod"]
strict = true`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadCheatData(tc.name, []byte(tc.toml))
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestEmbeddedSamplesAreValid is the guard that keeps shipped cheats correct:
// every embedded cheat file must parse, and every entry's template must be
// well-formed with every <var> in the template backed by a VarSpec-compatible
// name.
func TestEmbeddedSamplesAreValid(t *testing.T) {
	names, err := SampleFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 4 {
		t.Fatalf("expected at least 4 sample files, got %d", len(names))
	}
	total := 0
	for _, name := range names {
		data, err := SampleData(name)
		if err != nil {
			t.Fatalf("read sample %s: %v", name, err)
		}
		entries, err := LoadCheatData(name, data)
		if err != nil {
			t.Fatalf("sample %s failed to load: %v", name, err)
		}
		for _, e := range entries {
			total++
			if e.Source != entry.SourceCheat {
				t.Errorf("%s: entry %q has source %v, want cheat", name, e.Desc, e.Source)
			}
			tpl, err := template.Parse(e.Template)
			if err != nil {
				t.Errorf("%s: entry %q has invalid template %q: %v", name, e.Desc, e.Template, err)
				continue
			}
			// Every template variable should be assemblable: provide dummy
			// values for all vars and confirm assembly succeeds.
			vals := map[string]string{}
			for _, ref := range tpl.Vars() {
				vals[ref.Name] = "X"
			}
			if _, err := tpl.Assemble(vals); err != nil {
				t.Errorf("%s: entry %q failed to assemble: %v", name, e.Desc, err)
			}
		}
	}
	if total < 10 {
		t.Errorf("expected 10+ sample entries across files, got %d", total)
	}
	t.Logf("validated %d sample cheat entries across %d files", total, len(names))
}

func TestSeedCheats(t *testing.T) {
	root := t.TempDir()
	p := Paths{ConfigDir: filepath.Join(root, "cfg"), CheatsDir: filepath.Join(root, "cfg", "cheats"), DataDir: filepath.Join(root, "data")}
	written, err := SeedCheats(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) < 4 {
		t.Fatalf("expected to seed 4+ files, wrote %d", len(written))
	}
	// Seeding again writes nothing (no clobber).
	written2, err := SeedCheats(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(written2) != 0 {
		t.Errorf("second seed wrote %d files, want 0", len(written2))
	}
	// The seeded files should now load through the normal path.
	entries, err := LoadCheats(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 10 {
		t.Errorf("loaded %d entries after seeding, want 10+", len(entries))
	}

	// Deleting an offered sample is an intentional user choice; the manifest
	// prevents it from being recreated on later launches.
	names, err := SampleFiles()
	if err != nil {
		t.Fatal(err)
	}
	deleted := filepath.Join(p.CheatsDir, names[0])
	if err := os.Remove(deleted); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedCheats(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(deleted); !os.IsNotExist(err) {
		t.Errorf("deleted sample was recreated: %v", err)
	}
}

func TestLoadCheatsReturnsValidFilesWithErrors(t *testing.T) {
	root := t.TempDir()
	p := Paths{CheatsDir: root}
	valid := []byte("[[cmd]]\ntitle = \"ok\"\ntmpl = \"echo ok\"\n")
	if err := os.WriteFile(filepath.Join(root, "good.toml"), valid, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bad.toml"), []byte("[[cmd"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := LoadCheats(p)
	if err == nil {
		t.Fatal("expected malformed-file diagnostic")
	}
	if len(entries) != 1 || entries[0].Title != "ok" {
		t.Fatalf("valid cheat was lost: %+v", entries)
	}
}

func TestCheatStableID(t *testing.T) {
	v1 := "[[cmd]]\nid = \"git.log-author\"\ntitle = \"Search log\"\ntmpl = \"git log --author=<a>\"\n"
	v2 := "[[cmd]]\nid = \"git.log-author\"\ntitle = \"Search git log by author\"\ntmpl = \"git log -n 5 --author=<a>\"\n"
	a, err := LoadCheatData("a.toml", []byte(v1))
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadCheatData("a.toml", []byte(v2))
	if err != nil {
		t.Fatal(err)
	}
	if a[0].ID() != b[0].ID() {
		t.Errorf("edited cheat with the same id changed identity: %q vs %q", a[0].ID(), b[0].ID())
	}
	if a[0].ID() != "cheat:git.log-author" {
		t.Errorf("ID = %q", a[0].ID())
	}

	dup := v1 + v1
	if _, err := LoadCheatData("d.toml", []byte(dup)); err == nil {
		t.Error("duplicate ids within a file should be rejected")
	}
	if _, err := LoadCheatData("bad.toml", []byte("[[cmd]]\nid = \"has space\"\ntitle = \"x\"\ntmpl = \"x\"\n")); err == nil {
		t.Error("invalid id should be rejected")
	}
}

func TestLoadCheatsDuplicateIDAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	body := "[[cmd]]\nid = \"same\"\ntitle = \"x\"\ntmpl = \"echo x\"\n"
	for _, n := range []string{"a.toml", "b.toml"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := LoadCheats(Paths{CheatsDir: dir})
	if err == nil || len(entries) != 1 {
		t.Fatalf("want 1 entry and a duplicate-id error, got %d, %v", len(entries), err)
	}
}
