package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/catalog"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/claudecli"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner/runnertest"
)

// riseXMcpJSON is the rise-x-mcp plugin's real .mcp.json: two servers, named
// without any "plugin:" prefix.
const riseXMcpJSON = `{"mcpServers":{
  "rise-x":{"type":"http","url":"https://mcp.rise-x.io/mcp"},
  "rise-x-test":{"type":"http","url":"https://mcp-test.rise-x.io/mcp"}
}}`

func mcpPluginList(t *testing.T) string {
	t.Helper()
	installPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(installPath, ".mcp.json"), []byte(riseXMcpJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return `{
  "installed": [{"id":"rise-x-mcp@rise-x-public","version":"1.3.1","scope":"user","enabled":true,"installPath":"` + installPath + `"}],
  "available": [{"pluginId":"rise-x-mcp@rise-x-public","name":"rise-x-mcp","marketplaceName":"rise-x-public","source":"./plugins/rise-x-mcp"}]
}`
}

// A mirror of the public marketplace installs the same plugin under its own
// name, and its bundled servers are the same servers.
func TestRiseXMcpConfig_MirrorMarketplace(t *testing.T) {
	installPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(installPath, ".mcp.json"), []byte(riseXMcpJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	got := riseXMcpConfig([]claudecli.InstalledPlugin{
		{ID: "other-plugin@rise-x-public", InstallPath: t.TempDir()},
		{ID: "rise-x-mcp@acme-mirror", InstallPath: installPath},
	})
	names := make([]string, len(got))
	for i, cs := range got {
		names[i] = cs.Name
	}
	if len(names) != 2 || names[0] != "rise-x" || names[1] != "rise-x-test" {
		t.Fatalf("configured = %v, want the mirror's two servers", names)
	}
}

// With both installed, the public copy is the one the rest of the kit acts on.
func TestRiseXMcpConfig_PrefersPublicMarketplace(t *testing.T) {
	public := t.TempDir()
	if err := os.WriteFile(filepath.Join(public, ".mcp.json"),
		[]byte(`{"mcpServers":{"from-public":{"type":"http","url":"https://mcp.rise-x.io/mcp"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mirror := t.TempDir()
	if err := os.WriteFile(filepath.Join(mirror, ".mcp.json"), []byte(riseXMcpJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	got := riseXMcpConfig([]claudecli.InstalledPlugin{
		{ID: "rise-x-mcp@acme-mirror", InstallPath: mirror},
		{ID: "rise-x-mcp@rise-x-public", InstallPath: public},
	})
	if len(got) != 1 || got[0].Name != "from-public" {
		t.Fatalf("configured = %+v, want the public copy's server", got)
	}
}

func errorMessage(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return out.Error
}

func jobID(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	var out struct {
		JobID string `json:"jobId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.JobID
}

// mcp.login takes the bare names from the plugin's .mcp.json. The page sends
// overview.mcp.configured[].name, which is exactly those; the prefixed session
// name from `claude mcp list` is not a valid target.
func TestHandler_McpLogin_AcceptsBareConfiguredNames(t *testing.T) {
	fake := newFakeCLI(mcpPluginList(t))
	for _, server := range []string{"rise-x", "rise-x-test"} {
		fake.Set(fakeCLIPath, []string{"mcp", "login", "plugin:rise-x-mcp:" + server},
			runnertest.Result{Stdout: "ok\n"})
	}
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath)})

	for _, server := range []string{"rise-x", "rise-x-test"} {
		resp := post(t, baseURL+"/api/actions/mcp.login", token, map[string]any{"server": server})
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("%s status = %d, want 200", server, resp.StatusCode)
		}
		// One job at a time, so let this one land before starting the next.
		if status := waitForJob(t, baseURL, token, jobID(t, resp)); status != "succeeded" {
			t.Fatalf("%s job status = %q", server, status)
		}
	}

	resp := post(t, baseURL+"/api/actions/mcp.login", token, map[string]any{"server": "plugin:rise-x-mcp:rise-x"})
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("prefixed target status = %d, want 400", resp.StatusCode)
	}
	if got := errorMessage(t, resp); got != "unknown MCP server target; expected one of: rise-x, rise-x-test" {
		t.Fatalf("error = %q", got)
	}
}

// catalogNamesServer serves the marketplace manifest GitHub would.
func catalogNamesServer(t *testing.T, names ...string) *catalog.Catalog {
	t.Helper()
	entries := make([]string, len(names))
	for i, n := range names {
		entries[i] = `{"name":"` + n + `","source":"./plugins/` + n + `"}`
	}
	manifest := `{"name":"rise-x-public","plugins":[` + strings.Join(entries, ",") + `]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "marketplace.json") {
			_, _ = w.Write([]byte(manifest))
			return
		}
		_, _ = w.Write([]byte(`{"version":"1.5.0","sha":"deadbeef"}`))
	}))
	t.Cleanup(srv.Close)
	c := catalog.New(claudecli.MarketplaceRepo)
	c.RawBaseURL, c.APIBaseURL = srv.URL, srv.URL
	return c
}

const threePluginList = `{
  "installed": [],
  "available": [
    {"pluginId":"rise-x-mcp@rise-x-public","name":"rise-x-mcp","marketplaceName":"rise-x-public","source":"./plugins/rise-x-mcp"},
    {"pluginId":"rise-x-apps@rise-x-public","name":"rise-x-apps","marketplaceName":"rise-x-public","source":"./plugins/rise-x-apps"},
    {"pluginId":"rise-x-newthing@rise-x-public","name":"rise-x-newthing","marketplaceName":"rise-x-public","source":"./plugins/rise-x-newthing"}
  ]
}`

// A plugin the marketplace has just started shipping is installable straight
// after process start, with no page load first.
func TestHandler_PluginInstall_NewCatalogPluginBeforeOverview(t *testing.T) {
	fake := newFakeCLI(threePluginList)
	fake.Set(fakeCLIPath, []string{"plugin", "install", "rise-x-newthing@rise-x-public"},
		runnertest.Result{Stdout: "installed\n"})
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath),
		Catalog: catalogNamesServer(t, "rise-x-mcp", "rise-x-apps", "rise-x-newthing")})

	resp := post(t, baseURL+"/api/actions/plugin.install", token, map[string]any{"name": "rise-x-newthing"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// With no marketplace registered there is no "available" list at all, so the
// names come from the public marketplace.json. rise-x-onlyremote is in that
// manifest and nowhere else, so nothing but the manifest can have accepted it.
func TestHandler_PluginInstall_UnregisteredMarketplaceUsesCatalog(t *testing.T) {
	fake := newFakeCLI(threePluginList)
	fake.Set(fakeCLIPath, []string{"plugin", "marketplace", "list", "--json"}, runnertest.Result{Stdout: `[]`})
	fake.Set(fakeCLIPath, []string{"plugin", "marketplace", "add", claudecli.MarketplaceRepo},
		runnertest.Result{Stdout: "added\n"})
	fake.Set(fakeCLIPath, []string{"plugin", "install", "rise-x-onlyremote@rise-x-public"},
		runnertest.Result{Stdout: "installed\n"})
	baseURL, token := newServer(t, Config{
		Runner:    fake,
		LocateEnv: locateAt(fakeCLIPath),
		Catalog:   catalogNamesServer(t, "rise-x-mcp", "rise-x-apps", "rise-x-onlyremote"),
	})

	resp := post(t, baseURL+"/api/actions/plugin.install", token, map[string]any{"name": "rise-x-onlyremote"})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if status := waitForJob(t, baseURL, token, jobID(t, resp)); status != "succeeded" {
		t.Fatalf("job status = %q", status)
	}

	bad := post(t, baseURL+"/api/actions/plugin.install", token, map[string]any{"name": "not-in-the-catalog"})
	if bad.StatusCode != http.StatusBadRequest {
		bad.Body.Close()
		t.Fatalf("unknown plugin status = %d, want 400", bad.StatusCode)
	}
	if got := errorMessage(t, bad); got != "unknown plugin target" {
		t.Fatalf("error = %q", got)
	}
}

// A registered marketplace whose "available" list is empty still knows its own
// plugins: they are listed in the clone's marketplace.json.
func TestHandler_PluginInstall_EmptyAvailableUsesLocalClone(t *testing.T) {
	installLocation := t.TempDir()
	manifestDir := filepath.Join(installLocation, ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"rise-x-public","plugins":[{"name":"rise-x-mcp"},{"name":"rise-x-newthing"}]}`
	if err := os.WriteFile(filepath.Join(manifestDir, "marketplace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := newFakeCLI(`{"installed": [], "available": []}`)
	fake.Set(fakeCLIPath, []string{"plugin", "marketplace", "list", "--json"}, runnertest.Result{
		Stdout: strings.ReplaceAll(marketplaceListFixture, "INSTALL_LOCATION", installLocation)})
	fake.Set(fakeCLIPath, []string{"plugin", "install", "rise-x-newthing@rise-x-public"},
		runnertest.Result{Stdout: "installed\n"})
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath),
		Catalog: catalogNamesServer(t, "rise-x-mcp", "rise-x-apps", "rise-x-newthing")})

	resp := post(t, baseURL+"/api/actions/plugin.install", token, map[string]any{"name": "rise-x-newthing"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
