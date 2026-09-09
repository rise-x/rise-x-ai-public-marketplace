package instance

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	if err := Record(port); err != nil {
		t.Fatalf("Record: %v", err)
	}
	got := Running()
	if got == nil || got.URL != URL(port) {
		t.Fatalf("Running = %+v, want %s", got, URL(port))
	}
	if got.Version != "test" {
		t.Fatalf("Version = %q, want the header's value", got.Version)
	}
}

// The whole point of the probe: a recorded port that has since gone away, or
// been taken by something else, must not be reopened.
func TestRunning_StaleRecord(t *testing.T) {
	t.Run("no server", func(t *testing.T) {
		cacheHome(t)
		if err := Record(freePort(t)); err != nil {
			t.Fatal(err)
		}
		if got := Running(); got != nil {
			t.Fatalf("Running = %+v, want nil", got)
		}
	})
	t.Run("someone else's server", func(t *testing.T) {
		cacheHome(t)
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer other.Close()
		if err := Record(portOf(t, other.URL)); err != nil {
			t.Fatal(err)
		}
		if got := Running(); got != nil {
			t.Fatalf("Running = %+v, want nil", got)
		}
	})
}

func TestRunning_NoRecord(t *testing.T) {
	cacheHome(t)
	if got := Running(); got != nil {
		t.Fatalf("Running = %+v, want nil", got)
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
	if got := Running(); got != nil {
		t.Fatalf("Running = %+v, want nil", got)
	}
}

// The record holds no token: reopening needs the address, and the page is
// served the token by the server itself.
func TestRecord_KeepsNoSecretAndIsOwnerOnly(t *testing.T) {
	cacheHome(t)
	if err := Record(12345); err != nil {
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
	if err := Record(12345); err != nil {
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

	if err := Record(12345); err != nil {
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

// Two launches racing before either has bound must not both start: the jobs
// slot and the write slot are per-process.
func TestLock_OnlyOneHolder(t *testing.T) {
	cacheHome(t)
	release, err := Lock()
	if release == nil {
		t.Fatalf("first Lock did not take the lock: %v", err)
	}
	if second, err := Lock(); second != nil {
		t.Fatal("second Lock took a lock the first one holds")
	} else if err != nil {
		t.Fatalf("contention reported as a failure: %v", err)
	}
	release()
	release2, err := Lock()
	if release2 == nil {
		t.Fatalf("Lock did not free on release: %v", err)
	}
	release2()
}

// The case the sequential test above cannot reach: several launches arriving
// at once, including over a lock file left by a dead holder, must produce
// exactly one winner.
func TestLock_ConcurrentLaunchesLeaveOneWinner(t *testing.T) {
	cacheHome(t)
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".lock", []byte("999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	const launches = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	var releases []func()
	start := make(chan struct{})
	for i := 0; i < launches; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if release, _ := Lock(); release != nil {
				mu.Lock()
				releases = append(releases, release)
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(releases) != 1 {
		t.Fatalf("%d launches took the lock, want exactly 1", len(releases))
	}
	releases[0]()
}

// A kit that was killed leaves its lock file behind; the OS drops the lock
// with the process, so the next launch must take it rather than time out.
func TestLock_SurvivesADeadHolder(t *testing.T) {
	cacheHome(t)
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// Debris from a killed kit: the file is there, nothing holds it.
	if err := os.WriteFile(path+".lock", []byte("999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := Lock()
	if release == nil {
		t.Fatalf("a lock file with no live holder still blocked the launch: %v", err)
	}
	release()
}

// An upgraded kit must not silently hand back the old version's window.
func TestRunning_ReportsTheServedVersion(t *testing.T) {
	cacheHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(HeaderName, "0.1.0")
	}))
	defer srv.Close()
	if err := Record(portOf(t, srv.URL)); err != nil {
		t.Fatal(err)
	}
	got := Running()
	if got == nil || got.Version != "0.1.0" {
		t.Fatalf("Running = %+v, want version 0.1.0", got)
	}
}

// Removing the lock file while still holding it let one launch lock the inode
// being removed while the next created and locked a fresh one at the same
// name, so two kits each thought they were the only one. The file is now left
// in place, and this is the deterministic half of that: releasing must not
// unlink it.
func TestLock_ReleaseLeavesTheFileInPlace(t *testing.T) {
	cacheHome(t)
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	release, err := Lock()
	if release == nil {
		t.Fatalf("Lock: %v", err)
	}
	release()
	if _, err := os.Stat(path + ".lock"); err != nil {
		t.Fatalf("stat lock file after release: %v; removing it reopens the two-winner window", err)
	}
}

// The other half: launches taking and dropping the lock while others arrive
// must never overlap. Under -race this is also what catches a release path
// that lets a second holder in.
func TestLock_NeverTwoHoldersUnderChurn(t *testing.T) {
	cacheHome(t)
	var mu sync.Mutex
	held := 0
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 60; j++ {
				release, err := Lock()
				if release == nil {
					if err != nil {
						t.Errorf("Lock: %v", err)
						return
					}
					continue // somebody else has it, which is the point
				}
				mu.Lock()
				held++
				n := held
				mu.Unlock()
				if n != 1 {
					t.Errorf("%d launches held the lock at once", n)
				}
				mu.Lock()
				held--
				mu.Unlock()
				release()
			}
		}()
	}
	wg.Wait()
}
