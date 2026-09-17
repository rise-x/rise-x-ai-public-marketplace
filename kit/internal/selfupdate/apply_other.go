//go:build !windows

package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/fsutil"
)

// Apply puts newPath in place of exePath. Unix lets a running binary's file be
// replaced, so one rename does it.
//
// The destination's directory entry is flushed after the rename, the way every
// other write in the kit is: without it the swap lives in the directory cache,
// and a machine that loses power here can come back with neither the old binary
// nor the new one. Past the rename the update is installed, so a failed flush
// is a durability warning rather than a failed update, and says so.
//
// The work directory newPath came out of is deliberately not flushed: losing
// its entry costs nothing, because Cleanup collects that residue at the next
// launch either way.
func Apply(newPath, exePath string) error {
	if err := os.Rename(newPath, exePath); err != nil {
		return err
	}
	if err := fsutil.SyncDir(filepath.Dir(exePath)); err != nil {
		return fmt.Errorf("%w: %v", fsutil.ErrNotDurable, err)
	}
	return nil
}

// Relaunch starts exePath detached, so it outlives the process that is about
// to exit. Its own session keeps it off this process's terminal and out of
// reach of a Ctrl-C delivered here.
func Relaunch(exePath string, args []string) error {
	cmd := exec.Command(exePath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
