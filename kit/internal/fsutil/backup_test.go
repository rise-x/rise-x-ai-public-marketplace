package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

// The backup name is predictable, so a local attacker can plant a symlink on
// it. Following one would write the file's contents wherever the link points.
func TestBackup_RefusesAPlantedSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, ".npmrc")
	victim := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(victim, []byte("do not touch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	planted := BackupPath(src)
	if err := os.Symlink(victim, planted); err != nil {
		t.Skipf("cannot symlink here: %v", err)
	}

	name, err := Backup(src, []byte("secret\n"), 0o600)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if name == planted {
		t.Fatal("the backup used the planted name")
	}
	if got, _ := os.ReadFile(victim); string(got) != "do not touch\n" {
		t.Fatalf("wrote through the symlink: %q", got)
	}
}

// One-second granularity means two backups of the same file can collide; the
// second must not silently replace the first.
func TestBackup_DoesNotOverwriteAnEarlierBackup(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "settings.json")

	first, err := Backup(src, []byte("one\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Backup(src, []byte("two\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("both backups used the same name")
	}
	if got, _ := os.ReadFile(first); string(got) != "one\n" {
		t.Fatalf("the first backup was overwritten: %q", got)
	}
}

// The backup of a 0600 secrets file must not be readable by anyone else, and
// O_CREATE's perm argument is masked by the umask.
func TestBackup_KeepsTheSourceMode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, ".npmrc")
	name, err := Backup(src, []byte("_authToken=x\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("backup mode = %v, want 0600", perm)
	}
}

// An interrupted write must leave the original readable, not truncated.
func TestWriteAtomic_ReplacesAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new\n" {
		t.Fatalf("content = %q", got)
	}
	fi, _ := os.Stat(path)
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode = %v, want 0600", perm)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("left a temp file behind: %d entries", len(entries))
	}
}
