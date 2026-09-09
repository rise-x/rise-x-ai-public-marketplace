package mcp

import (
	"encoding/json"
	"net"
	"net/url"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// The two hosts Rise-X serves MCP from today.
const (
	prodURL = "https://mcp.rise-x.io/mcp"
	testURL = "https://mcp-test.rise-x.io/mcp"
)

var currentHosts = []string{"mcp.rise-x.io", "mcp-test.rise-x.io"}

// oldHostSuffixes are shared domains Rise-X has served MCP from before the
// move to mcp.rise-x.io (each entry written with its leading "."), such as
// ".azurecontainerapps.io"; this is the one place to add them. Other tenants
// use these domains too, so a host ending in one is only a hint — see
// retiredHost. A host known to be Rise-X's own, no hint needed, belongs in
// exactStaleHosts instead.
var oldHostSuffixes = []string{".azurecontainerapps.io"}

// exactStaleHosts are old Rise-X MCP hosts known exactly, each mapped to the
// address it should now point at. A match here identifies a Rise-X server on
// its own, no name/URL hint needed, and picks the suggested URL directly
// instead of sniffing the name or URL for "test".
var exactStaleHosts = map[string]string{
	// The dev environment; it maps to test.
	"mcp-server.bluefield-efc1f90d.australiaeast.azurecontainerapps.io": testURL,
	// Production.
	"mcp-server.lemonmeadow-b9fe5140.australiaeast.azurecontainerapps.io": prodURL,
}

// Scopes a stale server can be configured at.
const (
	ScopeUser    = "user"    // ~/.claude.json, top-level mcpServers
	ScopeLocal   = "local"   // ~/.claude.json, one project's mcpServers
	ScopeDesktop = "desktop" // claude_desktop_config.json; the CLI cannot fix these
)

// Stale is one MCP server pointing at an address Rise-X has moved off.
type Stale struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
	// ProjectPath is set for ScopeLocal only: the project the entry belongs
	// to, and the directory `claude mcp` must run in to change it.
	ProjectPath  string `json:"projectPath,omitempty"`
	URL          string `json:"url"`
	SuggestedURL string `json:"suggestedUrl"`
	// Transport is the entry's own "type", so an sse server is put back as
	// sse rather than silently rewritten as http.
	Transport string `json:"transport,omitempty"`
	// HasHeaders marks an entry carrying static headers, an Authorization
	// among them for a hand-added server. `claude mcp add` as the fix runs it
	// cannot put those back, and the fix is a remove followed by an add, so
	// repointing one here would destroy a credential that may exist nowhere
	// else. Reported, never fixed from the kit.
	HasHeaders bool `json:"hasHeaders,omitempty"`
}

// Fixable reports whether the kit may repoint this entry itself.
func (s Stale) Fixable() bool { return s.Scope != ScopeDesktop && !s.HasHeaders }

type configuredEntry struct {
	Type    string            `json:"type,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type claudeJSON struct {
	McpServers map[string]configuredEntry `json:"mcpServers"`
	Projects   map[string]struct {
		McpServers map[string]configuredEntry `json:"mcpServers"`
	} `json:"projects"`
}

// Scan reports the stale Rise-X connections in ~/.claude.json (user scope
// plus every project's local scope) and in the Desktop app's own config.
// Unreadable or invalid files contribute nothing.
func Scan(claudeJSONPath, desktopConfigPath string) []Stale {
	var out []Stale

	var cfg claudeJSON
	if readJSON(claudeJSONPath, &cfg) {
		out = append(out, staleIn(cfg.McpServers, ScopeUser, "")...)
		for _, path := range sortedKeys(cfg.Projects) {
			out = append(out, staleIn(cfg.Projects[path].McpServers, ScopeLocal, path)...)
		}
	}

	var desktop struct {
		McpServers map[string]configuredEntry `json:"mcpServers"`
	}
	if readJSON(desktopConfigPath, &desktop) {
		out = append(out, staleIn(desktop.McpServers, ScopeDesktop, "")...)
	}
	return out
}

func staleIn(servers map[string]configuredEntry, scope, projectPath string) []Stale {
	var out []Stale
	for _, name := range sortedKeys(servers) {
		entry := servers[name]
		if !remoteEntry(entry) || !isStale(name, entry.URL) {
			continue
		}
		transport := strings.ToLower(entry.Type)
		if transport == "" {
			transport = "http"
		}
		out = append(out, Stale{Name: name, Scope: scope, ProjectPath: projectPath,
			URL: entry.URL, SuggestedURL: SuggestedURL(entry.URL),
			Transport: transport, HasHeaders: len(entry.Headers) > 0})
	}
	return out
}

// remoteEntry keeps HTTP and SSE servers only: a stdio command has no address
// to be stale.
func remoteEntry(e configuredEntry) bool {
	switch strings.ToLower(e.Type) {
	case "http", "sse", "":
		return e.URL != ""
	default:
		return false
	}
}

// isStale reports whether a server points at an address Rise-X has moved off:
// an exactly known Rise-X host, or a shared old-host domain whose name or URL
// also hints at Rise-X. A partner's own Rise-X MCP server, hosted anywhere
// else, is never stale.
func isStale(name, rawURL string) bool {
	host := hostOf(rawURL)
	if host == "" || localHost(host) || slices.Contains(currentHosts, host) {
		return false
	}
	if _, ok := exactStaleHosts[host]; ok {
		return true
	}
	return retiredHost(host) && (riseXHint(name) || riseXHint(rawURL))
}

// SuggestedURL is the current address a stale server should point at: the
// mapped address for a host in exactStaleHosts, the test environment when the
// host itself is a test host, production otherwise.
func SuggestedURL(rawURL string) string {
	host := hostOf(rawURL)
	if u, ok := exactStaleHosts[host]; ok {
		return u
	}
	if testHostRe.MatchString(host) {
		return testURL
	}
	return prodURL
}

// testHostRe matches a host carrying "test" as a whole label or a
// dash-separated word: "mcp-test.rise-x.io", "x-test.example". The host is the
// only thing that decides, so neither a connector named "rise-x-latest" nor a
// "/latest/" path can send a production connector to the test environment.
var testHostRe = regexp.MustCompile(`(^|[.-])test([.-]|$)`)

// riseXHint reports whether s names Rise-X in any of the spellings that have
// been used for a connector or a host.
func riseXHint(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "rise-x") || strings.Contains(s, "risex") ||
		strings.Contains(s, "rise_x")
}

// retiredHost reports whether host lives on one of oldHostSuffixes' shared
// domains. A match is a hint the host is old, not proof by itself — isStale
// also requires a Rise-X name/URL hint before calling it stale.
func retiredHost(host string) bool {
	for _, suffix := range oldHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// localHost reports whether host is a machine-local address. A developer's own
// server is not an old address, and repointing one at production would break
// their setup.
func localHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified()
	}
	return false
}

// hostOf returns rawURL's hostname, or "" when it carries none - a typo like
// "https:/host/mcp" parses with an empty host, and the kit must not guess at
// what it meant.
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func readJSON(path string, into any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, into) == nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
