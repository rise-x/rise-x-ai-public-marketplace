package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/doctor"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/mcp"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner/runnertest"
)

// staleClaudeJSON has one stale Rise-X connection at user scope and one in a
// project, plus a server that is already on the current address.
const staleClaudeJSON = `{
  "mcpServers": {
    "rise-x": {"type": "http", "url": "https://mcp-server-prod.bluesea.eastus.azurecontainerapps.io/mcp"},
    "context7": {"type": "http", "url": "https://mcp.context7.com/mcp"}
  },
  "projects": {
    "PROJECT_PATH": {
      "mcpServers": {
        "rise-x-test": {"type": "http", "url": "https://mcp-server-test.bluesea.eastus.azurecontainerapps.io/mcp"}
      }
    }
  }
}`

// staleServer starts a kit that sees both stale connections and can answer the
// remove/add pair each one needs.
func staleServer(t *testing.T) (baseURL, token, projectPath string, fake *runnertest.Fake) {
	t.Helper()
	projectPath = t.TempDir()
	path := filepath.Join(t.TempDir(), ".claude.json")
	body := []byte(strings.ReplaceAll(staleClaudeJSON, "PROJECT_PATH", projectPath))
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	fake = newFakeCLI(pluginListFixture)
	for _, args := range [][]string{
		{"mcp", "remove", "rise-x", "-s", "user"},
		{"mcp", "add", "--transport", "http", "rise-x", "https://mcp.rise-x.io/mcp", "-s", "user"},
		{"mcp", "remove", "rise-x-test", "-s", "local"},
		{"mcp", "add", "--transport", "http", "rise-x-test", "https://mcp-test.rise-x.io/mcp", "-s", "local"},
	} {
		fake.Set(fakeCLIPath, args, runnertest.Result{Stdout: "ok\n"})
	}
	baseURL, token = newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath),
		ClaudeJSONPath: path})
	return baseURL, token, projectPath, fake
}

func TestGather_StaleMcp_ReportedOnOverviewAndDoctor(t *testing.T) {
	baseURL, token, projectPath, _ := staleServer(t)

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	if got.Mcp == nil || len(got.Mcp.Stale) != 2 {
		t.Fatalf("mcp = %+v, want two stale entries", got.Mcp)
	}
	user, local := got.Mcp.Stale[0], got.Mcp.Stale[1]
	if user.Name != "rise-x" || user.Scope != mcp.ScopeUser ||
		user.SuggestedURL != "https://mcp.rise-x.io/mcp" {
		t.Errorf("user entry = %+v", user)
	}
	if local.Name != "rise-x-test" || local.Scope != mcp.ScopeLocal ||
		local.ProjectPath != projectPath || local.SuggestedURL != "https://mcp-test.rise-x.io/mcp" {
		t.Errorf("local entry = %+v", local)
	}

	c := doctorCheck(t, baseURL, token, "mcp.stale")
	if c.Status != doctor.StatusWarn || c.Fix != "mcp.fix" || c.Detail == "" {
		t.Fatalf("doctor row = %+v", c)
	}
}

// One row's Fix repoints just that connection, and the local one runs in its
// own project so `-s local` writes to the right file.
func TestHandler_McpFix_OneLocalEntry(t *testing.T) {
	baseURL, token, projectPath, fake := staleServer(t)

	resp := post(t, baseURL+"/api/actions/mcp.fix", token, map[string]any{
		"name": "rise-x-test", "scope": "local", "projectPath": projectPath})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if status := waitForJob(t, baseURL, token, jobID(t, resp)); status != "succeeded" {
		t.Fatalf("job status = %q", status)
	}

	remove := findCall(t, fake, []string{"mcp", "remove", "rise-x-test", "-s", "local"})
	add := findCall(t, fake, []string{"mcp", "add", "--transport", "http", "rise-x-test",
		"https://mcp-test.rise-x.io/mcp", "-s", "local"})
	if remove > add {
		t.Fatalf("add ran before remove: %+v", fake.Calls)
	}
	if dir := fake.Calls[add].Dir; dir != projectPath {
		t.Fatalf("add ran in %q, want the project path %q", dir, projectPath)
	}
	if dir := fake.Calls[remove].Dir; dir != projectPath {
		t.Fatalf("remove ran in %q, want the project path %q", dir, projectPath)
	}
	// The user-scope entry was not named, so it must be untouched.
	if calledWith(fake, []string{"mcp", "remove", "rise-x", "-s", "user"}) {
		t.Fatalf("the user-scope entry was changed too: %+v", fake.Calls)
	}
}

// The doctor's own Fix repoints every connection the CLI can reach; a user
// scope entry runs in no particular directory.
func TestHandler_McpFix_All(t *testing.T) {
	baseURL, token, projectPath, fake := staleServer(t)

	resp := post(t, baseURL+"/api/actions/mcp.fix", token, nil)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if status := waitForJob(t, baseURL, token, jobID(t, resp)); status != "succeeded" {
		t.Fatalf("job status = %q", status)
	}

	user := findCall(t, fake, []string{"mcp", "add", "--transport", "http", "rise-x",
		"https://mcp.rise-x.io/mcp", "-s", "user"})
	if dir := fake.Calls[user].Dir; dir != "" {
		t.Fatalf("user scope ran in %q, want the kit's own directory", dir)
	}
	local := findCall(t, fake, []string{"mcp", "add", "--transport", "http", "rise-x-test",
		"https://mcp-test.rise-x.io/mcp", "-s", "local"})
	if dir := fake.Calls[local].Dir; dir != projectPath {
		t.Fatalf("local scope ran in %q, want %q", dir, projectPath)
	}
}

// Only a connection the scan found may reach the argv.
func TestHandler_McpFix_UnknownTarget_400(t *testing.T) {
	baseURL, token, _, _ := staleServer(t)

	for _, body := range []map[string]any{
		{"name": "attacker", "scope": "user"},
		{"name": "rise-x", "scope": "local", "projectPath": "/somewhere/else"},
		{"name": "context7", "scope": "user"},
	} {
		resp := post(t, baseURL+"/api/actions/mcp.fix", token, body)
		if resp.StatusCode != http.StatusBadRequest {
			resp.Body.Close()
			t.Fatalf("%+v status = %d, want 400", body, resp.StatusCode)
		}
		if got := errorMessage(t, resp); got != "unknown stale connection target" {
			t.Fatalf("%+v error = %q", body, got)
		}
	}
}

// A connector the Desktop app configured is beyond the CLI, so the action
// refuses it and says where to change it.
func TestHandler_McpFix_DesktopEntry_400(t *testing.T) {
	dataDir := t.TempDir()
	desktop := `{"mcpServers":{"rise-x":{"type":"http","url":"https://mcp-old.bluesea.eastus.azurecontainerapps.io/mcp"}}}`
	if err := os.WriteFile(filepath.Join(dataDir, "claude_desktop_config.json"), []byte(desktop), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := newFakeCLI(pluginListFixture)
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath),
		DesktopDataDir: dataDir})

	if c := doctorCheck(t, baseURL, token, "mcp.stale"); c.Status != doctor.StatusWarn || c.Fix != "" {
		t.Fatalf("doctor row = %+v, want a warn with no fix", c)
	}
	resp := post(t, baseURL+"/api/actions/mcp.fix", token, nil)
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if got := errorMessage(t, resp); got !=
		"nothing the CLI can change; update this one in Claude Desktop under Settings, Connectors" {
		t.Fatalf("error = %q", got)
	}
}

// findCall returns the index of the call made with args.
func findCall(t *testing.T, fake *runnertest.Fake, args []string) int {
	t.Helper()
	i := callIndex(fake, args)
	if i < 0 {
		t.Fatalf("no call with %v in %+v", args, fake.Calls)
	}
	return i
}
