// Package fsutil holds the small filesystem helpers more than one package
// needs.
package fsutil

import (
	"fmt"
	"time"
)

// BackupPath names the backup copy of path: "<path>.bak-<UTC timestamp>".
func BackupPath(path string) string {
	return fmt.Sprintf("%s.bak-%s", path, time.Now().UTC().Format("20060102T150405Z"))
}
