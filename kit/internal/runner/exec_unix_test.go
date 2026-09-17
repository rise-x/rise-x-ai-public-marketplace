//go:build !windows

package runner

import (
	"context"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Cancelling must kill the whole process group: the direct child exits at
// once, and only the sleeping grandchild is left holding stdout open.
func TestExecStream_Cancel_KillsGrandchild(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	pids := make(chan int, 1)
	start := time.Now()
	runStream(t, ctx, "grandchild", 30*time.Second, func(l Line) {
		if fields := strings.Fields(l.Text); len(fields) == 2 && fields[0] == "grandchild" {
			if pid, err := strconv.Atoi(fields[1]); err == nil {
				select {
				case pids <- pid:
				default:
				}
			}
		}
	})
	if elapsed := time.Since(start); elapsed >= waitDelay {
		t.Fatalf("Stream returned after %s: the pipes were closed by WaitDelay, not by the kill", elapsed)
	}

	var pid int
	select {
	case pid = <-pids:
	default:
		t.Fatal("the helper never reported its grandchild's pid")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); err != nil {
			return // the grandchild is gone
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("grandchild %d survived the cancel", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
