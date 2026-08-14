---
name: setup
description: "Walkthrough for connecting Claude Code / Cowork to the Rise-X MCP servers. TRIGGER when: the user just installed the rise-x-mcp plugin, says 'set up rise-x', 'connect to rise-x', 'configure rise-x mcp', 'onboard me to rise-x', or hits an authentication/authorization failure calling a Rise-X MCP tool (OAuth loop, 401/403, 'no ecosystem', 'no tenant'). DO NOT TRIGGER for normal Rise-X operations once the servers are already authenticated — see the rise-x-mcp skill for that."
metadata:
  author: rise-x
---

# Rise-X MCP Setup

## 0. Prerequisites

The plugin bundles MCP *clients* only — it is useless without a **Rise-X tenant account**. If the user's organization does not have a Rise-X tenant, authentication will fail no matter how carefully you follow the steps below. The failure is legible in one of two ways:

- OAuth login itself is rejected (no account found), or
- OAuth completes, but `list_ecosystems` errors with an authorization failure or returns an empty list (account exists, but no tenant/ecosystem access).

If either happens and the user isn't sure whether their org has a tenant, tell them plainly: contact Rise-X via https://rise-x.io to get one provisioned. Don't try to work around this — there is no local fix.

## 1. Two servers, fixed order

The plugin bundles two HTTP MCP servers:

| Server | URL | Purpose |
|---|---|---|
| `rise-x-test` | https://mcp-test.rise-x.io/mcp | Test/sandbox environment — use for experiments |
| `rise-x` | https://mcp.rise-x.io/mcp | Production — real ecosystem data |

**Always authenticate `rise-x-test` first.** Only move on to `rise-x` (prod) once the test server is verified working. This order catches account/tenant problems in a sandbox before they touch real data — with one limit worth telling the user: test and production are **separate identity stores**. The same email is a different user record in each environment, so a clean test sign-in confirms the test account only and says nothing about production access.

## 2. Detect the user's surface

Users are assumed to be on the **Claude Desktop app** or the **terminal CLI**. Run this once, via the Bash tool: `echo "${CLAUDE_CODE_ENTRYPOINT:-}"`.

- `claude-desktop` → Desktop app
- `cli` → terminal CLI
- anything else, or empty → **do not guess** — ask the user which one they're using (one question), then continue. A user on **Cowork** follows the Desktop-app path throughout.

This variable is undocumented/internal (the official env-vars reference only lists `CLAUDECODE=1`), so it may change without notice. Treat it as a best-effort hint, not a guarantee: if it ever stops matching one of the two values above, fall back to asking — never to guessing wrong.

## 3. Authenticate `rise-x-test`

**You (Claude) cannot perform these steps — never attempt to complete OAuth yourself; guide the user and wait.** Only the user can sign in through the browser.

Ask the user to, based on their surface (§2):
- **Desktop app:** add the server as a **custom connector** — Settings → Connectors → **Add custom connector** → name `rise-x-test`, URL `https://mcp-test.rise-x.io/mcp`, no headers — then complete OAuth in the browser when prompted. (The desktop app has no `/mcp` command, and plugin-bundled servers don't surface an in-app sign-in prompt of their own.) The same custom-connector route covers **Cowork** — connectors sync through the claude.ai account.
- **CLI:** run `/mcp` (an interactive panel — arrow keys select `rise-x-test`); if `/mcp` is unavailable (e.g. non-interactive), authenticate from a terminal instead: plugin-bundled servers register under a scoped name, so run `claude mcp list` to confirm it (expect `plugin:rise-x-mcp:rise-x-test`), then `claude mcp login plugin:rise-x-mcp:rise-x-test`.

The `claude mcp` fallback must run in the **user's own terminal** — never through your Bash tool, and never in a cloud session's shell: that sandbox is not the user's machine, so the command appears to succeed while changing nothing the user's client can see.

Then have them come back and tell you it's done. If the Rise-X tools don't appear after a successful sign-in, have the user re-enable the connector or start a fresh session.

If OAuth itself fails for them here (rejected login, no redirect back), tell them to resolve that before touching `rise-x` — the same account problem will just repeat on production.

## 4. Verify test connectivity

Make exactly **one** call to confirm the connection works: `list_ecosystems`.

- **Healthy response**: one or more ecosystems come back — the account is authenticated *and* attached to a tenant.
- **No-tenant failure**: the call errors with an authorization/permission failure, or returns an **empty list**. This means the account authenticated but has no Rise-X tenant access — see §0.

Do **not** verify with `whoami` or `get_active_ecosystem`: on an account that has never selected an ecosystem, both report `ecosystem: null` even when access is perfectly fine — nothing sets the active ecosystem until §6 (once set, the server remembers the selection per user, best-effort, so a returning user may see one already). They tell you *which* account and deployment the session is bound to — not whether access works.

Do not proceed to production until `list_ecosystems` comes back non-empty.

## 5. Authenticate `rise-x` (production)

Only after `rise-x-test` verifies cleanly — and remember (§1) that production is a separate identity store, so a passing test proves nothing about this step. **You (Claude) still cannot perform this step yourself** — the user has to run it.

Same flow as §3, for the production server:
- **Desktop app:** custom connector — name `rise-x`, URL `https://mcp.rise-x.io/mcp`, no headers, OAuth in the browser.
- **CLI:** `/mcp` (arrow keys select `rise-x`); or from a terminal: `claude mcp list` (expect `plugin:rise-x-mcp:rise-x`), then `claude mcp login plugin:rise-x-mcp:rise-x`.

Once they confirm, re-run `list_ecosystems` yourself against `rise-x` and confirm ecosystems come back.

## 6. Select an ecosystem

The connection isn't usable yet: **every Rise-X tool except the session tools requires an active ecosystem**, and on a new account nothing has set one yet (once set, the server remembers it per user, best-effort). The selection is **per server** — setting an ecosystem on `rise-x-test` sets nothing on `rise-x`; repeat this section on each server the user will work with. On each such server:

1. `list_ecosystems` — discover what the account can reach.
2. If exactly one comes back, `set_active_ecosystem` with it and tell the user which one is active.
3. If several come back, **ask the user which one** before setting it — and if they're unsure, have them check with their Rise-X contact. Nothing warns you when you're pointed at the wrong tenant; the data just looks plausible.

You're fully connected once both servers return ecosystems and an active ecosystem is set. From here, normal Rise-X operations (building flows, managing work items, configuring layouts, etc.) are covered by the main `rise-x-mcp` skill — it loads automatically whenever the conversation touches Rise-X.

Before wrapping up, suggest one piece of housekeeping: if the plugin was installed from the CLI or the desktop plugin browser, the user should enable auto-update for the `rise-x-public` marketplace — third-party marketplaces have it **disabled by default**, so without this one-time step plugin fixes never arrive. It's an interactive panel only the user can drive, in a terminal session: `/plugin` → **Marketplaces** tab → select `rise-x-public` → **Enable auto-update**. (Not applicable to installs that came through claude.ai's Customize page or organization distribution — those update through claude.ai.)

## 7. Troubleshooting

| Symptom | Likely cause | What to do |
|---|---|---|
| OAuth login loops or is rejected outright | No Rise-X account, or wrong account signed in to the browser | Confirm which account the user intended to use; if none exists, contact Rise-X support (§0) |
| OAuth succeeds but every tool call 401s / 403s | Token is for the wrong environment (test vs. prod), or the account has no tenant/ecosystem access | Re-check which server the user authenticated (`rise-x-test` vs `rise-x`); if tenant access is the issue, contact Rise-X support |
| `whoami` / `get_active_ecosystem` shows identity with `ecosystem: null` | **Normal** — no active ecosystem has been selected yet (§6) | Run `list_ecosystems`, then `set_active_ecosystem` |
| `list_ecosystems` returns an empty list | Account exists but isn't attached to a tenant | Contact Rise-X support — this isn't something Claude Code can fix locally |
| First call on the other server fails with "No ecosystem selected" | **Normal** — ecosystem selection is per server | Run `list_ecosystems`, then `set_active_ecosystem` on that server too (§6) |
| No `/mcp` command; servers absent from Connectors | Desktop app — bundled servers don't surface an in-app sign-in prompt | Add each server as a custom connector (§3/§5) |
| `claude mcp …` reports success but nothing changes | The command ran in a session's sandbox shell, not the user's machine | Have the user run it in their own terminal, or use the custom-connector route |
| Works on `rise-x-test` but not `rise-x` (or vice versa) | Only one server was authenticated | Repeat the auth flow (§3/§5) for the other server |
| Servers authenticated but no Rise-X tools appear | Connector disabled, or the session predates the connection | Re-enable the connector, or start a fresh session |
| Tool calls succeed but return data the user doesn't recognize | Wrong ecosystem selected in a tenant that has several | Re-run `list_ecosystems` and `set_active_ecosystem` (§6) |

**Which environment to use:** default to `rise-x-test` for anything exploratory — building a new workflow, trying out a component, testing an integration. Only use `rise-x` (production) once you deliberately mean to change real ecosystem data.

**Where to get help:**
- Plugin bugs (tool errors, this skill being wrong, packaging issues) → open an issue in this repository.
- Account, tenant, or authentication problems → Rise-X support at https://rise-x.io.
