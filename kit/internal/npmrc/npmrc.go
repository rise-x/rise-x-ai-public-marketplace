// Package npmrc finds and removes leftover Rise-X private-registry entries
// from ~/.npmrc — auth left behind by developer setup that partners
// otherwise carry around (and sometimes worry about) forever.
package npmrc

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/fsutil"
)

var (
	registryLineRe = regexp.MustCompile(`^\s*@rise-x:registry\s*=\s*(\S*)`)
	authLineRe     = regexp.MustCompile(`^\s*//([^/\s]+)/`)
)

const npmjsHost = "registry.npmjs.org"

// Result is what Clean did.
type Result struct {
	// Removed holds the removed lines with their values masked, so a
	// credential never reaches the HTTP response or a log.
	Removed []string
	// Backup is the backup file path, or "" if nothing was removed.
	Backup string
}

// Analyze reports which lines Clean would remove (masked), without touching
// the file.
func Analyze(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	_, removed := clean(string(data))
	return maskAll(removed), nil
}

// Clean removes Rise-X leftover registry/auth lines from path, preserving every
// other byte. It backs the file up first at the original permissions, since the
// backup holds the very credentials this removes; a file with nothing to remove
// is left untouched.
func Clean(path string) (Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, nil
		}
		return Result{}, err
	}
	mode := os.FileMode(0o600)
	if fi, serr := os.Stat(path); serr == nil {
		mode = fi.Mode().Perm()
	}

	kept, removed := clean(string(data))
	if len(removed) == 0 {
		return Result{}, nil
	}

	backup := fsutil.BackupPath(path)
	if err := os.WriteFile(backup, data, mode); err != nil {
		return Result{}, fmt.Errorf("backup %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(kept), mode); err != nil {
		return Result{}, fmt.Errorf("write %s: %w", path, err)
	}
	return Result{Removed: maskAll(removed), Backup: backup}, nil
}

// clean does the actual line filtering. Splitting with strings.SplitAfter
// keeps each line's own terminator attached, so CRLF/LF/mixed files and a
// missing final newline all round-trip untouched.
func clean(content string) (kept string, removed []string) {
	lines := strings.SplitAfter(content, "\n")

	// Pass 1: does any @rise-x:registry line point somewhere other than
	// npmjs? If so, its host's leftover auth lines are fair game too.
	removeHost := ""
	for _, line := range lines {
		text := strings.TrimRight(line, "\r\n")
		m := registryLineRe.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		if host := hostOf(m[1]); host != "" && !strings.EqualFold(host, npmjsHost) {
			removeHost = host
		}
	}

	var out strings.Builder
	for _, line := range lines {
		text := strings.TrimRight(line, "\r\n")

		if m := registryLineRe.FindStringSubmatch(text); m != nil {
			// Anything but npmjs goes, an empty value included: the plan
			// leaves only a registry line that points at npmjs in place.
			if !strings.EqualFold(hostOf(m[1]), npmjsHost) {
				removed = append(removed, text)
				continue
			}
			out.WriteString(line)
			continue
		}
		if removeHost != "" {
			if am := authLineRe.FindStringSubmatch(text); am != nil && strings.EqualFold(hostOf("//"+am[1]), removeHost) {
				removed = append(removed, text)
				continue
			}
		}
		out.WriteString(line)
	}
	return out.String(), removed
}

// hostOf returns the value's hostname without its port, so an explicit
// ":443" still matches the bare host the rules are written against.
func hostOf(value string) string {
	if u, err := url.Parse(value); err == nil && u.Host != "" {
		return u.Hostname()
	}
	// npmrc registry values are sometimes host-only ("//host/path" or bare
	// "host"); url.Parse needs a scheme to populate Host reliably.
	if u, err := url.Parse("//" + strings.TrimPrefix(value, "//")); err == nil {
		return u.Hostname()
	}
	return ""
}

func maskAll(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = mask(l)
	}
	return out
}

// mask keeps the host and the key name and elides the value: everything after
// the first "=" (or the first ":" when the line has no "=") is replaced.
func mask(line string) string {
	i := strings.Index(line, "=")
	if i < 0 {
		i = strings.Index(line, ":")
	}
	if i < 0 {
		return line
	}
	return line[:i+1] + "…"
}
