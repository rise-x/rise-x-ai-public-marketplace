//go:build !windows

package fsutil

import (
	"os"
	"syscall"
)

// oNoFollow makes the open fail rather than follow a symlink planted at the
// backup's predictable name.
const oNoFollow = syscall.O_NOFOLLOW

// openNoFollow opens path for reading, refusing a symlink at its last
// element. O_NOFOLLOW does it in the open itself, so there is no window
// between deciding and reading.
func openNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|oNoFollow, 0)
}
