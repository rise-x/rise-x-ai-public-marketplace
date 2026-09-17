package server

import (
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/doctor"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/mcp"
)

// OverviewResponse is GET /api/overview's shape.
type OverviewResponse struct {
	KitVersion string `json:"kitVersion"`
	// Kit is the kit's own update state; nil for a development build, which
	// never checks.
	Kit         *KitInfo         `json:"kit,omitempty"`
	CLI         *CLIInfo         `json:"cli"`
	Marketplace *MarketplaceInfo `json:"marketplace,omitempty"`
	Plugins     []PluginInfo     `json:"plugins,omitempty"`
	Mcp         *McpInfo         `json:"mcp,omitempty"`
	ReloadHint  bool             `json:"reloadHint"`
	// RunningJob is the job holding the jobs slot when the overview was read,
	// so a page that reloaded mid-install re-attaches to it instead of
	// showing an idle drawer and 409ing every button.
	RunningJob *RunningJob `json:"runningJob,omitempty"`
}

// RunningJob is enough for the page to resume polling one.
type RunningJob struct {
	ID     string `json:"id"`
	Action string `json:"action"`
}

// KitInfo is what the page needs to offer the kit's own update.
type KitInfo struct {
	// Latest is the newest release this build may move to, v-prefixed.
	Latest string `json:"latest,omitempty"`
	// LatestURL is that release's page, for the platforms the kit cannot
	// update itself on.
	LatestURL       string `json:"latestUrl,omitempty"`
	UpdateAvailable bool   `json:"updateAvailable"`
	// CanSelfUpdate is whether a release asset exists for this platform, so
	// the Update button can do the work rather than link out.
	CanSelfUpdate bool `json:"canSelfUpdate"`
	// CheckError is why GitHub could not be asked, in one line; Latest and
	// UpdateAvailable say nothing while it is set.
	CheckError string `json:"checkError,omitempty"`
}

type CLIInfo struct {
	Found   bool   `json:"found"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	// Source is where the binary was found: "path", "local-bin",
	// "desktop-bundle" or "windows-probe".
	Source string `json:"source,omitempty"`
}

type MarketplaceInfo struct {
	Registered      bool   `json:"registered"`
	InstallLocation string `json:"installLocation,omitempty"`
	// AutoUpdate is nil when the settings.json key isn't present at all.
	AutoUpdate *bool `json:"autoUpdate,omitempty"`
	// HeadStale is nil when GitHub was unreachable (skip, not a false "stale").
	HeadStale *bool `json:"headStale,omitempty"`
	// AutoUpdateMarketplace names the marketplace AutoUpdate was read for, so
	// the page's switch writes back to the same one.
	AutoUpdateMarketplace string `json:"autoUpdateMarketplace,omitempty"`
	// SettingsError is true when ~/.claude/settings.json could not be parsed,
	// so AutoUpdate could not be read and the switch must not be trusted.
	SettingsError bool `json:"settingsError,omitempty"`
	// CheckError is why `claude plugin marketplace list` could not be read.
	// Registered is meaningless while it is set: the answer is unknown, not
	// "no".
	CheckError string `json:"checkError,omitempty"`
}

// PluginInfo is one row of overview.plugins. It embeds the doctor's fact so
// gather fills each value once; the description is the page's alone.
type PluginInfo struct {
	doctor.PluginFact
	Description string `json:"description,omitempty"`
}

type McpInfo struct {
	Verdict    string                 `json:"verdict"`
	Servers    []mcp.Server           `json:"servers,omitempty"`
	Configured []mcp.ConfiguredServer `json:"configured,omitempty"`
	// Stale holds the connections pointing at an address Rise-X has moved
	// off, each with the address it should point at instead.
	Stale []mcp.Stale `json:"stale,omitempty"`
	// Connections are the Rise-X servers configured outside the plugin, in
	// ~/.claude.json or the Desktop app's config. They outlive a plugin
	// uninstall, so the page lists them whether or not the plugin is there.
	Connections []mcp.Connection `json:"connections,omitempty"`
	// DesktopConnectors names the Rise-X connectors the Claude Desktop app
	// gave the account's latest Claude Code session, when Verdict is
	// "desktop".
	DesktopConnectors []string `json:"desktopConnectors,omitempty"`
	// Message explains a verdict the page cannot read off the servers, such
	// as a `claude mcp list` that could not run at all.
	Message string `json:"message,omitempty"`
	// Raw is `claude mcp list`'s own output, passed through runner.Redact:
	// the page shows it verbatim, so a token in it must not reach the browser.
	Raw string `json:"raw,omitempty"`
}

// McpCheckFailedMessage is shown when `claude mcp list` could not be run or
// did not finish, so the connection state is genuinely unknown.
const McpCheckFailedMessage = "Could not check the connection right now."

// DoctorResponse is GET /api/doctor's shape.
type DoctorResponse struct {
	Checks []doctor.Check `json:"checks"`
}
