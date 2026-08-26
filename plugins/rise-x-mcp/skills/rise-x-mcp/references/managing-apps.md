# Federated Apps — registry + deployment tools

The MCP server manages **federated apps**: independent Module-Federation bundles that run inside
the Diana shell as custom UIs. Six tools cover the app registry and bundle deployment — you can
release an app end-to-end from a Claude session, no manual zip upload through the shell UI.

> **Scope:** these tools manage *deployment and the registry*. **Authoring** an app (scaffolding,
> `@rise-x/apps-sdk` code, connectors) is covered by the `rise-x-apps` skill in this marketplace's
> `rise-x-apps` plugin — that skill also produces the bundle zip these tools deploy. Requires a
> server with the federated-apps release; deploying requires the
> **environment-orchestrator** role.

## Tool inventory

| Tool | What it does |
|---|---|
| `request_bundle_upload()` | Step 1 of a deploy: returns a one-time `uploadUrl` + `uploadId` for staging the bundle zip |
| `deploy_app(upload_id, name, version, app_scope, app_id?, description?, icon?, feature_flags?)` | Step 3: deploys the staged bundle; new app (GUID generated) or new version of an existing `app_id`. `feature_flags` — optional `dict[str, bool]` of app behaviours, e.g. `{"isOfflineModeEnabled": true}`; echoed back as `featureFlags` (camelCase) on `get_app`/`list_apps` |
| `list_apps()` | Registry listing — `id`, `name`, `version`, `scope`, `remoteUrl`, `lastModified` per app |
| `get_app(app_id)` | Full manifest for one app (incl. `module`, `description`, `icon`, `featureFlags`) |
| `update_app(app_id, …fields)` | Metadata-only read-modify-write; pass just the fields that change. Does NOT touch the bundle. The merge is per-field: `feature_flags`, when passed, replaces the stored flags dictionary wholesale (pass the full dict; `{}` clears it) — omitted, the stored flags carry through unchanged. Known key: `isOfflineModeEnabled` — gates the shell's "Make available offline" card |
| `delete_app(app_id)` | Soft-delete from the registry (bundle blobs cleaned). Redeploying under the same id restores it |

## Deploying a bundle (the three-step flow)

A multi-MB zip can't travel through an MCP tool-call parameter, so the deploy is staged:

```
1. request_bundle_upload
     → uploadUrl   ({MCP_PUBLIC_URL}/apps/upload/{token})
     → uploadId    (keep it for step 3)

2. Upload the zip to the capability URL:
     curl -sS -X PUT --data-binary @my-app-bundle.zip '<uploadUrl>'

3. deploy_app(
     upload_id = <uploadId>,
     name      = "My App",            # human-readable display name
     version   = "1.0.0",             # semver; rejected (409) only if it equals the app's live version — bump every release
     app_scope = "my_app",            # MF scope, snake_case: [a-z][a-z0-9_]*
     app_id    = <GUID>,              # ONLY when releasing a new version of an existing app
   )
     → app id + canonical manifest (remoteUrl, version, scope, deployedAt, sizeBytes)
```

### Bundle requirements

- A zip of the app's **`dist/` folder contents** — `remoteEntry.js` must sit at the **archive
  root** (zip the contents, not the folder), or the shell can't load the remote.
- Size-capped (default 20 MB) — the PUT is rejected above the cap.

### Upload URL semantics

- **Single-use** and short-TTL (default 15 minutes) — one successful PUT consumes the *URL*; the
  staged bundle then waits for `deploy_app`. If the TTL lapses, call `request_bundle_upload`
  again.
- The token in the URL is the credential (bound to your session); staged bytes are inert until
  `deploy_app` forwards them to the platform with *your* bearer token.
- A `localhost_public_url` warning in the step-1 response means the server's `MCP_PUBLIC_URL` is
  the localhost default — the URL is only reachable when the MCP server runs locally.

## Release workflow recipes

**New app:** `request_bundle_upload` → PUT zip → `deploy_app` *without* `app_id` — a GUID is
generated and returned. Record it; it's the handle for every later operation.

**New version of an existing app:** `list_apps` first — find the app's `id` and *current*
`version`, pick a semver different from that live version (the platform rejects only an exact match
to it, but always bump) → three-step flow with `app_id` set.

**Rename / re-describe / change icon (no new bundle):** `update_app(app_id, name=…)` — it reads
the current manifest, merges only what you pass, writes it back. Never use it to fake a release:
bundle and version releases go through `deploy_app`.

**Retire an app:** `delete_app(app_id)` — soft-delete; the registry entry disappears and blobs are
cleaned, but redeploying with the same `app_id` restores the app.

## Validation rules (checked client-side before anything is consumed)

- `version` — semver (`1.2.3`, `1.0.0-rc.1`). Rejected only when it exactly matches the app's currently live version — always bump anyway.
- `app_scope` / `scope` — snake_case, `[a-z][a-z0-9_]*` (e.g. `todo_app`). Must equal the
  `ModuleFederationPlugin` `name` the app was built with, or the shell can't mount it.
- `name` — non-empty.
- Failing validation never consumes the staged upload — fix the field and call `deploy_app` again
  with the same `upload_id`.

## Pitfalls

1. **Zip the `dist/` contents, not the folder** — a zip with a top-level `dist/` directory
   deploys "successfully" but the shell 404s on `remoteEntry.js`.
2. **`app_scope` mismatch** — deploy-time `app_scope` must match the webpack MF scope
   (`app_<slug_with_underscores>` for scaffolded apps). Wrong scope = manifest loads, app never
   mounts.
3. **Version reuse** — the 409 fires only when the version string exactly equals the app's
   *currently live* version (no semver ordering, no history check); `list_apps` shows the
   current one to bump from, and bumping is still the recommended practice every release.
4. **Deploy failed? The staged bundle survives.** Neither client-side validation errors nor
   platform failures (409 duplicate version, 403 missing role, 404 unknown app id) consume the
   upload — fix the manifest field (e.g. bump `version` after a 409) and call `deploy_app` again
   with the **same `upload_id`**. Only a successful deploy consumes it (and the TTL still
   applies).
5. **These tools don't build anything** — produce the bundle with the `rise-x-apps` skill (its build phase
   covers the production build + zipping) and hand the zip to this flow.
