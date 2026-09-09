package claudemd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
