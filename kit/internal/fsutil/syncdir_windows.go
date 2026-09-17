//go:build windows

package fsutil

// syncDirOS is a no-op on Windows: a directory handle opened this way cannot
// be flushed, and NTFS orders the metadata write itself.
func syncDirOS(string) error { return nil }
