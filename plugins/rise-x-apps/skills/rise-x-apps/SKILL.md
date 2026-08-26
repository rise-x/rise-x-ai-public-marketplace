---
name: rise-x-apps
description: Create a Rise-X (Diana) federated app end-to-end - design interview, single-file HTML design mock on the Rise-X design system, scaffold (standalone in your own project, or under apps/ in the rise-x-app monorepo), implement with @rise-x/apps-sdk, deploy via the Rise-X MCP (test environment by default). Also use for app code in an existing app - shell hooks/accessors, connectors, query hooks, @rise-x/apps-sdk/ui components, lifecycle hooks. TRIGGERS on "create/design/build a Rise app", "scaffold an app", "deploy the app", and apps-sdk usage in app code. DO NOT TRIGGER when editing the @rise-x/apps-sdk package itself (packages/apps-sdk/ in the rise-x-app monorepo).
---

# rise-x-apps

Rise-X apps are independent React bundles loaded into the Diana shell at
runtime via Webpack Module Federation. The `@rise-x/apps-sdk` package (public
npm) provides the SDK and a scaffolder CLI. Apps are built in two contexts:
**standalone** in your own project (the common case), or under `apps/*`
inside the `rise-x-app` monorepo (Rise-X internal).

**Canonical doc:** the SDK README — `node_modules/@rise-x/apps-sdk/README.md`
in a scaffolded app; `packages/apps-sdk/README.md` in the rise-x-app
monorepo. Don't duplicate it; reference it when the user wants depth.

## The workflow — route by intent

A **new app** goes through four phases, in order. Read the reference for the
phase you're in *before* acting.

| Phase | What happens | Read |
| --- | --- | --- |
| 1. Design | Interview the user: understand the problem first, then ask targeted questions about the gaps. Pick the mobile UX level from the personas' devices. Build an HTML design mock on the Rise-X design system (`@rise-x/apps-sdk/ui`) and iterate until **explicit approval**. | `references/design.md` |
| 2. Integrate | Ask whether the app connects to existing flows/assets/agents; if nothing exists, offer to build them via the Rise-X MCP first (agents: `create_agent` — its returned id is the agent id the app uses). Without the MCP, the user creates them in the Rise-X app and supplies the ids, or connects the MCP. Record origin ids. | `references/design.md` |
| 3. Implement | Scaffold with the CLI, write app code on `@rise-x/apps-sdk`. | `references/build.md` |
| 4. Deploy | Ask whether to deploy, then deploy via the Rise-X MCP — **test environment unless the user names another**. | `references/build.md` §Build and deploy |

Existing-app work skips straight to the matching phase:

| Intent | Read |
| --- | --- |
| "Add a feature to app <x>" — anything that adds or changes UI | `references/design.md` first (mock → approval), then `references/build.md` |
| Shell hooks / connectors / query layer / lifecycle hooks — no UI change | `references/build.md` |
| "Redesign screen X" / "what should this look like" / migrate to the design system | `references/design.md` (mock → approval), then `references/build.md` |
| The app is OLD — no `@rise-x/ui` in webpack `shared`, hand-rolled UI, no `APP.md` | `references/upgrade.md` — ask about migrating before changing it; the migration itself goes mock-first |
| "Build/deploy the app" | `references/build.md` §Build and deploy |
| Changing the `@rise-x/apps-sdk` package itself | Not this skill — that's SDK development inside the rise-x-app monorepo (see its root `AGENTS.md`). |

## Hard rules (apply in every phase)

1. **No implementation before design approval.** Every change that adds or alters UI — a new app, a new feature in an existing app, a redesign, or a design-system upgrade — starts with an HTML design mock the user has **explicitly approved**. Only changes with no UI surface (pure logic, data wiring, fixes that don't change what's rendered) skip the mock.
2. **App UI MUST be built exclusively from `@rise-x/apps-sdk/ui`** (the `@rise-x/ui` design system). No hand-rolled markup for covered primitives, no other component libraries, no one-off styles. Design mocks carry the same requirement: one self-contained HTML file with the design-system stylesheet from `@rise-x/apps-sdk` inlined.
3. **The host owns the chrome.** App navigation is a LEFT rail built from the `Nav` primitives directly on the page background — never inside a Card, never a top bar (the host renders its own). No user/account UI in the app, ever — the host top bar owns identity. On mobile widths a bottom tab bar is the native pattern; from `@rise-x/apps-sdk` 0.9.0 the rail renders one for you — set `mobileNav="tabs"` on `AppFrame` (the scaffold does) rather than hand-rolling one. The canonical layout is the scaffold template's `App.tsx` (in a scaffolded app: its own `src/App.tsx`; the pristine template ships at `node_modules/@rise-x/apps-sdk/template/src/App.tsx`) — **read it first, and preserve its composition**.
4. **Mobile UX is a deliberate choice, not an afterthought.** Most apps deserve a dedicated mobile experience; pick the level (distinct experiences / native-feel adaptation / desktop-first) from the personas' devices — suggest with reasoning, or ask when unclear (`references/design.md` §2).
5. **Follow the Rise-X experience principles** — read `references/experience-principles.md` before any design or UI work: visual language, interaction rules, AI-state grammar, and the same care for empty/loading/error states as the happy path.
6. **Deploys are asked-for, and default to test.** Never deploy unprompted; when the user says deploy without naming an environment, use test (`rise-x-test` MCP server).
7. **Load the `rise-x-mcp` skill before any Rise-X MCP call** — discovery, flow/asset building, and deploys alike. It ships in the `rise-x-mcp` plugin from this marketplace — if it isn't installed, ask the user to install it before MCP work.
8. **The contract is `@rise-x/apps-sdk`** — never import shell internals, never bundle your own React (full Don'ts list in `references/build.md`).
9. **`APP.md` is the app's living context.** Create it at scaffold time from the design interview — the problem the app solves, the personas, and their full user journeys. Read it before changing an existing app; update it in the same change whenever behaviour or a journey changes. If an existing app has none, study the app and write it first (`references/upgrade.md` §Missing APP.md).
