//go:build !windows

package instance

import (
	"errors"
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory lock on an already-open file. The
// kernel drops it when the process dies, however it dies, so a killed kit
// leaves nothing for the next launch to time out on.
func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// heldByAnother separates "somebody else has it" from "this machine cannot be
// locked". flock reports contention as EWOULDBLOCK; ENOLCK, EOPNOTSUPP and
// friends come from filesystems that do not implement locks, and a launch
// must not read those as "wait for the other kit".
func heldByAnother(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}
