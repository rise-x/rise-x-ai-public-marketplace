package server

import (
	"context"
	"net/http"
	"os"
	"runtime"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/nodeinstall"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner"
)

// nodeInstallEnv describes this machine to the nodeinstall package, through
// the same seam DetectNode uses, so a test can point it at a temp home.
func (s *Server) nodeInstallEnv(nodeFound bool) nodeinstall.Env {
	env := s.nodeEnv(s.runner)
	return nodeinstall.Env{
		GOOS: runtime.GOOS, Home: env.Home,
		NvmDir: os.Getenv("NVM_DIR"), XDGConfigHome: os.Getenv("XDG_CONFIG_HOME"),
		LookPath: env.LookPath, Stat: env.Stat, NodeFound: nodeFound,
	}
}

// handleNodeInstall installs or updates Node.js: nvm on macOS and Linux,
// winget on Windows. It needs no claude CLI, and the page asks for
// confirmation before it is called.
func (s *Server) handleNodeInstall(w http.ResponseWriter, ctx context.Context, action string) {
	plan := nodeinstall.Plan(s.nodeInstallEnv(s.nodeInstalled()))
	if len(plan) == 0 {
		httpError(w, http.StatusBadRequest,
			"no Node.js installer on this machine; install it from nodejs.org")
		return
	}
	s.startJob(w, ctx, action, noCLI, nodeInstallJobTimeout, func(jctx context.Context, onLine func(string)) (int, error) {
		stream := func(name string, args []string) (int, error) {
			onLine(runner.Redact(runner.Argv(name, args)))
			return s.runner.Stream(jctx, name, args, func(l runner.Line) {
				onLine(runner.Redact(l.Text))
			})
		}
		for _, cmd := range plan {
			code, err := stream(cmd.Name, cmd.Args)
			if err == nil && cmd.Recovers(code) {
				// winget has no upgrade for a Node it did not install; that is
				// the wrong verb for this machine, not a failed job.
				code, err = stream(cmd.Name, cmd.FallbackArgs)
			}
			if err != nil || code != 0 {
				return code, err
			}
		}
		s.nodeCache.invalidate()
		return 0, nil
	})
}
