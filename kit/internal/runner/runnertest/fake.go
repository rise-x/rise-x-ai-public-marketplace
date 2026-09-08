// Package runnertest provides a runner.Runner double so no test depends on an
// installed claude CLI. It lives outside package runner so the fake - and its
// deliberate panic on an unregistered command - never ships in the binary.
package runnertest

import (
	"context"
	"fmt"
	"strings"
	"sync"

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
}

var _ runner.Runner = (*Fake)(nil)

func NewFake() *Fake {
	return &Fake{Responses: map[string]Result{}}
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

func (f *Fake) Run(_ context.Context, name string, args []string) (string, string, int, error) {
	r := f.lookup("", name, args)
	return r.Stdout, r.Stderr, r.ExitCode, r.Err
}

func (f *Fake) Stream(ctx context.Context, name string, args []string, onLine func(runner.Line)) (int, error) {
	return f.StreamDir(ctx, "", name, args, onLine)
}

// StreamDir answers the same canned Result as Stream; the requested directory
// is recorded on the Call so a test can assert the scope a command ran in.
func (f *Fake) StreamDir(_ context.Context, dir, name string, args []string, onLine func(runner.Line)) (int, error) {
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
