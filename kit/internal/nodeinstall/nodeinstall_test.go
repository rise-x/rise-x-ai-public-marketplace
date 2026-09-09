package nodeinstall

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner"
)

// homeWithNvm writes the nvm.sh a real nvm install leaves behind.
func homeWithNvm(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".nvm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".nvm", "nvm.sh"), []byte("# nvm\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func argvs(plan []Command) []string {
	out := make([]string, len(plan))
	for i, c := range plan {
		out[i] = runner.Argv(c.Name, c.Args)
	}
	return out
}

func TestPlan_NvmPresent(t *testing.T) {
	home := homeWithNvm(t)
	plan := Plan(Env{GOOS: "darwin", Home: home, Stat: os.Stat})
	if len(plan) != 2 {
		t.Fatalf("plan = %v, want the two nvm commands", argvs(plan))
	}
	want := ". '" + filepath.Join(home, ".nvm", "nvm.sh") + "' &&"
	for i, cmd := range []string{"nvm install --lts", `nvm alias default 'lts/*'`} {
		if plan[i].Name != "bash" || len(plan[i].Args) != 2 || plan[i].Args[0] != "-lc" {
			t.Fatalf("plan[%d] = %+v, want a bash -lc command", i, plan[i])
		}
		script := plan[i].Args[1]
		if !strings.HasPrefix(script, want) {
			t.Errorf("plan[%d] script %q does not source %s", i, script, want)
		}
		if !strings.HasSuffix(script, cmd) {
			t.Errorf("plan[%d] script %q does not run %q", i, script, cmd)
		}
	}
}

func TestPlan_NvmAbsent_InstallsNvmFirst(t *testing.T) {
	plan := Plan(Env{GOOS: "linux", Home: t.TempDir(), Stat: os.Stat})
	if len(plan) != 3 {
		t.Fatalf("plan = %v, want the installer plus the two nvm commands", argvs(plan))
	}
	first := plan[0]
	if first.Name != "bash" || len(first.Args) != 2 || first.Args[0] != "-c" {
		t.Fatalf("plan[0] = %+v, want a bash -c script", first)
	}
	script := first.Args[1]
	for _, want := range []string{
		"curl -fsSL " + nvmInstallURL,
		nvmInstallSHA,
		"checksum mismatch",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("installer script is missing %q:\n%s", want, script)
		}
	}
	if !strings.Contains(nvmInstallURL, nvmVersion) {
		t.Errorf("installer URL %q is not pinned to %s", nvmInstallURL, nvmVersion)
	}
}

func TestPlan_WindowsWithWinget(t *testing.T) {
	plan := Plan(Env{GOOS: "windows", LookPath: func(string) (string, error) {
		return `C:\winget.exe`, nil
	}})
	if len(plan) != 1 {
		t.Fatalf("plan = %v, want one winget command", argvs(plan))
	}
	// The resolved path, not the bare name: a bare name is looked up again at
	// run time, against whatever PATH the child ends up with.
	want := `C:\winget.exe install --id OpenJS.NodeJS.LTS -e --accept-source-agreements --accept-package-agreements`
	if got := runner.Argv(plan[0].Name, plan[0].Args); got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}
}

// `winget install` on an installed package exits non-zero instead of
// upgrading, and the doctor offers this action exactly when Node is present
// but too old.
func TestPlan_WindowsUpgradesWhenNodeIsAlreadyThere(t *testing.T) {
	plan := Plan(Env{GOOS: "windows", NodeFound: true, LookPath: func(string) (string, error) {
		return `C:\winget.exe`, nil
	}})
	if len(plan) != 1 {
		t.Fatalf("plan = %v, want one winget command", argvs(plan))
	}
	if got := plan[0].Args[0]; got != "upgrade" {
		t.Errorf("verb = %q, want upgrade", got)
	}
}

// The nvm installer honours $NVM_DIR, so the plan has to source the nvm it
// actually wrote rather than assuming $HOME/.nvm.
func TestPlan_HonoursNvmDir(t *testing.T) {
	dir := t.TempDir()
	nvm := filepath.Join(dir, "elsewhere")
	if err := os.MkdirAll(nvm, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nvm, "nvm.sh"), []byte("#\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// nvm lives at NVM_DIR, and nowhere near Home: the plan must see it as
	// installed and not prepend a second install step.
	plan := Plan(Env{GOOS: "darwin", Home: t.TempDir(), NvmDir: nvm, Stat: os.Stat})
	if len(plan) != 2 {
		t.Fatalf("plan = %v, want the two nvm commands with no install step", argvs(plan))
	}
	want := ". '" + filepath.Join(nvm, "nvm.sh") + "' &&"
	for _, c := range plan {
		if !strings.HasPrefix(c.Args[1], want) {
			t.Errorf("script %q does not source the nvm at NVM_DIR", c.Args[1])
		}
	}
}

// The pinned installer's own default is $XDG_CONFIG_HOME/nvm when that is
// set, so a plan that assumed $HOME/.nvm sourced a file the install had never
// written and the whole job failed with "No such file or directory".
func TestPlan_HonoursXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	xdg := filepath.Join(dir, "config")
	if err := os.MkdirAll(filepath.Join(xdg, "nvm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "nvm", "nvm.sh"), []byte("#\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := Plan(Env{GOOS: "linux", Home: t.TempDir(), XDGConfigHome: xdg, Stat: os.Stat})
	if len(plan) != 2 {
		t.Fatalf("plan = %v, want the two nvm commands with no install step", argvs(plan))
	}
	want := ". '" + filepath.Join(xdg, "nvm", "nvm.sh") + "' &&"
	for _, c := range plan {
		if !strings.HasPrefix(c.Args[1], want) {
			t.Errorf("script %q does not source the nvm under XDG_CONFIG_HOME", c.Args[1])
		}
	}
}

// NVM_DIR wins over XDG_CONFIG_HOME, the order the installer uses.
func TestPlan_NvmDirBeatsXDG(t *testing.T) {
	dir := t.TempDir()
	env := Env{GOOS: "linux", Home: dir, NvmDir: filepath.Join(dir, "explicit"),
		XDGConfigHome: filepath.Join(dir, "config"), Stat: os.Stat}
	plan := Plan(env)
	want := ". '" + filepath.Join(dir, "explicit", "nvm.sh") + "' &&"
	if !strings.HasPrefix(plan[len(plan)-1].Args[1], want) {
		t.Errorf("script %q does not prefer NVM_DIR", plan[len(plan)-1].Args[1])
	}
}

// Without winget there is nothing the kit can run, so no fix is offered and
// the doctor keeps its nodejs.org link.
func TestPlan_WindowsWithoutWinget(t *testing.T) {
	env := Env{GOOS: "windows", LookPath: func(string) (string, error) {
		return "", errors.New("not found")
	}}
	if plan := Plan(env); plan != nil {
		t.Errorf("plan = %v, want none", argvs(plan))
	}
	if CanInstall(env) {
		t.Error("CanInstall = true, want false without winget")
	}
}

func TestCanInstall_UnknownPlatform(t *testing.T) {
	if CanInstall(Env{GOOS: "plan9"}) {
		t.Error("CanInstall = true, want false on a platform with no installer")
	}
}

// winget refuses to upgrade a Node it did not install - nvm-windows, fnm, a
// zip - so the plan carries the install verb as the recovery for exactly that
// exit code, rather than reporting a failed job.
func TestPlan_WindowsUpgradeFallsBackToInstall(t *testing.T) {
	env := Env{GOOS: "windows", NodeFound: true, LookPath: func(string) (string, error) {
		return `C:\winget.exe`, nil
	}}
	plan := Plan(env)
	if len(plan) != 1 {
		t.Fatalf("plan = %v, want one winget command", argvs(plan))
	}
	if plan[0].Args[0] != "upgrade" {
		t.Fatalf("verb = %q, want upgrade when Node is present", plan[0].Args[0])
	}
	if len(plan[0].FallbackArgs) == 0 || plan[0].FallbackArgs[0] != "install" {
		t.Fatalf("FallbackArgs = %v, want the install verb", plan[0].FallbackArgs)
	}
	if !plan[0].Recovers(0x8A150014) {
		t.Error("no fallback for winget's \"no applicable upgrade\" code")
	}
	if plan[0].Recovers(1) {
		t.Error("fallback fired for a generic failure; only the wrong-verb code recovers")
	}
	if !plan[0].Recovers(-1978335212) {
		t.Error("the signed reading of the same DWORD is not recognised")
	}
}

// A machine with no home and no nvm directory has nowhere to install to, so
// there is no plan and the doctor keeps the nodejs.org link.
func TestPlan_NoHomeNoPlan(t *testing.T) {
	if plan := Plan(Env{GOOS: "darwin", Stat: os.Stat}); len(plan) != 0 {
		t.Fatalf("plan = %v, want nothing runnable", argvs(plan))
	}
}
