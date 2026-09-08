//go:build windows

package runner

import (
	"os/exec"
	"strconv"
	"syscall"
)

// setProcAttr hides the console window that would otherwise flash up when we
// spawn claude.exe, and puts the child in its own process group so killTree
// can reach the whole tree.
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// killTree kills the child and its descendants. `claude` is a Node wrapper
// whose grandchildren inherit stdout, so killing only the direct child leaves
// them running and holding the pipes open.
func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := kill.Run(); err != nil {
		return cmd.Process.Kill() // taskkill missing or refused
	}
	return nil
}
