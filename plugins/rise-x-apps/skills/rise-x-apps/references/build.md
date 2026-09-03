# Build phase — scaffold, app code, deploy

## When to bootstrap vs. write code

| Intent | Action |
| --- | --- |
| "Create a new app called X" / "Scaffold an app" | Design phase first (`references/design.md`). After approval, run the `init` CLI (see below). |
| "Add a feature to app <x>" / "Use shell user/env in my app" | Skip bootstrap. If the feature adds or changes UI, design phase first (`references/design.md`, mock → approval); then edit files under the app's `src/` using the patterns below. |
| "Wire installation behaviour" / "Migrate data on update" | Edit the app's `src/lifecycle.ts`. |
| "Build/deploy the app" | `pnpm build` in the app folder, then deploy via the Rise-X MCP (default: **test**) — see Build and deploy. |

## Bootstrap a new app

**Detect where you are first.** Apps are built both inside the rise-x-app
monorepo and by partners in their own projects — check whether the cwd is
inside rise-x-app (repo root has a `pnpm-workspace.yaml` listing `apps/*`).
Confirm naming with the user if unclear.

**Inside rise-x-app** — scaffold under `apps/<name>/`:

```bash
cd apps
npx @rise-x/apps-sdk init <name> --pm=pnpm
cd <name>
pnpm start
```

In-repo apps are pnpm workspace members (`apps/*` in `pnpm-workspace.yaml`).
The scaffolder writes a registry version for `@rise-x/apps-sdk` — change it to
`"workspace:*"` in the new app's `package.json` and run `pnpm install` at the
repo root so the app links to `packages/apps-sdk` directly. The repo's own
git conventions apply — commit only when the user asks.

**Move the approved mock in.** The design phase leaves it wherever the user
was working; put it at `design/<app>.v<n>.html` inside the new app now. §Build
and deploy checks the implementation against that path, and a mock left
outside the app is one nobody finds again.

**Outside the repo (partner project)** — scaffold wherever the user keeps
their code; `@rise-x/apps-sdk` resolves from the registry, so keep the
version the scaffolder writes:

```bash
npx @rise-x/apps-sdk init <name> --pm=pnpm   # drop --pm if pnpm isn't installed
cd <name>
git init && git add -A && git commit -m "chore: scaffold <name> Rise-X app"
```

Initialize git right after scaffolding (the template ships a `.gitignore`)
and maintain the app's history with **conventional commits** from the first
commit on (`type(scope?): summary`). The template ships
`commitlint.config.js` — verify with `npx commitlint --last`.

**Right after scaffolding, write the new app's `APP.md`** from the design
phase: a general description of the app and the problem it solves, the
personas, and their full user journeys. It's the app's living context — the
scaffolded `AGENTS.md` and `README.md` point at it, and every later change
reads and maintains it (see Writing app code).

CLI flags worth knowing:

| Flag | Default | Use |
| --- | --- | --- |
| `--pm=<npm\|yarn\|pnpm>` | auto | Force a package manager. **Prefer `pnpm` whenever it's available** — it's what Rise-X uses, and the commands throughout this guide assume it. Without the flag the CLI auto-detects from lockfiles in the cwd (falling back to npm), so pass it explicitly when scaffolding into an empty directory. |
| `--port=<n>` | `5101` | Standalone dev-server port. Must not collide with other apps — grep the sibling apps for `port:` in their `rsbuild.config.mts`, or `devServer.port` in a `webpack.local.config.js` if they predate the preset. |
| `--skip-install` | off | Skip install (faster scaffold; the user installs later). |
| `--json` | off | Emit `{ path, slug, scope, pkgName, port, pm, installed }` to stdout — useful for automation. |

What the scaffolder creates (**SDK >= 0.11.0** — earlier versions emit webpack configs):

```
<name>/
├── AGENTS.md              # agent guide: architecture in the Rise host, stack, commands, APP.md discipline
├── CLAUDE.md              # one-line @AGENTS.md import so Claude Code loads the same guide — edit AGENTS.md
├── README.md              # app readme; points at APP.md and AGENTS.md
├── package.json           # private; `start` = standalone dev, `start:federated` = MF dev, `build` = prod
├── tsconfig.json          # jsx: "react-jsx", strict
├── rsbuild.config.mts     # thin: calls defineAppConfig from @rise-x/apps-sdk/rsbuild
│                          # (the MF contract lives in the preset, not here)
├── public/index.html
└── src/
    ├── index.tsx          # `import('./bootstrap')` — MF async boundary
    ├── bootstrap.tsx      # standalone-only entrypoint; createMockShell
    ├── App.tsx            # THE CANONICAL APP LAYOUT (left-rail chrome + screens), exposed as ./App
    └── lifecycle.ts       # the module exposed as ./lifecycle
```

After scaffold there is **no Module Federation config in the app to edit** —
`rsbuild.config.mts` just calls `defineAppConfig` from `@rise-x/apps-sdk/rsbuild`
(SDK >= 0.9.0), and the preset owns the scope, `remoteEntry.js`, the `./App` +
`./lifecycle` exposes and every share, including the react family as
`singleton` + `import: false`. Read the scaffolded file if you need the shape.
An app may pass `exposes`, and `define` from **0.11.0**; it cannot add shares. The scaffolder
emits this config from **0.11.0** — earlier versions emit webpack configs, see
`references/upgrade.md`.

The scaffolded `src/App.tsx` **is the canonical app layout** (left rail from
the Nav primitives, PageHeader + content screens, no user UI). Build the app
by extending it — add screens, swap the stubs for real content — and preserve
its chrome composition; don't flatten it back to a bare component.

From **SDK >= 0.9.0** that composition is `AppFrame` / `AppRail` /
`AppContent` from `@rise-x/apps-sdk/ui`: `AppRail` is the nav rail and
`AppContent` is the app's ONE scroller. **From 0.11.0** the frame is also a CSS
container, so the layout answers to the region the host gave the app rather than
to the browser window; on 0.9.0 it is media-query based and answers to the
browser window.
Props are in `node_modules/@rise-x/apps-sdk/build/ui/components/app-frame.d.ts`
(`packages/apps-sdk/...` in the rise-x-app monorepo). Earlier SDKs hand-build
an `<aside>` rail in the template instead.

## Writing app code

**APP.md first.** Before changing an existing app, read its `APP.md`
(problem, personas, journeys) for context. When a change alters what the app
does or how a journey works, update `APP.md` in the same change. If the file
is missing, the app predates the convention — study the app and write one,
and check the other old-app signals too (`references/upgrade.md`): ask the
user about migrating to the current SDK + design system before piling new
work on old foundations.

### Runtime APIs from `@rise-x/apps-sdk`

```ts
import {
  // React hooks — call inside components
  useShellUser,
  useShellEnvironment,
  useShellNavigate,
  // Non-hook accessors — call outside React render path
  getShell,
  getShellUser,
  getShellEnvironment,
  getShellApi,           // legacy Diana axios instance
  getShellApiV4,         // typed: 'apps' | 'work' | 'config' | 'attachment' | 'asset'
  getShellAi,            // rise-x-ai gateway handle (bridge v3+), or null
  // Standalone dev
  createMockShell,
} from '@rise-x/apps-sdk';

// Domain connectors — typed wrappers over the raw clients (separate entry):
import {
  flows, work, assets, agents, ConnectorError,
  streamAgentReply, collectAgentReply, // consume agents.run/chat.send streams (accumulated)
} from '@rise-x/apps-sdk/connectors';

// react-query layer over the connectors — cached/deduped hooks (separate entry):
import { useFlows, useWorkRows, useSubmitWork, queryKeys } from '@rise-x/apps-sdk/query';
```

**Hooks vs accessors:**
- Hooks (`useShell*`) subscribe to changes — use inside components when you want re-renders on user/env switch.
- Accessors (`getShell*`) snapshot — use in event handlers, effects, non-React code (data stores, etc.).

**API calls** — prefer the typed connectors from `@rise-x/apps-sdk/connectors`; fall back to `getShellApiV4(name)` for endpoints they don't cover. Never instantiate your own axios.

| Connector | Domain | Key methods |
| --- | --- | --- |
| `flows` | flow discovery (read-only) | `list` (**work flows only**), `get`, `findTask`/`findTaskIn`, `getConfig`, `getLayout`, `flattenLayoutFields` |
| `work` | work items (read + write) | `start`, `get`, `getData`, `patchData`, `submit`, `delete`, `list`/`iterate`, `search`, `listRelated`, `getAudit` |
| `assets` | typed records ("entities"/"things") | `types` (**asset-type flows**), `get`, `search`, `quickSearch`, `list`/`iterate`, `listRelated`, `create`, `startEdit`, `clone`, `delete` |
| `agents` | configurable AI (config CRUD + streamed runs + server-persisted chats) | `list`, `get`, `create`, `update`, `delete`, `run`, `createChat`, `listChats`, `getChat`, `renameChat`, `deleteChat`, `getChatMessages` |

```ts
// Connectors (preferred) — flows discovery, work items, assets, configurable agents.
// Ship the flow's ORIGIN ID in app config; resolve everything else at runtime:
const FLOW_REF = '7732039e-…'; // flowOriginId (a concrete flow id also works here)
const flow = await flows.get(FLOW_REF); // resolves to the latest published version

const created = await work.start({ flowId: flow.id, data: { title: 'New' } });
await work.patchData(created.id, { originId: created.flowOriginId!, path: '$.title', value: 'Renamed' });
const action = (await work.get(created.id)).actions[0];
await work.submit({ workId: created.id, actionName: action.eventName ?? action.name! });
await work.delete(created.id); // permanent, no server-side undo

// Listing filters STRICTLY by flowOriginId — pass the flow object (or a bare
// origin id string). A concrete flow id here silently returns an empty list.
for await (const row of work.iterate({ flow, maxItems: 100 })) { /* … */ }

// Listing that needs each item's status, state, assignee or timestamps — or a
// filtered/sorted slice — belongs on work.search (SDK >= 0.7.0), not on
// list/iterate. The v4 index returns those ON the row, so you never fetch each
// item to derive them (the N+1 that makes dashboards slow), and the filter tree
// replaces client-side narrowing.
// EVERY v4 search (work AND asset) MUST pin flowOriginId with equals/in, at
// the top level or under `and` groups — an or-nested pin doesn't narrow and
// is rejected too. Environment-wide search is not supported: the server 400s
// on an unpinned search regardless of what else the filter says. The SDK
// types `filter` as required on both for this reason.
const page = await work.search({
  filter: { and: [
    // Required, always. `in` with several origin ids works too. Pin the
    // RESOLVED origin id, never FLOW_REF — that may hold a concrete version id,
    // which is a valid guid the index simply never matches (see below).
    { field: 'flowOriginId', operator: 'equals', values: [flow.flowOriginId] },
    // status values: Open/Closed/Completed/Deleted/Ok. Step-level values like
    // 'InProgress' live on flowState — a DIFFERENT field; filtering status by
    // them matches nothing.
    { field: 'status', operator: 'in', values: ['Open'] },
  ] },
  // Projection. A `data.*` path resolves against the PINNED flow's schema and
  // arrives under row.data with the `data.` prefix stripped.
  fields: ['status', 'assignedUsers', 'statusDisplay', 'data.total'],
  // sort and the createdBy/lastModifiedBy filters need a pinned search; on an
  // older API build they may fail, so pin first before assuming they're broken.
  sort: [{ field: 'created', direction: 'desc' }],
  pageSize: 50,
});
// row.status / row.flowState / row.assignedUsers / row.created / row.workCode /
// row.data — and row.statusDisplay carries the status label plus the active
// step's roleName. page.hasMore drives the next page.
// row.created / row.lastModified are ISO strings, but DATE VALUES INSIDE
// row.data arrive as {date, ticks, offset} objects — use the .date property,
// never new Date(row.data.x) directly.

// Assets: prefer typeOriginId / the AssetType object over the type name.
const supplier = (await assets.types()).find((t) => t.entityType === 'Supplier')!;

// assets.search (SDK >= 0.7.0) is the same v4 grammar as work.search, over the
// asset index — every reason to prefer it over list/iterate applies here too.
// It is also the indexed replacement for the v3 field search, which times out
// on large types.
// Same mandatory flowOriginId pin as work.search — see above.
const assetPage = await assets.search({
  filter: { and: [
    { field: 'flowOriginId', operator: 'equals', values: [supplier.flow!.flowOriginId] },
    // Asset status values: Open/Closed/Deleted ONLY. Work's Completed/Ok do not
    // exist here, and filtering by them matches nothing.
    { field: 'status', operator: 'equals', values: ['Open'] },
    // startsWith, NOT contains: substring matching on work/asset strings is
    // rejected with a 400 (see the operator restrictions below).
    { field: 'displayName', operator: 'startsWith', values: ['acme'] },
  ] },
  // Projection: a `data.*` path must EXIST in the pinned flow's data schema or
  // the request 400s naming the field (GET /api/v4/config/flow/{originId}/data-schema
  // lists them). Bare `data` skips that check and returns the whole document.
  fields: ['status', 'code', 'statusDisplay', 'data.supplier.rating'],
  sort: [{ field: 'created', direction: 'desc' }],
  pageSize: 50,
});
// row.code / row.entityType / row.status / row.created / row.data — same page
// envelope as work.search. `id` and `status` come back on every row even when
// `fields` omits them; other scalars drop out once `fields` is set.

// Fuzzy type-ahead across display fields is the one thing search can't do —
// that is assets.quickSearch (the v3 endpoint), and only for that.
const hits = await assets.quickSearch({ typeOriginId: supplier.flow!.flowOriginId, search: 'acme' });

const chat = agents.createChat({ agentId }); // memory is server-persisted
// streamAgentReply does the loop + delta accumulation; each snapshot.text is the
// full reply so far. (Assistant text streams as DELTAS — never read evt.data by
// hand.) For a one-shot answer: const { text } = await collectAgentReply(...).
try {
  for await (const s of streamAgentReply(chat.send('Summarize my open work'))) {
    setReply(s.text);                            // already accumulated
    // Report once, on end: agent error message, else a non-COMPLETED reason.
    if (s.done) {
      if (s.error) showError(s.error);           // mid-stream agent failure
      else if (s.reason !== 'COMPLETED') showError(s.reason); // TIMEOUT|CANCELLED|ERROR
    }
  }
} catch (err) {
  showError(String(err)); // transport/HTTP/abort failures THROW (not s.error)
}
// end_of_stream carries chat_id, captured on chat.chatId (and snapshot.chatId).
// Reopen later: agents.listChats({ agentId }) → createChat({ agentId, chatId })
// → getChatMessages(chatId) for the full transcript. useHistory:false = one-shot.
// Need reasoning/tool-call frames? Read the raw stream + agentMessageText/…readers.

// Raw client (fallback):
const apps = getShellApiV4('apps');
const { data } = await apps.get('/api/v4/config/apps');
```

**Reference by id, not by name.** Users rename flows, steps, tasks, and asset types freely (`displayName`), and internal `name`s change when a flow is rebuilt — ids are the only rename-proof reference. Bake `flowOriginId` (and task/step ids where needed) into app config; never hardcode display names. `flows.findTask({ flow, task })` matches by id, exact name, or case-insensitive displayName — the name forms are for dev-time exploration, not shipped code. When the user's input *is* a name (a search box), go through `flows.list({ search })` and let them pick.

**flowId vs flowOriginId.** A flow has one stable `flowOriginId` across versions plus a concrete `id` per published version — a bare "flow id" copied from a URL is usually the concrete one. `flows.get`/`getConfig` and `work.start` accept either and resolve to the latest published version, but `work.list`/`iterate` and `assets.list`/`iterate` filter strictly by origin id and return an **empty list, not an error**, when given a concrete id. When unsure, resolve first: `(await flows.get(ref)).flowOriginId`.

**`flows.list()` lists WORK flows only; asset-type flows come from `assets.types()`.** The two listings are **disjoint** — neither is a superset, and neither enumerates "all flows", so don't treat either as exhaustive. Only `flows.get()`/`findTask()` resolve a flow of either kind by id. This bites when sourcing the mandatory `flowOriginId` search pin: a work origin id in `assets.search` (or an asset origin id in `work.search`) is a valid guid of the wrong flow family, so it matches nothing and returns an **empty page with no error**. Work pin → `flows.list()`; asset pin → `assets.types()` (`type.flow.flowOriginId`).

**Search grammar limits** — identical for `work.search` and `assets.search`, since one server-side service backs both. The grammar's whole vocabulary is `equals`, `notEquals`, `in`, `notIn`, `contains`, `startsWith`, `endsWith`, `greaterThan`, `greaterThanOrEqual`, `lessThan`, `lessThanOrEqual`, `between`, `exists`, `notExists` — anything else 400s, so don't reach for SQL-ish spellings (`like`, `gt`, `>=`). **What a given field accepts is narrower than that, and knowing the list is not enough.** On work and asset search: **strings take `equals`/`notEquals`/`in`/`notIn`/`startsWith` only, and `contains`/`endsWith` are rejected with a 400** (unanchored regex is non-indexable, so they are allowed only on flow and company search); numbers and dates take equality and membership plus the four range operators and `between`; guid and array fields take equality and membership only; booleans take `equals`/`notEquals`. `data.*` string paths are cut back the same way as work/asset strings, on every resource. `exists`/`notExists` work on any field and take no `values`; `between` takes exactly two. **`pageSize` defaults to 25 and the server caps it at 100** — a larger value is clamped silently, so "fetch them all in one page" truncates with no error; page through `hasMore` instead. `hasMore` is always populated, an exact count is not: pass `includeTotalCount: true` to get `page.totalCount` when the UI shows "25 of 340", and leave it off otherwise — it runs a separate count facet over the whole match set on every request.

**Filter tree shape.** A node is either a leaf (`field` + `operator` + `values`) or a group (`and` **or** `or`) — never both, and never both group keys on the same node. `{ field: 'status', …, and: [...] }` and `{ and: [...], or: [...] }` are each rejected with a message naming the problem; wrap the leaf in its own group, or nest one group inside the other. Group nesting is capped at **5 levels** — a sixth returns `Filter group nesting exceeds maximum depth of 5`. Hand-written filters never approach that; it bites filter-builder UIs that let a user add nested condition groups without bounding the depth. All three arrive as a `ConnectorError` with `code: 'HTTP_ERROR'` — the server rejects the request rather than silently dropping conditions.

**Asset writes go through a draft work item** (the platform's edit model). `assets.create({ type })` and `assets.startEdit({ assetId, flowOriginId })` return the draft as a `WorkDetail` — fill it with `work.patchData()` and **save it with `work.submit()`**; nothing persists until the submit. An already-open draft is on `assets.get(id).draftWorkId` — resume it with `work.get()` instead of starting another. Asset types come from `assets.types()`; pass the whole `AssetType` (or its flow's origin id) as the `type` ref.

All connector failures normalize to `ConnectorError` (`code: 'SHELL_UNAVAILABLE' | 'SHELL_TOO_OLD' | 'AI_UNAVAILABLE' | 'HTTP_ERROR' | 'NOT_FOUND' | 'NETWORK_ERROR' | 'ABORTED' | 'PARSE_ERROR' | 'INVALID_ARG'`). `agents.run`/`createChat` need bridge v3 (`getAi`) and an environment with the AI gateway enabled — handle `SHELL_TOO_OLD`/`AI_UNAVAILABLE` gracefully. Chat memory is server-persisted: clients only handle `chatId` (`useHistory` defaults true; `chatId` + `useHistory:false` → `INVALID_ARG`). `listChats`/`getChat`/`renameChat`/`deleteChat`/`getChatMessages` read the chat store at `/api/v4/ai/agent-chat`.

### Fetching data in components — use the query layer

For component data, **default to `@rise-x/apps-sdk/query`** (react-query v5 over the connectors) instead of hand-rolling `useState`/`useEffect` fetches — caching, dedupe, refetch, and abort come free. The raw connectors remain the tool for event handlers, lifecycle hooks, and non-React code.

```tsx
import { useFlows, useWorkRows, useSubmitWork, dedupeRows } from '@rise-x/apps-sdk/query';

const { data: flows, error, isFetching } = useFlows();
const rows = useWorkRows({ flow: FLOW_ORIGIN_ID, pageSize: 50 }); // infinite; flatten with dedupeRows(rows.data)
const submit = useSubmitWork(); // submit.mutate({ workId, actionName }) — invalidates the right caches
```

Rules:
- **Never mount a `QueryClientProvider` with a client of your own** — that shadows the per-app client the shell mounts and breaks shell-managed caching. The SDK hooks need no provider at all: they pass the resolved client explicitly (host client when federated, per-bundle fallback standalone).
- **If the app calls react-query directly, wrap it in `<AppQueryProvider>`** (SDK >= 0.7.0, from `@rise-x/apps-sdk/query`; the scaffold's `App.tsx` already does). Plain APIs — `useQuery(flowQueries.list(args))`, `useQueryClient()`, devtools — read the client from context, and standalone dev has no provider, so they throw *"No QueryClient set"* the moment you leave the SDK hooks. `AppQueryProvider` publishes the *resolved* client, so it re-publishes the host's client when federated and the fallback when standalone.
- **The `@tanstack/react-query` share — `{ singleton: true, requiredVersion: '^5.0.0' }`, WITHOUT `import: false`** — belongs to the preset on a preset-built app, and to the app's own `shared` block on a webpack one. The missing `import: false` is deliberate: the bundled fallback keeps the app working on a host that doesn't share it. Don't shadow or remove it either way, or shell-managed caching breaks.
- Read hooks: `useFlows/useFlow/useFlowConfig/useFlowLayout/useFlowTask`, `useWork/useWorkData/useWorkRows/useWorkSearch/useRelatedWork/useWorkAudit`, `useAssetTypes/useAsset/useAssetSearch/useAssetQuickSearch/useAssetRows/useRelatedAssets`, `useAgents/useAgent/useAgentChats/useAgentChat/useAgentChatMessages`. All accept trailing `SdkQueryOptions` (`enabled`, `staleTime`, …); errors are `ConnectorError`.
- Mutations with built-in invalidation: `useStartWork`, `usePatchWorkData`, `useSubmitWork`, `useDeleteWork`, `useCreateAsset`, `useStartEditAsset`, `useCloneAsset`, `useDeleteAsset`, `useCreateAgent`, `useUpdateAgent`, `useDeleteAgent`, `useRenameAgentChat`, `useDeleteAgentChat`. If you write via a raw connector instead, call `invalidateAppSdkQueries(useAppQueryClient())` after.
- Keys are environment-scoped (`['rise-apps-sdk', envId, …]`) — ecosystem switches refetch automatically; `queryKeys` is exported for targeted invalidation. Advanced react-query features (select/suspense/prefetch) go through the factories: `useQuery(flowQueries.list(args))`.
- SSE streams (`agents.run`, chat `send`) and the async iterators stay on the connectors — don't wrap them in queries.

### UI components (`@rise-x/apps-sdk/ui`)

`import { Button, Dialog, cn } from '@rise-x/apps-sdk/ui'` — the Rise-X design
system (`@rise-x/ui`), and the **only allowed source of app UI: you MUST build
the UI exclusively from these components** — no hand-rolled markup for things
the system covers, no other component libraries, no one-off styles — they keep
every app on the shared Rise-X design. Only build custom when
the design system has no fitting primitive, and compose it from `cn` + the
existing pieces. Uphold the Rise-X experience principles
(`references/experience-principles.md`). Typings come from the SDK; the runtime
and its Tailwind CSS come from the **host** (the Diana app) via the Module
Federation share scope (the preset shares `@rise-x/ui` with no bundled
fallback) when running federated, and components follow the host's
light/dark theme automatically (tokens switch on a root-level `dark` class
the host controls). In standalone dev (`pnpm start`) the preset aliases
`@rise-x/ui` to the SDK's compiled standalone bundle instead — a
self-contained snapshot with its own CSS injected, so components render for
real without a host. It's a snapshot, not the source of truth: verify in a
host (or deployed) before release. Don't import `@rise-x/ui` directly and
don't add your own copies of its Radix/shadcn dependencies.

**App navigation lives on the LEFT — never in a top bar.** The host renders
its own top-bar navigation above every app, so an in-app top bar stacks two
nav bars and is not allowed. Put the app's navigation in a left sidebar/rail
(compose it from the design-system primitives) and keep the app's top edge
for content. On mobile widths, a **bottom tab bar** is the native pattern —
implement the mobile UX level chosen in the design phase (see
`references/design.md` §2 and the app's `APP.md`); the no-top-bar rule holds
at every width.

**SDK >= 0.11.0: don't hand-roll that bar.** Set `mobileNav="tabs"` on
`AppFrame` (the scaffold template already does) and `AppRail` renders it once
the frame is narrow — icon over label, safe-area padding, and a **More** sheet
for the overflow past a handful of destinations. The SDK README's AppFrame
section is the source of truth for that threshold and for `moreLabel`, which
localises the label. On earlier SDKs there is no such rail and the bar is
yours to build.

### Lifecycle hooks (`src/lifecycle.ts`)

Export any subset. The shell invokes them best-effort: errors are logged, **10s timeout per hook**, missing hooks skip silently. They run inside the shell page, so all the SDK accessors work from inside them.

```ts
import type { InstallHook, UpdateHook, UninstallHook } from '@rise-x/apps-sdk';
import localforage from 'localforage';   // or whatever you persist with

export const onInstall: InstallHook = async ({ manifest, user, environment }) => {
  // First time this device sees the app — seed defaults, pre-warm caches.
};

export const onUpdate: UpdateHook = async (ctx, { from, to }) => {
  // Version bumped in registry — migrate persisted data here.
};

export const onUninstall: UninstallHook = async ({ manifest }) => {
  // Drop anything you persisted. Be exhaustive — there's no second chance.
  await localforage.dropInstance({ name: `diana-app-${manifest.id}` });
};
```

`./lifecycle` is exposed by the preset for every app, so changing which hooks you export needs no config change — just keep the module at `src/lifecycle`.

### Standalone dev (outside the shell)

The scaffolded `src/bootstrap.tsx` already calls `createMockShell` when the standalone-root div is present, so `pnpm start` renders the app without running the full Diana shell.

**The mock has no backend.** Out of the box every connector call throws
`SHELL_UNAVAILABLE`, so a data-backed screen renders only its empty / loading /
error state locally — the content path (tables, charts, totals) never executes
until the app is deployed into a host. That is how content-path rendering bugs
reach production: standalone dev cannot see them, so their first real execution
is in front of a user.

**Seed `fixtures` so the real screens render before you deploy** (SDK >= 0.7.0).
This matters most when building **outside the `rise-x-app` monorepo** — the
common case — because there is no shell to run locally, which makes fixtures the
only pre-deploy test of the data path.

```ts
window.__DIANA_SHELL__ = createMockShell({
  user: { id: 'dev-user', name: 'Test User' },
  environment: { id: 'env-1', slug: 'dev', name: 'Dev Env' },
  fixtures: {
    // Rows are keyed by flowOriginId; '*' serves any flow. Served with the
    // request's paging, so pagination and work.iterate() behave as they would live.
    workRows: { '*': [{ id: 'w1', displayName: 'Q3 pricing review — Northline Industrial', workCode: 'PRC-2026-0184' }] },
    // Both search fixtures are served only when the request pins flowOriginId —
    // they reject an unpinned search exactly as the live endpoints do, so
    // standalone dev cannot pass where every host would 400.
    workSearch: [{ id: 'w1', workCode: 'PRC-2026-0184', status: 'Open', flowState: 'InProgress', created: '2026-01-31T09:14:00Z' }],
    workDetail: { w1: { id: 'w1', displayName: 'Q3 pricing review — Northline Industrial', actions: [] } },
    workData: { w1: { pricing: { rate: 1.425, currency: 'AUD' } } },
    // assets.types() is its own key — seed it whenever the screen resolves an
    // asset type before listing (the documented path).
    assetTypes: [{ id: 't1', entityType: 'Pump', flow: { flowOriginId: 'origin-a' } }],
    assetRows: { '*': [{ id: 'a1', displayName: 'Centrifugal pump 200mm — Bay 4' }] },
    // assets.search() rows; paged with the request's page/pageSize.
    assetSearch: [{ id: 'a1', displayName: 'Centrifugal pump 200mm — Bay 4', code: 'PMP-00184', status: 'Open', created: '2025-11-02T04:31:00Z' }],
    assetDetail: { a1: { id: 'a1', displayName: 'Centrifugal pump 200mm — Bay 4' } },
  },
});
```

Only seeded reads are served: an unseeded call throws `SHELL_UNAVAILABLE` naming
the fixture key that would have answered it, and writes always throw. Both are
deliberate — a mock that returned empty data, or pretended a write persisted,
would make a broken app look like a working one.

**Fixture data must look like real records, not `Item 1` / `foo` / `test`.** A
placeholder row renders a layout that live data will break. Specifically:

- **Real field names and shapes** — read an actual record via the Rise-X MCP or
  the platform UI and copy its structure; invented shapes test the app against a
  fiction.
- **The server's own enum values** — `status: 'Open'` (work: Open/Closed/
  Completed/Deleted/Ok; assets: Open/Closed/Deleted), step state on `flowState`.
  A made-up `'active'` renders a badge that never appears in production.
- **Plausible lengths and characters** — a name long enough to wrap or truncate,
  a code with its real format, non-ASCII where the domain has it. Uniformly
  short strings hide every overflow bug.
- **ISO timestamps**, and dates inside `data` in the platform's
  `{date, ticks, offset}` shape — otherwise date formatting is untested exactly
  where it is most likely wrong.
- **Enough rows to page** (more than one `pageSize`), plus at least one row with
  optional fields **missing** — that is the row that crashes a naive `.map()`.

To develop against a live backend instead, pass `api` / `apiV4` axios instances —
see the SDK README's standalone-dev section.

### Persistence

**Persist to the platform first.** App data belongs in the app's flow / asset / work-item integration (via the connectors) — that's where it's durable, shared, permissioned, and visible to the rest of the ecosystem. Reach for local persistence only for small device-local state (UI preferences, drafts, caches): install `localforage` (or use IndexedDB/Cache API directly) as a *direct* dep of your app, not via the SDK, and always clean up in `onUninstall` using a namespace tied to `manifest.id`.

## Build and deploy

```bash
cd <name>
pnpm build                                    # produces dist/
(cd dist && zip -r ../<name>-bundle.zip .)    # zip the CONTENTS of dist/, not the folder
```

**Why zip the contents, not the `dist/` folder itself:** the deploy pipeline extracts the archive under the app's served path. `remoteEntry.js` must be at the archive root so the shell can fetch it at runtime.

When the bundle is ready, **ask the user whether to deploy**. Two paths:

### Preferred — deploy via the Rise-X MCP

Load the `rise-x-mcp` skill first (mandatory before any Rise-X MCP call), then
pick the server for the target environment: **`rise-x-test` → test,
`rise-x` → production. If the user doesn't name an environment, deploy to
test** — most users want to try the app there first. Deploying requires the
environment-orchestrator role.

1. `request_bundle_upload` → returns a single-use, short-TTL `uploadUrl` plus an `uploadId`.
2. `curl -X PUT --data-binary @<name>-bundle.zip '<uploadUrl>'`
3. `deploy_app(upload_id, name, version, app_scope, app_id?, description?, icon?)` —
   omit `app_id` for a brand-new app (a GUID is generated and returned); pass
   the existing GUID to release a new version. `version` comes from the app's
   `package.json` and must be unique per app — bump it every release.
   `app_scope` is snake_case and **must match the MF scope the preset derives
   from the package name** (`@rise-x-apps/my-app` → `app_my_app`); the
   scaffolder's `--json` output reports it as `scope`.

The result carries the app `id` and the canonical manifest (`remoteUrl`,
`version`, `scope`, `deployedAt`). Confirm the app loads in the shell after.

### Fallback — deploy from the Apps UI (manual)

If the MCP isn't connected or the user prefers manual, the deploy dialog
lives on the **Apps page itself** (`/<environment>/apps`) → **New App** —
visible only to environment owners/orchestrators. (Under a local/localforage
registry this opens the register-manifest dialog instead — no upload.)

Provide these values for the user to copy/paste into the dialog:

| Field | Value | Notes |
| --- | --- | --- |
| `name` * | human-readable name |  |
| `version` * | from the app's `package.json` |  |
| `app_scope` * | `app_<slug_with_underscores>` | Must match the MF scope the preset derives from the package name. Regex: `^[a-z][a-z0-9_]*$`. |
| `bundle` * | the `.zip` you produced |  |
| `description` | short description of the app |  |
| `icon` | optional |  |

The app id is generated by the dialog (a Regenerate control on new deploys).
The host `POST`s to `/api/v4/config/apps/:id/deploy`; the server returns the
manifest with the live `remoteUrl` and `module` (defaults to `./App`).

## Don'ts

- **Don't try to shadow the preset's shares.** The preset shares react/react-dom/jsx-runtime as `singleton` + `import: false`, so the app uses the host's copy and MF rewires every `react` request regardless of what the app's `package.json` says. Re-declaring those shares yourself is what gives you cross-React-instance hook crashes when the app mounts inside the shell. Note that *declaring* `react` and `react-dom` is required, not forbidden — the template ships both in `devDependencies` and the preset's standalone branch resolves them from the app, so removing them breaks standalone dev.
- **Don't** import shell internals. The contract is `@rise-x/apps-sdk` — full stop. If you need something the SDK doesn't expose, propose it as an SDK addition in a separate PR (or request it from the Rise-X team) — don't reach into the host.
- **Don't** wire your own auth or call backend services directly. Go through the connectors (`@rise-x/apps-sdk/connectors`) or `getShellApiV4`.
- **Don't** hand-roll UI or pull in another component library — the app UI is built exclusively from `@rise-x/apps-sdk/ui` (see UI components).
- **Don't** put navigation in a top bar. The host already renders top-bar nav above the app; app navigation belongs in a left sidebar/rail.
- **Don't** assume the user or environment is non-null inside lifecycle hooks — accept the typed `ctx` and check.
- **Don't** modify `src/index.tsx`'s `import('./bootstrap')` line. That async dynamic import is the Module Federation boundary; the shell-side `eager: true` shares depend on it.

## Wrap-up — do this before reporting done

Whether you scaffolded a new app or modified an existing one, the user can't see the change in the running Diana shell until the bundle is deployed. **Always finish the task with build + zip + the deploy question** — never claim "done" without it.

1. **Verify the dev server still works.** `pnpm start` succeeds and the standalone app — rendering the real design system via the SDK's bundled snapshot — shows without console errors in the browser. For every data-backed screen, seed `fixtures` with **real-world-shaped** data (§Standalone dev) and confirm the **content** renders — rows, charts, totals — not just the empty state. A screen you have only ever seen empty is untested, and one you have only seen with `Item 1` placeholders is barely better; the deployed host is a customer environment, not a test bench.
2. **Verify against the approved design.** Open the approved
   `design/<app>.v<n>.html` next to the running app and compare screen by
   screen — layout, spacing, states, dark mode, and the mobile frame if the
   UX level has one. Fix drift in the app (or get the user's OK for the
   deviation) before going further.
3. **Run a code review.** Invoke the `code-review` skill with `--fix`
   (`/code-review --fix`) over the new app code and apply the fixes it
   confirms. If the skill isn't available, say so instead of skipping
   silently.
4. **Build.** `pnpm build` succeeds and produces `dist/remoteEntry.js`.
5. **Zip `dist/` contents** into `<name>-bundle.zip` (see Build and deploy above for the exact command — must zip the contents, not the folder).
6. **Ask the user whether to deploy.** If yes, deploy via the Rise-X MCP —
   **test environment unless the user names another** — following the steps in
   Build and deploy, and report the returned manifest (`id`, `version`,
   `remoteUrl`). If the MCP isn't available or the user prefers manual, output
   the deploy values for copy-paste:

   ```
   Deploy bundle ready: <name>/<name>-bundle.zip

   Open the Apps page (/<environment>/apps) → New App, and paste:

     name:       <human name>
     version:    <from package.json>
     app_scope:  app_<slug_with_underscores>
     bundle:     <name>/<name>-bundle.zip
     description: <short app description>
     icon:       (optional)
   ```

7. **Confirm the deploy landed.** Don't mark the task complete until the app loads in the shell — verify it yourself after an MCP deploy, or ask the user to confirm after a manual one.

If any of steps 1–3 fail, stop and fix before reaching out to the user. A failing build is not a "done" — it's a regression to be reported and resolved.
