package selfupdate

import (
	"os"
	"path/filepath"
	"strings"
)

// Cleanup removes what a previous update left beside the binary: the copy a
// Windows update moved aside, and any work directory a run that died between
// Download and Apply never removed. Best effort: the files are only in the
// way of disk space, and a still-running predecessor can keep its own locked.
func Cleanup(exePath string) {
	os.Remove(exePath + ".old")
	entries, err := os.ReadDir(filepath.Dir(exePath))
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), workPrefix) {
			os.RemoveAll(filepath.Join(filepath.Dir(exePath), e.Name()))
		}
	}
}
