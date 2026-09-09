//go:build !windows

package fsutil

import "os"

// SyncDir flushes the directory entry a newly created or renamed file needs
// to survive a crash: fsync on the file itself does not cover the name.
func SyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
