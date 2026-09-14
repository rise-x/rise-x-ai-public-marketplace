package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

const connectionsClaudeJSON = `{
  "mcpServers": {
    "rise-x": {"type": "http", "url": "https://mcp.rise-x.io/mcp"},
    "old-rise-x": {"type": "http", "url": "https://mcp-server.lemonmeadow-b9fe5140.australiaeast.azurecontainerapps.io/mcp"},
    "rise-x-keyed": {"type": "http", "url": "https://mcp.rise-x.io/mcp", "headers": {"Authorization": "Bearer x"}},
    "risex-gateway": {"type": "http", "url": "https://mcp.partnercorp.example/mcp"},
    "context7": {"type": "http", "url": "https://mcp.context7.com/mcp"},
    "codegraph": {"type": "stdio", "command": "codegraph"},
    "rise-x-mcp-local": {"type": "http", "url": "http://127.0.0.1:8080/mcp"},
    "typo": {"type": "http", "url": "https:/0.0.0.0:8080/mcp"}
  },
  "projects": {
    "/work/app": {
      "mcpServers": {
        "rise-x-test": {"type": "http", "url": "https://mcp-test.rise-x.io/mcp"}
      }
    }
  }
}`

func writeTemp(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConnections_RiseXOnlyAcrossScopes(t *testing.T) {
	claudeJSON := writeTemp(t, ".claude.json", connectionsClaudeJSON)
	desktop := writeTemp(t, "claude_desktop_config.json",
		`{"mcpServers":{"Rise-X":{"type":"http","url":"https://mcp.rise-x.io/mcp"}}}`)

	// The stale entry belongs to the stale scan and its Fix; the entry named
	// like Rise-X on a partner's own host is not Rise-X's to remove; the
	// keyed one is listed but never removable.
	got := Connections(claudeJSON, desktop)
	want := []Connection{
		{Name: "rise-x", Scope: ScopeUser, URL: "https://mcp.rise-x.io/mcp", Removable: true},
		{Name: "rise-x-keyed", Scope: ScopeUser, URL: "https://mcp.rise-x.io/mcp", HasHeaders: true},
		{Name: "rise-x-test", Scope: ScopeLocal, ProjectPath: "/work/app", URL: "https://mcp-test.rise-x.io/mcp", Removable: true},
		{Name: "Rise-X", Scope: ScopeDesktop, URL: "https://mcp.rise-x.io/mcp"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d connections %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestConnections_MissingFilesContributeNothing(t *testing.T) {
	dir := t.TempDir()
	if got := Connections(filepath.Join(dir, "none.json"), filepath.Join(dir, "none2.json")); len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}

func TestIsRiseX(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://mcp.rise-x.io/mcp", true},
		{"https://mcp-anything.rise-x.io/mcp", true},
		{"https://mcp-server.bluefield-efc1f90d.australiaeast.azurecontainerapps.io/mcp", false},
		{"https://example.com/mcp", false},
		{"https://mcp.context7.com/mcp", false},
		{"http://localhost:8080/mcp", false},
		{"https:/0.0.0.0:8080/mcp", false},
	}
	for _, c := range cases {
		if got := isRiseX(c.url); got != c.want {
			t.Errorf("isRiseX(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}
