// Package runner executes external commands (the claude CLI, node, git) with
// a consistent environment, and lets callers observe output as it streams.
package runner

import (
	"context"
	"regexp"
	"strings"
)

// Line is one line of output from a running command.
type Line struct {
	// Stderr is true when the line came from the command's stderr.
	Stderr bool
	Text   string
}

// Runner executes a command. The real implementation is exec.go; package
// runnertest holds the double tests use instead of an installed claude CLI.
type Runner interface {
	// Run executes name with args to completion and returns combined output.
	Run(ctx context.Context, name string, args []string) (stdout, stderr string, exitCode int, err error)
	// Stream executes name with args, invoking onLine for each output line as
	// it is produced, in the order received (interleaved across stdout and
	// stderr). onLine may be nil.
	Stream(ctx context.Context, name string, args []string, onLine func(Line)) (exitCode int, err error)
	// StreamDir is Stream with dir as the command's working directory; an
	// empty dir keeps the kit's own. `claude mcp ... -s local` writes to
	// whichever project it runs in, so that scope needs one.
	StreamDir(ctx context.Context, dir, name string, args []string, onLine func(Line)) (exitCode int, err error)
}

// Argv renders name and args as a shell-like display string for logs. It does
// not attempt full shell quoting; it only quotes args containing whitespace.
func Argv(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteIfNeeded(name))
	for _, a := range args {
		parts = append(parts, quoteIfNeeded(a))
	}
	return strings.Join(parts, " ")
}

func quoteIfNeeded(s string) string {
	if strings.ContainsAny(s, " \t\"") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

// redactKeyValue matches a credential key followed by its value. It is not
// anchored on a word boundary because npm's own keys are "_authToken" and
// "_password", where \b would not match; and it takes the rest of the line
// because a value can contain spaces ("Bearer <jwt>").
var redactKeyValue = regexp.MustCompile(
	`(?i)(authorization|token|secret|password|passwd|api[_-]?key|credential)(\s*[:=]\s*|\s+)\S.*`)

// redactPrefix catches bare credentials that carry a well-known prefix and no
// key at all, e.g. a GitHub PAT or an Anthropic API key echoed on its own.
var redactPrefix = regexp.MustCompile(`(ghp_|github_pat_|sk-ant-)\S+`)

// Redact replaces anything that looks like a token/secret/authorization
// key-value pair, or a bare prefixed credential, with a placeholder, so job
// logs are safe to display.
func Redact(s string) string {
	s = redactKeyValue.ReplaceAllString(s, "[redacted]")
	return redactPrefix.ReplaceAllString(s, "[redacted]")
}
