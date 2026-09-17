// Package selfupdate finds newer rise-x-kit releases on GitHub, downloads the
// release asset for the running platform, swaps the new binary in, and
// relaunches it. The download is checked against the release's own
// checksums.txt, which guards the transfer, not the publisher: both files
// come from the same release, so this is integrity, not authenticity. A
// development build (version "dev") is never offered an update: Latest
// returns an empty answer without contacting GitHub.
package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrRateLimited means GitHub's API refused the request (typically 403,
// unauthenticated rate limit); callers should skip the update check rather
// than report a failure.
var ErrRateLimited = errors.New("github api rate limited")

// StatusError is a non-200, non-rate-limited response from GitHub. Callers
// tell it apart from a network failure: the repo answered, we just can't use
// the answer.
type StatusError struct {
	URL  string
	Code int
}

func (e *StatusError) Error() string { return fmt.Sprintf("GET %s: HTTP %d", e.URL, e.Code) }

const (
	releasesTTL = 30 * time.Minute
	// failureTTL keeps a failed lookup for a short while so an offline or
	// rate-limited machine doesn't re-attempt at the client timeout on every
	// page refresh.
	failureTTL = 5 * time.Minute
	// tagPrefix is the kit's own release tag namespace; the repo also carries
	// plugin and marketplace releases under other prefixes.
	tagPrefix = "kit-v"
	// maxBodyBytes caps one API response.
	maxBodyBytes = 8 << 20
)

// Release is one kit release on GitHub.
type Release struct {
	Tag        string // "kit-v0.1.0-rc.2"
	Version    string // "v0.1.0-rc.2"
	Prerelease bool
	URL        string            // html_url of the release page
	Assets     map[string]string // asset name -> browser_download_url
}

// Checker asks GitHub for the newest kit release and remembers the answer.
type Checker struct {
	Repo       string // "owner/name"
	APIBaseURL string // default https://api.github.com
	// HTTPClient makes the API call, which is small and must answer fast.
	HTTPClient *http.Client
	// TransferClient downloads the assets. It carries no timeout of its own:
	// a few MB over a slow link takes longer than any fixed budget, so the
	// caller's context bounds it instead.
	TransferClient *http.Client

	mu       sync.Mutex
	releases []Release
	err      error
	at       time.Time
}

func New(repo string) *Checker {
	return &Checker{
		Repo:           repo,
		APIBaseURL:     "https://api.github.com",
		HTTPClient:     &http.Client{Timeout: 10 * time.Second},
		TransferClient: &http.Client{},
	}
}

func (c *Checker) apiClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *Checker) transferClient() *http.Client {
	if c.TransferClient != nil {
		return c.TransferClient
	}
	return http.DefaultClient
}

// ForgetFailures drops a remembered failure so the next Latest retries now.
// A successful answer keeps its cache: nothing on this machine changes what
// GitHub answers, and the unauthenticated rate limit is 60 requests an hour.
func (c *Checker) ForgetFailures() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		c.at = time.Time{}
	}
}

// Latest returns the newest release a binary at current may move to, and
// whether it is newer than current. A prerelease is offered only to a binary
// that is itself on a prerelease.
func (c *Checker) Latest(ctx context.Context, current string) (Release, bool, error) {
	if !Known(current) {
		return Release{}, false, nil
	}
	releases, err := c.list(ctx)
	if err != nil {
		return Release{}, false, err
	}

	allowPre := IsPrerelease(current)
	var best Release
	var found bool
	for _, r := range releases {
		if r.Prerelease && !allowPre {
			continue
		}
		if !found || Compare(r.Version, best.Version) > 0 {
			best, found = r, true
		}
	}
	if !found {
		return Release{}, false, nil
	}
	return best, Compare(best.Version, current) > 0, nil
}

func (c *Checker) list(ctx context.Context) ([]Release, error) {
	c.mu.Lock()
	if fresh(c.at, c.err) {
		releases, err := c.releases, c.err
		c.mu.Unlock()
		return releases, err
	}
	c.mu.Unlock()

	releases, err := c.fetch(ctx)

	// A failure the caller's own context caused is not remembered: a
	// five-second overview check must not decide for the five-minute update
	// that GitHub is down. The client's own timeout, and every other
	// failure, is remembered for failureTTL so an offline machine does not
	// pay the full wait on every page load.
	if err != nil && ctx.Err() != nil {
		return releases, err
	}
	c.mu.Lock()
	c.releases, c.err, c.at = releases, err, time.Now()
	c.mu.Unlock()
	return releases, err
}

func fresh(at time.Time, err error) bool {
	if at.IsZero() {
		return false
	}
	ttl := releasesTTL
	if err != nil {
		ttl = failureTTL
	}
	return time.Since(at) < ttl
}

type releaseJSON struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (c *Checker) fetch(ctx context.Context) ([]Release, error) {
	// The same page size the installers read: the repo also publishes plugin
	// releases, and the newest kit one has to be inside the page.
	url := fmt.Sprintf("%s/repos/%s/releases?per_page=100", c.APIBaseURL, c.Repo)
	body, err := c.get(ctx, url, maxBodyBytes)
	if err != nil {
		return nil, err
	}
	var raw []releaseJSON
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", url, err)
	}

	out := make([]Release, 0, len(raw))
	for _, r := range raw {
		if r.Draft || !strings.HasPrefix(r.TagName, tagPrefix) {
			continue
		}
		version := strings.TrimPrefix(r.TagName, "kit-")
		rel := Release{
			Tag:        r.TagName,
			Version:    version,
			Prerelease: r.Prerelease || IsPrerelease(version),
			URL:        r.HTMLURL,
			Assets:     make(map[string]string, len(r.Assets)),
		}
		for _, a := range r.Assets {
			rel.Assets[a.Name] = a.URL
		}
		out = append(out, rel)
	}
	return out, nil
}

func (c *Checker) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.apiClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{URL: url, Code: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s: response larger than %d bytes", url, limit)
	}
	return body, nil
}

// AssetName is the release asset for this platform and whether one exists.
// The darwin build is a universal binary, so its arch doesn't matter.
func AssetName(version, goos, goarch string) (string, bool) {
	switch {
	case goos == "darwin":
		return fmt.Sprintf("rise-x-kit_%s_darwin_universal.tar.gz", version), true
	case goos == "windows" && goarch == "amd64":
		return fmt.Sprintf("rise-x-kit_%s_windows_amd64.zip", version), true
	}
	return "", false
}

// BinaryName is the kit executable's file name on goos.
func BinaryName(goos string) string {
	if goos == "windows" {
		return "rise-x-kit.exe"
	}
	return "rise-x-kit"
}
