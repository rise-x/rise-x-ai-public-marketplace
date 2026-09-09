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

// Running returns the URL of a kit that is already listening, or "" when there
// is none. A file naming a port nothing answers on, or a port something else
// took, counts as none.
func Running() string {
	path, err := Path()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil || s.Port <= 0 {
		return ""
	}
	if !Answers(s.Port) {
		return ""
	}
	return URL(s.Port)
}

// Answers reports whether a kit is serving on port.
func Answers(port int) bool {
	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Get(URL(port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.Header.Get(HeaderName) != ""
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
