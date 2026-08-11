# Rise-X AI Public Marketplace

Rise-X's AI public plugin marketplace for Claude Code and Cowork (`rise-x-public`).

## Install

Paste this into any Claude Code session and let Claude do the installable
part:

```
Set up the Rise-X plugins from the public marketplace.

1. Run: claude plugin marketplace list
   If rise-x-public does not appear in the output, run:
   claude plugin marketplace add rise-x/rise-x-ai-public-marketplace

2. Run: claude plugin install rise-x-mcp@rise-x-public
3. Run: claude plugin install rise-x-apps@rise-x-public

Then stop and give me this checklist to complete myself, because you
cannot do these:

  - run /reload-plugins to activate the plugins in this session
  - open /plugin -> Marketplaces -> rise-x-public and look at
    auto-update. If it is not already enabled, enable it. Without it
    I will never receive any future version.
  - run /rise-x-mcp:setup to sign in to the Rise-X MCP servers

Do not attempt those three yourself, and do not ask me for any
credentials or tokens.
```

Drop the `rise-x-apps` line if you only need `rise-x-mcp` — each installed
plugin costs context in every session.

Then finish the three steps Claude hands back, yourself:

1. **`/reload-plugins`** — loads the skills and MCP servers into the current
   session. No restart needed.
2. **Enable auto-update.** Open `/plugin` → **Marketplaces** →
   `rise-x-public` and enable auto-update if it isn't already on.
   Third-party marketplaces have it **off by default**, and without it you
   never receive another version of either plugin.
3. **`/rise-x-mcp:setup`** — signs you in to the two Rise-X MCP servers
   (`rise-x-test` first, then `rise-x`). You complete the OAuth flow in your
   own browser; Claude verifies each server with a single call. See
   [Getting started](#getting-started) below for what to expect, and
   `plugins/rise-x-mcp/skills/setup/SKILL.md` for troubleshooting.

Finally, ask Claude to perform one real Rise-X operation to confirm the
whole chain works.

**On Claude Desktop:** open the integrated terminal with `` Ctrl+` `` (or the
Views menu) and paste there, or use **+** → **Plugins** → **Add plugin**.
Plugins are not available in WSL sessions. The Cowork tab is configured
separately, through **Customize** in the sidebar.

### Manual equivalent

If you'd rather run it yourself, the whole thing is five slash commands plus
the auto-update toggle — skip the third if you don't need `rise-x-apps`:

```
/plugin marketplace add rise-x/rise-x-ai-public-marketplace
/plugin install rise-x-mcp@rise-x-public
/plugin install rise-x-apps@rise-x-public
/reload-plugins
/rise-x-mcp:setup
```

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

#### Getting started

Install as described under [Install](#install), then run `/rise-x-mcp:setup`
— or just say "set up rise-x" and Claude runs the setup skill for you.
Onboarding always starts with the **test** server: you authenticate it via
`/mcp` (or, from a terminal,
`claude mcp login plugin:rise-x-mcp:rise-x-test`), Claude verifies with a
single call, then the same steps repeat for production. See
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
