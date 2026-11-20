package shell

import (
	"strings"
	"testing"
)

func TestZshInitContainsWidget(t *testing.T) {
	out := ZshInit("^G")
	for _, want := range []string{
		"commando search --print",
		"zle -N __commando_widget",
		"bindkey '^G' __commando_widget",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("zsh init missing %q", want)
		}
	}
}

func TestZshInitHasNoCaptureHook(t *testing.T) {
	// commando reads the shell history file directly, so it must NOT install a
	// preexec/precmd capture hook.
	out := ZshInit("^G")
	for _, forbidden := range []string{"add-zsh-hook preexec", "add-zsh-hook precmd", "history record"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("zsh init should not contain %q (no capture hook)", forbidden)
		}
	}
}

func TestZshInitCustomBinding(t *testing.T) {
	out := ZshInit("^T")
	if !strings.Contains(out, "bindkey '^T' __commando_widget") {
		t.Errorf("custom binding not applied:\n%s", out)
	}
}

func TestZshInitDefaultsBinding(t *testing.T) {
	out := ZshInit("")
	if !strings.Contains(out, "bindkey '^G' __commando_widget") {
		t.Error("empty binding should default to ^G")
	}
}

func TestInitFallsBackForUnsafeBinding(t *testing.T) {
	for name, out := range map[string]string{
		"zsh":  ZshInit(`'; echo unsafe`),
		"bash": BashInit(`"; echo unsafe`),
		"fish": FishInit(`; echo unsafe`),
	} {
		if strings.Contains(out, "echo unsafe") {
			t.Errorf("%s output included unsafe binding:\n%s", name, out)
		}
	}
}
