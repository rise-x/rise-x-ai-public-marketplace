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

	// stderrTailLines is how much stderr a returned error quotes.
	stderrTailLines = 3
)

func (Exec) Run(ctx context.Context, name string, args []string) (stdout, stderr string, exitCode int, err error) {
	var outBuf, errBuf strings.Builder
	exitCode, err = (Exec{}).Stream(ctx, name, args, func(l Line) {
		if l.Stderr {
			errBuf.WriteString(l.Text)
			errBuf.WriteByte('\n')
		} else {
			outBuf.WriteString(l.Text)
			outBuf.WriteByte('\n')
		}
	})
	return outBuf.String(), errBuf.String(), exitCode, err
}

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
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = killTree(cmd)
		case <-watchDone:
		}
	}()

	err := cmd.Wait()
	if errors.Is(err, exec.ErrWaitDelay) {
		// The child is reaped by now and a descendant still holds the pipes,
		// so this signals a pgid whose leader has already exited. The kernel
		// keeps that pid reserved while any member of the group remains, and
		// a member is precisely what is holding the pipes, so the group is
		// still ours. The residual case is a holder that left the group by
		// its own setsid; there the pid could in principle have been reused,
		// and the alternative is leaking the descendant, so we accept it.
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

func cmdError(name string, args []string, err error, stderrTail string) error {
	// The tail is the command's own output, so it gets the same redaction the
	// streamed lines get: this string becomes job.Error and is served over the
	// API.
	if stderrTail = Redact(stderrTail); stderrTail != "" {
		return fmt.Errorf("%s: %w: %s", Argv(name, args), err, stderrTail)
	}
	return fmt.Errorf("%s: %w", Argv(name, args), err)
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
