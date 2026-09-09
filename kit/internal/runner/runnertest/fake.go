// Package runnertest provides a runner.Runner double so no test depends on an
// installed claude CLI. It lives outside package runner so the fake - and its
// deliberate panic on an unregistered command - never ships in the binary.
package runnertest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner"
)

// Call records one invocation made through a Fake.
type Call struct {
	Name string
	Args []string
	// Dir is the working directory the call asked for; empty means the
	// process's own.
	Dir string
}

// Result is the canned response for one command.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// Fake is a Runner double for tests: no real process is ever started.
type Fake struct {
	mu sync.Mutex

	// Responses maps "name arg1 arg2" (Argv-joined) to a canned Result.
	// A missing entry is a test bug and panics, so failures are loud.
	Responses map[string]Result

	Calls []Call

	delay time.Duration
}

var _ runner.Runner = (*Fake)(nil)

func NewFake() *Fake {
	return &Fake{Responses: map[string]Result{}}
}

// SetDelay makes every later call block for d before answering, so a test can
// drive a real timeout (the wait still ends early if the context does).
func (f *Fake) SetDelay(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delay = d
}

// wait honours the caller's context: a context that is already done, or that
// ends while the delay runs, answers with its error instead of the Result.
func (f *Fake) wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	d := f.delay
	f.mu.Unlock()
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return ctx.Err()
	}
}

// Set registers the Result returned for name+args.
func (f *Fake) Set(name string, args []string, r Result) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Responses[key(name, args)] = r
}

func key(name string, args []string) string {
	return strings.Join(append([]string{name}, args...), "\x00")
}

func (f *Fake) lookup(dir, name string, args []string) Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Name: name, Args: append([]string(nil), args...), Dir: dir})
	r, ok := f.Responses[key(name, args)]
	if !ok {
		panic(fmt.Sprintf("runnertest.Fake: no response set for %s", runner.Argv(name, args)))
	}
	return r
}

func (f *Fake) Run(ctx context.Context, name string, args []string) (string, string, int, error) {
	if err := f.wait(ctx); err != nil {
		return "", "", -1, err
	}
	r := f.lookup("", name, args)
	return r.Stdout, r.Stderr, r.ExitCode, r.Err
}

func (f *Fake) Stream(ctx context.Context, name string, args []string, onLine func(runner.Line)) (int, error) {
	return f.StreamDir(ctx, "", name, args, onLine)
}

// StreamDir answers the same canned Result as Stream; the requested directory
// is recorded on the Call so a test can assert the scope a command ran in.
func (f *Fake) StreamDir(ctx context.Context, dir, name string, args []string, onLine func(runner.Line)) (int, error) {
	if err := f.wait(ctx); err != nil {
		return -1, err
	}
	r := f.lookup(dir, name, args)
	if onLine != nil {
		for _, line := range splitNonEmptyLines(r.Stdout) {
			onLine(runner.Line{Text: line})
		}
		for _, line := range splitNonEmptyLines(r.Stderr) {
			onLine(runner.Line{Stderr: true, Text: line})
		}
	}
	return r.ExitCode, r.Err
}

func splitNonEmptyLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}
