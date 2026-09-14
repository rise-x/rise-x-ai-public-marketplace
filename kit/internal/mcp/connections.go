package mcp

import (
	"slices"
	"strings"
)

// Connection is one Rise-X MCP server configured outside the rise-x-mcp
// plugin: at user or local scope in ~/.claude.json, or in the Desktop app's
// own config file. Removing the plugin leaves these behind, so the page shows
// them whether or not the plugin is installed. A server still on an address
// Rise-X has moved off is not here: the stale scan owns that one, with its
// Fix.
type Connection struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
	// ProjectPath is set for ScopeLocal only: the directory `claude mcp
	// remove -s local` must run in.
	ProjectPath string `json:"projectPath,omitempty"`
	URL         string `json:"url"`
	// HasHeaders marks an entry carrying static headers, an Authorization
	// among them for a hand-added server. Removing it would destroy a
	// credential that may exist nowhere else, so the kit never does.
	HasHeaders bool `json:"hasHeaders,omitempty"`
	// Removable is whether the kit may remove this entry itself, decided
	// here so the page and the action agree: not the Desktop app's file,
	// and not an entry with its own headers.
	Removable bool `json:"removable"`
}

// Connections lists the Rise-X connections configured in ~/.claude.json (user
// scope plus every project's local scope) and in the Desktop app's own
// config. Unreadable or invalid files contribute nothing.
func Connections(claudeJSONPath, desktopConfigPath string) []Connection {
	var out []Connection

	var cfg claudeJSON
	if readJSON(claudeJSONPath, &cfg) {
		out = append(out, connectionsIn(cfg.McpServers, ScopeUser, "")...)
		for _, path := range sortedKeys(cfg.Projects) {
			out = append(out, connectionsIn(cfg.Projects[path].McpServers, ScopeLocal, path)...)
		}
	}

	var desktop struct {
		McpServers map[string]configuredEntry `json:"mcpServers"`
	}
	if readJSON(desktopConfigPath, &desktop) {
		out = append(out, connectionsIn(desktop.McpServers, ScopeDesktop, "")...)
	}
	return out
}

func connectionsIn(servers map[string]configuredEntry, scope, projectPath string) []Connection {
	var out []Connection
	for _, name := range sortedKeys(servers) {
		entry := servers[name]
		// The stale check cannot fire today, since isRiseX admits only hosts
		// Rise-X serves from and isStale only hosts it has left; it is here
		// so the two lists stay disjoint if either set of hosts changes.
		if !remoteEntry(entry) || !isRiseX(entry.URL) || isStale(name, entry.URL) {
			continue
		}
		headers := len(entry.Headers) > 0
		out = append(out, Connection{Name: name, Scope: scope, ProjectPath: projectPath,
			URL: entry.URL, HasHeaders: headers, Removable: scope != ScopeDesktop && !headers})
	}
	return out
}

// isRiseX reports whether a server is on a host Rise-X owns. The name is
// deliberately not consulted: this list feeds a remove, and "rise-x" in a
// name a partner chose for their own server must not make it a target.
func isRiseX(rawURL string) bool {
	host := hostOf(rawURL)
	if host == "" || localHost(host) {
		return false
	}
	return slices.Contains(currentHosts, host) || strings.HasSuffix(host, ".rise-x.io")
}
