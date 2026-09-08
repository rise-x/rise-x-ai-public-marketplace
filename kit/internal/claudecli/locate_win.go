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
	var out []string
	out = append(out, walkForFile(env, filepath.Join(env.LocalAppData, "Programs", "Claude"), "claude.exe", 4)...)
	out = append(out, walkForFile(env, filepath.Join(env.LocalAppData, "AnthropicClaude"), "claude.exe", 4)...)

	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
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
