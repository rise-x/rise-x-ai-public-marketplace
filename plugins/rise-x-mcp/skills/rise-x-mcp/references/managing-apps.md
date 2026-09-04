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
| `deploy_app(upload_id, name, version, app_id?, description?, icon?, feature_flags?)` | Step 3: deploys the staged bundle; new app (GUID generated) or new version of an existing `app_id`. Checks the shape of the bundle's dependency manifest — see § Dependency manifest. `feature_flags` is a `string → bool` dict gating app behaviours (e.g. `{"isOfflineModeEnabled": true}`), stored on the manifest as `featureFlags`. **Omitted, the stored flags carry through unchanged; `{}` clears them** — so it is the FIRST deploy that must carry a flag, and a later release does not silently drop it |
| `list_apps()` | Registry listing — `id`, `name`, `version`, `scope`, `remoteUrl`, `lastModified` per app. No dependency fields |
| `get_app(app_id)` | Full manifest for one app (incl. `module`, `description`, `icon`, `featureFlags`, declared `dependencies`) |
| `update_app(app_id, …fields)` | Metadata-only read-modify-write; pass just the fields that change. Accepts `feature_flags` — **a passed dict replaces the stored flags wholesale** (one field, no per-key merge), so send it complete or keys you omit are silently dropped. Does NOT touch the bundle |
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
     version   = "1.0.0",             # semver; bump every release (409 only on the currently live version — see § Pitfalls #2 below)
     app_id    = <GUID>,              # ONLY when releasing a new version of an existing app
   )
     → app id + canonical manifest (remoteUrl, version, scope, deployedAt, sizeBytes,
       dependencies + dependencyEnvironment — the app's stored manifest after
       this deploy, if it has one)
     # scope is derived server-side from the `var <name>;` declaration in remoteEntry.js —
     # that returned `scope` is what the shell uses to mount the app
```

### Bundle requirements

- A zip of the app's **`dist/` folder contents** — `remoteEntry.js` must sit at the **archive
  root** (zip the contents, not the folder), or the shell can't load the remote.
- Size-capped (default 20 MB) — the PUT is rejected above the cap.
- May contain **`rise-x-app.json`** at the archive root — the app's dependency manifest,
  emitted by the app's build. Expected, not an error; see § Dependency manifest for what the
  deploy checks.

### Upload URL semantics

- **Single-use** and short-TTL (default 15 minutes) — one successful PUT consumes the *URL*; the
  staged bundle then waits for `deploy_app`. If the TTL lapses, call `request_bundle_upload`
  again.
- The token in the URL is the credential (bound to your session); staged bytes are inert until
  `deploy_app` forwards them to the platform with *your* bearer token.
- A `localhost_public_url` warning in the step-1 response means the server's `MCP_PUBLIC_URL` is
  the localhost default — the URL is only reachable when the MCP server runs locally.

## Dependency manifest (`rise-x-app.json`)

A bundle may declare the platform data it uses in a `rise-x-app.json` at the archive root: the
environment it was built for, plus a `dependencies` object **keyed by the dependency's alias**
(the alias is the key, not a field). Each entry carries a `kind` (`flow`, `assetType`, or
`agent`), an optional `label`/`description`, and the id it names — `flowOriginId` for a
flow or asset type, `agentId` for an agent (an agent has no origin id and no version chain).
The app's build emits it — these tools never author or edit it.

**Not every server has this release.** On a server without it, `get_app` returns no
`dependencies` and `deploy_app` returns no `dependencyCount` or `dependencyEnvironment`. Read
that absence as *not supported here*, not as *the app declares none*. A server that has the
release always returns `dependencyCount` on `deploy_app`.

**The file in the zip is the built copy.** The build writes it from the app's source
`rise-x-app.json`, which lists `environments` (plural) and each environment's ids. The built
copy names the one `environment` it targets and carries only that environment's ids, so a
hand-edited source manifest uses `environments`, never `environment`.

**The two `kind` spellings are not interchangeable.** The file takes the lower-camel values
above; `get_app` returns them capitalised (`Flow`, `AssetType`, `Agent`). Never copy a kind out
of a `get_app` result into a hand-edited manifest — the platform rejects the capitalised form.

**Deploy-time validation is shape only.** `deploy_app` checks that the file is valid JSON with a
`dependencies` key, that each entry has a known `kind` and the id field that kind takes, and
that the size and length bounds hold. It stores the declarations and stops there. **It never
looks the ids up.** A rejection comes back as **`error.code: http_400`** with a message naming
the shape problem. There is no dedicated error code to match on: `InvalidDependencyManifest` is
an internal code the response never carries, so the message is the signal. Fix
the file, rebuild, and restage — the ids live inside the zip, so reusing the same `upload_id`
redeploys the same broken manifest.

**A wrong-environment bundle deploys clean.** Because nothing resolves the ids, a bundle built
for test and deployed to production succeeds, stores test ids, and fails later inside the
running app. The deploy is not a check that the ids are good. The one signal you get is
`dependencyEnvironment`, the build target the stored manifest declared (present when the
manifest carried an `environment`, which the SDK build writes from 0.12.0): `deploy_app` and
`get_app` both return it, and nothing enforces it, so compare it against the ecosystem you
deployed into yourself.

**No manifest and an empty manifest are opposite outcomes.** A bundle with **no**
`rise-x-app.json` leaves the app's stored dependencies **untouched** — deliberate, so a deploy
from a manifest-unaware build cannot wipe them — and `deploy_app` echoes the retained set. A
bundle whose manifest declares **none** *clears* them. From SDK 0.12.0 the scaffolded app
template ships an empty manifest, so redeploying an app whose manifest was never filled clears
whatever the registry had recorded.

**On `deploy_app`, read `dependencyCount`, not the presence of `dependencies`.** The result omits
`dependencies` whenever the set is empty, so its absence covers the cleared case *and* the
never-had-any case — it never means "unchanged". `dependencyCount: 0` is the only thing that
says the app now declares none. Read no `dependencyCount` at all as a server without the
dependency-manifest release; only once you know the server has that release does its absence
mean the app has no stored manifest.
`dependencies` appears only when the set is non-empty.

**Reading dependencies back.** `get_app` is the only tool that returns them, and the way to
answer "what data does this app use". `list_apps` carries no dependency fields at all, not even
a count, because the platform omits them from its list endpoint — so there is no cheap way to
find the apps that declare something, and you call `get_app` per app. The list is absent when
the app has no manifest, empty when its manifest declared nothing. Every entry carries `name`
(the manifest's alias), `kind`, and `resourceId`; `label` and `description` appear only when the
manifest supplied them. Mind the casing: **the response uses `Flow`, `AssetType`, and `Agent`**,
not the manifest's lowercase values. `resourceId` is a flow origin id on a `Flow` or
`AssetType`, an agent id on an `Agent` — read `kind` to know which, and note there are no
resolved names in the entry.

**These are declarations, not lookups.** Nothing verified the ids at deploy, so an entry can
name a flow or agent that was deleted, or that never existed here. Resolve a `resourceId` with
the flow or agent tools before telling the user the app uses it, and read a miss as a stale
declaration rather than a broken tool.

**Declaring an agent doesn't make it callable from here.** No MCP tool runs or spawns an agent;
the declaration records it for the ecosystem and lets the app's own code call it.

## Release workflow recipes

**New app:** `request_bundle_upload` → PUT zip → `deploy_app` *without* `app_id` — a GUID is
generated and returned. Record it; it's the handle for every later operation.

**New version of an existing app:** `list_apps` first — find the app's `id` and *current*
`version`, pick a strictly newer semver (a duplicate of the currently live version is rejected
— see § Pitfalls #2 below) → three-step flow with `app_id` set.

**Rename / re-describe / change icon (no new bundle):** `update_app(app_id, name=…)` — it reads
the current manifest, merges only what you pass, writes it back. Never use it to fake a release:
bundle and version releases go through `deploy_app`.

**Retire an app:** `delete_app(app_id)` — soft-delete; the registry entry disappears and blobs are
cleaned, but redeploying with the same `app_id` restores the app.

## Validation rules (checked client-side before anything is consumed)

- `version` — semver (`1.2.3`, `1.0.0-rc.1`). Unique per app — bump every release.
- `name` — non-empty.
- Failing validation never consumes the staged upload — fix the field and call `deploy_app` again
  with the same `upload_id`.

## Deploy-time bundle checks (server-side, on `deploy_app`)

The platform inspects the uploaded zip itself before accepting the deploy:

- **No scope declaration** — if `remoteEntry.js` has no leading `var <name>;` declaration to
  derive the scope from, the deploy is rejected: `400 Invalid bundle: ...`. Fix: build with the
  SDK preset / webpack `ModuleFederationPlugin` rather than hand-editing the entry file.
- **Declared name not a usable scope** — if the declared name doesn't match `[a-z][a-z0-9_]*`,
  the deploy is rejected: `400 Invalid bundle: ...`. Fix: the package name must be lowercase
  kebab-case so it produces a valid `app_<slug_with_underscores>`.
- **Missing chunk** — if `remoteEntry.js`'s chunk map references a file that isn't in the zip, the
  deploy is rejected: `400 Invalid bundle: referenced chunk '<file>' is missing from the bundle`.
  This means the build or the zip is truncated/partial. Fix: rebuild and re-zip the `dist/`
  contents in full, then `request_bundle_upload` again (the old upload is stale by then anyway).

## Pitfalls

1. **Zip the `dist/` contents, not the folder** — a zip with a top-level `dist/` directory
   deploys "successfully" but the shell 404s on `remoteEntry.js`.
2. **Version reuse** — the platform rejects a duplicate version per app; `list_apps` shows the
   current one to bump from. The platform enforces this narrowly: the 409 fires only when the
   string exactly equals the app's currently live version, with no semver ordering or history
   check. Don't lean on that; reusing an older version string is accepted and leaves a confusing
   registry.
3. **Deploy failed? The staged bundle survives.** Neither client-side validation errors nor
   platform failures (409 duplicate version, 403 missing role, 404 unknown app id) consume the
   upload. Fix the failing `deploy_app` field (`name` or `version`; for example, bump `version`
   after a 409) and call `deploy_app` again with the **same `upload_id`**. Only a successful
   deploy consumes it (and the TTL still applies). A 400 invalid bundle is different: the staged
   zip itself is the problem, so rebuild and call `request_bundle_upload` again for a fresh
   `upload_id`.
4. **These tools don't build anything** — produce the bundle with the `rise-x-apps` skill (its build phase
   covers the production build + zipping) and hand the zip to this flow.
5. **A wrong-environment bundle deploys without complaint.** The deploy checks the dependency
   manifest's shape, never whether the ids exist, so test ids pushed to production store fine
   and break inside the running app. Compare the result's `dependencyEnvironment` against the
   ecosystem you deployed into, and if it is wrong, rebuild with the right `APP_ENV` and re-zip.
   Don't retry with the same `upload_id`: it still holds the old zip. An `http_400` naming
   dependencies is a *shape* problem in the file, not a missing id. And `rise-x-app.json` sitting
   at the archive root is *expected* — never "clean" it out of the zip.
