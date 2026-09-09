package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const testTimeout = 30 * time.Second

func TestStore_ConcurrentStart_Busy(t *testing.T) {
	s := NewStore()
	release := make(chan struct{})

	id, err := s.Start("plugin.install", testTimeout, func(ctx context.Context, onLine func(string)) (int, error) {
		<-release
		return 0, nil
	})
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty job id")
	}

	if _, err := s.Start("plugin.install", testTimeout, func(context.Context, func(string)) (int, error) {
		return 0, nil
	}); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Start err = %v, want ErrBusy", err)
	}

	close(release)
	waitFinished(t, s, id)
}

func TestStore_SinceSlicing(t *testing.T) {
	s := NewStore()
	started := make(chan struct{})
	proceed := make(chan struct{})

	id, err := s.Start("npmrc.clean", testTimeout, func(ctx context.Context, onLine func(string)) (int, error) {
		onLine("line 1")
		onLine("line 2")
		close(started)
		<-proceed
		onLine("line 3")
		return 0, nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started

	snap, ok := s.Snapshot(id, 0)
	if !ok {
		t.Fatal("Snapshot: job not found")
	}
	if len(snap.Log) != 2 || snap.Log[0].Text != "line 1" || snap.Log[1].Text != "line 2" {
		t.Fatalf("unexpected log: %+v", snap.Log)
	}

	snap, ok = s.Snapshot(id, 2)
	if !ok || len(snap.Log) != 0 {
		t.Fatalf("since=2 should have no new lines, got %+v", snap.Log)
	}

	close(proceed)
	waitFinished(t, s, id)
	snap, _ = s.Snapshot(id, 2)
	if len(snap.Log) != 1 || snap.Log[0].Text != "line 3" || snap.Log[0].Seq != 3 {
		t.Fatalf("unexpected tail log: %+v", snap.Log)
	}
	if snap.Job.Status != StatusSucceeded {
		t.Fatalf("job status = %v, want succeeded", snap.Job.Status)
	}
	if snap.Job.FinishedAt == nil {
		t.Fatal("expected FinishedAt to be set")
	}
}

func TestStore_UnknownID(t *testing.T) {
	s := NewStore()
	if _, ok := s.Snapshot("nope", 0); ok {
		t.Fatal("expected ok=false for unknown id")
	}
}

func TestStore_FailedExitCode(t *testing.T) {
	s := NewStore()
	id, err := s.Start("plugin.install", testTimeout, func(context.Context, func(string)) (int, error) {
		return 1, nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFinished(t, s, id)
	snap, _ := s.Snapshot(id, 0)
	if snap.Job.Status != StatusFailed || snap.Job.ExitCode != 1 {
		t.Fatalf("job = %+v, want failed/1", snap.Job)
	}
}

// A job whose fn never returns must not wedge the store: the deadline fails
// the job, records why, and frees the slot for the next action.
func TestStore_Timeout_FailsJobAndFreesStore(t *testing.T) {
	s := NewStore()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	id, err := s.Start("plugin.install", 20*time.Millisecond, func(ctx context.Context, onLine func(string)) (int, error) {
		<-release // ignores ctx entirely, like a claude wedged on a full pipe
		return 0, nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitFinished(t, s, id)
	snap, _ := s.Snapshot(id, 0)
	if snap.Job.Status != StatusFailed {
		t.Fatalf("status = %v, want failed", snap.Job.Status)
	}
	if !strings.Contains(snap.Job.Error, "timed out") {
		t.Fatalf("error = %q, want a timeout message", snap.Job.Error)
	}

	// The store is free again.
	waitUntil(t, func() bool {
		next, err := s.Start("marketplace.update", testTimeout, func(context.Context, func(string)) (int, error) {
			return 0, nil
		})
		return err == nil && next != ""
	})
}

// fn's context carries the deadline, so a well-behaved job stops on its own.
func TestStore_Timeout_CancelsJobContext(t *testing.T) {
	s := NewStore()
	id, err := s.Start("marketplace.add", 20*time.Millisecond, func(ctx context.Context, onLine func(string)) (int, error) {
		<-ctx.Done()
		return -1, ctx.Err()
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFinished(t, s, id)
	if snap, _ := s.Snapshot(id, 0); snap.Job.Status != StatusFailed {
		t.Fatalf("status = %v, want failed", snap.Job.Status)
	}
}

func TestStore_EvictsOldestJobs(t *testing.T) {
	s := NewStore()
	var first string
	for i := 0; i < maxRetainedJobs+5; i++ {
		id, err := s.Start("noop", testTimeout, func(context.Context, func(string)) (int, error) {
			return 0, nil
		})
		if err != nil {
			t.Fatalf("Start %d: %v", i, err)
		}
		if i == 0 {
			first = id
		}
		waitFinished(t, s, id)
	}
	if _, ok := s.Snapshot(first, 0); ok {
		t.Fatal("the oldest job should have been evicted")
	}
	s.mu.Lock()
	retained := len(s.jobs)
	s.mu.Unlock()
	if retained > maxRetainedJobs {
		t.Fatalf("retained %d jobs, want at most %d", retained, maxRetainedJobs)
	}
}

// waitFinished relies on the store freeing the running slot before it records
// a terminal status, so a caller that sees one can start the next job.
func waitFinished(t *testing.T, s *Store, id string) {
	t.Helper()
	waitUntil(t, func() bool {
		snap, ok := s.Snapshot(id, 0)
		return ok && snap.Job.Status != StatusRunning
	})
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition never became true")
}

// A command that ignores its context keeps writing after the timeout; those
// lines must not land in a job that already reported a verdict.
func TestStart_TimeoutDropsLateLines(t *testing.T) {
	s := NewStore()
	release := make(chan struct{})
	id, err := s.Start("slow", 20*time.Millisecond, func(ctx context.Context, onLine func(string)) (int, error) {
		<-ctx.Done()
		<-release
		onLine("late line")
		return 0, nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFinished(t, s, id)
	close(release)

	waitUntil(t, func() bool {
		snap, _ := s.Snapshot(id, 0)
		return len(snap.Log) > 0 // the "timed out" line is always recorded
	})
	// Give the late append a moment to be (wrongly) recorded.
	time.Sleep(20 * time.Millisecond)
	snap, _ := s.Snapshot(id, 0)
	for _, l := range snap.Log {
		if l.Text == "late line" {
			t.Fatalf("a line appended after the timeout was kept: %+v", snap.Log)
		}
	}
}

// Shutdown must not leave a claude process running behind a closed UI: the
// job's context is cancelled, the job fails, and the slot is free again -
// even for an fn that ignores its context.
func TestStore_CancelAll_FailsRunningJobAndFreesStore(t *testing.T) {
	s := NewStore()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	started := make(chan struct{})

	id, err := s.Start("plugin.install", testTimeout, func(ctx context.Context, onLine func(string)) (int, error) {
		close(started)
		<-release // ignores ctx, like a claude wedged on a full pipe
		return 0, nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started

	s.CancelAll()
	// WaitIdle answers for the work, not the slot, so a job that ignores its
	// context holds it out: that false is what makes main log "still running"
	// instead of claiming a clean shutdown.
	if s.WaitIdle(50 * time.Millisecond) {
		t.Fatal("WaitIdle reported idle while the job body was still running")
	}

	waitFinished(t, s, id)
	snap, _ := s.Snapshot(id, 0)
	if snap.Job.Status != StatusFailed {
		t.Fatalf("status = %v, want failed", snap.Job.Status)
	}
	if !strings.Contains(snap.Job.Error, "canceled") {
		t.Fatalf("error = %q, want a cancellation message", snap.Job.Error)
	}

	next, err := s.Start("marketplace.update", testTimeout, func(context.Context, func(string)) (int, error) {
		return 0, nil
	})
	if err != nil || next == "" {
		t.Fatalf("Start after CancelAll = %q, %v; want a new job", next, err)
	}
	waitFinished(t, s, next)
}

// A well-behaved job sees the cancellation on its own context.
func TestStore_CancelAll_CancelsJobContext(t *testing.T) {
	s := NewStore()
	id, err := s.Start("marketplace.add", testTimeout, func(ctx context.Context, onLine func(string)) (int, error) {
		<-ctx.Done()
		return -1, ctx.Err()
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	s.CancelAll()
	// A job that respects its context is what lets a shutdown say it waited.
	if !s.WaitIdle(2 * time.Second) {
		t.Fatal("WaitIdle did not see the cancelled job's body return")
	}
	waitFinished(t, s, id)
	if snap, _ := s.Snapshot(id, 0); snap.Job.Status != StatusFailed {
		t.Fatalf("status = %v, want failed", snap.Job.Status)
	}
}

// WaitIdle must report the truth rather than block for its whole timeout.
func TestStore_WaitIdle_ReportsBusy(t *testing.T) {
	s := NewStore()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	id, err := s.Start("plugin.install", testTimeout, func(context.Context, func(string)) (int, error) {
		<-release
		return 0, nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s.WaitIdle(20 * time.Millisecond) {
		t.Fatal("WaitIdle = true while a job is running")
	}
	s.CancelAll()
	waitFinished(t, s, id)
}
