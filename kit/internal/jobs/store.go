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
	mu   sync.Mutex
	job  Job
	log  []LogLine
	done bool
}

func (j *job) appendLine(text string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.done {
		return // fn may outlive its timeout; its late output is not the job's
	}
	j.log = append(j.log, LogLine{Seq: len(j.log) + 1, Text: text})
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
	s.jobs[id] = j
	s.runningID = id
	s.runningCancel = cancel
	s.mu.Unlock()

	go func() {
		defer cancel()
		type result struct {
			code int
			err  error
		}
		done := make(chan result, 1)
		go func() {
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

// WaitIdle blocks until no job is running or timeout elapses, and reports
// whether the store went idle.
func (s *Store) WaitIdle(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		idle := s.runningID == ""
		s.mu.Unlock()
		if idle {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(2 * time.Millisecond)
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
