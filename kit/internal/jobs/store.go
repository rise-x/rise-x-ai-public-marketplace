// Package jobs runs long commands in the background and lets the server
// poll their output. Only one job may run at a time, since every job is a
// mutating claude CLI call and running two concurrently would race.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrBusy is returned by Start when a job is already running.
var ErrBusy = errors.New("a job is already running")

// maxRetainedJobs bounds the finished jobs kept for polling. The drawer only
// ever shows the current one, so this is just enough history for a page that
// polls a job it started a moment ago.
const maxRetainedJobs = 50

// maxRetainedLines bounds one job's kept output. A `curl | bash` installer can
// emit hundreds of thousands of lines, and maxRetainedJobs of those are held
// at once; the drawer shows a tail, so the oldest lines are what to drop.
const maxRetainedLines = 5000

type Status string

const (
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

// Job is the public, immutable-per-snapshot view of one job.
type Job struct {
	ID         string     `json:"id"`
	Action     string     `json:"action"`
	Status     Status     `json:"status"`
	ExitCode   int        `json:"exitCode"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// LogLine is one sequenced line of job output.
type LogLine struct {
	Seq  int    `json:"seq"`
	Text string `json:"text"`
}

// Snapshot is what GET /api/jobs/{id}?since=N returns.
type Snapshot struct {
	Job Job       `json:"job"`
	Log []LogLine `json:"log"`
}

// Func is the work a job does. It must respect ctx; the store gives up on it
// after the job's timeout either way.
type Func func(ctx context.Context, onLine func(string)) (exitCode int, err error)

type job struct {
	mu  sync.Mutex
	job Job
	log []LogLine
	// nextSeq counts every line the job ever emitted, not the ones still
	// kept: the page polls with ?since=<seq>, so a sequence derived from the
	// slice length would repeat numbers after a drop and the browser would
	// silently skip the new lines carrying them.
	nextSeq int
	done    bool
}

func (j *job) appendLine(text string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.done {
		return // fn may outlive its timeout; its late output is not the job's
	}
	j.nextSeq++
	j.log = append(j.log, LogLine{Seq: j.nextSeq, Text: text})
	if len(j.log) > maxRetainedLines {
		j.log = append(j.log[:0], j.log[len(j.log)-maxRetainedLines:]...)
	}
}

func (j *job) snapshot(since int) Snapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	var lines []LogLine
	for _, l := range j.log {
		if l.Seq > since {
			lines = append(lines, l)
		}
	}
	return Snapshot{Job: j.job, Log: lines}
}

func (j *job) finish(exitCode int, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.done {
		return // the timeout already recorded a verdict
	}
	j.done = true
	j.job.ExitCode = exitCode
	now := time.Now()
	j.job.FinishedAt = &now
	if err != nil {
		j.job.Status = StatusFailed
		j.job.Error = err.Error()
	} else if exitCode != 0 {
		j.job.Status = StatusFailed
	} else {
		j.job.Status = StatusSucceeded
	}
}

// Store tracks jobs for the lifetime of the process.
type Store struct {
	mu        sync.Mutex
	jobs      map[string]*job
	finished  []string
	runningID string
	// runningCancel cancels the running job's context. Only one job runs at a
	// time, so one cancel func is the whole set.
	runningCancel context.CancelFunc
	// runningWork is closed when the last started job's fn actually returned.
	// The slot is freed earlier than that on the cancel and timeout paths, so
	// this is what a shutdown has to wait on.
	runningWork <-chan struct{}
}

func NewStore() *Store {
	return &Store{jobs: map[string]*job{}}
}

// Start runs fn in a new goroutine and returns its job ID, or ErrBusy if one is
// already running. At timeout the job fails and the slot is freed even if fn
// ignores its context.
func (s *Store) Start(action string, timeout time.Duration, fn Func) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	s.mu.Lock()
	if s.runningID != "" {
		s.mu.Unlock()
		cancel()
		return "", ErrBusy
	}
	id := newID()
	j := &job{job: Job{ID: id, Action: action, Status: StatusRunning, StartedAt: time.Now()}}
	work := make(chan struct{})
	s.jobs[id] = j
	s.runningID = id
	s.runningCancel, s.runningWork = cancel, work
	s.mu.Unlock()

	go func() {
		defer cancel()
		type result struct {
			code int
			err  error
		}
		done := make(chan result, 1)
		go func() {
			// Registered first, so it runs last: whatever else happens, the
			// close marks fn as no longer touching the machine.
			defer close(work)
			// The kit is a long-lived desktop helper, so a panic in one action
			// must fail that job, not take the process down with it.
			defer func() {
				if r := recover(); r != nil {
					done <- result{-1, fmt.Errorf("%s panicked: %v", action, r)}
				}
			}()
			code, err := fn(ctx, j.appendLine)
			done <- result{code, err}
		}()

		var code int
		var jerr error
		select {
		case r := <-done:
			code, jerr = r.code, r.err
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				j.appendLine("canceled")
				code, jerr = -1, fmt.Errorf("%s canceled: %w", action, ctx.Err())
			} else {
				j.appendLine(fmt.Sprintf("timed out after %s", timeout))
				code, jerr = -1, fmt.Errorf("%s timed out after %s", action, timeout)
			}
		}
		// Free the slot before recording the verdict: a poller that sees a
		// terminal status must be able to start the next job at once.
		s.release(id)
		j.finish(code, jerr)
	}()

	return id, nil
}

// Hold takes the running slot for a caller that does the work itself on its
// own goroutine, and returns the func that finishes the job and frees the
// slot. Unlike Start there is no deadline, deliberately: Start frees the slot
// at its timeout even while fn runs on, which is right for a wedged claude
// child but wrong for the synchronous file writers - freeing the slot under
// one of them lets a claude job rewrite the same file, which is the reason
// they take the slot at all. They are a read, a render and a rename, with no
// subprocess to wedge on.
//
// The returned func is idempotent, so a caller can defer it and still report
// its own error. CancelAll cannot reach a held job: there is no context to
// cancel, and a shutdown waits it out through WaitIdle instead.
func (s *Store) Hold(action string) (release func(err error), err error) {
	s.mu.Lock()
	if s.runningID != "" {
		s.mu.Unlock()
		return nil, ErrBusy
	}
	id := newID()
	j := &job{job: Job{ID: id, Action: action, Status: StatusRunning, StartedAt: time.Now()}}
	work := make(chan struct{})
	s.jobs[id] = j
	s.runningID = id
	s.runningCancel, s.runningWork = nil, work
	s.mu.Unlock()

	var once sync.Once
	return func(err error) {
		once.Do(func() {
			code := 0
			if err != nil {
				code = -1
			}
			s.release(id)
			j.finish(code, err)
			close(work)
		})
	}, nil
}

// Running names the job holding the slot, if any. A page that reloaded while
// a ten-minute install was going has no record of it in memory, and would
// otherwise show "nothing yet" while every action it offers 409s.
func (s *Store) Running() (id, action string, ok bool) {
	s.mu.Lock()
	id = s.runningID
	j := s.jobs[id]
	s.mu.Unlock()
	if id == "" || j == nil {
		return "", "", false
	}
	j.mu.Lock()
	action = j.job.Action
	j.mu.Unlock()
	return id, action, true
}

// release clears the running slot and evicts the oldest finished jobs.
func (s *Store) release(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runningID == id {
		s.runningID, s.runningCancel = "", nil
	}
	s.finished = append(s.finished, id)
	for len(s.finished) > maxRetainedJobs {
		delete(s.jobs, s.finished[0])
		s.finished = append(s.finished[:0], s.finished[1:]...)
	}
}

// CancelAll cancels every running job's context, so a shutdown does not leave
// a `claude plugin install` running behind a closed UI. Children run in their
// own process group, so a Ctrl-C in the launching terminal never reaches them.
func (s *Store) CancelAll() {
	s.mu.Lock()
	cancel := s.runningCancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// WaitIdle blocks until the last job's fn has returned or timeout elapses, and
// reports whether it returned. It waits on the work rather than on the running
// slot: a cancelled job frees the slot at once, while its claude child is still
// being killed, and a shutdown that stopped there would outlive nothing.
func (s *Store) WaitIdle(timeout time.Duration) bool {
	s.mu.Lock()
	work := s.runningWork
	s.mu.Unlock()
	if work == nil {
		return true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-work:
		return true
	case <-timer.C:
		return false
	}
}

// Snapshot returns the job's current state and any log lines with seq > since.
func (s *Store) Snapshot(id string, since int) (Snapshot, bool) {
	s.mu.Lock()
	j, ok := s.jobs[id]
	s.mu.Unlock()
	if !ok {
		return Snapshot{}, false
	}
	return j.snapshot(since), true
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
