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
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("badc0ffee0ddf00d1234567890abcdef12345678\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sha, err := LocalHEAD(dir)
	if err != nil {
		t.Fatalf("LocalHEAD: %v", err)
	}
	if sha != "badc0ffee0ddf00d1234567890abcdef12345678" {
		t.Fatalf("sha = %q, want the full object name", sha)
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

// The raw paths are a contract with the repo layout: plugins/<name>/
// .claude-plugin/plugin.json and .claude-plugin/marketplace.json on main.
// Getting one wrong reads as "no such plugin", not as an error.
func TestRawPaths(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Write([]byte(`{"version":"1.5.0","plugins":[{"name":"rise-x-apps"}]}`))
	}))
	t.Cleanup(srv.Close)

	c := New("rise-x/rise-x-ai-public-marketplace")
	c.RawBaseURL = srv.URL
	if _, err := c.RemoteVersion(context.Background(), "rise-x-apps"); err != nil {
		t.Fatalf("RemoteVersion: %v", err)
	}
	if _, err := c.CatalogNames(context.Background()); err != nil {
		t.Fatalf("CatalogNames: %v", err)
	}

	want := []string{
		"/rise-x/rise-x-ai-public-marketplace/main/plugins/rise-x-apps/.claude-plugin/plugin.json",
		"/rise-x/rise-x-ai-public-marketplace/main/.claude-plugin/marketplace.json",
	}
	if len(paths) != len(want) {
		t.Fatalf("requested %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("request %d = %q, want %q", i, paths[i], want[i])
		}
	}
}

// The ref file is trusted only because of where it sits, so a symlink planted
// under .git/refs must not hand back some unrelated file's first line as a
// commit SHA.
func TestLocalHEAD_RefusesASymlinkedRef(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secret, []byte("sk-do-not-leak\nrest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(gitDir, "refs", "heads", "main")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	sha, err := LocalHEAD(dir)
	if err == nil {
		t.Fatalf("LocalHEAD followed the symlink and returned %q", sha)
	}
	if sha != "" {
		t.Fatalf("sha = %q, want nothing", sha)
	}
}

// A response that keeps going is not a manifest, and reading it whole would
// let whatever answers the request grow the kit's heap.
func TestGet_RefusesAnOversizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := make([]byte, 1<<20)
		for i := 0; i < (maxBodyBytes>>20)+2; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	c := New("rise-x/rise-x-ai-public-marketplace")
	if _, err := c.get(context.Background(), srv.URL); err == nil {
		t.Fatal("a body past the cap was read whole")
	}
}

// HEAD and packed-refs are trusted for where they sit, the same as the loose
// ref, so neither may be followed into an unrelated file. A detached HEAD is
// also checked rather than returned verbatim: without that, whatever the
// first line happened to be was reported as the commit.
func TestLocalHEAD_RefusesSymlinkedSiblingsAndNonSHAs(t *testing.T) {
	t.Run("symlinked HEAD", func(t *testing.T) {
		clone := t.TempDir()
		gitDir := filepath.Join(clone, ".git")
		if err := os.MkdirAll(gitDir, 0o755); err != nil {
			t.Fatal(err)
		}
		secret := filepath.Join(t.TempDir(), "secret")
		if err := os.WriteFile(secret, []byte("badc0ffee0ddf00d1234567890abcdef12345678\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(secret, filepath.Join(gitDir, "HEAD")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if sha, err := LocalHEAD(clone); err == nil {
			t.Fatalf("followed the symlinked HEAD and returned %q", sha)
		}
	})

	t.Run("detached HEAD holding something else", func(t *testing.T) {
		clone := t.TempDir()
		gitDir := filepath.Join(clone, ".git")
		if err := os.MkdirAll(gitDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("#!/bin/sh\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if sha, err := LocalHEAD(clone); err == nil {
			t.Fatalf("reported %q as a commit", sha)
		}
	})
}
