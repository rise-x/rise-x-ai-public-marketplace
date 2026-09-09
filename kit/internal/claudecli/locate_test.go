package claudecli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner/runnertest"
)

func baseEnv(t *testing.T, goos string) Env {
	t.Helper()
	tmp := t.TempDir()
	notFound := func(string) (string, error) { return "", errors.New("not found") }
	return Env{
		Home:     tmp,
		GOOS:     goos,
		LookPath: notFound,
		FS:       hostFS(tmp),
		Runner:   runnertest.NewFake(),
	}
}

func verifiedOK(f *runnertest.Fake, path string) {
	f.Set(path, []string{"--version"}, runnertest.Result{Stdout: "2.1.258 (Claude Code)\n"})
}

func TestLocate_PathHit(t *testing.T) {
	env := baseEnv(t, "linux")
	bin := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env.LookPath = func(string) (string, error) { return bin, nil }
	verifiedOK(env.Runner.(*runnertest.Fake), bin)

	cli, err := Locate(context.Background(), env)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if cli.Path != bin {
		t.Errorf("Path = %q, want %q", cli.Path, bin)
	}
	if cli.Version != "2.1.258" {
		t.Errorf("Version = %q", cli.Version)
	}
}

func TestLocate_LocalBin(t *testing.T) {
	env := baseEnv(t, "linux")
	local := filepath.Join(env.Home, ".local", "bin", "claude")
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	verifiedOK(env.Runner.(*runnertest.Fake), local)

	cli, err := Locate(context.Background(), env)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if cli.Path != local {
		t.Errorf("Path = %q, want %q", cli.Path, local)
	}
}

func TestLocate_MacOSBundle_HighestVerified(t *testing.T) {
	env := baseEnv(t, "darwin")
	base := filepath.Join(env.Home, "Library", "Application Support", "Claude", "claude-code")

	mkVersion := func(version string, verified bool) string {
		dir := filepath.Join(base, version, "claude.app", "Contents", "MacOS")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(dir, "claude")
		if err := os.WriteFile(bin, []byte(""), 0o755); err != nil {
			t.Fatal(err)
		}
		if verified {
			if err := os.WriteFile(filepath.Join(base, version, ".verified"), []byte(""), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return bin
	}

	mkVersion("1.9.0", true)
	winner := mkVersion("2.3.4", true)
	mkVersion("9.9.9", false) // higher semver but not verified: must lose

	f := env.Runner.(*runnertest.Fake)
	verifiedOK(f, winner)

	cli, err := Locate(context.Background(), env)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if cli.Path != winner {
		t.Errorf("Path = %q, want %q (highest verified)", cli.Path, winner)
	}
}

func TestLocate_Windows_BoundedDepth(t *testing.T) {
	env := baseEnv(t, "windows")
	env.LocalAppData = filepath.Join(env.Home, "AppData", "Local")
	env.AppData = filepath.Join(env.Home, "AppData", "Roaming")

	shallow := filepath.Join(env.LocalAppData, "Programs", "Claude", "app-1.2.3", "claude.exe")
	if err := os.MkdirAll(filepath.Dir(shallow), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shallow, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}

	// Nested past depth 4 must not be found.
	tooDeep := filepath.Join(env.LocalAppData, "Programs", "Claude", "a", "b", "c", "d", "e", "claude.exe")
	if err := os.MkdirAll(filepath.Dir(tooDeep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tooDeep, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}

	f := env.Runner.(*runnertest.Fake)
	verifiedOK(f, shallow)

	cli, err := Locate(context.Background(), env)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if cli.Path != shallow {
		t.Errorf("Path = %q, want %q", cli.Path, shallow)
	}
}

func TestLocate_WindowsBundle_HighestSemver(t *testing.T) {
	env := baseEnv(t, "windows")
	env.AppData = filepath.Join(env.Home, "AppData", "Roaming")

	mkVersion := func(version string) string {
		dir := filepath.Join(env.AppData, "Claude", "claude-code", version)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(dir, "claude.exe")
		if err := os.WriteFile(bin, []byte(""), 0o755); err != nil {
			t.Fatal(err)
		}
		return bin
	}

	mkVersion("2.1.258") // no .verified anywhere: ground truth says it's unknown whether Windows writes one
	winner := mkVersion("2.1.260")

	f := env.Runner.(*runnertest.Fake)
	verifiedOK(f, winner)

	cli, err := Locate(context.Background(), env)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if cli.Path != winner {
		t.Errorf("Path = %q, want %q (highest semver)", cli.Path, winner)
	}
	if cli.Source != SourceDesktopBundle {
		t.Errorf("Source = %q, want %q", cli.Source, SourceDesktopBundle)
	}
}

// mkWindowsGroundTruth builds a fake tree mirroring the real Windows 11
// ground truth exactly: two Roaming bundle version dirs with no ".verified"
// marker, plus the native installer's ~/.local/bin/claude.exe.
func mkWindowsGroundTruth(t *testing.T, env *Env) (localBin string) {
	t.Helper()
	env.AppData = filepath.Join(env.Home, "AppData", "Roaming")

	for _, v := range []string{"2.1.258", "2.1.260"} {
		dir := filepath.Join(env.AppData, "Claude", "claude-code", v)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "claude.exe"), []byte(""), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	localBin = filepath.Join(env.Home, ".local", "bin", "claude.exe")
	if err := os.MkdirAll(filepath.Dir(localBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localBin, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	return localBin
}

func TestLocate_WindowsGroundTruth_PathHit(t *testing.T) {
	env := baseEnv(t, "windows")
	mkWindowsGroundTruth(t, &env)

	pathBin := filepath.Join(t.TempDir(), "claude.exe")
	if err := os.WriteFile(pathBin, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	env.LookPath = func(string) (string, error) { return pathBin, nil }

	f := env.Runner.(*runnertest.Fake)
	verifiedOK(f, pathBin)

	cli, err := Locate(context.Background(), env)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if cli.Path != pathBin {
		t.Errorf("Path = %q, want %q", cli.Path, pathBin)
	}
	if cli.Source != SourcePath {
		t.Errorf("Source = %q, want %q", cli.Source, SourcePath)
	}
}

func TestLocate_WindowsGroundTruth_PathEmpty(t *testing.T) {
	env := baseEnv(t, "windows")
	localBin := mkWindowsGroundTruth(t, &env)

	f := env.Runner.(*runnertest.Fake)
	verifiedOK(f, localBin)

	cli, err := Locate(context.Background(), env)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if cli.Path != localBin {
		t.Errorf("Path = %q, want %q", cli.Path, localBin)
	}
	if cli.Source != SourceLocalBin {
		t.Errorf("Source = %q, want %q", cli.Source, SourceLocalBin)
	}
}

func TestLocate_NotFound(t *testing.T) {
	env := baseEnv(t, "linux")
	if _, err := Locate(context.Background(), env); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// Reverse-lexical order put app-1.9.0 above app-1.10.0, so the probe verified
// and adopted the older CLI while the UI reported its version as the one in
// use. The ordered bundle path two functions over always got this right.
func TestLocate_WindowsProbe_HighestVersionFirst(t *testing.T) {
	env := baseEnv(t, "windows")
	env.LocalAppData = filepath.Join(env.Home, "AppData", "Local")
	env.AppData = filepath.Join(env.Home, "AppData", "Roaming")

	root := filepath.Join(env.LocalAppData, "Programs", "Claude")
	var newest string
	for _, v := range []string{"app-1.9.0", "app-1.10.0", "app-1.2.3"} {
		bin := filepath.Join(root, v, "claude.exe")
		if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(bin, []byte(""), 0o755); err != nil {
			t.Fatal(err)
		}
		if v == "app-1.10.0" {
			newest = bin
		}
	}

	// Every candidate verifies, so only the order decides which is adopted.
	f := env.Runner.(*runnertest.Fake)
	for _, v := range []string{"app-1.9.0", "app-1.10.0", "app-1.2.3"} {
		verifiedOK(f, filepath.Join(root, v, "claude.exe"))
	}

	cli, err := Locate(context.Background(), env)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if cli.Path != newest {
		t.Errorf("Path = %q, want the newest %q", cli.Path, newest)
	}
}

// A version directory carrying a pre-release suffix must still order by its
// release version. Taking everything after the last separator read
// "app-1.10.0-beta.2" as "beta.2", which is not a version, so that directory
// sorted last and an older sibling was adopted instead.
func TestVersionSuffix(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"app-1.10.0", "1.10.0"},
		{"app-1.9.0", "1.9.0"},
		{"app-1.10.0-beta.2", "1.10.0"},
		{"2.1.258-nightly.1", "2.1.258"},
		{"app-v2.1.3", "2.1.3"},
		{"1.2.3", "1.2.3"},
		{"app", ""},
		{"Claude", ""},
	} {
		if got := versionSuffix(tc.in); got != tc.want {
			t.Errorf("versionSuffix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
