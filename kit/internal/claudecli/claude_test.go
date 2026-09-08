package claudecli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner/runnertest"
)

const pluginListFixture = `{
  "installed": [
    {
      "id": "rise-x-mcp@rise-x-public",
      "version": "1.3.1",
      "scope": "user",
      "enabled": true,
      "installPath": "/Users/x/.claude/plugins/cache/rise-x-public/rise-x-mcp/1.3.1",
      "installedAt": "2026-05-15T19:26:54.529Z",
      "lastUpdated": "2026-07-24T09:20:22.462Z"
    }
  ],
  "available": [
    {
      "pluginId": "rise-x-mcp@rise-x-public",
      "name": "rise-x-mcp",
      "marketplaceName": "rise-x-public",
      "source": "./plugins/rise-x-mcp"
    },
    {
      "pluginId": "rise-x-apps@rise-x-public",
      "name": "rise-x-apps",
      "marketplaceName": "rise-x-public",
      "source": "./plugins/rise-x-apps"
    },
    {
      "pluginId": "agentforce-adlc@claude-plugins-official",
      "name": "agentforce-adlc",
      "marketplaceName": "claude-plugins-official",
      "source": { "source": "url", "url": "https://example.com/x.zip" }
    }
  ]
}`

const marketplaceListFixture = `[
  {
    "name": "rise-x-public",
    "source": "github",
    "repo": "rise-x/rise-x-ai-public-marketplace",
    "installLocation": "/Users/x/.claude/plugins/marketplaces/rise-x-public"
  },
  {
    "name": "rise-x",
    "source": "git",
    "url": "git@example:rise-x/rise-x-ai-marketplace.git",
    "installLocation": "/Users/x/.claude/plugins/marketplaces/rise-x"
  }
]`

const mcpListFixture = "Checking MCP server health…\n\n" +
	"plugin:rise-x-mcp:rise-x: https://mcp.rise-x.io/mcp (HTTP) - ! Needs authentication\n" +
	"plugin:context7:context7: npx -y @upstash/context7-mcp - ✔ Connected\n"

func TestClient_PluginListAvailable(t *testing.T) {
	f := runnertest.NewFake()
	f.Set("claude", []string{"plugin", "list", "--json", "--available"}, runnertest.Result{Stdout: pluginListFixture})
	c := New("claude", f)

	got, err := c.PluginListAvailable(context.Background())
	if err != nil {
		t.Fatalf("PluginListAvailable: %v", err)
	}
	if len(got.Installed) != 1 || got.Installed[0].ID != "rise-x-mcp@rise-x-public" {
		t.Fatalf("unexpected installed: %+v", got.Installed)
	}
	// The third entry's "source" is an object, not a string: a real CLI writes
	// it that way for any url-source marketplace, and it must not fail the
	// whole decode.
	if len(got.Available) != 3 || got.Available[1].Name != "rise-x-apps" {
		t.Fatalf("unexpected available: %+v", got.Available)
	}
	if got.Available[2].Name != "agentforce-adlc" {
		t.Fatalf("object-source entry did not decode: %+v", got.Available[2])
	}
}

func TestClient_Version_ShortensProductName(t *testing.T) {
	f := runnertest.NewFake()
	f.Set("claude", []string{"--version"}, runnertest.Result{Stdout: "2.1.258 (Claude Code)\n"})
	c := New("claude", f)

	got, err := c.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if got != "2.1.258" {
		t.Fatalf("Version = %q, want 2.1.258", got)
	}
}

func TestClient_MarketplaceList(t *testing.T) {
	f := runnertest.NewFake()
	f.Set("claude", []string{"plugin", "marketplace", "list", "--json"}, runnertest.Result{Stdout: marketplaceListFixture})
	c := New("claude", f)

	got, err := c.MarketplaceList(context.Background())
	if err != nil {
		t.Fatalf("MarketplaceList: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d marketplaces, want 2", len(got))
	}
	if got[0].Repo != MarketplaceRepo {
		t.Errorf("Repo = %q, want %q", got[0].Repo, MarketplaceRepo)
	}
	if got[1].URL == "" || got[1].Repo != "" {
		t.Errorf("git-source entry should carry URL not Repo: %+v", got[1])
	}
}

func TestClient_McpList_RawText(t *testing.T) {
	f := runnertest.NewFake()
	f.Set("claude", []string{"mcp", "list"}, runnertest.Result{Stdout: mcpListFixture})
	c := New("claude", f)

	got, err := c.McpList(context.Background())
	if err != nil {
		t.Fatalf("McpList: %v", err)
	}
	if !strings.Contains(got, "plugin:rise-x-mcp:rise-x") {
		t.Fatalf("expected raw text passthrough, got %q", got)
	}
}

// A `claude mcp list` the deadline killed must surface as an error, not as
// empty output: the page would otherwise report the partner has no MCP
// servers at all.
func TestClient_McpList_ContextDeadline_ReturnsError(t *testing.T) {
	f := runnertest.NewFake()
	f.Set("claude", []string{"mcp", "list"}, runnertest.Result{Stdout: mcpListFixture})
	f.SetDelay(5 * time.Second)
	c := New("claude", f)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	got, err := c.McpList(ctx)
	if err == nil {
		t.Fatalf("McpList err = nil, want a deadline error (raw %q)", got)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("McpList err = %v, want context.DeadlineExceeded", err)
	}
}

// The runner's own error - a timeout, a signalled process - must reach the
// caller with the stderr tail the runner collected.
func TestClient_McpList_RunnerError_QuotesStderr(t *testing.T) {
	f := runnertest.NewFake()
	f.Set("claude", []string{"mcp", "list"}, runnertest.Result{
		Stderr: "node: out of memory",
		Err:    context.DeadlineExceeded,
	})
	c := New("claude", f)

	_, err := c.McpList(context.Background())
	if err == nil {
		t.Fatal("McpList err = nil, want the runner's error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("McpList err = %v, want the wrapped runner error", err)
	}
	if !strings.Contains(err.Error(), "out of memory") {
		t.Fatalf("err = %q, want the stderr quoted", err)
	}
}

func TestClient_MarketplaceList_NonZeroExit(t *testing.T) {
	f := runnertest.NewFake()
	f.Set("claude", []string{"plugin", "marketplace", "list", "--json"}, runnertest.Result{Stderr: "boom", ExitCode: 1})
	c := New("claude", f)

	if _, err := c.MarketplaceList(context.Background()); err == nil {
		t.Fatal("expected error on non-zero exit")
	}
}

func TestClient_PluginInstall_ArgvAndLog(t *testing.T) {
	f := runnertest.NewFake()
	args := []string{"plugin", "install", "rise-x-apps@rise-x-public"}
	f.Set("claude", args, runnertest.Result{Stdout: "Installed rise-x-apps@1.5.0\n"})
	c := New("claude", f)

	var lines []string
	exitCode, err := c.PluginInstall(context.Background(), "rise-x-apps", func(s string) { lines = append(lines, s) })
	if err != nil {
		t.Fatalf("PluginInstall: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	if len(lines) < 2 {
		t.Fatalf("expected argv line + output, got %v", lines)
	}
	if lines[0] != "claude plugin install rise-x-apps@rise-x-public" {
		t.Errorf("first line should be argv, got %q", lines[0])
	}
	if lines[1] != "Installed rise-x-apps@1.5.0" {
		t.Errorf("second line = %q", lines[1])
	}
}

func TestClient_McpLogin_RedactsSecrets(t *testing.T) {
	f := runnertest.NewFake()
	args := []string{"mcp", "login", "plugin:rise-x-mcp:rise-x"}
	f.Set("claude", args, runnertest.Result{Stdout: "token=abc123secretvalue\n"})
	c := New("claude", f)

	var lines []string
	if _, err := c.McpLogin(context.Background(), "rise-x", func(s string) { lines = append(lines, s) }); err != nil {
		t.Fatalf("McpLogin: %v", err)
	}
	for _, l := range lines {
		if strings.Contains(l, "abc123secretvalue") {
			t.Fatalf("secret leaked into log: %q", l)
		}
	}
}
