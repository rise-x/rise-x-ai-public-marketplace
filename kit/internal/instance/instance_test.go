package instance

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// cacheHome points UserCacheDir at a temp tree, so a test never touches the
// real one.
func cacheHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if os.Getenv("HOME") != "" {
		t.Setenv("HOME", dir)
	}
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("LocalAppData", filepath.Join(dir, "cache"))
}

// kitOn starts a server that answers like a kit, on a real loopback port.
func kitOn(t *testing.T) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(HeaderName, "test")
	}))
	t.Cleanup(srv.Close)
	return portOf(t, srv.URL)
}

func portOf(t *testing.T, url string) int {
	t.Helper()
	n, err := strconv.Atoi(url[strings.LastIndex(url, ":")+1:])
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRunning_ReopensARecordedKit(t *testing.T) {
	cacheHome(t)
	port := kitOn(t)
	if err := Record(port, "0.1.0"); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if got, want := Running(), URL(port); got != want {
		t.Fatalf("Running = %q, want %q", got, want)
	}
}

// The whole point of the probe: a recorded port that has since gone away, or
// been taken by something else, must not be reopened.
func TestRunning_StaleRecord(t *testing.T) {
	t.Run("no server", func(t *testing.T) {
		cacheHome(t)
		if err := Record(freePort(t), "0.1.0"); err != nil {
			t.Fatal(err)
		}
		if got := Running(); got != "" {
			t.Fatalf("Running = %q, want \"\" for a port nothing answers on", got)
		}
	})
	t.Run("someone else's server", func(t *testing.T) {
		cacheHome(t)
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer other.Close()
		if err := Record(portOf(t, other.URL), "0.1.0"); err != nil {
			t.Fatal(err)
		}
		if got := Running(); got != "" {
			t.Fatalf("Running = %q, want \"\": that port is not a kit", got)
		}
	})
}

func TestRunning_NoRecord(t *testing.T) {
	cacheHome(t)
	if got := Running(); got != "" {
		t.Fatalf("Running = %q, want \"\"", got)
	}
}

func TestRunning_UnreadableRecord(t *testing.T) {
	cacheHome(t)
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Running(); got != "" {
		t.Fatalf("Running = %q, want \"\"", got)
	}
}

// The record holds no token: reopening needs the address, and the page is
// served the token by the server itself.
func TestRecord_KeepsNoSecretAndIsOwnerOnly(t *testing.T) {
	cacheHome(t)
	if err := Record(12345, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	path, _ := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(data)), "token") {
		t.Fatalf("record mentions a token: %s", data)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("record mode = %v, want 0600", perm)
	}
}

func TestClear_RemovesOurOwnRecordOnly(t *testing.T) {
	cacheHome(t)
	if err := Record(12345, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	path, _ := Path()

	// A kit started later owns the file; ours must leave it alone.
	if err := os.WriteFile(path, []byte(`{"port":999,"pid":999999}`), 0o600); err != nil {
		t.Fatal(err)
	}
	Clear()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Clear removed another instance's record: %v", err)
	}

	if err := Record(12345, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	Clear()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Clear left our own record behind: %v", err)
	}
}

// freePort returns a port nothing is listening on.
func freePort(t *testing.T) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	port := portOf(t, srv.URL)
	srv.Close()
	return port
}
