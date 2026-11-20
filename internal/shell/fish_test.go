package shell

import (
	"strings"
	"testing"
)

func TestFishInitContainsWidget(t *testing.T) {
	out := FishInit("^G")
	for _, want := range []string{
		"commando search --print",
		`commandline --replace -- "$out"`,
		"bind \\cg __commando_widget",
		"bind -M insert \\cg __commando_widget",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("fish init missing %q", want)
		}
	}
}

func TestFishInitCustomAndDefaultBinding(t *testing.T) {
	if out := FishInit("^T"); !strings.Contains(out, `\ct`) {
		t.Errorf("custom binding not applied:\n%s", out)
	}
	if out := FishInit(""); !strings.Contains(out, `\cg`) {
		t.Errorf("default binding not applied:\n%s", out)
	}
}
