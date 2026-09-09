package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/catalog"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/claudecli"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/doctor"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/mcp"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner/runnertest"
)

// installedPluginList is `claude plugin list --json --available` with both
// catalog plugins installed.
const installedPluginList = `{
  "installed": [
    {"id":"rise-x-mcp@rise-x-public","version":"1.3.1","scope":"user","enabled":true,"installPath":"/tmp/mcp"},
    {"id":"rise-x-apps@rise-x-public","version":"1.5.0","scope":"user","enabled":true,"installPath":"/tmp/apps"}
  ],
  "available": [
    {"pluginId":"rise-x-mcp@rise-x-public","name":"rise-x-mcp","marketplaceName":"rise-x-public","source":"./plugins/rise-x-mcp"},
    {"pluginId":"rise-x-apps@rise-x-public","name":"rise-x-apps","marketplaceName":"rise-x-public","source":"./plugins/rise-x-apps"}
  ]
}`

// marketplaceClone builds a marketplace clone directory: a plugin.json per
// plugin, and a .git/HEAD only when withGit.
func marketplaceClone(t *testing.T, withGit bool) string {
	t.Helper()
	dir := t.TempDir()
	for name, version := range map[string]string{"rise-x-mcp": "1.3.1", "rise-x-apps": "1.5.0"} {
		manifestDir := filepath.Join(dir, "plugins", name, ".claude-plugin")
		if err := os.MkdirAll(manifestDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"),
			[]byte(`{"version":"`+version+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if withGit {
		gitDir := filepath.Join(dir, ".git")
		if err := os.MkdirAll(gitDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("deadbeef\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// serverWithClone points the fake marketplace's installLocation at a real
// directory, so LocalVersion and LocalHEAD read something.
func serverWithClone(t *testing.T, installLocation string, cat *catalog.Catalog) (baseURL, token string) {
	t.Helper()
	fake := newFakeCLI(installedPluginList)
	fake.Set(fakeCLIPath, []string{"plugin", "marketplace", "list", "--json"}, runnertest.Result{
		Stdout: withInstallLocation(t, marketplaceListFixture, installLocation)})
	return newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath), Catalog: cat})
}

// statusCatalog answers plugin.json with code and the commits API with 200, so
// only the public-version lookup fails.
func statusCatalog(t *testing.T, code int) *catalog.Catalog {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "plugin.json") {
			w.WriteHeader(code)
			return
		}
		_, _ = w.Write([]byte(`{"sha":"deadbeef"}`))
	}))
	t.Cleanup(srv.Close)
	c := catalog.New(claudecli.MarketplaceRepo)
	c.RawBaseURL, c.APIBaseURL = srv.URL, srv.URL
	return c
}

func doctorCheck(t *testing.T, baseURL, token, id string) doctor.Check {
	t.Helper()
	var got DoctorResponse
	getJSON(t, baseURL+"/api/doctor", token, &got)
	for _, c := range got.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no check %q in %+v", id, got.Checks)
	return doctor.Check{}
}

// A 404 means GitHub answered, so the row must say what went wrong rather than
// fall back to the local clone's own version and claim "up to date".
func TestGather_PublicVersion404_IsNotOffline(t *testing.T) {
	baseURL, token := serverWithClone(t, marketplaceClone(t, true), statusCatalog(t, http.StatusNotFound))

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	for _, p := range got.Plugins {
		if p.Offline {
			t.Errorf("%s reported offline on a 404: %+v", p.Name, p)
		}
		if p.PublicCheckError != "HTTP 404" {
			t.Errorf("%s publicCheckError = %q, want HTTP 404", p.Name, p.PublicCheckError)
		}
	}

	c := doctorCheck(t, baseURL, token, "plugin.rise-x-mcp")
	if c.Status != doctor.StatusWarn || c.Fix != "" {
		t.Fatalf("plugin.rise-x-mcp = %+v, want a warn with no fix", c)
	}
	if c.Message != "Could not check the public version (HTTP 404)." {
		t.Fatalf("message = %q", c.Message)
	}
}

// A settings.json that isn't valid JSON can't be read for the autoUpdate key,
// so the row must say the file is unreadable rather than claim updates are
// off, and the overview must flag it for the page's tooltip.
func TestGather_SettingsJSONInvalid(t *testing.T) {
	claudeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	baseURL, token := newServer(t, Config{
		Runner:    newFakeCLI(pluginListFixture),
		LocateEnv: locateAt(fakeCLIPath),
		ClaudeDir: claudeDir,
	})

	c := doctorCheck(t, baseURL, token, "marketplace.autoupdate")
	if c.Status != doctor.StatusWarn || c.Fix != "" {
		t.Fatalf("marketplace.autoupdate = %+v", c)
	}
	if c.Message != doctor.SettingsUnreadableMessage {
		t.Fatalf("message = %q", c.Message)
	}

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	if got.Marketplace == nil || !got.Marketplace.SettingsError {
		t.Fatalf("Marketplace = %+v, want settingsError true", got.Marketplace)
	}
}

// A clone with no .git is a local problem, not an unreachable GitHub.
func TestGather_CloneWithoutGit_ReportsLocalError(t *testing.T) {
	baseURL, token := serverWithClone(t, marketplaceClone(t, false), fixtureCatalog(t))

	c := doctorCheck(t, baseURL, token, "marketplace.head")
	if c.Status != doctor.StatusWarn || c.Fix != "marketplace.update" {
		t.Fatalf("marketplace.head = %+v", c)
	}
	if c.Message != "The local copy of the skill catalog is unreadable." {
		t.Fatalf("message = %q", c.Message)
	}

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	if got.Marketplace.HeadStale != nil {
		t.Fatalf("headStale = %v, want omitted", *got.Marketplace.HeadStale)
	}
}

// available[].description is empty for the Rise-X plugins because
// marketplace.json declares none; the row must fall back to the local
// clone's own plugin.json.
func TestGather_DescriptionFallsBackToLocalClone(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"rise-x-mcp":  `{"version":"1.3.1"}`,
		"rise-x-apps": `{"version":"1.5.0","description":"Rise-X apps skill"}`,
	} {
		manifestDir := filepath.Join(dir, "plugins", name, ".claude-plugin")
		if err := os.MkdirAll(manifestDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	baseURL, token := serverWithClone(t, dir, fixtureCatalog(t))

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)

	var found bool
	for _, p := range got.Plugins {
		if p.Name != "rise-x-apps" {
			continue
		}
		found = true
		if p.Description != "Rise-X apps skill" {
			t.Errorf("rise-x-apps description = %q, want fallback from local clone", p.Description)
		}
	}
	if !found {
		t.Fatal("rise-x-apps not in overview.plugins")
	}
}

// /api/overview and /api/doctor share one gather, so a page load spawns each
// claude command once and fetches each catalog file once.
func TestHandler_ConcurrentOverviewAndDoctor_GatherOnce(t *testing.T) {
	var versionFetches, headFetches int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/commits/main") {
			atomic.AddInt64(&headFetches, 1)
		} else {
			atomic.AddInt64(&versionFetches, 1)
		}
		time.Sleep(50 * time.Millisecond) // a real GitHub round trip
		_, _ = w.Write([]byte(`{"version":"1.5.0","sha":"deadbeef"}`))
	}))
	t.Cleanup(srv.Close)
	cat := catalog.New(claudecli.MarketplaceRepo)
	cat.RawBaseURL, cat.APIBaseURL = srv.URL, srv.URL

	fake := newFakeCLI(installedPluginList)
	fake.Set(fakeCLIPath, []string{"plugin", "marketplace", "list", "--json"}, runnertest.Result{
		Stdout: withInstallLocation(t, marketplaceListFixture, marketplaceClone(t, true))})
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath), Catalog: cat})

	var wg sync.WaitGroup
	for _, path := range []string{"/api/overview", "/api/doctor"} {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodGet, baseURL+p, nil)
			if err != nil {
				t.Error(err)
				return
			}
			req.Header.Set("X-RiseX-Token", token)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("GET %s = %d", p, resp.StatusCode)
			}
		}(path)
	}
	wg.Wait()

	if versionFetches != 2 {
		t.Errorf("plugin.json fetches = %d, want 2 (one per plugin)", versionFetches)
	}
	if headFetches != 1 {
		t.Errorf("HEAD fetches = %d, want 1", headFetches)
	}

	spawns := map[string]int{}
	for _, c := range fake.Calls {
		spawns[strings.Join(c.Args, " ")]++
	}
	for _, args := range []string{
		"--version",
		"plugin marketplace list --json",
		"plugin list --json --available",
		"mcp list",
	} {
		if spawns[args] != 1 {
			t.Errorf("claude %s ran %d times, want 1 (all spawns: %v)", args, spawns[args], spawns)
		}
	}
}

// The doctor's fact and the page's row are the same struct now; the wire shape
// must not have moved. Key order is not part of the contract, the names are.
func TestPluginInfo_JSONShape(t *testing.T) {
	cases := []struct {
		name string
		in   PluginInfo
		want map[string]any
	}{
		{
			name: "installed",
			in: PluginInfo{
				PluginFact: doctor.PluginFact{Name: "rise-x-mcp", Installed: true, Enabled: true,
					LocalVersion: "1.3.1", PublicVersion: "1.5.0", UpdateAvailable: true},
				Description: "Rise-X MCP",
			},
			want: map[string]any{
				"name": "rise-x-mcp", "description": "Rise-X MCP", "installed": true, "enabled": true,
				"localVersion": "1.3.1", "publicVersion": "1.5.0", "updateAvailable": true,
			},
		},
		{
			name: "not installed",
			in:   PluginInfo{PluginFact: doctor.PluginFact{Name: "rise-x-apps"}},
			want: map[string]any{"name": "rise-x-apps", "installed": false, "enabled": false},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("plugin JSON = %s\nwant %v", raw, tc.want)
			}
		})
	}
}

// Without a CLI the machine facts are still worth reporting: they are what a
// partner fixes before installing Claude Code.
func TestDoctor_NoCLI_StillReportsMachineFacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	claudeDir := t.TempDir()
	settingsBody := `{"env":{"DISABLE_AUTOUPDATER":"1"}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settingsBody), 0o600); err != nil {
		t.Fatal(err)
	}
	npmrcBody := "@rise-x:registry=https://packages.example.com/_packaging/npm/registry/\n" +
		"//packages.example.com/_packaging/npm/registry/:_authToken=secret\n"
	if err := os.WriteFile(filepath.Join(home, ".npmrc"), []byte(npmrcBody), 0o600); err != nil {
		t.Fatal(err)
	}

	fake := runnertest.NewFake()
	fake.Set("/fake/node", []string{"--version"}, runnertest.Result{Stdout: "v20.11.0\n"})
	baseURL, token := newServer(t, Config{
		Runner:    fake,
		ClaudeDir: claudeDir,
		LocateEnv: locateNone,
		NodeEnv: func(r runner.Runner) doctor.Env {
			return doctor.Env{
				LookPath: func(string) (string, error) { return "/fake/node", nil },
				Runner:   r,
			}
		},
	})

	if c := doctorCheck(t, baseURL, token, "npmrc"); c.Status != doctor.StatusWarn || c.Fix != "npmrc.clean" {
		t.Errorf("npmrc = %+v, want a warn with the npmrc.clean fix", c)
	}
	if c := doctorCheck(t, baseURL, token, "env.autoupdater"); c.Status != doctor.StatusWarn {
		t.Errorf("env.autoupdater = %+v, want warn", c)
	}
	node := doctorCheck(t, baseURL, token, "node")
	if node.Status != doctor.StatusOK || !strings.Contains(node.Message, "20.11.0") {
		t.Errorf("node = %+v, want ok on 20.11.0", node)
	}
	if c := doctorCheck(t, baseURL, token, "cli"); c.Status != doctor.StatusFail || c.Fix != "cli.install" {
		t.Errorf("cli = %+v, want fail with the cli.install fix", c)
	}
	// The rows that need the CLI must skip, not offer a fix that would 400.
	for _, id := range []string{"marketplace.registered", "marketplace.autoupdate", "marketplace.head"} {
		c := doctorCheck(t, baseURL, token, id)
		if c.Status != doctor.StatusSkip || c.Fix != "" || c.Message != "Install Claude Code first." {
			t.Errorf("%s = %+v, want a skip with no fix", id, c)
		}
	}
}

// slowFake delays `claude plugin list`, the expensive part of a gather, so a
// test can prove an action handler never waits on one.
type slowFake struct {
	*runnertest.Fake
	delay time.Duration
}

func (f slowFake) Run(ctx context.Context, name string, args []string) (string, string, int, error) {
	if len(args) > 1 && args[0] == "plugin" && args[1] == "list" {
		time.Sleep(f.delay)
	}
	return f.Fake.Run(ctx, name, args)
}

// Validating the plugin name must come from marketplace.json, not from a
// gather: an action answers a click.
func TestHandler_PluginInstall_DoesNotWaitOnGather(t *testing.T) {
	clone := t.TempDir()
	manifestDir := filepath.Join(clone, ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "marketplace.json"),
		[]byte(`{"name":"rise-x-public","plugins":[{"name":"rise-x-apps","source":"./plugins/rise-x-apps"}]}`),
		0o644); err != nil {
		t.Fatal(err)
	}

	fake := newFakeCLI(pluginListFixture)
	fake.Set(fakeCLIPath, []string{"plugin", "marketplace", "list", "--json"},
		runnertest.Result{Stdout: withInstallLocation(t, marketplaceListFixture, clone)})
	fake.Set(fakeCLIPath, []string{"plugin", "install", "rise-x-apps@rise-x-public"},
		runnertest.Result{Stdout: "installed\n"})
	baseURL, token := newServer(t, Config{
		Runner:    slowFake{Fake: fake, delay: 5 * time.Second},
		LocateEnv: locateAt(fakeCLIPath),
	})

	start := time.Now()
	resp := post(t, baseURL+"/api/actions/plugin.install", token, map[string]any{"name": "rise-x-apps"})
	elapsed := time.Since(start)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if elapsed > time.Second {
		t.Fatalf("plugin.install answered after %s: it waited on a gather", elapsed)
	}

	var body struct {
		JobID string `json:"jobId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if status := waitForJob(t, baseURL, token, body.JobID); status != "succeeded" {
		t.Fatalf("job status = %q, want succeeded", status)
	}
	for _, c := range fake.Calls {
		if len(c.Args) > 1 && c.Args[0] == "plugin" && c.Args[1] == "list" {
			t.Fatalf("the action spawned %v", c.Args)
		}
	}
}

// updateAvailable is the server's call, so the page never compares versions.
func TestGather_UpdateAvailable(t *testing.T) {
	baseURL, token := serverWithClone(t, marketplaceClone(t, true), fixtureCatalog(t))

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	want := map[string]bool{"rise-x-mcp": true, "rise-x-apps": false} // 1.3.1 vs 1.5.0, 1.5.0 vs 1.5.0
	for _, p := range got.Plugins {
		if p.UpdateAvailable != want[p.Name] {
			t.Errorf("%s updateAvailable = %v, want %v (local %s, public %s)",
				p.Name, p.UpdateAvailable, want[p.Name], p.LocalVersion, p.PublicVersion)
		}
	}
}

// An action invalidates the gather, but node and ~/.npmrc are cached on their
// own clock; only "Rescan" re-probes the machine.
func TestGather_MachineProbesCachedUntilRescan(t *testing.T) {
	fake := newFakeCLI(pluginListFixture)
	fake.Set("/fake/node", []string{"--version"}, runnertest.Result{Stdout: "v20.11.0\n"})
	baseURL, token := newServer(t, Config{
		Runner:    fake,
		LocateEnv: locateAt(fakeCLIPath),
		NodeEnv: func(r runner.Runner) doctor.Env {
			return doctor.Env{
				LookPath: func(string) (string, error) { return "/fake/node", nil },
				Runner:   r,
			}
		},
	})

	nodeProbes := func() int {
		n := 0
		for _, c := range fake.Calls {
			if c.Name == "/fake/node" {
				n++
			}
		}
		return n
	}

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	post(t, baseURL+"/api/actions/autoupdate.set", token, map[string]any{"enabled": true}).Body.Close()
	getJSON(t, baseURL+"/api/overview", token, &got)
	if n := nodeProbes(); n != 1 {
		t.Fatalf("node probed %d times, want 1", n)
	}

	post(t, baseURL+"/api/actions/cli.rescan", token, nil).Body.Close()
	getJSON(t, baseURL+"/api/overview", token, &got)
	if n := nodeProbes(); n != 2 {
		t.Fatalf("node probed %d times after a rescan, want 2", n)
	}
}

// Run again and Refresh ask with ?fresh=1, which must re-probe the machine:
// a cached answer is why the button looked like it did nothing.
func TestGather_FreshBypassesTheCaches(t *testing.T) {
	fake := newFakeCLI(pluginListFixture)
	fake.Set("/fake/node", []string{"--version"}, runnertest.Result{Stdout: "v20.11.0\n"})
	baseURL, token := newServer(t, Config{
		Runner:    fake,
		LocateEnv: locateAt(fakeCLIPath),
		NodeEnv: func(r runner.Runner) doctor.Env {
			return doctor.Env{
				LookPath: func(string) (string, error) { return "/fake/node", nil },
				Runner:   r,
			}
		},
	})

	spawns := func() (pluginList, node int) {
		for _, c := range fake.Calls {
			switch {
			case c.Name == "/fake/node":
				node++
			case len(c.Args) > 1 && c.Args[0] == "plugin" && c.Args[1] == "list":
				pluginList++
			}
		}
		return pluginList, node
	}

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	getJSON(t, baseURL+"/api/doctor", token, &DoctorResponse{})
	if pl, node := spawns(); pl != 1 || node != 1 {
		t.Fatalf("a plain GET within the gather TTL re-probed: plugin list %d, node %d", pl, node)
	}

	getJSON(t, baseURL+"/api/doctor?fresh=1", token, &DoctorResponse{})
	if pl, node := spawns(); pl != 2 || node != 2 {
		t.Fatalf("fresh=1 did not re-probe: plugin list %d, node %d", pl, node)
	}

	getJSON(t, baseURL+"/api/overview?fresh=1", token, &got)
	if pl, node := spawns(); pl != 3 || node != 3 {
		t.Fatalf("fresh=1 on /api/overview did not re-probe: plugin list %d, node %d", pl, node)
	}
}

// Claude Code's desktop shell exports DISABLE_AUTOUPDATER to what it starts,
// so reading the kit's own environment made this row depend on how the kit was
// launched. Only settings.json counts.
func TestGather_AutoupdaterIgnoresProcessEnv(t *testing.T) {
	t.Setenv("DISABLE_AUTOUPDATER", "1")
	baseURL, token := newServer(t, Config{
		Runner:    newFakeCLI(pluginListFixture),
		LocateEnv: locateAt(fakeCLIPath),
	})
	if c := doctorCheck(t, baseURL, token, "env.autoupdater"); c.Status != doctor.StatusOK {
		t.Errorf("env.autoupdater = %+v, want ok", c)
	}
}

// A probe that could not answer is remembered for 10 seconds, not 60: the
// partner fixes the machine and presses Refresh.
func TestProbeCache_FailureExpiresSooner(t *testing.T) {
	if probeFailTTL != 10*time.Second {
		t.Fatalf("probeFailTTL = %v, want 10s", probeFailTTL)
	}
	var cache probeCache[int]
	calls := 0
	probe := func(failed bool) func() (int, bool) {
		return func() (int, bool) {
			calls++
			return calls, failed
		}
	}

	cache.getOrFail(probe(true))
	if cache.getOrFail(probe(true)); calls != 1 {
		t.Fatalf("a fresh failure was re-probed: %d calls", calls)
	}
	cache.at = time.Now().Add(-probeFailTTL - time.Millisecond)
	if cache.getOrFail(probe(false)); calls != 2 {
		t.Fatalf("a stale failure was not re-probed: %d calls", calls)
	}
	cache.at = time.Now().Add(-probeFailTTL - time.Millisecond)
	if cache.getOrFail(probe(false)); calls != 2 {
		t.Fatalf("a success was re-probed after the failure TTL: %d calls", calls)
	}
	cache.at = time.Now().Add(-probeTTL - time.Millisecond)
	if cache.getOrFail(probe(false)); calls != 3 {
		t.Fatalf("a stale success was not re-probed: %d calls", calls)
	}
}

// A `claude mcp list` that could not run - timed out, killed - leaves the
// connection state unknown with a message to show. It must never read as
// "not installed": nothing was learned about the servers at all.
func TestGather_McpListFailed_IsUnknownNotNotInstalled(t *testing.T) {
	fake := newFakeCLI(pluginListFixture)
	fake.Set(fakeCLIPath, []string{"mcp", "list"}, runnertest.Result{Err: errors.New("signal: killed")})
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath)})

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	if got.Mcp == nil {
		t.Fatal("no mcp block")
	}
	if got.Mcp.Verdict != string(mcp.VerdictUnknown) {
		t.Fatalf("verdict = %q, want unknown", got.Mcp.Verdict)
	}
	if got.Mcp.Message != McpCheckFailedMessage {
		t.Fatalf("message = %q, want %q", got.Mcp.Message, McpCheckFailedMessage)
	}
	if got.Mcp.Raw != "" {
		t.Fatalf("raw = %q, want nothing from a failed check", got.Mcp.Raw)
	}
}

// `claude mcp list` prints whatever a server's own config carries, so its
// output reaches the page redacted.
func TestGather_McpRawIsRedacted(t *testing.T) {
	fake := newFakeCLI(pluginListFixture)
	fake.Set(fakeCLIPath, []string{"mcp", "list"}, runnertest.Result{
		Stdout: "plugin:rise-x-mcp:rise-x: https://mcp.rise-x.io/mcp (HTTP) - ✔ Connected\nheader: Authorization: Bearer sk-ant-secret123\n"})
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath)})

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	if strings.Contains(got.Mcp.Raw, "sk-ant-secret123") {
		t.Fatalf("raw carries a credential: %q", got.Mcp.Raw)
	}
	if !strings.Contains(got.Mcp.Raw, "[redacted]") {
		t.Fatalf("raw = %q, want the credential redacted", got.Mcp.Raw)
	}
}

// npmrcServer points the kit at a temp ~/.npmrc holding a real-looking token.
func npmrcServer(t *testing.T) (baseURL, token, npmrcPath string) {
	t.Helper()
	npmrcPath = filepath.Join(t.TempDir(), ".npmrc")
	body := "@rise-x:registry=https://packages.example.com/_packaging/npm/registry/\n" +
		"//packages.example.com/_packaging/npm/registry/:_authToken=supersecrettoken\n" +
		"registry=https://registry.npmjs.org/\n"
	if err := os.WriteFile(npmrcPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	baseURL, token = newServer(t, Config{Runner: newFakeCLI(pluginListFixture),
		LocateEnv: locateAt(fakeCLIPath), NpmrcPath: npmrcPath})
	return baseURL, token, npmrcPath
}

// The doctor names the registry line it would rewrite, never a credential:
// the detail is shown on the page and copied into bug reports, and the private-feed
// auth line stays a legitimate credential either way.
func TestDoctor_NpmrcDetailIsMasked(t *testing.T) {
	baseURL, token, _ := npmrcServer(t)

	c := doctorCheck(t, baseURL, token, "npmrc")
	if c.Status != doctor.StatusWarn || c.Fix != "npmrc.clean" {
		t.Fatalf("npmrc row = %+v", c)
	}
	if strings.Contains(c.Detail, "supersecrettoken") {
		t.Fatalf("detail carries the token: %q", c.Detail)
	}
	if c.Detail != "@rise-x:registry=…" {
		t.Fatalf("detail = %q, want the masked registry line", c.Detail)
	}
}

// npmrc.clean end to end: the leftover @rise-x:registry line is rewritten to
// point at the public npm registry, the private-feed auth line is untouched, and a
// backup is left next to the file.
func TestHandler_NpmrcClean_RewritesTheRegistryLine(t *testing.T) {
	baseURL, token, path := npmrcServer(t)

	resp := post(t, baseURL+"/api/actions/npmrc.clean", token, map[string]any{"confirm": true})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Rewritten []string `json:"updated"`
		Backup    string   `json:"backup"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Rewritten) != 1 {
		t.Fatalf("rewritten = %v, want the one @rise-x:registry line", body.Rewritten)
	}
	for _, line := range body.Rewritten {
		if strings.Contains(line, "supersecrettoken") {
			t.Fatalf("response carries the token: %q", line)
		}
	}
	kept, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(kept), "@rise-x:registry=https://registry.npmjs.org/") {
		t.Fatalf("~/.npmrc still points @rise-x away from public npm: %s", kept)
	}
	if !strings.Contains(string(kept), "supersecrettoken") {
		t.Fatalf("~/.npmrc lost its private-feed auth line: %s", kept)
	}
	if !strings.Contains(string(kept), "registry=https://registry.npmjs.org/") {
		t.Fatalf("~/.npmrc lost its npmjs line: %s", kept)
	}
	if _, err := os.Stat(body.Backup); err != nil {
		t.Fatalf("backup %q: %v", body.Backup, err)
	}
	if c := doctorCheck(t, baseURL, token, "npmrc"); c.Status != doctor.StatusOK {
		t.Fatalf("npmrc row after the clean = %+v, want ok", c)
	}
}

// An unreadable settings.json must reach the page even with no CLI to gather
// the rest: it is what disables the auto-update switch.
func TestGather_NoCLI_ReportsSettingsError(t *testing.T) {
	claudeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	baseURL, token := newServer(t, Config{Runner: runnertest.NewFake(), LocateEnv: locateNone,
		ClaudeDir: claudeDir})

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	if got.CLI == nil || got.CLI.Found {
		t.Fatalf("cli = %+v, want not found", got.CLI)
	}
	if got.Marketplace == nil || !got.Marketplace.SettingsError {
		t.Fatalf("marketplace = %+v, want settingsError", got.Marketplace)
	}
	if got.Marketplace.Registered {
		t.Fatalf("marketplace = %+v, want registered false with no CLI", got.Marketplace)
	}
}

// notCheckedPrefix is what every row a claude command could not answer opens
// with; the doctor package owns the rest of the sentence.
const notCheckedPrefix = "Could not check right now:"

// An older CLI rejects --available with exit 1, and a wedged one hits the read
// timeout. Neither says the skills are missing, so no row may read "Not
// installed" or offer to install one on top of a copy that might be there.
func TestGather_PluginListUnreadable_SkillRowsAreNotChecked(t *testing.T) {
	for _, tc := range []struct {
		name string
		res  runnertest.Result
	}{
		{"exit 1", runnertest.Result{ExitCode: 1, Stderr: "error: unknown option '--available'"}},
		{"deadline", runnertest.Result{Err: context.DeadlineExceeded}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeCLI(installedPluginList)
			fake.Set(fakeCLIPath, []string{"plugin", "list", "--json", "--available"}, tc.res)
			baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath)})

			var ov OverviewResponse
			getJSON(t, baseURL+"/api/overview", token, &ov)
			if len(ov.Plugins) == 0 {
				t.Fatal("no skill rows at all")
			}
			for _, p := range ov.Plugins {
				if p.CheckError == "" {
					t.Errorf("%s: checkError is empty, so the page cannot tell unknown from missing", p.Name)
				}
				if p.Installed {
					t.Errorf("%s: reported installed off a list that could not be read", p.Name)
				}
			}

			var got DoctorResponse
			getJSON(t, baseURL+"/api/doctor", token, &got)
			for _, c := range got.Checks {
				// The banner reads its tone off the fail count, so an
				// unreadable list must leave no failure behind.
				if c.Status == doctor.StatusFail {
					t.Errorf("%s failed on an unreadable list: %s", c.ID, c.Message)
				}
				if !strings.HasPrefix(c.ID, "plugin.") {
					continue
				}
				if c.Status != doctor.StatusSkip {
					t.Errorf("%s status = %q, want skip: %s", c.ID, c.Status, c.Message)
				}
				if c.Fix != "" {
					t.Errorf("%s offers fix %q for something it could not check", c.ID, c.Fix)
				}
				if !strings.HasPrefix(c.Message, notCheckedPrefix) {
					t.Errorf("%s message = %q, want it to open with %q", c.ID, c.Message, notCheckedPrefix)
				}
			}
		})
	}
}

// The same for `claude plugin marketplace list`: a read that failed is not a
// marketplace that is missing, and "Set it up" would register a second copy.
func TestGather_MarketplaceListUnreadable_MarketplaceRowsAreNotChecked(t *testing.T) {
	fake := newFakeCLI(installedPluginList)
	fake.Set(fakeCLIPath, []string{"plugin", "marketplace", "list", "--json"},
		runnertest.Result{Err: context.DeadlineExceeded})
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath)})

	var ov OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &ov)
	if ov.Marketplace == nil || ov.Marketplace.CheckError == "" {
		t.Fatalf("marketplace.checkError is empty: %+v", ov.Marketplace)
	}

	for _, id := range []string{"marketplace.registered", "marketplace.autoupdate", "marketplace.head"} {
		c := doctorCheck(t, baseURL, token, id)
		if c.Status != doctor.StatusSkip {
			t.Errorf("%s status = %q, want skip: %s", c.ID, c.Status, c.Message)
		}
		if c.Fix != "" {
			t.Errorf("%s offers fix %q for something it could not check", c.ID, c.Fix)
		}
		if !strings.HasPrefix(c.Message, notCheckedPrefix) {
			t.Errorf("%s message = %q, want it to open with %q", c.ID, c.Message, notCheckedPrefix)
		}
	}
}

// An organisation-synced copy is proof on its own: the CLI's list never
// mentions it, so its row stays ok even when that list could not be read.
func TestGather_SyncedPlugin_StaysOKWhenPluginListFails(t *testing.T) {
	fake := newFakeCLI(`{"installed": [], "available": []}`)
	fake.Set(fakeCLIPath, []string{"plugin", "list", "--json", "--available"},
		runnertest.Result{ExitCode: 1, Stderr: "error: unknown option '--available'"})
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath),
		Catalog: versionCatalog(t, "1.3.1"), DesktopDataDir: syncedDesktopDir(t, "1.3.1")})

	p := pluginInfo(t, baseURL, token, riseXMcpPlugin)
	if p.CheckError != "" {
		t.Errorf("checkError = %q, want empty: the synced manifest answered instead", p.CheckError)
	}
	if !p.Installed || p.InstallSource != doctor.SourceOrganisation {
		t.Fatalf("plugin = %+v, want installed by the organisation", p)
	}
	if c := doctorCheck(t, baseURL, token, "plugin."+riseXMcpPlugin); c.Status != doctor.StatusOK {
		t.Errorf("status = %q, want ok: %s", c.Status, c.Message)
	}
}

// gateRunner delegates to a Fake but holds `claude mcp list` open until the
// test releases it, so a request can be cancelled at a known point mid-gather.
type gateRunner struct {
	*runnertest.Fake
	started chan struct{}
	release chan struct{}
	once    sync.Once
	// killed records that a caller's context ended the wait, which is what a
	// real runner would have killed the child process over.
	killed atomic.Bool
}

func newGateRunner(fake *runnertest.Fake) *gateRunner {
	return &gateRunner{Fake: fake, started: make(chan struct{}), release: make(chan struct{})}
}

func (g *gateRunner) Run(ctx context.Context, name string, args []string) (string, string, int, error) {
	if strings.Join(args, " ") == "mcp list" {
		g.once.Do(func() { close(g.started) })
		select {
		case <-g.release:
		case <-ctx.Done():
			g.killed.Store(true)
			return "", "", -1, ctx.Err()
		}
	}
	return g.Fake.Run(ctx, name, args)
}

// A browser reload mid-probe must not kill the claude child nor leave its
// death cached: the next load has to see the real answer.
func TestGather_RequestCancelledMidGather_DoesNotCacheAFailure(t *testing.T) {
	r := newGateRunner(newFakeCLI(installedPluginList))
	baseURL, token := newServer(t, Config{Runner: r, LocateEnv: locateAt(fakeCLIPath)})

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/overview", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-RiseX-Token", token)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	}()

	select {
	case <-r.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the gather never reached `claude mcp list`")
	}
	cancel()
	<-done
	close(r.release) // the detached gather is still waiting on it
	if r.killed.Load() {
		t.Error("the client disconnect ended `claude mcp list`")
	}

	var ov OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &ov)
	if ov.Mcp == nil {
		t.Fatal("no mcp block")
	}
	if ov.Mcp.Message != "" {
		t.Errorf("mcp.message = %q, want empty: the cancelled probe was cached", ov.Mcp.Message)
	}
	if ov.Marketplace == nil || !ov.Marketplace.Registered {
		t.Errorf("marketplace = %+v, want registered: partial facts were cached", ov.Marketplace)
	}
}

// The failure of a probe that was cut short is not the machine's answer, so it
// must not stand in for one until probeFailTTL runs out.
func TestProbeCache_CancelledAnswerIsNotRemembered(t *testing.T) {
	var c probeCache[int]
	calls := 0
	cancelled := func() (int, error) {
		calls++
		return calls, context.Canceled
	}

	if got := c.getOrForget(cancelled); got != 1 {
		t.Fatalf("first answer = %d, want 1", got)
	}
	if got := c.getOrForget(cancelled); got != 2 {
		t.Errorf("second answer = %d, want 2: the cancelled one was remembered", got)
	}

	if got := c.getOrForget(func() (int, error) { return 7, nil }); got != 7 {
		t.Fatalf("real answer = %d, want 7", got)
	}
	if got := c.getOrForget(func() (int, error) { t.Fatal("re-probed a good answer"); return 0, nil }); got != 7 {
		t.Errorf("cached answer = %d, want 7", got)
	}
}
