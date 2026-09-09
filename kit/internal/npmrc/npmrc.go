// Package npmrc finds and repoints leftover Rise-X `@rise-x:registry` lines
// in ~/.npmrc: developer setup once pointed the scope at a private registry,
// and that line is a leftover now that @rise-x packages are public. Auth
// lines are never touched, whatever host they name: the credentials another
// private scope still needs and a GitHub Packages token are both legitimate.
package npmrc

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/fsutil"
)

var registryLineRe = regexp.MustCompile(`^\s*@rise-x:registry\s*=\s*(\S*)`)

const npmjsHost = "registry.npmjs.org"

// npmjsRegistryURL is what an offending @rise-x:registry line's value is
// rewritten to.
const npmjsRegistryURL = "https://registry.npmjs.org/"

// Result is what Clean did.
type Result struct {
	// Rewritten holds the pre-fix line(s), masked so a credential never
	// reaches the HTTP response or a log, even though a registry line never
	// carries one itself.
	Rewritten []string
	// Backup is the backup file path, or "" if nothing was rewritten.
	Backup string
}

// Report is what one scan of the file found. Both fields come from the same
// read, so the masked lines and the host can never describe different
// versions of it.
type Report struct {
	// Lines are the @rise-x:registry lines Clean would rewrite, masked.
	Lines []string
	// Host is the registry host the first of them points at, unmasked: a
	// registry host is not a credential, so the doctor can name it directly.
	// "another registry" stands in for a value with no readable host.
	Host string
}

// Analyze reports what Clean would rewrite, without touching the file.
func Analyze(path string) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Report{}, nil
		}
		return Report{}, err
	}
	_, rewritten := clean(string(data))
	if len(rewritten) == 0 {
		return Report{}, nil
	}
	return Report{Lines: maskAll(rewritten), Host: hostLabel(rewritten[0])}, nil
}

// hostLabel names the registry an offending line points at.
func hostLabel(line string) string {
	m := registryLineRe.FindStringSubmatch(line)
	if m == nil {
		return "another registry"
	}
	if host := hostOf(m[1]); host != "" {
		return host
	}
	return "another registry"
}

// Clean rewrites Rise-X leftover @rise-x:registry lines in path to point at
// the public npm registry, preserving every other byte — auth lines
// included, whatever host they name. It backs the file up first at the
// original permissions; a file with nothing to rewrite is left untouched.
//
// The write lands on the file a symlinked ~/.npmrc resolves to. WriteAtomic
// renames over its target, so writing the link's own path would replace the
// link: a stow or chezmoi partner would keep the bad line in the repo they
// track and get it back at the next re-link.
func Clean(path string) (Result, error) {
	path = fsutil.Resolve(path)
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

	kept, rewritten := clean(string(data))
	if len(rewritten) == 0 {
		return Result{}, nil
	}

	backup, err := fsutil.Backup(path, data, mode)
	if err != nil {
		return Result{}, err
	}
	if err := fsutil.WriteAtomic(path, []byte(kept), mode); err != nil && !errors.Is(err, fsutil.ErrNotDurable) {
		// The backup is the whole point of the failure path: the auth lines
		// this file keeps may exist nowhere else, so name it even here.
		return Result{Backup: backup}, fmt.Errorf("write %s (your file is backed up at %s): %w", path, backup, err)
	}
	return Result{Rewritten: maskAll(rewritten), Backup: backup}, nil
}

// clean does the actual line rewriting. Splitting with strings.SplitAfter
// keeps each line's own terminator attached, so CRLF/LF/mixed files and a
// missing final newline all round-trip untouched. Only the value of an
// offending @rise-x:registry line changes — same line position, same line
// ending — so every other byte in the file, auth lines included, is
// identical to the input.
func clean(content string) (kept string, rewritten []string) {
	lines := strings.SplitAfter(content, "\n")

	var out strings.Builder
	for _, line := range lines {
		text := strings.TrimRight(line, "\r\n")
		term := line[len(text):]

		if loc := registryLineRe.FindStringSubmatchIndex(text); loc != nil {
			valStart, valEnd := loc[2], loc[3]
			if !strings.EqualFold(hostOf(text[valStart:valEnd]), npmjsHost) {
				rewritten = append(rewritten, text)
				out.WriteString(text[:valStart] + npmjsRegistryURL + text[valEnd:] + term)
				continue
			}
		}
		out.WriteString(line)
	}
	return out.String(), rewritten
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
