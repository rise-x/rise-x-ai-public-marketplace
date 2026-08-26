# Design phase — interview, integration, HTML mock

Run this phase before implementing **any UI-visible change**: every new app,
and every new feature, redesign, or design-system upgrade in an existing app.
For a new app run it in full; for a feature or upgrade, scale it down —
confirm the problem and the affected screens, then mock exactly those screens.
The gate is the same either way: **no implementation before the user
explicitly approves the mock**.

Which sections apply:

| You are | Read |
| --- | --- |
| Building a new app | All of it, in order: 1 interview, 2 mobile level, 3 integration, 4 mock. |
| Adding a feature or redesigning a screen | 1 (confirm the problem and the affected screens only) and 4. Skip 2 unless the feature is mobile-specific, and skip 3 unless the feature needs a flow, asset or agent that doesn't exist yet. |

For a new app the output is: a problem statement the user confirmed, a list
of integration targets (origin ids), an **explicitly approved** HTML design
mock, and the material for the app's `APP.md` (personas + user journeys).
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

- **Yes, existing ones** — discover the exact targets and record their
  **origin ids** (`flowOriginId`, asset-type origin id, agent id); those go
  into the app config at implementation time. Discover via the Rise-X MCP
  (`list_flows`, `list_asset_types`, `list_agents`) or ask the user to point
  at them.
- **Nothing suitable exists yet** — ask whether to **build the flow / assets /
  agent first using the Rise-X MCP**, so the app has something real to
  integrate with, then come back here. For an agent, create the configuration
  with the agent-management tools (`create_agent` — the `rise-x-mcp` skill's
  managing-agents reference covers it): the returned `id` **is** the agent id
  the app integrates against — record it.
- **No integration** — fine; note it and move on.

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
  as-is. Version the output as `design/<app>.v<version>.html`, relative to the
  app directory once one exists — before scaffolding, put it wherever the user
  is working and move it into the app's `design/` at scaffold time. Keep it
  out of `src/`.
- **Inline the compiled design-system stylesheet** — `build/ui/styles.css`
  in the SDK (Inter font embedded, Tailwind preflight included): from
  `node_modules/@rise-x/apps-sdk/build/ui/styles.css`, or
  `packages/apps-sdk/build/ui/styles.css` in the rise-x-app monorepo
  (rebuild apps-sdk there if the design system changed). The preflight is what
  gives the mock `box-sizing: border-box`, which a real host would provide —
  don't add a reset of your own. `demo.html` inlines this same file, so markup
  copied out of it renders in your mock exactly as it renders there.
- **Copy markup from the design-system demo** — `build/ui/demo.html` in the
  SDK (`node_modules/@rise-x/apps-sdk/build/ui/demo.html`, or
  `packages/apps-sdk/build/ui/demo.html` in the rise-x-app monorepo): every
  `@rise-x/ui` component pre-rendered with its real markup and classnames,
  plus six complete screens (app layout, AI chat, dashboard, chart gallery,
  mobile, activity log) as tabbed panels. Self-contained — its own stylesheet is
  inlined (that one is for viewing the page; your mock still inlines
  `styles.css`). It's the ground truth for rendered HTML — copy markup
  straight from it rather than reconstructing it by hand.
- **Navigate demo.html by its TOC, never by scanning.** The file opens with a
  navigation guide and a JSON table of contents
  (`<script type="application/json" id="demo-toc">`) listing every section,
  every screen, and the `data-slot` components each section demonstrates —
  read the first ~80 lines to get it. The markup below is pretty-printed, so
  pick the section from the TOC, then grep `data-section="<id>"` (a screen:
  `data-screen-panel="<id>"`) and read from the matched line.
  **Find the section first; a bare `data-slot` grep lands on the wrong
  element.** The first `data-slot="button"` in the file is the demo page's own
  theme toggle, and the first `data-variant="default"` is a badge, not a
  button. The TOC lists the slots each section demonstrates — that is what it
  is for.
- **Take variant classes from `classes.json`, never splice them.** For a
  combination the demo doesn't happen to render — a `default` button at size
  `sm`, say — read `build/ui/classes.json`
  (`node_modules/@rise-x/apps-sdk/build/ui/classes.json`;
  `packages/apps-sdk/build/ui/classes.json` in the rise-x-app monorepo): every
  variant of every variant-bearing component, straight from its own cva
  function and merged the way the component merges it. Each entry names its
  `dimensions` and the keys join those values in order, so a primary small
  button is `button.classes["default|sm"]`. Entries are named after the cva
  function rather than the file it lives in, so look for `trend` and
  `confidence`, not `statistic` and `ai`. Assembling one yourself out of a
  base fragment from one specimen and a size fragment from another produces
  markup that renders subtly wrong.
- **Fall back to the component types** for props or variants the demo
  doesn't exercise
  (`node_modules/@rise-x/apps-sdk/build/ui/components/<name>.d.ts`;
  `packages/ui/src/components/*` sources in the rise-x-app monorepo). The
  stylesheet contains utilities for every class the design system uses, so
  copied markup renders pixel-identical to the implemented app either way.
  Never hand-roll a look-alike of a component or restyle one.
- **Layout glue may be plain CSS** in the mock's own `<style>` block (page
  scaffolding, columns, spacing): the stylesheet ships the utilities the design
  system and the demo page use, not every utility that exists, so one you
  invent for the mock has no rule — write the layout as small custom CSS
  instead, and keep it to layout. Media-query utilities (`md:`) are worse than
  useless in a mock: they answer to the browser window, so a phone-width frame
  on a desktop screen still gets desktop styles. `AppFrame` does not have that
  problem — it is a CSS container and measures itself — so a mock frame sized
  like a phone renders the real mobile layout.
- **Dark/light**: tokens switch on the `dark` class on `<html>` — review both
  themes (a tiny inline toggle script in the mock is fine).
- **Show the chosen mobile UX level.** For "distinct experiences" and
  "native-feel adaptation", include a mobile-width frame alongside the desktop
  one, so the user approves both. The demo's **mobile** screen
  (`data-screen-panel="mobile"`) is the ground truth for it: a real
  `AppFrame mobileNav="tabs"` at phone width, with the bottom tab bar, a
  bottom sheet and thumb-sized rows.

Component inventory (see them rendered in `build/ui/demo.html`; for
props/variants read `node_modules/@rise-x/apps-sdk/build/ui/components/*.d.ts`
— or the `packages/ui/src/components/` sources in the rise-x-app monorepo):
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
  navigation above every app. On mobile widths the native pattern is a
  **bottom tab bar**, and the rail renders one for you: set
  `mobileNav="tabs"` on `AppFrame` (the scaffold already does). Past five
  destinations it keeps four and folds the rest behind **More** on its own.
  Never hand-roll a tab bar — the no-top-bar rule holds at every width.
- **No user/account UI anywhere in the app** — no avatar, no user name, no
  ecosystem label, no sign-out. The host top bar owns identity, in both the
  old and the new host.
- Uphold the experience principles (`references/experience-principles.md`): calm
  over flashy, hierarchy over decoration, data-first, progressive disclosure;
  empty, loading, error and wrong states get the same care as the happy path;
  keyboard- and screen-reader-friendly structure.

### Check the mock before you show it

A screenshot scaled down to fit a pane cannot tell 0px of padding from 56px,
and a control 22px wider than its container looks fine at that size. Before
each iteration goes to the user, measure a handful of computed values in the
browser and read the numbers:

- `getComputedStyle(el).boxSizing` on any input inside a padded box — it must
  be `border-box`, and the input's width must equal the container's inner
  width. If it doesn't, the stylesheet didn't inline.
- `paddingTop` / `paddingBottom` on a component you copied whole, against
  what the demo shows for it.
- The rendered height of anything you gave a size class to. Zero means the
  class has no rule.

Three failure modes hide behind a plausible screenshot, and each is invisible
until measured: a class you invented that the stylesheet has no rule for, a
component copied with a padding that renders as nothing, and a full-width
control overflowing its parent. Grepping every class in the mock against
`styles.css` catches the first before the browser does.

When approved, move to `references/build.md` — carry the approved mock, the
screen list, the integration origin ids, and the persona/journey material
(for `APP.md`) into implementation.
