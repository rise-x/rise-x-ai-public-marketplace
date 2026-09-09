package claudecli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/semver"
)

// defaultReadTimeout bounds the read-only --json commands, which should
// return almost instantly; it exists so a wedged claude process can't hang a
// request forever.
const defaultReadTimeout = 15 * time.Second

// Client runs claude CLI subcommands through a Runner.
type Client struct {
	Path   string
	Runner runner.Runner
}

func New(path string, r runner.Runner) *Client {
	return &Client{Path: path, Runner: r}
}

func (c *Client) run(ctx context.Context, args []string) (stdout string, err error) {
	ctx, cancel := context.WithTimeout(ctx, defaultReadTimeout)
	defer cancel()
	stdout, stderr, exitCode, err := c.Runner.Run(ctx, c.Path, args)
	if err != nil {
		return "", cmdError(runner.Argv(c.Path, args), err, stderr)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("%s: exit %d: %s", runner.Argv(c.Path, args), exitCode, runner.Redact(strings.TrimSpace(stderr)))
	}
	return stdout, nil
}

// cmdError wraps a command failure, quoting the stderr tail when there is
// one so a timeout says why rather than just "deadline exceeded".
func cmdError(argv string, err error, stderr string) error {
	if s := runner.Redact(strings.TrimSpace(stderr)); s != "" {
		return fmt.Errorf("%s: %w: %s", argv, err, s)
	}
	return fmt.Errorf("%s: %w", argv, err)
}

// Version runs `claude --version` and returns just the number.
func (c *Client) Version(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stdout, err := c.run(ctx, []string{"--version"})
	if err != nil {
		return "", err
	}
	return ShortVersion(stdout), nil
}

// ShortVersion keeps the number and drops the product name the CLI appends,
// so "2.1.258 (Claude Code)" reads as "2.1.258".
func ShortVersion(v string) string { return semver.Short(v) }

// MarketplaceList runs `claude plugin marketplace list --json`.
func (c *Client) MarketplaceList(ctx context.Context) ([]Marketplace, error) {
	stdout, err := c.run(ctx, []string{"plugin", "marketplace", "list", "--json"})
	if err != nil {
		return nil, err
	}
	var out []Marketplace
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		return nil, fmt.Errorf("parse marketplace list: %w", err)
	}
	return out, nil
}

// PluginListAvailable runs `claude plugin list --json --available`. Only the
// --available form is wrapped: without the flag the CLI returns a bare array
// of installed plugins, a different shape that no caller here wants.
func (c *Client) PluginListAvailable(ctx context.Context) (PluginListResult, error) {
	args := []string{"plugin", "list", "--json", "--available"}
	stdout, err := c.run(ctx, args)
	if err != nil {
		return PluginListResult{}, err
	}
	var out PluginListResult
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		return PluginListResult{}, fmt.Errorf("parse plugin list: %w", err)
	}
	return out, nil
}

// McpList runs `claude mcp list` and returns its raw text; callers parse it
// with the mcp package. The command can exit non-zero while still printing a
// useful per-server status list, so a non-zero exit is not treated as an
// error here. A timeout or a signalled process is: empty output must not read
// as "this partner has no MCP servers".
func (c *Client) McpList(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultReadTimeout)
	defer cancel()
	args := []string{"mcp", "list"}
	stdout, stderr, _, err := c.Runner.Run(ctx, c.Path, args)
	if err != nil {
		return "", cmdError(runner.Argv(c.Path, args), err, stderr)
	}
	return stdout, nil
}

// stream runs a mutating subcommand, feeding onLine the (redacted) argv line
// followed by each (redacted) output line as it arrives.
func (c *Client) stream(ctx context.Context, args []string, onLine func(string)) (int, error) {
	return c.streamIn(ctx, "", args, onLine)
}

// streamIn is stream with a working directory, for the subcommands that write
// to whichever project they run in.
func (c *Client) streamIn(ctx context.Context, dir string, args []string, onLine func(string)) (int, error) {
	if onLine != nil {
		onLine(runner.Redact(runner.Argv(c.Path, args)))
	}
	return c.Runner.StreamDir(ctx, dir, c.Path, args, func(l runner.Line) {
		if onLine != nil {
			onLine(runner.Redact(l.Text))
		}
	})
}

// MarketplaceAdd runs `claude plugin marketplace add <repo>`.
func (c *Client) MarketplaceAdd(ctx context.Context, onLine func(string)) (int, error) {
	return c.stream(ctx, []string{"plugin", "marketplace", "add", MarketplaceRepo}, onLine)
}

// MarketplaceUpdate runs `claude plugin marketplace update <name>`.
func (c *Client) MarketplaceUpdate(ctx context.Context, onLine func(string)) (int, error) {
	return c.stream(ctx, []string{"plugin", "marketplace", "update", MarketplaceName}, onLine)
}

// PluginInstall runs `claude plugin install <name>@<marketplace>`.
func (c *Client) PluginInstall(ctx context.Context, name string, onLine func(string)) (int, error) {
	return c.stream(ctx, []string{"plugin", "install", name + "@" + MarketplaceName}, onLine)
}

// PluginUpdate runs `claude plugin update <name>@<marketplace>`. The
// marketplace is a parameter because a plugin can be installed from a mirror
// of the public one, and updating it from the wrong marketplace installs a
// duplicate.
func (c *Client) PluginUpdate(ctx context.Context, name, marketplace string, onLine func(string)) (int, error) {
	return c.stream(ctx, []string{"plugin", "update", name + "@" + marketplace}, onLine)
}

// PluginUninstall runs `claude plugin uninstall <name>@<marketplace>`.
func (c *Client) PluginUninstall(ctx context.Context, name string, onLine func(string)) (int, error) {
	return c.stream(ctx, []string{"plugin", "uninstall", name + "@" + MarketplaceName}, onLine)
}

// McpRemove runs `claude mcp remove <name> -s <scope>` in dir.
func (c *Client) McpRemove(ctx context.Context, dir, name, scope string, onLine func(string)) (int, error) {
	return c.streamIn(ctx, dir, []string{"mcp", "remove", name, "-s", scope}, onLine)
}

// McpAdd runs `claude mcp add --transport http <name> <url> -s <scope>` in dir.
func (c *Client) McpAdd(ctx context.Context, dir, name, url, scope string, onLine func(string)) (int, error) {
	return c.streamIn(ctx, dir, []string{"mcp", "add", "--transport", "http", name, url, "-s", scope}, onLine)
}
