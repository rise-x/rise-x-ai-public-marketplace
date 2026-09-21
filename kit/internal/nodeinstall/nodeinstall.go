// Package nodeinstall builds the commands that install or update Node.js:
// nvm on macOS and Linux, winget on Windows.
package nodeinstall

import (
	"os"
	"path/filepath"
	"strings"
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
	// FallbackOn lists exit codes that mean the verb was wrong rather than
	// the machine broken. The caller then runs FallbackArgs instead of
	// failing: winget refuses to upgrade a Node it did not install, which is
	// every nvm-windows, fnm or zip install.
	FallbackOn   []int
	FallbackArgs []string
}

// Recovers reports whether code is one FallbackArgs answers.
func (c Command) Recovers(code int) bool {
	if len(c.FallbackArgs) == 0 {
		return false
	}
	for _, want := range c.FallbackOn {
		if want == code {
			return true
		}
	}
	return false
}

// Env is the OS access Plan needs, so tests describe a machine instead of
// probing this one.
type Env struct {
	GOOS string
	Home string
	// NvmDir is $NVM_DIR when the machine sets one, and XDGConfigHome is
	// $XDG_CONFIG_HOME. The nvm installer honours both, in that order, so
	// assuming $HOME/.nvm would install nvm in one place and then source it
	// from another.
	NvmDir        string
	XDGConfigHome string
	LookPath      func(file string) (string, error)
	Stat          func(name string) (os.FileInfo, error)
	// NodeFound says whether node is already installed, which is what decides
	// between winget's install and upgrade verbs.
	NodeFound bool
}

// nvmDir is where this machine keeps nvm, following the pinned installer's
// own nvm_install_dir: $NVM_DIR wins, then $XDG_CONFIG_HOME/nvm, then
// $HOME/.nvm.
func (env Env) nvmDir() string {
	if env.NvmDir != "" {
		return env.NvmDir
	}
	if env.XDGConfigHome != "" {
		return filepath.Join(env.XDGConfigHome, "nvm")
	}
	if env.Home == "" {
		return ""
	}
	return filepath.Join(env.Home, ".nvm")
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
	if env.nvmDir() == "" {
		return nil // nowhere to install to, and nowhere to source it from
	}
	steps := []Command{
		env.nvmShell("nvm install --lts"),
		env.nvmShell(`nvm alias default 'lts/*'`),
	}
	if hasNvm(env) {
		return steps
	}
	return append([]Command{{Name: "bash", Args: []string{"-c", nvmInstallScript}}}, steps...)
}

func hasNvm(env Env) bool {
	dir := env.nvmDir()
	if dir == "" || env.Stat == nil {
		return false
	}
	_, err := env.Stat(filepath.Join(dir, "nvm.sh"))
	return err == nil
}

// nvmShell runs one nvm command in a login shell with nvm sourced: nvm is a
// shell function, so it exists only once nvm.sh has been read. The directory
// is the one resolved here rather than a shell fallback, because the
// installer's default follows XDG_CONFIG_HOME: on such a machine
// "${NVM_DIR:-$HOME/.nvm}" names a path nvm was never written to, and `bash
// -lc` does not rescue it, since the installer edits ~/.bashrc and a
// non-interactive login shell does not read that.
func (env Env) nvmShell(cmd string) Command {
	src := shellQuote(filepath.Join(env.nvmDir(), "nvm.sh"))
	return Command{Name: "bash", Args: []string{"-lc", ". " + src + " && " + cmd}}
}

// shellQuote wraps s for a POSIX shell, so a home directory with a space or a
// quote in it still names one word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// wingetNoUpgrade is winget's "no applicable upgrade found", which is what an
// upgrade of a Node winget did not install reports - nvm-windows, fnm, or a
// zip. Windows hands the exit status back as the raw DWORD; the signed reading
// is listed too, since that is how a 32-bit process would see it.
var wingetNoUpgrade = []int{0x8A150014, -1978335212}

// wingetPlan installs or upgrades the LTS package with winget, when winget is
// there at all; without it the doctor keeps the nodejs.org link instead.
// `winget install` on an installed package exits non-zero rather than
// upgrading it, and the doctor offers this action exactly when Node is present
// but old, so the verb has to follow NodeFound - falling back to install when
// winget has no upgrade to apply because some other tool put Node there.
// Untested: no Windows machine has run this path yet.
func wingetPlan(env Env) []Command {
	if env.LookPath == nil {
		return nil
	}
	exe, err := env.LookPath("winget.exe")
	if err != nil {
		return nil
	}
	args := func(verb string) []string {
		return []string{verb, "--id", "OpenJS.NodeJS.LTS", "-e",
			"--accept-source-agreements", "--accept-package-agreements"}
	}
	if !env.NodeFound {
		return []Command{{Name: exe, Args: args("install")}}
	}
	return []Command{{
		Name: exe, Args: args("upgrade"),
		FallbackOn: wingetNoUpgrade, FallbackArgs: args("install"),
	}}
}
