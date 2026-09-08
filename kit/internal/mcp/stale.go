package mcp

import (
	"encoding/json"
	"net"
	"net/url"
	"os"
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

// oldHostSuffixes are the addresses Rise-X served MCP from before the move to
// mcp.rise-x.io; this is the one place to add them. An entry starting with "."
// is a whole domain other tenants share, so a server there is only reported
// when its own name or URL also says rise-x. An exact hostname identifies a
// Rise-X server on its own — see exactStaleHosts for the ones known exactly.
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
}

type configuredEntry struct {
	Type string `json:"type,omitempty"`
	URL  string `json:"url,omitempty"`
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
		out = append(out, Stale{Name: name, Scope: scope, ProjectPath: projectPath,
			URL: entry.URL, SuggestedURL: SuggestedURL(name, entry.URL)})
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

// isStale reports whether a server points at an address Rise-X has moved off.
func isStale(name, rawURL string) bool {
	host := hostOf(rawURL)
	if host == "" || localHost(host) || slices.Contains(currentHosts, host) {
		return false
	}
	if _, ok := exactStaleHosts[host]; ok {
		return true
	}
	return retiredHost(host) || riseXHint(name) || riseXHint(rawURL)
}

// SuggestedURL is the current address a stale server should point at: the
// mapped address for a host in exactStaleHosts, the test environment when the
// name or URL says "test", production otherwise.
func SuggestedURL(name, rawURL string) string {
	if u, ok := exactStaleHosts[hostOf(rawURL)]; ok {
		return u
	}
	if riseXTest(name) || riseXTest(rawURL) {
		return testURL
	}
	return prodURL
}

func riseXTest(s string) bool { return strings.Contains(strings.ToLower(s), "test") }

// riseXHint reports whether s names Rise-X in any of the spellings that have
// been used for a connector or a host.
func riseXHint(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "rise-x") || strings.Contains(s, "risex") ||
		strings.Contains(s, "rise_x")
}

// retiredHost reports whether host is one Rise-X itself has moved off.
func retiredHost(host string) bool {
	for _, suffix := range oldHostSuffixes {
		if strings.HasPrefix(suffix, ".") {
			continue // a shared domain never identifies a server on its own
		}
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
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
