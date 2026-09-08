// Package doctor turns gathered facts about the machine into a partner-facing
// checklist with fix buttons; every Message is a plain sentence.
package doctor

import (
	"fmt"
	"strings"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/mcp"
	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/semver"
)

type Status string

const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// Check is one row of the doctor's checklist.
type Check struct {
	ID      string `json:"id"`
	Status  Status `json:"status"`
	Title   string `json:"title"`
	Message string `json:"message"`
	// Detail is the newline-separated lines behind the row's "Show lines"
	// disclosure: the exact entries the Fix would change.
	Detail  string         `json:"detail,omitempty"`
	Fix     string         `json:"fix,omitempty"`
	FixArgs map[string]any `json:"fixArgs,omitempty"`
}

// Where a plugin on this machine came from. An empty InstallSource means it
// is not installed at all.
const (
	SourcePublic       = "public"       // CLI-installed from rise-x-public
	SourceMarketplace  = "marketplace"  // installed from another marketplace
	SourceOrganisation = "organisation" // pushed to this account by the organisation
)

// PluginFact is what Run needs to know about one catalog plugin. It is
// embedded in the server's PluginInfo, so its tags are the wire names too.
type PluginFact struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Enabled   bool   `json:"enabled"`
	// InstallSource is one of the Source* values above. Only SourcePublic may
	// be installed, updated or removed from the public marketplace; doing that
	// to either of the others would leave the machine with two copies.
	InstallSource string `json:"installSource,omitempty"`
	// SourceName is the marketplace InstallSource came from, when it has a
	// name the partner would recognise.
	SourceName     string `json:"sourceName,omitempty"`
	LocalVersion   string `json:"localVersion,omitempty"`
	PublicVersion  string `json:"publicVersion,omitempty"`
	VersionUnknown bool   `json:"versionUnknown,omitempty"` // claude reported version:"unknown"
	// Offline is true when PublicVersion came from the local clone because
	// GitHub was unreachable; comparing against it would always say "up to date".
	Offline bool `json:"offline,omitempty"`
	// PublicCheckError is why the public version could not be read when the
	// repo did answer, e.g. "HTTP 404".
	PublicCheckError string `json:"publicCheckError,omitempty"`
	// UpdateAvailable is LocalVersion behind PublicVersion. The server fills
	// it so the page never compares versions itself.
	UpdateAvailable bool `json:"updateAvailable,omitempty"`
}

// Facts is everything Run needs; every field is gathered elsewhere so this
// package stays independently testable from fixtures, no claude/network/FS.
type Facts struct {
	GOOS string

	CLIFound   bool
	CLIVersion string

	MarketplaceRegistered bool

	AutoUpdatePresent bool
	AutoUpdateEnabled bool
	// AutoUpdateMarketplace is the marketplace the two flags above were read
	// for: rise-x-public, unless the skills come from a mirror of it.
	AutoUpdateMarketplace string

	Plugins []PluginFact

	HeadStale      bool // local marketplace clone HEAD != GitHub main
	HeadSkip       bool // GitHub was unreachable; don't report stale
	HeadLocalError bool // the local clone's .git/HEAD could not be read

	NodeFound   bool
	NodeVersion string // e.g. "20.11.0"

	NpmrcOffendingLines []string

	// McpStale holds the MCP connections pointing at an address Rise-X has
	// moved off.
	McpStale []mcp.Stale

	GitFound bool // Windows only

	DisableAutoupdater     bool
	ForceAutoupdatePlugins bool
}

// Run evaluates every check against f. Order matches the plan's doctor
// table; git.windows only appears when f.GOOS == "windows".
func Run(f Facts) []Check {
	needCLI := []Check{marketplaceRegisteredCheck(f), autoUpdateCheck(f)}
	for _, p := range f.Plugins {
		needCLI = append(needCLI, pluginCheck(p))
	}
	needCLI = append(needCLI, headCheck(f))
	if !f.CLIFound {
		// Nothing here can be judged without the CLI, and every fix it would
		// offer needs one too.
		for i := range needCLI {
			needCLI[i] = Check{ID: needCLI[i].ID, Status: StatusSkip,
				Title: needCLI[i].Title, Message: installCLIFirst}
		}
	}

	checks := append([]Check{cliCheck(f)}, needCLI...)
	checks = append(checks, mcpStaleCheck(f), nodeCheck(f), npmrcCheck(f))
	if f.GOOS == "windows" {
		checks = append(checks, gitWindowsCheck(f))
	}
	return append(checks, envAutoupdaterCheck(f))
}

const waitingForMarketplace = "Waiting until the Rise-X marketplace is registered."

const installCLIFirst = "Install Claude Code first."

func cliCheck(f Facts) Check {
	if f.CLIFound {
		return Check{ID: "cli", Status: StatusOK, Title: "Claude Code",
			Message: fmt.Sprintf("Found version %s.", f.CLIVersion)}
	}
	return Check{ID: "cli", Status: StatusFail, Title: "Claude Code",
		Message: "Not found. Install it to manage skills.", Fix: "cli.install"}
}

// shortVersion keeps the number and drops anything the binary appends after a
// space.
func shortVersion(v string) string {
	if first, _, ok := strings.Cut(strings.TrimSpace(v), " "); ok {
		return first
	}
	return strings.TrimSpace(v)
}

func marketplaceRegisteredCheck(f Facts) Check {
	if allFromOrg(f.Plugins) {
		return Check{ID: "marketplace.registered", Status: StatusOK, Title: "Rise-X marketplace",
			Message: "Skills come from your organisation."}
	}
	if f.MarketplaceRegistered {
		return Check{ID: "marketplace.registered", Status: StatusOK, Title: "Rise-X marketplace",
			Message: "Claude knows where to find Rise-X skills."}
	}
	return Check{ID: "marketplace.registered", Status: StatusFail, Title: "Rise-X marketplace",
		Message: "Not registered yet.", Fix: "marketplace.add"}
}

// autoUpdateCheck reports on extraKnownMarketplaces.<name>.autoUpdate. Whether
// Claude Code honours that key at user scope is unverified; the kit writes it
// because the plan says to, and reports exactly what is in the file.
func autoUpdateCheck(f Facts) Check {
	const id = "marketplace.autoupdate"
	if allFromOrg(f.Plugins) {
		return Check{ID: id, Status: StatusOK, Title: "Automatic updates",
			Message: "Your organisation delivers skill updates automatically."}
	}
	if !f.MarketplaceRegistered {
		return Check{ID: id, Status: StatusSkip, Title: "Automatic updates", Message: waitingForMarketplace}
	}
	from := ""
	if f.AutoUpdateMarketplace != "" && f.AutoUpdateMarketplace != defaultMarketplace {
		from = " from " + f.AutoUpdateMarketplace
	}
	if f.AutoUpdatePresent && f.AutoUpdateEnabled {
		return Check{ID: id, Status: StatusOK, Title: "Automatic updates",
			Message: fmt.Sprintf("Claude Code updates Rise-X skills%s after each session starts.", from)}
	}
	return Check{ID: id, Status: StatusWarn, Title: "Automatic updates",
		Message: fmt.Sprintf("Turned off%s, so new skill versions will not arrive on their own.", from),
		Fix:     "autoupdate.set",
		FixArgs: map[string]any{"enabled": true, "marketplace": f.AutoUpdateMarketplace}}
}

// defaultMarketplace is the name every message leaves unsaid; only a mirror of
// it is worth naming out loud.
const defaultMarketplace = "rise-x-public"

// allFromOrg reports whether every catalog plugin was delivered by the
// organisation, in which case nothing on this machine needs the public
// marketplace.
func allFromOrg(plugins []PluginFact) bool {
	if len(plugins) == 0 {
		return false
	}
	for _, p := range plugins {
		if p.InstallSource != SourceOrganisation {
			return false
		}
	}
	return true
}

// pluginTitle is the same for every skill row; the check ID names the skill.
const pluginTitle = "Skill versions"

func pluginCheck(p PluginFact) Check {
	id := "plugin." + p.Name
	if !p.Installed {
		return Check{ID: id, Status: StatusFail, Title: pluginTitle, Message: "Not installed.",
			Fix: "plugin.install", FixArgs: map[string]any{"name": p.Name}}
	}
	if !p.Enabled {
		return Check{ID: id, Status: StatusWarn, Title: pluginTitle,
			Message: "Installed but turned off in Claude Code settings."}
	}
	// A copy from elsewhere is only reported, never fixed from here: installing
	// or updating it from the public marketplace would leave two copies.
	switch p.InstallSource {
	case SourceOrganisation:
		return Check{ID: id, Status: StatusOK, Title: pluginTitle,
			Message: "Installed by your organisation" + versionSuffix(p.LocalVersion) + "."}
	case SourceMarketplace:
		return Check{ID: id, Status: StatusOK, Title: pluginTitle,
			Message: fmt.Sprintf("Installed from %s%s.", sourceName(p), versionSuffix(p.LocalVersion))}
	}
	if p.VersionUnknown || p.LocalVersion == "" {
		return Check{ID: id, Status: StatusWarn, Title: pluginTitle,
			Message: "Installed without a version stamp. Update to fix.",
			Fix:     "plugin.update", FixArgs: map[string]any{"name": p.Name}}
	}
	if p.Offline {
		return Check{ID: id, Status: StatusSkip, Title: pluginTitle,
			Message: "Could not reach GitHub to check the public version."}
	}
	if p.PublicCheckError != "" {
		return Check{ID: id, Status: StatusWarn, Title: pluginTitle,
			Message: fmt.Sprintf("Could not check the public version (%s).", p.PublicCheckError)}
	}
	if p.PublicVersion != "" && semver.VersionLess(p.LocalVersion, p.PublicVersion) {
		return Check{ID: id, Status: StatusWarn, Title: pluginTitle,
			Message: fmt.Sprintf("%s installed, %s available.", p.LocalVersion, p.PublicVersion),
			Fix:     "plugin.update", FixArgs: map[string]any{"name": p.Name}}
	}
	return Check{ID: id, Status: StatusOK, Title: pluginTitle,
		Message: fmt.Sprintf("Version %s, up to date.", p.LocalVersion)}
}

func versionSuffix(version string) string {
	if version == "" {
		return ""
	}
	return " (" + version + ")"
}

func sourceName(p PluginFact) string {
	if p.SourceName == "" {
		return "another marketplace"
	}
	return p.SourceName
}

// mcpStaleCheck reports connections still pointing at an address Rise-X has
// moved off. Detecting them needs no CLI, but changing one does, and the
// Desktop app's own connectors are beyond the CLI either way.
func mcpStaleCheck(f Facts) Check {
	const id = "mcp.stale"
	const title = "Rise-X addresses"
	if len(f.McpStale) == 0 {
		return Check{ID: id, Status: StatusOK, Title: title, Message: "No old Rise-X connections."}
	}
	lines := make([]string, len(f.McpStale))
	fixable := false
	for i, st := range f.McpStale {
		lines[i] = fmt.Sprintf("%s (%s): %s → %s", st.Name, st.Scope, st.URL, st.SuggestedURL)
		if st.Scope != mcp.ScopeDesktop {
			fixable = true
		}
	}
	subject := "connections point"
	if len(f.McpStale) == 1 {
		subject = "connection points"
	}
	check := Check{ID: id, Status: StatusWarn, Title: title,
		Message: fmt.Sprintf("%d %s at an old Rise-X address.", len(f.McpStale), subject),
		Detail:  strings.Join(lines, "\n")}
	if fixable && f.CLIFound {
		check.Fix = "mcp.fix"
	}
	return check
}

func headCheck(f Facts) Check {
	const id = "marketplace.head"
	if !f.MarketplaceRegistered {
		return Check{ID: id, Status: StatusSkip, Title: "Skill catalog", Message: waitingForMarketplace}
	}
	if f.HeadLocalError {
		return Check{ID: id, Status: StatusWarn, Title: "Skill catalog",
			Message: "The local copy of the skill catalog is unreadable.", Fix: "marketplace.update"}
	}
	if f.HeadSkip {
		return Check{ID: id, Status: StatusSkip, Title: "Skill catalog",
			Message: "Could not reach GitHub to check."}
	}
	if f.HeadStale {
		return Check{ID: id, Status: StatusWarn, Title: "Skill catalog",
			Message: "A newer catalog is available.", Fix: "marketplace.update"}
	}
	return Check{ID: id, Status: StatusOK, Title: "Skill catalog", Message: "Up to date."}
}

// minNodeMajor/minNodeMinor is the floor rise-x-apps builds on; 20 is what it
// prefers.
const (
	minNodeMajor      = 18
	minNodeMinor      = 19
	prefNodeMajor     = 20
	nodeTooOldMessage = "Version %s is too old. Rise-X Apps needs Node 18.19 or newer."
)

func nodeCheck(f Facts) Check {
	const id = "node"
	if !f.NodeFound {
		return Check{ID: id, Status: StatusWarn, Title: "Node.js",
			Message: "Not found. Needed only for Rise-X Apps."}
	}
	version := shortVersion(strings.TrimPrefix(strings.TrimSpace(f.NodeVersion), "v"))
	major, minor := parseNodeVersion(f.NodeVersion)
	switch {
	case major >= prefNodeMajor:
		return Check{ID: id, Status: StatusOK, Title: "Node.js", Message: fmt.Sprintf("Version %s.", version)}
	case major > minNodeMajor || (major == minNodeMajor && minor >= minNodeMinor):
		return Check{ID: id, Status: StatusWarn, Title: "Node.js",
			Message: fmt.Sprintf("Version %s. Rise-X Apps works best on Node 20 or newer.", version)}
	default:
		return Check{ID: id, Status: StatusWarn, Title: "Node.js",
			Message: fmt.Sprintf(nodeTooOldMessage, version)}
	}
}

func parseNodeVersion(v string) (major, minor int) {
	parts := semver.VersionParts(v)
	if len(parts) > 0 {
		major = parts[0]
	}
	if len(parts) > 1 {
		minor = parts[1]
	}
	return major, minor
}

func npmrcCheck(f Facts) Check {
	const id = "npmrc"
	n := len(f.NpmrcOffendingLines)
	if n == 0 {
		return Check{ID: id, Status: StatusOK, Title: "npm configuration",
			Message: "Nothing left over from the old private Rise-X registry."}
	}
	noun := "lines"
	if n == 1 {
		noun = "line"
	}
	return Check{ID: id, Status: StatusWarn, Title: "npm configuration",
		Message: fmt.Sprintf("%d leftover %s from the old private Rise-X registry in ~/.npmrc.", n, noun),
		Detail:  strings.Join(f.NpmrcOffendingLines, "\n"),
		Fix:     "npmrc.clean"}
}

func gitWindowsCheck(f Facts) Check {
	const id = "git.windows"
	if f.GitFound {
		return Check{ID: id, Status: StatusOK, Title: "Git for Windows", Message: "Found."}
	}
	return Check{ID: id, Status: StatusWarn, Title: "Git for Windows",
		Message: "Not found. Claude Code Desktop will offer to install it."}
}

func envAutoupdaterCheck(f Facts) Check {
	const id = "env.autoupdater"
	if f.DisableAutoupdater && !f.ForceAutoupdatePlugins {
		return Check{ID: id, Status: StatusWarn, Title: "Auto-update environment",
			Message: "DISABLE_AUTOUPDATER is set without FORCE_AUTOUPDATE_PLUGINS, so skills will not update on their own."}
	}
	return Check{ID: id, Status: StatusOK, Title: "Auto-update environment",
		Message: "Nothing is blocking automatic updates."}
}
