// Package mcp parses `claude mcp list` output and reads a plugin's bundled
// .mcp.json, so the UI can show the Rise-X MCP server's connection state
// with the exact URLs a partner needs to add as a custom connector.
package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Server is one line of `claude mcp list` output.
type Server struct {
	// Name is e.g. "plugin:rise-x-mcp:rise-x", or "context7" for a bare one.
	Name string `json:"name"`
	// Target is the command or URL, e.g. "npx -y @upstash/context7-mcp".
	Target string `json:"target"`
	// Status is the raw text after " - ", e.g. "✔ Connected".
	Status string `json:"status"`
}

// Verdict summarizes a server's status by its leading glyph.
type Verdict string

const (
	VerdictConnected    Verdict = "connected"
	VerdictNeedsAuth    Verdict = "needs_auth"
	VerdictFailed       Verdict = "failed"
	VerdictPending      Verdict = "pending"
	VerdictUnknown      Verdict = "unknown"
	VerdictNotInstalled Verdict = "not_installed"
	// VerdictManaged means the servers come from a plugin the Claude Desktop
	// app manages, so `claude mcp list` cannot see their state at all.
	VerdictManaged Verdict = "managed"
)

// Parse reads `claude mcp list`'s text output into a list of servers,
// skipping the health-check banner line and any "[mcp-sdk] ..." noise.
func Parse(raw string) []Server {
	var out []Server
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[mcp-sdk]") || strings.HasPrefix(line, "Checking ") {
			continue
		}
		name, rest, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		target, status, ok := strings.Cut(rest, " - ")
		if !ok {
			continue
		}
		out = append(out, Server{Name: name, Target: target, Status: status})
	}
	return out
}

// riseXPluginPrefix is how the Rise-X MCP server appears in `claude mcp
// list` when installed as a plugin (session name "plugin:<plugin>:<server>").
const riseXPluginPrefix = "plugin:rise-x-mcp:"

// RiseXServers returns the subset of servers belonging to the rise-x-mcp
// plugin.
func RiseXServers(servers []Server) []Server {
	var out []Server
	for _, s := range servers {
		if strings.HasPrefix(s.Name, riseXPluginPrefix) {
			out = append(out, s)
		}
	}
	return out
}

// Overall picks the worst verdict among a set of servers, or
// VerdictNotInstalled if there are none.
func Overall(servers []Server) Verdict {
	if len(servers) == 0 {
		return VerdictNotInstalled
	}
	worst := VerdictConnected
	for _, s := range servers {
		v := statusVerdict(s.Status)
		if rank(v) > rank(worst) {
			worst = v
		}
	}
	return worst
}

// rank orders verdicts worst-last. Unknown sits on its own between pending and
// needs_auth: an unparseable status is a real problem, but the UI must not tell
// a partner to sign in on the strength of a line it could not read.
func rank(v Verdict) int {
	switch v {
	case VerdictConnected:
		return 0
	case VerdictPending:
		return 1
	case VerdictNeedsAuth:
		return 3
	case VerdictFailed:
		return 4
	default: // VerdictUnknown
		return 2
	}
}

func statusVerdict(status string) Verdict {
	switch {
	case strings.HasPrefix(status, "✔"):
		return VerdictConnected
	case strings.HasPrefix(status, "!"):
		return VerdictNeedsAuth
	case strings.HasPrefix(status, "✘"):
		return VerdictFailed
	case strings.HasPrefix(status, "⏸"):
		return VerdictPending
	default:
		return VerdictUnknown
	}
}

// ConfiguredServer is one entry from a plugin's .mcp.json.
type ConfiguredServer struct {
	Name string `json:"name"`
	// Type is "http" for a remote server; empty for a local command.
	Type    string   `json:"type,omitempty"`
	URL     string   `json:"url,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
}

type mcpConfigFile struct {
	McpServers map[string]struct {
		Type    string   `json:"type,omitempty"`
		URL     string   `json:"url,omitempty"`
		Command string   `json:"command,omitempty"`
		Args    []string `json:"args,omitempty"`
	} `json:"mcpServers"`
}

// ReadConfig reads <installPath>/.mcp.json, the plugin-bundled MCP server
// definitions (installPath comes from `claude plugin list --json`).
func ReadConfig(installPath string) ([]ConfiguredServer, error) {
	return ReadConfigFile(filepath.Join(installPath, ".mcp.json"))
}

// ReadConfigFile reads one .mcp.json by path, for a plugin directory the CLI
// does not report.
func ReadConfigFile(path string) ([]ConfiguredServer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f mcpConfigFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	out := make([]ConfiguredServer, 0, len(f.McpServers))
	for name, s := range f.McpServers {
		out = append(out, ConfiguredServer{Name: name, Type: s.Type, URL: s.URL, Command: s.Command, Args: s.Args})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
