//go:build windows

package fsutil

import (
	"errors"
	"os"
	"syscall"
)

// oNoFollow has no Windows equivalent in syscall. O_EXCL carries the weight
// there: CreateFile with CREATE_NEW refuses a name that already exists,
// reparse points included, so a planted link is rejected rather than followed.
const oNoFollow = 0

// openNoFollow opens path for reading and refuses a reparse point - Windows's
// symlinks, junctions and mount points - at its last element. Without
// FILE_FLAG_OPEN_REPARSE_POINT the open resolves the link and hands back the
// target, which is how a planted link made LocalHEAD report an unrelated
// file's first line as a commit.
//
// The check is on the opened handle rather than the path, so nothing can be
// swapped in between the two.
func openNoFollow(path string) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(h), path)

	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		f.Close()
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	if info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		f.Close()
		return nil, &os.PathError{Op: "open", Path: path, Err: errSymlink}
	}
	return f, nil
}

// errSymlink is what a refused reparse point reports, so a caller can tell it
// apart from the file simply not being there.
var errSymlink = errors.New("refusing to follow a symlink")
