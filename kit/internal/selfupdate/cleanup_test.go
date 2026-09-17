package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanup(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "rise-x-kit")
	old := exe + ".old"
	if err := os.WriteFile(old, []byte("previous"), 0o755); err != nil {
		t.Fatalf("write old: %v", err)
	}

	Cleanup(exe)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("%s still exists", old)
	}
	// A second call on a clean directory must stay quiet.
	Cleanup(exe)
}
