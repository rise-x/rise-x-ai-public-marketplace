# Design phase — interview, integration, HTML mock

Run this phase before implementing **any UI-visible change**: every new app,
and every new feature, redesign, or design-system upgrade in an existing app.
For a new app run it in full; for a feature or upgrade, scale it down —
confirm the problem and the affected screens, then mock exactly those screens.
The gate is the same either way: **no implementation before the user
explicitly approves the mock**.

For a new app the output is: a problem statement the user confirmed, a list
of integration targets (per-environment origin ids, ready for
`rise-x-app.json`), an **explicitly approved** HTML design mock, and the
material for the app's `APP.md` (personas + user journeys).
For changes to an existing app, the output is the approved mock plus the
`APP.md` updates the change implies.

**Before designing anything, read `references/experience-principles.md`** —
the Rise-X experience language: design direction, visual and interaction
principles, the AI-state grammar, and the definition of success. The mock and
the implemented app are both held to it.

## 1. Interview the user

Understand the problem before proposing anything:

1. **Problem first.** What problem does the app solve, for whom, and what does
   a good outcome look like? Reflect it back in one or two sentences and get
   confirmation before drilling down.
2. **Then targeted questions about the gaps** — only what the user hasn't
   already covered: screens and navigation, data in and out, roles and
   permissions, empty/loading/error states, volumes, mobile vs desktop.
   A few questions at a time, conversational — never a questionnaire dump.

Capture the personas and their journeys as you go — they become the app's
`APP.md` at scaffold time (see `references/build.md`). Note each persona's
device context — it drives the mobile UX level below.

## 2. Mobile UX — pick a level deliberately

Most apps deserve a **dedicated mobile UI/UX** — an iPhone/iPad-app feel, not
a squished desktop layout. Derive the level from the personas and their
devices: when the journeys make it obvious, decide and **suggest it with the
reasoning**; when they don't, ask the user.

| Level | When | What it means |
| --- | --- | --- |
| Distinct experiences | Different personas do different jobs on different devices | Form-factor-specific surfaces. Example: an item-identification app where warehouse users on iPhone/iPad get a scan-first interface (camera front and centre, big touch targets, minimal chrome) while desktop operators see the data-dense view of the same records. |
| Native-feel adaptation | Same jobs on every device | Same screens, mobile-native presentation: bottom tab-bar navigation, sheets instead of dialogs, touch-sized controls, single-column flows. |
| Desktop-first | Mobile use is incidental | Responsive degradation only — still verify nothing breaks at small widths. |

Record the chosen level and each persona's device context in the journeys —
they land in `APP.md` and shape the mock below.

## 3. Integration check

Ask whether the app should integrate with existing **flows (workflows), assets,
or agents**:

- **Yes, existing ones** — discover the exact targets and record their ids
  (`flowOriginId`, asset-type origin id, agent id). Discover via the Rise-X
  MCP (`list_flows`, `list_asset_types`, `list_agents`) or ask the user to
  point at them. All three land in the app's `rise-x-app.json` at scaffold
  time — one alias per target, with a `label` and `description`, and ids
  **per environment**: the same flow has a different origin id on each
  environment, so run the discovery on every environment the app ships to
  (per MCP server — `rise-x-test` → test, `rise-x` → production; starting
  with test only and adding prod later is fine). An agent is declared with
  `kind: "agent"` and its id field is `agentId`, not `flowOriginId`; an agent
  is created per ecosystem with no promotion path, so its id always differs
  between environments and the per-environment block is mandatory. See
  `references/build.md` §App dependencies.
- **Nothing suitable exists yet** — ask whether to **build the flow / assets /
  agent first using the Rise-X MCP**, so the app has something real to
  integrate with, then come back here. For an agent, create the configuration
  with the agent-management tools (`create_agent` — the `rise-x-mcp` skill's
  managing-agents reference covers it): the returned `id` **is** the agent id
  the app integrates against — record it, per environment.
- **No integration** — fine; note it and move on.

**What an agent dependency does and doesn't buy.** Declaring it records the
agent for ecosystem management and Ask Diana, and gives app code a bound
surface (`deps.<alias>.agent.run()` and friends). It does **not** let Diana
invoke the agent: there is no MCP tool that runs or spawns an agent, so the
agent only runs when the app's own code calls it. Don't promise the user
Diana-driven agent runs.

If the Rise-X MCP isn't available (the `rise-x-mcp` plugin isn't installed or
its servers aren't connected), don't invent ids: either ask the user to
connect the MCP, or have them create the flow / asset / agent in the Rise-X
app and paste the resulting id back here.

Before any Rise-X MCP call, load the `rise-x-mcp` skill — it is mandatory and
covers server selection, ecosystems, and routing.

## 4. Design mock — static HTML, iterate to approval

Produce a static HTML mock and present it in the browser. Iterate until the
user **explicitly approves** — no scaffolding or implementation before that
approval.

**Understand the canonical layout first**: read `template/src/App.tsx` in the
SDK (`node_modules/@rise-x/apps-sdk/template/src/App.tsx`;
`packages/apps-sdk/template/src/App.tsx` in the rise-x-app monorepo; in a
scaffolded app it became `src/App.tsx`). It encodes the chrome contract — left rail from the
Nav primitives on the page background, PageHeader + content screens, no user
UI — and the mock follows the same composition.

How to build the mock:

- **One self-contained HTML file per iteration.** Everything inlined — no
  external links, no dev server; the file opens from disk and is shareable
  as-is. Version the output (`design/<app>.v<version>.html`); keep it out of
  `src/`.
- **Inline the compiled design-system stylesheet** — `build/ui/styles.css`
  in the SDK (Inter font is embedded in it): from
  `node_modules/@rise-x/apps-sdk/build/ui/styles.css`, or
  `packages/apps-sdk/build/ui/styles.css` in the rise-x-app monorepo
  (rebuild apps-sdk there if the design system changed).
- **Reuse component markup and class strings** from the design-system source
  (`node_modules/@rise-x/apps-sdk/build/ui/reference/components/*`;
  `packages/ui/src/components/*` in the rise-x-app monorepo; every element
  carries `data-slot`).
  The stylesheet contains utilities for every class used in that source, so
  copied markup renders pixel-identical to the implemented app. Never
  hand-roll a look-alike of a component or restyle one.
- **Layout glue may be plain CSS** in the mock's own `<style>` block (page
  scaffolding, columns, spacing): the stylesheet only ships utilities used
  inside the design-system source, so mock-level utility classes won't apply
  — write the layout as small custom CSS instead, and keep it to layout.
- **Dark/light**: tokens switch on the `dark` class on `<html>` — review both
  themes (a tiny inline toggle script in the mock is fine).
- **Show the chosen mobile UX level.** For "distinct experiences" and
  "native-feel adaptation", include a mobile-width frame alongside the
  desktop one, so the user approves both.

Component inventory (browse
`node_modules/@rise-x/apps-sdk/build/ui/reference/components/` — or
`packages/ui/src/components/` in the rise-x-app monorepo — for
props/variants):
button, input (+numeric/field/action/group/affix), textarea, select, checkbox,
radio, switch, slider, toggle, toggle-group, segmented, label, badge, avatar,
card, table, data-table, list, nav, page-header, empty-state, statistic, chart,
stepper, timeline, progress, skeleton, spinner, dialog, alert-dialog, sheet,
popover, dropdown-menu, tooltip, alert, snackbar, sonner (toasts), message, ai,
calendar, file-upload, scroll-area, resizable, separator.

Design rules (apply to the mock exactly as they apply to the app):

- **Match the canonical layout** in the scaffold template
  (`template/src/App.tsx` in the SDK) — it is the chrome contract in code.
- **App navigation lives on the LEFT** — a rail built from the `Nav` /
  `NavItem` / `NavSection` primitives sitting **directly on the page
  background, never wrapped in a Card or Panel** (cards are for content, not
  chrome). Never a top bar: the host already renders its own top-bar
  navigation above every app. On mobile widths, a **bottom tab bar** is the
  native pattern — the no-top-bar rule holds at every width.
- **No user/account UI anywhere in the app** — no avatar, no user name, no
  ecosystem label, no sign-out. The host top bar owns identity, in both the
  old and the new host.
- Uphold the experience principles (`references/experience-principles.md`): calm
  over flashy, hierarchy over decoration, data-first, progressive disclosure;
  empty, loading, error and wrong states get the same care as the happy path;
  keyboard- and screen-reader-friendly structure.

When approved, move to `references/build.md` — carry the approved mock, the
screen list, the integration targets (per-environment origin ids, destined
for `rise-x-app.json`), and the persona/journey material (for `APP.md`) into
implementation.
