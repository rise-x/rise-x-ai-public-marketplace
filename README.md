# Rise-X AI Public Marketplace

Rise-X's AI public plugin marketplace for Claude Code and Cowork (`rise-x-public`).

## Install the marketplace

**Terminal (Claude Code CLI):**

```
/plugin marketplace add rise-x/rise-x-ai-public-marketplace
```

Then install whichever plugin you need from the list below, with
`/plugin install <plugin-name>@rise-x-public`. Recent Claude Code versions
activate the plugin as part of the install and say so (`Plugin is now
active.`); if the summary says `Run /reload-plugins to activate.` instead,
run that and its skills and MCP servers load into the current session, with
no restart needed.

While you're at it, enable auto-update for the marketplace — it's **off by
default** for third-party marketplaces like this one, and without it you
won't receive plugin fixes: run `/plugin`, open the **Marketplaces** tab,
select `rise-x-public`, choose **Enable auto-update**. Details in
[Keeping plugins up to date](#keeping-plugins-up-to-date).

**Desktop app (Code tab):** the `/plugin` slash commands are terminal-only
and won't run in the desktop app. Add the marketplace once from your own
terminal instead:

```
claude plugin marketplace add rise-x/rise-x-ai-public-marketplace
```

Then, back in a Code session, click the **+** button next to the prompt box →
**Plugins** → **Add plugin** to open the plugin browser, and pick the plugin
from this marketplace. (The **+** button is unavailable in cloud and WSL
sessions.) If the app points you to the Claude CLI instead, run the
marketplace command above in a terminal first, then retry. If the plugin's
skills don't appear afterwards, run `/reload-plugins`, or, if your app
version doesn't have it, close and reopen the app. Finally, enable
auto-update for the marketplace once, from a terminal session (see
[Keeping plugins up to date](#keeping-plugins-up-to-date)) — the desktop app
installs plugins from the same local state, so it picks updates up too.

**Cowork tab:** Cowork sources its plugins from the claude.ai-synced
**Customize** configuration, not from the CLI's local state, so the terminal
command above does not reach it. Add the marketplace in-app instead: open
**Customize → Plugins**, select **Add marketplace**, and enter
`rise-x/rise-x-ai-public-marketplace` (the GitHub `owner/repo` shorthand —
public repositories work for marketplaces you add yourself), then install
the plugin from its listing. Your organization may also distribute these
plugins centrally (below), in which case they arrive without any of this.

**Organization-distributed:** if your organization already delivers these
plugins, skip the steps above; they arrive automatically. Admins: Claude's
organization plugin sync (**Organization settings → Plugins**, Team and
Enterprise plans) accepts **private or internal repositories only**, so it
cannot sync this public repo directly. Mirror or fork it into a private or
internal repository and point the organization marketplace at that copy.
Cowork and Skills must be enabled for the organization first.

## Keeping plugins up to date

Every release of a plugin bumps its version, but by default you won't
receive releases automatically: Claude Code enables background auto-update
only for official Anthropic marketplaces — third-party marketplaces like
this one have it **disabled by default**. Pick one of these:

- **Enable auto-update (recommended, one-time):** in a terminal session run
  `/plugin`, open the **Marketplaces** tab, select `rise-x-public`, and
  choose **Enable auto-update**. Claude Code then refreshes the marketplace
  and updates installed plugins in the background after a session starts, on
  a random delay of up to ten minutes, so the running session keeps the
  versions it launched with. When something updated, it prompts you to run
  `/reload-plugins` (otherwise the new versions load on your next launch).
- **Update manually:**

  ```
  claude plugin marketplace update rise-x-public
  claude plugin update rise-x-mcp@rise-x-public
  claude plugin update rise-x-apps@rise-x-public   # one per installed plugin
  ```

  Slash-command equivalents work inside a terminal session; restart the
  session or run `/reload-plugins` to apply.
- **Organization-managed (zero-touch):** admins can register this marketplace
  through managed settings, as an `extraKnownMarketplaces` entry with
  `"autoUpdate": true`, and users then receive updates without doing
  anything. This works with the public repo as-is. Distributing through
  **Organization settings → Plugins** instead requires a private or internal
  mirror of this repository (see
  [Install the marketplace](#install-the-marketplace)); members then pick up
  changes on their next session or plugin refresh.

Notes: Cowork sources plugins from the claude.ai-synced Customize
configuration rather than the CLI's local state, so neither the terminal
commands nor the `/plugin` toggle above affect it. Update there by opening
**Customize → Plugins** and clicking **Update** on this marketplace — or, if
your organization distributes the plugins centrally, new versions arrive on
their own. And if your environment sets `DISABLE_AUTOUPDATER`, plugin
auto-updates are disabled too: set `FORCE_AUTOUPDATE_PLUGINS=1` alongside it
to keep plugin updates while managing Claude Code updates manually.

## Connecting to Rise-X from Claude

The full walkthrough from a fresh Claude install to a working, authenticated
Rise-X connection. This section is the canonical setup guide — link new users
here. It's also the same walkthrough the `rise-x-mcp` plugin's `setup` skill
drives in-session: after installing, you can just tell Claude "set up rise-x".

### Prerequisites

- **A Rise-X tenant account.** The plugin bundles MCP *clients* only — without
  a tenant account, authentication fails no matter how carefully you follow
  the steps below. If your organization doesn't have one, contact Rise-X via
  <https://rise-x.io> before starting.
- **Claude Code (terminal) or the Claude desktop app.** A plugin installed
  via the Claude Code CLI doesn't reach claude.ai web chat, the Chat tab, or
  Cowork — those surfaces take their plugins from the claude.ai-synced
  **Customize** configuration instead (installed there yourself, or
  distributed by your organization).
- **Network access.** Corporate environments should allowlist:
  - `mcp.rise-x.io` and `mcp-test.rise-x.io` — the MCP servers; their OAuth
    authorization and token endpoints live on these same hosts
  - the Rise-X sign-in page your browser is redirected to during OAuth — if
    your IT team needs the exact hostname, ask your Rise-X contact
  - `github.com` — this marketplace is fetched from GitHub
  - `mcp-proxy.anthropic.com` — only if you connect the servers as claude.ai
    custom connectors (the desktop and Cowork route below); that traffic is
    routed through Anthropic's connector proxy
  - Anthropic's own domains — see
    [network access requirements](https://code.claude.com/docs/en/desktop#network-access-requirements)
    for the desktop app, and
    [network configuration](https://code.claude.com/docs/en/network-config)
    for the standalone CLI, proxies, and custom certificate authorities

### 1. Install the plugin

Add the marketplace and install `rise-x-mcp` as described in
[Install the marketplace](#install-the-marketplace) above — slash commands in
a terminal session, or the plugin browser in a desktop Code session.

### 2. Sign in to both servers

The plugin bundles two HTTP MCP servers:

| Server | URL | Environment |
| --- | --- | --- |
| `rise-x-test` | `https://mcp-test.rise-x.io/mcp` | Test/sandbox — use for anything exploratory |
| `rise-x` | `https://mcp.rise-x.io/mcp` | Production — real ecosystem data |

**Always connect `rise-x-test` first**, and only move to production once test
verifies cleanly. Note that test and production are **separate identity
stores**: the same email is a different user record in each environment, so a
working test sign-in confirms the test account only — it says nothing about
production access.

**Claude Code CLI (terminal):** type `/mcp`, select `rise-x-test` (listed
under the `rise-x-mcp` plugin), and complete sign-in in your browser. If
`/mcp` isn't available (e.g. a non-interactive session), from your own
terminal:

```
claude mcp list                                   # expect plugin:rise-x-mcp:rise-x-test
claude mcp login plugin:rise-x-mcp:rise-x-test
```

**Claude desktop app (Code tab and Cowork):** the desktop app has no `/mcp`
command, and the plugin's bundled servers don't currently surface an in-app
sign-in prompt. Add each server as a **custom connector** instead:

> Settings → Connectors → **Add custom connector** → name `rise-x-test`, URL
> `https://mcp-test.rise-x.io/mcp`, no headers — then complete sign-in in
> your browser when prompted.

Run `claude mcp …` / `claude plugin …` commands **in your own terminal, never
inside a Claude session's shell**. In cloud-based sessions (claude.ai on the
web), that shell is a disposable sandbox — the commands appear to succeed
while changing nothing your app can see.

### 3. Verify with `list_ecosystems`

Ask Claude to run **`list_ecosystems`** against the server you just connected.

- **Working:** one or more ecosystems come back.
- **Not working:** the call returns an authorization error, or an empty list —
  the account signed in but isn't attached to a Rise-X tenant. Contact Rise-X
  rather than retrying; there is no local fix.

Don't use `whoami` as the health check: on an account that has never
selected an ecosystem it reports `ecosystem: null` even when access is fine
— the active ecosystem stays unset until step 5 (and once set, the server
remembers your selection, so it may already be populated in a later
session). `whoami` answers *which* account and environment you're on, not
whether access works.

### 4. Repeat for production

Connect `rise-x` the same way (CLI: `/mcp` or
`claude mcp login plugin:rise-x-mcp:rise-x`; desktop: custom connector with
`https://mcp.rise-x.io/mcp`) and verify again with `list_ecosystems` —
production is a separate identity store, so passing test doesn't carry over.

### 5. Choose your ecosystem

Ask Claude to run `set_active_ecosystem` with the ecosystem you want. The
selection is **per server**: setting it on `rise-x-test` sets nothing on
`rise-x`, so repeat this on each server you'll work with. If
`list_ecosystems` returned more than one, confirm which is correct before
proceeding — **nothing warns you when you're pointed at the wrong tenant**,
and every Rise-X tool except the session tools requires an active ecosystem.

You're connected once both servers return ecosystems and you've selected an
ecosystem on the server you'll work with.

### Troubleshooting

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| `whoami` returns your identity but `ecosystem: null` | **Normal** — no active ecosystem has been selected yet | Run `list_ecosystems`, then `set_active_ecosystem` |
| No `/mcp` command; servers absent from Connectors | Claude desktop app — bundled servers don't surface an in-app sign-in | Add each server as a custom connector (URLs above) |
| `claude mcp …` reports success but nothing changes | The command ran inside a cloud session's sandbox shell, not on your machine | Run it in your own terminal, or use the custom-connector route |
| Servers connected but no Rise-X tools appear | Connector disabled, or the session predates the connection | Enable/re-enable the connector, or start a fresh session |
| Marketplace won't load | Network can't reach GitHub, or the name was mistyped | Confirm exactly `rise-x/rise-x-ai-public-marketplace`; check `github.com` is allowlisted |
| Sign-in loops or is rejected | No Rise-X account, or the wrong account signed in to the browser | Check the browser's signed-in account; if no account exists, contact Rise-X |
| Sign-in works, tool calls return 401/403 | Wrong environment, or no tenant access | Confirm which server you signed in to; otherwise contact Rise-X |
| `list_ecosystems` returns an empty list | Account exists but isn't attached to a tenant | Contact Rise-X — there is no local fix |
| First call on the other server fails with "No ecosystem selected" | **Normal** — ecosystem selection is per server | Run `list_ecosystems`, then `set_active_ecosystem` on that server too |
| Tools return unfamiliar data | Wrong ecosystem selected in a multi-tenant account | Re-run `list_ecosystems` and `set_active_ecosystem` |

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
- Network access — see the
  [setup guide's prerequisites](#connecting-to-rise-x-from-claude)

**Quick start** — paste this into a fresh Claude Code session **in your
terminal** to install and connect the plugin (desktop app users: follow
[Connecting to Rise-X from Claude](#connecting-to-rise-x-from-claude)
instead):

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
5. Once everything works, remind me to enable auto-update for the marketplace
   so plugin fixes arrive automatically — I'll run `/plugin`, open the
   Marketplaces tab, select rise-x-public, and choose Enable auto-update.
```

**Getting started manually:** follow
[Connecting to Rise-X from Claude](#connecting-to-rise-x-from-claude) above —
or, after installing and reloading, just say "set up rise-x" and Claude walks
you through the same steps in-session via the plugin's `setup` skill
(`plugins/rise-x-mcp/skills/setup/SKILL.md`), including troubleshooting for
OAuth and authorization failures.

### rise-x-apps

Design, build, and deploy federated apps for the Rise-X platform with
[`@rise-x/apps-sdk`](https://www.npmjs.com/package/@rise-x/apps-sdk) (public
npm). The skill drives the full app lifecycle: a design interview, a
single-file HTML design mock on the Rise-X design system iterated to explicit
approval, scaffolding via the SDK's CLI (`npx @rise-x/apps-sdk init`),
implementation on the SDK's shell hooks / connectors / query layer / UI
components, and deployment to a Rise-X environment.

**What's inside:** the `rise-x-apps` skill with reference docs for each phase
(design, build, upgrade, offline) plus the Rise-X experience principles — the
visual language and interaction rules every Rise-X app is held to.

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
