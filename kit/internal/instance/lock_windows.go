//go:build windows

package instance

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	procLockFile = kernel32.NewProc("LockFileEx")
)

const (
	lockfileExclusiveLock   = 0x00000002
	lockfileFailImmediately = 0x00000001
)

// lockFile takes an exclusive byte-range lock on the whole file. Windows
// releases it when the handle closes, which includes the process dying.
func lockFile(f *os.File) error {
	var overlapped syscall.Overlapped
	r, _, err := procLockFile.Call(f.Fd(),
		lockfileExclusiveLock|lockfileFailImmediately, 0, 1, 0,
		uintptr(unsafe.Pointer(&overlapped)))
	if r == 0 {
		return err
	}
	return nil
}

// heldByAnother separates "somebody else has it" from "this machine cannot be
// locked". LockFileEx with LOCKFILE_FAIL_IMMEDIATELY reports contention as
// ERROR_LOCK_VIOLATION; anything else is a filesystem or handle problem a
// launch must not read as "wait for the other kit".
func heldByAnother(err error) bool {
	return errors.Is(err, syscall.Errno(errorLockViolation)) || errors.Is(err, syscall.Errno(errorSharingViolation))
}

const (
	errorSharingViolation = 32
	errorLockViolation    = 33
)
