package claudecli

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/semver"
)

// locateWindowsBundle returns the Windows Desktop app's bundled CLI paths
// under %APPDATA%\Claude\claude-code\<semver>\claude.exe, highest semver
// first. Whether Windows writes a ".verified" marker next to a version dir
// (as macOS does) isn't confirmed, so it isn't required here, but a version
// dir that has one sorts ahead of one that doesn't.
func locateWindowsBundle(env Env) []string {
	if env.AppData == "" {
		return nil
	}
	base := filepath.Join(env.AppData, "Claude", "claude-code")

	type candidate struct {
		version  string
		verified bool
		path     string
	}
	var candidates []candidate
	for _, e := range readDir(env, base) {
		if !e.IsDir() {
			continue
		}
		versionDir := filepath.Join(base, e.Name())
		bin := filepath.Join(versionDir, "claude.exe")
		if !fileExists(env, bin) {
			continue
		}
		candidates = append(candidates, candidate{
			version:  e.Name(),
			verified: fileExists(env, filepath.Join(versionDir, ".verified")),
			path:     bin,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].verified != candidates[j].verified {
			return candidates[i].verified
		}
		return semver.VersionLess(candidates[j].version, candidates[i].version)
	})

	paths := make([]string, len(candidates))
	for i, c := range candidates {
		paths[i] = c.path
	}
	return paths
}

// locateWindowsProbes walks install roots that aren't confirmed to hold the
// CLI but are cheap to check when they don't exist, bounded to depth <= 4 so
// a huge Programs tree can't cause a slow scan, newest-looking path first.
func locateWindowsProbes(env Env) []string {
	if env.LocalAppData == "" {
		return nil // Join would turn the empty root into a relative path
	}
	var out []string
	out = append(out, walkForFile(env, filepath.Join(env.LocalAppData, "Programs", "Claude"), "claude.exe", 4)...)
	out = append(out, walkForFile(env, filepath.Join(env.LocalAppData, "AnthropicClaude"), "claude.exe", 4)...)

	// Newest first by version, not by string: reverse-lexical puts app-1.9.0
	// above app-1.10.0, so the probe adopted the older CLI and the UI then
	// reported its version. Ties fall back to the path so the order is stable.
	sort.Slice(out, func(i, j int) bool {
		vi, vj := pathVersion(out[i]), pathVersion(out[j])
		if vi != vj {
			return semver.VersionLess(vj, vi)
		}
		return out[i] > out[j]
	})
	return out
}

// pathVersion pulls the version out of an install path like
// ...\Programs\Claude\app-1.10.0\claude.exe. The segment nearest the binary
// wins, and a path with no version-looking segment sorts as 0.
func pathVersion(path string) string {
	segments := strings.Split(filepath.ToSlash(path), "/")
	for i := len(segments) - 1; i >= 0; i-- {
		if v := versionSuffix(segments[i]); v != "" {
			return v
		}
	}
	return ""
}

// versionSuffix reads "1.10.0" out of "app-1.10.0" or "1.10.0", and returns ""
// for a segment that carries no dotted number.
func versionSuffix(segment string) string {
	if i := strings.LastIndexAny(segment, "-_"); i >= 0 {
		segment = segment[i+1:]
	}
	segment = strings.TrimPrefix(segment, "v")
	if segment == "" || !strings.Contains(segment, ".") {
		return ""
	}
	for _, r := range segment {
		if (r < '0' || r > '9') && r != '.' {
			return ""
		}
	}
	return segment
}

func walkForFile(env Env, root, filename string, maxDepth int) []string {
	if root == "" {
		return nil
	}
	var out []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		for _, e := range readDir(env, dir) {
			full := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if depth < maxDepth {
					walk(full, depth+1)
				}
				continue
			}
			if strings.EqualFold(e.Name(), filename) {
				out = append(out, full)
			}
		}
	}
	walk(root, 0)
	return out
}
