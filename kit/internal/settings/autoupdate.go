// Package settings reads and writes the one key the kit is allowed to touch in
// ~/.claude/settings.json: extraKnownMarketplaces.<name>.autoUpdate. Every
// other key keeps its value, position, escaping, and the file its BOM, line
// endings and permissions.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/fsutil"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// defaultMode is used only when settings.json does not exist yet: the file can
// hold an apiKeyHelper and an env block, so it starts owner-only.
const defaultMode os.FileMode = 0o600

// Source identifies where a marketplace was added from.
type Source struct {
	Source string `json:"source"`
	Repo   string `json:"repo"`
}

// Settings is settings.json's parsed top-level object. Read it once and pull
// every key from it: a gather needs both the autoUpdate flag and the env
// block, and re-reading the file could see two different versions of it.
type Settings struct {
	path string
	top  map[string]json.RawMessage
}

// Read parses path. A missing or empty file reads as empty settings, since a
// fresh Claude Code install may not have written one yet.
func Read(path string) (Settings, error) {
	raw, _, err := readFile(path)
	if err != nil {
		return Settings{path: path}, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return Settings{path: path}, fmt.Errorf("%s: invalid JSON: %w", path, err)
	}
	return Settings{path: path, top: top}, nil
}

// AutoUpdate reads extraKnownMarketplaces.<name>.autoUpdate. present is false
// when that key is absent - either because the marketplace has no entry yet,
// or because Claude Code wrote the entry without ever setting autoUpdate (the
// normal state after `claude plugin marketplace add`).
func (s Settings) AutoUpdate(name string) (enabled bool, present bool, err error) {
	ekm, err := extraKnownMarketplaces(s.top)
	if err != nil {
		return false, false, err
	}
	entryRaw, ok := ekm[name]
	if !ok {
		return false, false, nil
	}
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(entryRaw, &entry); err != nil {
		return false, false, fmt.Errorf("%s: extraKnownMarketplaces.%s: %w", s.path, name, err)
	}
	autoRaw, ok := entry["autoUpdate"]
	if !ok {
		return false, false, nil
	}
	if err := json.Unmarshal(autoRaw, &enabled); err != nil {
		return false, false, fmt.Errorf("%s: extraKnownMarketplaces.%s.autoUpdate: %w", s.path, name, err)
	}
	return enabled, true, nil
}

// Env reads the top-level "env" map (Claude Code lets partners set env vars
// there instead of in their shell profile). A missing key reads as empty.
func (s Settings) Env() (map[string]string, error) {
	envRaw, ok := s.top["env"]
	if !ok {
		return map[string]string{}, nil
	}
	var env map[string]string
	if err := json.Unmarshal(envRaw, &env); err != nil {
		return nil, fmt.Errorf("%s: env: %w", s.path, err)
	}
	return env, nil
}

func extraKnownMarketplaces(top map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	raw, ok := top["extraKnownMarketplaces"]
	if !ok {
		return map[string]json.RawMessage{}, nil
	}
	var ekm map[string]json.RawMessage
	if err := json.Unmarshal(raw, &ekm); err != nil {
		return nil, fmt.Errorf("extraKnownMarketplaces: %w", err)
	}
	if ekm == nil {
		ekm = map[string]json.RawMessage{}
	}
	return ekm, nil
}

// Writer edits ~/.claude/settings.json. It backs the file up once per
// process, on its first successful write, never again — the backup is meant
// to capture the pre-kit state, not every intermediate one.
type Writer struct {
	Path string

	// afterRead runs between reading the file and writing it back; only tests
	// set it, to stand in for Claude Code rewriting settings.json mid-edit.
	afterRead func()

	mu       sync.Mutex
	backedUp bool
}

func NewWriter(path string) *Writer {
	return &Writer{Path: path}
}

// errChanged means the file was rewritten while the kit was editing it, so
// installing the edit would drop whatever the other writer put there.
var errChanged = errors.New("settings.json changed while the kit was editing it")

// SetAutoUpdate creates or updates extraKnownMarketplaces.<name>, setting only
// autoUpdate and filling in source when the entry does not carry one and repo
// names a GitHub one. It refuses a settings file that isn't valid JSON, and
// writes atomically next to the file a symlink resolves to.
//
// Claude Code writes the same file for its own reasons - `claude plugin
// marketplace add` rewrites extraKnownMarketplaces - so this read-modify-write
// only installs its result while the file still looks exactly as it did when it
// was read. A file that moved under it is read and edited again once, and then
// the write is refused rather than clobbering the other writer.
func (w *Writer) SetAutoUpdate(name, repo string, enabled bool) (backupPath string, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	target := resolve(w.Path)
	for attempt := 0; attempt < 2; attempt++ {
		backupPath, err = w.setOnce(target, name, repo, enabled)
		if !errors.Is(err, errChanged) {
			return backupPath, err
		}
	}
	return "", fmt.Errorf("refusing to modify %s: another program is writing it; try again", w.Path)
}

// setOnce is one read-modify-write attempt. It returns errChanged when the
// file moved between the read and the write.
func (w *Writer) setOnce(target, name, repo string, enabled bool) (backupPath string, err error) {
	before := statStamp(target)
	original, exists, err := readTarget(target)
	if err != nil {
		return "", err
	}
	read := statStamp(target)
	if read != before {
		return "", errChanged // the bytes just read may be half of two versions
	}
	if w.afterRead != nil {
		w.afterRead()
	}

	out, err := w.edit(original, name, repo, enabled)
	if err != nil {
		return "", err
	}

	mode := defaultMode
	if fi, serr := os.Stat(target); serr == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", err
	}
	if exists && !w.backedUp {
		if backupPath, err = backup(target, original, mode); err != nil {
			return "", err
		}
		w.backedUp = true
	}
	if err := writeAtomic(target, out, mode, read); err != nil {
		return "", err
	}
	return backupPath, nil
}

// edit renders original with only extraKnownMarketplaces.<name>.autoUpdate
// changed, keeping every other key's value, position and escaping, and the
// file's own BOM and line endings.
func (w *Writer) edit(original []byte, name, repo string, enabled bool) ([]byte, error) {
	body := original
	hadBOM := bytes.HasPrefix(body, utf8BOM)
	if hadBOM {
		body = body[len(utf8BOM):]
	}
	crlf := bytes.Contains(body, []byte("\r\n"))
	if len(bytes.TrimSpace(body)) == 0 {
		body = []byte("{}") // a missing or empty file is an empty settings object
	}

	top, err := decodeObject(body)
	if err != nil {
		return nil, fmt.Errorf("refusing to modify %s: invalid JSON: %w", w.Path, err)
	}

	ekm := newObject()
	if raw, ok := top.get("extraKnownMarketplaces"); ok && !isNull(raw) {
		if ekm, err = decodeObject(raw); err != nil {
			return nil, fmt.Errorf("refusing to modify %s: extraKnownMarketplaces: %w", w.Path, err)
		}
	}

	entry := newObject()
	if raw, ok := ekm.get(name); ok && !isNull(raw) {
		if entry, err = decodeObject(raw); err != nil {
			return nil, fmt.Errorf("refusing to modify %s: extraKnownMarketplaces.%s: %w", w.Path, name, err)
		}
	}
	if _, ok := entry.get("source"); !ok && repo != "" {
		source, err := encodeJSON(Source{Source: "github", Repo: repo})
		if err != nil {
			return nil, err
		}
		entry.set("source", source)
	}
	autoUpdate, err := encodeJSON(enabled)
	if err != nil {
		return nil, err
	}
	entry.set("autoUpdate", autoUpdate)

	entryBytes, err := entry.marshal()
	if err != nil {
		return nil, err
	}
	ekm.set(name, entryBytes)
	ekmBytes, err := ekm.marshal()
	if err != nil {
		return nil, err
	}
	top.set("extraKnownMarketplaces", ekmBytes)

	out, err := top.indent()
	if err != nil {
		return nil, err
	}
	if crlf {
		out = bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
	}
	if hadBOM {
		out = append(append([]byte{}, utf8BOM...), out...)
	}
	return out, nil
}

// readTarget reads the file as-is, reporting whether it exists at all; a
// missing file is an empty settings object, not an error.
func readTarget(target string) (data []byte, exists bool, err error) {
	data, err = os.ReadFile(target)
	if err == nil {
		return data, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return nil, false, err
}

// stamp is a file's size and modification time, the pair used to notice
// another program writing settings.json. Compared only, never shown.
type stamp struct {
	exists bool
	size   int64
	mtime  int64
}

func statStamp(path string) stamp {
	fi, err := os.Stat(path)
	if err != nil {
		return stamp{}
	}
	return stamp{exists: true, size: fi.Size(), mtime: fi.ModTime().UnixNano()}
}

// resolve follows a symlinked settings.json to the real file, so a dotfiles
// setup keeps its link and the change lands in the repo the partner tracks.
func resolve(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

func isNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// backup copies the file's exact bytes - BOM, line endings and all - at the
// same permissions, so a 0600 secrets file never leaves a world-readable copy.
func backup(target string, original []byte, mode os.FileMode) (string, error) {
	path := fsutil.BackupPath(target)
	if err := os.WriteFile(path, original, mode); err != nil {
		return "", fmt.Errorf("backup %s: %w", target, err)
	}
	return path, nil
}

// readFile reads path, stripping (and reporting) a leading UTF-8 BOM. A
// missing or empty file reads as an empty JSON object, since a fresh Claude
// Code install may not have written settings.json yet.
func readFile(path string) (data []byte, hadBOM bool, err error) {
	data, err = os.ReadFile(path)
	if os.IsNotExist(err) {
		return []byte("{}"), false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if bytes.HasPrefix(data, utf8BOM) {
		data, hadBOM = data[len(utf8BOM):], true
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return []byte("{}"), hadBOM, nil
	}
	return data, hadBOM, nil
}

// writeAtomic installs data at path via a uniquely named temp file in the same
// directory, but only while path still matches expect: a rename over a file
// another program has just rewritten would silently drop its change.
func writeAtomic(path string, data []byte, mode os.FileMode, expect stamp) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() {
		// Removing a file already renamed away is a no-op; this only ever
		// cleans up a copy of settings.json left behind by a failure.
		tmp.Close()
		_ = os.Remove(name)
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp makes the file 0600; settings.json may be more permissive.
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	if statStamp(path) != expect {
		return errChanged
	}
	return os.Rename(name, path)
}
