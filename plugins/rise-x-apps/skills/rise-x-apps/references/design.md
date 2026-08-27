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
| Adding a feature or redesigning a screen | 1 (confirm the problem and the affected screens only) and 4. Skip 2 and take the app's mobile UX level from its `APP.md`, unless the feature is mobile-specific. Skip 3 unless the feature needs a flow, asset or agent that doesn't exist yet. |
| Migrating an app to the design system (`references/upgrade.md`) | 1 (what the screens do today and what is wrong with them) and 4, one mock per screen you migrate. Mobile level from `APP.md`; if the app has none, decide it with §2 and write it down. |

`references/experience-principles.md` is not one of the numbered sections and
is never skipped, on any of those paths.

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

From **0.10.0** that composition is three primitives out of
`@rise-x/apps-sdk/ui`, and the rest of this section names them:

| Primitive | What it is |
| --- | --- |
| `AppFrame` | The app's whole region. A CSS container, so every breakpoint below answers to the width the app was given rather than to the browser window, and a phone-sized frame renders the phone layout even on a desktop screen. |
| `AppRail` | The left nav rail. With `mobileNav="tabs"` on the frame it becomes a bottom tab bar once the frame is narrow. |
| `AppContent` | The one scrolling column. |

Props are in `node_modules/@rise-x/apps-sdk/build/ui/components/app-frame.d.ts`.
Before 0.10.0 none of them exist and the template hand-builds an `<aside>` rail
instead.

Every `build/ui/...` path below is **inside the SDK package**, not your app:
prefix it with `node_modules/@rise-x/apps-sdk/`, or with `packages/apps-sdk/`
in the rise-x-app monorepo.

**Version applicability.** `demo.html`, `classes.json`, the Tailwind preflight
inside `styles.css`, and `AppFrame` all ship from
**`@rise-x/apps-sdk` 0.10.0**. Check what the project actually has before
relying on any of them:

```bash
node -p "require('./node_modules/@rise-x/apps-sdk/package.json').version"
```

The relative path is deliberate: the bare specifier
(`@rise-x/apps-sdk/package.json`) throws `ERR_PACKAGE_PATH_NOT_EXPORTED` on
every SDK before 0.10.0, because the package's `exports` map has no entry for
it. The failure reads like a broken install rather than an old version.

On an older SDK: read markup from
`node_modules/@rise-x/apps-sdk/build/ui/reference/components/*.tsx` (the
component sources, shipped through 0.9.0), give the mock its own
`box-sizing` reset, and hand-build the mobile tab bar. Everything below marked
**0.10.0+** does not apply. Upgrading the SDK is usually the better move.

How to build the mock:

- **One self-contained HTML file per iteration.** Everything inlined — no
  external links, no dev server; the file opens from disk and is shareable
  as-is. Version the output as `design/<app>.v<n>.html`, relative to the
  app directory once one exists — before scaffolding, put it wherever the user
  is working and move it into the app's `design/` at scaffold time. Keep it
  out of `src/`.
- **Inline the compiled design-system stylesheet** — `build/ui/styles.css`
  in the SDK, from `node_modules/@rise-x/apps-sdk/build/ui/styles.css`, or
  `packages/apps-sdk/build/ui/styles.css` in the rise-x-app monorepo (rebuild
  apps-sdk there if the design system changed). Inter is embedded, so this one
  file is the mock's only dependency.
  **0.10.0+:** it carries Tailwind preflight, which is what gives the mock
  `box-sizing: border-box` the way a real host would — don't add a reset of
  your own. Before 0.10.0 there is no preflight: put
  `*,::before,::after{box-sizing:border-box}` in the mock's own `<style>`
  block, or every padded `w-full` control renders wider than its container.
- **Copy markup from the design-system demo** (**0.10.0+**) —
  `build/ui/demo.html` in the SDK
  (`node_modules/@rise-x/apps-sdk/build/ui/demo.html`, or
  `packages/apps-sdk/build/ui/demo.html` in the rise-x-app monorepo): every
  `@rise-x/ui` component pre-rendered with its real markup and classnames,
  plus a set of complete screens as tabbed panels. The TOC names the screens;
  don't assume a list. The page is self-contained **because it inlines
  `styles.css`** — the same file your mock inlines, and that identity is the
  whole point: markup copied out of the demo renders in your mock exactly as
  it renders there. It's the ground truth for rendered HTML, so copy from it
  rather than reconstructing by hand.
- **Navigate demo.html by its TOC, never by scanning** (**0.10.0+**). The file opens with a
  navigation guide and a JSON table of contents
  (`<script type="application/json" id="demo-toc">`) listing every section,
  every screen, and the `data-slot` components each section demonstrates —
  read the file down to that script's closing `</script>` to get it. The markup below is pretty-printed, so
  pick the section from the TOC, then grep `data-section="<id>"` (a screen:
  `data-screen-panel="<id>"`) and read from the matched line.
  **Find the section first; a bare `data-slot` grep lands on the wrong
  element.** The first `data-slot="button"` in the file is the demo page's own
  theme toggle, and the first `data-variant="default"` is a badge, not a
  button. The TOC lists the slots each section demonstrates — that is what it
  is for.
- **Take variant classes from `classes.json`, never splice them** (**0.10.0+**). For a
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
  `packages/ui/src/components/*` sources in the rise-x-app monorepo). Through
  0.9.0, `node_modules/@rise-x/apps-sdk/build/ui/reference/components/<name>.tsx`
  ships the component source itself and is the only markup you can read. The stylesheet contains
  utilities for every class the design system uses, so copied markup renders
  pixel-identical to the implemented app either way. Never hand-roll a
  look-alike of a component or restyle one.
- **Layout glue may be plain CSS** in the mock's own `<style>` block (page
  scaffolding, columns, spacing): the stylesheet ships the utilities the design
  system and the demo page use, not every utility that exists, so one you
  invent for the mock has no rule — write the layout as small custom CSS
  instead, and keep it to layout. Media-query utilities (`md:`) are worse than
  useless in a mock: they answer to the browser window, so a phone-width frame
  on a desktop screen still gets desktop styles. `AppFrame` (**0.10.0+**) does not have
  that problem — it is a CSS container and measures itself — so a mock frame
  sized like a phone renders the real mobile layout.
- **Dark/light**: tokens switch on the `dark` class on `<html>` — review both
  themes (a tiny inline toggle script in the mock is fine).
- **Show the chosen mobile UX level.** For "distinct experiences" and
  "native-feel adaptation", include a mobile-width frame alongside the desktop
  one, so the user approves both. **0.10.0+:** the demo's **mobile** screen
  (`data-screen-panel="mobile"`) is the ground truth for it — a real
  `AppFrame mobileNav="tabs"` at phone width, with the bottom tab bar, a
  bottom sheet and thumb-sized rows. Copy that markup rather than inventing a
  phone layout.

Component inventory (**0.10.0+** for the rendered specimens in
`node_modules/@rise-x/apps-sdk/build/ui/demo.html`, and for `app-frame` itself; for
props/variants read `node_modules/@rise-x/apps-sdk/build/ui/components/*.d.ts`
— or the `packages/ui/src/components/` sources in the rise-x-app monorepo):
button, input (+numeric/field/action/group/affix), textarea, select, checkbox,
radio, switch, slider, toggle, toggle-group, segmented, label, badge, avatar,
card, table, data-table, list, nav, app-frame (0.10.0+), page-header,
empty-state, statistic, chart,
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
  **bottom tab bar**, and the no-top-bar rule holds at every width.
  **0.10.0+:** the rail renders that bar for you — the app sets
  `mobileNav="tabs"` on `AppFrame`, which the scaffold template does, and it
  folds the overflow behind **More** on its own once there are too many
  destinations. The SDK README's AppFrame section owns that threshold; don't
  restate it. A mock has no props, so copy the rendered bar out of
  `data-screen-panel="mobile"` instead.
  Before 0.10.0 there is no such rail and the bar has to be built by hand.
- **No user/account UI anywhere in the app** — no avatar, no user name, no
  ecosystem label, no sign-out. The host top bar owns identity, in both the
  old and the new host.
- Uphold the experience principles (`references/experience-principles.md`): calm
  over flashy, hierarchy over decoration, data-first, progressive disclosure;
  empty, loading, error and wrong states get the same care as the happy path;
  keyboard- and screen-reader-friendly structure.

### Check the mock before you show it

A screenshot scaled down to fit a pane cannot tell 0px of padding from 56px,
and a control 22px wider than its container looks fine at that size. Three
failure modes hide behind a plausible screenshot: a class you invented that
the stylesheet has no rule for, a component copied with a padding that
renders as nothing, and a full-width control overflowing its parent. Two
checks catch them, in this order.

**1. Every class has a rule.** Mechanical, no browser. Save this as
`audit-mock.js` and run it against your mock and the stylesheet you inlined:

```js
const fs = require('fs');
const [mock, css] = process.argv.slice(2).map((f) => fs.readFileSync(f, 'utf8'));
const SPECIAL = ".[]()/%:#!'\"=&>+*~$^|@,";
const MARKER = /^(group\/|peer\/|lucide(-|$)|recharts-)/; // markers, never styled
const decode = (v) => v.replace(/&quot;/g, '"').replace(/&#39;/g, "'")
  .replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&');
const styled = (t) => {
  const sel = '.' + [...t].map((c) => (SPECIAL.includes(c) ? '\\' + c : c)).join('');
  for (let i = css.indexOf(sel); i !== -1; i = css.indexOf(sel, i + 1)) {
    const next = css[i + sel.length] || '';
    if (!/[\w-]/.test(next) && next !== '\\') return true; // the selector ends here
  }
  return false;
};
const used = new Set();
for (const [, v] of mock.matchAll(/class=["']([^"']*)["']/g))
  for (const t of decode(v).split(/\s+/)) if (t && !MARKER.test(t)) used.add(t);
// Parsing nothing must not read as passing: single-quoted markup, an empty
// file and swapped arguments all land here.
if (!used.size) {
  console.error('no class attributes found in ' + process.argv[2]);
  process.exit(2);
}
const missing = [...used].filter((t) => !styled(t));
process.exitCode = missing.length ? 1 : 0;
console.log(missing.length ? 'NO RULE: ' + missing.sort().join(', ') : 'every class has a rule');
```

```bash
node audit-mock.js design/<app>.v1.html node_modules/@rise-x/apps-sdk/build/ui/styles.css
```

Anything it names renders unstyled. Either it is a utility you invented, which
belongs in the mock's own `<style>` block as plain CSS, or you mistyped a
class you copied. Two exceptions are expected and fine: markup copied from the
demo's map or Ask Diana specimens is styled by leaflet's own sheet and by the
host's globals, so it reports unstyled there too.

**2. Measure, don't look.** Open the mock in the browser and read computed
values rather than judging a scaled screenshot:

```js
const input = document.querySelector('input');
getComputedStyle(input).boxSizing;                  // see below
input.getBoundingClientRect().width;                // must equal the container's inner width

const copied = document.querySelector('[data-slot="empty-state"]'); // any component you copied whole
getComputedStyle(copied)?.paddingTop;               // against what the demo shows for it
copied?.getBoundingClientRect().height;             // 0 means a size class on it has no rule
```

`boxSizing` reads `border-box` on **0.10.0+**, where the stylesheet carries
preflight. On an older SDK it reads `content-box` even when the stylesheet
inlined perfectly — that is the missing preflight, not a missing stylesheet,
and the fix is the reset in the mock's own `<style>` block, not re-inlining.

When approved, move to `references/build.md` — carry the approved mock, the
screen list, the integration targets (per-environment origin ids, destined
for `rise-x-app.json`), and the persona/journey material (for `APP.md`) into
implementation.
