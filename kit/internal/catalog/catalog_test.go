package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func newTestCatalog(t *testing.T, rawBody, apiBody string, apiStatus int) (*Catalog, *int) {
	t.Helper()
	var apiHits int
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rawBody))
	}))
	t.Cleanup(raw.Close)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiHits++
		if apiStatus != http.StatusOK {
			w.WriteHeader(apiStatus)
			return
		}
		w.Write([]byte(apiBody))
	}))
	t.Cleanup(api.Close)

	c := New("rise-x/rise-x-ai-public-marketplace")
	c.RawBaseURL = raw.URL
	c.APIBaseURL = api.URL
	return c, &apiHits
}

func TestRemoteVersion_CachedFiveMinutes(t *testing.T) {
	c, _ := newTestCatalog(t, `{"version":"1.5.0"}`, `{}`, http.StatusOK)

	v, err := c.RemoteVersion(context.Background(), "rise-x-apps")
	if err != nil {
		t.Fatalf("RemoteVersion: %v", err)
	}
	if v != "1.5.0" {
		t.Fatalf("version = %q, want 1.5.0", v)
	}

	// A cached hit doesn't need the server up; break it and confirm no error.
	c.RawBaseURL = "http://127.0.0.1:1" // nothing listening
	v2, err := c.RemoteVersion(context.Background(), "rise-x-apps")
	if err != nil || v2 != "1.5.0" {
		t.Fatalf("expected cached hit, got v=%q err=%v", v2, err)
	}
}

func TestRemoteHEAD_RateLimited_Skip(t *testing.T) {
	c, hits := newTestCatalog(t, `{}`, ``, http.StatusForbidden)

	_, err := c.RemoteHEAD(context.Background())
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if *hits != 1 {
		t.Fatalf("expected 1 API call, got %d", *hits)
	}
}

func TestRemoteHEAD_Success(t *testing.T) {
	c, _ := newTestCatalog(t, `{}`, `{"sha":"abc123"}`, http.StatusOK)

	sha, err := c.RemoteHEAD(context.Background())
	if err != nil {
		t.Fatalf("RemoteHEAD: %v", err)
	}
	if sha != "abc123" {
		t.Fatalf("sha = %q, want abc123", sha)
	}
}

func TestLocalVersion(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins", "rise-x-apps", ".claude-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"),
		[]byte(`{"version":"1.4.0","description":"Rise-X apps"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	c := New("rise-x/rise-x-ai-public-marketplace")
	v, d, err := c.LocalVersion(dir, "rise-x-apps")
	if err != nil {
		t.Fatalf("LocalVersion: %v", err)
	}
	if v != "1.4.0" {
		t.Fatalf("version = %q, want 1.4.0", v)
	}
	if d != "Rise-X apps" {
		t.Fatalf("description = %q, want Rise-X apps", d)
	}
}

func TestLocalHEAD_SymbolicRef(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "main"), []byte("deadbeef\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sha, err := LocalHEAD(dir)
	if err != nil {
		t.Fatalf("LocalHEAD: %v", err)
	}
	if sha != "deadbeef" {
		t.Fatalf("sha = %q, want deadbeef", sha)
	}
}

func TestLocalHEAD_PackedRefs(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	packed := "# pack-refs with: peeled fully-peeled sorted\ncafef00d refs/heads/main\n"
	if err := os.WriteFile(filepath.Join(gitDir, "packed-refs"), []byte(packed), 0o644); err != nil {
		t.Fatal(err)
	}

	sha, err := LocalHEAD(dir)
	if err != nil {
		t.Fatalf("LocalHEAD: %v", err)
	}
	if sha != "cafef00d" {
		t.Fatalf("sha = %q, want cafef00d", sha)
	}
}

func TestLocalHEAD_Detached(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("badc0ffee0ddf00d\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sha, err := LocalHEAD(dir)
	if err != nil {
		t.Fatalf("LocalHEAD: %v", err)
	}
	if sha != "badc0ffee0ddf00d" {
		t.Fatalf("sha = %q, want badc0ffee0ddf00d", sha)
	}
}

// A failed lookup is cached briefly too: offline or rate-limited, the page must
// not re-attempt every plugin at the client timeout on every refresh.
func TestRemoteVersion_NegativeCached(t *testing.T) {
	var hits int
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(raw.Close)
	c := New("rise-x/rise-x-ai-public-marketplace")
	c.RawBaseURL = raw.URL

	for i := 0; i < 3; i++ {
		if _, err := c.RemoteVersion(context.Background(), "rise-x-apps"); !errors.Is(err, ErrRateLimited) {
			t.Fatalf("call %d err = %v, want ErrRateLimited", i, err)
		}
	}
	if hits != 1 {
		t.Fatalf("made %d requests, want 1 (the failure should be cached)", hits)
	}
}

func TestRemoteHEAD_NegativeCached(t *testing.T) {
	c, hits := newTestCatalog(t, `{}`, ``, http.StatusForbidden)
	for i := 0; i < 3; i++ {
		if _, err := c.RemoteHEAD(context.Background()); !errors.Is(err, ErrRateLimited) {
			t.Fatalf("call %d err = %v, want ErrRateLimited", i, err)
		}
	}
	if *hits != 1 {
		t.Fatalf("made %d API calls, want 1 (the failure should be cached)", *hits)
	}
}
