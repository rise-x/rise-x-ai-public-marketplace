package claudemd

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/fsutil"
)

func write(t *testing.T, dir, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestApply_CreatesTheFile(t *testing.T) {
	dir := t.TempDir()
	res, err := Apply(dir)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Action != "added" || res.Backup != "" {
		t.Fatalf("res = %+v, want added with no backup", res)
	}
	if got := read(t, res.Path); !strings.Contains(got, "rise-x-kit") {
		t.Fatalf("file = %q", got)
	}
}

// Everything the partner wrote is theirs; only the block is the kit's.
func TestApply_KeepsThePartnersOwnInstructions(t *testing.T) {
	dir := t.TempDir()
	original := "# My rules\n\n- Always use tabs.\n"
	path := write(t, dir, original, 0o644)

	res, err := Apply(dir)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got := read(t, path)
	if !strings.HasPrefix(got, original) {
		t.Fatalf("the partner's text was not kept verbatim at the top:\n%q", got)
	}
	if !strings.Contains(got, startMarker) || !strings.Contains(got, endMarker) {
		t.Fatal("no block was added")
	}
	if res.Backup == "" {
		t.Fatal("an existing file must be backed up before the first change")
	}
	if backup := read(t, res.Backup); backup != original {
		t.Fatalf("backup = %q, want the original bytes", backup)
	}
}

// Running the installer twice must not stack two blocks.
func TestApply_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := Apply(dir); err != nil {
		t.Fatal(err)
	}
	first := read(t, Path(dir))

	res, err := Apply(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "unchanged" {
		t.Fatalf("second Apply = %q, want unchanged", res.Action)
	}
	if res.Backup != "" {
		t.Fatal("nothing changed, so nothing should have been backed up")
	}
	if got := read(t, Path(dir)); got != first {
		t.Fatal("the file moved on a no-op Apply")
	}
	if n := strings.Count(read(t, Path(dir)), startMarker); n != 1 {
		t.Fatalf("%d blocks, want 1", n)
	}
}

// A new kit version replaces the block in place, leaving both sides alone.
func TestApply_ReplacesAnOldBlockInPlace(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "top\n\n"+startMarker+"\nold text\n"+endMarker+"\n\nbottom\n", 0o644)

	res, err := Apply(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "updated" {
		t.Fatalf("Action = %q, want updated", res.Action)
	}
	got := read(t, path)
	if strings.Contains(got, "old text") {
		t.Fatal("the previous block survived")
	}
	if !strings.HasPrefix(got, "top\n\n") || !strings.HasSuffix(got, "\n\nbottom\n") {
		t.Fatalf("text around the block moved:\n%q", got)
	}
	if n := strings.Count(got, startMarker); n != 1 {
		t.Fatalf("%d blocks, want 1", n)
	}
}

func TestApply_KeepsFileMode(t *testing.T) {
	requirePOSIXModes(t)
	dir := t.TempDir()
	path := write(t, dir, "# mine\n", 0o600)
	if _, err := Apply(dir); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode = %v, want 0600", perm)
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "# mine\n\n- rule\n", 0o644)
	if _, err := Apply(dir); err != nil {
		t.Fatal(err)
	}

	res, err := Remove(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "removed" {
		t.Fatalf("Action = %q, want removed", res.Action)
	}
	got := read(t, path)
	if strings.Contains(got, "rise-x-kit") {
		t.Fatalf("the block survived: %q", got)
	}
	if !strings.Contains(got, "- rule") {
		t.Fatalf("the partner's text went with it: %q", got)
	}
}

func TestRemove_NothingToRemove(t *testing.T) {
	dir := t.TempDir()
	res, err := Remove(dir)
	if err != nil || res.Action != "unchanged" {
		t.Fatalf("Remove on a missing file = %+v, %v", res, err)
	}
	write(t, dir, "# mine\n", 0o644)
	res, err = Remove(dir)
	if err != nil || res.Action != "unchanged" || res.Backup != "" {
		t.Fatalf("Remove without a block = %+v, %v", res, err)
	}
}

// A half-deleted block must never cost the partner their own instructions. cut
// needs both markers, so a lone start once read as "no block": Apply appended a
// second one, and the next run spanned from the orphan to the new block's end.
func TestApply_MalformedMarkers_LeaveTheFileAlone(t *testing.T) {
	rules := "## IMPORTANT: never force push to main\n- always run the linter\n"
	cases := map[string]string{
		"lone start, rules after it":  "# My rules\n\n" + startMarker + "\n## Rise-X Kit\nold\n\n" + rules,
		"lone start, rules before it": "# My rules\n\n" + rules + "\n" + startMarker + "\n## Rise-X Kit\nold\n",
		"lone end":                    "# My rules\n\n" + rules + "\n" + endMarker + "\n",
		"two blocks":                  startMarker + "\nA\n" + endMarker + "\n\n" + rules + startMarker + "\nB\n" + endMarker + "\n",
		"end before start":            endMarker + "\n" + rules + startMarker + "\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := write(t, dir, body, 0o644)
			for pass := 1; pass <= 2; pass++ {
				if _, err := Apply(dir); !errors.Is(err, ErrMalformed) {
					t.Fatalf("pass %d: err = %v, want ErrMalformed", pass, err)
				}
			}
			if got := read(t, path); got != body {
				t.Fatalf("the file was modified:\n%q", got)
			}
			if _, err := Remove(dir); !errors.Is(err, ErrMalformed) {
				t.Fatalf("Remove err = %v, want ErrMalformed", err)
			}
		})
	}
}

// The backup must not be writable through a symlink somebody planted at the
// predictable name, and must not silently replace an earlier one.
func TestApply_BackupIsSafe(t *testing.T) {
	requirePOSIXModes(t)
	dir := t.TempDir()
	write(t, dir, "# mine\n", 0o600)
	victim := filepath.Join(dir, "VICTIM.txt")
	if err := os.WriteFile(victim, []byte("do not touch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	planted := fsutil.BackupPath(Path(dir))
	if err := os.Symlink(victim, planted); err != nil {
		t.Skipf("cannot symlink here: %v", err)
	}

	res, err := Apply(dir)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := read(t, victim); got != "do not touch\n" {
		t.Fatalf("the backup wrote through the planted symlink: %q", got)
	}
	if res.Backup == planted {
		t.Fatal("the backup reused the planted name")
	}
	if fi, err := os.Stat(res.Backup); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode = %v, %v; want 0600 like the source", fi.Mode().Perm(), err)
	}
}

// The same dotfiles case as ~/.npmrc: this file is a partner's own global
// instructions, and a rename over a symlink would strand their tracked copy.
func TestApply_KeepsASymlinkedClaudeMDLinked(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tracked := filepath.Join(dir, "dotfiles", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(tracked), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tracked, []byte("# my rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(claudeDir, "CLAUDE.md")
	if err := os.Symlink(tracked, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := Apply(claudeDir); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("Apply replaced the symlink with a plain file")
	}
	got, _ := os.ReadFile(tracked)
	if !strings.Contains(string(got), startMarker) || !strings.Contains(string(got), "# my rules") {
		t.Fatalf("tracked file = %q, want the partner's rules plus the block", got)
	}
}

// The backup is exactly what the partner needs when the write fails, and the
// error is all a caller keeps - main discards the Result - so it has to name
// it there, the way npmrc already does.
func TestWriteError_NamesTheBackup(t *testing.T) {
	got := writeError("/home/p/.claude/CLAUDE.md", "/home/p/.claude/CLAUDE.md.bak-x", os.ErrPermission)
	if !strings.Contains(got.Error(), "/home/p/.claude/CLAUDE.md.bak-x") {
		t.Fatalf("error = %q, want the backup path in it", got)
	}
	if !errors.Is(got, os.ErrPermission) {
		t.Fatalf("error = %q, want the cause still wrapped", got)
	}
	bare := writeError("/home/p/.claude/CLAUDE.md", "", os.ErrPermission)
	if strings.Contains(bare.Error(), "backed up") {
		t.Fatalf("error = %q, want no backup clause when there is no backup", bare)
	}
}

// requirePOSIXModes skips a test that asserts exact permission bits. Windows
// models only the read-only flag, so os.FileMode there is 0666 or 0444 and
// these assertions cannot hold; the behaviour they pin is a unix one.
func requirePOSIXModes(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not modelled on Windows")
	}
}
