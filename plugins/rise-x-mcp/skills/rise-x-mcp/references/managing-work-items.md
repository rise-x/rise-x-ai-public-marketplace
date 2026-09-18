# Managing Work Items

## What is a Work Item?

A **work item** is a running instance of a flow. It progresses through steps via submit actions, contains data fields, and tracks a status (`Open`, `Closed`, `Completed`, `Deleted`; `Ok` is a sync-process flag, not a state a user drives).

Work tools are also used as part of the **asset workflow pattern** — see `references/managing-assets.md`.

## ⚠️ Choosing the right tool for retrieval

| You need to… | Use |
|---|---|
| Find work items matching ANY filter (status, date range, assignee, `data.*` field value, server-side sort, paged results, etc.) | **`search_works`** — see `references/advanced-search.md` |
| Look up a single work item by GUID | `get_work(id)` |
| List a flow's work items with simple built-in filters (status, date range, paging) | `list_work(flow_origin_id, …)` — for arbitrary predicates / sort / projection use `search_works` |

**Do not** call `list_work` and iterate the result client-side to find matches — the server has a real filter / sort / projection engine via `search_works`. Reaching for `list_work` for filtered queries is the most common mistake; if there's a filter in the user's question (e.g. "open works", "works modified last week", "works where data.priority is High"), the right tool is `search_works`, not `list_work`.

> ⚠️ **`search_works` always requires `flowOriginId`.** EVERY request MUST contain a `flowOriginId` leaf — `equals` for one flow, `in` for multiple — on the AND-spine of the filter; the server checks this before resolving any schema, so an unpinned search returns 400 whatever fields it names. A request touching a `data.*` path (in `filter`, `sort`, OR `fields`) must put the pin under the same `and` group as that path, and should discover the valid paths first via `get_flow_data_schema(flow_origin_id)`. The bare `"data"` projection key (no dot) skips schema resolution but still needs the pin. On Work, `contains` and `endsWith` are **not available at all** — they are Flow- and Company-only, so Work string fields (both the static whitelist and `data.*`) accept only `equals`/`notEquals`/`in`/`notIn`/`startsWith`. Full wrong-vs-right examples in `references/advanced-search.md`.

## Tools

### `create_work(flow_id: str, step_name: str | None = None)`
Create a new work item by starting a published workflow.

**Parameters:**
- `flow_id` — the workflow's flow `id` (the published flow's primary identifier). Do **not** pass `flowOriginId` — that's reserved for asset instance operations like `create_asset` (see `references/managing-assets.md`).
- `step_name` — optional step name to start at. If omitted, starts at the first step.

**Returns** JSON with:
- `workId` — the new work item GUID
- `stepName` — current step the work is on
- `eventName` — the action event name (usually `"Submit"`) for advancing the work
- `nextSteps` — human-readable instructions for the next calls

This is **Step 1** of the work creation pattern:
1. `create_work(flow_id)` → get `workId`, `stepName`, `eventName`
2. `update_work_data(workId, ...)` → set field values (repeat as needed)
3. `submit_work(workId, eventName, stepName)` → advance to the next step

### `get_work(id: str, format: str = "summary")`
Get a work item by its GUID.

- `format="summary"` (default) — `id`, `workCode`, `name`, `displayName`, `flowId`, `flowOriginId`, `flowDisplayName`, `activeStepId`, `activeStepName`, `status`, `statusLabel`, `statusColor`, `flowState`, `workState`, `workStateName`, `currentState`, `flowType`, `roleName`, `canEdit`, `canDelete`, `canDelegate`, `attachments`, `createdDate`, `lastModified`, `createdBy`, `lastModifiedBy`, `data`, `assignedUsers`, `roles`, `relationships`, `tasks`, `globalActions`, `submitErrorRequestIds`, and `actions` (each `{stepName, stepId, stepDisplayName, canExecute, invitation, events: [{id, eventName, name, displayName}]}`). Adds an `omitted` note listing the dropped top-level keys that carried a value — typically `steps`, `users`, `chains`, `cardLayout`, `companies`, `company`, `lastModifiedTicks`, `schemaVersion`, `workStateColor` — and ends with "pass format='full' to include".
- `format="full"` — the raw v3 document, `steps` and all. Pass this when you need something the `omitted` note flagged.

Key fields in the summary response:
- `id` — work item GUID
- `activeStepName` — current step the work is on
- `actions` — available actions, with `events[].eventName` for `submit_work`
- `data` — form field values
- `status` — `Open`, `Closed`, `Completed`, `Deleted`, or `Ok` (`DianaWorkState`; `Ok` is a sync-process flag, not a state a user drives)

### `duplicate_work(id: str, response_format="summary")`
Duplicate a work item into a **brand-new work** on the same flow — the DIANA-native "Duplicate" button.

- The new work's starting data is filtered by the flow's `flowFeatures.createAndDuplicateWork.duplicate` config: `includePaths` (whitelist, applied first — `["$"]` clones everything) then `excludePaths` (removed after). With the feature unset the flow **full-clones** by default; when `duplicate.enabled` is `false` the API returns **404**. Configure the paths with `update_flow_properties` — see `references/managing-flows.md` § Duplicate & Create Work.
- Returns a mutation envelope for the **new** work (`workId` differs from the source) plus a `hint`. It does **not** modify the source work.
- **Primary use — verify an exclusion config:** `duplicate_work(sourceWorkId)` → `get_work(newWorkId)` → confirm the `excludePaths` fields are absent and the rest carried over. This is the only way to exercise the duplicate behaviour from MCP.

### `list_work(flow_origin_id, skip=0, limit=50, status="all", active_step_ids?, sort_field?, sort_direction?, date_from?, date_to?)`
List work items for a given workflow using the data grid. Paginated and filterable — full parameter and response-envelope details in the **list_work pagination & filters** section below.

> **Prefer `search_works` for ANY filtered query.** Use `list_work` only when you genuinely want every work item on a flow with no predicate. Calling `list_work` + filtering client-side is wrong — the server has a real filter / sort / projection engine; reaching for it via `search_works` is faster, smaller, and respects pagination. See `references/advanced-search.md`.

- `flow_origin_id` — **the workflow's stable `flowOriginId`, NOT a published-version `flowId`.** Works carry a single `flowOriginId` across every version of the flow they were created against, while `flowId` changes on every publish. Filtering by `flowId` would only return works pinned to that one version (and v3 doesn't fall back automatically). Passing the wrong id kind silently returns `[]` with no error. Get the origin id from `list_flows()` → `flowOriginId`, from any work's `flowOriginId` field, or from `get_flow_config(flow_id).flowOriginId`.

  This is the opposite convention from `create_work` / `get_flow_config` / `get_flow_steps`, which take the published `flow_id`. **Both ids look like GUIDs**, so the tool can't disambiguate at call time — use the right one.

- `skip` — pagination offset (default 0); `limit` — page size (default 50)
- `status` — `"all"` (default) | `"inProgress"` | `"completed"` | `"deleted"`
- `active_step_ids`, `sort_field`/`sort_direction`, `date_from`/`date_to` — optional filters (full details in the pagination section below)

The response is a paginated envelope `{items, returned, skip, limit, hasMore, nextSkip}` — loop on `nextSkip` until `hasMore` is false.

Response rows are **projected** to the useful fields only (cardLayout, per-role user lists, companies, and chains are stripped to keep context small). Each row contains:
- Identity: `id`, `workCode`, `displayName`, `flowId`, `flowOriginId`, `flowDisplayName`
- State: `activeStepId`, `status`, `state`, `statusLabel`, `statusColor`
- Permissions: `canEdit`, `canDelete`, `canDelegate`, `roleName`
- Audit: `createdBy` / `lastModifiedBy` (each `{id, email, displayName}`), `createdDate`, `lastModifiedDate`

Date field names differ by tool — see `references/common-pitfalls.md` pitfall #65.

For form data, status and actions, call `get_work(id)` (summary); for `cardLayout`, per-role user lists, `chains`, or `steps`, call `get_work(id, format="full")`.

### `search_works(filter, sort, fields, page, page_size, enforce_fields, include_total_count)`
**Advanced Work search with filter tree, sort, projection, and paging — including dynamic `data.*` fields.** This is the right tool whenever the user describes a filter, a sort, or wants specific fields. Full reference in `references/advanced-search.md`. Quick template:

```python
await search_works(
    filter={"and": [
        {"field": "flowOriginId", "operator": "equals", "values": ["<flow-origin-id>"]},
        {"field": "status",        "operator": "equals", "values": ["Open"]},
    ]},
    sort=[{"field": "lastModified", "order": "desc"}],
    page=1,
    page_size=25,
)
```

For dynamic `data.*` paths in the filter, call `get_flow_data_schema(flow_origin_id)` first to discover the valid paths for that flow.

### `update_work_data(id, json_path, operation, value, section_name)`
Update a field value on a work item.

**Parameters:**
- `id` — work item GUID
- `json_path` — path to the field, format: `"$.taskName.fieldName"` (e.g. `"$.vessel.name"`)
- `operation` — one of:
  - `"set"` — create or overwrite the value
  - `"push"` — append to an array
  - `"pull"` — remove the field
  - `"rename"` — rename the field (value = new name)
- `value` — the value to set/push, or new name for rename
- `section_name` — **required** — the task name scoping the data location (e.g. `"UntitledTask/Generated-..."` or the camelCase task name from `get_flow_steps`). Omitting it causes 500 errors.

#### `section_name` is the action set name's MIDDLE segment

The most reliable way to find it is to take the task's **action set name** and
read the middle out of it, rather than guessing a task name:

| Action set name | `section_name` |
|---|---|
| `purchaseOrderDetails/task1/Actionset` | `task1` |
| `bloodProducts/details/Actionset` | `details` |
| `ActionSet_1` (bare, no segments) | `Task_1` |

The bare `ActionSet_1` case has no middle segment to read — an asset type whose
action set is bare took `Task_1`. Verified on one supplier asset type; whether
`Task_N` always pairs with `ActionSet_N` was **not** established.

> ⚠️ **Passing the STEP name returns HTTP 500 with no useful message** —
> `{code: http_500, message: '500: None'}`. The server does not say what it
> wanted. Finding the right value is trial and error unless you read it off the
> action set, so read it off the action set.

#### Writing a date

A `date-picker` field stores an object, not a string: `{date, offset, ticks, timezone}`
(the READ shape — see `references/advanced-search.md` § Date and number quirks on `data.*`). For the
**write**, the helper that builds that object accepts a bare `YYYY-MM-DD` or a
full ISO instant:

```
update_work_data(workId, "$.inquiry.dueDate", "set", "2026-03-14", section_name=…)
update_work_data(workId, "$.inquiry.dueDate", "set", "2026-03-14T09:00:00Z", section_name=…)

# CRITICAL: anything else silently yields an EMPTY STRING. No warning, no
#   error — the field just ends up blank. Read the value back and confirm it
#   is an object with a `date`, not "".
```

Can be called multiple times for different fields before submitting. Must run **sequentially** — parallel calls cause connection errors.

### `submit_work(id, event_name, step_name, invitation=None)`
Submit work to advance it to the next step in its workflow. Leave `invitation`
as `None` unless you are deliberately supplying recipients — a non-null
invitation stops the server deriving recipients from the flow config.

**Parameters:**
- `id` — work item GUID
- `event_name` — the action event triggering the transition (e.g. `"Submit"`, `"Approve"`, `"Reject"`). Get from the work item's `actions` array.
- `step_name` — **the ACTION SET name, not the task name and not `activeStepName`'s display form.** Take it verbatim from the item's own offered action — see below.
- `invitation` — optional dict with routing destinations for the next step

#### `step_name` is the ACTION SET name

> ⚠️ **A task name returns `ok: true`, moves nothing, saves nothing, and leaves
> `statusLabel` unchanged** (e.g. still `Draft`). There is no error, no warning,
> and no `changed` diff to catch it. This cost hours twice in one session.

Four action set name shapes were observed coexisting in a **single** ecosystem,
so there is no convention to construct a name from:

| Observed name | Shape |
|---|---|
| `reviewRouteApprove_set` | flat, `_set` suffix |
| `purchaseOrderDetails/task1/Actionset` | hierarchical, slash form |
| `bloodProducts/details/Actionset` | hierarchical, slash form |
| `ActionSet_1` | bare |

**So take the name from the item, never build it:**

```
1. get_work(workId)
     → actions[].stepName          # THIS is the value for step_name
     → actions[].events[].eventName  # and this for event_name
   # or, in the asset create/edit path:
   #   create_asset / edit_asset response → sourceStepName
   # NOTE: `create_asset`'s `eventName: "Submit"` is a DISPLAY label — see
   #   `references/managing-assets.md` Common Mistakes #7.

2. submit_work(workId, event_name=<from events[]>, step_name=<from actions[].stepName>)

3. # CRITICAL: verify the transition LANDED. `ok: true` is not evidence of a
   #   transition — it is evidence the request was accepted.
   get_work(workId) → confirm `status` / `statusLabel` actually CHANGED.
   # Unchanged statusLabel with ok: true means the name was wrong and the
   #   submit was a no-op. Do not report success.
```

This is a different name from the one `ByStepName` action routing takes. A
`next[].nextStep` targeting `"ByStepName"` wants the internal camelCase **step**
name (`"supplierOffer"`) — see `references/actions-and-statuses.md` § Routing
with `next`. `submit_work`'s `step_name` wants the **action set** name. Both are
called a "step name" in the wire format; they are not interchangeable.

`search_works` cannot tell you the value. Its Work whitelist carries no
`activeStepId`, `activeStepName`, `currentState`, or `statusLabel` key at all —
per-task position only ever comes from `actions[].stepName` on `get_work`, so a
row fetched by search alone cannot say which task an item is on. See
`references/advanced-search.md` § Work Search and `data.*` Fields.

## Creating a New Work Item

```
1. create_work(flow_id)                      # start workflow → get workId, stepName, eventName
2. update_work_data(workId, "$.task.field",  # fill in fields (repeat as needed, sequentially)
     "set", "value", section_name)
3. submit_work(workId, eventName, stepName)  # advance to next step
   # CRITICAL: `stepName` here is the ACTION SET name from the create_work
   #   response, not a task name. Then re-read the work and confirm the status
   #   moved — see § `step_name` is the ACTION SET name.
```

The `flow_id` is the workflow's ID — find it via `get_flow_config` or from the flow creation step. The `section_name` for `update_work_data` is the task's `taskName` (slash form like `UntitledTask/Generated-<guid>` on v3-style flows, or bare like `Task_1` on v4 native flows). `get_flow_steps` projects `taskName` **out** of its response — fetch it via `get_flow_step(flow_id, step_id)` (pass the `id` field from `get_flow_steps` as `step_id`) and read `taskName` from the full step.

## Progressing an Existing Work Item

```
1. list_work(flow_origin_id)                 # find items in a workflow (pass the ORIGIN id, not the published flow id)
2. get_work(id)                              # inspect details, find stepName and actions
3. update_work_data(id, "$.task.field",      # fill in fields (repeat as needed)
     "set", "value", section_name)
4. submit_work(id, event_name, step_name)    # advance; BOTH come from
                                             #   get_work's actions[] — the
                                             #   action set name, verbatim
5. get_work(id)                              # CRITICAL: confirm statusLabel
                                             #   actually changed
```

## Important Notes

- `json_path` must start with `$` — e.g. `"$.reviewTask.approved"`, `"$.details.description"`
- The path follows the pattern `$.{taskCamelCase}.{fieldCamelCase}`
- `event_name` and `step_name` must match exactly what the flow expects — get them from the `get_work` response's `actions[]`, never by hand
- Submitting with the wrong event name fails. Submitting with the wrong `step_name` **does not fail** — it returns `ok: true` and does nothing. Always re-read the work and confirm the status moved (§ `step_name` is the ACTION SET name)
- A work item's `workCode` is **not unique** — two work items can carry the same code. Key on `id`

## list_work pagination & filters (server ≥ Trust & Signal)

`list_work` is paginated and filterable: `limit` (default 50), `skip`,
`status="all"|"inProgress"|"completed"|"deleted"`, `active_step_ids`,
`sort_field`/`sort_direction`, `date_from`/`date_to`. The response is an
envelope `{items, returned, skip, limit, hasMore, nextSkip}` — loop on
`nextSkip` until `hasMore` is false. Rows are projected; `get_work(id)` for the summary,
`get_work(id, format="full")` for the full record.
