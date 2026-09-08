package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/catalog"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/claudecli"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/doctor"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/mcp"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner/runnertest"
)

// syncedManifest lists rise-x-mcp twice, as the real one on this kind of
// machine does: chosen by the account holder, and pushed by the organisation.
const syncedManifest = `{"lastUpdated":1788855954758,"plugins":[
  {"id":"plugin_user","name":"rise-x-mcp","marketplaceName":"rise-x-ai-public-marketplace",
   "installedBy":"user","installationPreference":"available"},
  {"id":"plugin_org","name":"rise-x-mcp","marketplaceName":"rise-x/rise-x-ai-marketplace",
   "installedBy":"auto","installationPreference":"auto_install"}
]}`

// syncedDesktopDir builds the Desktop app's data directory with both
// rise-x-mcp entries materialised at version, each carrying the plugin's real
// .mcp.json.
func syncedDesktopDir(t *testing.T, version string) string {
	t.Helper()
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "local-agent-mode-sessions", "org-1", "account-1", "rpm")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(syncedManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"plugin_user", "plugin_org"} {
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := `{"name":"rise-x-mcp","version":"` + version + `"}`
		if err := os.WriteFile(filepath.Join(dir, ".claude-plugin", "plugin.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(riseXMcpJSON), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dataDir
}

// versionCatalog answers every plugin.json with version, so a test can put the
// public catalog ahead of or level with the machine.
func versionCatalog(t *testing.T, version string) *catalog.Catalog {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "marketplace.json"):
			_, _ = w.Write([]byte(`{"name":"rise-x-public","plugins":[{"name":"rise-x-mcp"}]}`))
		default:
			_, _ = w.Write([]byte(`{"version":"` + version + `","sha":"deadbeef"}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := catalog.New(claudecli.MarketplaceRepo)
	c.RawBaseURL, c.APIBaseURL = srv.URL, srv.URL
	return c
}

// syncedServer starts a kit whose only rise-x-mcp is the account-synced one:
// `claude plugin list` reports nothing installed.
func syncedServer(t *testing.T, localVersion, publicVersion string) (baseURL, token string) {
	t.Helper()
	fake := newFakeCLI(`{"installed": [], "available": [
      {"pluginId":"rise-x-mcp@rise-x-public","name":"rise-x-mcp","marketplaceName":"rise-x-public","source":"./plugins/rise-x-mcp"}
    ]}`)
	return newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath),
		Catalog: versionCatalog(t, publicVersion), DesktopDataDir: syncedDesktopDir(t, localVersion)})
}

func pluginInfo(t *testing.T, baseURL, token, name string) PluginInfo {
	t.Helper()
	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	for _, p := range got.Plugins {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no plugin %q in %+v", name, got.Plugins)
	return PluginInfo{}
}

// A skill the organisation pushed shows as installed at its own version, from
// the organisation, and offers nothing to press.
func TestGather_SyncedOrgPlugin(t *testing.T) {
	baseURL, token := syncedServer(t, "1.3.4", "1.3.4")

	p := pluginInfo(t, baseURL, token, "rise-x-mcp")
	if !p.Installed || !p.Enabled {
		t.Fatalf("plugin = %+v, want installed and enabled", p)
	}
	if p.InstallSource != doctor.SourceOrganisation {
		t.Fatalf("installSource = %q, want %q", p.InstallSource, doctor.SourceOrganisation)
	}
	if p.SourceName != "rise-x/rise-x-ai-marketplace" {
		t.Fatalf("sourceName = %q", p.SourceName)
	}
	if p.LocalVersion != "1.3.4" || p.UpdateAvailable {
		t.Fatalf("versions = %+v", p)
	}

	c := doctorCheck(t, baseURL, token, "plugin.rise-x-mcp")
	if c.Status != doctor.StatusOK || c.Fix != "" {
		t.Fatalf("doctor row = %+v, want an ok with no fix", c)
	}
	if c.Message != "Installed by your organisation (1.3.4)." {
		t.Fatalf("message = %q", c.Message)
	}
}

// Behind the public version, the row still offers nothing: updating from the
// public marketplace would install a second copy.
func TestGather_SyncedOrgPlugin_BehindPublic(t *testing.T) {
	baseURL, token := syncedServer(t, "1.3.3", "1.3.5")

	p := pluginInfo(t, baseURL, token, "rise-x-mcp")
	if p.LocalVersion != "1.3.3" || p.PublicVersion != "1.3.5" || !p.UpdateAvailable {
		t.Fatalf("plugin = %+v", p)
	}
	if c := doctorCheck(t, baseURL, token, "plugin.rise-x-mcp"); c.Fix != "" {
		t.Fatalf("doctor row = %+v, want no fix", c)
	}
}

// `claude mcp list` cannot see a synced plugin's servers, so their URLs come
// from the plugin directory instead, and the card says who manages them.
func TestGather_SyncedPlugin_ConnectionFromPluginDir(t *testing.T) {
	baseURL, token := syncedServer(t, "1.3.4", "1.3.4")

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	if got.Mcp == nil {
		t.Fatal("no mcp block")
	}
	if got.Mcp.Verdict != string(mcp.VerdictManaged) {
		t.Fatalf("verdict = %q, want managed", got.Mcp.Verdict)
	}
	urls := map[string]string{}
	for _, cs := range got.Mcp.Configured {
		urls[cs.Name] = cs.URL
	}
	if urls["rise-x"] != "https://mcp.rise-x.io/mcp" || urls["rise-x-test"] != "https://mcp-test.rise-x.io/mcp" {
		t.Fatalf("configured = %+v", got.Mcp.Configured)
	}
}

// With every skill delivered by the organisation, the marketplace rows stop
// asking for a marketplace nothing here needs.
func TestGather_SyncedOrgPlugin_MarketplaceRowsSatisfied(t *testing.T) {
	fake := newFakeCLI(`{"installed": [], "available": []}`)
	fake.Set(fakeCLIPath, []string{"plugin", "marketplace", "list", "--json"}, runnertest.Result{Stdout: `[]`})
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath),
		Catalog: versionCatalog(t, "1.3.4"), DesktopDataDir: syncedDesktopDir(t, "1.3.4")})

	if c := doctorCheck(t, baseURL, token, "marketplace.registered"); c.Status != doctor.StatusOK ||
		c.Message != "Skills come from your organisation." {
		t.Fatalf("marketplace.registered = %+v", c)
	}
	if c := doctorCheck(t, baseURL, token, "marketplace.autoupdate"); c.Status != doctor.StatusOK ||
		c.Message != "Your organisation delivers skill updates automatically." {
		t.Fatalf("marketplace.autoupdate = %+v", c)
	}
}

// mirrorPluginList is `claude plugin list --json --available` on a machine
// where rise-x-mcp came from a mirror of the public marketplace.
const mirrorPluginList = `{
  "installed": [
    {"id":"rise-x-mcp@rise-x","version":"1.3.3","scope":"user","enabled":true,"installPath":"/tmp/mirror"}
  ],
  "available": [
    {"pluginId":"rise-x-mcp@rise-x-public","name":"rise-x-mcp","marketplaceName":"rise-x-public","source":"./plugins/rise-x-mcp"}
  ]
}`

const mirrorMarketplaceList = `[
  {"name":"rise-x-public","source":"github","repo":"rise-x/rise-x-ai-public-marketplace","installLocation":"INSTALL_LOCATION"},
  {"name":"rise-x","source":"github","repo":"rise-x/rise-x-ai-marketplace","installLocation":"INSTALL_LOCATION"}
]`

func mirrorServer(t *testing.T) (baseURL, token string, fake *runnertest.Fake) {
	t.Helper()
	fake = newFakeCLI(mirrorPluginList)
	fake.Set(fakeCLIPath, []string{"plugin", "marketplace", "list", "--json"},
		runnertest.Result{Stdout: strings.ReplaceAll(mirrorMarketplaceList, "INSTALL_LOCATION", t.TempDir())})
	fake.Set(fakeCLIPath, []string{"plugin", "update", "rise-x-mcp@rise-x"},
		runnertest.Result{Stdout: "updated\n"})
	baseURL, token = newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath),
		Catalog: versionCatalog(t, "1.3.4")})
	return baseURL, token, fake
}

// A skill installed from a mirror of the public marketplace is named as such,
// and updates from that mirror.
func TestGather_OtherMarketplaceInstall(t *testing.T) {
	baseURL, token, fake := mirrorServer(t)

	p := pluginInfo(t, baseURL, token, "rise-x-mcp")
	if p.InstallSource != doctor.SourceMarketplace || p.SourceName != "rise-x" {
		t.Fatalf("plugin = %+v", p)
	}
	if p.LocalVersion != "1.3.3" || !p.UpdateAvailable {
		t.Fatalf("versions = %+v", p)
	}
	if c := doctorCheck(t, baseURL, token, "plugin.rise-x-mcp"); c.Status != doctor.StatusOK ||
		c.Fix != "" || c.Message != "Installed from rise-x (1.3.3)." {
		t.Fatalf("doctor row = %+v", c)
	}

	resp := post(t, baseURL+"/api/actions/plugin.update", token,
		map[string]any{"name": "rise-x-mcp", "marketplace": "rise-x"})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if status := waitForJob(t, baseURL, token, jobID(t, resp)); status != "succeeded" {
		t.Fatalf("job status = %q", status)
	}
	if !calledWith(fake, []string{"plugin", "update", "rise-x-mcp@rise-x"}) {
		t.Fatalf("calls = %+v", fake.Calls)
	}
}

// Only a marketplace Claude Code already knows may reach the argv.
func TestHandler_PluginUpdate_UnknownMarketplace_400(t *testing.T) {
	baseURL, token, _ := mirrorServer(t)

	resp := post(t, baseURL+"/api/actions/plugin.update", token,
		map[string]any{"name": "rise-x-mcp", "marketplace": "attacker-marketplace"})
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if got := errorMessage(t, resp); got != "unknown marketplace: attacker-marketplace" {
		t.Fatalf("error = %q", got)
	}
}

// The auto-update switch writes to the mirror the skills came from, and its
// entry gets no invented source when the CLI reports none.
func TestHandler_AutoUpdateSet_OtherMarketplace(t *testing.T) {
	claudeDir := t.TempDir()
	fake := newFakeCLI(mirrorPluginList)
	fake.Set(fakeCLIPath, []string{"plugin", "marketplace", "list", "--json"},
		runnertest.Result{Stdout: strings.ReplaceAll(mirrorMarketplaceList, "INSTALL_LOCATION", t.TempDir())})
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath),
		Catalog: versionCatalog(t, "1.3.4"), ClaudeDir: claudeDir})

	c := doctorCheck(t, baseURL, token, "marketplace.autoupdate")
	if c.Fix != "autoupdate.set" || c.FixArgs["marketplace"] != "rise-x" {
		t.Fatalf("doctor row = %+v", c)
	}

	resp := post(t, baseURL+"/api/actions/autoupdate.set", token,
		map[string]any{"enabled": true, "marketplace": "rise-x"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	written, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), `"rise-x"`) || !strings.Contains(string(written), `"autoUpdate": true`) {
		t.Fatalf("settings.json = %s", written)
	}
}

// An unparsable settings.json must refuse the write outright rather than
// leave the writer to fail after already deciding to touch the file.
func TestHandler_AutoUpdateSet_InvalidSettingsJSON_409(t *testing.T) {
	claudeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	baseURL, token := newServer(t, Config{
		Runner:    newFakeCLI(pluginListFixture),
		LocateEnv: locateAt(fakeCLIPath),
		ClaudeDir: claudeDir,
	})

	resp := post(t, baseURL+"/api/actions/autoupdate.set", token, map[string]any{"enabled": true})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	if got := errorMessage(t, resp); got != doctor.SettingsUnreadableMessage {
		t.Fatalf("error = %q", got)
	}
}

func calledWith(fake *runnertest.Fake, args []string) bool {
	return callIndex(fake, args) >= 0
}

// callIndex finds the first call the fake runner was asked to make with args.
func callIndex(fake *runnertest.Fake, args []string) int {
	for i, call := range fake.Calls {
		if slices.Equal(call.Args, args) {
			return i
		}
	}
	return -1
}
