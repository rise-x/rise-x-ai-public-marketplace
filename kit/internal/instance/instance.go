// Package instance records where a running kit is listening, so a second
// launch reopens that window instead of starting a rival server. Closing the
// browser leaves the kit running by design - a plugin install can outlast the
// tab - and without this the partner has no way back to a page whose port was
// picked at random and printed to a stdout nobody sees.
package instance

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// HeaderName is the response header handleIndex sets. Probing for it proves
// the port belongs to a kit and not to whatever reused the number.
const HeaderName = "X-Rise-X-Kit"

// probeTimeout bounds the check against a recorded port. It is a loopback
// request to a process that is either there or not.
const probeTimeout = 1500 * time.Millisecond

// state is what one running kit writes down. The token is deliberately absent:
// reopening needs only the address, and the page is served the token itself.
type state struct {
	Port int    `json:"port"`
	PID  int    `json:"pid"`
	Ver  string `json:"version,omitempty"`
}

// Path is the file a running kit records itself in. It lives in the user's
// cache directory: losing it costs the reopen and nothing else.
func Path() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "rise-x-kit", "instance.json"), nil
}

// URL is the address a kit on port serves.
func URL(port int) string { return fmt.Sprintf("http://127.0.0.1:%d/", port) }

// Found describes the kit a launch discovered already running.
type Found struct {
	URL string
	// Version is what that kit reports. It differs from ours after an
	// upgrade, when reopening hands back the previous version's window.
	Version string
}

// Running returns the kit that is already listening, or nil when there is
// none. A file naming a port nothing answers on, or a port something else
// took, counts as none.
func Running() *Found {
	path, err := Path()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil || s.Port <= 0 {
		return nil
	}
	version, ok := Answers(s.Port)
	if !ok {
		return nil
	}
	return &Found{URL: URL(s.Port), Version: version}
}

// Answers reports whether a kit is serving on port, and which version it says
// it is. Redirects are refused: a service squatting the port must not be able
// to send the probe somewhere that does carry the header.
func Answers(port int) (version string, ok bool) {
	client := &http.Client{
		Timeout:       probeTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Get(URL(port))
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	v := resp.Header.Get(HeaderName)
	return v, v != ""
}

// Lock takes the single-instance lock, or reports that another launch holds
// it. The jobs slot and the write slot are per-process, so two kits running at
// once could drive `claude plugin install` or rewrite ~/.claude/settings.json
// concurrently, which is what those slots exist to prevent. The probe alone
// cannot close that: two launches racing before either has bound would both
// find nothing. release is nil when the lock was not taken.
func Lock() (release func(), ok bool) {
	path, err := Path()
	if err != nil {
		return nil, false
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false
	}
	name := path + ".lock"
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if !os.IsExist(err) {
			return nil, false
		}
		// A lock left behind by a kit that was killed would wedge every later
		// launch, so an unheld one is cleared rather than trusted.
		if staleLock(name) {
			_ = os.Remove(name)
			return Lock()
		}
		return nil, false
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Close()
	return func() { _ = os.Remove(name) }, true
}

// staleLockAge is how long a lock file may sit before a launch assumes the
// process that made it is gone. Only the moments between two launches matter,
// so this is generous.
const staleLockAge = 2 * time.Minute

// staleLock reports whether the lock is old enough, and unbacked by a running
// kit, to be treated as debris.
func staleLock(name string) bool {
	fi, err := os.Stat(name)
	if err != nil {
		return true
	}
	if time.Since(fi.ModTime()) < staleLockAge {
		return false
	}
	return Running() == nil
}

// Record writes this process's port down. A failure is not fatal: it costs the
// next launch its reopen, not this one its window.
func Record(port int, version string) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(state{Port: port, PID: os.Getpid(), Ver: version})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Clear removes this process's record on the way out, so the next launch does
// not probe a port that has just gone away.
func Clear() {
	path, err := Path()
	if err != nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var s state
	// Only our own record: a kit started later owns the file now.
	if err := json.Unmarshal(data, &s); err == nil && s.PID != os.Getpid() {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
}
