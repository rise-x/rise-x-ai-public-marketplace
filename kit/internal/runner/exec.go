package runner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
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

	readBufBytes = 64 * 1024
	// maxLineBytes caps one output line. A longer line is dropped rather than
	// buffered, so a single huge blob can't exhaust memory - but draining
	// continues, so the child never blocks on a full pipe either.
	maxLineBytes = 1024 * 1024

	truncationMarker = "[line truncated]"
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

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return -1, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return -1, err
	}

	if err := cmd.Start(); err != nil {
		return -1, err
	}

	// onLine is called from two goroutines (stdout/stderr); serialize so
	// callers (job logs) don't need to be concurrency-safe themselves.
	var mu sync.Mutex
	safeOnLine := onLine
	if safeOnLine != nil {
		safeOnLine = func(l Line) {
			mu.Lock()
			defer mu.Unlock()
			onLine(l)
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go scanLines(stdout, false, safeOnLine, &wg)
	go scanLines(stderr, true, safeOnLine, &wg)

	// Wait first, then join the scanners: WaitDelay closes the pipes once the
	// process is gone, which is what unblocks a scanner still reading from a
	// grandchild's copy of them.
	err = cmd.Wait()
	// cmd.Cancel only fires if ctx.Done() beats the direct child's own exit;
	// when the child (a Node wrapper, say) exits on its own first, Cancel
	// never runs and its process group is never signaled. Reap it here too,
	// so no grandchild outlives Stream regardless of which one wins that race.
	_ = killTree(cmd)
	wg.Wait()

	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState != nil {
		// The process itself finished; only leaked pipes kept us waiting.
		return cmd.ProcessState.ExitCode(), nil
	}
	return -1, err
}

// scanLines reads r line by line and hands each line to onLine. A line longer
// than maxLineBytes is dropped and replaced, once per stream, by
// truncationMarker; reading then carries on with the next line, so neither an
// over-long line nor a closed pipe can wedge the goroutine.
func scanLines(r io.Reader, stderr bool, onLine func(Line), wg *sync.WaitGroup) {
	defer wg.Done()
	emit := func(text string) {
		if onLine != nil {
			onLine(Line{Stderr: stderr, Text: text})
		}
	}

	br := bufio.NewReaderSize(r, readBufBytes)
	var line []byte
	dropped, marked := false, false
	for {
		chunk, err := br.ReadSlice('\n')
		if !dropped {
			if len(line)+len(chunk) > maxLineBytes {
				dropped, line = true, nil
			} else {
				line = append(line, chunk...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue // more of the same line is still in the pipe
		}
		switch {
		case dropped:
			if !marked {
				emit(truncationMarker)
				marked = true
			}
		case err == nil || len(line) > 0:
			emit(trimEOL(line))
		}
		line, dropped = nil, false
		if err != nil {
			return
		}
	}
}

// trimEOL strips one trailing "\n" and the "\r" before it, matching what
// bufio.ScanLines did.
func trimEOL(b []byte) string {
	b = bytes.TrimSuffix(b, []byte("\n"))
	b = bytes.TrimSuffix(b, []byte("\r"))
	return string(b)
}
