//go:build windows

package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApply_Windows_MovesAsideThenInstalls(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "rise-x-kit.exe")
	newPath := filepath.Join(dir, "rise-x-kit.exe.new")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Apply(newPath, exe); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _ := os.ReadFile(exe)
	old, _ := os.ReadFile(exe + ".old")
	if string(got) != "new" || string(old) != "old" {
		t.Fatalf("exe=%q old=%q", got, old)
	}
	Cleanup(exe)
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Fatalf(".old still present: %v", err)
	}
}

// When the new binary cannot be moved in, the old one is put back.
func TestApply_Windows_RollsBackWhenInstallFails(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "rise-x-kit.exe")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "does-not-exist.new")
	if err := Apply(missing, exe); err == nil {
		t.Fatal("expected an error")
	}
	got, err := os.ReadFile(exe)
	if err != nil || string(got) != "old" {
		t.Fatalf("exe after rollback = %q (%v), want the old build back", got, err)
	}
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Fatalf(".old left behind after rollback: %v", err)
	}
}
