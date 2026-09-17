package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleList = "Checking MCP server health…\n" +
	"\n" +
	"plugin:rise-x-mcp:rise-x: https://mcp.rise-x.io/mcp (HTTP) - ! Needs authentication\n" +
	"[mcp-sdk] some noise line that must be ignored\n" +
	"plugin:context7:context7: npx -y @upstash/context7-mcp - ✔ Connected\n" +
	"plugin:github:github: https://api.githubcopilot.com/mcp/ (HTTP) - ✘ Failed to connect — HTTP 400: Error POSTing to endpoint: bad request: Authorization header is badly formatted\n" +
	"playwright-test: npx playwright run-test-mcp-server - ⏸ Pending approval (run `claude` to approve)\n"

func TestParse(t *testing.T) {
	servers := Parse(sampleList)
	if len(servers) != 4 {
		t.Fatalf("got %d servers, want 4: %+v", len(servers), servers)
	}

	want := []Server{
		{Name: "plugin:rise-x-mcp:rise-x", Target: "https://mcp.rise-x.io/mcp (HTTP)", Status: "! Needs authentication"},
		{Name: "plugin:context7:context7", Target: "npx -y @upstash/context7-mcp", Status: "✔ Connected"},
		{
			Name:   "plugin:github:github",
			Target: "https://api.githubcopilot.com/mcp/ (HTTP)",
			Status: "✘ Failed to connect — HTTP 400: Error POSTing to endpoint: bad request: Authorization header is badly formatted",
		},
		{Name: "playwright-test", Target: "npx playwright run-test-mcp-server", Status: "⏸ Pending approval (run `claude` to approve)"},
	}
	for i, w := range want {
		if servers[i] != w {
			t.Errorf("servers[%d] = %+v, want %+v", i, servers[i], w)
		}
	}
}

func TestRiseXServers(t *testing.T) {
	servers := Parse(sampleList)
	rx := RiseXServers(servers)
	if len(rx) != 1 || rx[0].Name != "plugin:rise-x-mcp:rise-x" {
		t.Fatalf("RiseXServers = %+v", rx)
	}
}

func TestOverall(t *testing.T) {
	cases := []struct {
		name    string
		servers []Server
		want    Verdict
	}{
		{"none installed", nil, VerdictNotInstalled},
		{"connected", []Server{{Status: "✔ Connected"}}, VerdictConnected},
		{"needs auth", []Server{{Status: "! Needs authentication"}}, VerdictNeedsAuth},
		{"failed wins over connected", []Server{{Status: "✔ Connected"}, {Status: "✘ Failed to connect"}}, VerdictFailed},
		{"pending", []Server{{Status: "⏸ Pending approval"}}, VerdictPending},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Overall(tc.servers); got != tc.want {
				t.Errorf("Overall() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRiseXToolset(t *testing.T) {
	if !RiseXToolset([]string{"add_components", "get_active_ecosystem", "list_flows"}) {
		t.Error("the Rise-X tool list was not recognised")
	}
	if RiseXToolset([]string{"create_draft", "reply"}) || RiseXToolset(nil) {
		t.Error("a connector without the Rise-X tools passed for Rise-X")
	}
}

func TestReadConfig(t *testing.T) {
	dir := t.TempDir()
	body := `{
  "mcpServers": {
    "rise-x": {"type": "http", "url": "https://mcp.rise-x.io/mcp"},
    "rise-x-test": {"type": "http", "url": "https://mcp-test.rise-x.io/mcp"}
  }
}`
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	servers, err := ReadConfig(dir)
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("got %d servers, want 2: %+v", len(servers), servers)
	}
	if servers[0].Name != "rise-x" || servers[0].URL != "https://mcp.rise-x.io/mcp" {
		t.Errorf("servers[0] = %+v", servers[0])
	}
	if servers[1].Name != "rise-x-test" {
		t.Errorf("servers[1] = %+v", servers[1])
	}
}

// An unparseable status must not be reported as "needs authentication": the UI
// would tell the partner to sign in on the strength of a line it could not read.
func TestOverall_UnknownIsNotNeedsAuth(t *testing.T) {
	unknown := Overall([]Server{{Status: "something new"}})
	if unknown != VerdictUnknown {
		t.Fatalf("Overall(unknown) = %v, want %v", unknown, VerdictUnknown)
	}
	// A real needs_auth still wins over an unknown, and failed still wins overall.
	mixed := Overall([]Server{{Status: "something new"}, {Status: "! Needs authentication"}})
	if mixed != VerdictNeedsAuth {
		t.Fatalf("Overall(unknown+needs_auth) = %v, want %v", mixed, VerdictNeedsAuth)
	}
	worst := Overall([]Server{{Status: "! Needs authentication"}, {Status: "✘ Failed to connect"}})
	if worst != VerdictFailed {
		t.Fatalf("Overall(needs_auth+failed) = %v, want %v", worst, VerdictFailed)
	}
}
