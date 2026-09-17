package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

// claudeJSONFixture is ~/.claude.json with one stale server at user scope, one
// at a project's local scope, one already on the current address, a stdio
// server, a machine-local dev server, and someone else's app on the Azure
// domain the old Rise-X addresses lived on.
const claudeJSONFixture = `{
  "numStartups": 12,
  "mcpServers": {
    "rise-x": {"type": "http", "url": "https://mcp-server-prod.bluesea.eastus.azurecontainerapps.io/mcp"},
    "rise-x-current": {"type": "http", "url": "https://mcp.rise-x.io/mcp"},
    "codegraph": {"type": "stdio", "command": "codegraph", "args": ["serve"]},
    "billing-api": {"type": "http", "url": "https://billing-api.bluesea.eastus.azurecontainerapps.io/mcp"},
    "rise-x-local": {"type": "http", "url": "http://localhost:8080/mcp"}
  },
  "projects": {
    "/Users/p/one": {
      "mcpServers": {
        "risex-test": {"type": "http", "url": "https://mcp-server-test.bluesea.eastus.azurecontainerapps.io/mcp"}
      }
    },
    "/Users/p/two": {"mcpServers": {}}
  }
}`

const desktopJSONFixture = `{
  "mcpServers": {
    "Rise_X": {"type": "http", "url": "https://mcp-old.bluesea.eastus.azurecontainerapps.io/mcp"}
  }
}`

func writeFixture(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScan(t *testing.T) {
	got := Scan(writeFixture(t, ".claude.json", claudeJSONFixture),
		writeFixture(t, "claude_desktop_config.json", desktopJSONFixture))

	want := []Stale{
		{Name: "rise-x", Scope: ScopeUser, Transport: "http",
			URL:          "https://mcp-server-prod.bluesea.eastus.azurecontainerapps.io/mcp",
			SuggestedURL: "https://mcp.rise-x.io/mcp"},
		{Name: "risex-test", Scope: ScopeLocal, ProjectPath: "/Users/p/one", Transport: "http",
			URL:          "https://mcp-server-test.bluesea.eastus.azurecontainerapps.io/mcp",
			SuggestedURL: "https://mcp-test.rise-x.io/mcp"},
		{Name: "Rise_X", Scope: ScopeDesktop, Transport: "http",
			URL:          "https://mcp-old.bluesea.eastus.azurecontainerapps.io/mcp",
			SuggestedURL: "https://mcp.rise-x.io/mcp"},
	}
	if len(got) != len(want) {
		t.Fatalf("scanned %d servers, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stale[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// Someone else's app on the shared Azure domain is not a Rise-X connection,
// and neither is a developer's own server on this machine.
func TestScan_LeavesOtherServersAlone(t *testing.T) {
	got := Scan(writeFixture(t, ".claude.json", claudeJSONFixture), "")
	for _, st := range got {
		switch st.Name {
		case "billing-api", "rise-x-local", "rise-x-current", "codegraph":
			t.Errorf("%s was reported stale: %+v", st.Name, st)
		}
	}
}

func TestScan_MissingFiles(t *testing.T) {
	if got := Scan(filepath.Join(t.TempDir(), "nope.json"), ""); got != nil {
		t.Fatalf("Scan = %+v, want nil", got)
	}
	if got := Scan(writeFixture(t, ".claude.json", "{not json"), ""); got != nil {
		t.Fatalf("Scan of invalid JSON = %+v, want nil", got)
	}
}

// The environment comes from the host alone: a connector named "test", or a
// path carrying "latest", must not repoint a production server at test.
func TestSuggestedURL(t *testing.T) {
	cases := []struct{ url, want string }{
		{"https://old.example.com/mcp", prodURL},
		{"https://mcp-test.old.example.com/mcp", testURL},
		{"https://MCP-TEST.example.com/mcp", testURL},
		{"https://x-test.example/mcp", testURL},
		{"https://test.example.com/mcp", testURL},
		{"https://rise-x-latest.bluesea.eastus.azurecontainerapps.io/mcp", prodURL},
		{"https://mcp-server-prod.bluesea.eastus.azurecontainerapps.io/latest/mcp", prodURL},
		{"https://contest.example.com/mcp", prodURL},
		// The bluefield host is the old dev environment; it maps to test even
		// though nothing in the host says "test" - the exact-host map is
		// consulted first.
		{"https://mcp-server.bluefield-efc1f90d.australiaeast.azurecontainerapps.io/mcp", testURL},
		{"https://mcp-server.lemonmeadow-b9fe5140.australiaeast.azurecontainerapps.io/mcp", prodURL},
	}
	for _, c := range cases {
		if got := SuggestedURL(c.url); got != c.want {
			t.Errorf("SuggestedURL(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestIsStale(t *testing.T) {
	cases := []struct {
		name, url string
		want      bool
	}{
		{"rise-x", "https://mcp.rise-x.io/mcp", false},
		{"rise-x-test", "https://mcp-test.rise-x.io/mcp", false},
		{"rise-x", "https://mcp.rise-x.io:443/mcp", false},
		// A partner's own Rise-X MCP, hosted anywhere but a known-retired
		// domain, is not stale - only the domains in oldHostSuffixes are.
		{"rise-x", "https://anything.example.com/mcp", false},
		{"anything", "https://risex.example.com/mcp", false},
		{"other", "https://example.com/mcp", false},
		// The shared azurecontainerapps.io domain is only a hint: a hit there
		// needs a Rise-X name or URL too.
		{"billing-api", "https://billing-api.bluesea.eastus.azurecontainerapps.io/mcp", false},
		{"rise-x", "https://mcp-server.bluesea.eastus.azurecontainerapps.io/mcp", true},
		// A typo leaves no host to judge, and the kit must not guess.
		{"rise-x-mcp-local", "https:/0.0.0.0:8080/mcp", false},
		{"rise-x-mcp-local", "http://127.0.0.1:8080/mcp", false},
		{"rise-x", "http://192.168.1.9:8080/mcp", false},
		{"rise-x", "https://internal-npm.local:3000/mcp", false},
		// An exact known host flags on its own, no rise-x hint in the name.
		{"mcp-dev", "https://mcp-server.bluefield-efc1f90d.australiaeast.azurecontainerapps.io/mcp", true},
		{"mcp-prod", "https://mcp-server.lemonmeadow-b9fe5140.australiaeast.azurecontainerapps.io/mcp", true},
	}
	for _, c := range cases {
		if got := isStale(c.name, c.url); got != c.want {
			t.Errorf("isStale(%q, %q) = %v, want %v", c.name, c.url, got, c.want)
		}
	}
}

// An sse entry must be put back as sse, and an entry carrying its own headers
// must not be repointed at all: the fix is a remove plus an add, and the add
// cannot recreate a header the partner set by hand.
func TestScan_TransportAndHeaderBoundEntries(t *testing.T) {
	const body = `{
  "mcpServers": {
    "rise-x-sse": {"type": "sse", "url": "https://mcp-server-prod.bluesea.eastus.azurecontainerapps.io/mcp"},
    "rise-x-auth": {"type": "http", "url": "https://mcp-server-prod.bluesea.eastus.azurecontainerapps.io/mcp",
                    "headers": {"Authorization": "Bearer nowhere-else"}}
  }
}`
	got := Scan(writeFixture(t, ".claude.json", body), "")
	if len(got) != 2 {
		t.Fatalf("scanned %d, want 2: %+v", len(got), got)
	}
	byName := map[string]Stale{got[0].Name: got[0], got[1].Name: got[1]}

	if sse := byName["rise-x-sse"]; sse.Transport != "sse" || !sse.Fixable() {
		t.Errorf("sse entry = %+v, want transport sse and fixable", sse)
	}
	if auth := byName["rise-x-auth"]; !auth.HasHeaders || auth.Fixable() {
		t.Errorf("header-bound entry = %+v, want hasHeaders and not fixable", auth)
	}
}
