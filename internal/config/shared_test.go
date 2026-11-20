package config

import "testing"

func TestSharedVarsMergeInUseOrder(t *testing.T) {
	data := `
[[vars.aws]]
name = "profile"
src  = "aws configure list-profiles"

[[vars.aws]]
name = "region"
default = "us-east-1"

[[cmd]]
desc = "ssh"
tmpl = "aws --profile <profile> --region <region> s3 ls"
use = ["aws"]
`
	entries, err := LoadCheatData("t.toml", []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if len(e.Vars) != 2 {
		t.Fatalf("want 2 merged vars, got %d: %+v", len(e.Vars), e.Vars)
	}
	if e.Vars[0].Name != "profile" || e.Vars[1].Name != "region" {
		t.Errorf("shared vars order = [%s %s], want [profile region]", e.Vars[0].Name, e.Vars[1].Name)
	}
	if e.Vars[0].Src != "aws configure list-profiles" || e.Vars[1].Default != "us-east-1" {
		t.Errorf("shared var contents not carried through: %+v", e.Vars)
	}
}

func TestCommandLocalOverridesShared(t *testing.T) {
	data := `
[[vars.common]]
name = "region"
default = "us-east-1"
src = "shared-src"

[[cmd]]
desc = "x"
tmpl = "cmd --region <region> --extra <extra>"
use = ["common"]

  [[cmd.var]]
  name = "region"
  default = "eu-west-1"

  [[cmd.var]]
  name = "extra"
`
	entries, err := LoadCheatData("t.toml", []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	e := entries[0]
	// region should appear once, overridden to eu-west-1 (and the shared src dropped
	// because the local spec fully replaces it), and keep its original position.
	var region, extra bool
	for _, v := range e.Vars {
		if v.Name == "region" {
			region = true
			if v.Default != "eu-west-1" {
				t.Errorf("region default = %q, want eu-west-1 (local override)", v.Default)
			}
			if v.Src != "" {
				t.Errorf("region src = %q, want empty (local override replaces shared)", v.Src)
			}
		}
		if v.Name == "extra" {
			extra = true
		}
	}
	if !region || !extra {
		t.Errorf("expected region and extra vars, got %+v", e.Vars)
	}
	// No duplicate region.
	count := 0
	for _, v := range e.Vars {
		if v.Name == "region" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("region appears %d times, want 1", count)
	}
}

func TestUnknownGroupErrors(t *testing.T) {
	data := `
[[cmd]]
desc = "x"
tmpl = "cmd <a>"
use = ["nope"]
[[cmd.var]]
name = "a"
`
	if _, err := LoadCheatData("t.toml", []byte(data)); err == nil {
		t.Error("expected error for unknown shared variable group")
	}
}

func TestMultipleGroups(t *testing.T) {
	data := `
[[vars.a]]
name = "x"
[[vars.b]]
name = "y"

[[cmd]]
desc = "d"
tmpl = "cmd <x> <y> <z>"
use = ["a", "b"]
[[cmd.var]]
name = "z"
`
	entries, err := LoadCheatData("t.toml", []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, v := range entries[0].Vars {
		names = append(names, v.Name)
	}
	if len(names) != 3 || names[0] != "x" || names[1] != "y" || names[2] != "z" {
		t.Errorf("merged var order = %v, want [x y z]", names)
	}
}
