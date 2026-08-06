# Rise-X AI Public Marketplace

Rise-X's AI public plugin marketplace for Claude Code and Cowork (`rise-x-public`).

## Install the marketplace

```
/plugin marketplace add rise-x/rise-x-ai-public-marketplace
```

Then install whichever plugin you need from the list below, with
`/plugin install <plugin-name>@rise-x-public`, followed by `/reload-plugins`
so its skills and MCP servers load into the current session — no restart
needed.

## Available plugins

### rise-x-mcp

Configure workflows, query flows and layouts, and manage work items on the
Rise-X Ecosystem Orchestration Platform (EOP) — via two bundled HTTP MCP
servers plus domain skills that route Claude to the right tools and
guardrails for each task: flows, steps, layouts, components, work items,
assets, dashboards, external-API integrations, and more.

**What's inside:** the domain skill (`skills/rise-x-mcp`) that routes Claude
to the right tools and reference docs for flows, layouts, work items,
assets, dashboards, search, apps, and integrations; a dedicated authoring
protocol for integration setup (secrets handling, slot-filling, dry-runs);
and a `setup` skill for first-time connection. Bundles two HTTP MCP servers —
`rise-x-test` (sandbox) and `rise-x` (production), configured in
`plugins/rise-x-mcp/.mcp.json`.

**Requirements:**
- Claude Code or Cowork
- A Rise-X tenant account (the plugin authenticates against your
  organization's Rise-X ecosystem — without a tenant, authentication will fail)
- Network access to `mcp.rise-x.io` and `mcp-test.rise-x.io`

**Quick start** — paste this into a fresh Claude Code session to install and
connect the plugin:

```
Help me install and set up the Rise-X plugin:

1. Run `claude plugin marketplace add rise-x/rise-x-ai-public-marketplace` and then
   `claude plugin install rise-x-mcp@rise-x-public` using your terminal tool. If
   the `claude` CLI isn't available to you, give me those two commands as
   `/plugin …` slash commands to run myself, and wait until I confirm.
2. Ask me to run `/reload-plugins` (only I can run slash commands) and wait for
   my confirmation.
3. Verify the plugin is installed and enabled; if anything failed, show me the
   exact error and how to fix it before going further.
4. Then follow the plugin's `setup` skill to connect the two Rise-X MCP servers.
   I'll complete the OAuth steps in my browser myself when you tell me to.
```

**Getting started manually:** after installing and reloading, ask Claude to
run the setup skill (or just say "set up rise-x") to walk through connecting
the MCP servers. Onboarding always starts with the **test** server — the user
authenticates it via `/mcp` (or, from a terminal,
`claude mcp login plugin:rise-x-mcp:rise-x-test`), Claude
verifies with a single call, then the same steps repeat for production. See
`plugins/rise-x-mcp/skills/setup/SKILL.md` for the full walkthrough,
including troubleshooting for OAuth and authorization failures.

### rise-x-apps

Design, build, and deploy federated apps for the Rise-X platform with
[`@rise-x/apps-sdk`](https://www.npmjs.com/package/@rise-x/apps-sdk) (public
npm). The skill drives the full app lifecycle: a design interview, a
single-file HTML design mock on the Rise-X design system iterated to explicit
approval, scaffolding via the SDK's CLI (`npx @rise-x/apps-sdk init`),
implementation on the SDK's shell hooks / connectors / query layer / UI
components, and deployment to a Rise-X environment.

**What's inside:** the `rise-x-apps` skill with reference docs for each phase
(design, build, upgrade) plus the Rise-X experience principles — the visual
language and interaction rules every Rise-X app is held to.

**Requirements:**
- Claude Code or Cowork
- Node.js ≥ 18.19 (for the scaffolder CLI and app builds)
- The **rise-x-mcp** plugin from this marketplace — used for integration
  discovery (flows, assets, agents) and for deploying app bundles; without
  it, deploys fall back to manual upload in the Rise-X Apps UI
- A Rise-X tenant account for deploys (deploying requires the
  environment-orchestrator role)

## Versioning

Each plugin's version lives only in its own
`plugins/<name>/.claude-plugin/plugin.json` — `marketplace.json`
intentionally carries no version field, since plugin.json takes precedence
and dual fields just mask edits. Any PR that changes files under
`plugins/<name>/` must bump that plugin first, via
`./scripts/bump.sh <name> patch|minor|major`; CI (`scripts/check-version.sh`)
rejects PRs that touch a plugin without a strictly-greater version. Changes
limited to a plugin's own `README.md` or its `tests/`/`test/` directory are
exempt and don't require a bump.

## Contributing

This repository does not accept pull requests from outside the Rise-X
organization — external PRs are automatically closed. If you hit a bug or
have a feature request, please [open an issue](../../issues) instead; we'll
take it from there.

## License

This repository is Rise-X proprietary intellectual property, provided under
a source-available license: you may install and use the plugins with the
Rise-X platform via Claude Code / Cowork, but modification and
redistribution are not permitted. See [LICENSE](LICENSE) for the full terms.
