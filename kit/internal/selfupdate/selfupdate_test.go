package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newTestChecker serves body for every releases request and counts the hits.
func newTestChecker(t *testing.T, body string, status int) (*Checker, *int) {
	t.Helper()
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	c := New("rise-x/rise-x-ai-public-marketplace")
	c.APIBaseURL = srv.URL
	return c, &hits
}

func release(tag string, pre, draft bool) string {
	return `{"tag_name":"` + tag + `","html_url":"https://example.test/` + tag +
		`","prerelease":` + boolLit(pre) + `,"draft":` + boolLit(draft) +
		`,"assets":[{"name":"checksums.txt","browser_download_url":"https://example.test/checksums.txt"}]}`
}

func boolLit(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func list(entries ...string) string {
	out := "["
	for i, e := range entries {
		if i > 0 {
			out += ","
		}
		out += e
	}
	return out + "]"
}

func TestLatest_PicksNewestByVersionNotListOrder(t *testing.T) {
	c, _ := newTestChecker(t, list(
		release("kit-v0.1.0", false, false),
		release("kit-v0.3.0", false, false),
		release("kit-v0.2.0", false, false),
	), http.StatusOK)

	rel, newer, err := c.Latest(context.Background(), "v0.1.0")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.Version != "v0.3.0" || !newer {
		t.Fatalf("got %q newer=%v, want v0.3.0 newer=true", rel.Version, newer)
	}
	if rel.Tag != "kit-v0.3.0" || rel.URL != "https://example.test/kit-v0.3.0" {
		t.Fatalf("tag/url = %q / %q", rel.Tag, rel.URL)
	}
	if rel.Assets["checksums.txt"] == "" {
		t.Fatalf("assets not carried: %v", rel.Assets)
	}
}

func TestLatest_IgnoresForeignTagsAndDrafts(t *testing.T) {
	c, _ := newTestChecker(t, list(
		release("rise-x-apps-v9.0.0", false, false),
		release("v8.0.0", false, false),
		release("kit-v0.9.0", false, true),
		release("kit-v0.2.0", false, false),
	), http.StatusOK)

	rel, newer, err := c.Latest(context.Background(), "v0.1.0")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.Version != "v0.2.0" || !newer {
		t.Fatalf("got %q newer=%v, want v0.2.0 newer=true", rel.Version, newer)
	}
}

func TestLatest_StableCurrentIgnoresPrereleases(t *testing.T) {
	c, _ := newTestChecker(t, list(
		release("kit-v0.1.0", false, false),
		release("kit-v0.2.0-rc.1", true, false),
	), http.StatusOK)

	rel, newer, err := c.Latest(context.Background(), "v0.1.0")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.Version != "v0.1.0" || newer {
		t.Fatalf("got %q newer=%v, want v0.1.0 newer=false", rel.Version, newer)
	}
}

func TestLatest_PrereleaseCurrentSeesPrereleases(t *testing.T) {
	c, _ := newTestChecker(t, list(
		release("kit-v0.1.0-rc.1", true, false),
		release("kit-v0.1.0-rc.2", true, false),
	), http.StatusOK)

	rel, newer, err := c.Latest(context.Background(), "v0.1.0-rc.1")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.Version != "v0.1.0-rc.2" || !newer {
		t.Fatalf("got %q newer=%v, want v0.1.0-rc.2 newer=true", rel.Version, newer)
	}
}

func TestLatest_CurrentIsNewest(t *testing.T) {
	c, _ := newTestChecker(t, list(release("kit-v0.2.0", false, false)), http.StatusOK)

	rel, newer, err := c.Latest(context.Background(), "v0.2.0")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if newer {
		t.Fatalf("newer = true for the current version %q", rel.Version)
	}
}

func TestLatest_DevMakesNoRequest(t *testing.T) {
	c, hits := newTestChecker(t, list(release("kit-v9.9.9", false, false)), http.StatusOK)

	for _, current := range []string{"dev", "", "unknown"} {
		rel, newer, err := c.Latest(context.Background(), current)
		if err != nil || newer || rel.Version != "" {
			t.Fatalf("Latest(%q) = %+v, %v, %v", current, rel, newer, err)
		}
	}
	if *hits != 0 {
		t.Fatalf("server saw %d requests, want 0", *hits)
	}
}

func TestLatest_RateLimited(t *testing.T) {
	c, _ := newTestChecker(t, "", http.StatusForbidden)

	if _, _, err := c.Latest(context.Background(), "v0.1.0"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}

func TestLatest_StatusError(t *testing.T) {
	c, _ := newTestChecker(t, "", http.StatusInternalServerError)

	_, _, err := c.Latest(context.Background(), "v0.1.0")
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusInternalServerError {
		t.Fatalf("err = %v, want *StatusError 500", err)
	}
}

func TestLatest_CachedWithinTTL(t *testing.T) {
	c, hits := newTestChecker(t, list(release("kit-v0.2.0", false, false)), http.StatusOK)

	for i := 0; i < 3; i++ {
		if _, _, err := c.Latest(context.Background(), "v0.1.0"); err != nil {
			t.Fatalf("Latest: %v", err)
		}
	}
	if *hits != 1 {
		t.Fatalf("server saw %d requests, want 1", *hits)
	}
}

func TestLatest_ForgetFailuresRetries(t *testing.T) {
	c, hits := newTestChecker(t, "", http.StatusForbidden)

	if _, _, err := c.Latest(context.Background(), "v0.1.0"); err == nil {
		t.Fatal("expected a failure")
	}
	if _, _, err := c.Latest(context.Background(), "v0.1.0"); err == nil {
		t.Fatal("expected the cached failure")
	}
	if *hits != 1 {
		t.Fatalf("server saw %d requests before ForgetFailures, want 1", *hits)
	}

	c.ForgetFailures()
	if _, _, err := c.Latest(context.Background(), "v0.1.0"); err == nil {
		t.Fatal("expected a failure")
	}
	if *hits != 2 {
		t.Fatalf("server saw %d requests after ForgetFailures, want 2", *hits)
	}
}

func TestAssetName(t *testing.T) {
	tests := []struct {
		goos, goarch string
		want         string
		ok           bool
	}{
		{"darwin", "arm64", "rise-x-kit_v0.1.0_darwin_universal.tar.gz", true},
		{"darwin", "amd64", "rise-x-kit_v0.1.0_darwin_universal.tar.gz", true},
		{"windows", "amd64", "rise-x-kit_v0.1.0_windows_amd64.zip", true},
		{"linux", "amd64", "", false},
		{"windows", "arm64", "", false},
	}
	for _, tt := range tests {
		got, ok := AssetName("v0.1.0", tt.goos, tt.goarch)
		if got != tt.want || ok != tt.ok {
			t.Errorf("AssetName(v0.1.0, %s, %s) = %q, %v; want %q, %v", tt.goos, tt.goarch, got, ok, tt.want, tt.ok)
		}
	}
}

func TestBinaryName(t *testing.T) {
	if got := BinaryName("windows"); got != "rise-x-kit.exe" {
		t.Errorf("BinaryName(windows) = %q", got)
	}
	if got := BinaryName("darwin"); got != "rise-x-kit" {
		t.Errorf("BinaryName(darwin) = %q", got)
	}
}

// A failure caused by the caller's own deadline is not remembered: the
// five-second overview check must not decide for the five-minute update.
func TestLatest_DeadlineNotCached(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(list(release("kit-v0.2.0", false, false))))
	}))
	t.Cleanup(srv.Close)
	c := New("rise-x/rise-x-ai-public-marketplace")
	c.APIBaseURL = srv.URL

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, _, err := c.Latest(ctx, "v0.1.0"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a deadline", err)
	}
	rel, newer, err := c.Latest(context.Background(), "v0.1.0")
	if err != nil || !newer || rel.Version != "v0.2.0" {
		t.Fatalf("after the deadline: rel=%+v newer=%v err=%v", rel, newer, err)
	}
	if n := hits.Load(); n != 2 {
		t.Fatalf("server saw %d requests, want 2 (the timed-out one was not cached)", n)
	}
}

// A later failure keeps the last good list rather than wiping it.
func TestLatest_FailureKeepsLastGoodList(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(list(release("kit-v0.2.0", false, false))))
	}))
	t.Cleanup(srv.Close)
	c := New("rise-x/rise-x-ai-public-marketplace")
	c.APIBaseURL = srv.URL

	if _, _, err := c.Latest(context.Background(), "v0.1.0"); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	c.ForgetFailures()
	c.mu.Lock()
	c.at = time.Time{} // expire the good answer
	c.mu.Unlock()
	if _, _, err := c.Latest(context.Background(), "v0.1.0"); err == nil {
		t.Fatal("expected the 500")
	}
	c.mu.Lock()
	kept := len(c.releases)
	c.mu.Unlock()
	if kept != 1 {
		t.Fatalf("releases after a failure = %d, want the last good list kept", kept)
	}
}
