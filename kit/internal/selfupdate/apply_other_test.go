//go:build !windows

package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApply_RenamesOverExistingBinary(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "rise-x-kit")
	newPath := exe + ".new"
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatalf("write exe: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o755); err != nil {
		t.Fatalf("write new: %v", err)
	}

	if err := Apply(newPath, exe); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read exe: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("exe = %q, want %q", got, "new")
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatalf("%s still exists", newPath)
	}
}

func TestRelaunch(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a process")
	}
	if err := Relaunch("/bin/sh", []string{"-c", "exit 0"}); err != nil {
		t.Fatalf("Relaunch: %v", err)
	}
}
