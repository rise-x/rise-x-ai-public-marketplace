package doctor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/semver"
)

// Env bundles the OS access DetectNode and DetectGit need, so tests can
// probe a t.TempDir() tree instead of the real machine.
type Env struct {
	Home     string
	LookPath func(file string) (string, error)
	Glob     func(pattern string) ([]string, error)
	Stat     func(name string) (os.FileInfo, error)
	Runner   runner.Runner
}

// DetectNode looks for node on PATH, then under common version-manager and
// package-manager install locations (nvm, fnm, Homebrew), since rise-x-apps
// partners often have node managed outside PATH-visible defaults.
func DetectNode(env Env) (version string, found bool) {
	candidates := env.candidatesInOrder()
	for _, c := range candidates {
		if v, ok := nodeVersionOf(env, c); ok {
			return v, true
		}
	}
	return "", false
}

func (env Env) candidatesInOrder() []string {
	var out []string
	if env.LookPath != nil {
		if p, err := env.LookPath("node"); err == nil {
			out = append(out, p)
		}
	}
	if env.Home != "" && env.Glob != nil {
		globs := []string{
			filepath.Join(env.Home, ".nvm", "versions", "node", "*", "bin", "node"),
			filepath.Join(env.Home, ".fnm", "node-versions", "*", "installation", "bin", "node"),
			filepath.Join(env.Home, ".local", "share", "fnm", "node-versions", "*", "installation", "bin", "node"),
		}
		var managed []string
		for _, g := range globs {
			if matches, err := env.Glob(g); err == nil {
				managed = append(managed, matches...)
			}
		}
		sortNewestFirst(managed)
		out = append(out, managed...)
	}
	out = append(out, "/opt/homebrew/bin/node", "/usr/local/bin/node")
	return out
}

// versionDirRe matches the directory nvm and fnm name an install after.
var versionDirRe = regexp.MustCompile(`^v\d+(\.\d+)*$`)

// sortNewestFirst orders version-manager installs newest first: Glob returns
// them in lexical order, which puts v9 above v22.
func sortNewestFirst(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		return semver.VersionLess(versionOfPath(paths[j]), versionOfPath(paths[i]))
	})
}

func versionOfPath(path string) string {
	for _, seg := range strings.Split(filepath.ToSlash(path), "/") {
		if versionDirRe.MatchString(seg) {
			return strings.TrimPrefix(seg, "v")
		}
	}
	return ""
}

func nodeVersionOf(env Env, path string) (string, bool) {
	if env.Runner == nil {
		return "", false
	}
	// The homebrew paths are guesses, not confirmed hits like PATH/glob
	// results, so skip invoking a binary that isn't even there.
	if env.Stat != nil {
		if _, err := env.Stat(path); err != nil {
			return "", false
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stdout, _, exitCode, err := env.Runner.Run(ctx, path, []string{"--version"})
	if err != nil || exitCode != 0 {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(stdout), "v")), true
}

// DetectGit reports whether git is on PATH (Windows only; the Claude Code
// Desktop app already prompts to install it when missing).
func DetectGit(env Env) bool {
	if env.LookPath == nil {
		return false
	}
	_, err := env.LookPath("git")
	return err == nil
}

// RealEnv builds an Env from the real OS.
func RealEnv(r runner.Runner) Env {
	home, _ := os.UserHomeDir()
	return Env{Home: home, LookPath: exec.LookPath, Glob: filepath.Glob, Stat: os.Stat, Runner: r}
}
