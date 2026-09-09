//go:build !windows

package fsutil

import "syscall"

// oNoFollow makes the open fail rather than follow a symlink planted at the
// backup's predictable name.
const oNoFollow = syscall.O_NOFOLLOW
