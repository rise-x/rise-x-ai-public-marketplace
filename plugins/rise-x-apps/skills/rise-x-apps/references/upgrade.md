# Upgrading an old app — SDK + design system migration

Apps scaffolded before the current `@rise-x/apps-sdk` predate the design
system and the docs conventions. Recognize them, and **ask before migrating**.

## Spotting an old app

Any of these marks the app as old:

- `webpack.config.js` `shared` has no `@rise-x/ui` entry (often no
  `@tanstack/react-query` either — only the react family).
- UI is not imported from `@rise-x/apps-sdk/ui`: hand-rolled CSS files, an
  own component kit or icon set, another component library.
- Data fetched with hand-rolled `useEffect`/axios instead of the `/query`
  hooks or `/connectors`.
- Flow / asset-type GUIDs hardcoded in source or a hand-rolled config module —
  no `rise-x-app.json` at the app root.
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
2. **Align `webpack.config.js` `shared` with the current template**
   (`node_modules/@rise-x/apps-sdk/template/webpack.config.js`;
   `packages/apps-sdk/template/webpack.config.js` in the rise-x-app
   monorepo): add `@rise-x/ui`
   `{ singleton: true, requiredVersion: false, import: false }` and
   `@tanstack/react-query` `{ singleton: true, requiredVersion: '^5.0.0' }`
   (no `import: false` there — the bundled fallback is deliberate). Leave the
   react-family entries as they are.
3. **Rebuild the UI on `@rise-x/apps-sdk/ui`.** Replace hand-rolled
   CSS/components/icon sets with design-system components (lucide icons ship
   with it); delete what they cover. Move any top-bar navigation to a left
   sidebar/rail (bottom tab bar on mobile). Run the design phase first —
   always (`references/design.md`): mock the migrated screens on the design
   system and get explicit approval before rebuilding. A component-library
   migration changes every screen; it is never a silent swap.
4. **Adopt the data layer.** Replace hand-rolled fetching with `/query` hooks
   in components; `/connectors` in event handlers and lifecycle hooks.
5. **Move origin ids into `rise-x-app.json`** (SDK ≥ 0.10.0). Create the
   manifest at the app root, declare every hardcoded flow / asset-type GUID
   as an aliased dependency (with `label`/`description` and per-environment
   ids), add `new RiseAppManifestPlugin()` (from `@rise-x/apps-sdk/webpack`)
   to `webpack.config.js` plugins as the current template does, and replace
   the GUID constants with `useAppDependencies()` reads
   (`references/build.md` §App dependencies).
6. **Add the docs the template now ships:** `APP.md` (see above), `AGENTS.md`,
   and the one-line `CLAUDE.md` pointer — copy the shape from the SDK's
   `template/` directory (`node_modules/@rise-x/apps-sdk/template/`;
   `packages/apps-sdk/template/` in the rise-x-app monorepo).
7. **Verify and deploy** per `references/build.md`: `pnpm build` produces
   `dist/remoteEntry.js` (with `APP_ENV` matching the deploy target), then
   the deploy question (test environment by default).
