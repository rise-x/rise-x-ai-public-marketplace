package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/catalog"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/claudecli"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/doctor"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/mcp"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/npmrc"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/semver"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/settings"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/synced"
)

const probeTTL = 60 * time.Second

// probeCache memoizes one probe of the machine for probeTTL. `claude mcp
// list` takes a couple of seconds per server, DetectNode shells out to node
// and npmrc.Analyze reads a file, so none of them should re-run for every
// /api/overview + /api/doctor pair one page load makes.
type probeCache[T any] struct {
	mu  sync.Mutex
	at  time.Time
	val T
	set bool
}

func (c *probeCache[T]) get(probe func() T) T {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.set && time.Since(c.at) < probeTTL {
		return c.val
	}
	c.val, c.at, c.set = probe(), time.Now(), true
	return c.val
}

func (c *probeCache[T]) invalidate() {
	c.mu.Lock()
	c.set = false
	c.mu.Unlock()
}

type mcpListResult struct {
	raw string
	err error
}

type nodeProbe struct {
	version string
	found   bool
}

// gather collects everything /api/overview and /api/doctor need in one
// pass, so both handlers share the same (cached) claudecli/catalog/mcp
// calls instead of duplicating them.
func (s *Server) gather(ctx context.Context) (OverviewResponse, doctor.Facts, error) {
	overview := OverviewResponse{KitVersion: s.version}
	facts := doctor.Facts{GOOS: runtime.GOOS}

	// One read of settings.json per gather: both the autoUpdate flag and the
	// env block come out of it.
	set, _ := settings.Read(s.settingsPath())

	if cli, found := s.cli(ctx); found {
		overview.CLI = &CLIInfo{Found: true, Path: cli.Path, Version: cli.Version, Source: cli.Source}
		facts.CLIFound, facts.CLIVersion = true, cli.Version
		s.gatherClaude(ctx, claudecli.New(cli.Path, s.runner), set, &overview, &facts)
	} else {
		overview.CLI = &CLIInfo{Found: false}
	}

	node := s.nodeCache.get(func() nodeProbe {
		v, ok := doctor.DetectNode(s.nodeEnv(s.runner))
		return nodeProbe{version: v, found: ok}
	})
	facts.NodeFound, facts.NodeVersion = node.found, node.version

	facts.NpmrcOffendingLines = s.npmrcCache.get(func() []string {
		lines, err := npmrc.Analyze(npmrcPath())
		if err != nil {
			return nil
		}
		return lines
	})

	if runtime.GOOS == "windows" {
		facts.GitFound = doctor.DetectGit(s.nodeEnv(s.runner))
	}

	facts.McpStale = s.staleMcp()
	facts.DisableAutoupdater, facts.ForceAutoupdatePlugins = autoupdaterEnv(set)

	overview.ReloadHint = s.getReloadHint()
	return overview, facts, nil
}

// gatherClaude fills in everything that needs the claude CLI.
func (s *Server) gatherClaude(ctx context.Context, client *claudecli.Client, set settings.Settings, overview *OverviewResponse, facts *doctor.Facts) {
	marketplaces, err := client.MarketplaceList(ctx)
	var mp *claudecli.Marketplace
	if err == nil {
		mp = findMarketplace(marketplaces, claudecli.MarketplaceName)
	}
	facts.MarketplaceRegistered = mp != nil
	overview.Marketplace = &MarketplaceInfo{Registered: mp != nil}

	plResult, plErr := client.PluginListAvailable(ctx)

	// The skills are gathered whether or not the public marketplace is
	// registered: an organisation-managed machine gets its skills from the
	// account and may never register one.
	installLocation := ""
	if mp != nil {
		installLocation = mp.InstallLocation
		overview.Marketplace.InstallLocation = installLocation
	}
	s.gatherPlugins(ctx, installLocation, plResult, plErr, overview, facts)
	if mp != nil {
		s.gatherHead(ctx, mp, overview, facts)
	}

	// Which marketplace's autoUpdate flag matters depends on where the
	// installed skills came from, so this reads settings.json only once the
	// skills are known.
	name := autoUpdateMarketplace(overview.Plugins)
	enabled, present, _ := set.AutoUpdate(name)
	facts.AutoUpdatePresent = present
	facts.AutoUpdateEnabled = enabled
	facts.AutoUpdateMarketplace = name
	if present {
		overview.Marketplace.AutoUpdate = &enabled
	}
	if name != claudecli.MarketplaceName {
		overview.Marketplace.AutoUpdateMarketplace = name
	}

	s.gatherMcp(ctx, client, plResult, plErr, overview)
}

// autoUpdateMarketplace is the marketplace whose autoUpdate flag governs the
// installed skills: a mirror of the public one when that is where they came
// from, the public one otherwise.
func autoUpdateMarketplace(plugins []PluginInfo) string {
	for _, p := range plugins {
		if p.InstallSource == doctor.SourceMarketplace && p.SourceName != "" {
			return p.SourceName
		}
	}
	return claudecli.MarketplaceName
}

func (s *Server) gatherPlugins(ctx context.Context, installLocation string, plResult claudecli.PluginListResult, plErr error, overview *OverviewResponse, facts *doctor.Facts) {
	var names []string
	if plErr == nil {
		for _, a := range plResult.Available {
			if a.MarketplaceName == claudecli.MarketplaceName {
				names = append(names, a.Name)
			}
		}
	}
	if len(names) == 0 {
		names = s.marketplaceNames(ctx, installLocation)
	}
	s.setCatalogNames(names)

	syncedPlugins := s.synced()

	for _, name := range names {
		pi := PluginInfo{PluginFact: doctor.PluginFact{Name: name}}

		for _, a := range plResult.Available {
			if a.Name == name && a.MarketplaceName == claudecli.MarketplaceName && a.Description != "" {
				pi.Description = a.Description
			}
		}

		installed, market := findInstalled(plResult.Installed, name)
		switch {
		case installed != nil:
			pi.Installed = true
			pi.Enabled = installed.Enabled
			pi.InstallSource = doctor.SourcePublic
			if market != claudecli.MarketplaceName {
				pi.InstallSource, pi.SourceName = doctor.SourceMarketplace, market
			}
			if installed.Version == "" || installed.Version == "unknown" {
				pi.VersionUnknown = true
			} else {
				pi.LocalVersion = installed.Version
			}
		default:
			if sp, ok := pickSynced(syncedPlugins, name); ok {
				// Nothing turns a synced plugin off locally, and the manifest
				// carries no such flag, so it counts as enabled.
				pi.Installed, pi.Enabled = true, true
				pi.LocalVersion, pi.SourceName = sp.Version, sp.Source
				pi.InstallSource = doctor.SourceMarketplace
				if sp.Org {
					pi.InstallSource = doctor.SourceOrganisation
				}
			}
		}

		v, err := s.catalog.RemoteVersion(ctx, name)
		switch {
		case err == nil:
			pi.PublicVersion = v
		case unreachable(err):
			if lv, _, lerr := s.catalog.LocalVersion(installLocation, name); lerr == nil {
				pi.PublicVersion, pi.Offline = lv, true
			}
		default:
			pi.PublicCheckError = checkErrorReason(err)
		}

		// marketplace.json declares no descriptions for the Rise-X plugins, so
		// fall back to the local clone's own plugin.json.
		if pi.Description == "" {
			if _, ld, lerr := s.catalog.LocalVersion(installLocation, name); lerr == nil {
				pi.Description = ld
			}
		}

		pi.UpdateAvailable = pi.LocalVersion != "" && pi.PublicVersion != "" &&
			semver.VersionLess(pi.LocalVersion, pi.PublicVersion)

		overview.Plugins = append(overview.Plugins, pi)
		facts.Plugins = append(facts.Plugins, pi.PluginFact)
	}
}

// findInstalled picks the CLI-installed copy of name and the marketplace it
// came from: the public marketplace's copy first (user scope over project
// scope), then any other marketplace's, since installing the public one on top
// of that would leave two copies behind.
func findInstalled(list []claudecli.InstalledPlugin, name string) (*claudecli.InstalledPlugin, string) {
	var public, other *claudecli.InstalledPlugin
	otherMarket := ""
	for i := range list {
		ip := &list[i]
		id, market, ok := strings.Cut(ip.ID, "@")
		if !ok || id != name {
			continue
		}
		if market == claudecli.MarketplaceName {
			if public == nil || ip.Scope == "user" {
				public = ip
			}
			continue
		}
		if other == nil {
			other, otherMarket = ip, market
		}
	}
	if public != nil {
		return public, claudecli.MarketplaceName
	}
	return other, otherMarket
}

// pickSynced finds name among the account-synced plugins, preferring the
// organisation's copy: that is the one the partner cannot change, so it is the
// one the page must describe.
func pickSynced(plugins []synced.Plugin, name string) (synced.Plugin, bool) {
	var pick synced.Plugin
	found := false
	for _, p := range plugins {
		if p.Name != name {
			continue
		}
		if !found || (p.Org && !pick.Org) {
			pick, found = p, true
		}
	}
	return pick, found
}

// unreachable reports whether err means GitHub could not be reached at all, as
// opposed to answering with something unusable.
func unreachable(err error) bool {
	var netErr net.Error // also matches the *url.Error http.Client returns
	return errors.Is(err, catalog.ErrRateLimited) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.As(err, &netErr)
}

// checkErrorReason turns a catalog failure into the short phrase the doctor
// shows in parentheses.
func checkErrorReason(err error) string {
	var statusErr *catalog.StatusError
	if errors.As(err, &statusErr) {
		return fmt.Sprintf("HTTP %d", statusErr.Code)
	}
	return "unreadable answer"
}

func findMarketplace(list []claudecli.Marketplace, name string) *claudecli.Marketplace {
	for i := range list {
		if list[i].Name == name {
			return &list[i]
		}
	}
	return nil
}

// riseXMcpPlugin is the catalog plugin that carries the MCP server
// definitions.
const riseXMcpPlugin = "rise-x-mcp"

// riseXMcpConfig reads the installed rise-x-mcp plugin's bundled .mcp.json.
// Any read or parse failure reads as "no configured servers".
func riseXMcpConfig(installed []claudecli.InstalledPlugin) []mcp.ConfiguredServer {
	for _, ip := range installed {
		if ip.ID != riseXMcpPlugin+"@"+claudecli.MarketplaceName {
			continue
		}
		cfg, err := mcp.ReadConfig(ip.InstallPath)
		if err != nil {
			return nil
		}
		return cfg
	}
	return nil
}

func (s *Server) gatherHead(ctx context.Context, mp *claudecli.Marketplace, overview *OverviewResponse, facts *doctor.Facts) {
	localHead, lerr := catalog.LocalHEAD(mp.InstallLocation)
	if lerr != nil {
		facts.HeadLocalError = true
		return // nothing to compare a remote SHA against
	}
	remoteHead, rerr := s.catalog.RemoteHEAD(ctx)
	if rerr != nil { // includes catalog.ErrRateLimited: skip, never "stale"
		facts.HeadSkip = true
		return
	}
	stale := localHead != remoteHead
	facts.HeadStale = stale
	overview.Marketplace.HeadStale = &stale
}

func (s *Server) gatherMcp(ctx context.Context, client *claudecli.Client, plResult claudecli.PluginListResult, plErr error, overview *OverviewResponse) {
	info := &McpInfo{Verdict: string(mcp.VerdictUnknown), Stale: s.staleMcp()}

	res := s.mcpCache.get(func() mcpListResult {
		raw, err := client.McpList(ctx)
		return mcpListResult{raw: raw, err: err}
	})
	if res.err == nil {
		info.Servers = mcp.RiseXServers(mcp.Parse(res.raw))
		info.Verdict = string(mcp.Overall(info.Servers))
		info.Raw = res.raw
	}

	if plErr == nil {
		info.Configured = riseXMcpConfig(plResult.Installed)
	}
	// A synced plugin is invisible to `claude mcp list`, so its own .mcp.json
	// is the only place its server URLs come from.
	if len(info.Configured) == 0 {
		if configured := syncedMcpConfig(s.synced()); len(configured) > 0 {
			info.Configured = configured
			info.Verdict = string(mcp.VerdictManaged)
		}
	}
	overview.Mcp = info
}

// syncedMcpConfig reads the .mcp.json bundled with the account-synced
// rise-x-mcp plugin, preferring the organisation's copy.
func syncedMcpConfig(plugins []synced.Plugin) []mcp.ConfiguredServer {
	p, ok := pickSynced(plugins, riseXMcpPlugin)
	if !ok {
		return nil
	}
	cfg, err := mcp.ReadConfigFile(synced.MCPConfigPath(p))
	if err != nil {
		return nil
	}
	return cfg
}

// autoupdaterEnv reports whether DISABLE_AUTOUPDATER/FORCE_AUTOUPDATE_PLUGINS
// are set, via the process environment or settings.json's "env" block.
func autoupdaterEnv(set settings.Settings) (disable, force bool) {
	disable = os.Getenv("DISABLE_AUTOUPDATER") != ""
	force = os.Getenv("FORCE_AUTOUPDATE_PLUGINS") != ""
	env, err := set.Env()
	if err != nil {
		return disable, force
	}
	if env["DISABLE_AUTOUPDATER"] != "" {
		disable = true
	}
	if env["FORCE_AUTOUPDATE_PLUGINS"] != "" {
		force = true
	}
	return disable, force
}

func npmrcPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".npmrc")
}
