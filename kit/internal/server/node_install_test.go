package server

import (
	"net/http"
	"testing"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/nodeinstall"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner/runnertest"
)

// node.install is the other action that must work with no claude CLI on the
// machine: nothing about installing Node.js needs one. The plan comes from the
// same env the handler builds, so this covers whichever platform runs the test.
func TestHandler_NodeInstall_RunsEveryStepWithoutCLI(t *testing.T) {
	plan := nodeinstall.Plan((&Server{nodeEnv: noNodeEnv}).nodeInstallEnv())
	fake := runnertest.NewFake()
	for _, cmd := range plan {
		fake.Set(cmd.Name, cmd.Args, runnertest.Result{Stdout: "ok\n"})
	}
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateNone})

	resp := post(t, baseURL+"/api/actions/node.install", token, nil)
	if len(plan) == 0 {
		// Windows without winget: there is nothing to run, so the action is
		// refused and the doctor row keeps its nodejs.org link.
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		return
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if status := waitForJob(t, baseURL, token, jobID(t, resp)); status != "succeeded" {
		t.Fatalf("job status = %q, want succeeded", status)
	}

	var got []string
	for _, c := range fake.Calls {
		got = append(got, runner.Argv(c.Name, c.Args))
	}
	if len(got) != len(plan) {
		t.Fatalf("ran %v, want the %d planned commands", got, len(plan))
	}
	for i, cmd := range plan {
		if want := runner.Argv(cmd.Name, cmd.Args); got[i] != want {
			t.Errorf("call %d = %q, want %q", i, got[i], want)
		}
	}
}
