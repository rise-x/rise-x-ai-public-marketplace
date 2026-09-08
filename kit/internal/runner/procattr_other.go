//go:build !windows

package runner

import (
	"os/exec"
	"syscall"
)

// setProcAttr puts the child in its own process group, so killTree can reach
// the whole tree.
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killTree kills the child's process group. `claude` is a Node wrapper whose
// grandchildren inherit stdout, so killing only the direct child leaves them
// running and holding the pipes open.
func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill() // no process group, e.g. Setpgid failed
	}
	return nil
}
