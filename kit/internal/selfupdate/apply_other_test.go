//go:build !windows

package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/fsutil"
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

// The rename installs the update; the directory flush only makes it durable.
// A failure past the rename therefore has to read as a warning, or the caller
// reports a successful update as failed and the partner reinstalls over a
// binary that is already the new one.
func TestApply_LateFlushFailureStillInstalled(t *testing.T) {
	real := fsutil.SyncDir
	fsutil.SyncDir = func(string) error { return errors.New("fsync: operation not supported") }
	t.Cleanup(func() { fsutil.SyncDir = real })

	dir := t.TempDir()
	exe := filepath.Join(dir, "rise-x-kit")
	newPath := exe + ".new"
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := Apply(newPath, exe)
	if !errors.Is(err, fsutil.ErrNotDurable) {
		t.Fatalf("err = %v, want ErrNotDurable", err)
	}
	got, rerr := os.ReadFile(exe)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != "new" {
		t.Fatalf("exe = %q, want the new bytes: the rename landed before the flush", got)
	}
}
