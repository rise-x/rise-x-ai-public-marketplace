package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is not a real test; it is re-executed as a subprocess by
// the tests below (the standard os/exec helper-process pattern), so Exec can
// be exercised without depending on any real external binary.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	args = args[1:]

	switch args[0] {
	case "stream-order":
		fmt.Fprintln(os.Stdout, "out1")
		fmt.Fprintln(os.Stderr, "err1")
		fmt.Fprintln(os.Stdout, "out2")
	case "exit-code":
		os.Exit(3)
	case "sleep":
		time.Sleep(60 * time.Second)
	case "grandchild":
		// Start a child that inherits stdout and outlives us, then exit
		// cleanly - the shape that used to hang Stream forever.
		name, sub := helperArgs("sleep")
		child := exec.Command(name, sub...)
		child.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		child.Stdout = os.Stdout
		if err := child.Start(); err != nil {
			os.Exit(4)
		}
		fmt.Fprintln(os.Stdout, "grandchild", child.Process.Pid)
	case "long-line":
		fmt.Fprintln(os.Stdout, "head")
		fmt.Fprintln(os.Stdout, strings.Repeat("a", 2*maxLineBytes))
		fmt.Fprintln(os.Stdout, "TAIL-MARKER")
	}
}

func helperArgs(sub string) (string, []string) {
	return os.Args[0], []string{"-test.run=TestHelperProcess", "--", sub}
}

// runStream runs Stream in a goroutine and fails the test if it has not
// returned within limit, instead of hanging until go test's own timeout.
func runStream(t *testing.T, ctx context.Context, sub string, limit time.Duration, onLine func(Line)) int {
	t.Helper()
	name, args := helperArgs(sub)
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, err := Exec{}.Stream(ctx, name, args, onLine)
		done <- result{code, err}
	}()
	select {
	case r := <-done:
		return r.code
	case <-time.After(limit):
		t.Fatalf("Stream(%s) still blocked after %s", sub, limit)
		return 0
	}
}

func TestExecStream_OrderAndLines(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	name, args := helperArgs("stream-order")

	var lines []Line
	exitCode, err := Exec{}.Stream(context.Background(), name, args, func(l Line) {
		lines = append(lines, l)
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3: %+v", len(lines), lines)
	}
	// stdout/stderr are separate pipes read concurrently, so only same-stream
	// order is guaranteed.
	var stdout []string
	for _, l := range lines {
		if !l.Stderr {
			stdout = append(stdout, l.Text)
		}
	}
	if len(stdout) != 2 || stdout[0] != "out1" || stdout[1] != "out2" {
		t.Fatalf("stdout lines = %v, want [out1 out2]", stdout)
	}
}

func TestExecStream_ExitCode(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	name, args := helperArgs("exit-code")

	exitCode, err := Exec{}.Stream(context.Background(), name, args, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if exitCode != 3 {
		t.Fatalf("exitCode = %d, want 3", exitCode)
	}
}

// A cancelled context must end the call, not leave the caller waiting on the
// child's own lifetime.
func TestExecStream_ContextDeadline_Returns(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	runStream(t, ctx, "sleep", 20*time.Second, nil)
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatalf("Stream returned after %s, want soon after the 200ms deadline", elapsed)
	}
}

// Regression for the grandchild case: exec.CommandContext kills only the
// direct child, so without WaitDelay the inherited stdout kept Stream blocked
// long past the deadline.
func TestExecStream_GrandchildHoldsPipe_StillReturns(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	runStream(t, ctx, "grandchild", 30*time.Second, nil)
	// waitDelay (5s) is the bound; the child sleeps 60s, so anything under
	// that proves the pipes were force-closed.
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Fatalf("Stream returned after %s, want the WaitDelay bound", elapsed)
	}
}

// A line past maxLineBytes must not swallow the rest of the stream, and must
// not wedge the child on a full pipe.
func TestExecStream_OverLongLine_KeepsDraining(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")

	var mu struct {
		lines []string
	}
	code := runStream(t, context.Background(), "long-line", 30*time.Second, func(l Line) {
		mu.lines = append(mu.lines, l.Text)
	})
	if code != 0 {
		t.Fatalf("exitCode = %d, want 0", code)
	}
	joined := strings.Join(mu.lines, "\n")
	if !strings.Contains(joined, "head") {
		t.Fatalf("lines before the long one were lost: %q", joined)
	}
	if !strings.Contains(joined, truncationMarker) {
		t.Fatalf("expected a %q marker, got %q", truncationMarker, joined)
	}
	if !strings.Contains(joined, "TAIL-MARKER") {
		t.Fatalf("lines after the long one were lost: %q", joined)
	}
	for _, l := range mu.lines {
		if len(l) > maxLineBytes {
			t.Fatalf("a line of %d bytes reached the caller", len(l))
		}
	}
}
