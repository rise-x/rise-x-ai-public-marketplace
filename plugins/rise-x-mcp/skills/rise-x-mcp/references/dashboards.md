# Performance Dashboards

Performance dashboards are standalone report pages built from sections and components, like flow layouts, but with no flow, steps, or work items behind the page itself. They have their **own** draft/publish lifecycle and their **own** component tools. The flow-layout tools (`add_components`, `update_component`, `replace_section_components`, `delete_components`) do not work on dashboards; use the `*dashboard*` tools.

Load the other two dashboard files only when the task needs them:

- `references/dashboards-aggregated.md`: load when the dashboard is or becomes aggregated. That is: `data_source` is set, a component carries `reportData`, a chart must sum values inside a sub-array of each work, or you use `total-by-clustered-bar-chart`. It holds the `dataSource` setup, which property each chart reads in Aggregated mode, the `reportData` recipe, and the aggregated pitfalls.
- `references/dashboards-charts.md`: load when you configure a chart or metric card beyond `label` and `width`, or when a tile renders blank, `NO DATA`, `0`, `Unknown`, or the word `undefined`. It holds the `dataPath` and `*Key` rules, the per-type gotchas, and the chart pitfalls.

For expression syntax inside `value`, `total.value`, and `tooltipFields[].value`, see `references/dynamicValue.md`.

## Draft Lifecycle (different from flows)

A dashboard has a published version and at most one draft.

- **Metadata** (name, icon, filters, `columns_data`, permissions, users, `data_source`): `update_dashboard_metadata(id, ...)` writes the published dashboard directly. No draft, no publish.
- **Components**: `get_dashboard_draft_layout(published_id)` returns a `draftId` (get-or-create, safe to call again). Edit with the component tools, then `publish_dashboard_draft_layout(draftId)`. Viewers see nothing until you publish. `reset_dashboard_draft_layout(draftId)` discards the staged edits and keeps the same `draftId` usable.
- **New dashboards**: `create_dashboard(components=[...])` and `duplicate_dashboard` publish at once, so they need no draft.

**Which id goes where**, the most common failure:

| Tool | Takes |
|---|---|
| `get_dashboard`, `get_dashboard_draft_layout`, `update_dashboard_metadata`, `duplicate_dashboard`, `delete_dashboard` | **published** dashboard id |
| `add_dashboard_components`, `update_dashboard_component`, `replace_dashboard_section_components`, `delete_dashboard_components`, `reset_dashboard_draft_layout`, `publish_dashboard_draft_layout` | `draftId` |

## Common build order

1. Find the source flow's `originId` (`search_flows` or `list_flows`). Every Works-mode dashboard tied to a workflow needs it in `defaultQueryFilters`.
2. If you need quick filters or explicit grid columns, read the flow's column keys with `manage_columns(flow_id, "list")`. To pick chart `*Key` paths, read one real work with `get_work` and note where the field sits (usually `work.data.<field>`).
3. Pick the mode (see [Dashboard Modes](#dashboard-modes-works-vs-aggregated)).
4. Call `create_dashboard` with `default_query_filters`, optional `quick_filter_property`, `data_source` only for Aggregated mode, and the seed `components`.
5. Read the `warnings` in the response, then open the dashboard and check that each tile shows data. A tile with `NO DATA`, `0`, or `Unknown` means a path problem: load `references/dashboards-charts.md`.
6. For later edits, use the draft cycle in [Draft Lifecycle](#draft-lifecycle-different-from-flows).

## Worked Example — Seed components at creation

```
create_dashboard(
    display_name="Ops Overview",
    default_query_filters={"flowOriginId": ["<source flow originId>"]},
    components=[
        {"component": "section", "label": "Overview", "properties": {"useContainer": True}},
        {"component": "metric-card", "label": "Total Orders",
         "properties": {"value": "=count({$.works:JSON})"}},
        {"component": "data-grid", "label": "Recent Orders"},
    ],
)
```

The tiles land under the section, get default widths, and the `data-grid` gets columns from the flow (see [Configuring data-grid columns](#configuring-data-grid-columns)). The API rejects the whole request if any component type is outside the [supported list](#supported-dashboard-component-types).

## Worked Example — Edit via draft

```
draft_id = get_dashboard_draft_layout(dashboard_id="<published id>").draftId
add_dashboard_components(draft_id=draft_id, parent_section_id="<section id>",
    components=[{"component": "metric-card", "label": "Open Orders",
                 "properties": {"value": "=count({$.works:JSON})"}}])
update_dashboard_component(draft_id=draft_id, component_id="<tile id>",
    component_data={"label": "Orders In Flight"})
publish_dashboard_draft_layout(draft_id=draft_id)
```

## Supported Dashboard Component Types

Dashboards accept only these types; flow-layout inputs such as `input-text` are rejected.

- Containers: `section`, `container`.
- Metric cards: `metric-card`, `grouped-metric-card`, `rating-metric-card`, `savings-metric-card`.
- Charts: `total-by-bar-chart`, `total-by-clustered-bar-chart`, `total-by-stacked-bar-chart`, `total-by-pie-chart`, `total-by-period-stacked-bar-chart`, `multiple-rating-chart`, `heatmap-chart`.
- Tabular: `data-grid`.

## Component Hierarchy (same as flow layouts)

```
Dashboard
  -> Section (parentId = "00000000-0000-0000-0000-000000000000")
     -> Components (parentId = section id)
     -> Container (parentId = section id)       [optional]
        -> Components (parentId = container id)
```

`add_dashboard_components` sets each non-section component's `parentId` in this order:

1. The call-level `parent_section_id` argument, if set, **overwrites** every non-section component's `parentId`, including one you set yourself.
2. Otherwise a component's own `parentId` field is kept.
3. Otherwise the component goes under the first `section` in the batch.

`parent_section_id` is only a tool argument. Inside a component dict the field is `parentId`; a `parent_section_id` key there has no effect. To put tiles under a container created in the same batch, generate the container's `id` yourself and set it as each child's `parentId` (see [`container`](#container)).

## Dashboard Modes (Works vs. Aggregated)

The mode is set once per dashboard by its `dataSource`, not per tile.

- **Works** (default; omit `data_source`): every tile gets the dashboard's filtered works array and computes in the browser. Use it when each chart groups works by a field on the work and counts them or sums another field on the work.
- **Aggregated** (`data_source={"name": "Aggregate", "properties": {}}`): every tile gets the rows produced server-side by its own `reportData`. Use it when the measure lives in a sub-array of each work (for example `work.data.cart[]`), when one chart needs work-level and sub-array measures together, when the work count is large, or for `total-by-clustered-bar-chart`, which renders only in this mode.

Switching mode is half the change. In Aggregated mode a tile without `reportData` gets no data, several charts read a different value property, and `heatmap-chart` has no aggregated path. Load `references/dashboards-aggregated.md` before you build or convert an aggregated dashboard.

## `defaultQueryFilters` — scoping the works array

A filter applied server-side to the works every tile receives. The viewer does not see it. Unknown keys are accepted and ignored, so a wrong key gives `NO DATA` with no error.

> **A workflow dashboard with no recognized filter is non-functional.** The works fetch needs at least one scoping key. With an empty or unrecognized filter, the fetch is rejected, `{$.works:JSON}` stays empty, and every tile shows `0` or `NO DATA`. Always set `flowOriginId` for a dashboard tied to one workflow.

### Recognized keys

| Key | Value |
|---|---|
| `flowOriginId` | `string[]` of flow origin GUIDs |
| `currentState` | `string[]` of the values in the following table |
| `displayName` | `string[]`, exact match on `work.displayName` |

A key that matches a flow property the flow declares as filterable also works. Use the property `key` from `manage_columns(flow_id, "list")`, not a data value-path. Other keys (`state`, `statusLabel`, `activeStepId`, `assignedTo`) are dropped; use `quickFilterProperty` for viewer-facing filters. Date values accept tokens that expand at render time: `"now"`, `"now-1d"`, `"now-7d"`, `"now-30d"`.

```jsonc
{"flowOriginId": ["<flow-guid>"], "currentState": ["Completed"]}
```

### `currentState` values

| Value | Use for |
|---|---|
| `"Open"` | Work on a non-terminal step: in flight, drafts, waiting. |
| `"Completed"` | Work that reached a terminal step. |
| `"Closed"` | Closed by a lifecycle operation. Rare. |
| `"Deleted"` | Soft-deleted. Rare. |
| `"Ok"` | Sync-process flag. Almost never used. |

Do not use `"InProgress"`, `"Complete"`, `"New"`, `"Draft"`, `"Cancelled"`, or values from `work.state` / `work.statusLabel`; they come from other enums and give `NO DATA`. To check a value, call `get_work(workId)` and copy `workStateName` verbatim.

## `quickFilterProperty` — user-facing pre-populated filters

The filter chips shown, and pre-selected, in the report toolbar. Shape: `{"keys": {"<flowPropertyKey>": ["<value>", ...]}}`.

> **The key must be the flow's filterable property `key`, not a data value-path.** A chip renders only when its key matches a flow property `key`. Any other key is accepted with no error and the report opens with no chips. This is the most common reason an MCP-built report loses its filters.

Get the key from `manage_columns(flow_id, "list")`, where `flow_id` is your `flowOriginId`. Each column has `key` (use this, for example `vendor`), `displayName` (label only), and `valuePaths` (data paths only). The key often does not resemble the label.

```jsonc
{"keys": {"vendor": ["Marinoil", "Audentis"]}}                          // right
{"keys": {"deliveryRequest.vendor.displayName": ["Marinoil"]}}          // wrong: no chip
```

An empty array (`{"vendor": []}`) shows the chip with nothing pre-selected. Values must be the property's option values. After setting quick filters, re-read with `get_dashboard(id)` and confirm each key is a column `key`.

## Layout Rendering & Widths

### Width values

`properties.width` is `"col-1"` to `"col-12"`, the span at the largest breakpoint. The API accepts one `col-N` token only, so compound widths such as `"col-4 col-md-6"` fail validation. Use `col-6` for two-up rows, `col-4` for three-up, `col-3` for four-up, and `col-12` for a full row.

### Responsive behavior — what the grid actually emits

The renderer derives five breakpoints (`xs` to `xl`) from the one `col-N`; you cannot set them yourself.

- Every tile spans 12 at `xs` (phones).
- `total-by-bar-chart`, `total-by-stacked-bar-chart`, `total-by-pie-chart`, `total-by-period-stacked-bar-chart`, and `multiple-rating-chart` span 6 at `md` and `lg` even when set to `col-12`, so two charts share a row there.
- Metric cards, `heatmap-chart`, and `data-grid` keep their `col-N` at `lg` and span at least 6 at `sm` and `md`.

For `col-4`: a chart spans 12/12/6/6/4 across `xs`/`sm`/`md`/`lg`/`xl`; a metric card spans 12/6/6/4/4.

### Putting it together — when tiles actually sit side-by-side

Tiles sit side by side only when both hold:

1. The parent emits a grid: the section has `useContainer: true`, or the tiles sit in a `container` (its `wrapWithGrid` defaults to `true`).
2. Each tile's `width` is under `col-12`.

## Configuring Containers (Section & Container)

### `section`

Properties: `get_schema("components", "section")`. Beyond the schema:

- `collapsible` is case-sensitive: `"None"` (default, no chevron), `"Collapsed"` (chevron, starts closed), `"Expanded"` (chevron, starts open). A lowercase value falls back to `"None"` silently.
- Set `useContainer: true` whenever the section holds two or more tiles that should share a row.
- `title` with `showTitle: true` renders a heading above the section.

### `container`

An optional grouping inside a section, for when one section holds two or more sub-groups that each need their own grid row. If you cannot name the sub-group it creates, skip it. Its `parentId` must be a section or another container; the null GUID is for sections only. `wrapWithGrid` (default `true`) wraps its children in a grid; set `false` only when the parent already supplies the grid. `get_schema("components", "container")` does not list `wrapWithGrid`.

Add a container and its children in one call by generating the container id yourself. Do not pass `parent_section_id` here, or it overwrites the children's `parentId`:

```python
container_id = str(uuid.uuid4())
add_dashboard_components(draft_id, components=[
    {"component": "section", "label": "Operations", "properties": {"useContainer": True}},
    {"component": "metric-card", "label": "In Transit",
     "properties": {"value": "=count({$.works:JSON})"}},
    {"id": container_id, "component": "container", "label": "Throughput"},
    {"component": "metric-card", "label": "Delivered", "parentId": container_id,
     "properties": {"value": "=count({$.works:JSON})"}},
])
```

The two-call form also works: add the section, tiles, and container first, then add the container's children with `parent_section_id=<container id>`.

### Container Pitfalls

1. **`collapsible` strings are case-sensitive.** `"collapsed"` falls through to `"None"` and the chevron never appears.
2. **`useContainer: false` + `width: "col-6"` looks broken.** With no grid wrapper, widths cannot compose and tiles stack full width. Set `useContainer: true` on the section, or wrap the tiles in a `container`.

## Configuring data-grid columns

A dashboard `data-grid` renders only the columns in its own `properties.columns`. The dashboard-level `columnsData` stores the viewer's column order and visibility; it does not define columns. Setting `columnsData` alone renders no headers.

When you omit `properties.columns` (or pass `[]`), the create, add, and replace tools fill it from the flow's default columns, but only when `defaultQueryFilters.flowOriginId` holds exactly one flow. If the grid still has no columns, the response carries a `backfill_skipped` warning and the grid renders zero headers.

To choose columns yourself, read the column shape with `get_schema("components", "data-grid")`. Use `name` values from `manage_columns(flow_id, "list")` so the grid resolves work data, and use `type: "status"` for `currentState` to get the status chip.

## Lifecycle & API Pitfalls

1. **Draft id, not published id.** See the table in [Draft Lifecycle](#draft-lifecycle-different-from-flows).
2. **Nested components in `get_dashboard(id, format="full")`.** The full record can nest child components under their section's `components` key, so the top-level array holds only sections. Read the record's `componentsLength` for the true total, not the top-level array length.
3. **`delete_dashboard_components` keeps going past failures.** It returns the `deleted` ids, a `failed` list, and a `partial_failure` warning. Retry only the `failed` ids.
