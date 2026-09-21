package claudecli

import (
	"path/filepath"
	"sort"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/semver"
)

// locateDarwin returns the macOS Desktop app's bundled CLI paths for versions
// Claude marked verified (a ".verified" file next to claude.app), highest
// semver first. Not build-tagged, so tests can run it under a simulated GOOS.
func locateDarwin(env Env) []string {
	if env.Home == "" {
		return nil
	}
	base := filepath.Join(env.Home, "Library", "Application Support", "Claude", "claude-code")

	type candidate struct {
		version string
		path    string
	}
	var candidates []candidate
	for _, e := range readDir(env, base) {
		if !e.IsDir() {
			continue
		}
		versionDir := filepath.Join(base, e.Name())
		bin := filepath.Join(versionDir, "claude.app", "Contents", "MacOS", "claude")
		if !fileExists(env, bin) || !fileExists(env, filepath.Join(versionDir, ".verified")) {
			continue
		}
		candidates = append(candidates, candidate{version: e.Name(), path: bin})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return semver.VersionLess(candidates[j].version, candidates[i].version)
	})

	paths := make([]string, len(candidates))
	for i, c := range candidates {
		paths[i] = c.path
	}
	return paths
}
