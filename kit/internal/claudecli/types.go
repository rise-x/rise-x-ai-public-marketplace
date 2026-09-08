package claudecli

// MarketplaceName and MarketplaceRepo identify this marketplace; every
// package that needs to name it (catalog, server, doctor) uses these instead
// of a repeated literal.
const (
	MarketplaceName = "rise-x-public"
	MarketplaceRepo = "rise-x/rise-x-ai-public-marketplace"
)

// InstalledPlugin is one entry of `claude plugin list --json`'s "installed".
type InstalledPlugin struct {
	ID          string `json:"id"` // "<plugin>@<marketplace>"
	Version     string `json:"version"`
	Scope       string `json:"scope"`
	Enabled     bool   `json:"enabled"`
	InstallPath string `json:"installPath"`
	ProjectPath string `json:"projectPath,omitempty"`
	InstalledAt string `json:"installedAt,omitempty"`
	LastUpdated string `json:"lastUpdated,omitempty"`
}

// AvailablePlugin is one entry of `claude plugin list --json --available`'s
// "available". "source" is deliberately absent: the CLI writes it as a string
// for a path source and as an object for a url source, so either Go type fails
// the whole decode for the other.
type AvailablePlugin struct {
	Name            string `json:"name"`
	MarketplaceName string `json:"marketplaceName"`
	Description     string `json:"description,omitempty"`
}

// PluginListResult is `claude plugin list --json [--available]`'s shape.
type PluginListResult struct {
	Installed []InstalledPlugin `json:"installed"`
	Available []AvailablePlugin `json:"available,omitempty"`
}

// Marketplace is one entry of `claude plugin marketplace list --json`.
// Exactly one of Repo (github source) or URL (git source) is set.
type Marketplace struct {
	Name            string `json:"name"`
	Source          string `json:"source"`
	Repo            string `json:"repo,omitempty"`
	URL             string `json:"url,omitempty"`
	InstallLocation string `json:"installLocation"`
}
