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

// WriteAtomic renames over its target, so a caller that hands it a symlink's
// own path replaces the link. Resolve is what keeps a dotfiles setup intact,
// and this pins the pair together.
func TestResolve_WriteAtomicKeepsASymlinkedFileLinked(t *testing.T) {
	dir := t.TempDir()
	tracked := filepath.Join(dir, "dotfiles", "npmrc")
	if err := os.MkdirAll(filepath.Dir(tracked), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tracked, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, ".npmrc")
	if err := os.Symlink(tracked, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := WriteAtomic(Resolve(link), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the write replaced the symlink with a plain file")
	}
	if got, _ := os.ReadFile(tracked); string(got) != "new\n" {
		t.Fatalf("tracked file = %q, want the new content", got)
	}
}

// A link whose target does not exist yet still names where the write belongs;
// EvalSymlinks refuses the whole path in that case.
func TestResolve_DanglingLinkNamesItsTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "CLAUDE.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got := Resolve(link); got != target {
		t.Fatalf("Resolve = %q, want the link's target %q", got, target)
	}
}

func TestResolve_PlainPathIsItself(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain")
	if got := Resolve(path); got != path {
		t.Fatalf("Resolve = %q, want %q", got, path)
	}
}

// A ref file is trusted because of where it sits, so a link planted there
// must not hand back some other file's bytes.
func TestReadNoFollow_RefusesASymlink(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte("sk-do-not-leak\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "ref")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got, err := ReadNoFollow(link); err == nil {
		t.Fatalf("read through a planted symlink: %q", got)
	}

	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, []byte("abc123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadNoFollow(plain)
	if err != nil || string(got) != "abc123\n" {
		t.Fatalf("ReadNoFollow(plain) = %q, %v", got, err)
	}
}
