//go:build !windows

package instance

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory lock on an already-open file. The
// kernel drops it when the process dies, however it dies, so a killed kit
// leaves nothing for the next launch to time out on.
func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
