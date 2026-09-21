//go:build windows

package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// detachedProcess is CREATE_NEW_PROCESS_GROUP's companion from the Windows
// process-creation flags; syscall carries the latter but not this one.
const detachedProcess = 0x00000008

// Apply puts newPath in place of exePath. Windows refuses to overwrite a
// running exe but allows renaming it, so the old binary is moved aside first
// and moved back if the second rename fails.
func Apply(newPath, exePath string) error {
	old := exePath + ".old"
	os.Remove(old)
	if err := os.Rename(exePath, old); err != nil {
		return fmt.Errorf("move %s aside: %w", exePath, err)
	}
	if err := os.Rename(newPath, exePath); err != nil {
		if rollback := os.Rename(old, exePath); rollback != nil {
			return fmt.Errorf("install %s: %w (and %s left at %s)", exePath, err, exePath, old)
		}
		return fmt.Errorf("install %s: %w", exePath, err)
	}
	return nil
}

// Relaunch starts exePath detached, so it outlives the process that is about
// to exit and does not receive this console's Ctrl-C.
func Relaunch(exePath string, args []string) error {
	cmd := exec.Command(exePath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | syscall.CREATE_NEW_PROCESS_GROUP,
	}
	return cmd.Start()
}
