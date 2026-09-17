// Package claudecli locates the claude CLI binary and wraps its --json
// commands and mutating subcommands. Reads and writes to Claude Code's own
// state always go through this CLI; the kit never edits plugin files itself.
package claudecli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner"
)

// ErrNotFound is returned by Locate when no working claude binary was found.
var ErrNotFound = errors.New("claude CLI not found")

// Where the binary was found, so the UI can say "Found in ...".
const (
	SourcePath          = "path"
	SourceLocalBin      = "local-bin"
	SourceDesktopBundle = "desktop-bundle"
	SourceWindowsProbe  = "windows-probe"
)

// CLI is a located, version-verified claude binary.
type CLI struct {
	Path    string
	Version string
	Source  string
}

// Env bundles everything Locate needs to search for and verify a claude
// binary, so tests can point it at a t.TempDir() tree under any simulated
// GOOS without needing a real claude install or a matching host OS.
type Env struct {
	Home string // user home directory
	GOOS string // runtime.GOOS, or a simulated value in tests

	LookPath func(file string) (string, error)
	// FS is the filesystem the search reads, rooted at the volume Home lives
	// on. OS paths are converted with fsPath.
	FS fs.FS

	// Windows search roots; empty (and unused) on other GOOS values.
	LocalAppData string
	AppData      string

	Runner  runner.Runner
	Timeout time.Duration // --version verification timeout; defaults to 5s.
}

// RealEnv builds an Env from the real OS and filesystem.
func RealEnv(r runner.Runner) Env {
	home, _ := os.UserHomeDir()
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = filepath.Join(home, "AppData", "Roaming")
	}
	return Env{
		Home:         home,
		GOOS:         runtime.GOOS,
		LookPath:     exec.LookPath,
		FS:           hostFS(home),
		LocalAppData: os.Getenv("LOCALAPPDATA"),
		AppData:      appData,
		Runner:       r,
	}
}

// hostFS roots the real filesystem at the volume home lives on. On Windows a
// search root on another volume is out of scope: the Desktop app installs
// under %LOCALAPPDATA%/%APPDATA%, which sit on the user profile's drive.
func hostFS(home string) fs.FS {
	return os.DirFS(filepath.VolumeName(home) + string(os.PathSeparator))
}

// fsPath converts an absolute OS path to the volume-relative, slash-separated
// form fs.FS expects ("/Users/x" and `C:\Users\x` both become "Users/x").
func fsPath(path string) string {
	path = strings.TrimPrefix(path, filepath.VolumeName(path))
	path = strings.TrimPrefix(filepath.ToSlash(path), "/")
	if path == "" {
		return "."
	}
	return path
}

// Locate searches, in order: PATH, ~/.local/bin/claude, the OS-specific
// Desktop app bundle (macOS claude.app / Windows %APPDATA%\Claude\claude-code),
// then on Windows the remaining speculative install-root probes, verifying
// each candidate with `--version` before accepting it.
func Locate(ctx context.Context, env Env) (*CLI, error) {
	if env.Timeout <= 0 {
		env.Timeout = 5 * time.Second
	}

	if env.LookPath != nil {
		if p, err := env.LookPath("claude"); err == nil {
			if cli, ok := verify(ctx, env, p, SourcePath); ok {
				return cli, nil
			}
		}
	}

	if env.Home != "" {
		local := filepath.Join(env.Home, ".local", "bin", "claude")
		if env.GOOS == "windows" {
			local += ".exe"
		}
		if fileExists(env, local) {
			if cli, ok := verify(ctx, env, local, SourceLocalBin); ok {
				return cli, nil
			}
		}
	}

	switch env.GOOS {
	case "darwin":
		for _, p := range locateDarwin(env) {
			if cli, ok := verify(ctx, env, p, SourceDesktopBundle); ok {
				return cli, nil
			}
		}
	case "windows":
		for _, p := range locateWindowsBundle(env) {
			if cli, ok := verify(ctx, env, p, SourceDesktopBundle); ok {
				return cli, nil
			}
		}
		for _, p := range locateWindowsProbes(env) {
			if cli, ok := verify(ctx, env, p, SourceWindowsProbe); ok {
				return cli, nil
			}
		}
	}

	return nil, ErrNotFound
}

func fileExists(env Env, path string) bool {
	if env.FS == nil {
		return false
	}
	_, err := fs.Stat(env.FS, fsPath(path))
	return err == nil
}

// readDir lists an OS directory through env.FS; a missing directory is an
// empty listing, since every search root here is a guess.
func readDir(env Env, path string) []fs.DirEntry {
	if env.FS == nil {
		return nil
	}
	entries, err := fs.ReadDir(env.FS, fsPath(path))
	if err != nil {
		return nil
	}
	return entries
}

func verify(ctx context.Context, env Env, path, source string) (*CLI, bool) {
	if env.Runner == nil {
		return nil, false
	}
	vctx, cancel := context.WithTimeout(ctx, env.Timeout)
	defer cancel()
	stdout, _, exitCode, err := env.Runner.Run(vctx, path, []string{"--version"})
	if err != nil || exitCode != 0 {
		return nil, false
	}
	return &CLI{Path: path, Version: ShortVersion(stdout), Source: source}, true
}
