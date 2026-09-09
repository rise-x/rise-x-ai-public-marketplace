//go:build windows

package runner

import (
	"os/exec"
	"strconv"
	"syscall"
)

// setProcAttr hides the console window that would otherwise flash up when we
// spawn claude.exe, and puts the child in its own process group so a Ctrl-C
// in a launching console is not delivered to it. It does NOT make killTree
// reach the tree: taskkill /T walks the parent-pid tree from a live snapshot
// and never looks at the process group. A job object is what would make the
// kill reliable here, and this does not have one yet.
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// killTree kills the child and its descendants, by pid. `claude` is a Node
// wrapper whose grandchildren inherit stdout, so killing only the direct child
// leaves them running and holding the pipes open. Once the direct child has
// exited, taskkill /T has no tree left to walk and the grandchild survives:
// see killAfterReap.
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

// killAfterReap is false on Windows. killTree names the child by pid, and once
// cmd.Wait has returned the process handle is released, so that pid may
// already belong to something else; taskkill /T would then force-kill an
// unrelated process and everything under it. Leaking a descendant that holds
// the pipes is the lesser harm, and the real fix is a job object with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, which needs a Windows runner to be
// verifiable at all.
const killAfterReap = false
