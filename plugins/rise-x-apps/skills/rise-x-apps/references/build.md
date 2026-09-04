# Build phase — scaffold, app code, deploy

## When to bootstrap vs. write code

| Intent | Action |
| --- | --- |
| "Create a new app called X" / "Scaffold an app" | Design phase first (`references/design.md`). After approval, run the `init` CLI (see below). |
| "Add a feature to app <x>" / "Use shell user/env in my app" | Skip bootstrap. If the feature adds or changes UI, design phase first (`references/design.md`, mock → approval); then edit files under the app's `src/` using the patterns below. |
| "Wire installation behaviour" / "Migrate data on update" | Edit the app's `src/lifecycle.ts`. |
| "Build/deploy the app" | `pnpm build` in the app folder, then deploy via the Rise-X MCP (default: **test**) — see Build and deploy. |
| "Make the app work offline" | `src/lifecycle.ts`'s `onOfflineDownload`, plus offline-aware reads/writes (SDK >= 0.12) — see `references/offline.md`. |

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

**Also fill `rise-x-app.json`** with the integration targets from the design
phase: the scaffolded manifest declares `test` and no dependencies yet, and every
flow, asset-type, or agent id the app depends on is declared there — never in
source. Add another environment only once you have its ids (see App dependencies
below).

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
├── package.json           # private; `start` = standalone dev, `start:federated` = MF dev, `build` = prod, plus `validate`, `build:staging`, `build:prod` (SDK >= 0.12.0)
├── rise-x-app.json        # app dependency manifest (SDK >= 0.12.0): alias → per-environment ids; the build preset resolves it (no dependencies yet)
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

### App dependencies (`rise-x-app.json`) — no GUID literals in source

**Needs `@rise-x/apps-sdk` >= 0.12.0.** This section is the normative statement
of the gate; the other mentions repeat it and point here.

**A flow, asset-type, or agent GUID in app source is a bug.** Every such id the
app depends on is declared in `rise-x-app.json` at the app project root — one
stable **alias** per target, and one id **per environment**, so the same source
builds for every environment the app ships to. App code consumes them through
`deps.<alias>`, which is the recommended way to reach any Rise-X object the app
knows at authoring time. The generic connectors with a ref cover what a
dependency can't express: flows the *user* picks at runtime (a search box, a
flow selector), discovery (`flows.list`, `assets.types`), and the operations no
bound surface carries. Those stay fully supported; nothing is deprecated.

Below is a *mature* manifest, from an app that ships to all three environments —
it shows the full shape, not the starting point. A new app declares only `test`
(see Writing the file), so don't copy the extra environments in unless you have
their ids.

```json
{
  "environments": ["test", "staging", "prod"],
  "dependencies": {
    "riskFlow": {
      "kind": "flow",
      "label": "Risk Management workflow",
      "description": "Source of all risk work items",
      "ids": {
        "test": { "flowOriginId": "11111111-1111-1111-1111-111111111111" },
        "staging": { "flowOriginId": "1a1a1a1a-1a1a-1a1a-1a1a-1a1a1a1a1a1a" },
        "prod": { "flowOriginId": "22222222-2222-2222-2222-222222222222" }
      }
    },
    "vesselType": {
      "kind": "assetType",
      "label": "Vessel",
      "ids": {
        "test": { "flowOriginId": "33333333-3333-3333-3333-333333333333" },
        "staging": { "flowOriginId": "3a3a3a3a-3a3a-3a3a-3a3a-3a3a3a3a3a3a" },
        "prod": { "flowOriginId": "44444444-4444-4444-4444-444444444444" }
      }
    },
    "triageAgent": {
      "kind": "agent",
      "label": "Medication triage",
      "description": "Answers identification questions from a photo",
      "ids": {
        "test": { "agentId": "55555555-5555-5555-5555-555555555555" },
        "staging": { "agentId": "5a5a5a5a-5a5a-5a5a-5a5a-5a5a5a5a5a5a" },
        "prod": { "agentId": "66666666-6666-6666-6666-666666666666" }
      }
    }
  }
}
```

#### Writing the file

**Declare only the environments you have real ids for.** A scaffolded app starts
at `"environments": ["test"]` — that is where apps are built and first deployed.
Most apps stay there for a while: the user usually has test ids and nothing else,
and that is a complete, valid manifest, not a half-finished one.

The rule that makes this matter: **every dependency needs an id for every
environment listed in `environments`.** The alias sits above `ids`, so the alias
set is identical across environments and each alias has to resolve in each one.
That gives you exactly one correct way to handle an id you don't have:

- ✅ **Leave the environment undeclared.** Ship `["test"]`, add `"staging"` or
  `"prod"` to `environments` on the day you have their ids, adding the matching
  `ids` block to every dependency in the same edit.
- ❌ **Never invent, guess, copy, or reuse an id** to fill a gap — not the test
  id in the prod slot, not a zero GUID, not a placeholder. A wrong-but-valid
  GUID passes every local check and fails at deploy, or worse, resolves to some
  unrelated object in that ecosystem.
- ❌ **Never leave one dependency's ids partial** while its environment stays
  declared. That build fails, and if you were to force it through, the bundle
  would ship without that dependency and the app would throw
  `unknown app dependency` in production.
- ❌ **Never delete a declared environment just to make `pnpm validate` pass.**
  If `prod` is declared, something ships there. Removing it is a decision for
  the user, not a way to get a green check.

If you need an id you don't have, **ask the user**, or look it up in that
ecosystem with the Rise-X MCP (`list_flows`, `list_asset_types`, `list_agents`
against the server for that environment — `rise-x-test` for test, `rise-x` for
production). The plugin ships those two servers only, so staging ids come from
the user or the staging shell, not from an MCP lookup. Discovery per environment
is the whole reason ids are keyed this way.

Fill `label` and `description` while you are there. They are optional to the
parser and not optional in practice: they are what ecosystem management and Ask
Diana show a human reading the app's dependencies later.

| Field | Required | Meaning |
| --- | --- | --- |
| `environments` | yes | the environments this app is built for; every `ids` key must appear here, **and every dependency must supply an id for each** |
| `<alias>.kind` | yes | `flow`, `assetType`, or `agent` |
| `<alias>.label` / `.description` | no | human-readable name and what the app uses it for — **fill them**: they feed ecosystem management and Ask Diana, not just the reader |
| `<alias>.ids.<env>.<idField>` | yes | the id on that environment (GUID) — the field name follows the `kind` |

**The id field name follows the `kind`**, and the wrong field for a kind is a
mismatch, not an extra to ignore: the build fails with a message naming the
field that kind takes.

| `kind` | Id field | Why |
| --- | --- | --- |
| `flow` | `flowOriginId` | the origin id, stable across the flow's versions |
| `assetType` | `flowOriginId` | an asset type is defined by an entity flow |
| `agent` | `agentId` | an agent has no origin id and no version chain — one agent is one document with one id |

**An agent's id always differs between test and prod.** Each agent is created
per ecosystem and there is no promotion path, so no single id works
everywhere: the per-environment block is mandatory for an agent, never
optional.

**What declaring an agent gives you — and what it doesn't.** It records the
agent as a declared dependency (ecosystem management, Ask Diana) and gives app
code the bound surface below. It does **not** make the agent invocable by
Diana: no MCP tool runs or spawns an agent, so the agent runs only when the
app's own code calls it.

The ids come from the same discovery as before — the Rise-X MCP
(`list_flows`, `list_asset_types`, `list_agents`) or the user — they just land
in the manifest, per environment, instead of in source (see
`references/design.md` §3).

**Per-environment builds.** The scaffolded build config wires the app-manifest
plugin for you — `defineAppConfig` from `@rise-x/apps-sdk/rsbuild` includes it,
so there is nothing to add (an app still on a hand-written webpack config adds
`new RiseAppManifestPlugin()` from `@rise-x/apps-sdk/webpack` instead). It
resolves the manifest for **one** environment — `APP_ENV`, default `test` — and
emits the flat, single-environment result as `dist/rise-x-app.json`, so it rides
the bundle zip. That emitted file carries a single `environment` (singular) plus
the resolved dependencies, while the source manifest is the one that lists
`environments`. Ids for other environments never ship. The build fails, naming
the alias and environment, on an unknown environment, a missing id, an
unsupported `kind`, an id field that doesn't belong to the `kind`, a non-GUID or
empty-GUID id, an alias or environment name that isn't a 1-64 character
identifier (letters, digits, `.`, `-`, `_`, starting with a letter or digit), a
`label` over 200 or a `description` over 1,000 characters, a control character in
either, two aliases that differ only by case, or more than 100 dependencies.
These are the deploy's own shape rules, mirrored so a manifest that builds also
deploys.

```bash
pnpm build                                    # → environment "test"
pnpm build:staging                            # → environment "staging"
pnpm build:prod                               # → environment "prod"
```

Those are the template's own scripts — `cross-env APP_ENV=<env>` in front of the
build. Go through `cross-env` for any environment you add: the inline
`APP_ENV=<env> pnpm build` form doesn't work in PowerShell or cmd.exe.

The staging and prod scripts ship ahead of the environments themselves, so on a
new app they fail until you declare the environment — with the list of what *is*
declared, which is the answer to "why won't this build":

```
rise-x-app.json is invalid:
  - unknown environment "prod" — declared environments: test
```

That is a manifest to extend (Writing the file), never a script to work around.

**Check every environment before you ship, not just the one you build.** A build
resolves only its own target, so `pnpm build` proves the test ids are complete
and says nothing about any other environment — a broken block there surfaces at
promotion, in someone else's hands:

```bash
pnpm validate                                 # every environment the manifest declares
pnpm validate -- --env=prod                   # just one
```

It exits non-zero if any environment fails, names the alias and environment, and
prints the ids each environment would ship. Run it after every edit to
`rise-x-app.json` and before building a bundle to deploy. `--json` emits
`{ ok, manifest, environments, scan }` if you need to read the result rather than
the report. Validation is **structural and offline**: it cannot tell you an id has
been deleted from the target ecosystem, and neither can the deploy, which only
checks the manifest's shape (see Build and deploy). A stale id surfaces in the
running app.

#### Finding ids already hardcoded in the code

A perfect manifest is no use if a GUID is pasted into a component anyway, so
`validate` also scans the app's TypeScript and lists Rise-X ids the manifest does
not declare. To run only that pass — and this is the command to reach for when
**migrating an old app**, or when you want to know whether an app complies at all:

```bash
npx @rise-x/apps-sdk scan                 # undeclared ids, with kind, alias and file:line
npx @rise-x/apps-sdk scan --show-ignored  # ...and what it set aside, and on what grounds
npx @rise-x/apps-sdk scan --json          # machine-readable, for working through a list
npx @rise-x/apps-sdk scan --strict        # exit non-zero on any finding (CI)
```

```
  Hardcoded ids — declare these in rise-x-app.json:

  77777777-7777-7777-7777-777777777777  flow
        alias?  riskMgmtFlow
        src/App.tsx:13  (RISK_MGMT_FLOW.id)
        src/App.tsx:14  (flowOriginId)

  1 undeclared id (5 dismissed by rule)
```

Findings inside `validate` are **warnings** and never change its exit code, so
never read a green `validate` as "no hardcoded ids" — read the warnings.

It classifies by reading names: the property a GUID is assigned to, the connector
or hook it goes into, the variable it is bound to. Each location names the
identifier the verdict came from, so you can check it. An id it cannot attribute
is reported with **no kind** rather than a guess — those are the ones to ask the
user about, or to resolve with `list_flows` / `list_asset_types` / `list_agents`
in that ecosystem.

`suggestedAlias` is a starting point, not an answer. It is trimmed of the words
that describe the id (`EXPENSE_FLOW_ORIGIN_ID` → `expenseFlow`), and it is
**null** when the source never named the object — a bare `agents.get('<guid>')`
has nothing to take a name from. An alias is the app's permanent handle on that
dependency and appears throughout the code, so confirm it with the user rather
than committing whatever the scan guessed, and never let two dependencies share
one.

**Check what it set aside before you trust a short list.** It skips what the
manifest doesn't carry: GUIDs used as object keys (a step-id lookup table), names
like `stepId` / `workId` / `assetId`, section and layout ids, integration
`endpointId`s, `sample`/`mock`/`fixture` data, and a bare row id passed to
`assets.get()` or `work.get()`. Those rules are a heuristic tuned on real apps, so
on an app they weren't written for they can drop a genuine dependency — one named
`seedFlow` would go. `--show-ignored` prints every dismissal with its file, line
and the identifier that triggered it, and the summary counts the scanner's own
judgements (`dismissed by rule`) apart from a human's (`silenced`, from a
`rise-x-app-ignore` comment). On a migration, read that list.

**It is a lint, not a proof.** It cannot see an id assembled at runtime or read
from config, so a clean report is evidence, not a guarantee. For a GUID that
genuinely isn't a Rise-X object, add `rise-x-app-ignore` in a comment on its line
or the line above — and say why:

```ts
// rise-x-app-ignore — correlation id for the audit trace, not a Rise-X object
const TRACE_ROOT = '0f9c1e77-...';
```

Never silence a finding you haven't understood, and never use the marker to get a
`--strict` run green.

**Runtime consumption.** The plugin injects the resolved manifest into the
bundle as a compile-time constant, so the accessors are synchronous and the
returned map is referentially stable (safe as a hook/effect dependency):

```ts
import {
  useAppDependencies,        // hook form — inside components
  getAppDependency,          // one alias — outside React
  getAppDependencies,        // the whole map
  type BoundFlowDependency,
  type BoundAssetTypeDependency,
  type BoundAgentDependency,
} from '@rise-x/apps-sdk/dependencies';

// Declare the app's aliases once for typed access without narrowing:
interface AppDeps {
  riskFlow: BoundFlowDependency;
  vesselType: BoundAssetTypeDependency;
  triageAgent: BoundAgentDependency;
}

const deps = useAppDependencies<AppDeps>();
deps.riskFlow.flowOriginId;                       // the raw id + kind/label/description
deps.triageAgent.agentId;                         // an agent names its id field agentId
const open = await deps.riskFlow.work.search({    // pre-bound connector call
  filter: { field: 'status', operator: 'equals', values: ['Open'] },
});
const vessels = await deps.vesselType.assets.list({ pageSize: 50 });
```

These names live on `@rise-x/apps-sdk/dependencies`, not the root, for the
same reason `/query` does: the subpath binds the connector layer, so an app
that never declares a dependency doesn't bundle it.

The resolved entries are a union over `kind`, so an unnarrowed `deps.<alias>`
carries only the fields its kind has — `flowOriginId` on a flow or asset type,
`agentId` on an agent. Declaring the aliases as above is how you skip the
narrowing.

Every dependency exposes the connector operations that take its ref, with the
ref pre-filled — each bound method is the existing connector call, nothing
more:

| `kind` | Bound surface |
| --- | --- |
| `flow` | `dep.flow.get()` / `getConfig()`, `dep.work.start()` / `list()` / `iterate()` / `search()` |
| `assetType` | `dep.assets.list()` / `iterate()` / `search()` / `quickSearch()` / `create()` |
| `agent` | `dep.agent.get()` / `run()` / `createChat()` / `listChats()` |

On the two bound `search()` methods `filter` is optional: the mandatory
`flowOriginId` pin **is** the dependency, so the bound call adds it and
`and`s your filter with it. The pin merges into a filter whose top level is
already an `and` group; a filter whose top level is an `or` group or a single
condition gets wrapped in a new `and`, which costs one of the five nesting
levels.

The agent surface binds only the agent-scoped operations. The chat operations
keyed by a **chat** id (`getChat`, `renameChat`, `deleteChat`,
`getChatMessages`) take a chat, not an agent, so they stay on the generic
`agents` connector — the streaming rules and error codes there apply
unchanged.

Anything not on the bound surface takes the id — `useFlowConfig(dep.flowOriginId)`
works for a flow or an asset type, since an asset type is backed by a flow
too, and `useAgent(dep.agentId)` for an agent — both take the raw id, so a
missing dependency surfaces before them: the accessors above throw a
`ConnectorError` naming the fix when no manifest reached the bundle, and
`getAppDependency` lists the declared aliases when the alias isn't one of them.

**Prefetch (optional).** Unlike the accessors, which throw on a missing
manifest because a read that returns nothing would hide a bug,
`prefetchAppDependencies` returns without doing anything when no manifest
reached the bundle: it is a cache warm, so there is nothing to fail loudly
about, and an app that ships without a manifest can keep the call. That is
what makes `void`ing it in an effect safe, and an SDK unit test pins it.
`prefetchAppDependencies(client)` from
`@rise-x/apps-sdk/query` warms react-query with what each declared dependency
needs: a flow or asset type gets its flow config and the layouts that config
points at; an agent gets its agent document — never a flow config, and never
its run stream, which is SSE and has nothing to cache. The client parameter is
required and must be the one from `useAppQueryClient()` — never construct a
second react-query client:

```tsx
import { prefetchAppDependencies, useAppQueryClient } from '@rise-x/apps-sdk/query';

const client = useAppQueryClient();
useEffect(() => { void prefetchAppDependencies(client); }, [client]);
```

**Standalone dev.** The preset runs the plugin in standalone mode too, so
`useAppDependencies()` works there with no extra wiring. Pass
`createMockShell({ dependencies })` only to *override* the built ids locally —
see §Standalone dev below.

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

// Declared app dependencies (rise-x-app.json): see App dependencies above for the SDK gate (separate entry):
import { useAppDependencies, getAppDependency, getAppDependencies } from '@rise-x/apps-sdk/dependencies';

// Domain connectors — typed wrappers over the raw clients (separate entry):
import {
  flows, work, assets, agents, ConnectorError,
  attachments, offline, // SDK >= 0.12 — the offline surface (references/offline.md)
  streamAgentReply, collectAgentReply, // consume agents.run/chat.send streams (accumulated)
} from '@rise-x/apps-sdk/connectors';

// react-query layer over the connectors — cached/deduped hooks (separate entry):
import { useFlows, useWorkRows, useSubmitWork, queryKeys } from '@rise-x/apps-sdk/query';
```

**Hooks vs accessors:**
- Hooks (`useShell*`) subscribe to changes — use inside components when you want re-renders on user/env switch.
- Accessors (`getShell*`) snapshot — use in event handlers, effects, non-React code (data stores, etc.).

**API calls** — work down this order and stop at the first rung that fits:

1. **A declared dependency** — `deps.<alias>` (from `@rise-x/apps-sdk/dependencies`) for every flow, asset type, and agent the app knows at authoring time. Its bound surface carries the connector operations that take that ref, pre-filled, so the id never appears in a call site.
2. **A query hook** — `@rise-x/apps-sdk/query` for data a component renders, passing `dep.flowOriginId` or `dep.agentId` as the ref. The bound surfaces return promises, so component fetches still go through the query layer.
3. **A generic connector** — `@rise-x/apps-sdk/connectors` for what a dependency can't express: a ref the *user* picks at runtime, discovery (`flows.list`, `assets.types`), and the operations no bound surface carries (the chat-id ones: `getChat`, `renameChat`, `deleteChat`, `getChatMessages`).
4. **`getShellApiV4(name)`** for endpoints no connector covers. Never instantiate your own axios.

| Connector | Domain | Key methods |
| --- | --- | --- |
| `flows` | flow discovery (read-only) | `list` (**work flows only**), `get`, `findTask`/`findTaskIn`, `getConfig`, `getLayout`, `flattenLayoutFields` |
| `work` | work items (read + write) | `start`, `get`, `getData`, `patchData`, `submit`, `delete`, `list`/`iterate`, `search`, `listRelated`, `getAudit`, `getMyAccess` |
| `assets` | typed records ("entities"/"things") | `types` (**asset-type flows**), `get`, `search`, `quickSearch`, `list`/`iterate`, `listRelated`, `create`, `startEdit`, `clone`, `delete` |
| `agents` | configurable AI (config CRUD + streamed runs + server-persisted chats) | `list`, `get`, `create`, `update`, `delete`, `run`, `createChat`, `listChats`, `getChat`, `renameChat`, `deleteChat`, `getChatMessages` |
| `attachments` | work-attachment blobs (SDK >= 0.12) | `getBlob`, `upload`, `delete` |
| `offline` | connectivity, the offline queue, and downloads (SDK >= 0.12) | see `references/offline.md` |

```ts
// Connectors (generic) — flows discovery, work items, assets, configurable agents.
// A flow the app knows at AUTHORING time is a dependency: declare it in
// rise-x-app.json and use its bound surface (deps.<alias>.work.…, see App
// dependencies above). The generic calls below take a ref — for flows the
// USER picks at runtime (from flows.list, a search box, a saved selection):
const flow = await flows.get(pickedRef); // origin or concrete id — resolves to the latest published version

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
    // RESOLVED origin id, never the picked ref — that may hold a concrete
    // version id, a valid guid the index simply never matches (see below).
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
// (A type known at authoring time is a dependency — deps.<alias>.assets.…;
// runtime discovery like this is for types the user picks.)
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

**Reference by id, not by name.** Users rename flows, steps, tasks, and asset types freely (`displayName`), and internal `name`s change when a flow is rebuilt — ids are the only rename-proof reference. Declare `flowOriginId`s in `rise-x-app.json` (task/step ids, which the manifest doesn't carry, stay in app config); never hardcode display names. `flows.findTask({ flow, task })` matches by id, exact name, or case-insensitive displayName — the name forms are for dev-time exploration, not shipped code. When the user's input *is* a name (a search box), go through `flows.list({ search })` and let them pick.

**flowId vs flowOriginId.** A flow has one stable `flowOriginId` across versions plus a concrete `id` per published version — a bare "flow id" copied from a URL is usually the concrete one. `flows.get`/`getConfig` and `work.start` accept either and resolve to the latest published version, but `work.list`/`iterate` and `assets.list`/`iterate` filter strictly by origin id and return an **empty list, not an error**, when given a concrete id. When unsure, resolve first: `(await flows.get(ref)).flowOriginId`.

**`flows.list()` lists WORK flows only; asset-type flows come from `assets.types()`.** The two listings are **disjoint** — neither is a superset, and neither enumerates "all flows", so don't treat either as exhaustive. Only `flows.get()`/`findTask()` resolve a flow of either kind by id. This bites when sourcing the mandatory `flowOriginId` search pin: a work origin id in `assets.search` (or an asset origin id in `work.search`) is a valid guid of the wrong flow family, so it matches nothing and returns an **empty page with no error**. Work pin → `flows.list()`; asset pin → `assets.types()` (`type.flow.flowOriginId`).

**Search grammar limits** — identical for `work.search` and `assets.search`, since one server-side service backs both. The grammar's whole vocabulary is `equals`, `notEquals`, `in`, `notIn`, `contains`, `startsWith`, `endsWith`, `greaterThan`, `greaterThanOrEqual`, `lessThan`, `lessThanOrEqual`, `between`, `exists`, `notExists` — anything else 400s, so don't reach for SQL-ish spellings (`like`, `gt`, `>=`). **What a given field accepts is narrower than that, and knowing the list is not enough.** On work and asset search: **strings take `equals`/`notEquals`/`in`/`notIn`/`startsWith` only, and `contains`/`endsWith` are rejected with a 400** (unanchored regex is non-indexable, so they are allowed only on flow and company search); numbers and dates take equality and membership plus the four range operators and `between`; guid and array fields take equality and membership only; booleans take `equals`/`notEquals`. `data.*` string paths are cut back the same way as work/asset strings, on every resource. `exists`/`notExists` work on any field and take no `values`; `between` takes exactly two. **`pageSize` defaults to 25 and the server caps it at 100** — a larger value is clamped silently, so "fetch them all in one page" truncates with no error; page through `hasMore` instead. `hasMore` is always populated, an exact count is not: pass `includeTotalCount: true` to get `page.totalCount` when the UI shows "25 of 340", and leave it off otherwise — it runs a separate count facet over the whole match set on every request.

**Filter tree shape.** A node is either a leaf (`field` + `operator` + `values`) or a group (`and` **or** `or`) — never both, and never both group keys on the same node. `{ field: 'status', …, and: [...] }` and `{ and: [...], or: [...] }` are each rejected with a message naming the problem; wrap the leaf in its own group, or nest one group inside the other. Group nesting is capped at **5 levels** — a sixth returns `Filter group nesting exceeds maximum depth of 5`. Hand-written filters never approach that; it bites filter-builder UIs that let a user add nested condition groups without bounding the depth. All three arrive as a `ConnectorError` with `code: 'HTTP_ERROR'` — the server rejects the request rather than silently dropping conditions.

**Asset writes go through a draft work item** (the platform's edit model). `assets.create({ type })` and `assets.startEdit({ assetId, flowOriginId })` return the draft as a `WorkDetail` — fill it with `work.patchData()` and **save it with `work.submit()`**; nothing persists until the submit. An already-open draft is on `assets.get(id).draftWorkId` — resume it with `work.get()` instead of starting another. Asset types come from `assets.types()`; pass the whole `AssetType` (or its flow's origin id) as the `type` ref.

All connector failures normalize to `ConnectorError` (`code: 'SHELL_UNAVAILABLE' | 'SHELL_TOO_OLD' | 'AI_UNAVAILABLE' | 'HTTP_ERROR' | 'NOT_FOUND' | 'NETWORK_ERROR' | 'ABORTED' | 'PARSE_ERROR' | 'INVALID_ARG'`). `agents.run`/`createChat` need bridge v3 (`getAi`) and an environment with the AI gateway enabled — handle `SHELL_TOO_OLD`/`AI_UNAVAILABLE` gracefully. Chat memory is server-persisted: clients only handle `chatId` (`useHistory` defaults true; `chatId` + `useHistory:false` → `INVALID_ARG`). `listChats`/`getChat`/`renameChat`/`deleteChat`/`getChatMessages` read the chat store at `/api/v4/ai/agent-chat`.

### Fetching data in components — use the query layer

For component data, **default to `@rise-x/apps-sdk/query`** (react-query v5 over the connectors) instead of hand-rolling `useState`/`useEffect` fetches — caching, dedupe, refetch, and abort come free. The raw connectors remain the tool for event handlers, lifecycle hooks, and non-React code.

Offline, the hooks answer from the platform's offline cache when the flow's data is downloaded, and settle into their **error** state (recovering on reconnect) when it is not (SDK >= 0.12 — see `references/offline.md`). Render both states: a work that was never downloaded errors rather than resolving, so a screen that only handles the happy path sits on a spinner that never settles.

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
localises the label. On earlier SDKs `AppRail` does not render one — on 0.9.0
it exists but goes to a horizontal scrolling strip at narrow widths — so the
bar is yours to build.

### Lifecycle hooks (`src/lifecycle.ts`)

Export any subset. The shell invokes them best-effort: errors are logged, **10s timeout per hook**, missing hooks skip silently — except `onOfflineDownload` (SDK >= 0.12), which is user-initiated, may run minutes, and where throwing **fails the download** (see `references/offline.md`). They run inside the shell page, so all the SDK accessors work from inside them.

```ts
import type { InstallHook, UpdateHook, UninstallHook, OfflineDownloadHook } from '@rise-x/apps-sdk';
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
is in front of a user. The `offline` connector (SDK >= 0.12) is the exception: its methods
throw `SHELL_TOO_OLD` when the offline bridge isn't available — all but
`offline.isOnline()`, which answers `true` there instead (`references/offline.md`).

**Seed `fixtures` so the real screens render before you deploy** (SDK >= 0.7.0).
This matters most when building **outside the `rise-x-app` monorepo** — the
common case — because there is no shell to run locally, which makes fixtures the
only pre-deploy test of the data path.

```ts
window.__DIANA_SHELL__ = createMockShell({
  user: { id: 'dev-user', name: 'Test User' },
  environment: { id: 'env-1', slug: 'dev', name: 'Dev Env' },
  // Overrides the resolved rise-x-app.json dependencies. The
  // build injects them in standalone mode too, so this is for pointing local
  // dev at different ids — or for an app that has no manifest yet.
  dependencies: {
    riskFlow: { kind: 'flow', flowOriginId: '11111111-1111-1111-1111-111111111111' },
    vesselType: { kind: 'assetType', flowOriginId: '33333333-3333-3333-3333-333333333333' },
    triageAgent: { kind: 'agent', agentId: '55555555-5555-5555-5555-555555555555' },
  },
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
pnpm validate                                 # every environment resolves — do this first
pnpm build                                    # produces dist/ — APP_ENV defaults to "test"
pnpm build:staging                            # staging bundle — carries staging ids only
pnpm build:prod                               # prod bundle — carries prod ids only
(cd dist && zip -r ../<name>-bundle.zip .)    # zip the CONTENTS of dist/, not the folder
```

**Why zip the contents, not the `dist/` folder itself:** the deploy pipeline extracts the archive under the app's served path. `remoteEntry.js` must be at the archive root so the shell can fetch it at runtime.

**Build for the environment you deploy to.** `RiseAppManifestPlugin` resolves
`rise-x-app.json` for the `APP_ENV` environment and emits the single-environment
result into `dist/`, so it rides the zip — a test build carries test ids only
(see §App dependencies). At deploy time the backend reads that file from the
zip, validates its **shape**, and stores the declarations. It does **not** look
the ids up, so **the deploy does not catch a test bundle shipped to prod**: it
succeeds, the wrong ids are stored, and the app breaks at runtime. The stored
app exposes `dependencies` plus a `dependencyEnvironment` naming the build
target the manifest claimed. Both `deploy_app` and `get_app` return it through
the Rise-X MCP. `dependencyEnvironment` is advisory, and comparing it against
the ecosystem is on you.

`pnpm validate` before the build is therefore the only check that sees every
environment: it catches a missing or malformed id for the environment you are
about to deploy to before the zip exists. What nothing catches is an id that is
well-formed but names an object that no longer exists.

When the bundle is ready, **ask the user whether to deploy**. Two paths:

### Preferred — deploy via the Rise-X MCP

Load the `rise-x-mcp` skill first (mandatory before any Rise-X MCP call), then
pick the server for the target environment: **`rise-x-test` → test,
`rise-x` → production. If the user doesn't name an environment, deploy to
test** — most users want to try the app there first. Deploying requires the
environment-orchestrator role. The plugin ships those two servers only, so there
is no staging server to look ids up on or deploy through. Build the bundle for
the **same environment** you deploy to (`APP_ENV` above), and check it yourself:
the backend only validates the shape of the zip's `rise-x-app.json`, so a bundle
carrying another environment's ids deploys without complaint and fails in the
running app.

1. `request_bundle_upload` → returns a single-use, short-TTL `uploadUrl` plus an `uploadId`.
2. `curl -X PUT --data-binary @<name>-bundle.zip '<uploadUrl>'`
3. `deploy_app(upload_id, name, version, app_id?, description?, icon?, feature_flags?)` —
   omit `app_id` for a brand-new app (a GUID is generated and returned); pass
   the existing GUID to release a new version. `version` comes from the app's
   `package.json` and must be unique per app — bump it every release (the
   enforcement is narrower than the practice — the rise-x-mcp plugin's
   `managing-apps.md` spells it out). The server derives the Module
   Federation scope from the `var <name>;` declaration in the zip's
   `remoteEntry.js`.
   `feature_flags` is an optional `dict` of app feature flags, e.g.
   `{"isOfflineModeEnabled": true}` — what makes the shell offer "Make
   available offline" (see `references/offline.md`).

The result carries the app `id` and the canonical manifest (`remoteUrl`,
`version`, `scope`, `deployedAt`, `sizeBytes`) — `scope` is always the
derived value, and that's what the shell uses — plus `dependencyCount`, the
`dependencies` themselves when that count is non-zero, and
`dependencyEnvironment` when the stored manifest named one. Confirm the app
loads in the shell after.

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
| `bundle` * | the `.zip` you produced |  |
| `description` | short description of the app |  |
| `icon` | optional |  |

The app id is generated by the dialog (a Regenerate control on new deploys).
The host `POST`s to `/api/v4/config/apps/:id/deploy`; the server returns the
manifest with the live `remoteUrl` and `module` (defaults to `./App`).

## Don'ts

- **Don't try to shadow the preset's shares.** The preset shares react/react-dom/jsx-runtime as `singleton` + `import: false`, so the app uses the host's copy and MF rewires every `react` request regardless of what the app's `package.json` says. Re-declaring those shares yourself is what gives you cross-React-instance hook crashes when the app mounts inside the shell. Note that *declaring* `react` and `react-dom` is required, not forbidden — the template ships both in `devDependencies` and the preset's standalone branch resolves them from the app, so removing them breaks standalone dev.
- **Don't** import shell internals. The contract is `@rise-x/apps-sdk` — full stop. If you need something the SDK doesn't expose, propose it as an SDK addition in a separate PR (or request it from the Rise-X team) — don't reach into the host.
- **Don't** wire your own auth or call backend services directly. Go through a declared dependency first (`deps.<alias>`), then the query hooks, then the connectors (`@rise-x/apps-sdk/connectors`), and only then `getShellApiV4` (§Runtime APIs).
- **Don't** put a flow, asset-type, or agent GUID literal in app source. Declare it in `rise-x-app.json` and read it through `useAppDependencies()` (§App dependencies); the generic connectors with a ref are for what a dependency can't express, starting with flows the user picks at runtime.
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
4. **Validate the manifest, and read the scan warnings.** The manifest,
   `validate` and `scan` need `@rise-x/apps-sdk` >= 0.12.0. `pnpm validate` exits
   zero, meaning every environment in `rise-x-app.json` resolves — not just the
   one you build. A build checks its own target only, so this is what catches a
   dependency you added for one environment and not the others it declares. Skip
   it and the gap surfaces at promotion, as a broken app in someone else's hands:
   the deploy checks the manifest's shape, not the ids, so it will not stop you.
   If an id is genuinely missing, ask the user for it or look it up in
   that ecosystem with the Rise-X MCP; never invent one, and never delete a
   declared environment to make the command pass.

   The same command warns about Rise-X ids still hardcoded in `src/`. Those
   warnings do **not** affect its exit code, so a green run is not a clean run:
   read them, and either declare each id in the manifest or mark it
   `rise-x-app-ignore` with a reason. Clear them before you deploy — the whole
   point of the manifest is that promoting the app to another environment doesn't
   need a code change.
5. **Build.** `pnpm build` succeeds and produces `dist/remoteEntry.js` — with
   `APP_ENV` matching the environment the bundle will deploy to (default
   `test`), so `dist/rise-x-app.json` carries that environment's ids.
6. **Zip `dist/` contents** into `<name>-bundle.zip` (see Build and deploy above for the exact command — must zip the contents, not the folder).
7. **Ask the user whether to deploy.** If yes, deploy via the Rise-X MCP —
   **test environment unless the user names another** — following the steps in
   Build and deploy, and report the returned manifest (`id`, `version`,
   `remoteUrl`). If the MCP isn't available or the user prefers manual, output
   the deploy values for copy-paste:

   ```
   Deploy bundle ready: <name>/<name>-bundle.zip

   Open the Apps page (/<environment>/apps) → New App, and paste:

     name:       <human name>
     version:    <from package.json>
     bundle:     <name>/<name>-bundle.zip
     description: <short app description>
     icon:       (optional)
   ```

8. **Confirm the deploy landed.** Don't mark the task complete until the app loads in the shell — verify it yourself after an MCP deploy, or ask the user to confirm after a manual one.

If any of steps 1–5 fail, stop and fix before reaching out to the user. A failing build or a manifest that doesn't validate is not a "done" — it's a regression to be reported and resolved.
