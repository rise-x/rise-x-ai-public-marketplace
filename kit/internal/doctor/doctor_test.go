package doctor

import (
	"strings"
	"testing"

	"github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/mcp"
)

func findCheck(t *testing.T, checks []Check, id string) Check {
	t.Helper()
	for _, c := range checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no check with id %q in %+v", id, checks)
	return Check{}
}

func baseFacts() Facts {
	return Facts{
		GOOS:                  "darwin",
		CLIFound:              true,
		CLIVersion:            "2.1.258",
		MarketplaceRegistered: true,
		AutoUpdatePresent:     true,
		AutoUpdateEnabled:     true,
		Plugins: []PluginFact{
			{Name: "rise-x-mcp", Installed: true, Enabled: true, LocalVersion: "1.3.1", PublicVersion: "1.3.1"},
			{Name: "rise-x-apps", Installed: true, Enabled: true, LocalVersion: "1.5.0", PublicVersion: "1.5.0"},
		},
		NodeFound:   true,
		NodeVersion: "20.11.0",
	}
}

func TestRun_AllHealthy(t *testing.T) {
	checks := Run(baseFacts())
	for _, c := range checks {
		if c.Status == StatusFail {
			t.Errorf("unexpected fail: %+v", c)
		}
		if c.Status == StatusWarn {
			t.Errorf("unexpected warn: %+v", c)
		}
	}
	// git.windows should not appear on darwin.
	for _, c := range checks {
		if c.ID == "git.windows" {
			t.Errorf("git.windows should not appear on darwin")
		}
	}
}

func TestRun_CLIMissing(t *testing.T) {
	f := baseFacts()
	f.CLIFound = false
	c := findCheck(t, Run(f), "cli")
	if c.Status != StatusFail || c.Fix != "cli.install" {
		t.Fatalf("cli check = %+v", c)
	}
}

func TestRun_MarketplaceNotRegistered_SkipsDownstream(t *testing.T) {
	f := baseFacts()
	f.MarketplaceRegistered = false
	checks := Run(f)

	if c := findCheck(t, checks, "marketplace.registered"); c.Status != StatusFail || c.Fix != "marketplace.add" {
		t.Fatalf("marketplace.registered = %+v", c)
	}
	if c := findCheck(t, checks, "marketplace.autoupdate"); c.Status != StatusSkip {
		t.Fatalf("marketplace.autoupdate = %+v", c)
	}
	if c := findCheck(t, checks, "marketplace.head"); c.Status != StatusSkip {
		t.Fatalf("marketplace.head = %+v", c)
	}
}

func TestRun_AutoUpdateMissing(t *testing.T) {
	f := baseFacts()
	f.AutoUpdatePresent = false
	f.AutoUpdateEnabled = false
	c := findCheck(t, Run(f), "marketplace.autoupdate")
	if c.Status != StatusWarn || c.Fix != "autoupdate.set" {
		t.Fatalf("autoupdate check = %+v", c)
	}
}

// An unreadable settings.json can't tell present from absent, so the row must
// warn about the file rather than claim automatic updates are off.
func TestRun_AutoUpdate_SettingsError(t *testing.T) {
	f := baseFacts()
	f.AutoUpdatePresent, f.AutoUpdateEnabled = false, false
	f.SettingsError = "invalid character 'n' looking for beginning of object key string"
	c := findCheck(t, Run(f), "marketplace.autoupdate")
	if c.Status != StatusWarn || c.Fix != "" {
		t.Fatalf("autoupdate check = %+v", c)
	}
	if c.Message != SettingsUnreadableMessage {
		t.Fatalf("message = %q", c.Message)
	}
}

func TestRun_PluginStates(t *testing.T) {
	cases := []struct {
		name       string
		plugin     PluginFact
		wantStatus Status
		wantFix    string
	}{
		{"not installed", PluginFact{Name: "rise-x-apps"}, StatusFail, "plugin.install"},
		{"disabled", PluginFact{Name: "rise-x-apps", Installed: true, Enabled: false}, StatusWarn, ""},
		{"version unknown", PluginFact{Name: "rise-x-apps", Installed: true, Enabled: true, VersionUnknown: true}, StatusWarn, "plugin.update"},
		{"behind", PluginFact{Name: "rise-x-apps", Installed: true, Enabled: true, LocalVersion: "1.4.0", PublicVersion: "1.5.0"}, StatusWarn, "plugin.update"},
		{"current", PluginFact{Name: "rise-x-apps", Installed: true, Enabled: true, LocalVersion: "1.5.0", PublicVersion: "1.5.0"}, StatusOK, ""},
		{"public version unknown: no false update", PluginFact{Name: "rise-x-apps", Installed: true, Enabled: true, LocalVersion: "1.5.0", PublicVersion: ""}, StatusOK, ""},
		// Numeric, not lexical: 1.10.0 is ahead of 1.9.0, and a local build
		// newer than the public one is not "behind".
		{"local ahead", PluginFact{Name: "rise-x-apps", Installed: true, Enabled: true, LocalVersion: "1.10.0", PublicVersion: "1.9.0"}, StatusOK, ""},
		{"same version, fewer segments", PluginFact{Name: "rise-x-apps", Installed: true, Enabled: true, LocalVersion: "1.5.0", PublicVersion: "1.5"}, StatusOK, ""},
		{"behind a two-digit minor", PluginFact{Name: "rise-x-apps", Installed: true, Enabled: true, LocalVersion: "1.9.0", PublicVersion: "1.10.0"}, StatusWarn, "plugin.update"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pluginCheck(tc.plugin)
			if got.Status != tc.wantStatus || got.Fix != tc.wantFix {
				t.Fatalf("pluginCheck(%+v) = %+v, want status=%v fix=%q", tc.plugin, got, tc.wantStatus, tc.wantFix)
			}
		})
	}
}

func TestRun_HeadStaleAndSkip(t *testing.T) {
	f := baseFacts()
	f.HeadStale = true
	if c := findCheck(t, Run(f), "marketplace.head"); c.Status != StatusWarn || c.Fix != "marketplace.update" {
		t.Fatalf("stale check = %+v", c)
	}

	f = baseFacts()
	f.HeadSkip = true
	if c := findCheck(t, Run(f), "marketplace.head"); c.Status != StatusSkip {
		t.Fatalf("skip check = %+v", c)
	}
}

func TestRun_Node(t *testing.T) {
	cases := []struct {
		found   bool
		version string
		want    Status
	}{
		{false, "", StatusWarn},
		{true, "18.19.0", StatusWarn},
		{true, "20.11.0", StatusOK},
		{true, "v22.1.0", StatusOK},
	}
	for _, tc := range cases {
		f := baseFacts()
		f.NodeFound = tc.found
		f.NodeVersion = tc.version
		if c := findCheck(t, Run(f), "node"); c.Status != tc.want {
			t.Errorf("node(%v,%q) = %v, want %v", tc.found, tc.version, c.Status, tc.want)
		}
	}
}

func TestRun_Npmrc(t *testing.T) {
	f := baseFacts()
	f.NpmrcOffendingLines = []string{"@rise-x:registry=..."}
	c := findCheck(t, Run(f), "npmrc")
	if c.Status != StatusWarn || c.Fix != "npmrc.clean" {
		t.Fatalf("npmrc check = %+v", c)
	}
}

func TestRun_GitWindows_OnlyOnWindows(t *testing.T) {
	f := baseFacts()
	f.GOOS = "windows"
	f.GitFound = false
	c := findCheck(t, Run(f), "git.windows")
	if c.Status != StatusWarn {
		t.Fatalf("git.windows = %+v", c)
	}

	f2 := baseFacts() // darwin
	for _, c := range Run(f2) {
		if c.ID == "git.windows" {
			t.Fatal("git.windows must not appear on non-Windows")
		}
	}
}

func TestRun_EnvAutoupdater(t *testing.T) {
	f := baseFacts()
	f.DisableAutoupdater = true
	if c := findCheck(t, Run(f), "env.autoupdater"); c.Status != StatusWarn {
		t.Fatalf("expected warn, got %+v", c)
	}

	f.ForceAutoupdatePlugins = true
	if c := findCheck(t, Run(f), "env.autoupdater"); c.Status != StatusOK {
		t.Fatalf("force override should be ok, got %+v", c)
	}
}

// A GitHub rate limit makes PublicVersion the local clone's own version, which
// would compare equal to anything installed. The row must say so, not "ok".
func TestPluginCheck_OfflineSkips(t *testing.T) {
	p := PluginFact{Name: "rise-x-mcp", Installed: true, Enabled: true,
		LocalVersion: "1.3.1", PublicVersion: "1.3.1", Offline: true}
	got := pluginCheck(p)
	if got.Status != StatusSkip {
		t.Fatalf("status = %v, want skip: %+v", got.Status, got)
	}
	if got.Message != "Could not reach GitHub to check the public version." {
		t.Fatalf("message = %q", got.Message)
	}
	if got.Fix != "" {
		t.Fatalf("a skipped row should offer no fix: %+v", got)
	}
}

// Offline must not mask a plugin that is missing or turned off.
func TestPluginCheck_OfflineStillReportsInstallState(t *testing.T) {
	notInstalled := pluginCheck(PluginFact{Name: "rise-x-apps", Offline: true})
	if notInstalled.Status != StatusFail || notInstalled.Fix != "plugin.install" {
		t.Fatalf("not installed = %+v", notInstalled)
	}
	disabled := pluginCheck(PluginFact{Name: "rise-x-apps", Installed: true, Offline: true})
	if disabled.Status != StatusWarn {
		t.Fatalf("disabled = %+v", disabled)
	}
}

func TestPluginCheck_Messages(t *testing.T) {
	cases := []struct {
		plugin PluginFact
		want   string
	}{
		{PluginFact{Name: "p"}, "Not installed."},
		{PluginFact{Name: "p", Installed: true}, "Installed but turned off in Claude Code settings."},
		{PluginFact{Name: "p", Installed: true, Enabled: true, VersionUnknown: true},
			"Installed without a version stamp. Update to fix."},
		{PluginFact{Name: "p", Installed: true, Enabled: true, LocalVersion: "1.3.1", PublicVersion: "1.3.4"},
			"1.3.1 installed, 1.3.4 available."},
		{PluginFact{Name: "p", Installed: true, Enabled: true, LocalVersion: "1.3.4", PublicVersion: "1.3.4"},
			"Version 1.3.4, up to date."},
	}
	for _, tc := range cases {
		if got := pluginCheck(tc.plugin).Message; got != tc.want {
			t.Errorf("pluginCheck(%+v).Message = %q, want %q", tc.plugin, got, tc.want)
		}
	}
}

// The plan's node row distinguishes below 18.19, 18.19 to 19, and 20+.
func TestRun_NodeThresholds(t *testing.T) {
	cases := []struct {
		version string
		want    Status
		message string
	}{
		{"16.20.0", StatusWarn, "Version 16.20.0 is too old. Rise-X Apps needs Node 18.19 or newer."},
		{"18.18.2", StatusWarn, "Version 18.18.2 is too old. Rise-X Apps needs Node 18.19 or newer."},
		{"18.19.0", StatusWarn, "Version 18.19.0. Rise-X Apps works best on Node 20 or newer."},
		{"19.9.0", StatusWarn, "Version 19.9.0. Rise-X Apps works best on Node 20 or newer."},
		{"v24.18.0", StatusOK, "Version 24.18.0."},
	}
	for _, tc := range cases {
		f := baseFacts()
		f.NodeVersion = tc.version
		c := findCheck(t, Run(f), "node")
		if c.Status != tc.want || c.Message != tc.message {
			t.Errorf("node(%q) = %v %q, want %v %q", tc.version, c.Status, c.Message, tc.want, tc.message)
		}
	}
}

func TestRun_NodeMissingMessage(t *testing.T) {
	f := baseFacts()
	f.NodeFound = false
	c := findCheck(t, Run(f), "node")
	if c.Message != "Not found. Needed only for Rise-X Apps." {
		t.Fatalf("node message = %q", c.Message)
	}
}

// The fix button says what it does, and only appears where the kit has an
// installer to run.
func TestRun_NodeFix(t *testing.T) {
	cases := []struct {
		name    string
		found   bool
		version string
		label   string
	}{
		{"missing", false, "", "Install Node.js"},
		{"too old", true, "16.20.0", "Update Node.js"},
		{"below 20", true, "18.19.0", "Update Node.js"},
		{"current", true, "24.18.0", ""},
	}
	for _, tc := range cases {
		f := baseFacts()
		f.CanInstallNode = true
		f.NodeFound, f.NodeVersion = tc.found, tc.version
		c := findCheck(t, Run(f), "node")
		if tc.label == "" {
			if c.Fix != "" {
				t.Errorf("node(%s) offers %q, want no fix", tc.name, c.Fix)
			}
			continue
		}
		if c.Fix != "node.install" || c.FixLabel != tc.label {
			t.Errorf("node(%s) = fix %q %q, want node.install %q", tc.name, c.Fix, c.FixLabel, tc.label)
		}
		if c.FixTitle != nodeFixTitle {
			t.Errorf("node(%s) title = %q, want the install explanation", tc.name, c.FixTitle)
		}
	}
}

// Windows without winget: nothing to run, so the row keeps its nodejs.org link.
func TestRun_NodeFixAbsentWhenNoInstaller(t *testing.T) {
	f := baseFacts()
	f.NodeFound = false
	f.CanInstallNode = false
	c := findCheck(t, Run(f), "node")
	if c.Fix != "" || c.FixLabel != "" {
		t.Errorf("node = fix %q %q, want none", c.Fix, c.FixLabel)
	}
}

func TestRun_NpmrcMessage(t *testing.T) {
	f := baseFacts()
	f.NpmrcOffendingLines = []string{"@rise-x:registry=…"}
	f.NpmrcOffendingHost = "npm.pkg.github.com"
	c := findCheck(t, Run(f), "npmrc")
	want := "The @rise-x packages are on the public npm registry now, but ~/.npmrc still points them at npm.pkg.github.com."
	if c.Message != want {
		t.Fatalf("npmrc message = %q, want %q", c.Message, want)
	}
	if c.FixLabel != "Point @rise-x at public npm" {
		t.Fatalf("npmrc fix label = %q", c.FixLabel)
	}
}

func TestRun_EnvAutoupdaterMessage(t *testing.T) {
	f := baseFacts()
	f.DisableAutoupdater = true
	c := findCheck(t, Run(f), "env.autoupdater")
	want := "DISABLE_AUTOUPDATER is set without FORCE_AUTOUPDATE_PLUGINS, so skills will not update on their own."
	if c.Message != want {
		t.Fatalf("env message = %q, want %q", c.Message, want)
	}

	// The ok copy names settings.json's env block, the only source the check
	// reads, so it never claims more than it checked.
	ok := findCheck(t, Run(baseFacts()), "env.autoupdater")
	wantOK := "Nothing in your Claude Code settings blocks automatic updates."
	if ok.Message != wantOK {
		t.Fatalf("env ok message = %q, want %q", ok.Message, wantOK)
	}
}

// No row may leak engineering vocabulary into the partner-facing copy.
func TestRun_MessagesAreSentences(t *testing.T) {
	variants := []Facts{baseFacts(), {}, {GOOS: "windows"}}
	f := baseFacts()
	f.MarketplaceRegistered, f.HeadSkip, f.DisableAutoupdater = false, true, true
	f.NpmrcOffendingLines = []string{"x"}
	variants = append(variants, f)

	for _, facts := range variants {
		for _, c := range Run(facts) {
			if c.Message == "" {
				t.Errorf("%s has an empty message", c.ID)
				continue
			}
			if !strings.HasSuffix(c.Message, ".") {
				t.Errorf("%s message is not a sentence: %q", c.ID, c.Message)
			}
			for _, banned := range []string{"user-scope", "unverified", "(", "->"} {
				if strings.Contains(c.Message, banned) {
					t.Errorf("%s message contains %q: %q", c.ID, banned, c.Message)
				}
			}
		}
	}
}

// A 404 or an unparseable plugin.json is not "offline": the repo answered, so
// say what went wrong instead of hiding the row.
func TestPluginCheck_PublicCheckError(t *testing.T) {
	c := pluginCheck(PluginFact{Name: "rise-x-apps", Installed: true, Enabled: true,
		LocalVersion: "1.5.0", PublicCheckError: "HTTP 404"})
	if c.Status != StatusWarn {
		t.Fatalf("status = %s, want warn", c.Status)
	}
	if c.Message != "Could not check the public version (HTTP 404)." {
		t.Fatalf("message = %q", c.Message)
	}
	if c.Fix != "" {
		t.Fatalf("fix = %q, want none", c.Fix)
	}
}

func TestHeadCheck_LocalUnreadable(t *testing.T) {
	f := baseFacts()
	f.HeadLocalError = true
	c := findCheck(t, Run(f), "marketplace.head")
	if c.Status != StatusWarn || c.Fix != "marketplace.update" {
		t.Fatalf("marketplace.head = %+v", c)
	}
	if c.Message != "The local copy of the skill catalog is unreadable." {
		t.Fatalf("message = %q", c.Message)
	}
}

// With no CLI on the machine, every row that needs one skips: their fixes all
// go through the CLI, so offering one would only 400.
func TestRun_NoCLI_SkipsCLIDependentRows(t *testing.T) {
	f := baseFacts()
	f.CLIFound = false
	f.Plugins = []PluginFact{{Name: "rise-x-apps"}}

	byID := map[string]Check{}
	for _, c := range Run(f) {
		byID[c.ID] = c
	}
	for _, id := range []string{"marketplace.registered", "marketplace.autoupdate", "marketplace.head", "plugin.rise-x-apps"} {
		c, ok := byID[id]
		if !ok {
			t.Fatalf("no %s row", id)
		}
		if c.Status != StatusSkip || c.Fix != "" || c.Message != installCLIFirst {
			t.Errorf("%s = %+v, want a skip with no fix", id, c)
		}
	}
	if c := byID["cli"]; c.Status != StatusFail || c.Fix != "cli.install" {
		t.Errorf("cli = %+v, want fail with cli.install", c)
	}
	if c := byID["npmrc"]; c.Status != StatusOK {
		t.Errorf("npmrc = %+v, want it judged on its own", c)
	}
}

// A skill the organisation pushed reads as fine and offers nothing: installing
// or updating it from the public marketplace would leave two copies behind.
func TestPluginCheck_OrganisationSource(t *testing.T) {
	c := pluginCheck(PluginFact{Name: "rise-x-mcp", Installed: true, Enabled: true,
		InstallSource: SourceOrganisation, SourceName: "rise-x/rise-x-ai-marketplace",
		LocalVersion: "1.3.4", PublicVersion: "1.3.4"})
	if c.Status != StatusOK || c.Fix != "" {
		t.Fatalf("check = %+v, want an ok with no fix", c)
	}
	if c.Message != "Installed by your organisation (1.3.4)." {
		t.Fatalf("message = %q", c.Message)
	}
}

// Behind the public version is still not something to fix here: the copy comes
// from elsewhere, so the row says so and offers no button.
func TestPluginCheck_OrganisationSource_BehindPublic(t *testing.T) {
	c := pluginCheck(PluginFact{Name: "rise-x-mcp", Installed: true, Enabled: true,
		InstallSource: SourceOrganisation, LocalVersion: "1.3.3", PublicVersion: "1.3.4",
		UpdateAvailable: true})
	if c.Status != StatusOK || c.Fix != "" {
		t.Fatalf("check = %+v, want an ok with no fix", c)
	}
	if c.Message != "Installed by your organisation (1.3.3)." {
		t.Fatalf("message = %q", c.Message)
	}
}

func TestPluginCheck_MarketplaceSource(t *testing.T) {
	c := pluginCheck(PluginFact{Name: "rise-x-mcp", Installed: true, Enabled: true,
		InstallSource: SourceMarketplace, SourceName: "rise-x", LocalVersion: "1.3.3"})
	if c.Status != StatusOK || c.Fix != "" {
		t.Fatalf("check = %+v, want an ok with no fix", c)
	}
	if c.Message != "Installed from rise-x (1.3.3)." {
		t.Fatalf("message = %q", c.Message)
	}
}

// With every skill delivered by the organisation, nothing on this machine
// needs the public marketplace or its auto-update switch.
func TestRun_AllSkillsFromOrganisation(t *testing.T) {
	f := baseFacts()
	f.MarketplaceRegistered, f.AutoUpdatePresent, f.AutoUpdateEnabled = false, false, false
	for i := range f.Plugins {
		f.Plugins[i].InstallSource = SourceOrganisation
	}
	checks := Run(f)

	registered := findCheck(t, checks, "marketplace.registered")
	if registered.Status != StatusOK || registered.Fix != "" {
		t.Fatalf("marketplace.registered = %+v", registered)
	}
	if registered.Message != "Skills come from your organisation." {
		t.Fatalf("message = %q", registered.Message)
	}

	auto := findCheck(t, checks, "marketplace.autoupdate")
	if auto.Status != StatusOK || auto.Fix != "" {
		t.Fatalf("marketplace.autoupdate = %+v", auto)
	}
	if auto.Message != "Your organisation delivers skill updates automatically." {
		t.Fatalf("message = %q", auto.Message)
	}
}

// Skills from a mirror of the public marketplace are governed by that mirror's
// own auto-update flag, so the fix has to name it.
func TestRun_AutoUpdate_OtherMarketplace(t *testing.T) {
	f := baseFacts()
	f.AutoUpdatePresent, f.AutoUpdateEnabled = false, false
	f.AutoUpdateMarketplace = "rise-x"
	f.Plugins[0].InstallSource, f.Plugins[0].SourceName = SourceMarketplace, "rise-x"

	c := findCheck(t, Run(f), "marketplace.autoupdate")
	if c.Status != StatusWarn || c.Fix != "autoupdate.set" {
		t.Fatalf("check = %+v", c)
	}
	if c.FixArgs["marketplace"] != "rise-x" || c.FixArgs["enabled"] != true {
		t.Fatalf("fixArgs = %+v", c.FixArgs)
	}
	if c.Message != "Turned off from rise-x, so new skill versions will not arrive on their own." {
		t.Fatalf("message = %q", c.Message)
	}
}

func TestRun_McpStale(t *testing.T) {
	f := baseFacts()
	if c := findCheck(t, Run(f), "mcp.stale"); c.Status != StatusOK ||
		c.Message != "No old Rise-X connections." {
		t.Fatalf("clean machine = %+v", c)
	}

	f.McpStale = []mcp.Stale{
		{Name: "rise-x", Scope: mcp.ScopeUser, URL: "https://old.example.com/mcp",
			SuggestedURL: "https://mcp.rise-x.io/mcp"},
		{Name: "rise-x-test", Scope: mcp.ScopeLocal, ProjectPath: "/Users/p/one",
			URL: "https://old-test.example.com/mcp", SuggestedURL: "https://mcp-test.rise-x.io/mcp"},
	}
	c := findCheck(t, Run(f), "mcp.stale")
	if c.Status != StatusWarn || c.Fix != "mcp.fix" {
		t.Fatalf("check = %+v", c)
	}
	if c.Message != "2 connections point at an old Rise-X address." {
		t.Fatalf("message = %q", c.Message)
	}
	wantDetail := "rise-x (user): https://old.example.com/mcp → https://mcp.rise-x.io/mcp\n" +
		"rise-x-test (local): https://old-test.example.com/mcp → https://mcp-test.rise-x.io/mcp"
	if c.Detail != wantDetail {
		t.Fatalf("detail = %q", c.Detail)
	}
}

// The CLI cannot touch a connector the Desktop app configured, so that row
// reports without offering a button.
func TestRun_McpStale_DesktopOnly_NoFix(t *testing.T) {
	f := baseFacts()
	f.McpStale = []mcp.Stale{{Name: "rise-x", Scope: mcp.ScopeDesktop,
		URL: "https://old.example.com/mcp", SuggestedURL: "https://mcp.rise-x.io/mcp"}}

	c := findCheck(t, Run(f), "mcp.stale")
	if c.Status != StatusWarn || c.Fix != "" {
		t.Fatalf("check = %+v, want a warn with no fix", c)
	}
	if c.Message != "1 connection points at an old Rise-X address." {
		t.Fatalf("message = %q", c.Message)
	}
}

// Every fix needs the CLI, so with none found the row still reports but offers
// no button.
func TestRun_McpStale_NoCLI_NoFix(t *testing.T) {
	f := baseFacts()
	f.CLIFound = false
	f.McpStale = []mcp.Stale{{Name: "rise-x", Scope: mcp.ScopeUser,
		URL: "https://old.example.com/mcp", SuggestedURL: "https://mcp.rise-x.io/mcp"}}

	if c := findCheck(t, Run(f), "mcp.stale"); c.Fix != "" {
		t.Fatalf("check = %+v, want no fix", c)
	}
}

// The npmrc row carries the masked line(s) it would rewrite, so the partner
// can read them before pressing Fix.
func TestRun_Npmrc_Detail(t *testing.T) {
	f := baseFacts()
	f.NpmrcOffendingLines = []string{"@rise-x:registry=…"}

	c := findCheck(t, Run(f), "npmrc")
	if c.Detail != "@rise-x:registry=…" {
		t.Fatalf("detail = %q", c.Detail)
	}
}
