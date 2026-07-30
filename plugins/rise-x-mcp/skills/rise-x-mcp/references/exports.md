# CSV / Report Exports (`manage_export`)

Flows can define one or more **export configurations** that turn work items into CSV/Excel rows. Manage them with `manage_export` — **never** pass `export`/`exports` to `update_flow_properties` (it rejects sub-resource keys up front and points you to the dedicated `manage_*` tool).

```
manage_export(flow_id, "list")
manage_export(flow_id, "add",    export={...})
manage_export(flow_id, "update", export_id=..., export={...})
manage_export(flow_id, "delete", export_id=...)
```

The flow must be in **draft** mode (`create_flow_draft` first if published), and you must `publish_flow` afterwards for the export to go live. Discover the full config shape with `get_schema("flows", "export")` (`FlowPocoExport`).

## The mental model (read this first)

An export config has **`shardConfigs`** (how many ROWS) and **`fieldConfigs`** (the COLUMNS + their values). Each path is resolved per work, scoped to the current shard combination.

1. **Path scoping.** `$.root.{$.shards.workId}` is the current work. Work-root fields are `workCode`, `status`, `statusDisplay.displayName`, `id`; **every form field lives under `data.*`** (e.g. `data.requestReason`, `data.equipment`).
2. **Auto-prefixing.** With `useExact: false` (the default), a path starting with `$.` (but not `$.root.`) is rewritten to `$.root.{$.shards.workId}.<rest>` — so you author **relative-to-the-work** paths.

### Path rule — the #1 export gotcha

**Author form-field paths with the `data.` segment: `$.data.<field>`.** A bare `$.<field>` resolves only for the work-root fields (`workCode`, `status`, `statusDisplay…`); for any form field it silently produces a **blank cell** (`-`).

| Want | valuePath |
|---|---|
| Work code | `$.workCode` (work-root) |
| Status label | `$.statusDisplay.displayName` (work-root) |
| A form field "Priority" | `$.data.priority` |
| A nested form field | `$.data.section.field` (the field's real `dataPath` minus the leading `$.`, prefixed with `data.`) |

**Scalars need no `manage_columns` entry** — the correct `$.data.<field>` path is enough (verified live: `siteCellId` / `requestDate` exported with no column). **A data-grid is the exception** — it must be registered as expandable work-grid column(s) to appear in the export at all; see below.

## `fieldConfigs` vs `shardConfigs`

- **`shardConfigs` decide how many ROWS** — a row-multiplier (array unwind), **not** an entity join. `key`, `jsonPath`, `header`, `sortOrder`, `isGuid` are used; `entityType` / `alternateKeys` / `propertyName` are inert here (import-side metadata).
- **`fieldConfigs` decide the COLUMNS and their values** — `columnPath` is the header, `valuePath` is what's read, `dataType` types it, `format` formats it.

Multiple shards produce the **cross-product** (one row per combination). Row identity is a hash of the shard values, so each shard `key` must resolve to a unique value per element or rows collapse to one.

### Expanding a data-grid to one row per element

Verified live 2026-06-18 (mirrored from a working production export). A data-grid needs **two** things — both required:

**1. Register the grid as expandable work-grid column(s)** (`manage_columns add`). This is what exposes the grid to the export's row projection — **without it the export emits one flat row with blank per-element cells, no matter how correct the export paths are.** Each column:
- `isExpandedProperty: true`
- `valuePaths: ["$.<grid>[{$.index}].<sub>"]` — note the **`[{$.index}]`** index token (the work-grid's positional row index)
- `componentId: <the data-grid component's id>` (from `get_layout`), an explicit `id` (pass a GUID — the first auto-assigned id can come back null and then collide on the next add), plus `key`, `displayName`, `type: "ENUM"`
- For the **export** you don't need one column per sub-field — a couple exposes the whole element and the `fieldConfigs` then read *every* sub-field. (Add one per sub-field only if you also want them as expandable columns in the work-grid UI.)

**2. The export config** (`manage_export`) — shard with `[*]`, per-element fields with a **bare** shard token in the filter:
- **Shard:** `$.data.<grid>[*].id` (bracket-star — array index).
- **Field:** `$.data.<grid>[?(@.id == $.shards.<key>)].<sub>` — the filter compares against the **bare** `$.shards.<key>`, **NOT** the quoted/braced `'{$.shards.<key>}'` (that never matches → blank cells + collapsed rows). Note the contrast: the path *prefix* uses the braced `{$.shards.workId}`, but the *filter expression* uses the bare `$.shards.<key>`.

```jsonc
// (a) manage_columns add — one+ expandable columns EXPOSE the grid to the export:
{ "id": "<guid>", "componentId": "<data-grid component id>", "key": "category",
  "displayName": "Category", "type": "ENUM", "isExpandedProperty": true,
  "valuePaths": ["$.equipment[{$.index}].category"] }

// (b) manage_export add/update:
{
  "name": "Equipment Register",
  "dateFormat": "dd/MM/yyyy",
  "shardConfigs": [
    { "key": "workId",      "header": "Work ID",      "jsonPath": "$.root.*.id",            "isGuid": true,  "sortOrder": 1 },
    { "key": "equipmentId", "header": "Equipment Row", "jsonPath": "$.data.equipment[*].id", "isGuid": false, "sortOrder": 2 }
  ],
  "fieldConfigs": [
    // work-level columns — no shard reference, repeat on every row
    { "columnPath": "Work Code",      "valuePath": "$.workCode",                  "dataType": "String", "sortOrder": 3 },
    { "columnPath": "Status",         "valuePath": "$.statusDisplay.displayName", "dataType": "String", "sortOrder": 4 },
    { "columnPath": "Request Reason", "valuePath": "$.data.requestReason",        "dataType": "String", "sortOrder": 5 },
    { "columnPath": "Request Date",   "valuePath": "$.data.requestDate",          "dataType": "Date", "format": "dd/MM/yyyy", "sortOrder": 6 },
    // per-element columns — BARE $.shards.<key> in the filter
    { "columnPath": "Category",       "valuePath": "$.data.equipment[?(@.id == $.shards.equipmentId)].category",     "dataType": "String", "sortOrder": 7 },
    { "columnPath": "Serial Number",  "valuePath": "$.data.equipment[?(@.id == $.shards.equipmentId)].serialNumber", "dataType": "String", "sortOrder": 8 },
    { "columnPath": "Quantity",       "valuePath": "$.data.equipment[?(@.id == $.shards.equipmentId)].quantity",     "dataType": "Number", "sortOrder": 9 }
  ]
}
```

Shard 1 makes one branch per work; shard 2 makes one row per equipment element. Work-level fields ignore the equipment shard, so they repeat on each row. A work with an empty grid emits a single row (blank per-element cells). If the element `id` can be blank/duplicate, key the shard on a guaranteed-unique field (e.g. `serialNumber`) instead.

## Dates

Date-picker fields store an **object** `{ date, offset, ticks, timezone }`. With `dataType: "Date"` the engine reaches into `.date` for you, so **either** form works:

- `valuePath: "$.data.requestDate"`, `dataType: "Date"`, `format: "dd/MM/yyyy"` — point at the object (recommended), or
- `valuePath: "$.data.requestDate.date"`, `dataType: "Date"` — point at the ISO string.

Set the config-level `dateFormat` too (default `dd/MM/yyyy`) — `Date` fields are coerced to it in several branches regardless of per-field `format`.

## Other notes

- **`dataType`** values: `String, Text, Number, Date, Boolean, Integer, Decimal, Object, Array, Comments`.
- **MatchColumn fan-out:** a `fieldConfig` with `matchColumn` (regex) over an array `valuePath` emits **multiple dynamic columns** (one per matching child key) — for wide/dynamic columns, not row expansion.
- **Empty cells render `-`.** Don't rely on truly blank cells. Values beginning `= + - @` are tab-prefixed to defuse Excel formula injection.
- **`sortOrder > 0`** on shards/fields sorts rows by those columns and orders the columns.
- **Server rewrites your paths on save** (the `$.root.{$.shards.workId}.…` form) and `get_schema`/`manage_export("list")` echoes the rewritten version — that's expected, not corruption.
- **`manage_export("list")` on the *origin* id can read stale right after `publish_flow`** — for a moment it may echo the *previous* version's config (this once looked like publish had "reverted" a fix when it hadn't). To verify what actually published, `create_flow_draft` from the published flow and `list` the draft. **`publish_flow` is what commits the export to the live version.**

## Common mistakes

1. **Omitting `data.`** — `$.requestReason` instead of `$.data.requestReason`. Form fields silently export blank. The single most common scalar error.
2. **Grid not registered as expandable column(s)** — without an `isExpandedProperty: true` column on the grid (`valuePaths: ["$.<grid>[{$.index}].<sub>"]`), the export omits the grid entirely → one flat row, blank per-element cells, no matter how correct the shard/fields are. **The most common data-grid export failure.**
3. **Quoted/braced shard token in the filter** — `[?(@.id == '{$.shards.equipmentId}')]` never matches → blank cells + collapsed rows. Use the **bare** form: `[?(@.id == $.shards.equipmentId)]`.
4. **Wrong grid shard wildcard** — the shard is `$.data.<grid>[*].id` (bracket-star). `..` / `.*` do **not** apply here; via the expandable-column projection the export sees the grid as an array.
5. **Treating `shardConfigs` as entity joins** — they only unwind arrays; `entityType` does nothing for export.
