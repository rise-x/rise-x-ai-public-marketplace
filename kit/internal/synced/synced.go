// Package synced reads the plugins the Claude Desktop app materialises from a
// claude.ai account: the ones an organisation pushes to everyone it manages,
// and the ones a user picks from a marketplace other than the CLI's. `claude
// plugin list` does not report these, so without reading them here the kit
// would tell a partner an installed skill is missing.
package synced

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// installedByOrg is manifest.json's "installedBy" value for a plugin the
// organisation pushed; "user" means the account holder chose it.
const installedByOrg = "auto"

// Plugin is one account-synced plugin on this machine.
type Plugin struct {
	Name    string
	Version string
	// Source is the marketplace the account installed it from, per the
	// manifest's marketplaceName.
	Source string
	// Org is true when the organisation pushed the plugin rather than the
	// account holder choosing it.
	Org bool
	// Dir is the materialised plugin directory.
	Dir string
}

// MCPConfigPath is the .mcp.json a synced plugin bundles. `claude mcp list`
// cannot see these servers, so it is the only place their URLs come from.
func MCPConfigPath(p Plugin) string { return filepath.Join(p.Dir, ".mcp.json") }

// DefaultDataDir is where the Claude Desktop app keeps its data.
func DefaultDataDir() string {
	if runtime.GOOS == "windows" {
		// Unverified: the Windows app is assumed to mirror macOS under %APPDATA%.
		return filepath.Join(os.Getenv("APPDATA"), "Claude")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "Claude")
}

type manifestFile struct {
	Plugins []struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		MarketplaceName string `json:"marketplaceName"`
		InstalledBy     string `json:"installedBy"`
	} `json:"plugins"`
}

type pluginFile struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Read collects every plugin listed in the signed-in account's manifests under
// <desktopDataDir>/local-agent-mode-sessions/<account>/<org>/rpm. Every
// account that ever signed in on the machine leaves its manifests behind, and
// only the current one's say what this account has: reading them all reported
// a previous account's organisation pushes as this one's. An entry whose
// plugin directory is not on disk is skipped: the manifest names what the
// account wants, the directory is what the machine actually has.
func Read(desktopDataDir string) ([]Plugin, error) {
	if desktopDataDir == "" {
		return nil, nil
	}
	account := signedInAccount(desktopDataDir)
	if account == "" {
		return nil, nil
	}
	paths, _ := filepath.Glob(filepath.Join(desktopDataDir,
		"local-agent-mode-sessions", account, "*", "rpm", "manifest.json"))

	var out []Plugin
	var errs []error
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var m manifestFile
		if err := json.Unmarshal(data, &m); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		root := filepath.Dir(path)
		for _, entry := range m.Plugins {
			if entry.Name == "" || entry.ID == "" {
				continue
			}
			dir := filepath.Join(root, entry.ID)
			if _, err := os.Stat(dir); err != nil {
				continue
			}
			out = append(out, Plugin{
				Name:    entry.Name,
				Version: readVersion(dir),
				Source:  entry.MarketplaceName,
				Org:     entry.InstalledBy == installedByOrg,
				Dir:     dir,
			})
		}
	}
	return out, errors.Join(errs...)
}

// ReadClaudeDir collects the plugins Claude Code syncs into
// <claudeDir>/plugins/synced/<name>, the layout the docs give for Cowork and
// cloud sessions. Those come from the account, so they count as organisation
// deliveries; the directory carries no marketplace name.
func ReadClaudeDir(claudeDir string) ([]Plugin, error) {
	if claudeDir == "" {
		return nil, nil
	}
	dirs, _ := filepath.Glob(filepath.Join(claudeDir, "plugins", "synced", "*"))

	var out []Plugin
	for _, dir := range dirs {
		fi, err := os.Stat(dir)
		if err != nil || !fi.IsDir() {
			continue
		}
		name, version := readPluginFile(dir)
		if name == "" {
			name = filepath.Base(dir)
		}
		out = append(out, Plugin{Name: name, Version: version, Org: true, Dir: dir})
	}
	return out, nil
}

// Connector is one connector the Desktop app handed a Claude Code session.
// A connector added under Customize > Connectors lives in the account, and
// the session file is the only place on disk that names it: by the name the
// partner typed and the tools it serves, never by address.
type Connector struct {
	Name  string
	Tools []string
}

type sessionFile struct {
	RemoteMcpServersConfig []struct {
		Name  string `json:"name"`
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	} `json:"remoteMcpServersConfig"`
}

// Connectors reads the connectors the Desktop app gave the signed-in
// account's latest Claude Code session, from
// <desktopDataDir>/claude-code-sessions/<account>/<org>/local_*.json. The app
// keeps that file current while the session runs, so the newest one by
// modification time is the account's connectors as of its last activity.
func Connectors(desktopDataDir string) ([]Connector, error) {
	if desktopDataDir == "" {
		return nil, nil
	}
	account := signedInAccount(desktopDataDir)
	if account == "" {
		return nil, nil
	}
	paths, _ := filepath.Glob(filepath.Join(desktopDataDir,
		"claude-code-sessions", account, "*", "local_*.json"))
	newest, at := "", time.Time{}
	for _, path := range paths {
		fi, err := os.Stat(path)
		if err != nil || !fi.ModTime().After(at) {
			continue
		}
		newest, at = path, fi.ModTime()
	}
	if newest == "" {
		return nil, nil
	}
	data, err := os.ReadFile(newest)
	if err != nil {
		return nil, err
	}
	var f sessionFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", newest, err)
	}
	out := make([]Connector, 0, len(f.RemoteMcpServersConfig))
	for _, c := range f.RemoteMcpServersConfig {
		tools := make([]string, 0, len(c.Tools))
		for _, t := range c.Tools {
			tools = append(tools, t.Name)
		}
		out = append(out, Connector{Name: c.Name, Tools: tools})
	}
	return out, nil
}

// configFile is the one key of the Desktop app's config.json read here.
type configFile struct {
	LastKnownAccountUUID string `json:"lastKnownAccountUuid"`
}

// signedInAccount reads which account the Desktop app is signed in as. Empty
// when that cannot be read: no manifest can then be told apart from a
// previous account's, so none is trusted.
func signedInAccount(desktopDataDir string) string {
	data, err := os.ReadFile(filepath.Join(desktopDataDir, "config.json"))
	if err != nil {
		return ""
	}
	var cfg configFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	id := cfg.LastKnownAccountUUID
	// It becomes one segment of the glob above, so it must be exactly that.
	if id == "" || id == "." || id == ".." || id != filepath.Base(id) || strings.ContainsAny(id, `*?[\`) {
		return ""
	}
	return id
}

func readVersion(dir string) string {
	_, version := readPluginFile(dir)
	return version
}

// readPluginFile reads <dir>/.claude-plugin/plugin.json. A missing or
// unreadable manifest reads as empty: the plugin is still installed, its
// version is just unknown.
func readPluginFile(dir string) (name, version string) {
	data, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err != nil {
		return "", ""
	}
	var f pluginFile
	if err := json.Unmarshal(data, &f); err != nil {
		return "", ""
	}
	return f.Name, f.Version
}
