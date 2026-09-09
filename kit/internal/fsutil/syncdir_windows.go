//go:build windows

package fsutil

// SyncDir is a no-op on Windows: a directory handle opened this way cannot
// be flushed, and NTFS orders the metadata write itself.
func SyncDir(string) error { return nil }
