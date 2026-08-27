# Upgrading an old app — SDK + design system migration

Apps scaffolded before the current `@rise-x/apps-sdk` predate the design
system and the docs conventions. Recognize them, and **ask before migrating**.

## Spotting an old app

Any of these marks the app as old:

- A `webpack.config.js` at the app root. Current apps have a 17-line
  `rsbuild.config.mts` instead, and the Module Federation contract lives in
  `@rise-x/apps-sdk/rsbuild`, not in the app. An older still-on-webpack app whose
  `shared` block has no `@rise-x/ui` entry (often no `@tanstack/react-query`
  either, only the react family) predates the design system as well.
- UI is not imported from `@rise-x/apps-sdk/ui`: hand-rolled CSS files, an
  own component kit or icon set, another component library.
- Data fetched with hand-rolled `useEffect`/axios instead of the `/query`
  hooks or `/connectors`.
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

1. **Bump the SDK.** `@rise-x/apps-sdk` to `workspace:*` in-repo (latest npm
   outside), then `pnpm install`.
2. **Move the build onto the preset.** Delete `webpack.config.js` and
   `webpack.local.config.js`, and add an `rsbuild.config.mts` — copy the shape
   from the SDK's `template/rsbuild.config.mts`
   (`node_modules/@rise-x/apps-sdk/template/`; `packages/apps-sdk/template/` in
   the rise-x-app monorepo):

   ```ts
   import { defineConfig } from '@rsbuild/core';
   import { defineAppConfig } from '@rise-x/apps-sdk/rsbuild';
   import pkg from './package.json';

   export default defineConfig(({ envMode }) =>
     defineAppConfig({ pkg, port: <this app's port>, standalone: envMode === 'standalone' }),
   );
   ```

   Then swap the build dependencies and scripts: drop webpack and its loaders,
   add `@rsbuild/core`, `@rsbuild/plugin-react` and `@rsbuild/plugin-type-check`
   as devDependencies, and set `build: rsbuild build`,
   `start: rsbuild dev --env-mode standalone`, `start:federated: rsbuild dev`.

   **There is no `shared` block to align any more.** The preset owns the whole
   Module Federation contract — the MF scope (derived from the package name,
   `@rise-x-apps/vendor-hub` -> `app_vendor_hub`), `remoteEntry.js`, the
   `./App` + `./lifecycle` exposes, and every share including `@rise-x/ui` and
   `@tanstack/react-query`. You upgrade the contract by upgrading the SDK. An app
   can pass only `exposes` (merged over the defaults) and `define`; it cannot add
   shares, so an app that needed a custom one is a conversation with the SDK, not
   a local edit. `.mts` rather than `.ts` is deliberate: it marks the file ESM,
   which a plain `.ts` warns about on every build, and `"type": "module"` is not
   an alternative because it breaks `commitlint.config.js`.
3. **Rebuild the UI on `@rise-x/apps-sdk/ui`.** Replace hand-rolled
   CSS/components/icon sets with design-system components (lucide icons ship
   with it); delete what they cover. Move any top-bar navigation to a left
   sidebar/rail (bottom tab bar on mobile — on SDK >= 0.10.0 that is
   `mobileNav="tabs"` on `AppFrame`, not a hand-built bar; see
   `references/build.md`). Run the design phase first —
   always (`references/design.md`): mock the migrated screens on the design
   system and get explicit approval before rebuilding. A component-library
   migration changes every screen; it is never a silent swap.
4. **Adopt the data layer.** Replace hand-rolled fetching with `/query` hooks
   in components; `/connectors` in event handlers and lifecycle hooks.
5. **Add the docs the template now ships:** `APP.md` (see above), `AGENTS.md`,
   and the one-line `CLAUDE.md` pointer — copy the shape from the SDK's
   `template/` directory (`node_modules/@rise-x/apps-sdk/template/`;
   `packages/apps-sdk/template/` in the rise-x-app monorepo).
6. **Verify and deploy** per `references/build.md`: `pnpm build` produces
   `dist/remoteEntry.js`, then the deploy question (test environment by
   default).
