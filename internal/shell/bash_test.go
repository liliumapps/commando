package shell

import (
	"strings"
	"testing"
)

func TestBashInitContainsWidget(t *testing.T) {
	out := BashInit("^G")
	for _, want := range []string{
		"commando search --print",
		`READLINE_LINE="$out"`,
		`bind -m emacs-standard -x '"\C-g":__commando_widget'`,
		`bind -m vi-insert -x '"\C-g":__commando_widget'`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("bash init missing %q", want)
		}
	}
}

func TestBashInitCustomAndDefaultBinding(t *testing.T) {
	if out := BashInit("^T"); !strings.Contains(out, `\C-t`) {
		t.Errorf("custom binding not applied:\n%s", out)
	}
	if out := BashInit(""); !strings.Contains(out, `\C-g`) {
		t.Errorf("default binding not applied:\n%s", out)
	}
}
