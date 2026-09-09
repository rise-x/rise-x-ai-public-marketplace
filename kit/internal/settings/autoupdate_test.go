package settings

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const repo = "rise-x/rise-x-ai-public-marketplace"

func write(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// readAutoUpdate and readEnvBlock keep the old one-call shape for the tests;
// production reads the whole file once via Read.
func readAutoUpdate(t *testing.T, path, name string) (bool, bool, error) {
	t.Helper()
	s, err := Read(path)
	if err != nil {
		return false, false, err
	}
	return s.AutoUpdate(name)
}

func readEnvBlock(t *testing.T, path string) (map[string]string, error) {
	t.Helper()
	s, err := Read(path)
	if err != nil {
		return nil, err
	}
	return s.Env()
}

func TestReadAutoUpdate_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{"foo":"bar"}`, 0o644)
	_, present, err := readAutoUpdate(t, path, "rise-x-public")
	if err != nil {
		t.Fatalf("ReadAutoUpdate: %v", err)
	}
	if present {
		t.Fatal("expected present=false")
	}
}

// The state every partner is in after `claude plugin marketplace add`: the
// entry exists, the autoUpdate key does not. That must read as "not set", not
// as "disabled".
func TestReadAutoUpdate_EntryWithoutAutoUpdateKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{"extraKnownMarketplaces":{"rise-x-public":{"source":{"source":"github","repo":"x"}}}}`, 0o644)
	enabled, present, err := readAutoUpdate(t, path, "rise-x-public")
	if err != nil {
		t.Fatalf("ReadAutoUpdate: %v", err)
	}
	if present || enabled {
		t.Fatalf("present=%v enabled=%v, want false/false", present, enabled)
	}
}

func TestReadAutoUpdate_Present(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{"extraKnownMarketplaces":{"rise-x-public":{"source":{"source":"github","repo":"x"},"autoUpdate":true}}}`, 0o644)
	enabled, present, err := readAutoUpdate(t, path, "rise-x-public")
	if err != nil {
		t.Fatalf("ReadAutoUpdate: %v", err)
	}
	if !present || !enabled {
		t.Fatalf("present=%v enabled=%v, want true/true", present, enabled)
	}
}

func TestReadAutoUpdate_PresentFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{"extraKnownMarketplaces":{"rise-x-public":{"autoUpdate":false}}}`, 0o644)
	enabled, present, err := readAutoUpdate(t, path, "rise-x-public")
	if err != nil || !present || enabled {
		t.Fatalf("enabled=%v present=%v err=%v, want false/true/nil", enabled, present, err)
	}
}

// Byte-for-byte: every key the kit does not own keeps its place, its content
// and its escaping, and only extraKnownMarketplaces is different.
func TestSetAutoUpdate_KeepsOrderIndentAndEscaping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	body := `{
  "permissions": {
    "allow": [
      "Bash(ls)"
    ]
  },
  "hooks": {
    "Stop": [
      {
        "command": "echo done && tail -1 log > out"
      }
    ]
  },
  "statusLine": {
    "command": "a < b"
  },
  "theme": "dark"
}
`
	write(t, path, body, 0o644)
	if _, err := NewWriter(path).SetAutoUpdate("rise-x-public", repo, true); err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	want := strings.TrimSuffix(body, "}\n") + `  "extraKnownMarketplaces": {
    "rise-x-public": {
      "source": {
        "source": "github",
        "repo": "` + repo + `"
      },
      "autoUpdate": true
    }
  }
}
`
	// The trailing "theme" line needs its comma now that a key follows it.
	want = strings.Replace(want, `"theme": "dark"`+"\n", `"theme": "dark",`+"\n", 1)
	if string(got) != want {
		t.Fatalf("output mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
	for _, escaped := range []string{`\u0026`, `\u003c`, `\u003e`} {
		if bytes.Contains(got, []byte(escaped)) {
			t.Fatalf("HTML escaping leaked into the file: %s", escaped)
		}
	}
}

func TestSetAutoUpdate_PreservesUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	body := `{
  "permissions": {"allow": ["Bash(ls)"]},
  "hooks": {"SessionStart": [{"command": "echo hi"}]}
}`
	write(t, path, body, 0o644)

	w := NewWriter(path)
	if _, err := w.SetAutoUpdate("rise-x-public", repo, true); err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]any
	if err := json.Unmarshal(got, &top); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	perms, ok := top["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions key lost: %+v", top)
	}
	allow, ok := perms["allow"].([]any)
	if !ok || len(allow) != 1 || allow[0] != "Bash(ls)" {
		t.Fatalf("permissions.allow mangled: %+v", perms)
	}
	if _, ok := top["hooks"]; !ok {
		t.Fatalf("hooks key lost: %+v", top)
	}

	ekm, ok := top["extraKnownMarketplaces"].(map[string]any)
	if !ok {
		t.Fatalf("extraKnownMarketplaces missing: %+v", top)
	}
	entry, ok := ekm["rise-x-public"].(map[string]any)
	if !ok || entry["autoUpdate"] != true {
		t.Fatalf("entry not created correctly: %+v", ekm)
	}
}

// The kit owns one key inside the entry; anything else Claude Code writes
// there stays, and an existing source is not rewritten.
func TestSetAutoUpdate_MergesIntoExistingEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{"extraKnownMarketplaces":{"rise-x-public":{"source":{"source":"github","repo":"someone/else"},"lastFetched":"2026-01-01","trusted":true}}}`, 0o644)

	if _, err := NewWriter(path).SetAutoUpdate("rise-x-public", repo, true); err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var top struct {
		EKM map[string]map[string]any `json:"extraKnownMarketplaces"`
	}
	if err := json.Unmarshal(got, &top); err != nil {
		t.Fatal(err)
	}
	entry := top.EKM["rise-x-public"]
	if entry["autoUpdate"] != true {
		t.Fatalf("autoUpdate not set: %+v", entry)
	}
	if entry["lastFetched"] != "2026-01-01" || entry["trusted"] != true {
		t.Fatalf("sibling keys dropped: %+v", entry)
	}
	source, _ := entry["source"].(map[string]any)
	if source["repo"] != "someone/else" {
		t.Fatalf("existing source was rewritten: %+v", source)
	}
}

func TestSetAutoUpdate_AddsSourceWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{"extraKnownMarketplaces":{"rise-x-public":{"trusted":true}}}`, 0o644)
	if _, err := NewWriter(path).SetAutoUpdate("rise-x-public", repo, true); err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Contains(got, []byte(repo)) {
		t.Fatalf("source not added: %s", got)
	}
}

// settings.json can hold an apiKeyHelper and an env block; a 0600 file must
// stay 0600, and so must its backup.
func TestSetAutoUpdate_PreservesFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{"theme":"dark"}`, 0o600)

	backup, err := NewWriter(path).SetAutoUpdate("rise-x-public", repo, true)
	if err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("settings.json mode = %v, want 0600", fi.Mode().Perm())
	}
	bfi, err := os.Stat(backup)
	if err != nil {
		t.Fatal(err)
	}
	if bfi.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode = %v, want 0600", bfi.Mode().Perm())
	}
}

func TestSetAutoUpdate_NewFileIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if _, err := NewWriter(path).SetAutoUpdate("rise-x-public", repo, true); err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("new settings.json mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestSetAutoUpdate_CreatesWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{}`, 0o644)
	w := NewWriter(path)
	if _, err := w.SetAutoUpdate("rise-x-public", repo, false); err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	enabled, present, err := readAutoUpdate(t, path, "rise-x-public")
	if err != nil || !present || enabled {
		t.Fatalf("enabled=%v present=%v err=%v, want false/true/nil", enabled, present, err)
	}
}

func TestSetAutoUpdate_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	w := NewWriter(path)
	if _, err := w.SetAutoUpdate("rise-x-public", repo, true); err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected settings.json to be created: %v", err)
	}
}

// --claude-dir can point at a directory that doesn't exist yet.
func TestSetAutoUpdate_CreatesMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh", "settings.json")
	if _, err := NewWriter(path).SetAutoUpdate("rise-x-public", repo, true); err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected settings.json to be created: %v", err)
	}
}

// A zero-byte file is a plausible state after a crashed editor, not a corrupt
// one; it reads as an empty settings object.
func TestSetAutoUpdate_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, "   \n\t\n", 0o600)
	if _, err := NewWriter(path).SetAutoUpdate("rise-x-public", repo, true); err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	if _, present, _ := readAutoUpdate(t, path, "rise-x-public"); !present {
		t.Fatal("expected the entry to be written")
	}
}

func TestSetAutoUpdate_PreservesBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"theme":"dark"}`)...), 0o600); err != nil {
		t.Fatal(err)
	}
	backup, err := NewWriter(path).SetAutoUpdate("rise-x-public", repo, true)
	if err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(got, utf8BOM) {
		t.Fatal("BOM was not preserved")
	}
	backupBody, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(backupBody, utf8BOM) {
		t.Fatal("the backup is not a byte copy: it lost the BOM")
	}
}

func TestSetAutoUpdate_PreservesCRLF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{\r\n  \"theme\": \"dark\"\r\n}\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWriter(path).SetAutoUpdate("rise-x-public", repo, true); err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("\r\n")) {
		t.Fatalf("CRLF was not preserved: %q", got)
	}
	if bytes.Contains(bytes.ReplaceAll(got, []byte("\r\n"), nil), []byte("\n")) {
		t.Fatalf("mixed line endings: %q", got)
	}
}

// A dotfiles setup symlinks settings.json; the link must survive and the real
// file must receive the change.
func TestSetAutoUpdate_FollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles.json")
	link := filepath.Join(dir, "settings.json")
	write(t, real, `{"theme":"dark"}`, 0o600)
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := NewWriter(link).SetAutoUpdate("rise-x-public", repo, true); err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced by a regular file")
	}
	if _, present, _ := readAutoUpdate(t, real, "rise-x-public"); !present {
		t.Fatal("the real file never got the change")
	}
}

func TestSetAutoUpdate_RefusesInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	body := `{
  // a comment, which is not valid JSON
  "foo": "bar"
}`
	write(t, path, body, 0o644)
	w := NewWriter(path)
	if _, err := w.SetAutoUpdate("rise-x-public", repo, true); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatal("file was modified despite being refused")
	}
}

func TestSetAutoUpdate_RefusesNonObjectMarketplaces(t *testing.T) {
	for _, v := range []string{`"oops"`, `[]`, `5`} {
		path := filepath.Join(t.TempDir(), "settings.json")
		body := `{"extraKnownMarketplaces":` + v + `,"theme":"dark"}`
		write(t, path, body, 0o644)
		if _, err := NewWriter(path).SetAutoUpdate("rise-x-public", repo, true); err == nil {
			t.Fatalf("extraKnownMarketplaces=%s: expected an error", v)
		}
		got, _ := os.ReadFile(path)
		if string(got) != body {
			t.Fatalf("extraKnownMarketplaces=%s: file was modified", v)
		}
	}
}

func TestSetAutoUpdate_OneBackupPerProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{"v":1}`, 0o644)
	w := NewWriter(path)

	backup1, err := w.SetAutoUpdate("rise-x-public", repo, true)
	if err != nil {
		t.Fatalf("first SetAutoUpdate: %v", err)
	}
	if backup1 == "" {
		t.Fatal("expected a backup path on first write")
	}
	backupBody, err := os.ReadFile(backup1)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(backupBody) != `{"v":1}` {
		t.Fatalf("backup content = %q, want original", backupBody)
	}

	backup2, err := w.SetAutoUpdate("rise-x-public", repo, false)
	if err != nil {
		t.Fatalf("second SetAutoUpdate: %v", err)
	}
	if backup2 != "" {
		t.Fatalf("expected no second backup, got %q", backup2)
	}

	// Second write took effect (unchecking writes false).
	enabled, _, _ := readAutoUpdate(t, path, "rise-x-public")
	if enabled {
		t.Fatal("expected autoUpdate=false after second write")
	}
}

func TestReadEnvBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{"env":{"DISABLE_AUTOUPDATER":"1"}}`, 0o644)
	env, err := readEnvBlock(t, path)
	if err != nil {
		t.Fatalf("ReadEnvBlock: %v", err)
	}
	if env["DISABLE_AUTOUPDATER"] != "1" {
		t.Fatalf("env = %+v", env)
	}
}

func TestReadEnvBlock_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{}`, 0o644)
	env, err := readEnvBlock(t, path)
	if err != nil || len(env) != 0 {
		t.Fatalf("env=%+v err=%v, want empty/nil", env, err)
	}
}

func TestSetAutoUpdate_NoTmpFileLeftBehind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{}`, 0o644)
	w := NewWriter(path)
	if _, err := w.SetAutoUpdate("rise-x-public", repo, true); err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("leftover tmp file: %s", e.Name())
		}
	}
}

// A write that cannot land must not leave a copy of settings.json behind: the
// tmp file carries the same contents as the real one.
func TestWriteAtomic_CannotLand_RemovesTmp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "settings.json")
	if err := os.MkdirAll(filepath.Join(target, "in-the-way"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(target, []byte(`{}`), 0o600, nil); err == nil {
		t.Fatal("expected a write onto a non-empty directory to fail")
	}
	assertNoTmpFiles(t, dir)
}

// The temp file is named uniquely, so a fixed "settings.json.tmp" a partner (or
// an older kit) left behind is neither read nor overwritten.
func TestWriteAtomic_DoesNotUseTheFixedTmpName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	write(t, path, `{}`, 0o644)
	sentinel := path + ".tmp"
	write(t, sentinel, "not mine", 0o644)

	if _, err := NewWriter(path).SetAutoUpdate("rise-x-public", repo, true); err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	kept, err := os.ReadFile(sentinel)
	if err != nil || string(kept) != "not mine" {
		t.Fatalf("settings.json.tmp = %q, err = %v; the fixed name must not be used", kept, err)
	}
	assertNoTmpFiles(t, dir)
}

// Claude Code rewrites settings.json for its own reasons. A write that landed
// while the kit was editing must not be clobbered: the kit redoes its edit on
// top of the new file.
func TestSetAutoUpdate_ConcurrentWrite_RedoesTheEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	write(t, path, `{"model":"opus"}`, 0o644)

	w := NewWriter(path)
	writes := 0
	w.afterRead = func() {
		writes++
		if writes > 1 {
			return // only the first attempt is interrupted
		}
		write(t, path, `{"model":"opus","extraKnownMarketplaces":{"acme":{"autoUpdate":false}}}`, 0o644)
	}

	if _, err := w.SetAutoUpdate("rise-x-public", repo, true); err != nil {
		t.Fatalf("SetAutoUpdate: %v", err)
	}
	if writes != 2 {
		t.Fatalf("read the file %d times, want 2 (one redo)", writes)
	}

	var got map[string]json.RawMessage
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("settings.json = %s: %v", data, err)
	}
	var ekm map[string]map[string]any
	if err := json.Unmarshal(got["extraKnownMarketplaces"], &ekm); err != nil {
		t.Fatal(err)
	}
	if _, ok := ekm["acme"]; !ok {
		t.Fatalf("the concurrent write was clobbered: %s", data)
	}
	if ekm["rise-x-public"]["autoUpdate"] != true {
		t.Fatalf("autoUpdate did not land: %s", data)
	}
	assertNoTmpFiles(t, dir)
}

// A file that keeps moving is left alone, with an error the page can show.
func TestSetAutoUpdate_KeepsChanging_Fails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	write(t, path, `{"model":"opus"}`, 0o644)

	w := NewWriter(path)
	round := 0
	w.afterRead = func() {
		round++
		write(t, path, `{"model":"opus","round":`+strconv.Itoa(round)+`}`, 0o644)
	}

	if _, err := w.SetAutoUpdate("rise-x-public", repo, true); err == nil ||
		!strings.Contains(err.Error(), "another program is writing it") {
		t.Fatalf("err = %v, want a clear refusal", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "autoUpdate") {
		t.Fatalf("the kit wrote over the other program: %s", data)
	}
	assertNoTmpFiles(t, dir)
}

func assertNoTmpFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".settings-") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("leftover tmp file: %s", e.Name())
		}
	}
}
