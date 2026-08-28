# Offline capability — code and data with no connection

## When to use

The plan calls for a federated app whose screens must keep working — rendering, editing, submitting —
with no network. That always means two things together: the app's own **bundle** must boot from cache
(cold reload while offline), and its **data** (works, layouts, flow config) must read and queue from
cache too. This covers both, in the order an app author actually hits them: prepare the flow and
app, pull data down, read, write, stay fresh, watch the sync queue — then a consolidated list of
platform gaps and a minimal example. That includes the `@rise-x/apps-sdk` connectors (`work`/`flows`/
`attachments` reads, explicit writes, the `offline` connector's queue tools) function by function with
their argument traps, the `onOfflineDownload` lifecycle hook, and which `@rise-x/apps-sdk` version the
offline behaviour needs.

Verified against `@rise-x/apps-sdk` **0.12.0** (bridge protocol v4) — where a claim is
version-sensitive, the text says so.

Not needed for an app that only wants a "you're offline" banner for UX copy — `offline.isOnline()`
covers that alone (§6). Not needed while merely scaffolding a new app — that's the scaffold
(`references/build.md`); come back to this reference once the scaffold exists and the plan asks for
offline support specifically.

Offline support also requires a platform release that carries it. On an older host, the offline
connector methods throw `ConnectorError("SHELL_TOO_OLD", …)` and the app card shows no "Make available
offline" action — there is nothing further to configure your way around. This reference assumes a host
that has it.

**The build, in order.** Each numbered step is a section below; the sections are organised by
subsystem; this list is the execution order.

1. **Flow prep first** (§2) — the offline flag via the flow draft cycle, and quick submit on the panel
   config of every task the app submits from (layout config — changeable in later flow versions).
2. **The app-level flag** (§2) — plan to pass `feature_flags: {"isOfflineModeEnabled": true}` on
   `deploy_app`, or set it later via `update_app`.
3. **Screens** (§4, §5) — reads through the connectors or the `/query` hooks (both answer from the
   offline cache, §4), every write routed by `writeMode` read fresh at the moment of the write.
4. **The data pull** (§3) — export `onOfflineDownload`; without it an offline-enabled app opens
   offline to an empty world.
5. **Freshness and queue visibility** (§6, §7) — browser connectivity events, one Refresh control,
   the pending-edits overlay.
6. **Prove it** (§9, "Prove it works") — six checks, the deployed test environment and the mock shell.
7. **Deploy** (§10) — including confirming the flags actually shipped.

## 1. What offline means here

Offline is two independent problems plus a coordination layer. Know which one you're touching before
you reach for a function.

| Subsystem | What it is | Who owns it |
|---|---|---|
| **Code** | The app's bundle — `remoteEntry.js` + chunks — cached read-only so a cold reload with no network still boots the app | The shell's download orchestrator + a service-worker route. Not app-facing. |
| **Data** | Flow config, layouts, works, attachments, and every queued-but-unsent write | The connectors (`work`, `flows`, `attachments`, `offline`) from `@rise-x/apps-sdk` — this is what your app code calls |
| **Availability** | Whether a download is actually complete and intact right now | Derived on every check, never a stored flag — storage can be evicted without telling the app |

Asset-backed screens are online-only today — §8, gap 5.

A download for an offline-enabled app captures three artifacts, in order: the app **manifest**
(`remoteUrl` + `featureFlags`), a **file listing** enumerating every chunk the bundle needs, and the
**bundle** files themselves. All three land in cache **before** your app's own `onOfflineDownload` hook
runs (§3) — by the time your data-download code executes, it is already running from the cached
bundle, not the network.

**There is no raw bridge object for your app to reach for.** The app-facing surface is the
connectors — `work`, `flows`, `attachments`, `offline` (all from `@rise-x/apps-sdk/connectors`) —
each of which decides its own read source and never asks you to branch on connectivity. `getShell()`
does expose the raw handles, but everything `getOffline()` offers that a running app needs is
already on the `offline` connector with its error contract — the two subscription methods are the
exception, reachable only inside `onOfflineDownload` via `tools.offline` (§3); outside the hook,
connectivity changes are the browser's own `online`/`offline` events (§6). The only sanctioned direct
`getCache()` use is §8's task-name workaround, which exists precisely because the connector surface
lacks that read. Use the connectors for everything else.

**Two separate feature-flag bags gate all of this**, at two different levels, and confusing them
disables offline silently rather than erroring:

- **Flow-level**: `flow.properties.featureFlags.isOfflineModeEnabled` — this flow's work data may
  queue writes and create works offline. Set it up correctly per §2.
- **App-level**: the app manifest's `featureFlags.isOfflineModeEnabled` — this app's *code* may be
  promised offline (drives the "Make available offline" card). Set at deploy time — §2.

An app that both reads and writes offline needs both flags set: the app flag on its own manifest, the
flow flag on every flow it depends on. Neither bag is `flowFeatures` — that's a separate, often-empty
bag used for unrelated per-flow toggles (e.g. barcode scanning); patching it instead of `featureFlags`
silently leaves offline disabled with no error pointing at the mistake.

Three rules govern everything past this point; each is expanded where it's actually used, not here:

- **Reads resolve their own source — you never choose one.** Expanded in §4.
- **Writes are routed by you, explicitly — nothing is gated for you.** Expanded in §5.
- **Sync is shell-owned** — your app queues, observes, and can ask (`requestSync`); the shell's own
  machinery drains the outbox. Expanded in §7.

### The bridge under the connectors, mapped

Everything offline rides on two handles the shell exposes over its versioned bridge — the
`window.__DIANA_SHELL__` object `references/build.md`'s mock-shell setup also assigns; the SDK
exports the protocol number as the `SHELL_BRIDGE_VERSION` constant, and 4 is the version that
carries these: `getCache(): CacheApi | null`
— raw, read-only views of exactly what a download stored — and `getOffline(): OfflineApi | null` —
connectivity, queueing, sync, and downloads. Both are **internal transport between the shell and the
SDK**; the connectors are the management layer your app talks to. `null` means the host predates the
handle, and a handle being present says nothing about which methods it carries — the connectors
feature-detect the *method*, not the accessor, and throw `ConnectorError("SHELL_TOO_OLD", …)` where a
raw call would die on a bare `TypeError`. This table is the map from bridge capability to what your
app actually calls; if a bridge method has no app-facing row, that is a decision, not an omission.
Both handles are also session-guarded — the cache and outbox are keyed to the signed-in user. Before
the shell has a session, every **read** fails soft — the cache reads answer `null`, and the `offline`
connector's list reads (`listQueuedWorkOperations`, `listDownloadedWorkIds`, `listDownloadedFlowIds`)
answer `[]`; nothing user-keyed legitimately exists yet. Every **write** (`queue*`,
`downloadFlowWorks`) **throws** rather than file into a bucket nothing will ever sync. `requestSync`
silently no-ops pre-session — or offline (§7).

**`CacheApi` — every method feeds one connector read's cache branch (§4).** The connector adds the
read ladder on top: network-first with a 10s bound while online, the cached value on an unreachable
server, errors normalized to `ConnectorError`.

| Bridge method | Managed as | Notes |
|---|---|---|
| `readWork(workId)` | `work.get()` | The server's raw work document, mapped through the same mapper as the network response — no queued edits folded in |
| `readWorkData(workId, {path?})` | `work.getData()` | The shell resolves `path` with the same JSONPath the server's `?path=` accepts |
| `readMyWorkAccess(workId)` | `work.getMyAccess()` | `[]` = downloaded with no roles, `null` = not downloaded — the read policy relies on the distinction |
| `readFlow(flowId)` | `flows.getConfig()` | Configs are cached under **version** ids; an origin id scans the downloaded set for the newest downloaded version |
| `readLayout(layoutId)` | `flows.getLayout()` | The flat `parentId` list, which is also what the network returns — one mapper, nothing rearranged |
| `readAttachmentBlob(args)` | `attachments.getBlob()` | The one async cache read — bytes live in the blob store |

The only sanctioned direct use of `getShell().getCache()` is the documented task-name workaround in
§8; everywhere else, going around the connector means going around the read ladder and its error
contract.

**`OfflineApi` — two tiers of exposure:**

| Bridge method | Managed as |
|---|---|
| `isOnline`, `getWorkSyncStatus`, `listQueuedWorkOperations`, `queueWorkDataUpdate`, `queueWorkAction`, `queueWorkCreation`, `queueWorkAttachmentUpload`, `queueWorkAttachmentDeletion`, `requestSync`, `listDownloadedWorkIds`, `listDownloadedFlowIds`, `downloadFlowWorks`, `getFlowWorksDownloadInfo` | Re-exposed one-for-one on the `offline` connector (§5, §7), behind the `SHELL_TOO_OLD` feature detection |
| `subscribeOnline`, `subscribeFlowDownload` | Hook-only: they exist on the raw `tools.offline` handle `onOfflineDownload` receives (§3) — drive `reportProgress` and download-time reactions from them there. Outside the hook, connectivity changes are the browser's own `window` `online`/`offline` events (§6) |

`tools.offline` inside `onOfflineDownload` is the **raw** `OfflineApi` — the one place app code holds
the bridge handle itself. Treat it as scoped to the download: use it for the data pull and progress,
and let the connectors carry everything your running UI does.

## 2. Prepare the flow + app

**The flow's offline flag is set through the flow draft cycle, not a one-shot pre-publish edit.**
`create_flow_draft` → `update_flow_properties(draftId, { featureFlags: … })` → `publish_flow` — the
`rise-x-mcp` plugin's `references/managing-flows.md` documents the cycle, and it is the only path that
sticks. Follow through the same way that reference does: read the flag back before publishing
(`get_flow_config(draftId, path="properties.featureFlags")`), and after `publish_flow` take the
flow's **new published id** from the publish envelope — or re-resolve it via `list_flows` — since
every id you held before publishing is stale except the `flowOriginId`, the one id stable across
republishes (§3 relies on exactly that). `PATCH
/api/v4/config/flow/{id}/properties` only applies a `featureFlags` update to a **draft** flow id; a
properties update aimed at an already-**published** flow id silently strips everything except identity
fields at the REST layer, and — via the MCP — surfaces as a `dropped_property` warning with
`changed: []`. So always target a draft: to change the flag later, open a new draft off the published
flow, patch that draft's id, then publish again. The bag itself is a plain `string → bool` dictionary on
the flow's properties — the key is exactly `isOfflineModeEnabled` — and it is shared with unrelated
flags the platform seeds (`UseCompletedName`, …). The update **replaces the whole bag** — properties
merge per-field, so a non-null `featureFlags` wins wholesale and any seeded key you didn't echo is
dropped — so read what is already in it and send it complete. One more reach limit: the runtime reads
the flag from each work's **own pinned flow version**, and publishing a new version does not move
already-open works by default — a flag change reaches works created after the publish, not the ones
already open.

**Quick submit is a task-level switch, not a per-action one.** `quickSubmit` lives on the `panelConfig`
of a step/task in the flow config document, and the shell's toolbar checks it once per task — not per
action; the clicked action's own config is never consulted by that gate. There is no flow-builder UI
control for it today, so it is set on the flow document directly. `quickSubmit` is a client-side
contract — the server never reads it (verified against the shell's submit path). The work page
auto-submits when the task is quick-submit, but routes a non-quick-submit task through a
recipient-picker drawer, and the only guard against an empty recipient list lives in that drawer's
submit hook. `offline.queueWorkAction` (§5) resolves the configured invitees itself and **bypasses that
guard entirely**, so submitting from a non-quick-submit task whose action config resolves nobody queues
fine and syncs with an empty recipient list — and nothing rejects it: there is no server-side
validation, so what comes back is an invitation naming nobody, or — when the step restricts the action
to a `PartyName` (a flow-config role) — §5's silent `200` no-op. Quick submit makes the picker-less path
the task's *configured* contract, which is why it belongs on the panel config of every task the app
submits from; where a flow's config resolves nobody for an action you must submit anyway, pass
`recipients` (§5).

**The app-level flag is a deliberate step at deploy time**: pass
`feature_flags: {"isOfflineModeEnabled": true}` to `deploy_app` (it survives redeploys), or set it on
an existing app via `update_app`. `update_app` merges field by field — only what you pass changes —
but `feature_flags` is a single field, so a passed dict **replaces the stored flags wholesale**; send
the full dict, not just the key you're changing. The shell's deploy form has no field for it.

## 3. Pull data down

The whole download is one user action: the app card's **Make available offline** control, rendered
only when the manifest carries the app-level flag (§1). The shell then runs a fixed sequence —
manifest → file listing → bundle files → your `onOfflineDownload` hook, if the lifecycle module
exports one — and afterwards derives availability by re-verifying the cached bundle against the
manifest on every check, so storage eviction shows up as "not available offline" on the card rather
than as a broken boot.

The bundle download (§1) never fetches work data itself — only the manifest, file listing, and bundle
chunks. Without your own hook, an offline-enabled app opens offline to an empty world. Export
`onOfflineDownload` from `src/lifecycle.ts` alongside `onInstall`/`onUpdate`/`onUninstall`. The flow
reference it needs is app config, recorded once at integration time — not something to resolve by
searching flow names at runtime, since a shipped app that hardcodes a flow's display name breaks the
moment a user renames the flow:

```ts
import type { OfflineDownloadHook } from "@rise-x/apps-sdk";

const FLOW_ORIGIN_ID = "…"; // flowOriginId — app config, recorded at integration time

export const onOfflineDownload: OfflineDownloadHook = async (_ctx, { offline, reportProgress }) => {
  const result = await offline.downloadFlowWorks({ flowOriginId: FLOW_ORIGIN_ID });
  reportProgress({ processed: result.total, total: result.total });
};
```

`downloadFlowWorks` (§5) already throws when any work in the flow fails to pull, so the hook fails
loudly on its own — there is no separate lookup step that could silently skip a missing flow.

- The shell calls this **after** the app's own bundle is already fully cached — the hook's own code
  (and anything it imports) is already running from the offline copy by the time it executes.
- **`tools.offline` here is the raw shell `OfflineApi`, not the `offline` connector import from
  `@rise-x/apps-sdk`.** They share almost the same method names (`downloadFlowWorks`,
  `getWorkSyncStatus`, `queueWorkDataUpdate`, …), which makes the distinction easy to miss, but the raw
  handle also carries methods the connector deliberately does not expose —
  `subscribeFlowDownload`, `subscribeOnline` — and it has no per-method
  `SHELL_TOO_OLD` guard: a missing method throws a bare `TypeError`, not a `ConnectorError`. Treat
  `tools.offline` as narrower-guaranteed than the connector, not a drop-in swap for it.
- `reportProgress({ processed, total })` feeds the shell's single download progress bar. `total` is
  whatever unit your app is counting.
- **Throwing fails the whole download.** Unlike `onInstall`/`onUpdate`/`onUninstall` — fire-and-forget
  reconciliation the shell runs behind a short timeout and swallows errors from — this hook is
  user-initiated, can legitimately run minutes, and its failure is reported to the user as "not
  available offline." Don't catch and swallow just to report success on partial data — a failed pull
  should throw loudly (the way `downloadFlowWorks` itself does on a failed work, §5) rather than
  silently reporting success on an empty or partial pull. The bundle stays cached regardless, so a retry
  only redoes the data pull.
- `ctx` is a **snapshot at invocation** — `{ manifest: { id, version, name }, user, environment }` —
  taken when the hook starts; a pull that runs minutes does not see a mid-flight user or environment
  switch through it.
- One `downloadFlowWorks` call captures everything §4's cache-backed reads need for that flow: the
  flow's works (each work's server document, data document, and your roles on it), **every flow version those
  works were created under** plus all of their layouts (conditional layouts included), and the asset
  lists any search-things components on those layouts need. There is no `signal` — a pull is not
  cancellable once started.
- To report progress *during* the pull instead of once at the end: `tools.offline.subscribeFlowDownload(
  flowOriginId, listener)` fires whenever the download state moves (the raw-handle-only method, see
  above — it exists because `getFlowWorksDownloadInfo` reads a store outside React and polling it
  alone sits on `idle`); inside the listener read `getFlowWorksDownloadInfo(flowOriginId)`'s
  `processed`/`total` for the run in flight and feed them to `reportProgress`.

## 4. Read

Every read on `work`, `flows`, and `attachments` resolves through one shared policy
inside the SDK, so there is exactly one implementation of "where do these bytes come
from" and the reads cannot drift apart:

```
offline                          → cache when downloaded; a MISS FALLS THROUGH to the branch below
                                   (isOnline() is navigator.onLine — an interface, not a reachable server)
online — or that offline miss — request bounded at 10s
  ├─ resolved                    → fresh value
  ├─ 404                         → null   (it does not exist)
  ├─ other 4xx, or aborted       → throw  (the server refused; do not mask it with cache)
  ├─ unreachable/timeout/5xx + cached → cached value
  └─ unreachable/timeout/5xx, no cache → throw   ← where "offline and never downloaded" lands
```

So for `get`, `getMyAccess`, `getLayout`, `getConfig` and `getBlob`, `null` reaches your code for
exactly one reason: the server said the thing does not exist. `getData` alone adds a second — a
`path` that selects nothing also answers `null`, from either source (see its row below). "Offline
and not downloaded" is **not** a `null` on any of them — the miss falls through, the fetch fails,
and the read **throws** — because a `null` that meant either would be indistinguishable
from "not downloaded".

What your app experiences, function by function:

| Call | Fresh online | Cached offline (or network unreachable) | `null` means | Throws when |
|---|---|---|---|---|
| `work.get(workId)` | Full `WorkDetail` from `GET /api/v4/work/{id}` | Same shape, mapped from the cached copy of the server's work document, with **no queued edits folded in** — see below | Does not exist (server 404) — **only** that; offline-and-not-downloaded throws instead (see the ladder) | Server unreachable + nothing cached — including offline with nothing downloaded; any non-404 4xx; aborted |
| `work.getData(workId, {path?})` | The data document, or the subtree at `path` | Same subtree — the shell resolves `path` against the cached document with the same JSONPath the server's `?path=` accepts, so `$.parts[0].qty` answers identically either way | Weaker than `get`'s: a `path` that resolves to nothing also answers `null`, so `null` is not a work-missing signal here | Same |
| `work.getMyAccess(workId)` | `WorkAccessInfo` from `/my-roles` | From `readMyWorkAccess` — `[]` is a real "downloaded, no roles" answer, distinct from `null` ("not downloaded") | Same — 404 only | Same |
| `flows.getLayout(layoutId)` | The flat `parentId` component list from the network | The same flat list from the cached copy — one mapper, nothing rearranged, so a component walk behaves identically either way | Same — 404 only | Same |
| `flows.getConfig(flowId)` | Flow config from the network; an origin id is resolved to the latest version id | From the cached copy. Configs are cached under **version** ids, so an origin id — the stable one an app persists — is matched by scanning the downloaded configs and taking the **newest downloaded** version — the newest that *exists* would need the versions endpoint, i.e. the network | Same — 404 only | Same |
| `attachments.getBlob({id, resourceId, thumb?, version?})` | Bytes from `/api/v4/attachments/{id}` (or `/thumb`) | From the offline blob store — resolves a still-pending offline upload first, then the downloaded cache | Same — 404 only | Same |

`work.get()` also maps the work's attachments — `attachments: { id, fileName, title, path,
mimeType }[]` — which is what the attachments part of §5 reads from.

**Every other read is network-only and throws offline** — `work.list`, `work.search`, `work.iterate`, `work.listRelated`, `work.getAudit`, `flows.list`, `flows.get`, and `flows.findTask` (which calls `flows.list`). Only the six rows above have a cache branch, and there is no `source` option to force one either way.

**The `@rise-x/apps-sdk/query` hooks follow this same ladder.** `useWork`, `useWorkData`,
`useFlowConfig`, `useFlowLayout` and the rest run these same connector reads as react-query
`queryFn`s, and `createAppQueryClient` sets `networkMode: 'offlineFirst'` with an offline-aware
retry: mounted offline, a hook answers from the offline cache when the data is downloaded, and
settles into its **error** state (recovering on reconnect) when it is not — the same two outcomes as
a direct connector call. The hooks are a provided tool, not an obligation — an app that manages its
own caching is free to use that instead. One direct-connector shape is ordinary async state —
`WorkDetail` and `ConnectorError` are both importable from `@rise-x/apps-sdk/connectors` — with all
three outcomes of the read ladder handled:

```tsx
const [state, setState] = useState<
  | { kind: "loading" }
  | { kind: "ready"; work: WorkDetail | null } // null = does not exist on the server (404)
  | { kind: "error"; error: ConnectorError }   // unreachable + uncached (incl. offline + not downloaded), refused, SHELL_TOO_OLD
>({ kind: "loading" });

useEffect(() => {
  let alive = true;
  work.get(workId).then(
    (w) => alive && setState({ kind: "ready", work: w }),
    (e) => alive && setState({ kind: "error", error: e }),
  );
  return () => { alive = false; };
}, [workId]);
```

A screen that renders all three is offline-correct by construction — with "not downloaded" surfacing
through the **error** state (offline with nothing cached throws) and `null` meaning the work does not
exist on the server.

**Raw reads never show queued edits — the overlay is derived, not stored.** `work.get()`/
`work.getData()` map the network response and the cached copy through the *same* mapper, so the two
sources are shape-identical by construction — but the cached copy is the server's document **as
downloaded**, with nothing your app has since queued folded in. `listQueuedWorkOperations()` (§7)
carries each queued operation's `payload` as the app queued it (its shape varies by `kind` — a
`dataUpdate` carries `{ id, originId, path, operation, sectionId, value }`, where the payload's `id`
is the update's own id — distinct from the operation row's `id` — and `operation` is the same name
vocabulary `queueWorkDataUpdate` accepts). So an app can derive what is pending from the queue itself
— no separate state, and nothing to reconcile: the shell hands out only still-queued items (a synced
item stops appearing; the `isSynced` field on the row is the raw outbox flag and reads `false` here),
so the derivation falls back to the server's value on its own. Note the payload stores the write's
path under `path`, not `dataPath` — the same rename `patchData` makes (§5); if that ever drifted, the
match below would return "nothing pending" forever, which is why the helper matches on `path`
deliberately:

```ts
// The queue IS the overlay: the last queued write to a path is the pending value.
// hasPending keeps a queued null distinguishable from "nothing pending".
const pendingValue = (
  workId: string,
  dataPath: string,
): { hasPending: boolean; value?: unknown } => {
  const ops = offline.listQueuedWorkOperations({ workId, kind: "dataUpdate" });
  for (let i = ops.length - 1; i >= 0; i--) { // newest last — last write wins
    if (ops[i].isSynced) continue; // free guard; the shell already filters synced items out
    const p = ops[i].payload as { path?: string; value?: unknown };
    if (p?.path === dataPath) return { hasPending: true, value: p.value };
  }
  return { hasPending: false };
};

// In the component: queueWorkDataUpdate returns void and there is no queue subscription (§6),
// so nothing re-renders on its own — bump local state after every queue call.
const [, setQueueTick] = useState(0);

async function submitQty(workId: string, dataPath: string, value: unknown) {
  // originId first: patchData requires it, and resolving it is a network read (§4's ladder) that
  // must not sit between the writeMode read and the write. App config works here too (§3).
  const originId = (await work.get(workId))?.flowOriginId;
  if (!originId) throw new Error(`work ${workId} not found`);
  const { writeMode } = offline.getWorkSyncStatus(workId);
  if (writeMode === "queued") {
    offline.queueWorkDataUpdate({ workId, dataPath, value });
    setQueueTick((t) => t + 1); // re-render so pendingValue is re-derived
  } else {
    await work.patchData(workId, { originId, path: dataPath, value });
  }
}

// Render:
//   const pending = pendingValue(workId, dataPath);
//   value shown = pending.hasPending ? pending.value
//                                    : await work.getData(workId, { path: dataPath })
```

Two boundaries of the derivation, both by construction. It matches paths by **string equality** — a
queued write to `$.parts` is invisible to `pendingValue(workId, "$.parts[0].qty")` and vice versa, so
derive at the same path granularity you write at. And it only sees `dataUpdate` operations — queued
attachment uploads/deletions are their own kinds (§7), not writes to `$.attachments`. Reading `value`
as the pending state holds for plain value writes — `queueWorkDataUpdate`'s default; a queued
`push`/`addToSet` payload carries the added item, not the resulting array, so if your app passes
`operation` (§5), branch on `payload.operation` in the derivation too.

An app that prefers a hand-kept overlay (its own state written at queue time) is free to keep one —
that variant needs reconciliation: clear a work's entries when `getWorkSyncStatus(workId).allSynced`
flips `true`, then re-fetch to confirm the server's value. The queue-derived form above needs none.

## 5. Write

The rule the whole write surface is built to enforce: queue methods enqueue, they never decide
connectivity for you. Read-source rules are §4's — this section is the
write-side counterpart.

`work.patchData`, `work.submit`, `attachments.upload`, `attachments.delete` are **network-only** — they
throw (via `ConnectorError`) if the request fails and do not know how to queue. Calling one while
offline is a caller error, not a supported path. `offline.queue*` methods are the queued counterparts.
On a host with no offline support at all, every one of them (except `isOnline()`, §5) throws
`ConnectorError("SHELL_TOO_OLD", …)` rather than a bare `TypeError` — the connector does this
feature-detection for you, so don't probe the host for offline support yourself.

### Route every write by `writeMode`, read fresh at the moment of the write

```ts
// Anything async the write needs (like patchData's originId) resolves BEFORE the
// writeMode read — nothing may sit between the read and the write it routes.
const originId = (await work.get(workId))?.flowOriginId;
if (!originId) throw new Error(`work ${workId} not found`);

const { writeMode } = offline.getWorkSyncStatus(workId);
if (writeMode === "queued") {
  offline.queueWorkDataUpdate({ workId, dataPath, value }); // preserves order
} else {
  await work.patchData(workId, { originId, path: dataPath, value }); // clear to send
}
```

That is the entire decision. Never add an `if (online)` of your own next to it, and never cache
`writeMode` across an `await` — it folds in "offline **on a flow whose offline flag is set** (§2)" and
"online but this work already has something pending" (sticky, so replay order survives a mid-queue
reconnect), and reading it once at mount or holding it across an await reorders that work's queue
silently. The flag qualifier is also where a §2 misconfiguration finally surfaces: offline on a flow
*without* the flag reads `'direct'`, so the write goes to the network and fails instead of queueing.

### Every `offline` connector method, exact signature

| Method | Signature | Notes |
|---|---|---|
| `isOnline()` | `(): boolean` | The one method that answers rather than throwing on a host with no offline support — `true` there. |
| `getWorkSyncStatus(workId)` | `(workId: string): WorkSyncStatus` | `{ writeMode: 'direct' \| 'queued', hasPendingItems, syncProgress, hasError, allSynced }`. Read immediately before every write — never once at mount. |
| `listQueuedWorkOperations(args?)` | `(args?: { workId?: string; kind?: QueuedWorkOperationKind }): QueuedWorkOperation[]` | `{ id, kind, workId, payload, queuedAt, isSynced }` — `payload` is the operation's data exactly as queued; its shape varies by `kind`. `kind` is one of `workCreation \| dataUpdate \| attachmentUpload \| attachmentDeletion \| actionExecution \| attachmentTitleUpdate` — `attachmentTitleUpdate` is produced only by the shell's own attachment UI, observable here but not producible from this connector. Newest last — the order replay follows. |
| `queueWorkDataUpdate(args)` | `(args: UpdateWorkDataArgs): void` | **Synchronous.** No await, no `.then`. |
| `queueWorkAction(args)` | `(args: ExecuteWorkActionArgs): void` | **Synchronous.** See "Submitting" below. |
| `queueWorkCreation(args)` | `(args: CreateWorkArgs): { workId: string; code: string }` | **Synchronous — returns the result directly, not a Promise.** Builds an `OFFLINE-…`-coded stub locally; `workId` is real and stable immediately and stays stable after sync (only `code` swaps off `OFFLINE-…`). Pass `workCode` to use a caller-chosen code instead — see "Custom work codes" below. |
| `queueWorkAttachmentUpload(args)` | `(args: UploadWorkAttachmentArgs): Promise<void>` | The one queue method that IS async — bytes are written to the blob store before the item is queued. |
| `queueWorkAttachmentDeletion(args)` | `(args: DeleteWorkAttachmentArgs): void` | **Synchronous.** |
| `requestSync(args?)` | `(args?: { workId?: string }): Promise<void>` | See §7. |
| `listDownloadedWorkIds(flowOriginId?)` | `(flowOriginId?: string): string[]` | See "Finding your app's own works" below. |
| `listDownloadedFlowIds()` | `(): string[]` | Ids of every flow config held locally right now — a download or ordinary online browsing puts them there — as **version** ids (what configs are keyed by), not origin ids. |
| `downloadFlowWorks(args)` | `(args: DownloadFlowWorksArgs): Promise<DownloadFlowWorksResult>` | `{ flowOriginId, flowId? }` → `{ total, failedWorkIds }`. **Throws when any work fails** — on resolve, every work landed and `failedWorkIds` is empty. Omit `flowId` and the shell resolves the current version. |
| `getFlowWorksDownloadInfo(flowOriginId)` | `(flowOriginId: string): FlowDownloadInfo` | `{ status: 'idle'\|'preparing'\|'downloading'\|'done'\|'error', downloadedAt: number \| null, processed, total }` — `processed`/`total` count works for the run in flight (both `0` when idle). Derived, not stored — recomputed from the cache each call. |

Among the `queue*` methods only `queueWorkAttachmentUpload` is async; `requestSync` and
`downloadFlowWorks` are also promise-returning. Awaiting one of the synchronous `queue*` methods is
harmless but writing `.then()` on one is a bug your type-checker won't catch if you've cast loosely —
the return type is the tell.

### Data updates: `dataPath`, `value`, and the two write shapes' asymmetry

`dataPath` is a JSON path into the work's data document (`$.partName`, `$.parts[0].qty`); `value` is
stored and read back exactly as passed — nothing coerces it. Numeric inputs commonly persist as
**strings** (`"4"`, not `4`) if that's what the layout component that owns the field stores.

`offline.queueWorkDataUpdate`'s `originId` (the flow's `flowOriginId`) is optional — the shell resolves
it for you from the work. `work.patchData`'s `WorkDataPatch.originId` on the exact same field is
**required** — it is a thin wrapper over the raw `PATCH /api/v4/work/{id}/data` body with no
shell-side resolution, so a `'direct'`-routed write needs you to supply it yourself (read it off
`(await work.get(workId))?.flowOriginId` first if you don't already have it). Writing one branch as if it
mirrored the other's defaulting is a real trap the `writeMode` split invites.

**The operation argument.** `queueWorkDataUpdate` (and `patchData`, via its own body) defaults to a
plain **set**; `UpdateWorkDataArgs.operation` selects another patch operation by name —
`"set" | "push" | "pull" | "unset" | "addToSet" | "merge"`. The queued payload carries the same name
back out (§4's overlay derivation branches on it). Most apps never pass it — set is the write layout
components make.

**The task-node argument.** Every queue method takes the same optional `sectionId`, authorized against a
node of the work's step tree: `work.steps` (steps) → `step.steps` (tasks) → `task.steps` (action
sets). The id it wants is the **task** node's; the plausible-but-wrong value is that task's parent
**step** id, which produces writes the server rejects or misattributes. (`activeStepId` — the step, not
the task — isn't even on `WorkDetail`; it only appears on `WorkRow` from `work.list()` and on the flow
types, so `work.get()` won't hand it to you by that name.) Omit it — the shell resolves the work's
active task, same as the work page's own writes.
Resolving it requires the work to already be in the cache; if it is not, the call **throws** rather
than queueing a guessed id the server would reject at sync time.

### Creating work

`offline.queueWorkCreation({ flowId, targetTaskName })` and its online counterpart `work.start({
stepName })` both expect the flow config's **internal** `taskName` (e.g. `"Task_1"`) — no connector
exposes that value in that form today; see §8, gap 1 for the workaround.

**`flowId` means something different on each path.** `queueWorkCreation`'s `flowId` must be the
concrete flow **version** id the flow's config was cached under — passing a `flowOriginId` instead is a
cache-miss and throws. `work.start()`'s `flowId` is more forgiving: the server resolves either a
concrete id or a `flowOriginId` (even a stale version id) to the latest published version. Don't reuse
one call's `flowId` value for the other without checking which shape you have.

**Finding your app's own works — no bookkeeping needed.** `offline.listDownloadedWorkIds(flowOriginId)`
already includes offline-created works: `queueWorkCreation` seeds the same cache this id scan reads, so
a work you just created offline shows up immediately. Union that with `work.list()` while online to get
the complete list — don't invent app-side storage (e.g. `localStorage`) to track "works I created."
Above all, never persist a work's **code**: `id` is stable across sync, but `code` starts as an
`OFFLINE-…` placeholder and swaps to the server's real code once synced, so a stored code goes stale
silently. Navigate and look things up by `id`.

This union isn't optional redundancy: once a queued `workCreation` item syncs, the shell removes that
work's cached entries without repopulating them (§6's "post-sync server truth" — same removal), so the
work briefly drops out of `listDownloadedWorkIds` until a network read brings it back. Unioning with an
online `work.list()` is what covers that gap.

**Custom work codes — both paths, two different mechanisms.** The server lifts a `workCode` key
out of the startAt request body and uses it **verbatim** as the work's code; the flow's code template
only mints one when none is supplied.

- **Offline**: `offline.queueWorkCreation({ flowId, targetTaskName, workCode })`. The code is used
  locally instead of the `OFFLINE-…` placeholder and — unlike the placeholder — **survives sync
  unchanged**: the outbox marks it `persistWorkCode` and replay sends it to the server. The call
  **throws** if the code collides (case-insensitively) with a queued creation or any cached work — and
  a supplied-but-blank/whitespace code throws too, rather than falling back to the placeholder — so
  surface those errors to the user.
- **Online**: no dedicated parameter — put it in the data document: `work.start({ flowId, stepName,
  data: { workCode } })`. That is the platform's own mechanism.

Two sharp edges. **Nothing checks uniqueness server-side** — the server assigns the code without a
lookup, so two creations with the same code produce two works with the same code, silently; the
offline path's local check only covers what that device knows (its outbox + cache). And a custom code
**bypasses the flow's code template counter** entirely, so mixing custom and generated codes leaves
gaps in the generated sequence. Codes are display identity, not identity: keep navigating and looking
things up by `id`.

### Submitting: two shapes, plus a payload contract queued submits must get right

Online, `work.submit({ workId, actionName })` where `actionName` is `action.eventName ?? action.name` —
a label, not an id.

Queued, `offline.queueWorkAction({ workId, actionId })` — and that is the whole call. The shell
resolves everything else off the cached work: event name, step name, the task node, the queue-row
label, and the configured invitees (resolved against current work data, exactly as the work page
does). An `actionId` the work does not have **throws at the call site** rather than queueing a
malformed replay — so read the actions fresh from `(await work.get(workId))?.actions ?? []` (null =
the work does not exist, §4) instead of holding them across renders; a stale render's id may already
be gone.

- `recipients?` overrides only `to`/`cc` per destination. Supply it **only** when the flow config
  resolves nobody and the action still needs a recipient — the case the work page shows a picker
  for. Omitting it is correct everywhere else: the shell computes the config default, which also
  keeps the server able to derive recipients on replay — a non-null `invitation` in the submit body
  stops the server deriving recipients from flow config itself, so an empty list is never neutral.
- `sectionId?` defaults to the task holding the action; pass one only to target a different task.

`work.submit` also takes an optional `stepName` to disambiguate same-named actions — canonically the
**leaf action-set step's name**, not its parent step's. Its online failure modes are split, and
neither is a clean error. A `stepName` the flow does not have **fails on the server and surfaces as
a 403 "Access Denied"** — the `startAt` path swallows the real message entirely, `submit` at least
leaks it — so a mystifying 403 here can mean "bad step name", not "no permission". A bad **action**,
by contrast, is a **silent no-op**: when the server's can-execute check comes back false — wrong or
mismatched event name, no current action to execute, or a `PartyName` role mismatch — nothing aborts,
the transaction commits, and the request answers `200` with the work unchanged. The trap bridging the
two: the step resolver also accepts a step's **display name**, which resolves — to the wrong node of
the step tree — and the action lookup under it then finds nothing, landing you in the silent `200`,
not the 403: a submit passing the step's display name answers `200` and never advances, while the
canonical leaf name advances on the same event. A 200 therefore proves nothing; the
re-read below is the only honest signal.

**Queuing an action is not confirmation the workflow advanced.** `queueWorkAction` resolving means
the item queued; `allSynced` means it replayed — neither means the step moved. An app that knows its
flow is free to render the next step optimistically; the *confirmation* is a re-read (`work.get`)
once `allSynced` flips (§6).

**And `allSynced` still isn't proof the workflow moved.** The server can answer a submit with `200`
without transitioning the work at all — its can-execute check comes back false without erroring when
the action is gated by `PartyName` role permissions, when the event name doesn't match, or when no
current action resolves — so the queue drains clean and the step is exactly where it was. Nothing
an app sends changes this, and no client-side check detects it: the only honest signal is the
re-read. Surface the work's actual state rather than reporting success from a drained queue, and if
you see a submit that syncs but never advances, suspect the flow's action permissions before your
own code.

### Attachments

**Uploads return no id, on either path.** Both `attachments.upload(args)` and
`offline.queueWorkAttachmentUpload(args)` resolve to `void` — there is no way to correlate your own
upload with the resulting attachment record until you re-read the work. Don't design a flow that needs
the new attachment's id immediately after upload; re-fetch first.

**Deletion's args differ far more than `patchData`'s.** `attachments.delete({ id })` takes just the id
online. `offline.queueWorkAttachmentDeletion({ workId, id, fileName, folder, mimeType })` needs four
more fields queued — there is no shell-side lookup that backfills them the way `patchData`'s `originId`
gets defaulted on the queued data-update path. Read them off the work's own attachments —
`(await work.get(workId))?.attachments` (guard the `null`, §4) carries `id`, `fileName`, `mimeType`,
and `path`: pass `path` as `folder`, verbatim. The stored `path` IS the sanitized folder string the
shell matches components on (see Folder binding, below).

**Folder binding — exact-match, case-sensitive, and not an online/offline divergence.** An upload is
visible in an Attachments/AttachmentsGrid layout component **if and only if** the `folder` your app
passes equals that component's `layoutComponent.properties.folder` byte-for-byte, case-sensitive — the
shell's filter is exact string equality against the attachment's stored path. No prefix matching, no
normalization; an attachment can upload successfully and still
never appear in the component bound to a slightly different folder string.

- An attachment record has **no `folder` field** — the folder is stored as the attachment's `path`. Upload posts to `/api/v4/attachments/work/{workId}/{folder}`, and the server
  writes that path segment into `Path`.
- If a component's `properties.folder` is unset, the component **defaults to the literal string
  `'unknown'`** — a rendering default, not a business concept. For such a component your app must pass exactly `folder: "unknown"`, not omit the
  argument or invent a name.
- `dataPath: "$.attachments"` is a red herring for targeting — every Attachments component on a work
  shares that same array. `folder` is the only discriminator between them.
- The server sanitizes the folder (characters with ASCII code ≤ 44 and invalid filename characters
  become `-`) — keep folder values to letters/digits/hyphens. A value with spaces,
  `/`, or punctuation comes back different from what was sent and silently breaks the exact-match
  filter even when client and server "agree" on the string you typed.
- `attachments.upload`, `offline.queueWorkAttachmentUpload`, and the offline sync replay all encode
  `folder` identically into the same endpoint — if an upload is invisible, look at the folder string,
  not at which write path produced it.
- The typed connector surface doesn't expose a component's configured folder at all (§8, gap 2), and a
  core validation defect means an unconfigured "required" Attachments component can never pass
  validation (§8, gap 3) — configure an explicit `folder` on any Attachments component you mark
  required.

## 6. Stay fresh

There is no subscription API on the public connector for connectivity or sync changes —
`subscribeFlowDownload`/`subscribeOnline` exist only on the raw `tools.offline` handle inside
`onOfflineDownload` (§3), not here. Build freshness from three signals, and never a timer:

1. **Connectivity** — listen to the browser's own `window` `online`/`offline` events, not a polled
   `offline.isOnline()`. Call `isOnline()` inside the event handler if you need the synchronous local
   answer at that moment.
2. **Re-read after your own writes** — `work.get()`/`work.getData()` are network calls whenever the app
   is online (§4's read ladder). Call them right after a write you just made.
3. **One explicit Refresh control for everything else** — a sync status bar or queue table should
   update from a user-triggered Refresh action that calls the cheap local reads
   (`offline.getWorkSyncStatus(workId)`, `offline.listQueuedWorkOperations({ workId })`, both
   synchronous and network-free), not from `setInterval`.

Polling work reads on a timer does two kinds of damage: if you re-seed form state from the response, it
wipes out whatever the user is mid-typing; and once a work list unions an online `work.list()` call
(§5), a timer turns that into a repeated full-list network hit that floods the API. Neither failure
mode needs a timer to avoid — a Refresh button and the two signals above cover every legitimate case.

**Post-sync server truth needs a NETWORK re-read — there is no `ensureWork`.** When a queued
`workCreation` item syncs, the shell's own sync orchestrator
**removes** the cached entries for that work (`getWork`/`getWorkData`/`getMyWorkRoles`) once the whole
sync batch settles — it does not update them in place. There is no method anywhere to
revalidate a single entry, and nothing repopulates a removed one. Practically: once `allSynced`
flips for that work — not when `requestSync()` resolves, which only means "asked" (§7) — don't trust a
cache-only read of it; call `work.get(workId)` (or `work.getData`), which is network-first while online
by design and will hit the server rather than hand back a stale local copy. If the app also uses the
`@rise-x/apps-sdk/query` hooks (§4), call `invalidateAppSdkQueries(useAppQueryClient())` at the same
moment — otherwise those hooks keep serving react-query's pre-sync cache.

## 7. Sync + queue visibility

`offline.requestSync(args?)` only *asks* the shell to drain the outbox — it resolves as soon as the
request is posted ("asked", not drained: it does not even wait for the attempt), and it silently
no-ops while offline or before a session exists. There is no bridge method that guarantees — or even
observes — a sync completing synchronously with your call; poll `getWorkSyncStatus` (via the Refresh
pattern in §6) for the outcome. Surface `getWorkSyncStatus(workId).hasError` in the Refresh control —
true means a sync attempt failed and the queue still holds the items.

`offline.listQueuedWorkOperations(args?)` returns `{ id, kind, workId, payload, queuedAt, isSynced }`,
newest last — the order replay follows. `payload` is the operation's data exactly as queued (shape
varies by `kind`), so this call answers *what* is pending as well as *that* something is — §4 shows
using it for the pending-edits overlay.

## 8. Known platform gaps

Each of these is a real limitation in the current platform, not a mistake in your app code. Each entry:
what's missing, the workaround, and what a fix would look like.

1. **`targetTaskName` for work creation has no connector-exposed form.** `offline.queueWorkCreation({
   flowId, targetTaskName })` and `work.start({ stepName })` both expect the flow config's internal
   `taskName` (e.g. `"Task_1"`) — `flows.get()` only gives step names (`"Step_1"`, and passing one
   throws `Task Step_1 not found in flow <name>`), and
   `flows.getConfig()` maps a task's `name` to a `taskDisplayName` ("Task 1"), also wrong. Workaround:
   read the raw cached flow document — the `taskName` on each step node of
   `getShell().getCache().readFlow(flowId)` — falling back online to
   ``getShellApiV4('config').get(`/api/v4/config/flow/${flowId}`)``. Fix would look like:
   `FlowConfigTask` carrying the internal `taskName` alongside the display name.

2. **An app can't discover a component's folder through the typed surface.** `LayoutComponent` (from
   `@rise-x/apps-sdk/connectors`) declares only
   `component`/`name`/`label`/`dataPath`/`components` — no `properties`. At runtime the raw records
   pass through as-is (a cast, not a rebuild), so `properties.folder` IS present on the object, just
   not on the type.
   Workaround: cast and read `properties.folder`, defaulting to `"unknown"` yourself when it's absent.
   Fix would look like: adding `properties` to the `LayoutComponent` type.

3. **Required-validation on an Attachments component doesn't apply the `'unknown'` default that
   rendering uses.** Validation compares against the raw configured folder while rendering defaults an
   unset one to `'unknown'` — so a required Attachments component with no folder configured can never
   validate.
   Workaround: always configure an explicit `folder` on any Attachments component you mark required.
   Fix would look like: aligning `validate` with render's default.

4. **A synced work creation temporarily disappears from the offline work list.** After a queued
   `workCreation` item syncs, the shell removes that work's cached entries without repopulating them
   (§6), so it drops out of `offline.listDownloadedWorkIds()` until a network read brings it back.
   Workaround: union `listDownloadedWorkIds()` with an online `work.list()` (§5) rather than trusting
   the offline id list alone. Fix would look like: the sync orchestrator repopulating the cache entry
   instead of only removing it.

5. **Asset-backed screens have no offline support at all.** The asset-draft pattern
   (`assets.create`/`assets.startEdit` → `work.patchData` → `work.submit`) is network-only end to end —
   nothing seeds the offline cache for the draft, entity flows are served by a separate API family the
   offline download never touches, and there is no queue or download primitive for assets.
   Workaround: treat asset screens as online-only and gate them on `offline.isOnline()` with an explicit
   offline empty-state. Fix would look like: offline cache seeding and queueing for asset drafts and
   their entity-flow config.

## 9. Dev and test loops

Where things live — apps have two homes, and the offline surface is identical in both:

- **Standalone, in your own project** — the common case: a self-contained app with its own lockfile,
  `@rise-x/apps-sdk` from the public npm registry; no workspace at the repo root.
- **Under `apps/*` inside the `rise-x-app` monorepo** — Rise-X internal: an app living inside the
  shell's own repo as a workspace member, `@rise-x/apps-sdk` via `workspace:*`.

Either way, the shell that hosts your app is the platform's — you deploy into it.

Two loops — see `references/build.md` for the scaffolding both assume:

- **Standalone** (`pnpm start`, `createMockShell()`): instant rebuilds, but the mock shell returns
  `null` from `getOffline()` and has no cache wired — nothing offline-related is real here, only layout
  and pure UI. The two failure modes are distinct: OFFLINE connector methods (`offline.*`) throw
  `SHELL_TOO_OLD` down this path — the offline connector maps a missing handle or method to that code —
  while other connector calls fail with the mock's general no-backend behaviour, `SHELL_UNAVAILABLE`
  (see `references/build.md`). Both are worth testing (your error/empty-state handling). The
  standalone `fixtures` mechanism feeds connector *reads* only — nothing makes the mock's
  `getOffline()` non-null, so no offline behaviour becomes real here.
- **Deployed to the test environment** (build → zip → deploy via the Rise-X MCP, §10): the only place
  offline behavior is real. The hosted shell is a production build, so the service worker, the
  download orchestration, the queue, and sync all work there exactly as they will for users — verify
  everything offline in a browser against it, per "Prove it works" below.

**Seeding a flow's offline flag for testing** goes through the same draft cycle as §2 — and
double-check you're patching `featureFlags`, not `flowFeatures`.

Testing offline needs the flow's own data downloaded first (`offline.downloadFlowWorks`, or the app's
download card) — an offline-enabled app with no downloaded flow data is testing the offline-miss
**throw** path, not the happy path.

### Prove it works — six checks that define done

Six checks. The first four run against the deployed test environment — the hosted shell is where
offline is real, with the flow's data downloaded; the fifth runs standalone against the mock shell;
the sixth back on the deployed environment.

1. **Cold boot.** Download the app and its data, cut connectivity, reload the shell. The app's screens
   render from cache — not a skeleton, not a spinner.
2. **Both offline read states.** A downloaded work renders its data; a work that was never downloaded
   surfaces your error/empty state via the read **throwing** (unreachable + nothing cached — it does
   not resolve `null`). An infinite spinner here means the app's own loading state never settles —
   the read (hook or direct call) either answers or errors; render both.
3. **Offline write.** An edit queues: the shell's Sync view lists the item, your overlay shows the
   pending value, and the raw read still returns the pre-edit value (§4 — that is correct behavior,
   not a bug to fix).
4. **Reconnect and drain.** After sync, `allSynced` flips, a network re-read shows server truth, and an
   offline-created work still resolves by the same `id` with its code swapped off `OFFLINE-…` (or kept,
   if you passed a custom `workCode`).
5. **Degraded host.** Standalone against the mock shell, every offline-dependent surface shows its
   error/empty state via `SHELL_TOO_OLD` — no crashed screen, no raw `TypeError`.
6. **Flag misconfiguration probe.** On a flow WITH the offline flag,
   `offline.getWorkSyncStatus(workId).writeMode` reads `'queued'` while offline; on a flow without it,
   it reads `'direct'` (and the write would go to the network and fail) — the §2 misconfiguration that
   no other check catches.

## 10. Deploy checklist

Full deploy mechanics — zipping the bundle, the exact MCP calls — are in
`references/build.md` §Build and deploy; this list is what's specific to an offline-enabled app.

- Deploys go through the Rise-X MCP (`request_bundle_upload` → `deploy_app`) or the shell's New App /
  New release form.
- Zip the **contents** of `dist/`, not the folder — `remoteEntry.js` must sit at the archive root.
- Confirm the offline flags actually shipped: pass `feature_flags` on `deploy_app` and read the
  manifest back (`get_app` — the flags echo as `featureFlags`), and on every flow the app depends on,
  the flow-level flag plus quick submit (§2). A deploy that misses one ships an app that looks normal
  and silently is not offline-capable.
- On an offline-enabled app, a version bump makes every offline user's downloaded copy `stale`; the
  card's update path (download the new version, then prefix-delete the old one) is one click, but a
  deploy is not invisible to someone who downloaded the previous version for offline use.

## 11. Minimal end-to-end example

Feature-detect implicitly via the connector's own `ConnectorError`, gate the write on `writeMode` read
fresh (§5), and read back immediately to prove the trap in §4 for real: a queued write's read-back
still shows the pre-edit value.

```ts
import { offline, work } from "@rise-x/apps-sdk/connectors";

async function submitQty(workId: string, dataPath: string, value: unknown) {
  try {
    // originId first (patchData requires it; the queued call defaults it, §5) — the network
    // read must not sit between the writeMode read and the write it routes.
    const snapshot = await work.get(workId);
    if (!snapshot?.flowOriginId) throw new Error(`work ${workId} not found`);

    const { writeMode } = offline.getWorkSyncStatus(workId);
    if (writeMode === "queued") {
      // Synchronous — no await. sectionId and originId both omitted; the shell resolves
      // the active task and defaults originId from the work (§5).
      offline.queueWorkDataUpdate({ workId, dataPath, value });
    } else {
      await work.patchData(workId, {
        originId: snapshot.flowOriginId,
        path: dataPath,
        value,
      });
    }
  } catch (e) {
    // ConnectorError("SHELL_TOO_OLD", …) on a host with no offline support at all. Surface it —
    // swallowing here is the silent success §3 warns against.
    console.error("write failed", e);
    throw e;
  }

  // On the 'direct' path this is the server's own echo; on the 'queued' path this is still the
  // pre-edit cached value (§4) — don't expect it to reflect the pending write until it syncs.
  const data = await work.getData(workId, { path: dataPath });
  console.log("value now reads back as", data);
}
```
