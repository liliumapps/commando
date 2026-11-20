//go:build windows

package shellcmd

import "os/exec"

// setProcessGroup is a no-op on Windows; WaitDelay still bounds how long Run
// waits for children holding the output pipes.
func setProcessGroup(*exec.Cmd) {}
