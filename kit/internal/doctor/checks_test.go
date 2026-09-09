package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner/runnertest"
)

func TestDetectNode_PathHit(t *testing.T) {
	f := runnertest.NewFake()
	f.Set("/usr/bin/node", []string{"--version"}, runnertest.Result{Stdout: "v20.11.0\n"})
	env := Env{
		LookPath: func(string) (string, error) { return "/usr/bin/node", nil },
		Runner:   f,
	}
	v, found := DetectNode(env)
	if !found || v != "20.11.0" {
		t.Fatalf("DetectNode() = %q, %v", v, found)
	}
}

func TestDetectNode_NvmFallback(t *testing.T) {
	home := t.TempDir()
	nodeBin := filepath.Join(home, ".nvm", "versions", "node", "v20.11.0", "bin", "node")
	if err := os.MkdirAll(filepath.Dir(nodeBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nodeBin, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}

	f := runnertest.NewFake()
	f.Set(nodeBin, []string{"--version"}, runnertest.Result{Stdout: "v20.11.0\n"})
	env := Env{
		Home:     home,
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
		Glob:     filepath.Glob,
		Runner:   f,
	}
	v, found := DetectNode(env)
	if !found || v != "20.11.0" {
		t.Fatalf("DetectNode() = %q, %v", v, found)
	}
}

func TestDetectNode_NotFound(t *testing.T) {
	env := Env{
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
		Glob:     func(string) ([]string, error) { return nil, nil },
		Stat:     func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		Runner:   runnertest.NewFake(),
	}
	if _, found := DetectNode(env); found {
		t.Fatal("expected not found")
	}
}

func TestDetectGit(t *testing.T) {
	env := Env{LookPath: func(string) (string, error) { return "/usr/bin/git", nil }}
	if !DetectGit(env) {
		t.Fatal("expected DetectGit=true")
	}
	env.LookPath = func(string) (string, error) { return "", errors.New("no") }
	if DetectGit(env) {
		t.Fatal("expected DetectGit=false")
	}
}

// Glob returns nvm's install dirs in lexical order, which puts v9 above v22;
// the newest install is the one to report.
func TestDetectNode_NvmPicksNewest(t *testing.T) {
	home := t.TempDir()
	f := runnertest.NewFake()
	for _, v := range []string{"v18.19.0", "v22.4.0", "v9.0.0"} {
		bin := filepath.Join(home, ".nvm", "versions", "node", v, "bin", "node")
		if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(bin, []byte(""), 0o755); err != nil {
			t.Fatal(err)
		}
		f.Set(bin, []string{"--version"}, runnertest.Result{Stdout: v + "\n"})
	}

	env := Env{
		Home:     home,
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
		Glob:     filepath.Glob,
		Stat:     os.Stat,
		Runner:   f,
	}
	v, found := DetectNode(env)
	if !found || v != "22.4.0" {
		t.Fatalf("DetectNode() = %q, %v, want 22.4.0", v, found)
	}
}
