// Package shellcmd runs short-lived shell commands behind commando's dynamic
// sources: candidate lists, previews, and project-provider listers.
package shellcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

const (
	// DefaultTimeout bounds interactive lookup commands so a slow external tool
	// cannot hang the picker indefinitely.
	DefaultTimeout = 8 * time.Second
	// DefaultMaxOutput is large enough for candidate lists while avoiding
	// unbounded memory growth from noisy commands.
	DefaultMaxOutput = 1 << 20
)

// Options controls shell command execution.
type Options struct {
	Dir       string
	Env       []string
	Timeout   time.Duration
	MaxOutput int
}

// Result is the captured result of a shell command.
type Result struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	TimedOut  bool
	Truncated bool
	Err       error
}

// Run executes command via /bin/sh -c with bounded runtime and output.
func Run(command string, opts Options) Result {
	return RunContext(context.Background(), command, opts)
}

// RunContext is Run with caller cancellation in addition to the configured
// timeout.
func RunContext(parent context.Context, command string, opts Options) Result {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	maxOutput := opts.MaxOutput
	if maxOutput <= 0 {
		maxOutput = DefaultMaxOutput
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	setProcessGroup(cmd)
	// Stop waiting for stray children that still hold stdout/stderr open once
	// the shell itself has been killed.
	cmd.WaitDelay = 250 * time.Millisecond
	cmd.Dir = opts.Dir
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}

	var stdout, stderr limitBuffer
	stdout.limit = maxOutput
	stderr.limit = maxOutput / 4
	if stderr.limit < 4096 {
		stderr.limit = 4096
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := Result{
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		ExitCode:  exitCode(err),
		TimedOut:  errors.Is(ctx.Err(), context.DeadlineExceeded),
		Truncated: stdout.truncated || stderr.truncated || stdout.full() || stderr.full(),
		Err:       err,
	}
	if res.TimedOut {
		res.Err = fmt.Errorf("command timed out after %s", timeout)
	} else if errors.Is(ctx.Err(), context.Canceled) {
		res.Err = context.Canceled
	}
	return res
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

type limitBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.truncated = true
		_, _ = b.buf.Write(p[:remaining])
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *limitBuffer) full() bool {
	return b.limit > 0 && b.Len() >= b.limit
}

func (b *limitBuffer) Len() int { return b.buf.Len() }

func (b *limitBuffer) String() string { return b.buf.String() }
