package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Exec is the real Runner, invoking os/exec.
type Exec struct{}

var _ Runner = Exec{}

// extraEnv is appended to the current process environment for every command:
// non-interactive, no ANSI color codes, no fancy terminal capabilities.
var extraEnv = []string{"CI=1", "NO_COLOR=1", "TERM=dumb"}

const (
	// waitDelay bounds the wait for the command's pipes after its process
	// exits or is killed. `claude` is a Node wrapper, so a grandchild can
	// inherit stdout and hold it open long after the deadline; without this
	// the job would never end and the jobs store would stay busy forever.
	waitDelay = 5 * time.Second

	// maxLineBytes caps one output line. A longer line is dropped rather than
	// buffered, so a single huge blob can't exhaust memory - but draining
	// continues, so the child never blocks on a full pipe either.
	maxLineBytes = 1024 * 1024

	truncationMarker = "[line truncated]"

	// maxRunBytes caps what Run keeps per stream. Run's callers parse a CLI's
	// answer - a version string, a JSON list - so output past this is a
	// runaway command rather than a reply, and the whole of it is held in
	// memory. Stream, which the jobs drawer uses, is capped by the store.
	maxRunBytes = 4 * 1024 * 1024

	// OutputTruncationMarker is appended in place of the output Run dropped.
	// A parser that finds it in what it was handed is looking at a prefix of
	// the answer, not the answer.
	OutputTruncationMarker = "[output truncated]"

	// stderrTailLines is how much stderr a returned error quotes.
	stderrTailLines = 3
)

func (Exec) Run(ctx context.Context, name string, args []string) (stdout, stderr string, exitCode int, err error) {
	var outBuf, errBuf capped
	exitCode, err = (Exec{}).Stream(ctx, name, args, func(l Line) {
		if l.Stderr {
			errBuf.line(l.Text)
		} else {
			outBuf.line(l.Text)
		}
	})
	return outBuf.String(), errBuf.String(), exitCode, err
}

// capped collects lines until it has maxRunBytes of them, then keeps only a
// note that it stopped. Each line is already capped, but their number is not.
type capped struct {
	b    strings.Builder
	full bool
}

func (c *capped) line(text string) {
	if c.full {
		return
	}
	if c.b.Len()+len(text)+1 > maxRunBytes {
		c.full = true
		c.b.WriteString(OutputTruncationMarker + "\n")
		return
	}
	c.b.WriteString(text)
	c.b.WriteByte('\n')
}

func (c *capped) String() string { return c.b.String() }

func (Exec) Stream(ctx context.Context, name string, args []string, onLine func(Line)) (int, error) {
	return Exec{}.StreamDir(ctx, "", name, args, onLine)
}

func (Exec) StreamDir(ctx context.Context, dir, name string, args []string, onLine func(Line)) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdin = strings.NewReader("")
	cmd.Cancel = func() error { return killTree(cmd) }
	cmd.WaitDelay = waitDelay
	setProcAttr(cmd)

	// onLine is called from two goroutines (stdout/stderr); serialize so
	// callers (job logs) don't need to be concurrency-safe themselves.
	var mu sync.Mutex
	var tail []string
	emit := func(l Line) {
		mu.Lock()
		defer mu.Unlock()
		if l.Stderr {
			tail = append(tail, l.Text)
			if len(tail) > stderrTailLines {
				tail = tail[1:]
			}
		}
		if onLine != nil {
			onLine(l)
		}
	}

	// Our own writers, not StdoutPipe: exec then owns the pipes and their
	// copying goroutines, so cmd.Wait drains every byte the child wrote
	// before returning, and WaitDelay force-closes them when a grandchild
	// keeps its copy open.
	outW := &lineWriter{onLine: emit}
	errW := &lineWriter{stderr: true, onLine: emit}
	cmd.Stdout = outW
	cmd.Stderr = errW

	if err := cmd.Start(); err != nil {
		return -1, err
	}

	// exec.CommandContext only invokes cmd.Cancel if ctx.Done() wins a race
	// against cmd.Process.Wait() returning. A wrapper like `claude` can exit
	// on its own well before ctx is done, leaving a grandchild holding
	// stdout - in that case the stdlib never calls Cancel and we'd sit out
	// the full WaitDelay before the group gets killed. Watch ctx ourselves
	// so the kill is unconditional and immediate.
	//
	// The guard narrows that watcher to the window where the group is still
	// ours; it cannot close it, since the check cannot be atomic with wait4. Once Wait has returned on the normal exit path no member
	// of the group is left, the kernel is free to reuse the pid, and killing
	// the group would signal whatever took it.
	var guard killGuard
	watchDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			guard.run(func() { _ = killTree(cmd) })
		case <-watchDone:
		}
	}()

	err := cmd.Wait()
	guard.stop()
	close(watchDone)
	if errors.Is(err, exec.ErrWaitDelay) && killAfterReap {
		// The child is reaped by now and a descendant still holds the pipes,
		// so this signals a pgid whose leader has already exited. The kernel
		// keeps that pid reserved while any member of the group remains, and
		// a member is precisely what is holding the pipes, so the group is
		// still ours. The residual case is a holder that left the group by
		// its own setsid; there the pid could in principle have been reused,
		// and the alternative is leaking the descendant, so we accept it.
		//
		// None of that reasoning holds on Windows, where the kill is by pid
		// and the pid is free the moment Wait returns, so killAfterReap is
		// false there.
		_ = killTree(cmd)
	}
	outW.Close()
	errW.Close()

	code := -1
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		code = 0
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	case errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState != nil:
		// The process itself finished; only leaked pipes kept us waiting.
		code = cmd.ProcessState.ExitCode()
	}

	mu.Lock()
	stderrTail := strings.Join(tail, "; ")
	mu.Unlock()

	// A command the deadline killed exits with code -1; reporting that as a
	// success with no error made a timed-out `claude mcp list` read as "no
	// servers at all".
	if ctxErr := ctx.Err(); ctxErr != nil {
		return code, cmdError(name, args, ctxErr, stderrTail)
	}
	if code < 0 {
		if err == nil {
			err = errors.New("did not exit normally")
		}
		return code, cmdError(name, args, err, stderrTail)
	}
	return code, nil
}

// killGuard runs a kill only while the child is still unreaped. stop closes
// that window, and a kill already inside it finishes before stop returns.
type killGuard struct {
	mu      sync.Mutex
	stopped bool
}

func (g *killGuard) run(kill func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.stopped {
		kill()
	}
}

func (g *killGuard) stop() {
	g.mu.Lock()
	g.stopped = true
	g.mu.Unlock()
}

func cmdError(name string, args []string, err error, stderrTail string) error {
	// Both halves are the command's own text, so both get the redaction the
	// streamed lines get: this string becomes job.Error and is served over the
	// API.
	argv := Redact(Argv(name, args))
	if stderrTail = Redact(stderrTail); stderrTail != "" {
		return fmt.Errorf("%s: %w: %s", argv, err, stderrTail)
	}
	return fmt.Errorf("%s: %w", argv, err)
}

// lineWriter splits what a command writes into lines and hands each one to
// onLine. Partial lines are buffered until their "\n" arrives; Close emits
// whatever is left, so a command that exits without a trailing newline still
// reports its last line. A line longer than maxLineBytes is dropped and
// replaced, once per stream, by truncationMarker.
type lineWriter struct {
	stderr  bool
	onLine  func(Line)
	buf     []byte
	dropped bool
	marked  bool
}

func (w *lineWriter) Write(p []byte) (int, error) {
	n := len(p)
	for {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			w.accumulate(p)
			return n, nil
		}
		w.accumulate(p[:i])
		w.flush()
		p = p[i+1:]
	}
}

// Close emits the trailing partial line, if any. It is not io.Closer for the
// command's benefit - exec never calls it - but for ours, after cmd.Wait.
func (w *lineWriter) Close() {
	if w.dropped || len(w.buf) > 0 {
		w.flush()
	}
}

func (w *lineWriter) accumulate(chunk []byte) {
	if w.dropped {
		return
	}
	if len(w.buf)+len(chunk) > maxLineBytes {
		w.dropped, w.buf = true, nil
		return
	}
	w.buf = append(w.buf, chunk...)
}

func (w *lineWriter) flush() {
	switch {
	case w.dropped:
		if !w.marked {
			w.emit(truncationMarker)
			w.marked = true
		}
	default:
		w.emit(trimCR(w.buf))
	}
	w.buf, w.dropped = nil, false
}

func (w *lineWriter) emit(text string) {
	if w.onLine != nil {
		w.onLine(Line{Stderr: w.stderr, Text: text})
	}
}

// trimCR strips the "\r" of a "\r\n" line ending, matching what
// bufio.ScanLines did.
func trimCR(b []byte) string {
	return string(bytes.TrimSuffix(b, []byte("\r")))
}
