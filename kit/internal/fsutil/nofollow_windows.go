//go:build windows

package fsutil

// oNoFollow has no Windows equivalent in syscall. O_EXCL carries the weight
// there: CreateFile with CREATE_NEW refuses a name that already exists,
// reparse points included, so a planted link is rejected rather than followed.
const oNoFollow = 0
