package mcp

import (
	"slices"
	"strings"
)

// Connection is one Rise-X MCP server configured outside the rise-x-mcp
// plugin: at user or local scope in ~/.claude.json, or in the Desktop app's
// own config. Removing the plugin leaves these behind, so the page shows
// them whether or not the plugin is installed.
type Connection struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
	// ProjectPath is set for ScopeLocal only: the directory `claude mcp
	// remove -s local` must run in.
	ProjectPath string `json:"projectPath,omitempty"`
	URL         string `json:"url"`
}

// Removable reports whether the kit may remove this entry itself. The
// Desktop app's config is the app's alone.
func (c Connection) Removable() bool { return c.Scope != ScopeDesktop }

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
		if !remoteEntry(entry) || !isRiseX(name, entry.URL) {
			continue
		}
		out = append(out, Connection{Name: name, Scope: scope, ProjectPath: projectPath, URL: entry.URL})
	}
	return out
}

// isRiseX reports whether a server is a Rise-X one: on a current Rise-X
// host, on an address Rise-X has moved off, or on any other host when its
// name or URL says Rise-X. A partner's own server on localhost is not.
func isRiseX(name, rawURL string) bool {
	host := hostOf(rawURL)
	if host == "" || localHost(host) {
		return false
	}
	if slices.Contains(currentHosts, host) || isStale(name, rawURL) {
		return true
	}
	return strings.HasSuffix(host, ".rise-x.io") || riseXHint(name)
}
