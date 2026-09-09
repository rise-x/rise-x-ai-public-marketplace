// Package claudemd maintains the kit's block in the user's global
// ~/.claude/CLAUDE.md, so a Claude Code session knows how to open Rise-X Kit
// when a partner asks for it. Everything outside the two markers is the
// partner's and is copied through byte for byte.
package claudemd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/fsutil"
)

// The block's fences. Anything between them belongs to the kit and is
// replaced wholesale; a partner who deletes both markers keeps their edit.
const (
	startMarker = "<!-- rise-x-kit:start -->"
	endMarker   = "<!-- rise-x-kit:end -->"
)

// Block is what the kit writes. It is short on purpose: this file is read into
// every Claude Code session on the machine.
const Block = startMarker + `
## Rise-X Kit

` + "`rise-x-kit`" + ` is a local app for managing Rise-X skills in Claude Code
Desktop: install and update skills, check the Rise-X connection, and run a
setup doctor.

To open it, run ` + "`rise-x-kit`" + `. It prints its address and opens the browser.
Run it again to reopen that window: a second run finds the instance already
running instead of starting another, so running it is always safe.

- Closing the browser tab leaves the app running. **Quit** in the page stops it.
- If ` + "`rise-x-kit`" + ` is not found, the install steps are at
  https://github.com/rise-x/rise-x-ai-public-marketplace/tree/main/kit
` + endMarker

// Result is what Apply did.
type Result struct {
	Path string
	// Action is "added", "updated" or "unchanged".
	Action string
	// Backup is the backup file path, or "" when nothing was written.
	Backup string
}

// Path is the global CLAUDE.md inside claudeDir, resolved through a symlink
// to the file it names: WriteAtomic renames over its target, so writing the
// link's own path would replace a dotfiles link with a plain file and strand
// the partner's tracked copy.
func Path(claudeDir string) string {
	return fsutil.Resolve(filepath.Join(claudeDir, "CLAUDE.md"))
}

// ErrMalformed means the file's markers are not a single matched pair. The
// kit refuses to write in that case: it cannot tell which text is its own, and
// guessing would replace the span between somebody else's marker and its own,
// taking whatever the partner wrote in between with it.
var ErrMalformed = errors.New("the rise-x-kit markers in this file are not a single matched pair, so it was left alone; fix or delete them and run this again")

// Apply adds or refreshes the kit's block in claudeDir/CLAUDE.md. An existing
// file is backed up at its own permissions before the first change, since it is
// the partner's own instructions to Claude.
func Apply(claudeDir string) (Result, error) {
	path := Path(claudeDir)
	res := Result{Path: path}

	original, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return res, err
	}
	if err := checkMarkers(string(original)); err != nil {
		return res, err
	}

	out, action := merge(string(original), exists)
	if action == "unchanged" {
		res.Action = action
		return res, nil
	}

	mode := os.FileMode(0o644)
	if fi, serr := os.Stat(path); serr == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return res, err
	}
	if exists {
		if res.Backup, err = fsutil.Backup(path, original, mode); err != nil {
			return res, err
		}
	}
	if err := fsutil.WriteAtomic(path, []byte(out), mode); err != nil {
		// The backup is what the partner needs precisely now, and the error is
		// the only thing the caller keeps on a failure, so name it there too.
		return res, writeError(path, res.Backup, err)
	}
	res.Action = action
	return res, nil
}

// Remove takes the kit's block back out, for an uninstall. A file that never
// carried one is left alone.
func Remove(claudeDir string) (Result, error) {
	path := Path(claudeDir)
	res := Result{Path: path, Action: "unchanged"}

	original, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return res, nil
		}
		return res, err
	}
	if err := checkMarkers(string(original)); err != nil {
		return res, err
	}
	before, after, found := cut(string(original))
	if !found {
		return res, nil
	}

	mode := os.FileMode(0o644)
	if fi, serr := os.Stat(path); serr == nil {
		mode = fi.Mode().Perm()
	}
	if res.Backup, err = fsutil.Backup(path, original, mode); err != nil {
		return res, err
	}
	if err := fsutil.WriteAtomic(path, []byte(join(before, after)), mode); err != nil {
		return res, writeError(path, res.Backup, err)
	}
	res.Action = "removed"
	return res, nil
}

// writeError names the backup in the message: these instructions are the
// partner's own, and a caller that only keeps the error would otherwise never
// mention the copy sitting next to the file.
func writeError(path, backup string, err error) error {
	if backup == "" {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return fmt.Errorf("write %s (your file is backed up at %s): %w", path, backup, err)
}

// merge renders the file with the block in it, and says what that changed.
func merge(original string, exists bool) (out, action string) {
	before, after, found := cut(original)
	if found {
		if joinBlock(before, after) == original {
			return original, "unchanged"
		}
		return joinBlock(before, after), "updated"
	}
	if !exists || strings.TrimSpace(original) == "" {
		return Block + "\n", "added"
	}
	return strings.TrimRight(original, "\n") + "\n\n" + Block + "\n", "added"
}

// checkMarkers rejects anything but zero or one matched pair, in order. A lone
// start marker is the dangerous one: cut would report "no block", merge would
// append a second, and the next run would then span from the orphan to the new
// block's end and delete everything the partner wrote in between.
func checkMarkers(content string) error {
	starts := strings.Count(content, startMarker)
	ends := strings.Count(content, endMarker)
	if starts == 0 && ends == 0 {
		return nil
	}
	if starts != 1 || ends != 1 {
		return ErrMalformed
	}
	if strings.Index(content, startMarker) > strings.Index(content, endMarker) {
		return ErrMalformed
	}
	return nil
}

// cut splits original around an existing block, keeping the text on each side.
func cut(original string) (before, after string, found bool) {
	i := strings.Index(original, startMarker)
	if i < 0 {
		return original, "", false
	}
	j := strings.Index(original[i:], endMarker)
	if j < 0 {
		return original, "", false
	}
	return original[:i], original[i+j+len(endMarker):], true
}

func joinBlock(before, after string) string { return before + Block + after }

// join closes the gap a removed block leaves, without collapsing the
// partner's own blank lines elsewhere.
func join(before, after string) string {
	joined := strings.TrimRight(before, "\n") + "\n" + strings.TrimLeft(after, "\n")
	if strings.TrimSpace(joined) == "" {
		return ""
	}
	return joined
}
