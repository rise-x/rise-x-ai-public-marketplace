# Performance Dashboards

Performance dashboards are standalone UI surfaces (no flow, no steps, no work items) built from the same sections + components system as flow layouts. They have their **own** draft/publish lifecycle and their **own** set of component tools — do not reuse flow-layout tools.

## Contents

- [Tool Inventory](#tool-inventory) — every dashboard tool grouped by purpose, plus which id (published vs. `draftId`) goes where.
- [Draft Lifecycle](#draft-lifecycle-different-from-flows) — the get-or-create draft → edit → publish pattern. **Different from flows.**
- [Component Hierarchy](#component-hierarchy-same-as-flow-layouts) — sections and parenting rules.
- [Permissions & Users](#permissions--users) — `Private` vs. `Ecosystem`, role assignment.
- [`defaultQueryFilters` — scoping the works array](#defaultqueryfilters--scoping-the-works-array) — recognized keys (`currentState`, `displayName`, `flowOriginId`), the `currentState` vs `work.state`/`work.status` two-enum trap, date tokens. **Read this before authoring filters; the wrong key fails silently.**
- [`quickFilterProperty` — user-facing pre-populated filters](#quickfilterproperty--user-facing-pre-populated-filters) — the filter chips shown **and pre-selected** in the report toolbar. **Keys must be the flow property `key`, not a data value-path — the #1 reason MCP-built reports show no filters.**
- Worked examples — [seed components at creation](#worked-example--seed-components-at-creation) (no draft) and [edit via draft](#worked-example--edit-via-draft).
- [Supported Dashboard Component Types](#supported-dashboard-component-types) — the whitelist; flow-layout components are rejected.
- [Layout Rendering & Widths](#layout-rendering--widths) — 12-column grid, `col-{1-12}` width syntax, 5-point responsive breakpoints (different from work layouts), and the chart-vs-non-chart derating that the renderer applies automatically.
- [Configuring Containers (Section & Container)](#configuring-containers-section--container) — section properties (`collapsible`, `useContainer`) and when to reach for the nested `container`.
- [Configuring data-grid columns](#configuring-data-grid-columns) — the grid reads columns from the component's `properties.columns`, not the dashboard `columnsData`. Server backfills from the source flow when `defaultQueryFilters.flowOriginId` resolves to one flow; the two persistence layers and when to use each.
- [Dashboard Modes (Works vs. Aggregated)](#dashboard-modes-works-vs-aggregated) — `layout.dataSource` controls which render path each chart takes; the table shows every component's dispatch style (split / in-component branch / unified / no-aggregated). Read this before authoring chart properties.
  - [Top-metric / average overlays in Aggregated mode](#top-metric--average-overlays-in-aggregated-mode) — how `total.value`, `secondTotal.value`, `average.value` resolve against `fullData.data`.
  - [Setting the dashboard `dataSource`](#setting-the-dashboard-datasource) — pass `data_source` on `create_dashboard` / `update_dashboard_metadata` to flip Works vs. Aggregated mode.
- [Configuring Charts & Metric Cards](#configuring-charts--metric-cards-non-aggregated-dashboards) — non-aggregated mode. Read [Shared Concepts](#shared-concepts) first; the per-component subsections follow.
  - Charts: [`total-by-bar-chart`](#total-by-bar-chart) · [`total-by-clustered-bar-chart`](#total-by-clustered-bar-chart) · [`total-by-stacked-bar-chart`](#total-by-stacked-bar-chart) · [`total-by-pie-chart`](#total-by-pie-chart) · [`total-by-period-stacked-bar-chart`](#total-by-period-stacked-bar-chart) · [`multiple-rating-chart`](#multiple-rating-chart) · [`heatmap-chart`](#heatmap-chart)
  - Metric cards: [`metric-card`](#metric-card) · [`grouped-metric-card`](#grouped-metric-card) · [`rating-metric-card`](#rating-metric-card) · [`savings-metric-card`](#savings-metric-card)
  - [Chart & Metric-Card Pitfalls](#chart--metric-card-pitfalls) — formula syntax, key vs. path, heatmap items.
- [Configuring Aggregated Reports (`reportData`)](#configuring-aggregated-reports-reportdata) — aggregated mode. Per-component `reportData` schema, `expandByDataPath`, `groupBy`, `fields`, operations enum, and which `*Key` properties the aggregated render path actually reads.
- [Lifecycle & API Pitfalls](#lifecycle--api-pitfalls) — draft id confusion, destructive replace, partial failures.

For expression syntax used inside `value`, `total.value`, `tooltipFields[].value`, etc., see `references/dynamicValue.md`.

## Tool Inventory

**Dashboard CRUD & metadata** (no draft required):
- `list_dashboards(page?, page_size?, sort_field?, sort_order?, query?)` — `sort_field` is one of `displayName` (default), `name`, `uniqueName`, `layoutType`, `permissionType`, `environmentId`, `createdBy`, `lastModified`, `created`, `lastModifiedBy`. `sort_order` is `"asc"` or `"desc"` (default). `query` is a case-insensitive substring match on display name. Items are projected to: `id`, `originId`, `displayName`, `iconData`, `permissionType`, `createdBy`, `ownersCount`.
- `get_dashboard(id)`
- `create_dashboard(display_name, permission_type?, icon_data?, default_query_filters?, quick_filter_property?, columns_data?, is_expand_by_feature_enabled?, show_manage_dashboard_buttons?, add_users?, components?, data_source?)` — pass `components` to seed sections + children in a single request; no draft needed. Pass `data_source` to mark the dashboard as Aggregated mode (see [Setting the dashboard `dataSource`](#setting-the-dashboard-datasource)). Pass `show_manage_dashboard_buttons=false` to hide the Update / Create buttons on the Performance Report page (the full-screen performance view accessible from a dashboard; server default is `true`). Missing `properties.width` is filled with `col-4` for charts and metric cards and `col-12` for `data-grid` (see [Width values](#width-values)). **For `data-grid` components without `properties.columns`**, the server backfills column definitions from the source flow when `default_query_filters.flowOriginId` resolves to a single flow (see [Configuring data-grid columns](#configuring-data-grid-columns)); `columns_data` is auto-derived from the resulting columns when omitted.
- `duplicate_dashboard(id, display_name?)` — atomically deep-clones a published dashboard: components are cloned with fresh GUIDs (nested `parentId`s remapped), the source ACL is propagated minus the caller and the system user (you're re-granted as Owner by the create flow), the dashboard-level `dataSource` is deep-cloned, and the copy is published immediately (no draft). When `display_name` is omitted or whitespace the server uses `"<source.DisplayName> -Copy"`.
- `update_dashboard_metadata(id, ...)` — PATCHes the published dashboard directly. Use for name, icon, filters, columns, permissions, user membership, the dashboard-level `data_source`, and `show_manage_dashboard_buttons` (boolean; pass `false` to hide or `true` to restore the Update / Create buttons on the Performance Report page — Owner or Editor required).
- `delete_dashboard(id)` — soft delete; requires Owner role.

**Dashboard draft lifecycle** (for layout/component edits):
- `get_dashboard_draft_layout(dashboard_id)` — pass the **published** dashboard id. Returns `draftId` + `components`.
- `add_dashboard_components(draft_id, components, parent_section_id?, after_component_id?)` — same default-width fill-in as `create_dashboard` (charts + metric cards → `col-4`, `data-grid` → `col-12`). For `data-grid` without `properties.columns`, the server reads the parent dashboard's `defaultQueryFilters.flowOriginId` and backfills columns from the source flow.
- `update_dashboard_component(draft_id, component_id, component_data)` — PATCH semantics; include `"component"` key to change type. **Forwards `component_data` verbatim** — no default-width fill-in.
- `replace_dashboard_section_components(draft_id, section_id, components)` — destructive within the section. Same default-width fill-in as `create_dashboard`.
- `delete_dashboard_components(draft_id, component_ids)` — deletes sequentially, one request per id. On first failure the tool returns an error listing which ids were already deleted before the error, so callers can resume cleanly instead of retrying the whole batch.
- `reset_dashboard_draft_layout(draft_id)` — discards every staged edit on the draft and re-clones the published dashboard's content into it. The draft itself is kept (same `draftId`), only its content is rolled back. Use this to abandon an edit session without deleting the draft.
- `publish_dashboard_draft_layout(draft_id)` — publishes so changes land on the canonical dashboard. The response lists the published components (flattened) and a `componentCount` covering sections **and** their children.

**Component shape gotcha — nested vs flat.** `get_dashboard(id, format="full")` returns the raw dashboard record, where `components` is a *hierarchy*: child components are nested under their section's `components` key, so the top-level array holds only root components (usually just sections). The record's `componentsLength` field carries the true flat total. Do not read the top-level array length as "how many components the dashboard has" — a dashboard whose tiles all live in one section looks like a single component that way. Every other dashboard tool (`get_dashboard(format="summary")`, `get_dashboard_draft_layout`, publish/reset responses) already flattens the tree to one line per component, nested children included — prefer those views for counting or navigating components.

## Draft Lifecycle (different from flows)

1. `get_dashboard_draft_layout(dashboard_id)` → capture `draftId`. This endpoint is **get-or-create** — safe to call whether or not a draft already exists.
2. Edit using the component tools above, passing `draftId` each time.
3. `publish_dashboard_draft_layout(draftId)` — changes land on the published dashboard.

To abandon edits mid-session: `reset_dashboard_draft_layout(draftId)` rolls the draft's content back to the current published dashboard. The `draftId` stays valid afterwards, so subsequent component tools can continue editing from the freshly-reset baseline. This is the cheaper alternative to deleting the dashboard or starting a new draft.

There is no combined "do-everything" dashboard tool — each step is a separate call so failures are localised and retryable.

**Which id goes where** — the single most common failure mode:

| Tool | Takes |
|---|---|
| `get_dashboard_draft_layout` | **published** dashboard id |
| `add_dashboard_components` | `draftId` |
| `update_dashboard_component` | `draftId` |
| `replace_dashboard_section_components` | `draftId` |
| `delete_dashboard_components` | `draftId` |
| `reset_dashboard_draft_layout` | `draftId` |
| `publish_dashboard_draft_layout` | `draftId` |
| `duplicate_dashboard` / `update_dashboard_metadata` / `get_dashboard` / `delete_dashboard` | **published** dashboard id |

## Component Hierarchy (same as flow layouts)

```
Dashboard
  -> Section (parentId = "00000000-0000-0000-0000-000000000000")
     -> Components (parentId = section's ID)
        OR
     -> Container (parentId = section's ID)        [optional intermediate grouping]
        -> Components (parentId = container's ID)
```

- Sections are top-level containers; their `parentId` is always the null GUID.
- Non-section components MUST have `parentId` pointing to a section, or to a container nested inside a section.
- `container` is optional — most dashboards group tiles directly under a section. See [Configuring Containers](#configuring-containers-section--container) for when to add a nested container.
- Orphans are rejected with an error.

When calling `add_dashboard_components`, parenting is resolved per non-section component in this priority order:

1. **Call-level `parent_section_id` argument** — if set, the tool writes that id into every non-section component's `parentId`, overwriting anything already there. Use this when the whole batch lands under a single section.
2. **First-section auto-parent** — if no `parent_section_id` is passed and a `section` dict appears in `components`, every following non-section component without an explicit `parentId` is auto-parented to that section.
3. **Per-component `parentId` field** — set `parentId` directly on a component dict to override the auto-parent rule (e.g. to nest a tile under a container in the same batch). This is honored only when the call-level `parent_section_id` is omitted; otherwise rule 1 overwrites it.

`parent_section_id` is **only** valid as a tool argument. Putting `"parent_section_id": "..."` inside a component dict has no effect — the dashboard layout schema's parent field is `parentId`. Use `parentId` for per-component parenting.

Component ids and `name` / `dataPath` are auto-generated from `label` if missing. To reference a same-batch parent (e.g. a container) from a child's `parentId`, pre-generate the parent's `id` client-side and pass both in the same call.

## Permissions & Users

- `permission_type`: `"Private"` (default) or `"Ecosystem"`. Setting `"Ecosystem"` requires being an environment owner.
- Dashboard user roles: `"Owner"`, `"Editor"`, `"Viewer"`.
- `create_dashboard` auto-adds the caller as Owner; `add_users` on create cannot include the caller.
- User membership changes go through `update_dashboard_metadata(id, add_users=..., remove_user_ids=...)` — no draft needed.

## `defaultQueryFilters` — scoping the works array

Pre-applied filter that scopes which works flow into every chart and metric card on the dashboard. Pass via `create_dashboard(default_query_filters=...)` or `update_dashboard_metadata(id, default_query_filters=...)`. Unknown keys are accepted but silently ignored — a wrong key produces `NO DATA` with no error.

> **A workflow dashboard with no recognized filter is non-functional.** The works fetch requires at least one scoping key — if `defaultQueryFilters` is empty or only contains unrecognized keys, the request is rejected upstream, `{$.works:JSON}` stays empty, and every tile renders `0` / `NO DATA`. **Always set at least `flowOriginId` when building a dashboard tied to one workflow.**

### Recognized keys

| Key | Value | Example |
|---|---|---|
| `flowOriginId` | `string[]` of flow GUIDs | `["ec2e4937-..."]` |
| `currentState` | `string[]` of work-state names (see below) | `["Completed"]` |
| `displayName` | `string[]` exact match on `work.displayName` | `["Pacific Order"]` |

Anything else (`state`, `statusLabel`, `activeStepId`, `assignedTo`, …) is **not** honored — use `quickFilterProperty` for user-facing runtime filters instead.

### `currentState` values

These five values are the **only** valid `currentState` strings. Use them verbatim — do not invent variants, do not translate from other status fields.

| Value | When to use |
|---|---|
| `"Open"` | Any work on a non-terminal step (in-flight orders, drafts, awaiting-something). This is the catch-all for "still being worked on". |
| `"Completed"` | Work reached a terminal step (e.g. delivered, fulfilled, finalised). |
| `"Closed"` | Explicitly closed via a lifecycle operation. Rare. |
| `"Deleted"` | Soft-deleted. Rare. |
| `"Ok"` | Sync-process flag. Almost never used in user-facing filters. |

⚠️ **Do NOT use:** `"InProgress"`, `"Complete"` (no `-d`), `"New"`, `"Draft"`, `"Cancelled"`, or anything you see in `work.state` / `work.statusLabel`. Those come from different enums and silently produce `NO DATA` when used as `currentState`. `work.status` carries the same five `DianaWorkState` values as the table above, but copy the value from `workStateName` all the same.

**How to verify a value before authoring a filter.** If you're unsure which `currentState` value applies to a specific work, call `get_work(workId)` and read the `workStateName` field — it always contains the exact string to use:

```yaml
workState: 1
workStateName: Open      # ← copy this verbatim into currentState
```

### Canonical examples

```jsonc
// Delivered / completed orders for one flow
{"flowOriginId": ["<flow-guid>"], "currentState": ["Completed"]}

// In-flight orders for one flow
{"flowOriginId": ["<flow-guid>"], "currentState": ["Open"]}
```

### Date tokens

For date-range filter values you can pass tokens that the platform expands to ISO timestamps at render time: `"now"`, `"now-1d"`, `"now-7d"`, `"now-30d"`, etc. Pass verbatim inside the array value.

### Filter keys beyond the recognized three

Custom flow-property filters pass through too. The three platform-level keys (`flowOriginId`, `currentState`, `displayName`) are always recognized; in addition, any key matching a property the flow has declared as filterable (e.g. `assignedTeam`, `region`) will be honored. Keys with no platform-level handler and no matching filterable flow property are silently dropped. As with `quickFilterProperty` (below), a custom key must be the flow property **`key`**, not a data value-path — discover valid keys with `manage_columns(flow_id, "list")`.

## `quickFilterProperty` — user-facing pre-populated filters

Where `defaultQueryFilters` scopes the works array **invisibly** (server-side), `quickFilterProperty` controls the filter chips shown **and pre-selected** in the report toolbar. Use it when the user should see the filter applied and be able to change it. Pass via `create_dashboard(quick_filter_property=...)` or `update_dashboard_metadata(id, quick_filter_property=...)`.

Shape:

```jsonc
{"keys": {"<flowPropertyKey>": ["<value>", ...]}}
```

> **The key must be the flow's filterable property `key` — NOT a data value-path.** A chip renders only when its key matches a flow property `key`. A value-path key is silently dropped with **no error**: the report opens with **no filter chips**. This is the single most common reason an MCP-built report "loses" its filters.

**Required: discover the key before you set it.** You cannot guess these keys — the `key` often does not resemble the field's label (e.g. the "Vendor" column's key may be `selectDeliveryPort`). Before passing `quick_filter_property`, **always** call `manage_columns(flow_id, "list")` (the `flow_id` is your `defaultQueryFilters.flowOriginId`) and copy the exact `key`. Each column has three distinct fields — use the right one:

| Field | Example | Use as quick-filter key? |
|---|---|---|
| `key` | `vendor` | ✅ **Yes — this is the key.** |
| `displayName` | `Vendor` | ❌ No (human label only). |
| `valuePaths` | `["$.deliveryRequest.vendor"]` | ❌ No (data path only). |

```jsonc
// ✅ RIGHT — property key, value is the option value
{"keys": {"vendor": ["Marinoil", "Audentis"]}}

// ❌ WRONG — data value-path; chip never renders
{"keys": {"deliveryRequest.vendor.displayName": ["Marinoil", "Audentis"]}}
```

Notes:
- An **empty array** (`{"vendor": []}`) shows the chip with **no preselection** — the user picks values.
- Values must be the actual option values for that property.
- The API accepts any key shape **without error** — a wrong key is not rejected, it just produces an empty toolbar. After creating a dashboard with quick filters, verify by re-reading it with `get_dashboard(id)` and confirming every `quickFilterProperty.keys` entry is also a column `key` from `manage_columns(flow_id, "list")`.

## Worked Example — Seed components at creation

Skip the draft dance entirely when you already know the layout. `create_dashboard` accepts an optional `components` list that is seeded atomically in a single request. Same normalization as `add_dashboard_components`: auto-generated `id`, `name` / `dataPath` derived from `label`, sections parented to the null GUID, non-section components auto-parented to the first section in the batch.

```
create_dashboard(
    display_name="Ops Overview",
    default_query_filters={"flowOriginId": ["<source flow originId>"]},
    components=[
        {"component": "section", "label": "Overview"},
        {"component": "rating-metric-card", "label": "Total Sales"},
        {"component": "data-grid", "label": "Recent Orders"},
    ],
)
# Returns the published dashboard with its id, originId, and components.
# No draft / publish cycle needed.
#
# Because default_query_filters.flowOriginId resolves to a single flow, the
# server backfills the data-grid's properties.columns from that flow's
# defaultOn columns (see "Configuring data-grid columns" below) and derives a
# matching columns_data block on the dashboard. Pass columns explicitly to
# override either step.
```

Component types must be in the dashboard whitelist (see [Supported Dashboard Component Types](#supported-dashboard-component-types) below) — the API rejects the whole request if any component type is invalid.

## Worked Example — Edit via draft

Add a section with one input, update its label, then publish:

```
# 1. Resolve the draft
resp = get_dashboard_draft_layout(dashboard_id="<published dashboard GUID>")
draft_id = resp.draftId

# 2. Add a section + a child metric card in one call
add_dashboard_components(
    draft_id=draft_id,
    components=[
        {"component": "section", "label": "Overview"},
        {"component": "rating-metric-card", "label": "Total Sales"},
    ],
)
# The section id is returned in the response; capture it and its child id.

# 3. Patch the metric card
update_dashboard_component(
    draft_id=draft_id,
    component_id="<rating-metric-card component id>",
    component_data={"label": "Total Revenue"},
)

# 4. Publish
publish_dashboard_draft_layout(draft_id=draft_id)
```

## Supported Dashboard Component Types

Dashboards accept ONLY the component types below — these differ from flow-layout components (no form inputs like `input-text`, `input-select`, etc.). Any component outside this whitelist is rejected by the API.

**Containers**
- `section` — top-level container, `parentId` must be the null GUID
- `container` — nested grouping inside a section

**Metric cards**
- `metric-card` — single value KPI tile
- `grouped-metric-card` — multiple related metrics in one tile
- `rating-metric-card` — rating / score display
- `savings-metric-card` — savings-specific metric tile

**Charts**
- `total-by-bar-chart`
- `total-by-clustered-bar-chart`
- `total-by-stacked-bar-chart`
- `total-by-pie-chart`
- `total-by-period-stacked-bar-chart`
- `multiple-rating-chart`
- `heatmap-chart`

**Tabular**
- `data-grid` — tabular listing

## Layout Rendering & Widths

Dashboards render through the same **12-column MUI grid** flow layouts use, with two important differences vs `references/layouts-and-components.md` (work layouts):

- **Two-level nesting.** Tiles live inside a `section`, optionally wrapped in a nested `container`. Sections are top-level (parent = null GUID); containers are an optional intermediate row used only when one section needs multiple sub-groups.
- **Five-point responsive breakpoints.** Each tile receives explicit `xs`, `sm`, `md`, `lg`, `xl` Grid props. Work layouts use only `xs` / `md`; dashboards lay out differently on mobile, tablet, laptop, and desktop, as described in this section.

### Width values

`properties.width` accepts `"col-{1-12}"` — the number is the column span at the **largest** breakpoint (`xl`). At smaller breakpoints the renderer derates automatically (see below), so explicit per-breakpoint widths are rarely needed.

| Width | xl span | Use for |
|---|---|---|
| `col-12` | 100% | Full-width: `data-grid` (MCP default), single hero chart, banners |
| `col-6` | 50% | Two-up KPI rows, two-up charts |
| `col-4` | 33% | Three-up rows — charts and metric cards (MCP default) |
| `col-3` | 25% | Four-up small KPI rows |

**MCP server default widths.** When you call `create_dashboard`, `add_dashboard_components`, or `replace_dashboard_section_components` and don't supply a `width` (neither top-level `width` nor `properties.width`), the MCP server fills one in based on the component type:

| Component type | Default `properties.width` |
|---|---|
| `data-grid` | `col-12` (full row) |
| Any chart (`total-by-bar-chart`, `total-by-clustered-bar-chart`, `total-by-stacked-bar-chart`, `total-by-pie-chart`, `total-by-period-stacked-bar-chart`, `multiple-rating-chart`, `heatmap-chart`) | `col-4` (three-up) |
| Any metric card (`metric-card`, `grouped-metric-card`, `rating-metric-card`, `savings-metric-card`) | `col-4` (three-up) |
| `section`, `container`, or any other type | not touched (the section/container has its own width logic) |

Any width you pass through — on `width` or `properties.width` — is preserved verbatim, even if the value is unusual (e.g. `col-12` for a metric card). The defaults only fire when the field is missing or empty. `update_dashboard_component` (PATCH) never touches width — it forwards `component_data` verbatim.

### Responsive behavior — what the grid actually emits

The responsive width logic derives the per-breakpoint span from your `width`. Two rules dominate:

1. **Mobile (`xs`) is always 12.** Every tile is full-width on phones regardless of `width`. There is no way to opt out.
2. **Charts get a different `md` / `lg` default than other tiles.** Chart components (`total-by-bar-chart`, `total-by-stacked-bar-chart`, `total-by-pie-chart`, `total-by-period-stacked-bar-chart`, `multiple-rating-chart`) collapse to **6** at `md` / `lg` even when you set `col-12`, so two charts share a row on a laptop. Non-chart tiles (metric cards, `heatmap-chart`, `data-grid`) keep your `col` value at `lg` and clamp to ≥ 6 at `sm` / `md`.

Concretely, for `width: "col-4"`:

| Component | xs | sm | md | lg | xl |
|---|---|---|---|---|---|
| chart (e.g. `total-by-bar-chart`) | 12 | 12 | 6 | 6 | 4 |
| non-chart (e.g. `metric-card`) | 12 | 6 | 6 | 4 | 4 |

This is why a four-up KPI row of `metric-card` tiles with `width: "col-3"` lays out cleanly on desktop, drops to two-up on tablet, and stacks on phone — without you authoring per-breakpoint widths.

**Per-breakpoint width overrides are not authorable today.** The frontend code parser reads `col-sm-N`, `col-md-N`, `col-lg-N`, `col-xl-N` prefixes — *not* `sm-N` / `md-N` — but the API regex is `^col-(?:[1-9]|1[0-2])$`, which rejects spaces and multi-token strings. So compound widths like `"col-4 col-md-6 col-sm-12"` fail API validation on every mutating endpoint. Stick to single `col-N` values and rely on the auto-derived breakpoints above.

### Putting it together — when tiles actually sit side-by-side

Two things must both be true for a section to lay tiles out across a row:

1. **The section emits a Grid container.** Either set `useContainer: true` on the section, or wrap the tiles in a nested `container` (whose `wrapWithGrid` defaults to `true`). Without one of these, there is no `<Grid container spacing={2}>` wrapper, percentage widths can't compose, and tiles stack full-width — see [Container Pitfalls](#container-pitfalls) #2.
2. **Each child sets a fitting `width`.** `col-6` (two-up), `col-4` (three-up), or `col-3` (four-up). A child of `col-12` always claims a full row.

For section / container property details (`collapsible`, `useContainer`, `wrapWithGrid`) see [Configuring Containers](#configuring-containers-section--container) below.

## Configuring Containers (Section & Container)

Every dashboard tile lives inside a `section`; that's the top-level grouping. `container` is an *optional* nested grouping used when one section needs to hold two or more visually distinct sub-groups (e.g. a row of KPI tiles followed by a separately-grouped chart row). Most dashboards use sections only — reach for `container` only when you can articulate which sub-grouping it creates.

### `section`

Required: none. (`label`, `id`, and `parentId` are required by the platform itself; `parentId` for a section is always the null GUID.)

Optional:
- `title` (string) — section heading text.
- `showTitle` (bool, default `false`) — render the title above the section.
- `collapsible` — `"None"` | `"Collapsed"` | `"Expanded"`. Default `"None"` (no chevron). `"Collapsed"` renders the chevron with the section initially closed; `"Expanded"` renders the chevron with the section initially open. *Why this matters:* dashboards with many tiles benefit from a collapsible "Details" / "Historical" section so the above-the-fold view stays focused.
- `useContainer` (bool, default `false`) — when `true`, the section's children are rendered inside a 12-column `<Grid container spacing={2}>` wrapper, so child `width: "col-6"` etc. produce a true side-by-side grid. When `false`, children render in document order with no grid wrapper. *How to apply:* set `useContainer: true` whenever the section holds two or more tiles that should sit side-by-side; leave `false` for a single full-width chart.

Example — collapsible "Last 30 days" section that lays out four KPI tiles across one row:

```jsonc
{
  "component": "section",
  "label": "Last 30 days",
  "properties": {
    "title": "Last 30 days",
    "showTitle": true,
    "collapsible": "Expanded",
    "useContainer": true
  }
}
```

### `container`

Required: none.

Optional:
- `wrapWithGrid` (bool, default `true`) — when `true`, children are wrapped in `<Grid container spacing={2} rowSpacing={2}>`. Set to `false` only when the outer section / container is already supplying the grid layout and you want this nested group to render flush.

`parentId` must point at a section (or another container). Containers cannot live at the dashboard root — the null GUID is reserved for sections.

Example — three tiles where the last two are grouped inside their own container row. Because the children need two different parents (section and container) in the same conceptual layout, do this in two calls. The first creates the section, the in-section metric, and the container; the second adds the container's children with `parent_section_id` pointed at the container id.

```python
# Call 1 — section, one tile under the section, and the container.
# No parent_section_id at the call level; the section in the batch auto-parents
# the metric-card and the container.
result1 = add_dashboard_components(draft_id, components=[
    {
        "component": "section",
        "label": "Operations",
        "properties": {"title": "Operations", "showTitle": True, "useContainer": True},
    },
    {
        "component": "metric-card",
        "label": "In Transit",
        "dataPath": "",
        "properties": {"value": "=count({$.works:JSON})"},
    },
    {
        "component": "container",
        "label": "Throughput",
        "properties": {"wrapWithGrid": True},
    },
])
# Read the returned layout to get the container id.
container_id = next(c["id"] for c in result1.components if c["label"] == "Throughput")

# Call 2 — both children land under the container via the call-level argument.
add_dashboard_components(draft_id, parent_section_id=container_id, components=[
    {"component": "metric-card", "label": "Delivered", "dataPath": "",
     "properties": {"value": "=count({$.works:JSON})"}},
    {"component": "metric-card", "label": "Delayed", "dataPath": "",
     "properties": {"value": "=count({$.works:JSON})"}},
])
```

Single-call alternative — pre-generate the container id and reference it via `parentId` (the dashboard schema field) on each child. Do **not** pass `parent_section_id` at the call level in this form, or the tool will overwrite every child's `parentId` with the section id and the container nesting collapses.

```python
import uuid
container_id = str(uuid.uuid4())

add_dashboard_components(draft_id, components=[
    {"component": "section", "label": "Operations",
     "properties": {"title": "Operations", "showTitle": True, "useContainer": True}},
    {"component": "metric-card", "label": "In Transit", "dataPath": "",
     "properties": {"value": "=count({$.works:JSON})"}},
    {"id": container_id, "component": "container", "label": "Throughput",
     "properties": {"wrapWithGrid": True}},
    {"component": "metric-card", "label": "Delivered",
     "parentId": container_id, "dataPath": "",
     "properties": {"value": "=count({$.works:JSON})"}},
    {"component": "metric-card", "label": "Delayed",
     "parentId": container_id, "dataPath": "",
     "properties": {"value": "=count({$.works:JSON})"}},
])
```

In both forms, the parent-id field on the component dict is `parentId` (the layout schema's field), not `parent_section_id`. The `parent_section_id` name is only valid as the tool argument.

### When to reach for `container` vs. just nesting under a `section`

- **Default:** put tiles directly under a section with `useContainer: true`. One level of nesting handles ~90% of dashboards.
- **Use a `container`** only when one section contains two or more *distinct* sub-groupings that each need their own internal grid, or when a sub-row needs different spacing than the rest of the section. *How to apply:* if you can't articulate which sub-grouping the container creates, skip it.

### Container Pitfalls

1. **`collapsible` strings are case-sensitive.** `SectionCollapsibleModeEnum` exact values are `"None"`, `"Collapsed"`, `"Expanded"`. A lowercase `"collapsed"` falls through to the default `"None"` branch and the chevron silently never appears. *Why:* the section component compares against the enum string directly with no case-insensitive coercion.
2. **`useContainer: false` + `width: "col-6"` looks broken.** Without the grid wrapper, percentage-based widths can't lay out side-by-side and tiles stack full-width. Either set `useContainer: true` on the section, or wrap the tiles in a `container` whose `wrapWithGrid` is `true`.

## Configuring data-grid columns

The dashboard `data-grid` is the only component whose rendered columns come from the **component's own** `properties.columns` array — not from the dashboard-level `columnsData`. If `properties.columns` is missing or empty, the grid renders zero headers regardless of filters, work data, or `columnsData`.

### How the server backfills

`create_dashboard`, `add_dashboard_components`, and `replace_dashboard_section_components` all backfill `properties.columns` on any `data-grid` that omits it, **provided the dashboard's `defaultQueryFilters.flowOriginId` resolves to exactly one flow**. The backfill picks columns marked `defaultOn: true` on the source flow (the same list `manage_columns(flow_id, "list")` returns) and converts them to data-grid column entries.

- `create_dashboard` reads `default_query_filters` from the call args.
- `add_dashboard_components` / `replace_dashboard_section_components` fetch the dashboard via `get_dashboard(draft_id)` to read its existing `defaultQueryFilters`.

When the backfill runs and `columns_data` was not supplied to `create_dashboard`, the server also derives `columnsData.orderedFields` + `columnVisibilityModel` from the resulting grid columns so the dashboard's persistence layer stays in sync.

Backfill is a no-op when:
- The data-grid already has at least one entry in `properties.columns` (pass an explicit list to override defaults; an empty list `[]` is treated the same as missing and still triggers backfill).
- `defaultQueryFilters.flowOriginId` is missing, empty, or has multiple entries.
- The flow's column list is unreachable (the failure is logged and the grid renders empty).

### Authoring columns explicitly

Supply `properties.columns` directly when you want non-default columns (computed/derived, formatted differently, or scoped to a flow other than the dashboard filter). Each entry follows `DataGridColumnSchema`:

```
{"component": "data-grid", "label": "Recent Orders", "properties": {
    "columns": [
        {"type": "string", "name": "workCode",      "displayName": "Order #",       "columnWidth": 140},
        {"type": "string", "name": "customerName",  "displayName": "Customer",      "columnWidth": 180},
        {"type": "number", "name": "amountPaid",    "displayName": "Amount Paid",   "decimals": 2,
                                                    "align": "right", "headerAlign": "right"},
        {"type": "status", "name": "currentState",  "displayName": "Status"},
        {"type": "date",   "name": "createdDate",   "displayName": "Created",
                                                    "format": "dd MMM yyyy HH:mm"},
    ]
}}
```

Common `type` values: `string`, `number`, `date`, `status` (renders the WorkStatus chip — canonical for `currentState`), `currency`, `boolean`, `select`, `link`, `user-link`, `rating`. Full list lives in `DataGridColumnTypeEnum`. Use `name` values from `manage_columns(flow_id, "list")` so the grid can resolve work data by path.

### `properties.columns` vs. dashboard `columnsData`

| Field | Layer | Purpose |
|---|---|---|
| `data-grid` component `properties.columns` | per-component | **Defines** the columns the grid renders. Required for headers to appear. |
| Dashboard `columnsData.orderedFields` + `columnVisibilityModel` | dashboard-level | **Persists** the user's per-column order / hide-show choices. Optional. |

The two layers are independent: setting `columnsData` alone does not render columns; setting `properties.columns` alone renders the default order with all columns visible. The MCP server auto-syncs them at creation time when one is present and the other isn't.

## Dashboard Modes (Works vs. Aggregated)

Dashboards have two render modes, picked **per dashboard** by `layout.dataSource`:

| Mode | `layout.dataSource.name` | Data each chart sees | Where math happens |
|---|---|---|---|
| **Works** (default) | unset / `"works"` | the full work array (filtered by dashboard filters + date range) | client-side, in the chart |
| **Aggregated** | `"Aggregate"` | the rows produced by each component's own `reportData` | server-side, before the response reaches the chart |

> **Use `"Aggregate"` for aggregated dashboards.** Any other non-`works` string (including `"data"`) requires the target environment to have a matching named driver registered, and most environments don't. Fetching aggregated data on a dashboard whose `dataSource.name` is anything other than `"Aggregate"` or `"works"` typically fails with a generic 400 from the dashboard data fetch. See [Setting the dashboard `dataSource`](#setting-the-dashboard-datasource).

At render time, the dashboard checks `layout.dataSource` and (per component) either dispatches to one of two implementations, switches a code branch inside a single implementation, or has no aggregated handling at all:

| Component | Dispatch style | Works | Aggregated |
|---|---|---|---|
| `total-by-bar-chart` | **Split component** | `TotalByBarChartWithCalculations` | `TotalByBarChartWithAggregations` |
| `total-by-stacked-bar-chart` | Split | `…WithCalculations` | `…WithAggregations` |
| `total-by-pie-chart` | Split | `…WithCalculations` | `…WithAggregations` |
| `total-by-period-stacked-bar-chart` | Split | `…WithCalculations` | `…WithAggregations` |
| `grouped-metric-card` | Split | `…WithCalculations` (reads `value` as a `{label: number}` map) | `…WithAggregations` (reads `valueLabel` / `value` / `secondValue` per aggregated row) |
| `metric-card` | **In-component branch** (`hasDifferentDataSource`) | `dashboardDynamicValue({ value, works })` | `dynamicValue(value, { data: <aggregated row> })` |
| `rating-metric-card` | In-component branch | same Works/Aggregated split via `dynamicValue` | same |
| `savings-metric-card` | In-component branch | `handleSavingsCalculations.getByValue(works, ...)` | unpacks `dynamicValue(value, { data })` into High / Mid / Low |
| `multiple-rating-chart` | Single unified path; `useDashboardComponent` already returns the right `data` shape | reads first record per group via `dynamicValue(calculateSeriesKey, { data: records[0] })` | same — works because the data scope is already the aggregated rows |
| `heatmap-chart` | **No Aggregated path** — always treats `data` as the works array | works | reads `data` as if it were `IWork[]` (likely broken in pure Aggregated mode unless a custom `reportData` recipe yields rows shaped like works) |
| `data-grid` | Mode-aware but unified | works | uses server-aggregated rows |
| `total-by-clustered-bar-chart` | **Aggregated-only** | **No renderer** — Works path emits `null` (blank tile) | `TotalByClusteredBarChartWithAggregations` — groups by `groupSeriesByKey` (X-axis) × `groupStackByKey` (clusters), measure `calculateStackKey ?? calculateSeriesKey` |

**The two paths read different property names** for the same conceptual job. For most charts both paths read `groupSeriesByKey`, but for the value/measure key:

| Chart | Works branch reads | Aggregated branch reads |
|---|---|---|
| `total-by-bar-chart` | `calculateSeriesKey`, `countSeriesKey` | **`calculateStackKey`** (no fallback) |
| `total-by-stacked-bar-chart` | `calculateSeriesKey` | `calculateStackKey` |
| `total-by-period-stacked-bar-chart` | `calculateSeriesKey` | **`calculateSeriesKey`** — exception; `calculateStackKey` is **ignored** here |
| `total-by-pie-chart` | `calculateSeriesKey` | `calculateSeriesKey ?? calculateStackKey` (has fallback) |

If you author an aggregated dashboard with `calculateSeriesKey: "data.quantity"` on a `total-by-bar-chart`, the chart routes to the Aggregated branch, finds `calculateStackKey` undefined, multiplies every bar by 0, and renders the "NO DATA" empty state. **Always rename to `calculateStackKey` for aggregated bar charts.** See [Chart & Metric-Card Pitfalls](#chart--metric-card-pitfalls) #9.

> **`total-by-period-stacked-bar-chart` is the exception to the "aggregated → `calculateStackKey`" rule.** Its aggregated branch runs each period bucket through the period render hook, which computes the bar's value from **`calculateSeriesKey`** (segments are still grouped by `groupStackByKey`); `calculateStackKey` is never read in Aggregated mode. So an aggregated period chart left with `calculateSeriesKey: ""` (even with `calculateStackKey` set to the measure) **counts the aggregated rows per bucket instead of summing the measure** — bars show small integers, or the chart looks empty when each bucket has one row. Set `calculateSeriesKey` to the measure's `targetPath`, and keep `calculateStackKey` aligned to the same field for consistency.

Three other consequences of being in Aggregated mode:

1. **Tile data comes from the tile's own `reportData`,** not from the dashboard's works array. Without `reportData` on the component, the tile receives no data.
2. **`*Key` properties point at fields on the aggregated row** (e.g. the `targetPath` you set in `groupBy` / `fields`), not at fields on a work. Typically `"data.<targetField>"`.
3. **The works array is NOT a reliable source for chart values in Aggregated mode.** `dynamicValue` expressions like `{$.works:JSON}` and `{$.works..fieldName}` may resolve to `undefined` or an empty list because the payload `dynamicValue` runs against in Aggregated mode is the aggregated projection (`data.<reportName>`), not the works array. Anything that needs to count works, sum a field, or walk into nested arrays should be authored as a server-side `reportData` field group and read via `$.data.<targetPath>`. Use `{$.works:JSON}` only as a last-resort tooltip extra on chart shapes that you've verified merge the works array into the chart's per-row data (varies by chart type and Aggregated driver). See Pitfall #8 in [Aggregated-Mode Pitfalls](#aggregated-mode-pitfalls).

### Top-metric / average overlays in Aggregated mode

When a chart sets `showTotal: true` or `showAverage: true` in Aggregated mode, the value is computed by resolving each expression against **`fullData.data`**, not against the per-row aggregated dataset.

What that means for authoring:

- **`total.value`, `secondTotal.value`, `average.value`** are `dynamicValue` expressions evaluated against the dashboard's full aggregated payload (`layout.data`), not the rows array. Typically a `$.`-prefixed path that walks into a separately-named `reportData` field group (e.g. `"$.totals[0].totalQuantity"`).
- **`total.title`, `total.units`** (and the same on `secondTotal` / `average`) are literal strings shown next to the rendered number. If `units` is missing on the inner block, the hook falls back to the chart's `unitOfMeasure` and finally to `""`.
- Gating is per-pill: **`total` requires `showTotal: true`** and **`average` requires `showAverage: true`**, but **`secondTotal` renders whenever it is present** — it is *not* gated by `showTotal`. Author each flag together with its corresponding `total` / `average` object; `secondTotal` needs no flag.

Charts that read these hooks: `total-by-bar-chart` (aggregated; average only), `total-by-stacked-bar-chart` (total + secondTotal + average), `total-by-pie-chart` (total + secondTotal), `multiple-rating-chart` (average only via `useAggregatedAverage`). `total-by-period-stacked-bar-chart` does **not** use either hook today — total/average aren't shown on the period chart.

Read [Configuring Charts & Metric Cards](#configuring-charts--metric-cards-non-aggregated-dashboards) for the property semantics that apply to both modes (key vs. path, "Unknown" buckets, tooltip scopes), then [Configuring Aggregated Reports](#configuring-aggregated-reports-reportdata) for the `reportData` schema.

### Setting the dashboard `dataSource`

`create_dashboard` and `update_dashboard_metadata` both accept a `data_source` argument that controls Works-vs-Aggregated dispatch. Shape:

```jsonc
{
  "name": "Aggregate",                         // "works" → Works mode; "Aggregate" → Aggregated mode
  "properties": {}                             // free-form bag, rarely needed
}
```

- Omit `data_source` for Works mode (the default).
- **For Aggregated mode, use `{"name": "Aggregate", "properties": {}}`.** This is the portable choice — it works on every environment without any environment-level configuration. Each component's own `reportData` drives the rows.
- **Avoid `{"name": "data"}` and other custom names.** They only work on environments that have a matching named driver registered. Aggregated fetches on such dashboards typically return a generic 400 from the dashboard data endpoint when the driver isn't present, with no useful error in the payload — easy to misdiagnose as a permissions, schema, or `reportData` problem.
- **Don't confuse `dataSource.name` with `dataSource.properties.reportName`.** `name` is the dispatch sentinel. `properties.reportName` is just a label that some custom drivers read; with the generic `Aggregate` mode, each component's own `reportData` drives the rows and `reportName` is unused. Putting an arbitrary string in `name` does **not** make the dashboard "aggregated by report X" — it makes it "look up driver X, fail if absent."

`get_dashboard` returns the configuration back under the same singular `dataSource` field. Setting `data_source` requires Owner / Editor / environment-owner on the dashboard, matching the gate on `defaultQueryFilters`.

**If a dashboard returns 400 when fetching aggregated data:** check `get_dashboard(id).dataSource.name`. If it's anything other than `"Aggregate"` (or `"works"`/unset for non-aggregated), patch with `update_dashboard_metadata(id, data_source={"name": "Aggregate", "properties": {}})`. The aggregated rendering and each tile's `reportData` are unchanged — only the dispatch label is.

Pair with the per-component changes in the [Dashboard Modes](#dashboard-modes-works-vs-aggregated) table — switching the dashboard to Aggregated mode is one half of the change; aligning each chart's `*Key` properties and authoring its `reportData` is the other.

## Configuring Charts & Metric Cards (Non-Aggregated Dashboards)

This section covers the default, non-aggregated dashboard mode: the chart receives the dashboard's array of work items and computes grouping / sums / counts client-side.

### Shared Concepts

> Several properties below take expression strings (`value`, `total.value`, `tooltipFields[].value`, etc.). Read `references/dynamicValue.md` for the syntax — get this wrong and tiles render blank.

- **Where data comes from.** Each chart reads an array of work items, resolved from the top-level `dataPath` field on the component (`properties.dataPath` is ignored at render time — the renderer destructures only the top-level field). The renderer hooks two distinct code paths depending on the shape of `dataPath`:
  - **Empty, `null`, or exactly `"$.works"`** → the chart receives the dashboard's full works array verbatim. This is the canonical, healthy default.
  - **A plain `$.`-prefixed string with no `=` and no `{` token** → the path resolver short-circuits to a **lodash `_.get`-style lookup** against the dashboard data. Lodash `get` understands dotted paths and `[<n>]` integer indexers, **but not `[*]` wildcards or `..` recursive descent.** So `"$.summary"` works, `"$.works.0.data.cart"` works, but `"$.works[*].data.cart[*]"` returns `undefined` → chart receives `[]` → renders `NO DATA`. The wildcard form looks documented (the path-sanitizer does accept `[*]`), but the fast path never reaches that code — only the formula path does.
  - **A `=`-prefixed mathjs formula or any value containing `{`** → bypasses the fast path, falls through to a full JSONPath evaluator (`jsonpath.query`/`value`) with the bracket-aware sanitizer. This is the only renderer path that supports `[*]`, filters, slices, etc. The formula form is the canonical Works-mode "filter the works array" pattern, e.g. `dataPath: '=filterByDeepValue({$.works:JSON}, "data.key", "test")'`.
- **`dataPath` validation, in one rule.** The backend validates non-empty `DataPath` values on both add and PATCH endpoints against `^(?:|[$@=][^\x00-\x1F\\;]*)$` — i.e. empty, or starts with `$`, `@`, or `=` and contains no control chars, backslashes, or semicolons. This is intentionally aligned with the SDK's dataPath dispatcher, so anything the renderer can resolve can also be sent through the API. The regex accepts `[`, `]`, `*`, `?`, `(`, `)`, `=`, `{`, `}`, `'`, `"`, `,`, `:` inside the value — which means filters, mathjs formulas, and bracket-notation paths are all authorable through MCP.
- **MCP server pre-processor — be aware.** For `create_dashboard`, `add_dashboard_components`, and `replace_dashboard_section_components`, the server auto-derives `dataPath = "$." + camelCase(label)` whenever the caller-supplied value is falsy (missing, `None`, or `""`). `update_dashboard_component` (PATCH) is the only tool that forwards `component_data` verbatim with no pre-processing — so it's the escape hatch when you want a specific value (including `""`) to actually reach the API.
- **What `dataPath` shapes the SDK actually renders.** The API will store any of the values below, but the renderer's dispatch logic decides whether a value resolves to anything. The SDK dispatcher runs three branches, picked by the first/leading character:
  - **Empty / `null` / `"$.works"`** → chart receives the full works array. Canonical default.
  - **`$.`-prefixed, no `=` and no `{`** (e.g. `"$.summary"`, `"$.foo.bar"`, `"$.foo[0]"`) → lodash `_.get`. Understands dotted paths and integer indexers — **does not** understand `[*]` wildcards or `..` recursive descent. So `"$.works[*].data.cart[*]"` API-validates fine but lodash returns `undefined` → empty chart.
  - **`$.`-prefixed AND contains `=` or `{`** (e.g. `"$.values[?(@.group == 'supplier')].metrics"`) → bypasses lodash, falls to the JSONPath branch via `tryGetValueFromData`. Filters, slices, unions, wildcards, recursive descent all work here because of the `=` inside the filter expression.
  - **`=`-prefixed** (e.g. `"=filterByDeepValue({$.works:JSON}, \"data.key\", \"value\")"`, `"=count({$.works:JSON})"`) → mathjs formula. `{$.path}` tokens inside are interpolated via JSONPath. This is the canonical Works-mode "filter the works array" pattern.
  - **`@`-prefixed** (e.g. `"@row.foo"`, `"@options.value"`, `"@[$.id]"`) → row / repeater / chart-context resolution.
- **Sub-array charts in Works mode — authorable now.** Two paths work:
  - **Filter form:** `dataPath: "$.values[?(@.group == 'supplier')].metrics"` — the `==` inside the filter pushes the renderer past the lodash fast path into the JSONPath branch.
  - **Formula form:** `dataPath: "=filterByDeepValue({$.works:JSON}, \"data.key\", \"value\")"` — the `=` prefix routes to mathjs, which resolves `{$.works:JSON}` via JSONPath before applying the helper.
  - **Aggregated mode is still the cleanest option** for any non-trivial sub-array reporting (`reportData.expandByDataPath: "data.cart"` etc.). Flip the dashboard with `data_source` — see [Setting the dashboard `dataSource`](#setting-the-dashboard-datasource).
- **Wrong (silent failure): `"$.works[*]"` or `"$.works..cart"` on their own.** Both API-validate, but neither contains `=` or `{`, so the renderer takes the lodash fast path — and lodash understands neither `[*]` nor `..`. The chart silently renders empty. *Fix:* wrap in a formula (`"=$.works..cart"` is too literal for mathjs; use a token: `"={$.works..cart:JSON}"`) or move to Aggregated mode.
- **Keys vs. paths.** Properties named `*Key` (e.g. `groupSeriesByKey`, `calculateSeriesKey`) take a **field accessor on a work item** — never a JSONPath. The accessor goes through lodash `_.property`, so:
  - Flat field: `"courier"` → reads `work.courier`.
  - **Nested field via dotted path: `"data.courier"` → reads `work.data.courier`.** This is the common case in Rise-X because form field values typically live under `work.data.<fieldName>`.
  - `"$.courier"` is **wrong** here — it isn't a JSONPath and the leading `$` will fail the lookup, dumping every work into the "Unknown" bucket (see pitfalls).
  - Properties named `*Path` (e.g. `xAxis.valuePath`, metric-card `value`) are different — they accept `$.`-prefixed JSONPaths or `=`-prefixed `dynamicValue` expressions.
- **Counting vs. summing.** For chart families that support both, omit the calculate key to count work items in each bucket; supply the calculate key to sum a numeric field.
- **Unknown buckets.** Items whose grouping field resolves to missing / `undefined` / `null` / the strings `"undefined"`, `"null"`, `"unknown"` are re-bucketed under the literal label `"Unknown"`. If every bar is labelled "Unknown", the cause is almost always that `groupSeriesByKey` points at the wrong path on the work — try the dotted-path form (e.g. `"data.courier"`).
- **Title / width.** All charts honor `title` (string) and `showTitle` (bool). All accept a `width` like `"col-12"`, `"col-6"`, etc.
- **Tooltip fields.** Most charts accept `tooltipFields: [{ name, value, unit? }]`. `value` is run through `dynamicValue` against **two** data scopes:
  - `works` — the dashboard's full works array (use token form: `"{$.works..fieldName}"` or `=`-prefixed mathjs formulas like `"=sum({$.works..amount})"`).
  - `@options` — the **current bar / slice / row's chart context**, exposing `category` (the bucket label), `value` (the bucket's numeric value), `percentage`, `name` (series name), `total`. Reference these as `"{@options.category}"`, `"{@options.value}"`, etc. — token form only, no `$.` prefix.

  A bare string like `"courier"` is treated as a literal and prints the word `courier` in the tooltip — see pitfall #8.

### `total-by-bar-chart`

Counts or sums work items grouped by one field, rendered as bars.

Required:
- `groupSeriesByKey` — accessor on each work to group by. Use the dotted path that matches where the field actually lives on the work — typically `"data.<fieldName>"` for form-field values (e.g. `"data.courier"`). A flat field name like `"courier"` only works if the field is at the work root; if every bar shows "Unknown" the path is wrong.

Optional:
- `calculateSeriesKey` — accessor for the numeric field to sum per group (same dotted-path rules as `groupSeriesByKey`). Omit to count items.
- `horizontal` (bool) — horizontal bars when `true`.
- `hasLabelsAtTheEndOfDataBars` (bool) — render value at bar end.
- `unitOfMeasure` (string), `decimals` (number).
- `tooltipFields`.

Example — count of works per courier (field at `work.data.courier`):

```jsonc
{
  "component": "total-by-bar-chart",
  "label": "Order Count by Courier",
  "dataPath": "",                       // ← required; see pitfall #12
  "properties": {
    "title": "Order Count by Courier",
    "showTitle": true,
    "width": "col-12",
    "horizontal": false,
    "hasLabelsAtTheEndOfDataBars": true,
    "groupSeriesByKey": "data.courier",
    "calculateSeriesKey": "",           // ← required empty init; see pitfall #13
    "calculateStackKey": "",
    "countSeriesKey": "",
    "tooltip": "",
    "unitOfMeasure": " ",               // ← single space; see pitfall #14
    "tooltipFields": [
      { "name": "Courier", "value": "{@options.category}" },
      { "name": "Orders",  "value": "{@options.value}" }
    ]
  }
}
```

For "no unit" set `unitOfMeasure` to a single space `" "`, NOT `""` / `null` / omitted — those all render as the literal text `undefined` on bar labels and Y-axis ticks. See pitfall #14.

The X-axis label and the bar's count come from the chart's own context (`@options.category` and `@options.value`) — not from the works array — so the tooltip remains correct even when `calculateSeriesKey` is added later.

To sum order amount per courier instead, add `"calculateSeriesKey": "amount"` and a `unitOfMeasure`.

### `total-by-clustered-bar-chart`

> ⚠️ **Aggregated mode only.** The SDK ships a renderer, but it is wired only to the Aggregated path: when the dashboard's `dataSource` is Aggregated (`"Aggregate"` or a custom aggregate driver) the chart renders via `TotalByClusteredBarChartWithAggregations`. On a **Works-mode** dashboard the dispatch returns `null` (the "with calculations" path is explicitly not supported yet), so the tile renders blank. **On Works dashboards prefer `total-by-bar-chart`; reach for the clustered chart only on Aggregated dashboards.**

Clustered (grouped) bar chart: two grouping dimensions drawn as side-by-side bars. In the Aggregated render path it groups rows by `groupSeriesByKey` for the **X-axis category** and by `groupStackByKey` for the **clusters within each category**, summing the measure `calculateStackKey ?? calculateSeriesKey` per (category, cluster) pair. It also supports an `average` overlay block (`showAverage` / `hasAverageLine` / `averageLinesFromSeries`), `allowNegativeValues`, and end-of-bar labels.

Required (Aggregated mode):
- `groupSeriesByKey` — accessor for the X-axis category grouping (dotted path against the aggregated row, e.g. `"tags.supplier"`).
- `groupStackByKey` — accessor for the cluster grouping within each category (e.g. `"tags.product"`). This is a **real** second grouping dimension on this chart (see note below).
- `calculateStackKey` (with `calculateSeriesKey` as fallback) — the measure to sum per (category, cluster), e.g. `"=round({$.savingsUsdPerMt}, 2)"`.

Optional:
- `calculateSeriesKey` — fallback measure when `calculateStackKey` is unset. (Only consulted via the `calculateStackKey ?? calculateSeriesKey` chain; the Works-mode "count items when omitted" behavior never fires because the Works renderer is `null`.)
- `countSeriesKey` — present for symmetry with `total-by-bar-chart`'s empty-init convention.
- `horizontal` (bool, default `false`) — horizontal bars when `true`.
- `hasLabelsAtTheEndOfDataBars` (bool) — render value at bar end.
- `showAverage` (bool), `hasAverageLine` (bool), `averageLinesFromSeries` (bool), `average: { label, value }` — average / target overlay block. `averageLinesFromSeries` (unique to this chart — not present on `total-by-bar-chart` or `total-by-stacked-bar-chart`) draws one average line per series instead of one global line.
- `showPercentageForTooltip` (bool) — adds a `%` row to the bar tooltip.
- `allowNegativeValues` (bool) — when `true`, negative values are not clipped at zero.
- `unitOfMeasure` (string), `decimals` (number), `displayAllDecimals` (bool), `tooltip` (string), `tooltipFields`.

Schema defaults from `OnNew` (`Width = "col-12"`, `ShowTitle = false`, `Horizontal = false`, and all of `groupSeriesByKey` / `groupStackByKey` / `calculateSeriesKey` / `calculateStackKey` / `countSeriesKey` / `unitOfMeasure` initialized to empty strings) — so when authoring via MCP, seed those `*Key` properties as empty strings explicitly (same empty-init convention as `total-by-bar-chart`, see pitfall #13).

Example — Aggregated dashboard: average savings per supplier (X-axis) clustered by product, with an average overlay. The `dataPath` is a JSONPath filter into the aggregated rows (Aggregated mode); `groupSeriesByKey`/`groupStackByKey` are plain dotted lodash paths against each row (no `$.` — same rule as every other chart, see pitfall #6); `calculateStackKey` is a formula expression (valid because the `=` prefix routes it through `dynamicValue`'s formula evaluator, not the lodash fast path):

```jsonc
{
  "component": "total-by-clustered-bar-chart",
  "label": "Average savings by product ($/MT)",
  "dataPath": "$.values[?(@.group == 'product_supplier')].metrics",
  "properties": {
    "title": "Average savings by product ($/MT)",
    "showTitle": true,
    "width": "col-4",
    "horizontal": false,
    "hasLabelsAtTheEndOfDataBars": true,
    "allowNegativeValues": true,
    "groupSeriesByKey": "tags.supplier",   // X-axis category (dotted path, no $. prefix)
    "groupStackByKey": "tags.product",     // clusters within each category
    "calculateStackKey": "=round({$.savingsUsdPerMt}, 2)",
    "calculateSeriesKey": "=round({$.savingsUsdPerMt}, 2)",
    "countSeriesKey": "",
    "decimals": 2,
    "displayAllDecimals": true,
    "unitOfMeasure": "$/MT",
    "tooltipFields": [
      { "name": "Savings", "value": "{@options.value}", "unit": "$/MT" }
    ]
  }
}
```

For an X-axis-only chart with a mean/target line and no second grouping, set `groupStackByKey` to the same accessor as `groupSeriesByKey` (or leave it empty) and use the `average` block: `showAverage` / `hasAverageLine` / `averageLinesFromSeries` (the last draws one average line per series instead of one global line — unique to this chart). For a stacked (not side-by-side) second dimension, use `total-by-stacked-bar-chart` instead.

### `total-by-stacked-bar-chart`

Two-dimensional grouping: one field becomes the X-axis category, another splits each bar into stacked segments.

Required:
- `groupSeriesByKey` — X-axis category field.
- `groupStackByKey` — segment field within each bar.

Optional:
- `calculateStackKey` — sum this numeric field per (category, segment) cell. Omit to count items.
- `calculateSeriesKey` — sum used for total/labels at the category level.
- `horizontal`, `showTotal`, `total: { title, value, units? }` (optionally `secondTotal: { title, value, units? }`), `unitOfMeasure`, `decimals`, `tooltipFields`. Same total/pill semantics as `total-by-pie-chart` (both read the `useAggregatedTotal` hook): `title` is the pill caption, `label` is ignored.

```jsonc
{
  "component": "total-by-stacked-bar-chart",
  "label": "Orders by Region & Status",
  "dataPath": "",                       // ← required; see pitfall #12
  "properties": {
    "title": "Orders by Region & Status",
    "showTitle": true,
    "groupSeriesByKey": "data.region",
    "groupStackByKey": "data.status",
    "horizontal": false
  }
}
```

### `total-by-pie-chart`

One-dimensional grouping rendered as a pie.

Required:
- `groupSeriesByKey`.

Optional:
- `calculateSeriesKey` — sum per slice. Omit to count.
- `countSeriesKey` — accessor used by `groupAndCalculateWorksValueOfAKey` to count items grouped under the same key when `calculateSeriesKey` is empty. Empty-init like the bar chart.
- `limit` (number) — keep top N slices (default 10).
- `showConnectors` (bool), `unitOfMeasure`, `decimals`.
- `showTotal` (bool) + `total: { value, title?, units? }` and optionally `secondTotal: { value, title?, units? }` — these are **header pills** rendered in the card header *above* the pie, **not** text inside the donut. `total` renders only when `showTotal: true`; `secondTotal` renders whenever it is present. Each `value` is a `dynamicValue` expression: the **Works** branch resolves it against the works array; the **Aggregated** branch resolves it against the **full report payload** (`fullData.data`) — so the expression can walk into a *different* group than the slices' own `dataPath`. That cross-group reach is what lets a per-slice donut also surface a broader total (e.g. an all-data total) — see the comparison-pill recipe below. `title` is the pill's caption; `units` falls back to the chart's `unitOfMeasure`, then `""`. A `label` key inside these blocks is **ignored** — use `title`.
- `totalLabel` (string) — the caption under the number shown in the **center** of the donut. The center always displays the **sum of the rendered slices** with this caption beneath it. The center is visible **unless `secondTotal` is set**: setting `secondTotal` hides the center and turns both `total` and `secondTotal` into side-by-side header pills. `totalLabel` is a chart-level property (sibling of `total`), **not** a field inside `total`.
- `tooltipFields`.

Example — **Works mode** (dotted `*Key` accessors; `dataPath: ""` reads the works array):

```jsonc
{
  "component": "total-by-pie-chart",
  "label": "Revenue by Region",
  "dataPath": "",                       // ← required; see pitfall #12
  "properties": {
    "title": "Revenue by Region",
    "showTitle": true,
    "groupSeriesByKey": "data.region",
    "calculateSeriesKey": "data.amount",
    "unitOfMeasure": "$",
    "decimals": 2,
    "limit": 8
  }
}
```

**Recipe — donut-center total plus a broader comparison total as a header pill (Aggregated mode).** The donut center shows the sum of the slices that are drawn (a *scoped* total), and a header pill shows a *different, broader* total read from another group in the aggregated payload — e.g. a per-category breakdown of one scope alongside an all-data total. The same shape covers any "this slice of the data vs. the whole dataset" comparison; the group names below are illustrative — use whatever groups your component's `reportData` produces.

- Keep the slices scoped — `dataPath` points at the scoped group and `groupSeriesByKey` / `calculateSeriesKey` are set as usual. The donut center auto-sums those slices, so the center number is the scoped total.
- **`*Key` accessors stay dotted lodash paths**, resolved against each aggregated row (`groupSeriesByKey: "tags.region"`, `calculateSeriesKey: "totalVolume"`) — the same rule as everywhere else in this doc (pitfalls #1 / #10 / #6), **not** `$.`-prefixed. (In Aggregated mode `dynamicValue` happens to strip a leading `$.`, so `"$.tags.region"` resolves the same as `"tags.region"` — but keep them dotted for consistency; don't rely on the strip.) This is distinct from `dataPath` and the pill's `value` below, which are `*Path` properties and **do** take `$.`-prefixed JSONPaths (including `[?(...)]` filters).
- `showTotal: true` plus `total: { title, value, units? }` produces the pill. Because the Aggregated branch resolves `value` against the full payload, point it at a **different group** than the slices' `dataPath` — e.g. a grand-total group: `"$.values[?(@.group == 'total')].metrics[0].<measureField>"`.
- `totalLabel` sets the donut-center caption.
- **Do not set `secondTotal`.** Setting it hides the center and turns both totals into pills. Use `secondTotal` only when you deliberately want two header pills and no center number.

```jsonc
{
  "component": "total-by-pie-chart",
  "label": "Volume by region",
  "dataPath": "$.values[?(@.group == 'region')].metrics",   // scoped slices (one group)
  "properties": {
    "title": "Volume by region",
    "showTitle": true,
    "groupSeriesByKey": "tags.region",
    "calculateSeriesKey": "totalVolume",
    "unitOfMeasure": "MT",
    "decimals": 0,
    "showConnectors": true,
    "showTotal": true,
    "totalLabel": "Selected scope total",                   // donut-center caption (sum of slices)
    "total": {                                              // header pill, read from a different group
      "title": "All-data total",
      "value": "$.values[?(@.group == 'total')].metrics[0].totalVolume",
      "units": "MT"
    }
  }
}
```

### `total-by-period-stacked-bar-chart`

Time-series stacked bars. Buckets work items by a date field into periods, then stacks each period by a category field.

Required:
- `groupSeriesByKey` — **date** field on the work (ISO 8601 string).
- `groupStackByKey` — segment field.

Optional:
- `calculateSeriesKey` — sum per (period, segment). **This is the value key the aggregated period chart reads** (not `calculateStackKey` — see the exception note under [Dashboard Modes](#dashboard-modes-works-vs-aggregated)). Omit to count.
- `calculateStackKey` — keep aligned to the same field as `calculateSeriesKey` for consistency with the other stacked charts; ignored by the period render path in Aggregated mode.
- `period` — `"Day"` | `"Week"` | `"Month"` | `"Year"`. If omitted, period is auto-selected from the date range.
- `unitOfMeasure`, `decimals`, `labelNumberFormatterOptions` (Intl.NumberFormat options, e.g. `{ "notation": "compact" }`).

```jsonc
{
  "component": "total-by-period-stacked-bar-chart",
  "label": "Monthly Volume by Status",
  "dataPath": "",                       // ← required; see pitfall #12
  "properties": {
    "title": "Monthly Volume by Status",
    "showTitle": true,
    "groupSeriesByKey": "data.createdDate",
    "groupStackByKey": "data.status",
    "calculateSeriesKey": "data.volume",  // ← aggregated period chart reads THIS for the bar value
    "calculateStackKey": "data.volume",   // keep aligned; ignored by the period render path
    "period": "Month"
  }
}
```

### `multiple-rating-chart`

Renders one star-rating row per group, taking the rating value from the first work in each group.

Required:
- `groupStackByKey` — field whose distinct values become the rows (e.g. `"vendor"`).
- `calculateSeriesKey` — path to the rating value on a work (use `$.`-prefixed path, e.g. `"$.performanceRating"`).

Optional:
- `showAverage` (bool), `average: { label, value, title?, units? }` — when `showAverage: true`, an aggregated-average pill appears above the ratings list, computed via `useAggregatedAverage(average.value)` against `fullData.data`. Empty / missing `average` hides the pill.
- `tooltip` (string) — icon-tooltip text shown next to the chart title.
- `decimals`, `displayAllDecimals` (bool), `tooltipFields`.

```jsonc
{
  "component": "multiple-rating-chart",
  "label": "Vendor Ratings",
  "properties": {
    "title": "Vendor Ratings",
    "showTitle": true,
    "groupStackByKey": "vendor",
    "calculateSeriesKey": "$.performanceRating",
    "showAverage": true,
    "decimals": 1
  }
}
```

### `heatmap-chart`

Counts work items in a fixed grid defined by enumerated X and Y axis values.

Required:
- `xAxis.valuePath` — field name on a work used to place it on X.
- `yAxis.valuePath` — field name used to place it on Y.
- `xAxis.items: [{ value, displayName }]` and `yAxis.items: [{ value, displayName }]` — explicit cell labels. Cells are the cartesian product of the two `items` lists.

Optional:
- `xAxis.title`, `yAxis.title`.
- `categoryPath` — field used in tooltip category label.
- `countFlags: [{ valuePath, displayName }]` — adds count breakdowns to each cell tooltip.
- `colors` — hex color array (≥ 2). Omit to use the default gradient.
- `validationFilters: [{ valuePath, rules }]` — exclude items failing a validation rule.

```jsonc
{
  "component": "heatmap-chart",
  "label": "Status by Region",
  "properties": {
    "title": "Status by Region",
    "showTitle": true,
    "xAxis": {
      "title": "Region",
      "valuePath": "region",
      "items": [
        { "value": "EU", "displayName": "Europe" },
        { "value": "NA", "displayName": "North America" }
      ]
    },
    "yAxis": {
      "title": "Status",
      "valuePath": "status",
      "items": [
        { "value": "Open",   "displayName": "Open" },
        { "value": "Closed", "displayName": "Closed" }
      ]
    }
  }
}
```

### `metric-card`

Single KPI tile.

Required:
- `value` — expression evaluated by `dynamicValue` (see `references/dynamicValue.md`). Either a JSONPath (`"$.summary.revenue"`), a token-substituted string, or a `=`-prefixed mathjs formula. Reference works via tokens — not as a free symbol. Examples: `"=count({$.works:JSON})"` (canonical "count of works"), `"=sum({$.works..amount})"`, `"=mean({$.works..score})"`. **Do not** use `..id` to count works — `id` is a top-level work field and isn't reachable via recursive descent from the dashboard's `works` array; the formula will render `0`.

Optional:
- `unitOfMeasure`, `decimals`, `displayAllDecimals` (bool).
- `format` — Luxon duration format (e.g. `"hh:mm:ss"`) when the value is a duration in seconds.

```jsonc
{
  "component": "metric-card",
  "label": "Total Orders",
  "dataPath": "",                       // ← required; see pitfall #12
  "properties": {
    "title": "Total Orders",
    "showTitle": true,
    "width": "col-4",
    "value": "=count({$.works:JSON})"
  }
}
```

### `grouped-metric-card`

Multi-row metric tile. **Property semantics depend on the dashboard's mode** — this is the most mode-divergent component in the dashboard set.

**Works mode** (`layout.dataSource` unset or `"works"`):

`value` must evaluate to an object of `{ label: number }`. The component iterates that map and renders one row per key.

Required:
- `value` — formula returning a label→number map. Use the Rise-X grouped helpers documented in `references/dynamicValue.md`. Pass the works array via the `:JSON` formatter so the helper receives a real array. Examples: `"=groupedCountOfFilteredValues({$.works:JSON}, \"\", \"\", \"status\")"` (count works per status), `"=groupedSumOfFilteredValues({$.works:JSON}, \"\", \"\", \"region\", \"amount\")"` (sum amount per region).

Optional:
- `secondValue` — same `{ label: number }` shape; rendered as a secondary column on the matching row.
- `showTotal`, `total: { label, value, title?, units? }` — bottom-row total. `value` evaluates against the works array.
- `unitOfMeasure`, `decimals`.
- `valueLabel` is **ignored** in Works mode.

**Aggregated mode** (`layout.dataSource.name: "Aggregate"`):

The component receives the aggregated rows array (one row per `groupBy` bucket) and evaluates `valueLabel` / `value` / `secondValue` **per row**. Each row becomes one rendered metric line.

Required:
- `valueLabel` — `dynamicValue` expression resolved against each row to produce the row label. Typically a path like `"$.data.modelName"` or a formatted expression. Defaults to empty string if unresolved.
- `value` — `dynamicValue` expression resolved against each row to produce the row's numeric value. Typically `"$.data.quantity"` (matching the `targetPath` of a `fields[]` entry in `reportData`). Defaults to `0` if unresolved.

Optional:
- `secondValue` — `dynamicValue` per-row expression for the secondary column. Defaults to `""` if unresolved.
- `showTotal`, `total: { value, title?, units? }` — total/footer row. In Aggregated mode `total.value` evaluates against `fullData.data` (the dashboard's full aggregated payload), not against the rows. Use `:JSON` token forms or aggregate-engine paths.
- `unitOfMeasure`, `decimals`.

```jsonc
{
  "component": "grouped-metric-card",
  "label": "Orders by Status",
  "dataPath": "",                       // ← required; see pitfall #12
  "properties": {
    "title": "Orders by Status",
    "showTitle": true,
    "width": "col-4",
    "value": "=groupedCountOfFilteredValues({$.works:JSON}, \"\", \"\", \"status\")",
    "showTotal": true,
    "total": { "label": "Total", "value": "=count({$.works:JSON})" }
  }
}
```

### `rating-metric-card`

A single star-rating tile.

Required:
- `value` — expression evaluating to a number (intended range 0–5), e.g. `"=mean({$.works..satisfactionScore})"`.

Optional:
- `decimals`, `displayAllDecimals`.

```jsonc
{
  "component": "rating-metric-card",
  "label": "Customer Satisfaction",
  "dataPath": "",                       // ← required; see pitfall #12
  "properties": {
    "title": "Customer Satisfaction",
    "showTitle": true,
    "width": "col-4",
    "value": "=mean({$.works..satisfactionScore})",
    "decimals": 1
  }
}
```

### `savings-metric-card`

Specialised tile that runs one of three weighted savings formulas across all works. Uniquely among metric cards, this component is **mode-aware**: in Works mode it computes savings from the works array directly; in Aggregated mode (`hasDifferentDataSource: true`) it reads a pre-computed savings object from `value` and unpacks it into High / Mid / Low rows.

Required:
- `calculateFn` — `"savingsByQuantity"` | `"savingsByEfficiency"` | `"savingsTotal"`. Drives Works-mode computation.
- `value` (Aggregated mode only) — a `dynamicValue` expression that resolves to an object of shape `[{ name: "high"|"mid"|"low", value: number[] }, ...]`. In Works mode this property is ignored.

Required for `savingsByQuantity` / `savingsTotal`:
- `calculateSavingsByQuantityKey` — work field with quantity.
- `calculateSavingsByProductPriceKey` — work field with unit price.

Required for `savingsByEfficiency` / `savingsTotal`:
- `calculateSavingsByTimeKey` — work field with time invested.

Required:
- `ranks: { high, mid, low }` — weighting per severity level for the chosen formula.
- `ranksTotal: { savingsByQuantity: {...}, savingsByEfficiency: {...} }` — only when `calculateFn = "savingsTotal"`.

Optional:
- `unitOfMeasure`, `decimals`.

```jsonc
{
  "component": "savings-metric-card",
  "label": "Quantity Savings",
  "dataPath": "",                       // ← required; see pitfall #12
  "properties": {
    "title": "Quantity Savings",
    "showTitle": true,
    "width": "col-4",
    "calculateFn": "savingsByQuantity",
    "calculateSavingsByQuantityKey": "quantity",
    "calculateSavingsByProductPriceKey": "price",
    "ranks": { "high": 1.5, "mid": 1.0, "low": 0.5 },
    "unitOfMeasure": "$",
    "decimals": 2
  }
}
```

### Chart & Metric-Card Pitfalls

1. **`*Key` properties are lodash paths, not JSONPaths.** They go through `_.property`, so flat names (`"courier"`) and dotted paths (`"data.courier"`) both work — but `"$.courier"` does not. In Rise-X, form-field values typically live at `work.data.<field>`, so the dotted form is usually what you want. *Why this matters:* a chart that "renders" but shows every bar as "Unknown" almost always means the key resolved to `undefined` on every work and got re-bucketed. *How to apply:* if your data is at `work.foo` use `"foo"`; if it's at `work.data.foo` use `"data.foo"`. Never start a `*Key` with `$` — dotted is the one form that resolves in **both** Works and Aggregated mode. (Aggregated mode happens to *tolerate* a leading `$.`: that branch resolves `*Key` via `dynamicValue`, which strips a leading `$.` before the lodash lookup, so `"$.tags.region"` and `"tags.region"` behave identically there. Works mode does **not** strip it, so `"$.courier"` fails outright and re-buckets everything to "Unknown". Don't rely on the tolerance — stay dotted everywhere.)
2. **All bars labelled "Unknown" = wrong path on `groupSeriesByKey`.** The chart re-buckets `undefined` / `null` / `"unknown"` results into a literal `"Unknown"` group (`Support.normalizeWithUnknown`). The bar is real (the count is right), but the X-axis label is the giveaway — fix the path, not the data.
3. **Aggregated field names in non-aggregated mode.** Don't reference fields like `"orderCount"` that only exist in an aggregated payload. In non-aggregated mode the chart sees raw works; group by an actual work field and let the chart count.
4. **`countSeriesKey` is not "what to count".** For bar/pie charts, omit `calculateSeriesKey` to count items. Use `calculateSeriesKey` to sum a numeric field. Don't put a label like `"orderCount"` into a `*Key` property hoping the chart will name the count axis.
5. **Period chart needs a real date field.** `groupSeriesByKey` on `total-by-period-stacked-bar-chart` must point to an ISO 8601 date string on each work. Non-date fields produce an empty chart.
6. **Heatmap requires explicit `items`.** Empty `xAxis.items` or `yAxis.items` produces an empty grid; the component does not infer cells from the data.
7. **Expressions go through `dynamicValue`.** `=`-prefixed strings are mathjs formulas, but `works` isn't a free symbol — reference it via tokens like `{$.works..amount}` (recursive descent into each work's `data` payload) or `{$.works:JSON}` (full array, required for the "count of works" pattern: `=count({$.works:JSON})`). Top-level work fields (`id`, `displayName`, `createdDate`, etc.) are **not** reachable via `..` — use `:JSON` instead. See `references/dynamicValue.md` for the full syntax and helper list.
8. **Tooltip `value` is `dynamicValue`, not a field name.** A bare string like `"courier"` is treated as a literal and the tooltip prints the word `courier`. To show the bar's own label/value, use the chart-context tokens: `"{@options.category}"` (bucket label), `"{@options.value}"` (bucket numeric value), `"{@options.percentage}"`, `"{@options.total}"`. To show an aggregate over the whole works array, use `"=sum({$.works..amount})"` etc. *Why this matters:* the tooltip's data scope is the chart's own per-bucket context (`@options`) plus the full `works` array — it is **not** scoped to the underlying work record, so referencing a raw field name has nothing to resolve against.
9. **Aggregated `total-by-bar-chart` reads `calculateStackKey`, not `calculateSeriesKey`.** When `layout.dataSource.name` is non-Works (typically `"Aggregate"`), the chart routes to `TotalByBarChartWithAggregations`, which reads **`calculateStackKey`** with no fallback. A property of `calculateSeriesKey` on an aggregated bar chart is silently ignored, every bar's value is `0`, and the chart renders the "NO DATA" state. The pie chart's aggregated path has a `calculateSeriesKey ?? calculateStackKey` fallback; the bar chart does not. *How to apply:* whenever a `total-by-bar-chart` has `reportData` (or sits on a layout with `dataSource.name: "Aggregate"`), use `calculateStackKey`. For `total-by-stacked-bar-chart`, `calculateStackKey` is the right name in the Aggregated branch. **`total-by-period-stacked-bar-chart` is the exception:** its aggregated branch runs through the period render hook and reads **`calculateSeriesKey`** for each bar's value — `calculateStackKey` is ignored there. An aggregated period chart with `calculateSeriesKey: ""` (and only `calculateStackKey` set) counts the aggregated rows per bucket instead of summing the measure, so bars show small integers or the chart looks blank. Set `calculateSeriesKey` to the measure's `targetPath`.
10. **Aggregated chart `*Key` paths point at the aggregated row, not at a work.** The `targetPath` you set inside `reportData.groupBy[]` and `reportData.fields[]` is what the chart sees on each row — typically `"data.<targetField>"`. So `"groupSeriesByKey": "data.modelName"` matches the row produced by `groupBy: [{ targetPath: "data.modelName" }]`. Pointing a `*Key` at a work-level field (e.g. `"data.courier"` when `targetPath` is `"data.modelName"`) renders every row as "Unknown".
11. **Sub-array charts in Works mode — pick a shape the SDK will dispatch to JSONPath/mathjs, not lodash.** The chart's `dataPath` needs to reach the right SDK code path; the API regex no longer blocks any of the candidate shapes (it's aligned with the dispatcher), but the SDK still routes plain `$.`-prefixed values into a lodash `_.get` fast path that doesn't understand wildcards or recursive descent. So:
    - **Plain wildcard `"$.works[*].data.cart[*]"`** — passes the API regex now, but the SDK takes the lodash branch (`$.` + no `=` + no `{`) → lodash returns `undefined` → chart renders `NO DATA`. Same outcome for `"$.works..cart"`.
    - **Filter form `"$.values[?(@.group == 'supplier')].metrics"`** — works end-to-end. The `==` inside the filter satisfies the SDK's "contains `=`" condition, pushing dispatch to the JSONPath branch.
    - **Mathjs formula `"=filterByDeepValue({$.works:JSON}, \"data.key\", \"value\")"`** — works end-to-end. `=` prefix routes to mathjs; the `{$.works:JSON}` token is interpolated via JSONPath before the helper runs.
    *How to apply:* (a) prefer **Aggregated mode** for any non-trivial sub-array reporting — set `data_source` to `{"name": "Aggregate", "properties": {}}` via `create_dashboard` or `update_dashboard_metadata` (see [Setting the dashboard `dataSource`](#setting-the-dashboard-datasource)) and author each chart's `reportData`. (b) If you need to stay in Works mode, use the filter or formula form above; do not use plain `[*]` or `..` paths.
12. **`dataPath` auto-derivation is in the MCP server, not the API — and PATCH is the only escape hatch.** Three of the four mutating dashboard tools (`create_dashboard`, `add_dashboard_components`, `replace_dashboard_section_components`) run a pre-processor that does, per non-section component: `if label and not dataPath: dataPath = f"$.{camelCase(label)}"`. Falsy here means missing, `None`, or `""` — so passing `"dataPath": ""` on these three tools is **silently replaced** with `$.<labelCamelCase>` *before* the API ever sees it. That auto-derived path points at a non-existent field on each work, so charts render `NO DATA` and metric cards render `0`. (The backend also fills `null` DataPaths with a default, but only on `null`, not on `""` — and the API regex short-circuits empty/null to valid. So the empty string would pass through to storage just fine if it ever escaped the MCP server's wrapper.) **`update_dashboard_component` (PATCH) forwards `component_data` verbatim** — no MCP pre-processing, no API auto-derive — and the API regex accepts `""`. That makes PATCH the single tool through which a truly empty `dataPath` can be set. *How to apply:* (a) on every new non-section dashboard component you add through `create_dashboard` / `add_dashboard_components` / `replace_dashboard_section_components`, inspect the returned layout — anything with `dataPath: $.<labelCamelCase>` is broken until PATCHed; (b) follow up with `update_dashboard_component(draft_id, component_id, {"dataPath": ""})` to clear it; (c) alternatively, pre-supply a real, valid dotted path (e.g. `"dataPath": "$.summary"`) on the original call to short-circuit the auto-derive (truthy values are preserved); (d) if you want the chart to operate on the full works array directly, `""` and `"$.works"` are both equivalent at render time.
13. **Match the schema's `OnNew` empty-string init shape for `*Key` properties.** The bar / pie / stacked / period-stacked schemas' `OnNew` methods seed every `*Key` property (`calculateSeriesKey`, `calculateStackKey`, `countSeriesKey`, `groupSeriesByKey`, `groupStackByKey`, `unitOfMeasure`) to `string.Empty` on creation. The frontend code doesn't *require* these to be present and the grouping helpers (`groupAndCalculateWorksValueOfAKey`, `groupBy`) handle that without crashing. **But:** the stacked / period-stacked charts call `throwIfNoRequiredPropertiesForStackedChart` with `requiredProps: ['groupStackByKey']`, which **does** throw — and the boundary catches it as a render error. *How to apply:* (a) on **stacked / period-stacked charts**, you must include `groupStackByKey` with a real value (not empty) or the tile throws. (b) on **all chart types**, prefer seeding the full `*Key` block as empty strings to match the schema's `OnNew` shape — it keeps PATCH diffs clean and avoids reintroducing undefined-vs-empty mismatch later. (c) "NO DATA" rendering when keys are absent is usually a *data* issue (rows not matching the path), not a missing-property issue — check `dataPath` first (pitfall #12).
14. **`unitOfMeasure: undefined` renders as the literal text `undefined` on `total-by-bar-chart` (and the other bar charts).** The bar chart's Y-axis tick formatter and stack-label formatter interpolate the unit directly with no fallback: `` `${number.format(this.value)} ${unitOfMeasure}` ``. JS template literals coerce `undefined` to the string `"undefined"`, so missing the key produces `"2 undefined"`, `"1 undefined"`, etc. on every tick. (`null` coerces to `"null"`; `""` renders just a trailing space — those are different failure modes, only `undefined` produces literal `"undefined"`. The schema's `OnNew` seeds the property to `""` on new components, so this only bites when a PATCH explicitly sets `null` or unsets the key.) Metric cards are **not** affected — `getMetricCardValue` uses `${units ? ` ${units}` : ''}`. *How to apply:* on bar / stacked-bar / period-stacked / clustered-bar charts, when the chart needs no unit set `"unitOfMeasure": " "` (a single space) or `""` (empty string). Both render as a visually-empty suffix. Use a real word (`"$"`, `"kg"`, `"units"`) when the suffix is meaningful. Don't leave the key unset on these charts.
15. **Aggregated mode + custom drivers: `dataPath: ""` is wrong on metric cards.** Pitfall #12 above is correct for Works mode but flips on Aggregated dashboards whose `dataSource.name` is a custom driver (e.g. `"AraDelivery"`, `"Aggregate"`). The render path is `useDashboardComponent` → `getDashboardComponentWorksByDataPath` → `dynamicValue(dataPath, {data})`. When `dataPath === ""`, `dynamicValue` short-circuits on the empty/falsy path (its leading `if (!val) return val` guard) and returns `""`. The metric-card then evaluates its formula against `data: ""` → normalized to `{}` → every `{$.values[...]}` token resolves to `undefined`/`[]` → mathjs renders `0`. *How to apply:* on aggregated metric cards, set `"dataPath": "$"` to pass the full payload, or `"$.data"` if the API response is wrapped (the `/api/v3/data/aggregate/{layoutId}` endpoint returns `{id, data: {values:[...]}, errors, ...}` — components receive the whole envelope, so `$.data` is what gets you to the inner `values`).
16. **The aggregate API response wrapper breaks `..` paths silently.** The data lookup (`tryGetValueFromData`) calls `getValueByPath`, and if the result is `undefined` AND `data?.data` exists, it retries against `data.data` — this is the rescue that lets existing charts work despite the wrapper. But the fallback only triggers on `undefined`, not on `[]`. `getValueByPath` uses `jsonpath.value` (returns `undefined` on no-match) when the path has no `..`, and `jsonpath.query` (returns `[]` on no-match) when the path contains `..`. Net effect: any path with `..` against the wrapped response gets `[]` (no fallback) → `:JSON` formatter stores `[]` → `sum([])` → `0`. *How to apply:* either set the component's `dataPath: "$.data"` so the formula's tokens resolve against the inner object, or rewrite tokens to include the `data.` prefix (`{$.data.values[...]..value:JSON}`). Pitfall #15 is the cleaner fix.
17. **`jsonpath` library doesn't support `&&`, `||`, or chained filters.** `[?(@.x == 'a' && @.y == 'b')]` returns empty silently. `[?A][?B]` does not act as filter-then-filter — the second filter is ignored. Single-condition filters with `==` and `!=` work; `..` works. Workarounds for "two conditions": (a) restructure with `..` between filters when the data shape allows; (b) exploit arithmetic redundancy — when the data contains an "All" aggregate row that equals the sum of per-key rows, you can isolate the aggregate value with `sum(per-key matches) / 2`; (c) filter in the per-row expression of a grouped component rather than in the `dataPath`. *Why it matters:* combined with `sum([])` returning `0`, this is a common reason an aggregated metric card silently renders `0` even though the underlying chart on the same dashboard works.
18. **Aggregated `grouped-metric-card` `dataPath` must resolve to an array.** The component calls `.map()` on the resolved data. If `dataPath` ends in a filter `[?(...)]` with no `..` earlier in the path, `getValueByPath` uses `jsonpath.value` and returns the first matching element only — a single object. The component then throws `.map is not a function`. *How to apply:* include `..` somewhere in the path (e.g. `$.data.values[?(@.group=='offer')]..metrics[?(@.labels.measure=='Sufficient offers')]`) to route through `jsonpath.query`, which always returns an array.

## Configuring Aggregated Reports (`reportData`)

When a dashboard runs in Aggregated mode (see [Dashboard Modes](#dashboard-modes-works-vs-aggregated)), each chart / metric card draws from its **own** server-aggregated dataset declared on the component as `reportData`. The aggregation engine takes the full set of works visible to the dashboard, runs the recipe in `reportData`, and returns the resulting rows as the chart's data.

Use Aggregated mode whenever:

- The chart needs to chart **values that don't exist on a work** — e.g. a per-cart-line `quantity` summed by `modelName`, where the cart lines are buried inside `work.data.cart[]`.
- The same chart needs both work-level and sub-array-level aggregations in one dataset.
- The data volume is large enough that client-side grouping over the full works array is too slow.

If the chart can be expressed as "group works by one of their fields, sum / count another", stay in Works mode — `reportData` adds operational complexity that isn't worth it for the simple cases.

### `reportData` Schema

`reportData` is an array of independent **field groups**. Each field group is one logical dataset the component can read by `name`. The schema:

```jsonc
"reportData": [
  {
    "id": "<guid, optional>",
    "name": "cartRowsByModel",                            // unique name within this component
    "condition": "",                                       // ⚠ NOT evaluated at the group level — put row filters in fields[].condition (see "Filtering rows with condition")
    "dataType": 2,                                         // hint for value typing — see enum below
    "expandByDataPath": "$.orderDetails.cart",             // optional — expand each work into N rows from this array
    "groupBy": [                                           // 0+ group-by axes
      {
        "sourcePath": "$.orderDetails.cart.model.displayName",  // FULL path from work root — see "Paths under expand" below
        "targetPath": "modelName",                              // key the chart will read via *Key
        "missingValue": "Unknown"                               // bucket label when sourcePath resolves to null
      }
    ],
    "fields": [                                            // 1+ measure columns
      {
        "dataPaths": ["$.orderDetails.cart.quantity"],     // FULL path from work root, not relative to the expanded row
        "operations": [0],                                 // see operations enum below
        "targetPath": "quantity",                          // column the chart will read via calculateStackKey, etc.
        "condition": null,                                 // optional row-level filter for this field
        "format": null,                                    // optional Luxon / Intl format string
        "useExpand": true,                                 // if true, operate over the expanded rows; if false, over the work
        "countNulls": false                                // for Count, treat null as a counted item
      }
    ],
    "calculations": [],                                    // 0+ derived columns computed after fields
    "properties": ["$.orderDetails.cart"]                  // declares MongoDB projection — see "What `properties` does" below
  }
]
```

#### `operations` enum (numeric on the wire)

Aggregate operations the engine accepts:

| Value | Operation | Meaning |
|---|---|---|
| 0 | `Sum` | Sum of the collected values (across expanded rows and works) — **not** a sum across the `dataPaths` entries; see [Multiple `dataPaths` — first-found coalesce](#multiple-datapaths--first-found-coalesce-schema-fallback) |
| 1 | `Average` | Mean |
| 2 | `Count` | Count items (with `countNulls: true` to include nulls) |
| 3 | `Max` | Maximum |
| 4 | `Min` | Minimum |
| 5 | `StdDev` | Standard deviation |
| 6 | `Median` | 50th percentile |
| 7 | `Mode` | Most common value |
| 8 | `Calculation` | Use the expression in `calculations[]` (only valid in `calculations`) |

The wire format is the integer (e.g. `"operations": [0]`), not the name.

#### `dataType` enum (numeric)

Hints how the engine should coerce values when the first row's type is ambiguous:

| Value | Type |
|---|---|
| 0 | `String` |
| 1 | `Decimal` |
| 2 | `Integer` |
| 3 | `Date` |
| 4 | `Boolean` |

Use `2` (Integer) for counts and integer sums, `1` (Decimal) for currency / floats, `3` (Date) for date bucketing.

#### `expandByDataPath` — flattening sub-arrays

When set (e.g. `"$.orderDetails.cart"`), the engine iterates the array and, for each element, **replaces the array in place** with that single element before evaluating `groupBy.sourcePath` and `fields[].dataPaths`. The root shape of the work data is **unchanged** — the array path now points at one record instead of the array.

**Paths under `expandByDataPath` must be FULL paths from the work root**, not relative to the expanded record. To read the current cart line's `quantity` you write `"$.orderDetails.cart.quantity"`, not `"$.quantity"`. To group by the line's model name you write `"$.orderDetails.cart.model.displayName"`, not `"$.model.displayName"`. The "relative" short form does not work — the engine looks up `data.SelectToken(path)` against the unchanged root, so a relative path resolves against the wrong scope and returns null, and the group then drops out of the response entirely (see pitfall #11).

Mix expanded and unexpanded measures by toggling `field.useExpand`: `true` (default) operates over expanded rows; `false` fires only once per work (the first iteration), so a `$.work` Count or a top-level scalar Sum stays per-work.

If `expandByDataPath` is omitted, every work contributes exactly one row.

Worked example — sum cart-line `quantity` grouped by t-shirt name:

```jsonc
{
  "name": "topShirtsByQuantity",
  "dataType": 2,
  "expandByDataPath": "$.orderDetails.cart",
  "groupBy": [
    {"sourcePath": "$.orderDetails.cart.model.displayName", "targetPath": "model", "missingValue": "Unknown"}
  ],
  "fields": [
    {"dataPaths": ["$.orderDetails.cart.quantity"], "operations": [0], "targetPath": "quantity", "useExpand": true}
  ],
  "properties": ["$.orderDetails.cart"]
}
```

#### What `properties` actually does

`properties` is the list of paths the engine adds to the MongoDB **projection** when fetching works. **Anything NOT listed here is not in the projected document** — `groupBy.sourcePath`, `fields[].dataPaths`, and `condition` tokens then resolve to `null`. The engine does not auto-derive this list from the rest of the report; you have to declare it.

**Per-dashboard union, but make each report self-sufficient.** The aggregator builds one MongoDB projection from the **union of every report's `properties`** on the dashboard. So if dashboard A's `revenueByMonth` report declares `$.paymentDetails.amountPaid` and dashboard A's `ordersByPaymentMethod` report declares only `[$.work]`, the union still includes `$.paymentDetails.amountPaid`. That sometimes "rescues" a report whose own `properties` is incomplete — and it's the most common source of silent bugs when reports are added, removed, or reordered. **Declare every path your own report uses in its own `properties` array** so the chart keeps working when other reports change.

**Rule of thumb — what to put in `properties`:**

| Report shape | Minimum `properties` |
|---|---|
| Count-of-works metric (`dataPaths: ["$.work"]`, `Count`, no `groupBy`) | `["$.work"]` |
| Sum/Avg on a top-level form field (`dataPaths: ["$.paymentDetails.amountPaid"]`) | `["$.paymentDetails.amountPaid"]` |
| Group-by a form field (`groupBy.sourcePath: "$.paymentDetails.paymentMethod"`) + Count of `$.work` | `["$.work", "$.paymentDetails.paymentMethod"]` |
| Conditional aggregate (`condition: "'{$.result}' == 'Delivered'"`) | …plus `"$.result"` |
| Expand over an array (`expandByDataPath: "$.orderDetails.cart"`, paths under it) | `["$.orderDetails.cart"]` — projecting the parent returns the whole sub-document including all nested fields, so you don't need to list inner cart fields |

**Symptom of a missing path:** the chart renders with one bucket only (every row collapses to `missingValue`, usually `"Unknown"`), and the legend shows just that one entry. If only the data-grouping is off, check `properties` before assuming the underlying data is wrong — it's almost always the projection.

#### The `$.work.X` namespace — what's accessible at the work level

`fields[].dataPaths`, `groupBy.sourcePath`, and any `condition` can also reach into a **synthetic work object** that the aggregation engine attaches to each work, addressed via the `$.work.X` prefix. This object exposes a **fixed, minimal set of work-level fields** — it is NOT the full work record:

| Path | What it returns |
|---|---|
| `$.work.id` | Work id (GUID) |
| `$.work.chains` | The work's chain map (each chain's events) |
| `$.work.workState`, `$.work.flowState` | Current step state |
| `$.work.workStateName` | String form of the state |
| `$.work.statusDisplay` | Status object — `displayName`, `color`, `bgColor`, `workType`, `icon`, `stateName`, `dateTimeIso`, `roleName`, `eventIndex`, `value` |
| `$.work.currentState` | Latest `currentState` chain event's `displayName` (falls back to `statusDisplay.displayName`) |
| `$.work.previousState` | Previous `currentState` chain event's `displayName` |
| `$.work.status` | Work state enum value |
| `$.work.createdDate` | Work creation date as a `{date, offset, ticks, timezone}` object. **`ticks` is normalized to Unix-epoch milliseconds** (not .NET ticks), so it can be subtracted directly from UI-entered date fields such as `$.delivery.actualDeliveryDate.ticks` in cycle-time expressions |

**Not in the namespace.** `$.work.created.date`, `$.work.displayName`, `$.work.lastModified`, `$.work.flowOriginId`, `$.work.workCode` — these resolve to `null`. Note that the **flat** `$.work.createdDate` *is* exposed (see the row above), but the **nested v3 form** `$.work.created.date` is not. Reports ported from v3 layouts that use `sourcePath: "$.work.created.date"` for "the work's created date" silently bucket every row into `missingValue` in v4 Aggregated mode — switch them to `$.work.createdDate`.

**Work-level dates.** Two dates are reliably exposed at the work level: `$.work.createdDate` (when the work was opened — `ticks` in Unix-ms; use this for order-to-delivery / cycle-time metrics) and `$.work.statusDisplay.dateTimeIso` (an ISO 8601 timestamp of when the work's current status was set). For a `total-by-period-stacked-bar-chart` with `period: "Month"`, `statusDisplay.dateTimeIso` buckets works by the month they entered the matching status (e.g. month the invoice was Approved).

**Condition expressions** (e.g. `condition: "'{$.work.statusDisplay.displayName}' == 'Approved'"`) interpolate `{$.path}` tokens against the same merged context: `$.work.X` paths against the synthetic object, all other paths against the (expanded) row. Wrap each interpolated token and each literal in single quotes; comparison is case-sensitive string equality. The condition is evaluated per row before the operation runs — if it returns false the value is skipped.

#### What the aggregator can actually see — projected data, not full works

The Aggregated engine does **not** load each work in full. It builds a MongoDB projection from the union of all `groupBy.sourcePath`, `fields[].dataPaths`, and `expandByDataPath` declared on the dashboard, then fetches only those fields. That has two consequences worth designing around:

**1. Paths into referenced assets/contracts/POs don't resolve.** Fields like `invoice.contract`, `invoice.purchaseOrder`, `invoice.asset` are stored on the work as *shallow references* — typically just `{ id, displayName, imageUrl }` plus a thin `data` envelope. The fully-hydrated asset payload (e.g. `contract.data.contractAsset.contractor.displayName`) is materialised on demand by the work API but is **not** in the work BSON the aggregator scans. So `sourcePath: "$.invoice.contract.data.contractAsset.contractor.displayName"` falls through to `missingValue` for every row, and "by vendor" charts label everything `Unknown`.

| What you want | Use | Don't use |
|---|---|---|
| Contract name | `$.invoice.contract.displayName` | `$.invoice.contract.data.displayName` |
| Asset name | `$.invoice.asset.displayName` | `$.invoice.asset.data.*` |
| PO number | `$.invoice.poNumber` (if persisted on the work) | `$.invoice.purchaseOrder.data.*` |
| Vendor | A field persisted on the work at create time (e.g. `$.invoice.vendorName`) | `$.invoice.contract.data.contractAsset.contractor.displayName` |

**Rule of thumb:** if `get_work` shows a value at the path but you didn't write it directly to `work.data.*` in a flow step, assume the aggregator can't see it. If you need that value in a chart, either denormalize it onto the work at the step that creates the work, or chart something else.

**2. Pre-computed scalars beat `expandByDataPath` + Sum.** v4 work forms often persist summary totals alongside arrays (e.g. `invoice.services_Total` next to `invoice.services[]`). Aggregating the scalar with no expand is more reliable than expanding the array and summing a nested field — see the next pitfall on expand groups silently dropping out of the response. Prefer:

```jsonc
{
  "expandByDataPath": null,
  "fields": [{
    "dataPaths": ["$.invoice.services_Total"],
    "operations": [0],        // Sum across works
    "targetPath": "total",
    "useExpand": false
  }]
}
```

over `expandByDataPath: "$.invoice.services"` + Sum of `$.invoice.services.total`, whenever the scalar exists.

#### `fields[].dataPaths` accepts `dynamicValue` formulas (per-row math)

Each entry in `fields[].dataPaths` may be either a plain path (`"$.orderDetails.cart.quantity"`, `"$.order.product.premium"`) **or** a `=`-prefixed `dynamicValue` formula evaluated **per (expanded) row before the `operations` are applied**. This is the canonical way to chart a derived measure — line revenue, weighted average, gross margin — without pre-computing it on the work.

```jsonc
"fields": [
  {
    "dataPaths": ["={$.orderDetails.cart.quantity} * {$.orderDetails.cart.unitPrice}"],   // formula tokens use FULL paths
    "operations": [0],                                 // Sum across all cart lines
    "targetPath": "revenue",
    "useExpand": true                                  // operate over expanded cart rows
  }
]
```

How it works: with `expandByDataPath: "$.orderDetails.cart"` and `useExpand: true`, the engine iterates the array — for each cart line it replaces `$.orderDetails.cart` with that single line, then evaluates the formula. `{$.orderDetails.cart.quantity}` reads the current line's `quantity`, `{$.orderDetails.cart.unitPrice}` reads its `unitPrice`. The product is one number per row; `operations[0]: Sum` totals those into the bucket. The chart reads `targetPath: "revenue"` via `calculateStackKey: "revenue"`.

When to reach for this:

- The required measure is the product / sum / ratio of two persisted fields and isn't itself stored on the row. Computing it inside `dataPaths` is preferred over a pre-aggregation `Calculation` because it avoids materialising an intermediate field.
- The data-grid that captures the measure uses a render-only computed column (`column.value`) — those values are **not** persisted to `work.data.cart[i]`, so a downstream aggregation can't read them. Recomputing via a `dataPaths` formula is the canonical fix. (See `references/layouts-and-components.md` "Computed Columns" for why.)

Example: a formula such as `"={$.order.product.quantityAvg} * {$.order.product.premium}"` can be used to chart per-row premium volume without relying on a separately persisted intermediate field.

`calculations[]` (op `8`, see below) is for formulas that combine **already-aggregated** `targetPath` values (e.g. `share = data.quantity / data.totalQuantity`); use `dataPaths` formulas for **per-row** math that should then be summed/averaged.

#### Multiple `dataPaths` — first-found coalesce (schema fallback)

`fields[].dataPaths` is a **list**, and the engine treats the entries as **ordered alternatives, not as terms to add together**. For each (expanded) row it walks the list and uses the **first path that resolves to a usable value**, then stops — a first-non-null *coalesce*. The chosen value is what `operations` (Sum / Average / …) then aggregate **across rows and works**. Listing two paths does **not** sum them; to sum two persisted fields per row, use a single `=`-prefixed formula entry instead (`"={$.a} + {$.b}"`).

This is the supported way to chart **one measure that different works store under different paths** — e.g. a flow whose schema changed between versions, or whose value is captured as a per-line array on some works and a scalar on others:

```jsonc
"fields": [
  {
    "dataPaths": [
      "$.deliveryDocuments.meterReadings.deliveredVolumeLiters",  // works using the array shape (with expandByDataPath)
      "$.deliveryDocuments.deliveredVolumeL"                       // works using the scalar shape
    ],
    "operations": [0],
    "targetPath": "total",
    "useExpand": true
  }
]
```

A work matching *either* shape contributes to the same `Sum`. Notes and caveats:

- **Declare every alternative in `properties`.** A path not in the projection resolves to `null` (see [What `properties` actually does](#what-properties-actually-does)), so the coalesce never reaches it.
- **Expand interaction.** `expandByDataPath` is a single, group-level array path. Works that have that array expand into N rows (the first listed path resolves per row); works that lack it yield one un-expanded row (the first path misses and the scalar fallback resolves once). Both feed the same `Sum`, which is correct as long as each alternative represents the same per-work quantity.
- **The "Data path not found" error still appears — but the value is correct.** Each path that misses logs a per-work `"Data path '<path>' not found in data."` into the response's error map **before** falling through to the next path. So on works that don't carry the first-listed path you will still see that error string even though the metric computes correctly from a later path. The value is right; the error is noise.
- **To avoid the noise**, gate by schema with `fields[].condition` instead of coalescing: a field whose `condition` is false is skipped **silently** (no error logged). Declare two fields with the **same** `targetPath` (one per schema, each with the matching `useExpand`) — they **do** merge, because collected values accumulate by `targetPath` before the operation runs. This is more verbose and needs a reliable schema-presence condition, so prefer multi-`dataPaths` unless the error noise is a problem.

#### `groupBy.targetPath` must match the chart's `*Key`

The `targetPath` you set in `groupBy[].targetPath` is the field name on each output row that the chart reads via its `*Key` properties. Conventionally `"data.<fieldName>"`. So for a bar chart:

- `groupBy: [{ targetPath: "data.modelName", ... }]` ↔ `properties.groupSeriesByKey: "data.modelName"`.
- `fields: [{ targetPath: "data.quantity", ... }]` ↔ `properties.calculateStackKey: "data.quantity"`.

Mismatched paths render every bar as "Unknown" or `0`.

### Worked Example — `total-by-bar-chart` summing per-cart-line quantities by model

Setup: each work has a `orderDetails.cart[]` with `{ model: <asset-link>, quantity: <int> }` entries (the form section's `dataPath` is `$.orderDetails`, with a data-grid `cart` child). Goal: a horizontal bar chart of total units sold per T-shirt model.

```jsonc
{
  "component": "total-by-bar-chart",
  "label": "Top T-shirt Models",
  "name": "topTshirtModels",
  "dataPath": "$.data.topTshirtModels",     // matches the report `name` under `data`
  "properties": {
    "title": "Top T-shirt Models",
    "showTitle": true,
    "width": "col-6",
    "horizontal": true,
    "hasLabelsAtTheEndOfDataBars": true,
    "decimals": 0,
    "unitOfMeasure": "units",
    "groupSeriesByKey": "model",            // matches groupBy.targetPath below
    "calculateStackKey": "quantity"         // aggregated branch reads *Stack*, NOT *Series*
  },
  "reportData": [
    {
      "name": "topTshirtModels",
      "dataType": 2,                                          // Integer
      "expandByDataPath": "$.orderDetails.cart",              // one row per cart line
      "groupBy": [
        {
          "sourcePath": "$.orderDetails.cart.model.displayName", // FULL path from work root
          "targetPath": "model",
          "missingValue": "Unknown"
        }
      ],
      "fields": [
        {
          "dataPaths": ["$.orderDetails.cart.quantity"],         // FULL path from work root
          "operations": [0],                                     // Sum
          "targetPath": "quantity",
          "useExpand": true,
          "countNulls": false
        }
      ],
      "calculations": [],
      "properties": ["$.orderDetails.cart"]                      // projects the cart array (sub-fields included)
    }
  ]
}
```

Why this renders correctly:

1. The dashboard layout's `dataSource.name: "Aggregate"` routes the chart through `TotalByBarChartWithAggregations`.
2. `expandByDataPath: "data.cart"` flattens each work into N cart-line rows.
3. `groupBy` buckets those rows by `$.orderDetails.cart.model.displayName` (the FULL path — after expand the cart array is replaced by the current line, so the path still walks from the work root), exposing the bucket label as `model` (`missingValue: "Unknown"` catches lines whose model is missing).
4. `fields[0]` sums each row's `$.orderDetails.cart.quantity` into `quantity`, with `useExpand: true` so the sum operates over cart lines (not the parent work).
5. Chart properties `groupSeriesByKey` (`"model"`) and `calculateStackKey` (`"quantity"`) read those exact target paths.

### Counting works instead of summing a measure

To get a per-bucket **count of works** (not a sum), use `Count` operation (`2`) and pick any always-present field as `dataPaths`:

```jsonc
"fields": [
  {
    "dataPaths": ["id"],
    "operations": [2],          // Count
    "targetPath": "data.count",
    "useExpand": false,         // count works, not expanded rows
    "countNulls": false
  }
]
```

The chart then uses `"calculateStackKey": "data.count"`. Use `useExpand: true` to count expanded sub-rows instead.

### Filtering rows with `condition`

**Put row filters on `fields[].condition`, not on the field-group-level `condition`.** The aggregator evaluates **only** the per-field `fields[].condition`. The group-level `condition` (the one next to `name` / `groupBy` / `fields`) is currently **not applied** — it is silently ignored, so every row is counted/summed regardless of its value. A filter placed there is a no-op. The classic symptom: a status/category chart shows *all* works instead of the filtered subset, and two charts with different group-level conditions render identical numbers.

To filter a whole field-group by one condition, **repeat that condition on every `fields[]` entry**. (To scope which works reach the report at all, use the dashboard-level `defaultQueryFilters` instead — see above.)

`fields[].condition` accepts a `dynamicValue`-style expression evaluated against each row. Tokens use **full paths from the work root** (the same rule as `groupBy.sourcePath` / `fields[].dataPaths` — see [`expandByDataPath` — flattening sub-arrays](#expandbydatapath--flattening-sub-arrays)). Wrap each interpolated token **and** each literal in single quotes; comparison is case-sensitive string equality. Every path the condition references must also be declared in `properties` (or be a `$.work.X` namespace path), or it projects to `null` and the comparison fails for every row. Examples:

```jsonc
// Count only works currently in the "Order Rejected" status:
{
  "dataPaths": ["$.work"],
  "operations": [2],                                   // Count
  "condition": "'{$.work.statusDisplay.displayName}' == 'Order Rejected'",
  "targetPath": "total",
  "useExpand": false
}

// This particular field counts only Won outcomes:
{
  "dataPaths": ["id"],
  "operations": [2],
  "condition": "'{$.offer.outcomeType}' == 'Won'",
  "targetPath": "data.totalWon",
  "useExpand": false
}
```

### `calculations[]` — derived columns

Use when a measure depends on multiple already-aggregated values. Each `calculations[]` entry has the same shape as a `fields[]` entry plus a `calculation` field — a `dynamicValue` formula referencing earlier `targetPath`s within the same group:

```jsonc
"calculations": [
  {
    "calculation": "={$.data.quantity} / {$.data.totalQuantity}",
    "operations": [8],          // Calculation
    "targetPath": "data.share"
  }
]
```

Calculations run after `fields` so they can reference `targetPath`s defined earlier in the same field group.

### Multiple Datasets per Component

A component can declare more than one field group. Different chart properties read different datasets by name (e.g. `total.value: "$.totals[0].volumeTotal"` reads from a `totals` field group while the chart's main bars read from a `byModel` field group). This pattern is most common on `grouped-metric-card` where the main `value` and the `total.value` come from different aggregation recipes.

### Aggregated-Mode Pitfalls

1. **Authoring `reportData` without setting the layout's `dataSource`.** `reportData` is dead config in Works mode — the chart receives raw `IWork[]` and the report is silently ignored. Set `data_source` via `create_dashboard` or `update_dashboard_metadata` to `{"name": "Aggregate", "properties": {}}` before authoring `reportData`. See [Setting the dashboard `dataSource`](#setting-the-dashboard-datasource).
2. **Property name confusion across charts.** Per the table in [Dashboard Modes](#dashboard-modes-works-vs-aggregated): `total-by-bar-chart` aggregated reads `calculateStackKey`, not `calculateSeriesKey`. Aggregated `total-by-stacked-bar-chart` also reads `calculateStackKey`; **`total-by-period-stacked-bar-chart` is the exception — its aggregated path reads `calculateSeriesKey`** (period render hook), so set that one's value via `calculateSeriesKey`. Aggregated pie has a fallback so either name works there. Default to `calculateStackKey` on aggregated bar/stacked charts, but use `calculateSeriesKey` on the period chart.
3. **`useExpand: false` on a Sum/Average over an expanded array undercounts.** When `expandByDataPath` is set, the engine yields one row per expanded element. A field with `useExpand: false` fires **only on the first** row per work — so Sum picks up just `<expandPath>[0].<field>` per work and silently ignores every other element (often rendering `0` when the first element's path doesn't carry a numeric value). To aggregate across **all** expanded rows set `useExpand: true` (or omit it — `null` defaults to processing every iteration). v3-imported reports often have explicit `useExpand: false` on Sum fields; flip them when porting to v4. The inverse case — `useExpand: true` on a group with no `expandByDataPath` — is also broken: it operates over an empty per-work expansion and returns 0. Match `useExpand` to whether `expandByDataPath` is set on the same group.
4. **Forgetting `missingValue`.** Without `missingValue`, rows whose `sourcePath` resolves to null are dropped from the chart entirely (not bucketed). Set `missingValue: "Unknown"` if dropping silent rows would understate totals.
5. **Wire format is integers, not strings.** `operations: ["sum"]` and `dataType: "Integer"` are rejected — use `operations: [0]`, `dataType: 2`. The OpenAPI generator surfaces these as `NUMBER_0..NUMBER_8` to make this visible at the type level.
6. **Mixing `*Path` and `*Key` conventions.** `reportData.groupBy[].sourcePath` is a JSONPath-style expression (`"model.data.displayName"` — leading `$.` accepted but not required), but `properties.groupSeriesByKey` is a lodash dotted path (no `$.`). Don't paste paths between them — what works in one shape silently fails in the other.
7. **`calculations[].operations` must be `[8]`.** Calculations use the `Calculation` operation (`8`) and the actual logic comes from `calculation`. Setting `operations: [0]` (Sum) on a calculation row makes the engine try to sum the formula instead of evaluating it.
8. **Client-side mathjs against `{$.works:JSON}` is unreliable in Aggregated mode.** In Aggregated mode the server returns only `data.<reportName>` projections — the `{$.works:JSON}` array isn't always present in the payload `dynamicValue` runs against. A `metric-card` with `value: "=count({$.works:JSON})"` or `value: "=countOfDeepValues({$.works:JSON}, '$.data.x..y', true)"` on an Aggregated dashboard typically renders `0`. For every count / sum / deep-array measure in Aggregated mode, author it as a server-side `reportData` field group and read the result via `dataPath: "$.data.<reportName>[0]"` with `value: "$.<targetPath>"`.
9. **Sourcing dates from work-level fields.** The flat `$.work.createdDate` *does* resolve — it's the work's creation date with `ticks` in **Unix-ms** (use it for cycle-time math like `({$.delivery.actualDeliveryDate.ticks} - {$.work.createdDate.ticks}) / 86400000.0`). But the nested v3 form `$.work.created.date` does **not** resolve — it isn't on the synthetic work surface (see [The `$.work.X` namespace](#the-workx-namespace--whats-accessible-at-the-work-level)), so reports ported from v3 silently bucket every row into `missingValue`. For "when the work entered its current status" (what most "by month" / "by period" charts actually want) use `$.work.statusDisplay.dateTimeIso`.
10. **Asset / contract / PO reference paths resolve to `Unknown`.** Paths that walk into a referenced asset's deep payload — `$.invoice.contract.data.contractAsset.contractor.displayName`, `$.invoice.purchaseOrder.data.*`, `$.invoice.asset.data.*` — are not in the aggregator's MongoDB projection (only the shallow reference is on the work). Every row's groupBy bucket falls through to `missingValue`. Group by a value that lives directly on the work (e.g. `$.invoice.contract.displayName` for the contract's own name, or a denormalized field like `$.invoice.vendorName` if the flow step writes one). See [What the aggregator can actually see](#what-the-aggregator-can-actually-see--projected-data-not-full-works).
11. **Expand groups disappear from the response when no values are added.** A field group with `expandByDataPath` set whose `fields[].dataPaths` returns nothing for every work (or whose expand path resolves to empty/missing across all works) is omitted from the response entirely — the key under `data.<reportName>` is absent, not present with `total: 0`. A `metric-card` reading `dataPath: "$.data.<name>[0]"` then resolves to `undefined`, defaults to `0`, and renders "0" with no error. The chart shows "No data".

    **The #1 cause of this is using a path that's relative to the cart line instead of full from the work root.** `dataPaths: ["$.quantity"]` next to `expandByDataPath: "$.orderDetails.cart"` resolves to null on every iteration, no values are added, the key drops. The correct form is `dataPaths: ["$.orderDetails.cart.quantity"]` — see [`expandByDataPath` — flattening sub-arrays](#expandbydatapath--flattening-sub-arrays). If your chart renders "No data" but the engine clearly processed works (other reports on the same dashboard return data), this is almost always the path-shape error. To confirm without server logs, temporarily switch `expandByDataPath` to a known-good top-level path and verify the report key appears; then fix the inner paths to use the full form.

    **Other causes:** the expand array key is genuinely empty across every work (in which case the engine falls back to one bucket per work but with no field values — same "no values added" outcome), or the work data uses a different schema than the report expects (see pitfall #12). Fix: either populate the array on at least one work, aggregate a pre-computed scalar with no expand (`$.invoice.services_Total` rather than expand-and-sum `$.invoice.services.total`), or align the paths to the actual stored shape.
12. **Layout changes leave historical works on the old schema.** Form fields that move into a new section (e.g. `cart` → `orderDetails.cart` when a section is added in a later flow version) persist their old paths on works created against the older layout. A report using the new path (`$.orderDetails.cart`) finds nothing on the legacy works, while a report using the old path (`$.cart`) finds nothing on the new ones. There's no auto-migration. Options: (a) accept the partial coverage and acknowledge the report only reflects new orders, (b) **list both paths in one field's `dataPaths`** — the engine uses the first that resolves per work (first-found coalesce), so works on either schema feed the same metric; add every alternative to `properties` and expect the harmless per-work "Data path not found" log for works that miss the first-listed path (see [Multiple `dataPaths` — first-found coalesce](#multiple-datapaths--first-found-coalesce-schema-fallback)), or (c) backfill the historical works server-side to the new schema. When you author a report and existing works don't show up, sanity-check by fetching one historical work and comparing its top-level keys against the form's current section layout.
13. **A no-`groupBy` KPI can show a total while a date-bucketed period chart over the *same* measure shows nothing (or far less).** A `metric-card` with no `groupBy` sums the measure across every matching work; a `total-by-period-stacked-bar-chart` additionally requires each contributing work to carry a non-empty bucket-date at `groupBy.sourcePath` (e.g. `$.delivery.actualDeliveryDate.date`). When the works that hold the measure are a **different set** than the works that populate the date — common when the measure lives in an expanded array (`$.deliveryDocuments.meterReadings.deliveredVolumeLiters`) on some works while the delivery-date field is only filled on others — the period chart drops every dateless row, so the KPI reports a non-zero total while the time-series is empty or undercounts. The two tiles disagreeing is the tell. Before trusting a "by month / period" chart: fetch one contributing work and confirm **both** the bucket-date path **and** the measure path are populated on the *same* work. If they're structurally disjoint, either denormalise a single delivery-date onto the works that carry the volume, or bucket by a date that is always present (`$.work.statusDisplay.dateTimeIso`). Also keep the period chart's measure identical to the KPI's (same `dataPaths` + `expandByDataPath`) so their totals reconcile — sourcing one from `meterReadings[].deliveredVolumeLiters` and the other from a sibling scalar like `delivery.postDelivery.meterReading` will make them disagree even when both render.

### Authoring via MCP Tools

To work with `reportData` and aggregated dashboards through the MCP layer:

- **Read it:** `get_dashboard(id)` returns `dataSource` at the dashboard root and the components include any `reportData` blocks.
- **Add a tile with `reportData`:** include the full component object (with `reportData` inside) in the `components` list for `add_dashboard_components(draft_id, components)`. The MCP server passes the payload through verbatim, so any field name accepted by the dashboard layout API is accepted here.
- **Edit `reportData` on an existing tile:** `update_dashboard_component(draft_id, component_id, component_data)` is PATCH-style; pass `{"reportData": [...]}` and only that block is replaced. Pass an empty array to clear it.
- **Set or change the layout's `dataSource`:** pass `data_source` to `create_dashboard` (on new dashboards) or `update_dashboard_metadata` (on existing ones). Shape and dispatch semantics are documented in [Setting the dashboard `dataSource`](#setting-the-dashboard-datasource).

## Lifecycle & API Pitfalls

1. **Don't mix tools.** Flow-layout `add_components` / `update_component` / `replace_section_components` / `delete_components` do NOT work on dashboards. Use the `dashboard_`-prefixed tools.
2. **Draft id, not published id.** All component / reset / publish tools take `draftId`. Only `get_dashboard_draft_layout`, `get_dashboard`, `update_dashboard_metadata`, and `delete_dashboard` take the published id.
3. **`replace_dashboard_section_components` is destructive** within the target section — omitted components are deleted. Pass the full list of children you want to keep. Cannot include section-type components.
4. **Non-section needs parentId.** Same rule as flow layouts — supply `parent_section_id` or include a section in the same `add_dashboard_components` batch.
5. **Metadata ≠ layout.** `update_dashboard_metadata` hits the published dashboard directly with no draft — use it for name / icon / filters / permissions / users. It does NOT touch components.
6. **Partial `add_dashboard_components` failure.** If a batch errors mid-add some components may have been created. Call `get_dashboard_draft_layout` again to re-read components before retrying; delete unwanted duplicates with `delete_dashboard_components`.
