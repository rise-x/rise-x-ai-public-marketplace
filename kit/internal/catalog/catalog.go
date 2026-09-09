// Package catalog compares installed plugin versions and the marketplace
// clone's HEAD against the public repo on GitHub, so the UI can show
// "update available" / "catalog stale" without partners running git by hand.
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrRateLimited means GitHub's API refused the request (typically 403,
// unauthenticated rate limit); callers should skip the stale check rather
// than report false staleness.
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
	remoteVersionTTL = 5 * time.Minute
	remoteHeadTTL    = 10 * time.Minute
	catalogNamesTTL  = 5 * time.Minute
	// failureTTL keeps a failed lookup for a short while so an offline or
	// rate-limited machine doesn't re-attempt every plugin, at the client
	// timeout each, on every page refresh.
	failureTTL = 60 * time.Second
)

// Catalog fetches public plugin.json versions and the marketplace repo's
// main-branch HEAD from GitHub, caching both to stay well under the
// unauthenticated 60 req/hour rate limit.
type Catalog struct {
	Repo       string // "owner/name"
	RawBaseURL string // default https://raw.githubusercontent.com
	APIBaseURL string // default https://api.github.com
	HTTPClient *http.Client

	mu       sync.Mutex
	versions map[string]versionEntry
	headSHA  string
	headErr  error
	headAt   time.Time
	names    []string
	namesErr error
	namesAt  time.Time
}

// versionEntry caches one plugin's public version, or the error that lookup
// failed with.
type versionEntry struct {
	value string
	err   error
	at    time.Time
}

func fresh(at time.Time, err error, ttl time.Duration) bool {
	if at.IsZero() {
		return false
	}
	if err != nil {
		ttl = failureTTL
	}
	return time.Since(at) < ttl
}

// ForgetFailures drops the cached lookup failures, so the next gather retries
// GitHub instead of repeating a minute-old error. Successful lookups keep
// their own cache: nothing on this machine changes what GitHub answers, and
// the rate limit is 60 requests an hour.
func (c *Catalog) ForgetFailures() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, e := range c.versions {
		if e.err != nil {
			delete(c.versions, name)
		}
	}
	if c.headErr != nil {
		c.headAt = time.Time{}
	}
	if c.namesErr != nil {
		c.namesAt = time.Time{}
	}
}

func New(repo string) *Catalog {
	return &Catalog{
		Repo:       repo,
		RawBaseURL: "https://raw.githubusercontent.com",
		APIBaseURL: "https://api.github.com",
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		versions:   map[string]versionEntry{},
	}
}

type pluginManifest struct {
	Version     string `json:"version"`
	Description string `json:"description"`
}

// RemoteVersion fetches plugins/<name>/.claude-plugin/plugin.json from the
// repo's main branch and returns its "version", cached for 5 minutes on
// success and for failureTTL on any failure.
func (c *Catalog) RemoteVersion(ctx context.Context, plugin string) (string, error) {
	c.mu.Lock()
	if e, ok := c.versions[plugin]; ok && fresh(e.at, e.err, remoteVersionTTL) {
		c.mu.Unlock()
		return e.value, e.err
	}
	c.mu.Unlock()

	version, err := c.fetchVersion(ctx, plugin)

	c.mu.Lock()
	c.versions[plugin] = versionEntry{value: version, err: err, at: time.Now()}
	c.mu.Unlock()
	return version, err
}

func (c *Catalog) fetchVersion(ctx context.Context, plugin string) (string, error) {
	url := fmt.Sprintf("%s/%s/main/plugins/%s/.claude-plugin/plugin.json", c.RawBaseURL, c.Repo, plugin)
	body, err := c.get(ctx, url)
	if err != nil {
		return "", err
	}
	var m pluginManifest
	if err := json.Unmarshal(body, &m); err != nil {
		return "", fmt.Errorf("parse %s: %w", url, err)
	}
	return m.Version, nil
}

// LocalVersion reads plugins/<name>/.claude-plugin/plugin.json from the
// marketplace's local clone (installLocation, from `claude plugin
// marketplace list --json`), returning its version and description.
func (c *Catalog) LocalVersion(installLocation, plugin string) (version, description string, err error) {
	path := filepath.Join(installLocation, "plugins", plugin, ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var m pluginManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return "", "", fmt.Errorf("parse %s: %w", path, err)
	}
	return m.Version, m.Description, nil
}

type marketplaceManifest struct {
	Plugins []struct {
		Name string `json:"name"`
	} `json:"plugins"`
}

// CatalogNames fetches .claude-plugin/marketplace.json from the repo's main
// branch and returns the plugin names it lists, cached for 5 minutes.
func (c *Catalog) CatalogNames(ctx context.Context) ([]string, error) {
	c.mu.Lock()
	if fresh(c.namesAt, c.namesErr, catalogNamesTTL) {
		names, err := c.names, c.namesErr
		c.mu.Unlock()
		return names, err
	}
	c.mu.Unlock()

	url := fmt.Sprintf("%s/%s/main/.claude-plugin/marketplace.json", c.RawBaseURL, c.Repo)
	names, err := c.get(ctx, url)
	var out []string
	if err == nil {
		if out, err = manifestNames(names); err != nil {
			err = fmt.Errorf("parse %s: %w", url, err)
		}
	}

	c.mu.Lock()
	c.names, c.namesErr, c.namesAt = out, err, time.Now()
	c.mu.Unlock()
	return out, err
}

// LocalCatalogNames reads the same manifest from the marketplace's local clone.
func LocalCatalogNames(installLocation string) ([]string, error) {
	path := filepath.Join(installLocation, ".claude-plugin", "marketplace.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out, err := manifestNames(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return out, nil
}

func manifestNames(data []byte) ([]string, error) {
	var m marketplaceManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(m.Plugins))
	for _, p := range m.Plugins {
		if p.Name != "" {
			out = append(out, p.Name)
		}
	}
	return out, nil
}

type commitResponse struct {
	SHA string `json:"sha"`
}

// RemoteHEAD fetches the main branch's current commit SHA, cached for 10
// minutes on success and for failureTTL on any failure. Returns ErrRateLimited
// on a 403/429 so the caller can skip the stale check instead of reporting a
// false positive.
func (c *Catalog) RemoteHEAD(ctx context.Context) (string, error) {
	c.mu.Lock()
	if fresh(c.headAt, c.headErr, remoteHeadTTL) {
		sha, err := c.headSHA, c.headErr
		c.mu.Unlock()
		return sha, err
	}
	c.mu.Unlock()

	sha, err := c.fetchHEAD(ctx)

	c.mu.Lock()
	c.headSHA, c.headErr, c.headAt = sha, err, time.Now()
	c.mu.Unlock()
	return sha, err
}

func (c *Catalog) fetchHEAD(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/commits/main", c.APIBaseURL, c.Repo)
	body, err := c.get(ctx, url)
	if err != nil {
		return "", err
	}
	var cr commitResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", fmt.Errorf("parse %s: %w", url, err)
	}
	return cr.SHA, nil
}

func (c *Catalog) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.HTTPClient.Do(req)
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
	return io.ReadAll(resp.Body)
}

// LocalHEAD reads the marketplace clone's current commit SHA from .git,
// handling a symbolic-ref HEAD (normal branch checkout), a detached HEAD
// (bare SHA), and a ref that only exists in packed-refs (shallow clones
// often pack the branch ref instead of writing a loose ref file).
func LocalHEAD(installLocation string) (string, error) {
	gitDir := filepath.Join(installLocation, ".git")
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(head))

	ref, isSymbolic := strings.CutPrefix(line, "ref: ")
	if !isSymbolic {
		return line, nil // detached HEAD: a bare SHA
	}
	// A ref is a path under .git; anything else is not one, and joining it
	// would read a file elsewhere and report its first line as a commit.
	if !strings.HasPrefix(ref, "refs/") || strings.Contains(ref, "..") {
		return "", fmt.Errorf("HEAD names %q, which is not a ref", ref)
	}

	if data, err := os.ReadFile(filepath.Join(gitDir, ref)); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	packed, err := os.ReadFile(filepath.Join(gitDir, "packed-refs"))
	if err != nil {
		return "", fmt.Errorf("ref %s not found as a loose or packed ref", ref)
	}
	for _, l := range strings.Split(string(packed), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "^") {
			continue
		}
		fields := strings.Fields(l)
		if len(fields) == 2 && fields[1] == ref {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("ref %s not found in packed-refs", ref)
}
