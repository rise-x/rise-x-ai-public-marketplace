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

// signedInRoot builds the Desktop app's data directory with config.json
// naming a signed-in account, and returns that account's
// local-agent-mode-sessions/<account>/<org>/rpm.
func signedInRoot(t *testing.T) (dataDir, root string) {
	t.Helper()
	dataDir = t.TempDir()
	config := `{"lastKnownAccountUuid":"account-1"}`
	if err := os.WriteFile(filepath.Join(dataDir, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(dataDir, "local-agent-mode-sessions", "account-1", "org-1", "rpm")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return dataDir, root
}

// syncedDesktopDir builds the Desktop app's data directory with both
// rise-x-mcp entries materialised at version, each carrying the plugin's real
// .mcp.json.
func syncedDesktopDir(t *testing.T, version string) string {
	t.Helper()
	dataDir, root := signedInRoot(t)
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
			_, _ = w.Write([]byte(`{"version":"` + version + `","sha":"deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"}`))
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
		runnertest.Result{Stdout: withInstallLocation(t, mirrorMarketplaceList, t.TempDir())})
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
		runnertest.Result{Stdout: withInstallLocation(t, mirrorMarketplaceList, t.TempDir())})
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
// leave the writer to fail after already deciding to touch the file. It is a
// 422, not the 409 a busy job gets: the page shows the server's own message.
func TestHandler_AutoUpdateSet_InvalidSettingsJSON_422(t *testing.T) {
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
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
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

// desktopSyncedDir is the Desktop app's data directory with one rise-x-mcp the
// account holder picked themselves - not an organisation push - materialised
// at version.
func desktopSyncedDir(t *testing.T, version string) string {
	t.Helper()
	dataDir, root := signedInRoot(t)
	dir := filepath.Join(root, "plugin_user")
	if err := os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"plugins":[{"id":"plugin_user","name":"rise-x-mcp","marketplaceName":"",` +
		`"installedBy":"user","installationPreference":"available"}]}`
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude-plugin", "plugin.json"),
		[]byte(`{"name":"rise-x-mcp","version":"`+version+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(riseXMcpJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return dataDir
}

func desktopSyncedServer(t *testing.T, localVersion, publicVersion string) (baseURL, token string) {
	t.Helper()
	fake := newFakeCLI(`{"installed": [], "available": [
      {"pluginId":"rise-x-mcp@rise-x-public","name":"rise-x-mcp","marketplaceName":"rise-x-public","source":"./plugins/rise-x-mcp"}
    ]}`)
	return newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath),
		Catalog: versionCatalog(t, publicVersion), DesktopDataDir: desktopSyncedDir(t, localVersion)})
}

// A skill only Claude Desktop knows about is reported as such. It used to read
// as "installed from another marketplace" with no marketplace name, and the
// Update button then installed a second copy from the public one.
func TestGather_DesktopSyncedPlugin(t *testing.T) {
	baseURL, token := desktopSyncedServer(t, "1.3.3", "1.3.5")

	p := pluginInfo(t, baseURL, token, "rise-x-mcp")
	if !p.Installed || !p.Enabled {
		t.Fatalf("plugin = %+v, want installed and enabled", p)
	}
	if p.InstallSource != doctor.SourceDesktop {
		t.Fatalf("installSource = %q, want %q", p.InstallSource, doctor.SourceDesktop)
	}
	if p.LocalVersion != "1.3.3" || !p.UpdateAvailable {
		t.Fatalf("versions = %+v", p)
	}

	c := doctorCheck(t, baseURL, token, "plugin.rise-x-mcp")
	if c.Status != doctor.StatusOK || c.Fix != "" {
		t.Fatalf("doctor row = %+v, want an ok with no fix", c)
	}
	if c.Message != "Installed through Claude Desktop (1.3.3)." {
		t.Fatalf("message = %q", c.Message)
	}
}

// The public marketplace must not be allowed to update a copy it did not
// install, whatever the page sends.
func TestHandler_PluginUpdate_DesktopCopy_400(t *testing.T) {
	baseURL, token := desktopSyncedServer(t, "1.3.3", "1.3.5")

	for _, body := range []map[string]any{
		{"name": "rise-x-mcp"},
		{"name": "rise-x-mcp", "marketplace": ""},
		{"name": "rise-x-mcp", "marketplace": "rise-x-public"},
	} {
		resp := post(t, baseURL+"/api/actions/plugin.update", token, body)
		if resp.StatusCode != http.StatusBadRequest {
			resp.Body.Close()
			t.Fatalf("%v status = %d, want 400", body, resp.StatusCode)
		}
		if got := errorMessage(t, resp); !strings.Contains(got, "did not come from the public marketplace") {
			t.Fatalf("%v error = %q", body, got)
		}
	}
}

// The same rule must not block the reinstall the page offers for a
// CLI-installed copy with no version stamp.
func TestHandler_PluginUpdate_PublicCopy_Allowed(t *testing.T) {
	pluginList := `{
  "installed": [{"id":"rise-x-apps@rise-x-public","version":"unknown","scope":"user","enabled":true,"installPath":"/tmp/apps"}],
  "available": [
    {"pluginId":"rise-x-mcp@rise-x-public","name":"rise-x-mcp","marketplaceName":"rise-x-public","source":"./plugins/rise-x-mcp"},
    {"pluginId":"rise-x-apps@rise-x-public","name":"rise-x-apps","marketplaceName":"rise-x-public","source":"./plugins/rise-x-apps"}
  ]
}`
	fake := newFakeCLI(pluginList)
	fake.Set(fakeCLIPath, []string{"plugin", "update", "rise-x-apps@rise-x-public"},
		runnertest.Result{Stdout: "updated\n"})
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath)})

	resp := post(t, baseURL+"/api/actions/plugin.update", token, map[string]any{"name": "rise-x-apps"})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if status := waitForJob(t, baseURL, token, jobID(t, resp)); status != "succeeded" {
		t.Fatalf("job status = %q", status)
	}
}

// The same guard has to cover install and remove, not just update: all three
// verbs run against @rise-x-public, and the page's own buttons are hidden on
// a render that may already be stale. This shipped guarded for update only.
func TestHandler_PluginInstallAndUninstall_ForeignCopy_400(t *testing.T) {
	for _, tc := range []struct {
		name   string
		server func(t *testing.T) (string, string)
		want   string
	}{
		{"desktop", func(t *testing.T) (string, string) { return desktopSyncedServer(t, "1.3.3", "1.3.5") },
			"did not come from the public marketplace"},
		{"organisation", func(t *testing.T) (string, string) { return syncedServer(t, "1.3.3", "1.3.5") },
			"did not come from the public marketplace"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			baseURL, token := tc.server(t)

			resp := post(t, baseURL+"/api/actions/plugin.uninstall", token, map[string]any{"name": "rise-x-mcp"})
			if resp.StatusCode != http.StatusBadRequest {
				resp.Body.Close()
				t.Fatalf("uninstall status = %d, want 400", resp.StatusCode)
			}
			if got := errorMessage(t, resp); !strings.Contains(got, tc.want) {
				t.Fatalf("uninstall error = %q", got)
			}

			resp = post(t, baseURL+"/api/actions/plugin.install", token, map[string]any{"name": "rise-x-mcp"})
			if resp.StatusCode != http.StatusBadRequest {
				resp.Body.Close()
				t.Fatalf("install status = %d, want 400", resp.StatusCode)
			}
			if got := errorMessage(t, resp); !strings.Contains(got, "already installed from somewhere else") {
				t.Fatalf("install error = %q", got)
			}
		})
	}
}

// The other half of the same guard: a copy installed from a mirror is not the
// public marketplace's to remove either. This branch reads the CLI's own
// list, so it only answers once a page load has paid for one.
func TestHandler_PluginUninstall_MirrorCopy_400(t *testing.T) {
	baseURL, token, _ := mirrorServer(t)
	// The page load the partner necessarily did before pressing anything.
	pluginInfo(t, baseURL, token, "rise-x-mcp")

	resp := post(t, baseURL+"/api/actions/plugin.uninstall", token, map[string]any{"name": "rise-x-mcp"})
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if got := errorMessage(t, resp); !strings.Contains(got, "did not come from the public marketplace") {
		t.Fatalf("error = %q", got)
	}
}

const needsAuthList = "Checking MCP server health…\n\n" +
	"plugin:rise-x-mcp:rise-x: https://mcp.rise-x.io/mcp (HTTP) - ! Needs authentication\n" +
	"plugin:rise-x-mcp:rise-x-test: https://mcp-test.rise-x.io/mcp (HTTP) - ! Needs authentication\n"

// writeDesktopSession writes the Desktop app's session file for the
// signed-in account, naming the connectors in body.
func writeDesktopSession(t *testing.T, dataDir, body string) {
	t.Helper()
	dir := filepath.Join(dataDir, "claude-code-sessions", "account-1", "org-1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "local_s.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A connector added in Claude Desktop lives in the account, so `claude mcp
// list` keeps reporting the plugin's own copy as needing Claude Code's sign-in
// however many times the partner signs in through the app. The connectors the
// app handed the latest Claude Code session answer instead.
func TestGather_DesktopConnectorsAnswerNeedsAuth(t *testing.T) {
	cases := []struct {
		name      string
		session   string
		verdict   string
		connected []string
	}{
		{"rise-x connector", `{"remoteMcpServersConfig":[` +
			`{"uuid":"u1","name":"Gmail","tools":[{"name":"create_draft"}]},` +
			`{"uuid":"u2","name":"Rise-X","tools":[{"name":"get_active_ecosystem"},{"name":"list_flows"}]},` +
			`{"uuid":"u3","name":"Rise-X-Test","tools":[{"name":"get_active_ecosystem"}]}]}`,
			"desktop", []string{"Rise-X", "Rise-X-Test"}},
		{"other connectors only", `{"remoteMcpServersConfig":[` +
			`{"uuid":"u1","name":"Gmail","tools":[{"name":"create_draft"}]}]}`,
			"needs_auth", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeCLI(installedPluginList)
			fake.Set(fakeCLIPath, []string{"mcp", "list"}, runnertest.Result{Stdout: needsAuthList})
			dataDir, _ := signedInRoot(t)
			writeDesktopSession(t, dataDir, tc.session)
			baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath), DesktopDataDir: dataDir})

			var got OverviewResponse
			getJSON(t, baseURL+"/api/overview", token, &got)
			if got.Mcp.Verdict != tc.verdict || !slices.Equal(got.Mcp.DesktopConnectors, tc.connected) {
				t.Fatalf("mcp = %+v, want verdict %q with connectors %v", got.Mcp, tc.verdict, tc.connected)
			}
		})
	}
}
