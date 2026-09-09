package synced

import (
	"os"
	"path/filepath"
	"testing"
)

// manifestEntry is one plugins[] entry of the manifest fixture.
type manifestEntry struct {
	id, name, marketplace, installedBy, version string
	// noDir leaves the plugin directory out, the state of an entry the
	// account wants but the app has not materialised.
	noDir bool
}

// writeManifest builds <dataDir>/local-agent-mode-sessions/<org>/<account>/rpm
// with a manifest.json and one directory per entry.
func writeManifest(t *testing.T, dataDir string, entries ...manifestEntry) string {
	t.Helper()
	root := filepath.Join(dataDir, "local-agent-mode-sessions", "org-1", "account-1", "rpm")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	body := `{"lastUpdated":1788855954758,"plugins":[`
	for i, e := range entries {
		if i > 0 {
			body += ","
		}
		body += `{"id":"` + e.id + `","name":"` + e.name + `","marketplaceName":"` + e.marketplace +
			`","installedBy":"` + e.installedBy + `","installationPreference":"auto_install"}`
		if e.noDir {
			continue
		}
		writePluginDir(t, filepath.Join(root, e.id), e.name, e.version)
	}
	body += `]}`

	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func writePluginDir(t *testing.T, dir, name, version string) {
	t.Helper()
	manifestDir := filepath.Join(dir, ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if version == "" {
		return // an installed plugin whose version cannot be read
	}
	body := `{"name":"` + name + `","version":"` + version + `"}`
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The same plugin can be listed twice: pushed by the organisation and chosen
// by the account holder. Read reports both, with their own sources.
func TestRead_OrgAndUserEntries(t *testing.T) {
	dataDir := t.TempDir()
	writeManifest(t, dataDir,
		manifestEntry{id: "plugin_user", name: "rise-x-mcp", marketplace: "rise-x-ai-public-marketplace",
			installedBy: "user", version: "1.3.4"},
		manifestEntry{id: "plugin_org", name: "rise-x-mcp", marketplace: "rise-x/rise-x-ai-marketplace",
			installedBy: "auto", version: "1.3.4"},
	)

	got, err := Read(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("read %d plugins, want 2: %+v", len(got), got)
	}
	if got[0].Org || got[0].Source != "rise-x-ai-public-marketplace" || got[0].Version != "1.3.4" {
		t.Errorf("user entry = %+v", got[0])
	}
	if !got[1].Org || got[1].Source != "rise-x/rise-x-ai-marketplace" {
		t.Errorf("org entry = %+v", got[1])
	}
	if want := filepath.Join(got[1].Dir, ".mcp.json"); MCPConfigPath(got[1]) != want {
		t.Errorf("MCPConfigPath = %q, want %q", MCPConfigPath(got[1]), want)
	}
}

// The manifest names what the account wants; the directory is what the
// machine has. An entry with no directory is not installed.
func TestRead_SkipsEntryWithoutDirectory(t *testing.T) {
	dataDir := t.TempDir()
	writeManifest(t, dataDir,
		manifestEntry{id: "plugin_gone", name: "rise-x-apps", marketplace: "m", installedBy: "auto", noDir: true},
		manifestEntry{id: "plugin_here", name: "rise-x-mcp", marketplace: "m", installedBy: "auto", version: "1.3.4"},
	)

	got, err := Read(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "rise-x-mcp" {
		t.Fatalf("read %+v, want only rise-x-mcp", got)
	}
}

// A plugin directory with no readable plugin.json is still installed; only its
// version is unknown.
func TestRead_MissingVersion(t *testing.T) {
	dataDir := t.TempDir()
	writeManifest(t, dataDir,
		manifestEntry{id: "plugin_x", name: "rise-x-mcp", marketplace: "m", installedBy: "auto"})

	got, _ := Read(dataDir)
	if len(got) != 1 || got[0].Version != "" || !got[0].Org {
		t.Fatalf("read %+v", got)
	}
}

func TestRead_NoDesktopData(t *testing.T) {
	got, err := Read(filepath.Join(t.TempDir(), "nothing-here"))
	if err != nil || got != nil {
		t.Fatalf("Read = %+v, %v; want nil, nil", got, err)
	}
	if got, err := Read(""); err != nil || got != nil {
		t.Fatalf("Read(\"\") = %+v, %v; want nil, nil", got, err)
	}
}

// An invalid manifest is reported, and never mistaken for "nothing synced".
func TestRead_InvalidManifest(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "local-agent-mode-sessions", "org-1", "account-1", "rpm")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dataDir); err == nil {
		t.Fatal("Read accepted an invalid manifest")
	}
}

func TestReadClaudeDir(t *testing.T) {
	claudeDir := t.TempDir()
	writePluginDir(t, filepath.Join(claudeDir, "plugins", "synced", "rise-x-mcp"), "rise-x-mcp", "1.3.4")

	got, err := ReadClaudeDir(claudeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("read %d plugins, want 1: %+v", len(got), got)
	}
	if got[0].Name != "rise-x-mcp" || got[0].Version != "1.3.4" || !got[0].Org {
		t.Errorf("plugin = %+v", got[0])
	}
}

// A directory whose plugin.json is missing still names the plugin.
func TestReadClaudeDir_NameFromDirectory(t *testing.T) {
	claudeDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(claudeDir, "plugins", "synced", "rise-x-apps"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, _ := ReadClaudeDir(claudeDir)
	if len(got) != 1 || got[0].Name != "rise-x-apps" {
		t.Fatalf("read %+v", got)
	}
}
