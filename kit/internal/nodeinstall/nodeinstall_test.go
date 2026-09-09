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
	plan := Plan(Env{GOOS: "darwin", Home: homeWithNvm(t), Stat: os.Stat})
	if len(plan) != 2 {
		t.Fatalf("plan = %v, want the two nvm commands", argvs(plan))
	}
	for i, want := range []string{"nvm install --lts", `nvm alias default 'lts/*'`} {
		if plan[i].Name != "bash" || len(plan[i].Args) != 2 || plan[i].Args[0] != "-lc" {
			t.Fatalf("plan[%d] = %+v, want a bash -lc command", i, plan[i])
		}
		script := plan[i].Args[1]
		if !strings.Contains(script, `. "$HOME/.nvm/nvm.sh" &&`) {
			t.Errorf("plan[%d] script %q does not source nvm.sh", i, script)
		}
		if !strings.HasSuffix(script, want) {
			t.Errorf("plan[%d] script %q does not run %q", i, script, want)
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
	want := "winget install --id OpenJS.NodeJS.LTS -e --accept-source-agreements --accept-package-agreements"
	if got := runner.Argv(plan[0].Name, plan[0].Args); got != want {
		t.Errorf("argv = %q, want %q", got, want)
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
