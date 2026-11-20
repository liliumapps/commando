package cli

import (
	"errors"
	"io"
	"os"
	"os/exec"
)

// execViaShell runs cmd through the user's shell, wiring stdio through so the
// command behaves as if typed. It returns the command's exit code. This backs
// `search --exec`; the default flow instead prints the command for the shell
// widget to place on the command line.
func execViaShell(cmd string, stdout, stderr io.Writer) int {
	shellPath := os.Getenv("SHELL")
	if shellPath == "" {
		shellPath = "/bin/sh"
	}
	c := exec.Command(shellPath, "-c", cmd)
	c.Stdin = os.Stdin
	c.Stdout = stdout
	c.Stderr = stderr
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		return 1
	}
	return 0
}
