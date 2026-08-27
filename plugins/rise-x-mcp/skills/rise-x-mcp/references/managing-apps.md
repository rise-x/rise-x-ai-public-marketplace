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
| `deploy_app(upload_id, name, version, app_scope, app_id?, description?, icon?)` | Step 3: deploys the staged bundle; new app (GUID generated) or new version of an existing `app_id`. Validates the bundle's dependency manifest against the target ecosystem — see § Dependency manifest |
| `list_apps()` | Registry listing — `id`, `name`, `version`, `scope`, `remoteUrl`, `lastModified`, `dependencyCount` per app |
| `get_app(app_id)` | Full manifest for one app (incl. `module`, `description`, `icon`, resolved `dependencies`) |
| `update_app(app_id, …fields)` | Metadata-only read-modify-write; pass just the fields that change. Does NOT touch the bundle |
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
     version   = "1.0.0",             # semver; must be unique per app — bump every release
     app_scope = "my_app",            # MF scope, snake_case: [a-z][a-z0-9_]*
     app_id    = <GUID>,              # ONLY when releasing a new version of an existing app
   )
     → app id + canonical manifest (remoteUrl, version, scope, deployedAt, sizeBytes,
       dependencies — echoed from the bundle's dependency manifest, if it has one)
```

### Bundle requirements

- A zip of the app's **`dist/` folder contents** — `remoteEntry.js` must sit at the **archive
  root** (zip the contents, not the folder), or the shell can't load the remote.
- Size-capped (default 20 MB) — the PUT is rejected above the cap.
- May contain **`rise-x-app.json`** at the archive root — the app's dependency manifest,
  emitted by the app's build. Expected, not an error; see § Dependency manifest for how it's
  validated at deploy time.

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
`agent`), an optional `label`/`description`, and the id it resolves to — `flowOriginId` for a
flow or asset type, `agentId` for an agent (an agent has no origin id and no version chain).
The app's build emits it — these tools never author or edit it.

**The two `kind` spellings are not interchangeable.** The file takes the lower-camel values
above; `get_app` returns them capitalised (`Flow`, `AssetType`, `Agent`). Never copy a kind out
of a `get_app` result into a hand-edited manifest — the platform rejects the capitalised form.

**Deploy-time validation.** `deploy_app` validates the manifest against the *target* ecosystem
before it stores anything. A rejection comes back as **`error.code: http_400`** whose message
names the failing entries by alias, label, kind, and id. There is no dedicated error code to
match on — `InvalidDependencyManifest` and `UnresolvedDependencies` are names the platform logs
internally, never fields the response carries — so the message is the signal. It is truncated at
500 characters, and each failing entry costs ~165 of them, so expect the first two and assume
more. Two things are rejected:

- **A malformed file** — bad JSON, or missing/invalid fields.
- **A declared dependency that doesn't resolve in the target ecosystem.** Usually a bundle
  **built for a different environment** (test-environment ids deployed to production): rebuild
  with the right `APP_ENV` and restage. The ids live inside the zip, so reusing the same
  `upload_id` redeploys the same broken manifest.

Agents are validated the same way, with one deliberate difference: an agent that doesn't
resolve gives the **same reason** whether it is absent, deleted, or lives in another ecosystem.
A failed deploy therefore never confirms that an agent exists somewhere else — don't read the
message as "wrong ecosystem" evidence.

**No manifest and an empty manifest are opposite outcomes.** A bundle with **no**
`rise-x-app.json` leaves the app's stored dependencies **untouched** — deliberate, so a deploy
from a manifest-unaware build cannot wipe them — and `deploy_app` echoes the retained set. A
bundle whose manifest declares **none** *clears* them. The scaffolded app template ships an
empty manifest, so redeploying an app whose manifest was never filled clears whatever the
registry had recorded.

**Read `dependencyCount`, not the presence of `dependencies`.** The result omits `dependencies`
whenever the set is empty, so its absence covers the cleared case *and* the never-had-any case
— it never means "unchanged". `dependencyCount: 0` is the only thing that says the app now
declares none; no `dependencyCount` at all says the app has no stored manifest. `dependencies`
appears only when the set is non-empty.

**Reading dependencies back.** `get_app` returns a `dependencies` list — the way to answer
"what data does this app use". It is absent when the app has no manifest, empty when its
manifest declared nothing. Read each entry's `kind` to know which fields apply, and mind the
casing: **the response uses `Flow`, `AssetType`, and `Agent`**, not the manifest's lowercase
values. A `Flow` or `AssetType` entry carries `flowOriginId` plus the resolved `flowId` and
`flowName` in the active ecosystem; an `Agent` entry carries `agentId` and `agentName`, and none
of the flow fields. Every entry carries the declared `name` (the manifest's alias) and `kind`;
`label` and `description` appear only when the manifest supplied them. `list_apps` rows carry a
`dependencyCount`, and a successful `deploy_app` echoes `dependencies` in its result.

**Declaring an agent doesn't make it callable from here.** No MCP tool runs or spawns an agent;
the declaration records it for the ecosystem and lets the app's own code call it.

## Release workflow recipes

**New app:** `request_bundle_upload` → PUT zip → `deploy_app` *without* `app_id` — a GUID is
generated and returned. Record it; it's the handle for every later operation.

**New version of an existing app:** `list_apps` first — find the app's `id` and *current*
`version`, pick a strictly newer semver (a duplicate version is rejected) → three-step flow with
`app_id` set.

**Rename / re-describe / change icon (no new bundle):** `update_app(app_id, name=…)` — it reads
the current manifest, merges only what you pass, writes it back. Never use it to fake a release:
bundle and version releases go through `deploy_app`.

**Retire an app:** `delete_app(app_id)` — soft-delete; the registry entry disappears and blobs are
cleaned, but redeploying with the same `app_id` restores the app.

## Validation rules (checked client-side before anything is consumed)

- `version` — semver (`1.2.3`, `1.0.0-rc.1`). Unique per app.
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
3. **Version reuse** — the platform rejects a duplicate version per app; `list_apps` shows the
   current one to bump from.
4. **Deploy failed? The staged bundle survives.** Neither client-side validation errors nor
   platform failures (409 duplicate version, 403 missing role, 404 unknown app id) consume the
   upload — fix the manifest field (e.g. bump `version` after a 409) and call `deploy_app` again
   with the **same `upload_id`**. Only a successful deploy consumes it (and the TTL still
   applies).
5. **These tools don't build anything** — produce the bundle with the `rise-x-apps` skill (its build phase
   covers the production build + zipping) and hand the zip to this flow.
6. **An `http_400` naming dependencies on deploy = wrong-environment bundle.** The declared
   ids (`flowOriginId`s, or an `agentId`) don't exist in the target ecosystem — almost always a
   bundle built against another environment (test ids pushed to production). Don't retry with
   the same `upload_id`: it still holds the old zip. Rebuild with the right `APP_ENV`, re-zip,
   and run the three-step flow again. And `rise-x-app.json` sitting at the archive root is
   *expected* — never "clean" it out of the zip to get past the check.
