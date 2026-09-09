package server

import (
	"context"
	"net/http"
	"runtime"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/nodeinstall"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner"
)

// nodeInstallEnv describes this machine to the nodeinstall package, through
// the same seam DetectNode uses, so a test can point it at a temp home.
func (s *Server) nodeInstallEnv() nodeinstall.Env {
	env := s.nodeEnv(s.runner)
	return nodeinstall.Env{GOOS: runtime.GOOS, Home: env.Home, LookPath: env.LookPath, Stat: env.Stat}
}

// handleNodeInstall installs or updates Node.js: nvm on macOS and Linux,
// winget on Windows. It needs no claude CLI, and the page asks for
// confirmation before it is called.
func (s *Server) handleNodeInstall(w http.ResponseWriter, ctx context.Context, action string) {
	plan := nodeinstall.Plan(s.nodeInstallEnv())
	if len(plan) == 0 {
		httpError(w, http.StatusBadRequest,
			"no Node.js installer on this machine; install it from nodejs.org")
		return
	}
	s.startJob(w, ctx, action, noCLI, nodeInstallJobTimeout, func(jctx context.Context, onLine func(string)) (int, error) {
		for _, cmd := range plan {
			onLine(runner.Redact(runner.Argv(cmd.Name, cmd.Args)))
			code, err := s.runner.Stream(jctx, cmd.Name, cmd.Args, func(l runner.Line) {
				onLine(runner.Redact(l.Text))
			})
			if err != nil || code != 0 {
				return code, err
			}
		}
		s.nodeCache.invalidate()
		return 0, nil
	})
}
