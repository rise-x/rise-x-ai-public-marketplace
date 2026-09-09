// Package buildinfo carries version metadata set at link time via -ldflags.
package buildinfo

// Version and Commit are overridden at build time, e.g.:
//
//	go build -ldflags "-X github.com/rise-x/rise-x-ai-public-marketplace/kit/internal/buildinfo.Version=0.1.0 ..."
var (
	Version = "dev"
	Commit  = "none"
)

// String renders a one-line "version (commit)" summary.
func String() string {
	return Version + " (" + Commit + ")"
}
