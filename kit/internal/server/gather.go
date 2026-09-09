package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/catalog"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/claudecli"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/doctor"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/mcp"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/nodeinstall"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/npmrc"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/runner"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/semver"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/settings"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/synced"
)

const (
	probeTTL = 60 * time.Second
	// probeFailTTL is how long a probe that could not answer is remembered. A
	// wedged claude, a node that isn't installed yet: the partner fixes those
	// and presses Refresh, and must not be told the old answer for a minute.
	probeFailTTL = 10 * time.Second
)

// probeCache memoizes one probe of the machine: probeTTL for an answer,
// probeFailTTL for a failure. `claude mcp list` takes a couple of seconds per
// server, DetectNode shells out to node and npmrc.Analyze reads a file, so
// none of them should re-run for every /api/overview + /api/doctor pair one
// page load makes.
type probeCache[T any] struct {
	mu     sync.Mutex
	at     time.Time
	val    T
	set    bool
	failed bool
}

// get memoizes a probe whose answer is always usable.
func (c *probeCache[T]) get(probe func() T) T {
	return c.getOrFail(func() (T, bool) { return probe(), false })
}

// getOrFail memoizes a probe that can fail; failed is the probe's own verdict
// on whether its answer is usable.
func (c *probeCache[T]) getOrFail(probe func() (val T, failed bool)) T {
	return c.memo(func() (T, bool, bool) {
		val, failed := probe()
		return val, failed, true
	})
}

// getOrForget is getOrFail for a probe that can be interrupted: an answer
// whose error is context.Canceled is returned but never remembered, so a
// probe that was cut short cannot stand in for the real one for probeFailTTL.
func (c *probeCache[T]) getOrForget(probe func() (val T, err error)) T {
	return c.memo(func() (T, bool, bool) {
		val, err := probe()
		return val, err != nil, !errors.Is(err, context.Canceled)
	})
}

// peek returns a remembered answer without running the probe. An action
// handler uses it where the probe is a claude spawn: it must answer a click,
// so a cold cache means "no opinion" rather than a wait.
func (c *probeCache[T]) peek() (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ttl := probeTTL
	if c.failed {
		ttl = probeFailTTL
	}
	if !c.set || time.Since(c.at) >= ttl {
		var zero T
		return zero, false
	}
	return c.val, !c.failed
}

// memo is the shared body: keep says whether the answer may be remembered at
// all, failed whether it may only be remembered for probeFailTTL.
func (c *probeCache[T]) memo(probe func() (val T, failed, keep bool)) T {
	c.mu.Lock()
	defer c.mu.Unlock()
	ttl := probeTTL
	if c.failed {
		ttl = probeFailTTL
	}
	if c.set && time.Since(c.at) < ttl {
		return c.val
	}
	val, failed, keep := probe()
	if !keep {
		return val
	}
	c.val, c.failed = val, failed
	c.at, c.set = time.Now(), true
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
	set, err := settings.Read(s.settingsPath())
	if err != nil {
		facts.SettingsError = oneLine(err.Error())
	}
	// Filled in before anything that needs the CLI: a machine without one
	// still has a settings.json, and the page's auto-update switch must know
	// when it cannot be read.
	overview.Marketplace = &MarketplaceInfo{SettingsError: facts.SettingsError != ""}

	if cli, found := s.cli(ctx); found {
		overview.CLI = &CLIInfo{Found: true, Path: cli.Path, Version: cli.Version, Source: cli.Source}
		facts.CLIFound, facts.CLIVersion = true, cli.Version
		s.gatherClaude(ctx, claudecli.New(cli.Path, s.runner), set, &overview, &facts)
	} else {
		overview.CLI = &CLIInfo{Found: false}
		// Detecting stale addresses needs no CLI, so the connection card must
		// still show them when there is none to gather the rest.
		overview.Mcp = &McpInfo{Verdict: string(mcp.VerdictUnknown), Stale: s.staleMcp()}
	}

	node := s.nodeCache.getOrFail(func() (nodeProbe, bool) {
		v, ok := doctor.DetectNode(s.nodeEnv(s.runner))
		return nodeProbe{version: v, found: ok}, !ok
	})
	facts.NodeFound, facts.NodeVersion = node.found, node.version
	facts.CanInstallNode = nodeinstall.CanInstall(s.nodeInstallEnv(node.found))

	report := s.npmrcCache.getOrFail(func() (npmrc.Report, bool) {
		r, err := npmrc.Analyze(s.npmrcPath)
		if err != nil {
			return npmrc.Report{}, true
		}
		return r, false
	})
	facts.NpmrcOffendingLines, facts.NpmrcOffendingHost = report.Lines, report.Host

	if runtime.GOOS == "windows" {
		facts.GitFound = doctor.DetectGit(s.nodeEnv(s.runner))
	}

	facts.McpStale = s.staleMcp()
	facts.DisableAutoupdater, facts.ForceAutoupdatePlugins = autoupdaterEnv(set)

	overview.ReloadHint = s.getReloadHint()
	if id, action, ok := s.jobs.Running(); ok {
		overview.RunningJob = &RunningJob{ID: id, Action: action}
	}
	return overview, facts, nil
}

// gatherClaude fills in everything that needs the claude CLI.
func (s *Server) gatherClaude(ctx context.Context, client *claudecli.Client, set settings.Settings, overview *OverviewResponse, facts *doctor.Facts) {
	marketplaces, err := client.MarketplaceList(ctx)
	var mp *claudecli.Marketplace
	if err == nil {
		mp = findMarketplace(marketplaces, claudecli.MarketplaceName)
	} else {
		facts.MarketplaceListError = cliCheckError(err)
		overview.Marketplace.CheckError = facts.MarketplaceListError
	}
	facts.MarketplaceRegistered = mp != nil
	overview.Marketplace.Registered = mp != nil

	// Through the cache, so an action handler checking its target against
	// this same list a moment later is answered from the page load that just
	// paid for it rather than spawning claude again.
	plResult, plErr := s.pluginList(ctx)

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
	// The catalog is marketplace.json, not the CLI's "available" list: that
	// list leaves out whatever is installed, so a skill's row vanished the
	// moment its Install finished.
	names := s.marketplaceNames(ctx, installLocation)
	if len(names) == 0 && plErr == nil {
		names = publicPluginNames(plResult)
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
				// No CLI install record, so the CLI cannot update or remove
				// this copy: it is Claude Desktop's, whatever marketplace the
				// account picked it from.
				pi.InstallSource = doctor.SourceDesktop
				if sp.Org {
					pi.InstallSource = doctor.SourceOrganisation
				}
			}
		}

		if plErr != nil && !pi.Installed {
			// The list that would have said so could not be read, and no
			// synced manifest answered instead: not installed is a guess.
			pi.CheckError = cliCheckError(plErr)
		}

		localVersion, localDescription, localErr := s.catalog.LocalVersion(installLocation, name)

		v, err := s.catalog.RemoteVersion(ctx, name)
		switch {
		case err == nil:
			pi.PublicVersion = v
		case unreachable(err):
			if localErr == nil {
				pi.PublicVersion, pi.Offline = localVersion, true
			}
		default:
			pi.PublicCheckError = checkErrorReason(err)
		}

		// marketplace.json declares no descriptions for the Rise-X plugins, so
		// fall back to the local clone's own plugin.json.
		if pi.Description == "" && localErr == nil {
			pi.Description = localDescription
		}

		pi.UpdateAvailable = pi.LocalVersion != "" && pi.PublicVersion != "" &&
			semver.VersionLess(pi.LocalVersion, pi.PublicVersion)

		overview.Plugins = append(overview.Plugins, pi)
		facts.Plugins = append(facts.Plugins, pi.PluginFact)
	}
}

// publicPluginNames is every plugin the CLI lists from the public marketplace,
// installed or not, for when marketplace.json itself cannot be read.
func publicPluginNames(res claudecli.PluginListResult) []string {
	var names []string
	for _, a := range res.Available {
		if a.MarketplaceName == claudecli.MarketplaceName {
			names = append(names, a.Name)
		}
	}
	for _, ip := range res.Installed {
		id, market, ok := strings.Cut(ip.ID, "@")
		if ok && market == claudecli.MarketplaceName && !slices.Contains(names, id) {
			names = append(names, id)
		}
	}
	return names
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

// cliCheckError turns a failed read-only claude command into the cause the
// doctor and the page name. A deadline is the common one and says nothing
// worth quoting, so it gets a sentence of its own.
func cliCheckError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "Claude Code did not answer in time"
	}
	// This string is rendered in a doctor row, so it must not carry whatever
	// the CLI printed on stderr verbatim.
	return runner.Redact(oneLine(err.Error()))
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
// The marketplace half of the ID is not pinned: the same plugin can be
// installed from a mirror of the public one, and its servers are the same
// servers. Any read or parse failure reads as "no configured servers".
func riseXMcpConfig(installed []claudecli.InstalledPlugin) []mcp.ConfiguredServer {
	var fallback *claudecli.InstalledPlugin
	for i := range installed {
		ip := &installed[i]
		id, market, ok := strings.Cut(ip.ID, "@")
		if !ok || id != riseXMcpPlugin {
			continue
		}
		if market == claudecli.MarketplaceName {
			return readMcpConfig(ip.InstallPath)
		}
		if fallback == nil {
			fallback = ip
		}
	}
	if fallback == nil {
		return nil
	}
	return readMcpConfig(fallback.InstallPath)
}

func readMcpConfig(installPath string) []mcp.ConfiguredServer {
	cfg, err := mcp.ReadConfig(installPath)
	if err != nil {
		return nil
	}
	return cfg
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

	res := s.mcpCache.getOrForget(func() (mcpListResult, error) {
		raw, err := client.McpList(ctx)
		return mcpListResult{raw: raw, err: err}, err
	})
	if res.err == nil {
		info.Servers = mcp.RiseXServers(mcp.Parse(res.raw))
		info.Verdict = string(mcp.Overall(info.Servers))
		info.Raw = runner.Redact(res.raw)
	} else {
		// A check that could not run says nothing about whether the servers
		// are configured, so the verdict stays unknown - never "not
		// installed" on the strength of a timed-out or killed command.
		info.Message = McpCheckFailedMessage
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
			info.Message = "" // the Desktop app owns them; nothing failed
		}
	}
	// A connector added under Customize > Connectors lives in the account, in
	// no file `claude mcp list` reads, so the plugin's own copy of the
	// connection keeps asking for Claude Code's sign-in however many times the
	// partner signs in through the Desktop app. What the app handed the
	// latest Claude Code session answers instead; only a copy the CLI itself
	// has connected, or no copy at all, outranks that.
	switch mcp.Verdict(info.Verdict) {
	case mcp.VerdictConnected, mcp.VerdictNotInstalled:
	default:
		if names := s.desktopConnectors(); len(names) > 0 {
			info.Verdict, info.DesktopConnectors, info.Message = string(mcp.VerdictDesktop), names, ""
		}
	}
	overview.Mcp = info
}

// desktopConnectors names the Rise-X connectors the Desktop app gave the
// account's latest Claude Code session.
func (s *Server) desktopConnectors() []string {
	return s.connectorsCache.get(func() []string {
		connectors, _ := synced.Connectors(s.desktopDataDir)
		var names []string
		for _, c := range connectors {
			if mcp.RiseXToolset(c.Tools) {
				names = append(names, c.Name)
			}
		}
		return names
	})
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

// autoupdaterEnv reports whether settings.json's "env" block sets
// DISABLE_AUTOUPDATER/FORCE_AUTOUPDATE_PLUGINS. The kit's own process
// environment is deliberately not read: Claude Code's desktop shell exports
// DISABLE_AUTOUPDATER to what it launches, so the answer would depend on
// whether the partner started the kit from a session or from Finder.
func autoupdaterEnv(set settings.Settings) (disable, force bool) {
	env, err := set.Env()
	if err != nil {
		return false, false
	}
	return env["DISABLE_AUTOUPDATER"] != "", env["FORCE_AUTOUPDATE_PLUGINS"] != ""
}

// nodeInstalled reports the cached node probe's verdict, for the winget verb.
func (s *Server) nodeInstalled() bool {
	return s.nodeCache.getOrFail(func() (nodeProbe, bool) {
		v, ok := doctor.DetectNode(s.nodeEnv(s.runner))
		return nodeProbe{version: v, found: ok}, !ok
	}).found
}

func defaultNpmrcPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".npmrc")
}

// oneLine collapses an error message onto a single line, so a JSON syntax
// error's own newlines don't break the doctor row it's folded into.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
