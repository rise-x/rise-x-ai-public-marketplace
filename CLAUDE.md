# rise-x-ai-public-marketplace — maintainer guide

This is the `rise-x-public` Claude Code / Cowork plugin marketplace. It ships
Rise-X plugins — currently one, `rise-x-mcp` (skills plus two bundled HTTP
MCP servers), with more expected to follow. A plugin's implementation may
live partly outside this repo (e.g. rise-x-mcp's MCP server code is private)
— this repo only ships what Claude Code actually installs: plugin manifests,
skills, and reference docs. That makes this repo itself the shipped product:
every skill/reference edit lands verbatim in a customer's Claude session, so
review it like production code, not internal notes.

## Versioning

Each plugin's version lives **only** in its own
`plugins/<name>/.claude-plugin/plugin.json` — never add a version to
`marketplace.json`; plugin.json silently takes precedence and dual fields
just mask edits. This field is also Claude Code's update cache key: merging a
change without bumping it means installed users receive nothing.

Any PR touching `plugins/<name>/**` must bump that plugin:

```
./scripts/bump.sh <name> patch|minor|major
```

Exempt: a PR whose changes under `plugins/<name>/` are limited to
`plugins/<name>/README.md` and/or `plugins/<name>/tests/` or
`plugins/<name>/test/` does not require a bump.

A PR that adds a brand-new plugin must also add it to the "Which plugin?"
dropdown in `.github/ISSUE_TEMPLATE/bug_report.yml`, and add it to the
"Which MCP server?" dropdown there too if the plugin bundles MCP servers.

CI enforces this — `scripts/check-version.sh`, run inside the `validate`
job, does two independent things: (1) for each plugin with non-exempt
changes, compares its `plugin.json` version against the PR base and requires
it to be strictly greater; (2) separately verifies that `marketplace.json`
entries and `plugins/<name>/` directories stay consistent with each other
(every local-source entry has a matching directory and vice versa) — this
check does not involve version numbers at all, since marketplace.json never
carries one.

## Validate before any PR

```
claude plugin validate .
claude plugin validate ./plugins/<name>   # for every plugin directory
```

All must pass. Also run each with `--strict` — it should pass too.

## Public-repo scrub rules (repo-wide)

Never commit, in any plugin: internal ecosystem/tenant IDs, personal names
or emails, internal hostnames.

Known-benign, expected hits: the `localhost_public_url` warning documented in
`plugins/rise-x-mcp/skills/rise-x-mcp/references/managing-apps.md`, generic
"feedback" wording in
`plugins/rise-x-mcp/skills/rise-x-mcp/references/validation.md`, and this
file (it quotes the pattern above). Anything else is a real hit — fix it.
Future known-benign hits specific to one plugin belong in that plugin's own
"Per-plugin rules" section above, not here.

## Process

Use conventional commits standard. Never commit directly to `main` — a ruleset requires a PR, code-owner
review, and a passing `validate` check. PRs opened from outside the org are
auto-closed by workflow; external input arrives via issues, not PRs.
