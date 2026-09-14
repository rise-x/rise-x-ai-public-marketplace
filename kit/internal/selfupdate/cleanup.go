package selfupdate

import "os"

// Cleanup removes the binary a previous Windows update moved aside. Best
// effort: the file is only in the way of disk space, and a still-running
// predecessor can keep it locked.
func Cleanup(exePath string) {
	os.Remove(exePath + ".old")
}
