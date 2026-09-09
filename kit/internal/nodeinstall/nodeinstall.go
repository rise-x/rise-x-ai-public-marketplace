// Package nodeinstall builds the commands that install or update Node.js:
// nvm on macOS and Linux, winget on Windows.
package nodeinstall

import (
	"os"
	"path/filepath"
)

// The pinned nvm release the kit installs from, and the sha256 of that exact
// install.sh. The script is verified against this before it runs, so a
// changed or replaced download aborts instead of executing.
const (
	nvmVersion    = "v0.40.3"
	nvmInstallSHA = "2d8359a64a3cb07c02389ad88ceecd43f2fa469c06104f92f98df5b6f315275f"
	nvmInstallURL = "https://raw.githubusercontent.com/nvm-sh/nvm/" + nvmVersion + "/install.sh"
)

// nvmInstallScript downloads the pinned installer, checks its sha256 and runs
// it. Either checksum tool may be the one present: shasum ships with macOS,
// sha256sum with most Linux distributions.
const nvmInstallScript = `set -e
f="$(mktemp)"
trap 'rm -f "$f"' EXIT
curl -fsSL ` + nvmInstallURL + ` -o "$f"
sum="$(shasum -a 256 "$f" 2>/dev/null || sha256sum "$f")"
case "$sum" in
  ` + nvmInstallSHA + `*) ;;
  *) echo "nvm installer checksum mismatch; nothing was installed" >&2; exit 1 ;;
esac
bash "$f"
`

// Command is one step of a plan: an argv the job runner streams.
type Command struct {
	Name string
	Args []string
}

// Env is the OS access Plan needs, so tests describe a machine instead of
// probing this one.
type Env struct {
	GOOS     string
	Home     string
	LookPath func(file string) (string, error)
	Stat     func(name string) (os.FileInfo, error)
}

// Plan returns the commands that install or update Node.js on env's platform,
// in order, or nil when the kit has no installer to drive there.
func Plan(env Env) []Command {
	switch env.GOOS {
	case "darwin", "linux":
		return nvmPlan(env)
	case "windows":
		return wingetPlan(env)
	}
	return nil
}

// CanInstall reports whether Plan has anything to run, so the doctor offers
// the fix only where it works.
func CanInstall(env Env) bool {
	return len(Plan(env)) > 0
}

// nvmPlan installs the current LTS release through nvm and makes it the
// default, installing nvm itself first when it isn't there. PROFILE is
// deliberately left unset: the installer appends to the user's shell profile
// as it normally does, so new shells - and new Claude Code sessions - see node.
func nvmPlan(env Env) []Command {
	steps := []Command{
		nvmShell("nvm install --lts"),
		nvmShell(`nvm alias default 'lts/*'`),
	}
	if hasNvm(env) {
		return steps
	}
	return append([]Command{{Name: "bash", Args: []string{"-c", nvmInstallScript}}}, steps...)
}

func hasNvm(env Env) bool {
	if env.Home == "" || env.Stat == nil {
		return false
	}
	_, err := env.Stat(filepath.Join(env.Home, ".nvm", "nvm.sh"))
	return err == nil
}

// nvmShell runs one nvm command in a login shell with nvm sourced: nvm is a
// shell function, so it exists only once nvm.sh has been read.
func nvmShell(cmd string) Command {
	return Command{Name: "bash", Args: []string{"-lc", `. "$HOME/.nvm/nvm.sh" && ` + cmd}}
}

// wingetPlan installs the LTS package with winget, when winget is there at
// all; without it the doctor keeps the nodejs.org link instead. Untested: no
// Windows machine has run this path yet.
func wingetPlan(env Env) []Command {
	if env.LookPath == nil {
		return nil
	}
	if _, err := env.LookPath("winget.exe"); err != nil {
		return nil
	}
	return []Command{{Name: "winget", Args: []string{
		"install", "--id", "OpenJS.NodeJS.LTS", "-e",
		"--accept-source-agreements", "--accept-package-agreements",
	}}}
}
