# Federated Apps: registry and deployment

A **federated app** is an independent Module Federation bundle that runs inside the Diana shell
as a custom UI. The six app tools deploy bundles and manage the registry; their descriptions
give the arguments, the deploy flow, and the return shapes.

These tools build nothing. **Authoring** an app (scaffolding, `@rise-x/apps-sdk` code,
connectors, the production build and zip) is the `rise-x-apps` skill in the `rise-x-apps`
plugin. Deploying needs the environment-orchestrator role.

## Release recipes

**New app:** `request_bundle_upload` → PUT the zip → `deploy_app` without `app_id`. Record the
returned id; it is the handle for every later call.

**New version of an existing app:** `list_apps` to find the app's `id` and current `version` →
pick a strictly newer semver → the same three steps with `app_id` set.

**Rename, re-describe, or change the icon:** `update_app(app_id, …)` only. Never use it to fake
a release; a new bundle or version goes through `deploy_app`.

**Retire an app:** `delete_app(app_id)`. Redeploying under the same id restores it.

## Staging the bundle

- When your runtime has an `upload_bundle` tool (a sandbox with no network access), it replaces
  steps 1 and 2: call it with the zip's path relative to the workspace root and pass the
  `upload_id` it returns to `deploy_app`. Do not call `request_bundle_upload` or `curl` there.
  With `upload_bundle`, what can expire is the staged upload: when `deploy_app` answers
  `upload_not_found`, call `upload_bundle` again, not `request_bundle_upload` as the hint says.
- A `localhost_public_url` warning from `request_bundle_upload` means the server's
  `MCP_PUBLIC_URL` is the localhost default, so the URL works only when the MCP server runs
  locally.
- A failed `deploy_app` keeps the staged bundle: client-side validation errors and platform
  failures (409 duplicate version, 403 missing role, 404 unknown app id) consume nothing. Fix
  the field and call `deploy_app` again with the **same** `upload_id` within its TTL. Only a
  successful deploy consumes it. The exception is a problem inside the zip, such as a rejected
  dependency manifest: rebuild and restage, because the staged upload holds the old zip.

## Version and scope

- The platform rejects a duplicate version with **409** only when the string equals the app's
  currently live version. It does not compare semver or check history, so an older version
  string is accepted and leaves a confusing registry. Always bump past the live version.
- `app_scope` must equal the Module Federation scope the app was built with, or the manifest
  loads and the app never mounts. For an `@rise-x/apps-sdk` app the scope comes from the package
  name with the npm scope stripped, hyphens turned into underscores, and an `app_` prefix:
  `@rise-x-apps/todo-app` → **`app_todo_app`**. Prefer the `scope` the scaffolder prints in its
  `--json` output over deriving it. The build preset honours an `APP_SCOPE` override, for
  registering two builds of one app side by side.

## Dependency manifest (`rise-x-app.json`)

A bundle may declare the flows, asset types, and agents it uses in a `rise-x-app.json` at the
archive root. The app's build writes it; these tools never author it. It is expected in the
zip; never remove it.

- **Format.** `dependencies` is an object keyed by the dependency's alias (the alias is the key,
  not a field). Each entry has a `kind` (`flow`, `assetType`, or `agent`), an optional
  `label` / `description`, and the id its kind takes: `flowOriginId` for a flow or asset type,
  `agentId` for an agent. Never copy a capitalised `kind` (`Flow`) from a `get_app` result
  into a manifest; the platform rejects it.
- **Built copy vs source.** The app's source `rise-x-app.json` lists `environments` (plural)
  with each environment's ids. The build writes one `environment` and only that environment's
  ids into the zip. A hand-edited source manifest uses `environments`, never `environment`.
- **Wrong-environment bundles deploy clean.** Nothing resolves the ids, so a bundle built for
  test deploys to production, stores test ids, and fails inside the running app. Compare
  `dependencyEnvironment` with the ecosystem you deployed into. It is present when the
  manifest named an `environment`, which the SDK build writes from version 0.12.0. If it is
  wrong, rebuild with the right `APP_ENV`, re-zip, and restage.
- **Empty manifests clear.** From SDK 0.12.0 the scaffolded template ships an empty manifest,
  so redeploying an app whose manifest was never filled clears the dependencies the registry
  had recorded.
- **Older servers.** A server without the dependency-manifest release returns no
  `dependencies` from `get_app` and no `dependencyCount` or `dependencyEnvironment` from
  `deploy_app`. Read that as not supported here, not as the app declaring none. A server with
  the release always returns `dependencyCount`.
- **Declaring an agent does not make it callable from here.** No MCP tool runs an agent; the
  declaration records it for the ecosystem and the app's own code calls it.
