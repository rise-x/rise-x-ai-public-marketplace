// Package server wires the other internal packages into the kit's HTTP API:
// GET /api/overview, GET /api/doctor, POST /api/actions/{name},
// GET /api/jobs/{id}, and the embedded browser page.
package server

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/catalog"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/claudecli"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/doctor"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/jobs"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/mcp"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/settings"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/synced"
)

// gatherTTL is how long one gather's result serves later requests. The page
// loads /api/overview and /api/doctor together, and every gather spawns
// several claude processes.
const gatherTTL = 3 * time.Second

// Config configures a new Server.
type Config struct {
	Port      int // for the Host-header check; the listener itself is main's job
	Token     string
	ClaudeDir string // defaults to "~/.claude"
	Version   string
	Runner    runner.Runner // nil defaults to runner.Exec{}

	// LocateEnv overrides how the claude CLI is searched for; nil defaults
	// to claudecli.RealEnv. Tests use this to point Locate at a fake CLI
	// instead of the real filesystem/PATH.
	LocateEnv func(runner.Runner) claudecli.Env

	// Catalog overrides the GitHub-backed catalog; nil defaults to
	// catalog.New(claudecli.MarketplaceRepo). Tests use this to avoid real
	// network calls.
	Catalog *catalog.Catalog

	// NodeEnv overrides Node.js/Git discovery; nil defaults to
	// doctor.RealEnv. Tests use this to avoid probing the real machine.
	NodeEnv func(runner.Runner) doctor.Env

	// DesktopDataDir is the Claude Desktop app's data directory, holding the
	// plugins an account syncs and the app's own MCP config; empty defaults to
	// synced.DefaultDataDir.
	DesktopDataDir string

	// ClaudeJSONPath is the CLI's own config, scanned for MCP servers; empty
	// defaults to ~/.claude.json.
	ClaudeJSONPath string

	// NpmrcPath is the npm config the doctor reads and npmrc.clean edits;
	// empty defaults to ~/.npmrc.
	NpmrcPath string
}

// Server holds everything the HTTP handlers need.
type Server struct {
	port      int
	token     string
	claudeDir string
	version   string

	runner         runner.Runner
	locateEnv      func(runner.Runner) claudecli.Env
	nodeEnv        func(runner.Runner) doctor.Env
	catalog        *catalog.Catalog
	writer         *settings.Writer
	jobs           *jobs.Store
	desktopDataDir string
	claudeJSONPath string
	npmrcPath      string

	mcpCache    probeCache[mcpListResult]
	nodeCache   probeCache[nodeProbe]
	npmrcCache  probeCache[[]string]
	syncedCache probeCache[[]synced.Plugin]
	// staleCache holds the MCP scan: ~/.claude.json carries every project the
	// partner has ever opened, so it is not a file to re-read per request.
	staleCache probeCache[[]mcp.Stale]
	// installLocCache holds the marketplace clone's path, so validating a
	// plugin name in an action handler costs at most one claude spawn a
	// minute.
	installLocCache probeCache[string]

	// gatherMu serializes gather itself, so two concurrent requests share one
	// pass instead of each spawning its own claude processes.
	gatherMu    sync.Mutex
	gatherAt    time.Time
	gatherOK    bool
	gatherOv    OverviewResponse
	gatherFacts doctor.Facts

	mu            sync.Mutex
	cachedCLI     *claudecli.CLI
	reloadHint    bool
	catalogNames  []string
	quitRequested chan struct{}
	quitOnce      sync.Once
}

func New(cfg Config) *Server {
	r := cfg.Runner
	if r == nil {
		r = runner.Exec{}
	}
	locateEnv := cfg.LocateEnv
	if locateEnv == nil {
		locateEnv = claudecli.RealEnv
	}
	cat := cfg.Catalog
	if cat == nil {
		cat = catalog.New(claudecli.MarketplaceRepo)
	}
	nodeEnv := cfg.NodeEnv
	if nodeEnv == nil {
		nodeEnv = doctor.RealEnv
	}
	desktopDataDir := cfg.DesktopDataDir
	if desktopDataDir == "" {
		desktopDataDir = synced.DefaultDataDir()
	}
	claudeJSONPath := cfg.ClaudeJSONPath
	if claudeJSONPath == "" {
		claudeJSONPath = defaultClaudeJSONPath()
	}
	npmrcPath := cfg.NpmrcPath
	if npmrcPath == "" {
		npmrcPath = defaultNpmrcPath()
	}
	s := &Server{
		port:           cfg.Port,
		token:          cfg.Token,
		claudeDir:      cfg.ClaudeDir,
		version:        cfg.Version,
		runner:         r,
		locateEnv:      locateEnv,
		nodeEnv:        nodeEnv,
		catalog:        cat,
		jobs:           jobs.NewStore(),
		desktopDataDir: desktopDataDir,
		claudeJSONPath: claudeJSONPath,
		npmrcPath:      npmrcPath,
		quitRequested:  make(chan struct{}),
	}
	s.writer = settings.NewWriter(s.settingsPath())
	// Locate the CLI once here, so the page's first two requests don't both
	// pay for the filesystem search and a `claude --version`.
	_, _ = s.relocate(context.Background())
	return s
}

// cachedGather runs gather under gatherMu, reusing a result younger than
// gatherTTL. Every action that changes what gather would report invalidates it.
func (s *Server) cachedGather(ctx context.Context) (OverviewResponse, doctor.Facts, error) {
	s.gatherMu.Lock()
	defer s.gatherMu.Unlock()
	if s.gatherOK && time.Since(s.gatherAt) < gatherTTL {
		return s.gatherOv, s.gatherFacts, nil
	}
	overview, facts, err := s.gather(ctx)
	if err != nil {
		return overview, facts, err
	}
	s.gatherOv, s.gatherFacts, s.gatherAt, s.gatherOK = overview, facts, time.Now(), true
	return overview, facts, nil
}

func (s *Server) invalidateGather() {
	s.gatherMu.Lock()
	s.gatherOK = false
	s.gatherMu.Unlock()
}

// invalidateClaudeProbes drops every cache whose answer a claude CLI job can
// change. The node and npmrc probes read the machine rather than Claude Code,
// so they are not in it.
func (s *Server) invalidateClaudeProbes() {
	s.mcpCache.invalidate()
	s.syncedCache.invalidate()
	s.staleCache.invalidate()
	s.installLocCache.invalidate()
	s.invalidateGather()
}

// Quit is closed when the "quit" action runs, so main can shut the process
// down.
func (s *Server) Quit() <-chan struct{} { return s.quitRequested }

// Jobs is the store main cancels on shutdown: a claude child runs in its own
// process group, so nothing else stops it.
func (s *Server) Jobs() *jobs.Store { return s.jobs }

func (s *Server) settingsPath() string { return filepath.Join(s.claudeDir, "settings.json") }

// desktopConfigPath is the Claude Desktop app's own MCP config. The kit only
// reads it: the CLI cannot change what the app configured.
func (s *Server) desktopConfigPath() string {
	return filepath.Join(s.desktopDataDir, "claude_desktop_config.json")
}

func defaultClaudeJSONPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude.json")
}

// synced reads the plugins the Desktop app materialised from the account, from
// both layouts the docs describe.
func (s *Server) synced() []synced.Plugin {
	return s.syncedCache.get(func() []synced.Plugin {
		plugins, _ := synced.Read(s.desktopDataDir)
		fromClaudeDir, _ := synced.ReadClaudeDir(s.claudeDir)
		return append(plugins, fromClaudeDir...)
	})
}

// staleMcp scans the CLI's and the Desktop app's configs for connections
// pointing at an address Rise-X has moved off.
func (s *Server) staleMcp() []mcp.Stale {
	return s.staleCache.get(func() []mcp.Stale {
		return mcp.Scan(s.claudeJSONPath, s.desktopConfigPath())
	})
}

// relocate re-runs claudecli.Locate and updates the cached CLI. Called on
// startup, and after "Install CLI" / "Rescan" per the plan.
func (s *Server) relocate(ctx context.Context) (*claudecli.CLI, error) {
	cli, err := claudecli.Locate(ctx, s.locateEnv(s.runner))
	s.mu.Lock()
	if err == nil {
		s.cachedCLI = cli
	} else {
		s.cachedCLI = nil
	}
	s.mu.Unlock()
	return cli, err
}

// cli returns the located claude binary, searching for it on first use (an
// action can fire before the page's first /api/overview request). ok is false
// when no claude binary was found.
func (s *Server) cli(ctx context.Context) (*claudecli.CLI, bool) {
	s.mu.Lock()
	cached := s.cachedCLI
	s.mu.Unlock()
	if cached != nil {
		return cached, true
	}
	located, _ := s.relocate(ctx)
	return located, located != nil
}

// client wraps the located CLI for running subcommands.
func (s *Server) client(ctx context.Context) (*claudecli.Client, bool) {
	cli, ok := s.cli(ctx)
	if !ok {
		return nil, false
	}
	return claudecli.New(cli.Path, s.runner), true
}

func (s *Server) setReloadHint() {
	s.mu.Lock()
	s.reloadHint = true
	s.mu.Unlock()
}

func (s *Server) getReloadHint() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reloadHint
}

// clearReloadHint drops the "restart Claude Code" hint once the page has
// dismissed it, so the next /api/overview doesn't bring it back.
func (s *Server) clearReloadHint() {
	s.mu.Lock()
	s.reloadHint = false
	s.mu.Unlock()
}

func (s *Server) setCatalogNames(names []string) {
	s.mu.Lock()
	s.catalogNames = names
	s.mu.Unlock()
}

func (s *Server) getCatalogNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.catalogNames
}

// remoteCatalogTimeout bounds the GitHub fallback below: an action handler is
// answering a click, so it must not wait on a slow network.
const remoteCatalogTimeout = 5 * time.Second

// isKnownPlugin reports whether name is a plugin this marketplace ships. It
// never runs a gather - an action handler must not wait on one - so it takes
// the CLI's own "available" list when a gather has already filled it, then
// the local marketplace clone, then the public catalog.
func (s *Server) isKnownPlugin(ctx context.Context, name string) bool {
	if name == "" {
		return false
	}
	names := s.getCatalogNames()
	if len(names) == 0 {
		names = s.marketplaceNames(ctx, s.installLocation(ctx))
	}
	return slices.Contains(names, name)
}

// marketplaceNames reads the plugin names straight from marketplace.json: the
// local clone at installLocation when there is one, otherwise the public copy
// on GitHub.
func (s *Server) marketplaceNames(ctx context.Context, installLocation string) []string {
	if installLocation != "" {
		if names, err := catalog.LocalCatalogNames(installLocation); err == nil {
			return names
		}
	}
	ctx, cancel := context.WithTimeout(ctx, remoteCatalogTimeout)
	defer cancel()
	names, _ := s.catalog.CatalogNames(ctx) // already cached for 5 minutes
	return names
}

// installLocation returns the local marketplace clone's path per `claude
// plugin marketplace list --json`, or "" when there is none.
func (s *Server) installLocation(ctx context.Context) string {
	return s.installLocCache.get(func() string {
		client, ok := s.client(ctx)
		if !ok {
			return ""
		}
		list, err := client.MarketplaceList(ctx)
		if err != nil {
			return ""
		}
		if mp := findMarketplace(list, claudecli.MarketplaceName); mp != nil {
			return mp.InstallLocation
		}
		return ""
	})
}
