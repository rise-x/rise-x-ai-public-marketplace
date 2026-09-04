# Advanced Search

Five MCP tools wrap the v4 advanced-search endpoints: `search_flows`, `search_companies`, `search_works`, `search_assets`, and `get_flow_data_schema`. They cover filter-tree queries (AND/OR groups), typed sort, projection, paging, and — for **Work and Asset** — dynamic `data.*` filtering across a flow's user-defined fields.

## ⚠️ When to use these tools — and when NOT to

> **If the user mentions a filter, condition, predicate, sort, range, "find/search/list X where ...", or wants only certain fields back — use `search_works` / `search_flows` / `search_companies` / `search_assets`.** Never reach for `list_work` / `list_flows` / `list_assets` and iterate the response client-side. The server has a real filter-tree-aware query engine; running through 200 work items in your own loop is wasteful, ignores paging, can't sort properly, and gives the user a worse answer.

**Use these when the user asks for:**
- "Find / search / list X where ..." with any condition
- "Show me open works", "works on this flow", "works modified last week", "works assigned to ...", "works where `data.priority` is High"
- Anything mentioning a date range, status, assignee, or any other field-level predicate
- "Find assets of type X", "vessels where `data.imoNumber` starts with 9", "open assets modified this week" → `search_assets`
- Sorting, paging, or specific projected fields
- Discovering what dynamic `data.*` paths a flow / asset type exposes (`get_flow_data_schema`)

**`list_work` / `list_flows` / `list_assets` are ONLY for the unfiltered "give me everything" case** (e.g. "list every flow in this ecosystem" with no further constraint; `list_assets` is additionally scoped to a single asset type). If you find yourself about to filter the response in code, stop and use `search_*` instead.

## Tool Map

| Tool | Endpoint | Use when |
|---|---|---|
| `search_flows(filter, sort, fields, page, page_size, enforce_fields, include_total_count)` | POST `/api/v4/config/flow/search` | Filter flows by status, type, modified-date range, etc. |
| `search_companies(filter, sort, fields, page, page_size, enforce_fields, include_total_count)` | POST `/api/v4/config/company/search` | Filter companies by name, subscription, etc. |
| `search_works(filter, sort, fields, page, page_size, enforce_fields, include_total_count)` | POST `/api/v4/work/search` | Filter work items, INCLUDING by dynamic `data.*` fields defined on the work's flow |
| `search_assets(filter, sort, fields, page, page_size, enforce_fields, include_total_count)` | POST `/api/v4/asset/search` | Filter **assets** (`DianaCompanyEntity` records), INCLUDING by dynamic `data.*` fields defined on the asset type's flow. Prefer over `list_assets` (single asset type, no filtering) for any condition/sort/projection. |
| `get_flow_data_schema(flow_origin_id)` | GET `/api/v4/config/flow/{flowOriginId}/data-schema` | Discover the `data.*` paths a flow / asset type defines — call BEFORE composing a `search_works` **or** `search_assets` payload that uses `data.*` |

All search tools return paged JSON with `items[]`, `page`, `pageSize`, `hasMore` (always populated), `totalCount` (only when `include_total_count=True`, otherwise `null`). See [§Counts & pagination](#counts--pagination).

## Filter Tree Grammar

Every search tool takes a `filter` argument. The filter is either a **leaf** (a single field condition) or a **group** (`and`/`or`) of nested filters.

### Leaf

```json
{ "field": "<fieldKey>", "operator": "<op>", "values": [<v1>, <v2>?] }
```

### Group

```json
{ "and": [<filter1>, <filter2>, ...] }
{ "or":  [<filter1>, <filter2>, ...] }
```

Groups can nest up to depth 5. Combine groups freely: `{"and": [<leaf>, {"or": [<leaf>, <leaf>]}]}`.

### Leaf-XOR-group — one shape per node

A filter node is exactly **one** of: a leaf condition (`field` + `operator` + `values`), an `and` group, or an `or` group. Mixing shapes on the same node is rejected with 400 — the boolean structure has to be unambiguous, so the server (and the local builder, defence-in-depth) refuses combined shapes:

```python
# ❌ Leaf + And children on the same node — 400
{"field": "name", "operator": "equals", "values": ["x"], "and": [{...}]}

# ❌ Both And + Or on the same node — 400
{"and": [{...}], "or": [{...}]}

# ✅ Wrap the leaf inside an explicit and-group:
{"and": [
    {"field": "name", "operator": "equals", "values": ["x"]},
    {...},
]}

# ✅ Nest one group inside the other:
{"and": [
    {"field": "name", "operator": "equals", "values": ["x"]},
    {"or": [{...}, {...}]},
]}
```

### Operators

**Use these exact strings** — the server's `RiseFilterOperator` enum is serialized verbatim. Common shortened forms (`gt`, `gte`, `lt`, `lte`, `eq`, `ne`) are NOT accepted; the validator throws on any unknown operator.

| Operator (exact JSON string) | Allowed on | Values | Notes |
|---|---|---|---|
| `equals` | all types | 1 | |
| `notEquals` | all types | 1 | |
| `in` | all except boolean | 1+ | Set membership. Rejected on boolean (use `equals`/`notEquals`) — listing both values of a 2-valued type is a no-op → 400. |
| `notIn` | all except boolean | 1+ | Rejected on boolean. |
| `contains` | Flow / Company strings | 1 | Substring match. **Flow and Company only** (their whitelist columns are indexed) — `search_works` / `search_assets` reject it, and it is never allowed on `data.*` (use `startsWith`). Not allowed on guid / number / date / boolean. |
| `startsWith` | string only | 1 | Leading-anchored, case-insensitive prefix match. Allowed on all string fields (whitelist + `data.*`) on every resource. |
| `endsWith` | Flow / Company strings | 1 | Trailing-anchored, case-insensitive suffix match. **Flow and Company only** — `search_works` / `search_assets` reject it, and it is never allowed on `data.*`. |
| `between` | number / date | exactly 2 | Inclusive range |
| `greaterThan` | number / date | 1 | NOT `gt` |
| `greaterThanOrEqual` | number / date | 1 | NOT `gte` |
| `lessThan` | number / date | 1 | NOT `lt` |
| `lessThanOrEqual` | number / date | 1 | NOT `lte` |
| `exists` | all types | 0 | Field is present |
| `notExists` | all types | 0 | Field is absent |

Strings on `data.*` paths also support wildcard `prefix*` and `*suffix` via `equals` (server converts to bounded regex). Double wildcards (`*both*`) return 400.

`contains` and `endsWith` are **Flow- and Company-only**: accepted on `search_flows` and `search_companies` string fields, whose whitelist columns are indexed, and **rejected with 400** everywhere else — on `search_works` and `search_assets` (whitelist) strings, and on **all** `data.*` string paths (unanchored / trailing-anchored regex is non-indexable on dynamic fields). The portable string operators — valid on every resource and on `data.*` — are `equals` / `notEquals` / `in` / `notIn` / `startsWith`. The simple `prefix*` / `*suffix` wildcard form above is a separate, non-operator-gated path and still works on `data.*` strings (including for Work).

> ⚠️ **Empty `contains` / `startsWith` / `endsWith` values are rejected.** Passing `{"operator": "endsWith", "values": [""]}` or `[" "]` (whitespace) — likewise for `contains` / `startsWith` — returns 400 from the validator — previously this silently produced a match-everything regex that scanned every document. If the caller's intent is "any/no value", use `exists` / `notExists` instead.

## `enforce_fields` Behaviour

`fields` is **opt-in**. By default the server ignores it and runs the resource's default projection (Always + Fallback entries). Set `enforce_fields=True` on the request to validate `fields` against the whitelist AND apply it as the Mongo projection.

- `enforce_fields=False` (default) → `fields` is ignored entirely. Response carries the fallback projection (every resource's "default columns"). Safe default — Rise resources are dynamic and a hard-coded whitelist may not match the runtime schema.
- `enforce_fields=True` → server validates each `fields` entry against the resource whitelist (400 on unknown keys) and projects only those + the Always-projected keys. Required to surface any **opt-in** field (Work `data.*`, `statusDisplay.*`, `comments`, Flow `publishStatus`, etc.) — without it those keys stay null in the response regardless of what's in `fields`.

## Counts & pagination

`hasMore` is **always populated** on every response (computed server-side as `page * pageSize < totalCount` on the `exactCount=true` path, or derived from a `pageSize+1` probe on the fast path). It is the primary signal for paging — use it to decide whether to fetch the next page.

`totalCount` is **opt-in** via `include_total_count=True`. Behaviour:

| `include_total_count` | What the server runs | `totalCount` | `hasMore` |
|---|---|---|---|
| `False` (default) | Fast path: fetches `pageSize + 1` documents in one round-trip, derives `hasMore` from the extra row. No `$facet + $count` sub-pipeline. | `null` | populated |
| `True` | Full `$facet + $count` aggregation. | populated (exact) | populated |

Server-side knob: maps to `includeTotalCount` in the POST body (or the `_exactCount=true` query-string parameter on the simple GET endpoint). Default is `false` for everything; opt into the slower count when you genuinely need the total (e.g. rendering "N of M results").

## Default soft-delete exclusion

`search_works`, `search_assets`, and `search_flows` hide tombstoned records from the response by default. The exclusion is applied as an AND-clause on top of the caller's filter, but **only when the caller's filter doesn't reference the deletion field**. Referencing the field — even with `notEquals`, `in`, or any other operator — signals "I'm managing this dimension consciously" and the server steps back.

| Resource | Default exclusion | Field that opts out of the default |
|---|---|---|
| `search_works` | `status != "Deleted"` | `status` |
| `search_assets` | `status != "Deleted"` | `status` |
| `search_flows` | `state not in ["Deleted", "Archived"]` | `state` |
| `search_companies` | (none — Company has no soft-delete) | n/a |

**Workflow:**

```python
# Default — deleted works are hidden:
await search_works(filter={"and": [{"field": "flowOriginId", "operator": "equals", "values": ["..."]}]})

# Include deleted works — any reference to `status` opts out of the default:
await search_works(filter={"and": [
    {"field": "flowOriginId", "operator": "equals", "values": ["..."]},
    {"field": "status",       "operator": "in",     "values": ["Open", "Closed", "Deleted"]},
]})

# Only deleted flows:
await search_flows(filter={"field": "state", "operator": "equals", "values": ["Deleted"]})
```

The simple-search query-string equivalent works the same way: `?status=Deleted` or `?state=Archived` opts out of the default exclusion.

**Implementation detail (you don't need this to use the tools, but it explains some quirks).** Legacy MongoDB documents in this codebase store enum fields in two forms — sometimes as the BSON string (`"Deleted"`), sometimes as the underlying int (`2`). The server's default-exclusion filter matches BOTH forms, and so do caller-side `equals`/`notEquals`/`in`/`notIn` on the coerced enum fields (`status`, `flowState`, Flow `state`, `publishStatus`), so `status = "Deleted"` isolates int-form records as reliably as string-form ones. The other operators (`startsWith`, ranges) skip the enum parse.

## Work Search and `data.*` Fields

> ⚠️ **EVERY `search_works` request MUST carry a `flowOriginId` leaf — `equals` for one flow, `in` for multiple — on the AND-spine of the filter.** This is not a `data.*` rule: the server checks the pin *before* it resolves any schema, so an unpinned search returns 400 referencing the discovery URL no matter which fields it names. Only `equals`/`in` count toward it, and a pin nested solely inside an `or` does not (an OR branch does not guarantee every returned row was filtered by it). A `data.*` path needs the pin for the additional reason that the server resolves the flow's schema from it. Workflow: (1) call `get_flow_data_schema(flow_origin_id)` to discover the valid paths, (2) compose `search_works` with the `flowOriginId` leaf AND your `data.*` condition under the same `and` group.

**Wrong** — `data.*` filter with no `flowOriginId` leaf:

```python
# ❌ 400 — server can't resolve which flow's schema to validate data.customer.name against
result = await search_works(
    filter={"field": "data.customer.name", "operator": "equals", "values": ["Bob"]},
)
```

**Right** — `flowOriginId` is a sibling of every `data.*` leaf under an `and`:

```python
result = await search_works(
    filter={
        "and": [
            {"field": "flowOriginId", "operator": "equals", "values": ["aaaa-bbbb-..."]},
            {"field": "data.customer.name", "operator": "equals", "values": ["Bob"]},
        ]
    },
    fields=["id", "displayName", "data.customer.name"],
    enforce_fields=True,   # required to actually project the data.* paths
)
```

The bare `"data"` key in `fields` (no dot, whole-tree projection — see the [whole-data escape hatch](#whole-data-escape-hatch-data-key-no-dot) below) is the one path that skips **schema resolution**, so it needs no `get_flow_data_schema` call. It does not skip the **pin**: the `flowOriginId` leaf is still required, because that check runs ahead of schema resolution.

---

Works carry per-flow user-defined data alongside the static POCO fields. The static whitelist below is searchable without a `get_flow_data_schema` call — it is **not** exempt from the `flowOriginId` pin, which every `search_works` request needs:

| Field key | Type | Notes |
|---|---|---|
| `id` | guid | Work item GUID — direct lookup |
| `name`, `displayName`, `workCode` | string | |
| `normalisedName` | string | Pre-uppercased copy of `name`; faster case-insensitive search |
| `status` | string (enum, PascalCase) | one of: `"Open"`, `"Closed"`, `"Completed"`, `"Deleted"`, `"Ok"` (from `DianaWorkState`). `"Deleted"` is **hidden by default** — see [§ Default soft-delete exclusion](#default-soft-delete-exclusion). Matching rules: ⚠️ enum note below the table. |
| `flowState` | string (enum, PascalCase) | one of: `"NotStarted"`, `"Created"`, `"New"`, `"InProgress"`, `"Rework"`, `"Complete"`, `"Skipped"`, `"Cancelled"`, `"Declined"`, `"Deleted"` (from `DianaStepState`). Matching rules: ⚠️ enum note below the table; a value that is not a member (e.g. `"InReview"`) matches nothing. |
| `flowDisplayName` | string | free-form — the flow's human display name. |
| `flowType` | string | free-form — flow-defined identifier (e.g. `"vessel-inspection"`). |
| `flowId`, `flowOriginId`, `environmentId`, `createdBy`, `lastModifiedBy` | guid | `equals`/`notEquals`/`in`/`notIn`/`exists`/`notExists` only |
| `comments`, `initiatorPartyName` | string | Wildcard `prefix*` / `*suffix` supported |
| `assignedUsers.id` | guid | filter "works assigned to user X" — most common case |
| `assignedUsers.displayName` | string | filter by assigned user's display name |
| `assignedUsers.email` | string | filter by assigned user's email |
| `assignedUsers.companyId` | guid | filter "works assigned to anyone from company X" |
| `assignedUsers` | object — projection-only | whole-array escape hatch; rejects filter / sort with 400. Listing the bare key or any `assignedUsers.*` sub-path projects every RiseSimpleUser field (id, displayName, email, companyId). |
| `entities` | guid (mongo path `entities`) | matches works linked to a specific asset id; `in [...]` for any of several assets; array-of-Guids semantics |
| `lastModified`, `created` | date | range/comparison ops supported |
| `statusDisplay.displayName`, `statusDisplay.color`, `statusDisplay.bgColor`, `statusDisplay.workType`, `statusDisplay.icon`, `statusDisplay.stateName`, `statusDisplay.roleName`, `statusDisplay.value` | string | UI-rendered status snapshot — same fields `list_work` surfaces as `statusLabel` / `roleName` / `color`. |
| `statusDisplay` | object — projection-only | whole-sub-doc escape hatch; rejects filter / sort with 400 |

> ⚠️ **Enum values must be members of the enum.** Stored values are PascalCase (`"Open"`, `"Active"`, `"InProgress"`, `"Entity"`); use that spelling. The Type column says how a field compares: `(enum)` is coerced, `(closed set)` compares the exact string. The five coerced enum fields — Work `status` and `flowState`, asset `status`, Flow `state` and `publishStatus`: on them `equals`/`notEquals`/`in`/`notIn` parse the value case-insensitively and match both stored forms (name and int), so `"open"` finds `"Open"`; every other operator on those fields skips the enum parse. The closed sets (`flowResourceType`, `publishMode`, `resourceType`) have no enum parse: `equals`/`notEquals`/`in`/`notIn` compare the exact string, so on them a wrong-cased value (`"entity"` for `flowResourceType`) is a non-member. A non-member value on either kind (`"entity"` on `flowResourceType`, `"InReview"` on `flowState`, `"Suspended"` on Work `status`) matches nothing: `equals`/`in` return an **empty** page and `notEquals`/`notIn` return **everything**, never a 400. `startsWith`/`endsWith`/`contains` are case-insensitive wherever the operator is allowed (see the operator table). Also `"Open"` appears in BOTH Work `status` AND Flow `state` with different semantics — independent enums, don't assume cross-resource equivalence.

> ⚠️ **Object-typed fields can't be filter or sort leaves.** Any whitelist entry whose type is `object` is a projection root — it exists to opt the whole sub-doc into the response, not to be compared. Using one as a filter / sort leaf returns `400 — Cannot filter on '<field>': this path resolves to an object`. On Work this affects `assignedUsers`, `statusDisplay`, and `data`. **Drop to a typed sub-path instead.** Example — the formerly-supported `assignedUsers` equality is now a `.id` filter:
>
> ```python
> # ❌ Object root — 400:
> filter = {"field": "assignedUsers", "operator": "equals", "values": ["<userid>"]}
>
> # ✅ Typed sub-path:
> filter = {"field": "assignedUsers.id", "operator": "equals", "values": ["<userid>"]}
> ```

> ⚠️ **Mirror rule for projection: use the bare root key, not sub-paths.** When listing an Object root in `fields`, the bare key projects the entire sub-doc: `["statusDisplay"]` returns every `statusDisplay.*` field, `["assignedUsers"]` returns the whole assigned-users array. Sub-paths in `fields` are deduped server-side back to the root, so enumerating them adds noise without trimming the response payload — the projection always pulls the whole sub-doc.
>
> ```python
> # ❌ Over-specified — same payload, 8x the noise
> fields = ["statusDisplay.displayName", "statusDisplay.color",
>           "statusDisplay.bgColor", "statusDisplay.workType",
>           "statusDisplay.icon", "statusDisplay.stateName",
>           "statusDisplay.roleName", "statusDisplay.value"]
>
> # ✅ Bare root — same response, 1 entry
> fields = ["statusDisplay"]
> ```
>
> Exception: `data.*` granular paths DO trim the response (DataList projection is path-aware). List specific `data.foo.bar` paths when you know what you need; reach for bare `"data"` only for schema-free exploration.

**Projection behaviour.** Each whitelist entry has a `ProjectionMode` that controls when it appears in the response:

- **Always-projected** (every response, regardless of `fields`): `id`, `status`.
- **Fallback-projected** (response when `fields` is omitted or `[]`): `name`, `displayName`, `workCode`, `flowState`, `flowDisplayName`, `flowId`, `flowOriginId`, `environmentId`, `lastModified`, `created`, `createdBy`, `lastModifiedBy`, `assignedUsers` (whole array).
- **Opt-in only** (project only when listed in `fields`): `normalisedName`, `flowType`, `comments`, `initiatorPartyName`, `entities`, every `assignedUsers.*` sub-path, every `statusDisplay.*` sub-path, `data.*`.

When `fields` is populated, the response contains **only** the listed fields plus the Always set. For Object-typed roots (`assignedUsers`, `statusDisplay`, `data`), listing the bare key OR any `<key>.<sub>` sub-path opts the whole sub-doc into the response. `data.foo` granular paths project just the listed sub-paths (NOT the whole tree).

Dynamic fields live under the `data.*` namespace. They're discovered per-flow via the `get_flow_data_schema` tool.

### Required workflow for `data.*` (always — no exceptions besides the bare `"data"` key)

1. **Discover** — call `get_flow_data_schema(flow_origin_id)` to get the flat list of valid paths. Skip this only if you already know the exact path from a prior call in the same session.
2. **Filter — flowOriginId is mandatory** — every `search_works` payload MUST include a `flowOriginId` leaf on the AND-spine: `equals` for one flow, `in [...]` for multiple. A payload naming any `data.*` path (in `filter`, `sort`, OR `fields`) must put it under the same `and` group as that path, since the server also resolves the schema from it. Missing → 400 with the discovery URL.
3. **Compose** — reference `data.*` paths exactly as returned by step 1.

End-to-end example showing both leaves under one `and`:

```python
# Step 1: discover
schema = await get_flow_data_schema(flow_origin_id="aaaa-bbbb-...")
# schema is a JSON array; example entries:
#   {"path": "data.custom.value", "type": "array", "isArray": true}
#   {"path": "data.otherValue", "type": "string"}

# Step 2+3: filter + project
result = await search_works(
    filter={
        "and": [
            {"field": "flowOriginId", "operator": "equals", "values": ["aaaa-bbbb-..."]},
            {"field": "data.custom.value[0].name", "operator": "equals", "values": ["Custom Name"]},
        ]
    },
    fields=["id", "displayName", "status", "data.custom.value", "data.otherValue"],
    enforce_fields=True,   # required to actually project the data.* paths
)
```

### Multi-flow merge

`flowOriginId in [flowA, flowB, flowC]` is supported — the server resolves each flow's schema in parallel and merges the available paths into a union. Two rules:

- **Missing flow** → 400 listing the missing ids.
- **Type conflict** → 400 listing every conflict (e.g. `'data.foo': String in flow A vs Number in flow B`). If you hit this, narrow the filter to one flow, or use the whole-data escape hatch below.

### Whole-data escape hatch (`"data"` key, no dot)

Add the bare `"data"` key to `fields` to project the **entire** data sub-tree of each work — no schema lookup. The `flowOriginId` pin is still required, as on every search; use `in [...]` to span several flows in one query:

```python
result = await search_works(
    filter={"and": [
        {"field": "flowOriginId", "operator": "equals", "values": ["aaaa-bbbb-..."]},
        {"field": "status", "operator": "equals", "values": ["Open"]},
    ]},
    fields=["id", "displayName", "data"],
    enforce_fields=True,   # required to project the bare "data" sub-tree
)
```

- Useful for: schema-free exploration, polymorphic responses across heterogeneous flows.
- Trade-off: every work returns its full data tree — large payloads. Prefer specific `data.*` paths when you know what you need.
- `"data"` is projection-only. Filtering or sorting on `"data"` returns 400 (it's an object, not a scalar).
- When `"data"` is in the same request as specific `data.x.y` paths, the parent wins — the granular paths are subsumed.

### Date and number quirks on `data.*`

- **Dates** (`equals`, `between`, `greaterThan`, sort, …) — caller writes the schema path **exactly as returned by `get_flow_data_schema`** (e.g. `data.invoice.dueDate`); do **NOT** append `.date` yourself. The stored value is a `{date, ticks, offset, timezone?}` sub-doc where `ticks` is an **epoch-millisecond string** (NOT a numeric long — set by the frontend). The server parses the ISO `.date` field to a real date for **both** comparison and sort; rows with a missing / unparseable date are dropped, so they never spuriously match `<` / `<=`. (This applies to Work `dataList` arrays and Asset's folded `data` alike.)
- **Number equality** (`equals`, `in`) dual-coerces both string and decimal forms — `5` and `"5"` both match.
- **Number range** (`between`, `greaterThan`, etc.) requires typed-decimal storage. Mixed-type rows (some stored as `"10"` string) silently miss.
- **Boolean equality** (`equals`/`notEquals` only — `in`/`notIn` are rejected on boolean fields) dual-coerces native BSON booleans and JSON-string forms (`true`/`"true"` both match) — some flow data stores booleans as strings rather than actual booleans.

## Asset Search and `data.*` Fields

`search_assets` (POST `/api/v4/asset/search`) queries **assets** — the `DianaCompanyEntity` records created from an asset type. It shares the **entire** filter grammar, operator set, `enforce_fields` behaviour, paging, and the `data.*` + `flowOriginId` contract with `search_works` — the notes here are the **asset-specific deltas only**. When in doubt, the Work rules above apply.

> ⚠️ **Same rule as Work: EVERY `search_assets` request REQUIRES a `flowOriginId` leaf** (`equals` for one asset type, `in` for several) on the AND-spine, whether or not it touches `data.*`. A specific `data.*` path (filter, sort, OR projection) must additionally carry it under the same `and` group as that path, so the server can resolve which asset type's schema to validate against. Missing → 400. Discover paths with `get_flow_data_schema(flow_origin_id)` first (the asset type's `flowOriginId` comes from `search_flows` with `flowResourceType=Entity`, or `list_asset_types`). The bare `"data"` projection key skips the schema lookup but not the pin (whole-data hatch — see below).

### Asset filterable fields (static whitelist)

Searchable without a `get_flow_data_schema` call — but still inside the mandatory `flowOriginId` pin, like every other asset search:

| Field key | Type | Notes |
|---|---|---|
| `id` | guid | Asset (entity) GUID — direct lookup |
| `displayName` | string | the asset's display name (the UI list/grid label) |
| `normalisedName` | string | pre-uppercased copy for fast case-insensitive search |
| `status` | string (enum, PascalCase) | `DianaEntityStatus`: `"Open"` (in edit), `"Closed"` (reserved — not currently used), `"Deleted"`. `"Deleted"` is **hidden by default** unless the filter references `status` — see [§ Default soft-delete exclusion](#default-soft-delete-exclusion). ⚠️ **Distinct enum from Work** — do NOT reuse Work's `DianaWorkState` values (`"Completed"`, `"Ok"`); assets only have Open/Closed/Deleted. Matching rules: ⚠️ enum note under the Work table. |
| `entityType` | string | the asset type's ThingType identifier (e.g. `"vessel"`, `"nmrk-one-car"`) |
| `code` | string | asset code |
| `flowId` | guid | the asset type's current published flow id |
| `flowOriginId` | guid | the asset type's stable origin id — **this is the pin** for `data.*` queries |
| `flowType` | string | free-form flow-defined identifier |
| `companyId` | guid | owning company |
| `sequence` | number | ordering within the asset type |
| `created`, `lastModified` | date | range / comparison ops supported |
| `createdBy`, `lastModifiedBy` | guid | `equals`/`notEquals`/`in`/`notIn`/`exists`/`notExists` only |
| `statusDisplay.displayName` | string | UI status snapshot |
| `statusDisplay` | object — projection-only | whole-sub-doc escape hatch; rejects filter / sort with 400 |
| `data` | object — projection-only | whole-data escape hatch; rejects filter / sort with 400 (see below) |

Operators, enum matching, the Object-root-can't-be-a-filter/sort-leaf rule, and the empty-`startsWith` guard are all **identical to Work**. `contains` / `endsWith` are **Flow- and Company-only** — `search_assets` rejects them with 400 (use `startsWith`). Mixed-type tolerance and the date/number quirks are identical too (see the Work [§ Date and number quirks](#date-and-number-quirks-on-data)).

### Asset `data.*` projection trims like Work — with one shape caveat

Granular projection matches Work: listing `data.foo.bar` in `fields` (with `enforce_fields=True`) returns **only** those folded paths — `fields:["data.price"]` returns `{data:{price:…}}`, not the whole doc. Bare `data` returns the whole folded sub-doc (see below). For **present** fields the response is byte-identical to Work.

**One asset-specific caveat — absent fields:** for a requested granular path the asset **doesn't have**, asset **omits** it (the folded `data` object simply lacks that key; an all-absent projection yields `data:{}`, or a null `data` when the asset has no data at all), whereas Work **materializes** it as `{foo:null}`. Present values are identical on both; only the absent-field shape differs. Don't rely on a requested-but-absent `data.*` key coming back as `null` on asset — it won't be there.

### Bare `data` whole-data hatch (no dot)

Identical to Work: add the bare `"data"` key to `fields` (with `enforce_fields=True`) to project the whole folded `data` sub-document with no schema lookup — bounded only by the per-document ACL. The `flowOriginId` pin is still required; `in [...]` several asset types to span them in one query. Projection-only: filtering or sorting on bare `data` returns 400. Bare `data` differs from a specific `data.*` projection in scope and in schema handling: bare `data` returns the whole doc and skips schema resolution, whereas a specific `data.*` trims to that path and must resolve the pinned type's schema.

### Multi-flow (multi-asset-type) merge

`flowOriginId in [typeA, typeB, …]` merges each asset type's `data.*` schema into a union — same rules as Work (missing id → 400; type conflict → 400; fall back to the bare `data` hatch to sidestep a conflict).

## Flow Search filterable fields

The static whitelist for `search_flows`. Five string fields have closed value sets (`state`, `flowResourceType`, `publishStatus`, `publishMode`, `resourceType`) — listing the value set on each so callers don't have to guess. `state` and `publishStatus` are coerced enum fields (case-insensitive `equals`/`in`, both stored forms); the other three compare the exact string — see the ⚠️ enum note under the Work table.

| Field key | Type | Notes |
|---|---|---|
| `id`, `flowOriginId`, `environmentId`, `createdBy`, `lastModifiedBy`, `flowId` | guid | `equals`/`notEquals`/`in`/`notIn`/`exists`/`notExists` only. `flowId` is a domain alias for `id` (the source POCO declares `FlowId { get => Id; set { } }`) — the search layer exposes both as separate whitelist keys for symmetry with `IDianaFlowResource`-based filters, and both project to the same value. |
| `name`, `normalisedName`, `displayName`, `description`, `uniqueName` | string | free-form. `normalisedName` is pre-uppercased for fast case-insensitive search. |
| `state` | string (enum, PascalCase) | one of: `"Open"`, `"Active"`, `"Archived"`, `"Deleted"` (from `DianaFlowStatus`). Note: distinct from Work `status`. `"Deleted"` and `"Archived"` are **hidden by default** — see [§ Default soft-delete exclusion](#default-soft-delete-exclusion). |
| `flowResourceType` | string (closed set, PascalCase) | one of: `"Work"`, `"Entity"`, `"User"`, `"Company"`. `"Entity"` = asset type; `"Work"` = workflow. `"User"` / `"Company"` are rare system flows. |
| `entityType` | string | free-form — tag value from the flow's `Tags["EntityType"]` dictionary; the asset type's ThingType (e.g. `"vessel"`), the same value the MCP surfaces as `thingType`. Set only on `"Entity"` flows. |
| `publishStatus` | string (enum, PascalCase) | one of: `"Draft"`, `"Published"`, `"Revised"`, `"Deleted"` (from `DianaPublishStatus`). A fifth value `"Publishing"` exists but is a transient/internal state — callers see one of the four listed values once the publish completes. |
| `group` | string | free-form — tag value from the flow's `Tags["Group"]` dictionary. |
| `lastModified`, `created` | date | range / comparison ops supported |
| `copiedFromId` | guid | the source flow this one was duplicated from (lineage tracking). |
| `environment` | string | the environment **name** this flow belongs to (e.g. `"MarineStream"`, `"Woodside"`) — sourced from the flow's `Tags["Environment"]` entry, surfaced as `IDianaFlow.Environment`. Distinct from `environmentId` above — `environmentId` is the implicit per-request scope guid (always AND-ed in by the server's base filter on every request, before the caller's filter runs), whereas `environment` is a queryable human-readable label column. Most callers should rely on the implicit `environmentId` scope and ignore this field. |
| `cardLayoutId`, `summaryCardLayoutId` | guid | layout references — card layout + summary card layout. |
| `template` | boolean | `true` for template flows that are duplicated when callers create a new flow off them. |
| `hasRepeaterSection` | boolean | `true` for flows whose layout contains a repeater section (relevant for data-grid rendering). |
| `blockChainEnabled` | boolean | `true` for flows that publish to a chain. |
| `sequence` | number | flow-defined ordering within an ecosystem (lower number = earlier). |
| `publishMode` | string (closed set, PascalCase) | one of: `"Default"`, `"UpdateOpenItems"`, `"DoNotUpdateOpenItems"`. Controls how a new version propagates to open work items / assets when a flow is published. |
| `resourceType` | string (closed set, PascalCase) | typically `"Workflow"` or `"Flow"` on flow records. Distinct from `flowResourceType` above (the narrower work / entity / user / company set). |
| `versionNumber` | number | numeric version (1, 2, 3, …) extracted from the current `dianaVersion`. |
| `versionName` | string | display label for the current version (`"v2.3"`, `"Q1 release"`, etc.). |
| `fromDate`, `toDate` | date | nested from `dianaVersion.fromDate` / `dianaVersion.toDate` — the active window of the current version. |

## Company Search filterable fields

The static whitelist for `search_companies`. No enum fields — all strings are user-supplied free-form values.

| Field key | Type | Notes |
|---|---|---|
| `id` | guid | direct lookup |
| `name`, `displayName`, `shortCode`, `companyNumber` | string | free-form, user-supplied. `contains` / `startsWith` / `endsWith` all supported — Company shares Flow's indexed-column allowance, unlike Work and Asset. |
| `domains` | string array | user-supplied domain names; `in` matches any element |
| `lastModified`, `created` | date | range / comparison ops supported |

## Projection — `fields` request property

Every `search_*` tool's response shape is controlled by **two** request properties: `fields` AND `enforce_fields`. `fields` alone is ignored — the server only applies a slim projection when `enforce_fields=True` AND `fields` is non-empty:

- **`enforce_fields=False` (default), any `fields` value** → `fields` is **ignored**; the resource's **fallback** projection is returned (backwards compatible).
- **`enforce_fields=True` AND `fields` omitted / empty (`[]`)** → still falls back to the resource's **fallback** projection (server-side guard: empty list is treated as not-supplied).
- **`enforce_fields=True` AND `fields` populated** → ONLY the listed fields are returned, PLUS the resource's **always-projected** fields (the non-nullable summary properties — listed below).

Always-projected per resource (returned in every response):

| Resource | Always-projected keys |
|---|---|
| Work | `id`, `status` |
| Asset | `id`, `status` |
| Flow | `id`, `state`, `flowResourceType` |
| Company | `id` |

Fallback projection per resource (returned when `fields` is omitted / empty):

| Resource | Fallback keys |
|---|---|
| Work | always + `name`, `displayName`, `workCode`, `flowState`, `flowDisplayName`, `flowId`, `flowOriginId`, `environmentId`, `lastModified`, `created`, `createdBy`, `lastModifiedBy`, `assignedUsers` |
| Asset | always + `displayName`, `entityType`, `code`, `flowId`, `flowOriginId`, `companyId`, `created`, `lastModified`, `createdBy`, `lastModifiedBy` |
| Flow | always + `name`, `normalisedName`, `displayName`, `description`, `uniqueName`, `group`, `entityType`, `flowOriginId`, `flowId`, `environmentId`, `lastModified`, `created`, `createdBy`, `lastModifiedBy` |
| Company | always + `name`, `displayName`, `shortCode`, `companyNumber`, `domains`, `lastModified`, `created` |

Opt-in keys (never in the fallback — must be listed explicitly in `fields`):

- **Work**: `normalisedName`, `flowType`, `comments`, `initiatorPartyName`, `entities`, every `assignedUsers.*` sub-path (`assignedUsers.id`, `.displayName`, `.email`, `.companyId`), every `statusDisplay.*` sub-path, `data.*`.
- **Flow**: `publishStatus`, `copiedFromId`, `environment`, `cardLayoutId`, `summaryCardLayoutId`, `template`, `hasRepeaterSection`, `blockChainEnabled`, `sequence`, `publishMode`, `resourceType`, `versionNumber`, `versionName`, `fromDate`, `toDate` (the 14 extended-whitelist fields beyond `publishStatus`; all become projectable when listed in `fields` with `enforce_fields=True`).
- **Asset**: `normalisedName`, `flowType`, `sequence`, `statusDisplay` (+ any `statusDisplay.*`), `data` (+ any `data.*`). Granular `data.*` paths trim to those paths (same as Work); bare `data` = whole doc.
- **Company**: (none today.)

**Object-typed roots** — Work: `assignedUsers`, `statusDisplay`, `data`; Asset: `statusDisplay`, `data` (`assignedUsers` is Work-only). The bare key returns 400 if used as a filter / sort leaf ("resolves to an object"). The projection rules below apply to both resources. For projection:
- Bare `assignedUsers` (**prefer this**) or any `assignedUsers.*` sub-path → whole `assignedUsers` array projected (every `RiseSimpleUser` field). Sub-paths in `fields` are deduped to the root server-side; they add no projection benefit. Use them only for filtering.
- Bare `statusDisplay` (**prefer this**) or any `statusDisplay.*` sub-path → whole `statusDisplay` sub-doc projected. Sub-paths in `fields` are deduped to the root server-side; they add no projection benefit. Use them only for filtering.
- Bare `data` → whole data sub-tree projected. **Work:** the `DataList` merged across every section start-to-end (later sections override earlier ones on shared paths). **Asset:** the folded entity `data` object (not a DataList). Large payload — escape hatch.
- `data.foo` sub-paths → granular projection (only the listed sub-paths come back). **Preferred for `data` when payload matters** — `data.*` is the one exception where sub-paths actually trim the response. **(`search_assets` trims the same way; see [§ Asset Search](#asset-search-and-data-fields) for its one absent-field shape caveat.)**

## Response Shape

```jsonc
{
  "items": [
    {
      "id": "9f8a-...",
      "displayName": "Invoice #1042",
      "status": "Open",
      // ... other whitelisted POCO fields ...
      "data": {                          // present only when data.* was projected
        "custom": { "value": [{"name": "Custom Name"}] },
        "otherValue": "hello"
      }
    }
  ],
  "totalCount": 42,                      // populated when include_total_count=True; null on the default fast path
  "page": 1,
  "pageSize": 25,
  "hasMore": true
}
```

Notes on the shape:

- `data` is a **nested JObject** mirroring the source data tree — `data.step2.toggle1` surfaces as `{"data": {"step2": {"toggle1": true}}}`, NOT a flat dotted-key dict. The outer JSON property is already named `data`, so the inner keys drop the `data.` prefix.
- Date fields under `data.*` round-trip as `{date, ticks, offset}` JSON sub-objects (not flat ISO strings).
- Bool / number / string types are preserved end-to-end.
- On `search_assets`, a requested-but-absent `data.*` path is **omitted** from the folded `data` object (an all-absent projection yields `data: {}`), whereas Work materialises it as `{"foo": null}`. See [§ Asset Search and data.* Fields](#asset-search-and-data-fields).

## Sample Requests

### Flow search — filter by flowResourceType and publishStatus

```python
await search_flows(
    filter={
        "and": [
            {"field": "flowResourceType", "operator": "equals", "values": ["Entity"]},
            {"field": "publishStatus", "operator": "equals", "values": ["Published"]},
        ]
    },
    sort=[{"field": "lastModified", "order": "desc"}],
    fields=["id", "flowOriginId", "displayName", "entityType"],
    enforce_fields=True,
    page=1,
    page_size=50,
)
```

### Work search — open works on a flow, newest first

```python
await search_works(
    filter={
        "and": [
            {"field": "status", "operator": "equals", "values": ["Open"]},
            {"field": "flowOriginId", "operator": "equals", "values": ["aaaa-bbbb-..."]},
        ]
    },
    sort=[{"field": "lastModified", "order": "desc"}],
)
```

### Work search — multi-flow data filter

```python
await search_works(
    filter={
        "and": [
            {"field": "flowOriginId", "operator": "in",     "values": ["aaaa", "bbbb"]},
            {"field": "data.priority", "operator": "equals", "values": ["High"]},
        ]
    },
    fields=["id", "displayName", "status", "data.priority", "data.assignedTeam"],
    enforce_fields=True,   # required to actually project the data.* paths
)
```

### Work search — discovery first, then filter

```python
schema = await get_flow_data_schema(flow_origin_id="aaaa-bbbb-...")
# schema is a JSON array — iterate it and use each entry["path"] to compose the payload

await search_works(
    filter={
        "and": [
            {"field": "flowOriginId", "operator": "equals", "values": ["aaaa-bbbb-..."]},
            {"field": "data.invoice.amount", "operator": "greaterThanOrEqual", "values": [1000]},
        ]
    },
    sort=[{"field": "data.invoice.dueDate", "order": "desc"}],
    fields=["id", "displayName", "data.invoice.amount", "data.invoice.dueDate"],
    enforce_fields=True,   # required to actually project the data.* paths
)
```

### Asset search — data.* filter with the flowOriginId pin, trimmed projection

```python
# flowOriginId comes from search_flows (flowResourceType=Entity) or list_asset_types; discover the data.* paths first
schema = await get_flow_data_schema(flow_origin_id="vvvv-tttt-...")

await search_assets(
    filter={
        "and": [
            {"field": "flowOriginId",   "operator": "equals",     "values": ["vvvv-tttt-..."]},
            {"field": "status",         "operator": "equals",     "values": ["Open"]},
            {"field": "data.imoNumber", "operator": "startsWith",  "values": ["9"]},
        ]
    },
    sort=[{"field": "lastModified", "order": "desc"}],
    fields=["id", "displayName", "entityType", "data.imoNumber"],
    enforce_fields=True,   # required to project the data.* paths (trimmed to just these)
)
```

`contains` / `endsWith` are Flow- and Company-only — assets use `startsWith`. Requesting a specific `data.*` path trims the folded `data` object to just that path; bare `data` returns the whole sub-document (still pinned, but no schema lookup). See [§ Asset Search and data.* Fields](#asset-search-and-data-fields).

## Common Pitfalls

1. **Forgetting `flowOriginId` on `data.*` Work search** → 400 referencing the discovery URL. Add a `flowOriginId equals` (or `in`) leaf to the filter, OR use the bare `"data"` projection key to skip schema validation.
2. **Calling `search_works` before `get_flow_data_schema`** → likely 400 on an unknown `data.*` path. Discover first when in doubt.
3. **Type conflict in multi-flow merge** → narrow the filter to one flow or use the whole-data key.
4. **`contains` on a Guid field** → 400. Guid fields only accept `equals`/`notEquals`/`in`/`notIn`/`exists`/`notExists`.
5. **Range op (`between`/`greaterThan`/`lessThan`) on a mixed-type `data.*` number field** → silently misses string-stored values. Range ops require typed storage; equality dual-coerces.
6. **Setting `fields` without `enforce_fields=True`** → the server ignores `fields` by default and returns the fallback projection. Opt-in fields (`data.*`, `statusDisplay.*`, `comments`, `publishStatus`, etc.) stay null. Set `enforce_fields=True` when you actually want the projection applied.
7. **Filtering on `data` (no dot)** → 400. The whole-data key is projection-only.
8. **Mixed-shape filter node** (leaf + `and`/`or` children, or both `and` + `or` on the same node) → 400. A node is exactly one of leaf, And-group, Or-group. Wrap the leaf in an explicit `and` instead.
9. **Assuming `search_assets` treats `data.*` projection differently from Work** → it doesn't: granular `data.*` paths trim on both, and present values are identical. The only asset difference is shape-only — a requested-but-**absent** `data.*` path is **omitted** on asset, whereas Work materializes it as `null`.
10. **Using `list_assets` to filter or sort** → `list_assets` lists a single asset type via the data grid with no filter engine. Use `search_assets` for any condition, sort, or projection.
11. **Reusing Work `status` values on `search_assets`** → asset `status` is `DianaEntityStatus` (`"Open"` / `"Closed"` / `"Deleted"` only). Work's `"Completed"` / `"Ok"` are not asset states; see #12 for what a non-member value does.
12. **A coerced-enum value that is not a member** (`"InReview"` on `flowState`, `"Suspended"` on Work `status`) → matches nothing: `equals`/`in` return an **empty** page, `notEquals`/`notIn` return **everything**, never a 400. Check the value against the enum lists in the field tables before trusting the result.
