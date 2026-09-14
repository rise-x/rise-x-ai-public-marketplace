package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/claudecli"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/doctor"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/selfupdate"
)

// releasesChecker serves releasesJSON as the repo's release list and assets
// by name under /dl/, and returns a checker pointed at it plus a hit counter.
func releasesChecker(t *testing.T, releasesJSON string, assets map[string][]byte) *selfupdate.Checker {
	t.Helper()
	c, _ := releasesCheckerCounting(t, releasesJSON, assets)
	return c
}

func releasesCheckerCounting(t *testing.T, releasesJSON string, assets map[string][]byte) (*selfupdate.Checker, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch {
		case r.URL.Path == "/repos/"+claudecli.MarketplaceRepo+"/releases":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(strings.ReplaceAll(releasesJSON, "BASE", srv.URL)))
		case strings.HasPrefix(r.URL.Path, "/dl/"):
			body, ok := assets[strings.TrimPrefix(r.URL.Path, "/dl/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := selfupdate.New(claudecli.MarketplaceRepo)
	c.APIBaseURL = srv.URL
	return c, &hits
}

// releaseJSON is one kit release with the two platform assets and checksums.
func releaseJSON(version string, prerelease bool) string {
	return fmt.Sprintf(`{"tag_name":"kit-%[1]s","html_url":"https://github.com/rel/%[1]s","draft":false,"prerelease":%[2]v,"assets":[
	  {"name":"rise-x-kit_%[1]s_darwin_universal.tar.gz","browser_download_url":"BASE/dl/rise-x-kit_%[1]s_darwin_universal.tar.gz"},
	  {"name":"rise-x-kit_%[1]s_windows_amd64.zip","browser_download_url":"BASE/dl/rise-x-kit_%[1]s_windows_amd64.zip"},
	  {"name":"checksums.txt","browser_download_url":"BASE/dl/checksums.txt"}]}`, version, prerelease)
}

func TestGather_KitUpdate_Available(t *testing.T) {
	releases := "[" + releaseJSON("v0.1.0-rc.3", true) + "," + releaseJSON("v0.1.0-rc.2", true) + "]"
	baseURL, token, _ := newTestServerVersion(t, "v0.1.0-rc.2", releasesChecker(t, releases, nil))

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	if got.Kit == nil || !got.Kit.UpdateAvailable || got.Kit.Latest != "v0.1.0-rc.3" ||
		got.Kit.LatestURL != "https://github.com/rel/v0.1.0-rc.3" {
		t.Fatalf("kit = %+v, want rc.3 available", got.Kit)
	}
	wantSelf := runtime.GOOS == "darwin" || (runtime.GOOS == "windows" && runtime.GOARCH == "amd64")
	if got.Kit.CanSelfUpdate != wantSelf {
		t.Fatalf("canSelfUpdate = %v on %s/%s", got.Kit.CanSelfUpdate, runtime.GOOS, runtime.GOARCH)
	}

	c := doctorCheck(t, baseURL, token, "kit")
	if c.Status != doctor.StatusWarn || !strings.Contains(c.Message, "v0.1.0-rc.3") {
		t.Fatalf("doctor row = %+v", c)
	}
	if wantSelf && c.Fix != "kit.update" {
		t.Fatalf("doctor row offers %q, want kit.update", c.Fix)
	}
	var got2 DoctorResponse
	getJSON(t, baseURL+"/api/doctor", token, &got2)
	if got2.Checks[0].ID != "kit" {
		t.Fatalf("first check = %q, want the kit's own row first", got2.Checks[0].ID)
	}
}

// A stable build is not offered a prerelease, and a build on the newest
// version is told so.
func TestGather_KitUpdate_StableIgnoresPrerelease(t *testing.T) {
	releases := "[" + releaseJSON("v0.2.0-rc.1", true) + "," + releaseJSON("v0.1.0", false) + "]"
	baseURL, token, _ := newTestServerVersion(t, "v0.1.0", releasesChecker(t, releases, nil))

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	if got.Kit == nil || got.Kit.UpdateAvailable || got.Kit.Latest != "v0.1.0" {
		t.Fatalf("kit = %+v, want no update", got.Kit)
	}
	c := doctorCheck(t, baseURL, token, "kit")
	if c.Status != doctor.StatusOK || c.Fix != "" {
		t.Fatalf("doctor row = %+v", c)
	}
}

// A development build has no version to compare, so it neither asks GitHub
// nor gets a row.
func TestGather_KitUpdate_DevBuildNeverChecks(t *testing.T) {
	checker, hits := releasesCheckerCounting(t, "["+releaseJSON("v9.0.0", false)+"]", nil)
	baseURL, token, _ := newTestServerVersion(t, "dev", checker)

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	if got.Kit != nil {
		t.Fatalf("kit = %+v, want none for a dev build", got.Kit)
	}
	var doc DoctorResponse
	getJSON(t, baseURL+"/api/doctor", token, &doc)
	for _, c := range doc.Checks {
		if c.ID == "kit" {
			t.Fatalf("dev build got a kit row: %+v", c)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("GitHub was asked %d times for a dev build", n)
	}
}

// GitHub refusing the check is a skipped row, never a false "up to date".
func TestGather_KitUpdate_RateLimited_Skips(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	checker := selfupdate.New(claudecli.MarketplaceRepo)
	checker.APIBaseURL = srv.URL
	baseURL, token, _ := newTestServerVersion(t, "v0.1.0", checker)

	var got OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &got)
	if got.Kit == nil || got.Kit.CheckError == "" || got.Kit.UpdateAvailable {
		t.Fatalf("kit = %+v, want a check error", got.Kit)
	}
	if c := doctorCheck(t, baseURL, token, "kit"); c.Status != doctor.StatusSkip {
		t.Fatalf("doctor row = %+v, want skip", c)
	}
}

// The whole update, end to end against a fake release: the running file is
// replaced by the asset's binary and main is asked to relaunch.
func TestHandler_KitUpdate_ReplacesBinaryAndRequestsRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the rename-aside path is not exercised on this runner")
	}
	if _, ok := selfupdate.AssetName("v0", runtime.GOOS, runtime.GOARCH); !ok {
		t.Skipf("no release asset for %s/%s, so nothing to download here", runtime.GOOS, runtime.GOARCH)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, selfupdate.BinaryName(runtime.GOOS))
	if err := os.WriteFile(exe, []byte("old build"), 0o755); err != nil {
		t.Fatal(err)
	}

	const version = "v0.1.0-rc.3"
	asset, _ := selfupdate.AssetName(version, runtime.GOOS, runtime.GOARCH)
	payload := []byte("new build")
	archive := tarGzOne(t, selfupdate.BinaryName(runtime.GOOS), payload)
	sum := sha256.Sum256(archive)
	checksums := hex.EncodeToString(sum[:]) + "  " + asset + "\n"
	checker := releasesChecker(t, "["+releaseJSON(version, true)+"]",
		map[string][]byte{asset: archive, "checksums.txt": []byte(checksums)})

	baseURL, token, srv := newTestServerVersion(t, "v0.1.0-rc.2", checker, func(cfg *Config) { cfg.ExePath = exe })

	resp := post(t, baseURL+"/api/actions/kit.update", token, map[string]any{})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if status := waitForJob(t, baseURL, token, jobID(t, resp)); status != "succeeded" {
		t.Fatalf("job status = %q", status)
	}
	got, err := os.ReadFile(exe)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("binary = %q (%v), want the new build", got, err)
	}
	select {
	case <-srv.Restart():
	case <-time.After(5 * time.Second):
		t.Fatal("restart was never requested")
	}
	// The job must not raise the "restart Claude Code" hint: nothing about
	// Claude Code changed.
	var ov OverviewResponse
	getJSON(t, baseURL+"/api/overview", token, &ov)
	if ov.ReloadHint {
		t.Fatal("kit.update set the reload hint")
	}
	// The work directory is gone, and nothing else may start now.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("install dir holds %d entries after the update, want the binary alone", len(entries))
	}
	resp = post(t, baseURL+"/api/actions/marketplace.update", token, map[string]any{})
	if resp.StatusCode != http.StatusConflict {
		resp.Body.Close()
		t.Fatalf("action while restarting = %d, want 409", resp.StatusCode)
	}
	if msg := errorMessage(t, resp); msg != restartingMessage {
		t.Fatalf("409 message = %q, want the restart one, not the busy one", msg)
	}
	// Quit must still work: it is the way out if the relaunch looks stuck.
	resp = post(t, baseURL+"/api/actions/quit", token, map[string]any{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("quit while restarting = %d, want 200", resp.StatusCode)
	}
}

// A download that fails changes nothing, whatever the failure: the running
// binary is untouched, no work directory is left, and no restart is asked for.
func TestHandler_KitUpdate_Failures_NoRestart(t *testing.T) {
	if _, ok := selfupdate.AssetName("v0", runtime.GOOS, runtime.GOARCH); !ok || runtime.GOOS == "windows" {
		t.Skipf("not exercised on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	const version = "v0.1.0-rc.3"
	asset, _ := selfupdate.AssetName(version, runtime.GOOS, runtime.GOARCH)
	archive := tarGzOne(t, selfupdate.BinaryName(runtime.GOOS), []byte("new build"))
	good := sha256.Sum256(archive)
	cases := []struct {
		name   string
		assets map[string][]byte
	}{
		{"checksum mismatch", map[string][]byte{asset: archive,
			"checksums.txt": []byte(strings.Repeat("0", 64) + "  " + asset + "\n")}},
		{"asset missing (404)", map[string][]byte{
			"checksums.txt": []byte(hex.EncodeToString(good[:]) + "  " + asset + "\n")}},
		{"archive without the binary", map[string][]byte{asset: tarGzOne(t, "README", []byte("x")),
			"checksums.txt": []byte(func() string { s := sha256.Sum256(tarGzOne(t, "README", []byte("x"))); return hex.EncodeToString(s[:]) }() + "  " + asset + "\n")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			exe := filepath.Join(dir, selfupdate.BinaryName(runtime.GOOS))
			if err := os.WriteFile(exe, []byte("old build"), 0o755); err != nil {
				t.Fatal(err)
			}
			checker := releasesChecker(t, "["+releaseJSON(version, true)+"]", tc.assets)
			baseURL, token, srv := newTestServerVersion(t, "v0.1.0-rc.2", checker, func(cfg *Config) { cfg.ExePath = exe })

			resp := post(t, baseURL+"/api/actions/kit.update", token, map[string]any{})
			if status := waitForJob(t, baseURL, token, jobID(t, resp)); status != "failed" {
				t.Fatalf("job status = %q, want failed", status)
			}
			got, _ := os.ReadFile(exe)
			if string(got) != "old build" {
				t.Fatalf("binary = %q, want untouched", got)
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 1 {
				t.Fatalf("install dir holds %d entries, want the binary alone", len(entries))
			}
			select {
			case <-srv.Restart():
				t.Fatal("a failed update asked for a restart")
			case <-time.After(2 * restartDelay):
			}
		})
	}
}

// While an update job holds the slot, a second one is the ordinary busy 409,
// distinct from the restart 409.
func TestHandler_KitUpdate_Busy_409(t *testing.T) {
	fake := newFakeCLI(pluginListFixture)
	fake.SetDelay(2 * time.Second)
	checker := releasesChecker(t, "["+releaseJSON("v0.1.0", false)+"]", nil)
	baseURL, token := newServer(t, Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath), Version: "v0.1.0", Updates: checker,
		ExePath: filepath.Join(t.TempDir(), "rise-x-kit")})

	first := post(t, baseURL+"/api/actions/marketplace.update", token, map[string]any{})
	first.Body.Close()
	resp := post(t, baseURL+"/api/actions/kit.update", token, map[string]any{})
	if resp.StatusCode != http.StatusConflict {
		resp.Body.Close()
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	if msg := errorMessage(t, resp); msg != "a job is already running" {
		t.Fatalf("409 message = %q", msg)
	}
}

// Nothing newer is a failed job, not a download.
func TestHandler_KitUpdate_NothingNewer_Fails(t *testing.T) {
	checker := releasesChecker(t, "["+releaseJSON("v0.1.0", false)+"]", nil)
	baseURL, token, _ := newTestServerVersion(t, "v0.1.0", checker, func(cfg *Config) { cfg.ExePath = filepath.Join(t.TempDir(), "rise-x-kit") })

	resp := post(t, baseURL+"/api/actions/kit.update", token, map[string]any{})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if status := waitForJob(t, baseURL, token, jobID(t, resp)); status != "failed" {
		t.Fatalf("job status = %q, want failed", status)
	}
}

// newTestServerVersion is the default fixture with a versioned build and a
// release checker of the test's choosing.
func newTestServerVersion(t *testing.T, version string, checker *selfupdate.Checker, opts ...func(*Config)) (baseURL, token string, srv *Server) {
	t.Helper()
	fake := newFakeCLI(pluginListFixture)
	cfg := Config{Runner: fake, LocateEnv: locateAt(fakeCLIPath), Version: version, Updates: checker}
	for _, opt := range opts {
		opt(&cfg)
	}
	return newServerWith(t, cfg)
}

func tarGzOne(t *testing.T, member string, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: member, Mode: 0o755, Size: int64(len(payload)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
