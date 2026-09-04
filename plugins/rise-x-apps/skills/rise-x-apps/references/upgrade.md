# Upgrading an old app — build, SDK and design-system migrations

Apps scaffolded before the current `@rise-x/apps-sdk` may be behind on any of
three fronts — the build (still webpack), the design system, and the docs
conventions — and they move independently. Recognize which, and **ask before
migrating**.

## Spotting an old app

Any of these marks the app as old:

- A `webpack.config.js` at the app root. Current apps have a thin
  `rsbuild.config.mts` instead, and the Module Federation contract lives in
  `@rise-x/apps-sdk/rsbuild`, not in the app. An older still-on-webpack app whose
  `shared` block has no `@rise-x/ui` entry (often no `@tanstack/react-query`
  either, only the react family) predates the design system as well.
- UI is not imported from `@rise-x/apps-sdk/ui`: hand-rolled CSS files, an
  own component kit or icon set, another component library.
- Data fetched with hand-rolled `useEffect`/axios instead of the `/query`
  hooks or `/connectors`.
- Flow / asset-type / agent GUIDs hardcoded in source or a hand-rolled config
  module — no `rise-x-app.json` at the app root. `npx @rise-x/apps-sdk scan`
  answers this one for you, without reading the app first (SDK 0.12.0 or later).
- No `APP.md` / `AGENTS.md` at the app root.
- An in-app top-bar navigation (now disallowed — nav belongs on the left).

## Ask before migrating

When the user wants changes to an old app, **ask first** whether to migrate
to the current apps-sdk and design system — before, or together with, the
requested change. Frame the tradeoff in a sentence: migration puts the app on
the shared Rise-X design and modern data layer but widens the diff; skipping
it keeps the change small, though new work still must not add more
hand-rolled UI. Never start a migration unasked.

## Missing APP.md

Old apps have no `APP.md`. Before substantial work, **understand the app and
write one**: read the screens, routes, lifecycle, and integrations (flow /
asset-type / agent ids in the config), infer the personas and journeys the UI
implies, and distill that into `APP.md`. Confirm anything uncertain with the
user rather than guessing.

## Migration checklist

**Three independent migrations share this list — check which ones you're doing.**

| Axis | Steps | Applies when |
| --- | --- | --- |
| build | 1, 2, 6 | the app is still on a webpack config |
| design system + data layer | 3, 4 | hand-rolled UI instead of `@rise-x/apps-sdk/ui`, or `useEffect`/axios instead of the query hooks, or an in-app top bar |
| docs | 5 | `APP.md` / `AGENTS.md` / `CLAUDE.md` are missing |

They move independently. An app already current on the design system and the
query hooks but still on webpack needs the build axis and, if its docs are
missing, the docs axis — not steps 3 and 4, and so no design phase and no
approval gate, because no screen changes. Don't run it through those out of
habit.

1. **Bump the SDK.** `@rise-x/apps-sdk` to `workspace:*` in-repo (latest npm
   outside), then `pnpm install`.
2. **Move the build onto the preset.** Read the app's dev port out of
   `webpack.local.config.js` first (`devServer.port`) — that file goes in this
   step and is the only place the value lives. Then delete `webpack.config.js`
   and `webpack.local.config.js` and add an `rsbuild.config.mts`, copying the
   shape from the SDK's `template/rsbuild.config.mts` (its `port` is a
   scaffolder placeholder — use the port you just read). Swap the build deps and
   scripts: drop webpack and its loaders, add `@rsbuild/core`,
   `@rsbuild/plugin-react` and `@rsbuild/plugin-type-check`, and set
   `build: rsbuild build`, `start: rsbuild dev --env-mode standalone`,
   `start:federated: rsbuild dev`. Keep the extension `.mts`: it marks the file
   ESM, which a plain `.ts` warns about on every build. Do **not** reach for
   `"type": "module"` in `package.json` instead — it breaks
   `commitlint.config.js`. Add `resolveJsonModule` to `tsconfig.json` if
   absent — the config imports `package.json`.

   **There is no `shared` block to align any more:** the preset owns the whole
   Module Federation contract, and you upgrade it by upgrading the SDK. See
   `references/build.md`.
3. **Rebuild the UI on `@rise-x/apps-sdk/ui`.** Replace hand-rolled
   CSS/components/icon sets with design-system components (lucide icons ship
   with it); delete what they cover. Move any top-bar navigation to a left
   sidebar/rail (bottom tab bar on mobile — on SDK >= 0.11.0 that is
   `mobileNav="tabs"` on `AppFrame`, not a hand-built bar; see
   `references/build.md`). Run the design phase first —
   always (`references/design.md`): mock the migrated screens on the design
   system and get explicit approval before rebuilding. A component-library
   migration changes every screen; it is never a silent swap.
4. **Adopt the data layer.** Replace hand-rolled fetching with `/query` hooks
   in components; `/connectors` in event handlers and lifecycle hooks. In an
   offline-capable app, bump `@rise-x/apps-sdk` to >= 0.12 before this step —
   that is where the hooks' offline-cache behaviour lives
   (`references/offline.md` §4). Check that version is available first
   (`npm view @rise-x/apps-sdk version`) — on an older SDK this step lands
   without the offline behaviour.
5. **Move ids into `rise-x-app.json`** (needs the SDK version named in
   `references/build.md` §App dependencies). **Start by finding them — don't
   read the code hunting for GUIDs by hand:**

   ```bash
   npx @rise-x/apps-sdk scan            # every undeclared id, with its kind
   npx @rise-x/apps-sdk scan --json     # same, machine-readable
   ```

   Each finding carries the id, the inferred kind, a suggested alias, and every
   file:line it appears at. Work from that list, and treat an unclear kind as a
   question for the user (or a `list_flows` / `list_asset_types` / `list_agents`
   call), never a guess.

   Then run `npx @rise-x/apps-sdk scan --show-ignored` and read what it set
   aside. It skips step ids, row ids, section ids, integration endpoints and
   sample data by name — right on the apps those rules were written against, and
   not guaranteed on this one. A migration is exactly when a wrongly dismissed
   dependency costs the most, because nothing downstream will notice it.

   Then create the manifest at the app root, declare each real dependency as an
   aliased entry (with `label`/`description` and per-environment ids — an
   agent's id field is `agentId`, not `flowOriginId`), and replace the GUID
   constants with `useAppDependencies()` reads (`references/build.md`
   §App dependencies). Re-run `scan` when you're done: a clean report is the
   evidence the migration is complete, and it is the only check that notices a
   constant you declared but forgot to stop using. For CI,
   `npx @rise-x/apps-sdk scan --strict` exits non-zero on a finding, a skipped
   scan, or an unreadable file, so a green report from plain `scan` is not the
   gate. An app being migrated
   usually has ids for one environment only — the one it was built against — so
   collect the rest before declaring the environment: every dependency needs an
   id for every environment the manifest declares. Finish with
   `npx @rise-x/apps-sdk validate`, the spelling to use before the template's
   `validate` script exists in `package.json`; add that script and the
   `build:<env>` ones while you are in there, and run `pnpm validate` from then
   on.
6. **Add the docs.** `AGENTS.md` and the one-line `CLAUDE.md` pointer can be
   copied from the SDK's `template/` directory
   (`node_modules/@rise-x/apps-sdk/template/`; `packages/apps-sdk/template/` in
   the rise-x-app monorepo, which also ships a `README.md`). `APP.md` is **not**
   in the template — it is per-app and you write it, see *Missing APP.md* above.
   While you are in `package.json`, the template also ships
   `typecheck: tsc --noEmit`; add it if absent. The preset's `pluginTypeCheck`
   only runs inside an rsbuild invocation, so this script is how you type-check
   without doing a full build.
7. **Verify and deploy** per `references/build.md`: `pnpm validate` passes for
   every declared environment, `pnpm build` produces `dist/remoteEntry.js`
   (with `APP_ENV` matching the deploy target), then the deploy question (test
   environment by default).
