package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
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
	case "many-lines":
		// Buffered, so the lines land in the pipes as a burst and the process
		// exits immediately after: the tail used to be lost when cmd.Wait
		// closed the read ends before the readers had drained them.
		bo, be := bufio.NewWriterSize(os.Stdout, 64*1024), bufio.NewWriterSize(os.Stderr, 64*1024)
		for i := 1; i <= manyLines; i++ {
			fmt.Fprintln(bo, manyLine("out", i))
			fmt.Fprintln(be, manyLine("err", i))
		}
		bo.Flush()
		be.Flush()
	case "no-eol":
		fmt.Fprint(os.Stdout, "first\nlast-without-newline")
	case "streams":
		fmt.Fprintln(os.Stdout, "STDOUT-ONLY")
		fmt.Fprintln(os.Stderr, "STDERR-ONLY")
	case "partial-then-sleep":
		fmt.Fprintln(os.Stdout, "partial")
		fmt.Fprintln(os.Stderr, "something went wrong")
		time.Sleep(60 * time.Second)
	case "long-line":
		fmt.Fprintln(os.Stdout, "head")
		fmt.Fprintln(os.Stdout, strings.Repeat("a", 2*maxLineBytes))
		fmt.Fprintln(os.Stdout, "TAIL-MARKER")
	}
}

// manyLines is how much output the "many-lines" helper writes per stream. The
// lines are padded so each stream is far more than one pipe buffer, which is
// what puts output in flight when the child exits.
const manyLines = 2000

func manyLine(prefix string, i int) string {
	return fmt.Sprintf("%s %d %s", prefix, i, strings.Repeat("x", 100))
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

// Every line a command writes must reach the caller, including the ones
// written just before it exits: with StdoutPipe, cmd.Wait closed the read
// ends as soon as the child was reaped and the tail was silently dropped.
func TestExecStream_ManyLines_AllDelivered(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	name, args := helperArgs("many-lines")

	var mu sync.Mutex
	var out, errs []string
	var slept bool
	code, err := Exec{}.Stream(context.Background(), name, args, func(l Line) {
		mu.Lock()
		defer mu.Unlock()
		if !slept {
			// Lag the consumer once, so the child has exited and been reaped
			// with most of its output still sitting in the pipes - the shape
			// that used to lose everything past the first read.
			slept = true
			time.Sleep(300 * time.Millisecond)
		}
		if l.Stderr {
			errs = append(errs, l.Text)
		} else {
			out = append(out, l.Text)
		}
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if code != 0 {
		t.Fatalf("exitCode = %d, want 0", code)
	}
	for _, s := range []struct {
		stream string
		lines  []string
		prefix string
	}{{"stdout", out, "out"}, {"stderr", errs, "err"}} {
		if len(s.lines) != manyLines {
			t.Fatalf("%s: got %d lines, want %d", s.stream, len(s.lines), manyLines)
		}
		for i, got := range s.lines {
			if want := manyLine(s.prefix, i+1); got != want {
				t.Fatalf("%s line %d = %q, want %q", s.stream, i+1, got, want)
			}
		}
	}
}

// A command that exits without a trailing newline still wrote a last line.
func TestExecStream_NoTrailingNewline_DeliversLastLine(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	name, args := helperArgs("no-eol")

	var lines []string
	if _, err := (Exec{}).Stream(context.Background(), name, args, func(l Line) {
		lines = append(lines, l.Text)
	}); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	want := []string{"first", "last-without-newline"}
	if len(lines) != 2 || lines[0] != want[0] || lines[1] != want[1] {
		t.Fatalf("lines = %v, want %v", lines, want)
	}
}

// Guards the stream attribution itself: swapping stdout and stderr anywhere
// between the writers and Run's buffers must fail here.
func TestExecRun_StreamsNotSwapped(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	name, args := helperArgs("streams")

	stdout, stderr, code, err := Exec{}.Run(context.Background(), name, args)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Fatalf("exitCode = %d, want 0", code)
	}
	if !strings.Contains(stdout, "STDOUT-ONLY") || strings.Contains(stdout, "STDERR-ONLY") {
		t.Fatalf("stdout = %q, want only the stdout line", stdout)
	}
	if !strings.Contains(stderr, "STDERR-ONLY") || strings.Contains(stderr, "STDOUT-ONLY") {
		t.Fatalf("stderr = %q, want only the stderr line", stderr)
	}
}

// A command the deadline killed must not look like a clean run: it used to
// return exit code -1 with a nil error, which read as "the command said
// nothing" to every caller.
func TestExecRun_ContextDeadline_ReturnsError(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	name, args := helperArgs("partial-then-sleep")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	type result struct {
		stdout, stderr string
		code           int
		err            error
	}
	done := make(chan result, 1)
	go func() {
		out, errOut, code, err := Exec{}.Run(ctx, name, args)
		done <- result{out, errOut, code, err}
	}()

	var r result
	select {
	case r = <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("Run still blocked long after its deadline")
	}
	if r.err == nil {
		t.Fatalf("Run err = nil, want a deadline error (code %d, stdout %q)", r.code, r.stdout)
	}
	if !errors.Is(r.err, context.DeadlineExceeded) {
		t.Fatalf("Run err = %v, want context.DeadlineExceeded", r.err)
	}
	if !strings.Contains(r.err.Error(), "something went wrong") {
		t.Fatalf("err = %q, want the stderr tail quoted", r.err)
	}
	if !strings.Contains(r.stdout, "partial") {
		t.Fatalf("stdout = %q, want the output written before the deadline", r.stdout)
	}
}

// The ctx watcher may not kill once cmd.Wait has returned: on the normal exit
// path no member of the child's group is left, so the pgid is free and the
// signal would land on whatever recycled the pid. The guard is what draws
// that line, and stop must be a hard boundary even under a concurrent kill.
func TestKillGuard_NoKillOnceStopped(t *testing.T) {
	for i := 0; i < 200; i++ {
		var g killGuard
		var mu sync.Mutex
		stopped, killedAfterStop := false, false

		done := make(chan struct{})
		go func() {
			defer close(done)
			g.run(func() {
				mu.Lock()
				if stopped {
					killedAfterStop = true
				}
				mu.Unlock()
			})
		}()

		g.stop()
		mu.Lock()
		stopped = true
		mu.Unlock()
		<-done

		mu.Lock()
		bad := killedAfterStop
		mu.Unlock()
		if bad {
			t.Fatalf("run %d: killed after stop returned", i)
		}
	}
}

// Before stop, the kill is exactly what has to happen: a cancelled command
// whose grandchild holds the pipes only dies because of it.
func TestKillGuard_KillsBeforeStop(t *testing.T) {
	var g killGuard
	killed := false
	g.run(func() { killed = true })
	if !killed {
		t.Fatal("the guard suppressed a kill while the child was still running")
	}
	g.stop()
	g.run(func() { t.Fatal("the guard let a kill through after stop") })
}

// Run holds its whole result in memory, so a command that never stops talking
// must not be able to grow the heap a line at a time.
func TestExecRun_CapsTotalOutput(t *testing.T) {
	var c capped
	line := strings.Repeat("x", 1024)
	for i := 0; i < (maxRunBytes/len(line))+100; i++ {
		c.line(line)
	}
	got := c.String()
	if len(got) > maxRunBytes+len(OutputTruncationMarker)+1 {
		t.Fatalf("kept %d bytes, want no more than the %d-byte cap", len(got), maxRunBytes)
	}
	if !strings.HasSuffix(got, OutputTruncationMarker+"\n") {
		t.Fatal("the cap was hit but nothing said so")
	}
	if strings.Count(got, OutputTruncationMarker) != 1 {
		t.Fatal("the marker repeats once per dropped line")
	}
}
