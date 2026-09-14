//go:build !windows

package selfupdate

import (
	"os"
	"os/exec"
	"syscall"
)

// Apply puts newPath in place of exePath. Unix lets a running binary's file be
// replaced, so one rename does it.
func Apply(newPath, exePath string) error {
	return os.Rename(newPath, exePath)
}

// Relaunch starts exePath detached, so it outlives the process that is about
// to exit. Its own session keeps it off this process's terminal and out of
// reach of a Ctrl-C delivered here.
func Relaunch(exePath string, args []string) error {
	cmd := exec.Command(exePath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
