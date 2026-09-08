package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/catalog"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/claudecli"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/doctor"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner/runnertest"
)

const fakeCLIPath = "/fake/claude"

var errNotFound = errors.New("not found")

const marketplaceListFixture = `[
  {"name":"rise-x-public","source":"github","repo":"rise-x/rise-x-ai-public-marketplace","installLocation":"INSTALL_LOCATION"}
]`

// The last entry's "source" is an object, the shape a real CLI writes for a
// url-source marketplace; it must not fail the decode of the whole list.
const pluginListFixture = `{
  "installed": [],
  "available": [
    {"pluginId":"rise-x-mcp@rise-x-public","name":"rise-x-mcp","marketplaceName":"rise-x-public","source":"./plugins/rise-x-mcp"},
    {"pluginId":"rise-x-apps@rise-x-public","name":"rise-x-apps","marketplaceName":"rise-x-public","source":"./plugins/rise-x-apps"},
    {"pluginId":"agentforce-adlc@claude-plugins-official","name":"agentforce-adlc","marketplaceName":"claude-plugins-official","source":{"source":"url","url":"https://example.com/x.zip"}}
  ]
}`

// newFakeCLI answers the read-only commands /api/overview makes.
func newFakeCLI(pluginList string) *runnertest.Fake {
	fake := runnertest.NewFake()
	fake.Set(fakeCLIPath, []string{"--version"}, runnertest.Result{Stdout: "2.1.258 (Claude Code)\n"})
	fake.Set(fakeCLIPath, []string{"plugin", "marketplace", "list", "--json"}, runnertest.Result{Stdout: marketplaceListFixture})
	fake.Set(fakeCLIPath, []string{"plugin", "list", "--json", "--available"}, runnertest.Result{Stdout: pluginList})
	fake.Set(fakeCLIPath, []string{"mcp", "list"}, runnertest.Result{Stdout: "Checking MCP server health…\n"})
	return fake
}

// locateAt points Locate at a fake binary; locateNone simulates a machine with
// no claude anywhere.
func locateAt(path string) func(runner.Runner) claudecli.Env {
	return func(r runner.Runner) claudecli.Env {
		return claudecli.Env{
			LookPath: func(string) (string, error) { return path, nil },
			Runner:   r,
		}
	}
}

func locateNone(r runner.Runner) claudecli.Env {
	return claudecli.Env{
		LookPath: func(string) (string, error) { return "", errNotFound },
		Runner:   r,
	}
}

// noNodeEnv reports "not found" for Node and Git without touching the real
// machine.
func noNodeEnv(r runner.Runner) doctor.Env {
	return doctor.Env{
		LookPath: func(string) (string, error) { return "", errNotFound },
		Stat:     func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		Runner:   r,
	}
}

// fixtureCatalog keeps /api/overview and /api/doctor off the network. It also
// serves the marketplace manifest, since that is what validates a plugin name
// an action names.
func fixtureCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	return catalogNamesServer(t, "rise-x-mcp", "rise-x-apps")
}

// newServer starts an httptest server around a Server built from cfg, filling
// in the fields every test shares.
func newServer(t *testing.T, cfg Config) (baseURL, token string) {
	t.Helper()
	ts := httptest.NewUnstartedServer(nil)
	cfg.Port = ts.Listener.Addr().(*net.TCPAddr).Port
	cfg.Token = "test-token"
	if cfg.ClaudeDir == "" {
		cfg.ClaudeDir = t.TempDir()
	}
	// Both default to real locations on this machine, so every test that does
	// not supply its own fixture gets an empty tree instead.
	if cfg.DesktopDataDir == "" {
		cfg.DesktopDataDir = t.TempDir()
	}
	if cfg.ClaudeJSONPath == "" {
		cfg.ClaudeJSONPath = filepath.Join(t.TempDir(), ".claude.json")
	}
	if cfg.Catalog == nil {
		cfg.Catalog = fixtureCatalog(t)
	}
	if cfg.NodeEnv == nil {
		cfg.NodeEnv = noNodeEnv
	}
	ts.Config.Handler = New(cfg).Handler()
	ts.Start()
	t.Cleanup(ts.Close)
	return ts.URL, cfg.Token
}

// newTestServer is the default fixture: a fake claude on PATH.
func newTestServer(t *testing.T) (baseURL, token string, fake *runnertest.Fake) {
	t.Helper()
	fake = newFakeCLI(pluginListFixture)
	baseURL, token = newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath)})
	return baseURL, token, fake
}

func TestHandler_CSRF_Missing_403(t *testing.T) {
	baseURL, _, _ := newTestServer(t)
	resp, err := http.Get(baseURL + "/api/overview")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestHandler_BadHost_400(t *testing.T) {
	baseURL, token, _ := newTestServer(t)
	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/overview", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-RiseX-Token", token)
	req.Host = "evil.example.com"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandler_UnknownAction_400(t *testing.T) {
	baseURL, token, _ := newTestServer(t)
	resp := post(t, baseURL+"/api/actions/no.such.action", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandler_PluginInstall_Happy(t *testing.T) {
	baseURL, token, fake := newTestServer(t)
	fake.Set(fakeCLIPath, []string{"plugin", "install", "rise-x-apps@rise-x-public"},
		runnertest.Result{Stdout: "Installed rise-x-apps@1.5.0\n"})

	resp := post(t, baseURL+"/api/actions/plugin.install", token, map[string]any{"name": "rise-x-apps"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		JobID string `json:"jobId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.JobID == "" {
		t.Fatal("expected a jobId")
	}
	if status := waitForJob(t, baseURL, token, body.JobID); status != "succeeded" {
		t.Fatalf("job status = %q, want succeeded", status)
	}
}

func TestHandler_UnknownPluginTarget_400(t *testing.T) {
	baseURL, token, _ := newTestServer(t)
	resp := post(t, baseURL+"/api/actions/plugin.install", token, map[string]any{"name": "totally-not-a-plugin"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// blockingRunner delegates to a Fake but holds one command open, so a test can
// have a job that is genuinely still in flight.
type blockingRunner struct {
	*runnertest.Fake
	blockOn []string
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingRunner(fake *runnertest.Fake, blockOn []string) *blockingRunner {
	return &blockingRunner{
		Fake:    fake,
		blockOn: blockOn,
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

// StreamDir is the one to override: every mutating claudecli call goes through
// it, and Fake.Stream delegates to it too.
func (b *blockingRunner) StreamDir(ctx context.Context, dir, name string, args []string, onLine func(runner.Line)) (int, error) {
	if strings.Join(args, "\x00") == strings.Join(b.blockOn, "\x00") {
		b.once.Do(func() { close(b.started) })
		select {
		case <-b.release:
			return 0, nil
		case <-ctx.Done():
			return -1, ctx.Err()
		}
	}
	return b.Fake.StreamDir(ctx, dir, name, args, onLine)
}

// Only one job at a time: while one is genuinely running, the next action gets
// a 409 rather than racing it.
func TestHandler_ConcurrentJob_409(t *testing.T) {
	blocked := []string{"plugin", "marketplace", "update", "rise-x-public"}
	r := newBlockingRunner(newFakeCLI(pluginListFixture), blocked)
	t.Cleanup(func() { close(r.release) })
	baseURL, token := newServer(t, Config{Runner: r, LocateEnv: locateAt(fakeCLIPath)})

	first := post(t, baseURL+"/api/actions/marketplace.update", token, nil)
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", first.StatusCode)
	}

	select {
	case <-r.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the first job never started")
	}

	second := post(t, baseURL+"/api/actions/marketplace.update", token, nil)
	defer second.Body.Close()
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("second request status = %d, want 409", second.StatusCode)
	}
}

func TestHandler_Overview_MarketplaceRegistered(t *testing.T) {
	baseURL, token, _ := newTestServer(t)
	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)

	if got.CLI == nil || !got.CLI.Found {
		t.Fatalf("CLI = %+v, want found", got.CLI)
	}
	if got.CLI.Source != claudecli.SourcePath {
		t.Fatalf("CLI.Source = %q, want %q", got.CLI.Source, claudecli.SourcePath)
	}
	if got.Marketplace == nil || !got.Marketplace.Registered {
		t.Fatalf("Marketplace = %+v, want registered", got.Marketplace)
	}
	if len(got.Plugins) != 2 {
		t.Fatalf("Plugins = %+v, want 2", got.Plugins)
	}
}

// An entry with no autoUpdate key must leave autoUpdate out of the response
// entirely, so the UI can tell "not set" from "set to false".
func TestHandler_Overview_AutoUpdateOmittedWhenKeyAbsent(t *testing.T) {
	claudeDir := t.TempDir()
	body := `{"extraKnownMarketplaces":{"rise-x-public":{"source":{"source":"github","repo":"rise-x/rise-x-ai-public-marketplace"}}}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	baseURL, token := newServer(t, Config{
		Runner:    newFakeCLI(pluginListFixture),
		LocateEnv: locateAt(fakeCLIPath),
		ClaudeDir: claudeDir,
	})

	var raw struct {
		Marketplace map[string]any `json:"marketplace"`
	}
	getJSON(t, baseURL+"/api/overview", token, &raw)
	if _, ok := raw.Marketplace["autoUpdate"]; ok {
		t.Fatalf("autoUpdate should be omitted, got %+v", raw.Marketplace)
	}
}

func TestHandler_Index_NoTokenNeeded(t *testing.T) {
	baseURL, _, _ := newTestServer(t)
	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// The page carries the CSRF token, so it must not be framable or sniffable.
func TestHandler_Index_SecurityHeaders(t *testing.T) {
	baseURL, _, _ := newTestServer(t)
	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	want := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"Cache-Control":          "no-store",
		"Referrer-Policy":        "no-referrer",
	}
	for header, value := range want {
		if got := resp.Header.Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
	csp := resp.Header.Get("Content-Security-Policy")
	for _, directive := range []string{"default-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP %q is missing %q", csp, directive)
		}
	}
}

// cli.install is the one action that must work on a machine with no claude
// binary: it is what puts one there.
func TestHandler_CliInstall_WorksWithoutCLI(t *testing.T) {
	fake := runnertest.NewFake()
	name, args, err := installCommand(runtime.GOOS)
	if err != nil {
		t.Skipf("no installer for this OS: %v", err)
	}
	fake.Set(name, args, runnertest.Result{Stdout: "installed\n"})
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateNone})

	resp := post(t, baseURL+"/api/actions/cli.install", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// Every other job action still needs the CLI.
func TestHandler_MarketplaceAdd_RequiresCLI(t *testing.T) {
	baseURL, token := newServer(t, Config{Runner: runnertest.NewFake(), LocateEnv: locateNone})
	resp := post(t, baseURL+"/api/actions/marketplace.add", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// mcp.login's target must be one of the servers the plugin declares, so
// nothing a page sends can shape the claude argv.
func TestHandler_McpLogin_TargetValidation(t *testing.T) {
	installPath := t.TempDir()
	mcpJSON := `{"mcpServers":{"rise-x":{"type":"http","url":"https://mcp.rise-x.io/mcp"}}}`
	if err := os.WriteFile(filepath.Join(installPath, ".mcp.json"), []byte(mcpJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	pluginList := `{
  "installed": [{"id":"rise-x-mcp@rise-x-public","version":"1.3.1","scope":"user","enabled":true,"installPath":"` + installPath + `"}],
  "available": [{"pluginId":"rise-x-mcp@rise-x-public","name":"rise-x-mcp","marketplaceName":"rise-x-public","source":"./plugins/rise-x-mcp"}]
}`
	fake := newFakeCLI(pluginList)
	fake.Set(fakeCLIPath, []string{"mcp", "login", "plugin:rise-x-mcp:rise-x"}, runnertest.Result{Stdout: "ok\n"})
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath)})

	bad := post(t, baseURL+"/api/actions/mcp.login", token, map[string]any{"server": "--help --dangerously"})
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("arbitrary target status = %d, want 400", bad.StatusCode)
	}

	missing := post(t, baseURL+"/api/actions/mcp.login", token, nil)
	missing.Body.Close()
	if missing.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing target status = %d, want 400", missing.StatusCode)
	}

	ok := post(t, baseURL+"/api/actions/mcp.login", token, map[string]any{"server": "rise-x"})
	defer ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("configured target status = %d, want 200", ok.StatusCode)
	}
}

// The doctor's rows are the partner-facing copy; they must stay plain
// sentences with no engineering caveats.
func TestHandler_Doctor_PlainMessages(t *testing.T) {
	baseURL, token, _ := newTestServer(t)
	var got DoctorResponse
	getJSON(t, baseURL+"/api/doctor", token, &got)

	byID := map[string]doctor.Check{}
	for _, c := range got.Checks {
		byID[c.ID] = c
	}
	if msg := byID["cli"].Message; msg != "Found version 2.1.258." {
		t.Errorf("cli message = %q", msg)
	}
	if msg := byID["marketplace.registered"].Message; msg != "Claude knows where to find Rise-X skills." {
		t.Errorf("marketplace message = %q", msg)
	}
	if msg := byID["marketplace.autoupdate"].Message; msg != "Turned off, so new skill versions will not arrive on their own." {
		t.Errorf("autoupdate message = %q", msg)
	}
	for _, c := range got.Checks {
		if strings.Contains(c.Message, "user-scope") {
			t.Errorf("%s still carries the internal caveat: %q", c.ID, c.Message)
		}
	}
}

func post(t *testing.T, url, token string, body map[string]any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-RiseX-Token", token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func getJSON(t *testing.T, url, token string, into any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-RiseX-Token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatal(err)
	}
}

func waitForJob(t *testing.T, baseURL, token, id string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var snap struct {
			Job struct {
				Status string `json:"status"`
			} `json:"job"`
		}
		getJSON(t, baseURL+"/api/jobs/"+id, token, &snap)
		if snap.Job.Status != "running" {
			return snap.Job.Status
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job never finished")
	return ""
}
