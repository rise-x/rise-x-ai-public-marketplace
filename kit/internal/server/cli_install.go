package server

import (
	"context"
	"fmt"
	"runtime"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner"
)

// runCLIInstall runs Claude Code's own one-line installer for the host OS.
func runCLIInstall(ctx context.Context, r runner.Runner, onLine func(string)) (int, error) {
	name, args, err := installCommand(runtime.GOOS)
	if err != nil {
		return -1, err
	}
	if onLine != nil {
		onLine(runner.Redact(runner.Argv(name, args)))
	}
	return r.Stream(ctx, name, args, func(l runner.Line) {
		if onLine != nil {
			onLine(runner.Redact(l.Text))
		}
	})
}

func installCommand(goos string) (name string, args []string, err error) {
	switch goos {
	case "darwin", "linux":
		return "bash", []string{"-c", "curl -fsSL https://claude.ai/install.sh | bash"}, nil
	case "windows":
		return "powershell", []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
			"irm https://claude.ai/install.ps1 | iex"}, nil
	default:
		return "", nil, fmt.Errorf("no installer for GOOS=%s", goos)
	}
}
