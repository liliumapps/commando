package shellcmd

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunCapturesStdout(t *testing.T) {
	res := Run("printf 'a\\nb\\n'", Options{})
	if res.Err != nil {
		t.Fatalf("Run failed: %v", res.Err)
	}
	if res.Stdout != "a\nb\n" {
		t.Errorf("stdout = %q", res.Stdout)
	}
}

func TestRunTimesOut(t *testing.T) {
	res := Run("sleep 1", Options{Timeout: 10 * time.Millisecond})
	if !res.TimedOut {
		t.Fatalf("expected timeout, got err=%v", res.Err)
	}
}

func TestRunTruncatesOutput(t *testing.T) {
	res := Run("printf '0123456789abcdef'", Options{MaxOutput: 8})
	if !res.Truncated {
		t.Fatal("expected truncated output")
	}
	if res.Stdout != "01234567" {
		t.Errorf("stdout = %q, want truncated prefix", res.Stdout)
	}
}

func TestTimeoutKillsPipeline(t *testing.T) {
	start := time.Now()
	res := Run("sleep 5 | cat", Options{Timeout: 300 * time.Millisecond})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("pipeline outlived its timeout: %v", elapsed)
	}
	if !res.TimedOut {
		t.Errorf("TimedOut = false, want true (err=%v)", res.Err)
	}
}

func TestCancelKillsPipeline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel() }()
	start := time.Now()
	res := RunContext(ctx, "sleep 5 | cat", Options{})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("cancelled pipeline kept running: %v", elapsed)
	}
	if !errors.Is(res.Err, context.Canceled) {
		t.Errorf("Err = %v, want context.Canceled", res.Err)
	}
}
