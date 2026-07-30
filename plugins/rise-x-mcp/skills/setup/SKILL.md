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
- OAuth completes, but every tool call returns an authorization error (account exists, but no tenant/ecosystem access).

If either happens and the user isn't sure whether their org has a tenant, tell them plainly: contact Rise-X via https://rise-x.io to get one provisioned. Don't try to work around this — there is no local fix.

## 1. Two servers, fixed order

The plugin bundles two HTTP MCP servers:

| Server | URL | Purpose |
|---|---|---|
| `rise-x-test` | https://mcp-test.rise-x.io/mcp | Test/sandbox environment — use for experiments |
| `rise-x` | https://mcp.rise-x.io/mcp | Production — real ecosystem data |

**Always authenticate `rise-x-test` first.** Only move on to `rise-x` (prod) once the test server is verified working. This order catches account/tenant problems in a sandbox before they touch real data.

Cowork: same flow, via each connector's own authorization prompt on first use — same order, same verification.

## 2. Detect the user's surface

Users are assumed to be on the **Claude Desktop app** or the **terminal CLI**. Run this once, via the Bash tool: `echo "${CLAUDE_CODE_ENTRYPOINT:-}"`.

- `claude-desktop` → Desktop app
- `cli` → terminal CLI
- anything else, or empty → **do not guess** — ask the user which one they're using (one question), then continue

This variable is undocumented/internal (the official env-vars reference only lists `CLAUDECODE=1`), so it may change without notice. Treat it as a best-effort hint, not a guarantee: if it ever stops matching one of the two values above, fall back to asking — never to guessing wrong.

## 3. Authenticate `rise-x-test`

**You (Claude) cannot perform these steps — never attempt to complete OAuth yourself; guide the user and wait.** Only the user can trigger `/mcp` or sign in through the browser.

Ask the user to, based on their surface (§2):
- **Desktop app:** type `/mcp` in the message input box, select `rise-x-test` (listed under the rise-x-mcp plugin), and complete OAuth in the default browser.
- **CLI:** run `/mcp` (an interactive panel — arrow keys select `rise-x-test`); if `/mcp` is unavailable (e.g. non-interactive), authenticate from a terminal instead: plugin-bundled servers register under a scoped name, so run `claude mcp list` to confirm it (expect `plugin:rise-x-mcp:rise-x-test`), then `claude mcp login plugin:rise-x-mcp:rise-x-test`.

Then come back and tell you it's done.

If OAuth itself fails for them here (rejected login, no redirect back), tell them to resolve that before touching `rise-x` — the same account problem will just repeat on production.

## 4. Verify test connectivity

Make exactly **one** call to confirm the connection works: `whoami`. If `whoami` isn't available, fall back to `get_active_ecosystem`.

- **Healthy response**: returns your user identity and active ecosystem (tenant/workspace) — a real name, email, or ecosystem identifier comes back.
- **No-tenant failure**: the call itself errors out with an authorization/permission failure, or returns an identity with no ecosystem attached. This means the account authenticated but has no Rise-X tenant access — see §0.

Do not proceed to production until this call comes back healthy.

## 5. Authenticate `rise-x` (production)

Only after `rise-x-test` verifies cleanly. **You (Claude) still cannot perform this step yourself** — the user has to run it.

Ask the user to, based on their surface (§2):
- **Desktop app:** type `/mcp` in the message input box, select `rise-x` (listed under the rise-x-mcp plugin), and complete OAuth in the default browser.
- **CLI:** run `/mcp` (arrow keys select `rise-x`); if unavailable, authenticate from a terminal instead: confirm the scoped name with `claude mcp list` (expect `plugin:rise-x-mcp:rise-x`), then `claude mcp login plugin:rise-x-mcp:rise-x`.

Then come back and tell you it's done. Once they confirm, re-run the same single verification call (`whoami`, or `get_active_ecosystem`) yourself against `rise-x` and confirm a healthy response.

You're fully connected once both servers pass verification. From here, normal Rise-X operations (building flows, managing work items, configuring layouts, etc.) are covered by the main `rise-x-mcp` skill — it loads automatically whenever the conversation touches Rise-X.

## 6. Troubleshooting

| Symptom | Likely cause | What to do |
|---|---|---|
| OAuth login loops or is rejected outright | No Rise-X account, or wrong account signed in to the browser | Confirm which account the user intended to use; if none exists, contact Rise-X support (§0) |
| OAuth succeeds but every tool call 401s / 403s | Token is for the wrong environment (test vs. prod), or the account has no tenant/ecosystem access | Re-check which server the user authenticated (`rise-x-test` vs `rise-x`); if tenant access is the issue, contact Rise-X support |
| `whoami` / `get_active_ecosystem` returns identity but no ecosystem | Account exists but isn't attached to a tenant | Contact Rise-X support — this isn't something Claude Code can fix locally |
| Works on `rise-x-test` but not `rise-x` (or vice versa) | Only one server was authenticated | Repeat the auth flow (§3/§5) for the other server |
| Tool calls succeed but return data the user doesn't recognize | Connected to the wrong ecosystem/workspace within a tenant that has several | Use `get_active_ecosystem` / `list_ecosystems` (from the main `rise-x-mcp` skill) to confirm and switch |

**Which environment to use:** default to `rise-x-test` for anything exploratory — building a new workflow, trying out a component, testing an integration. Only use `rise-x` (production) once you deliberately mean to change real ecosystem data.

**Where to get help:**
- Plugin bugs (tool errors, this skill being wrong, packaging issues) → open an issue in this repository.
- Account, tenant, or authentication problems → Rise-X support at https://rise-x.io.
