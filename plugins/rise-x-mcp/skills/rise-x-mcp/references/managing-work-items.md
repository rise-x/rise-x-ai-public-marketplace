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
2. `update_work_data_bulk(workId, fields, section_name)` → set every field value in ONE call
   (use `update_work_data` for a single field, or for `push` / `pull` / `rename`)
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

### `update_work_data_bulk(id, fields, section_name, response_format="summary")`
Set **many** work-data fields in ONE request. Prefer this over repeated `update_work_data`
calls whenever writing more than one field — it wraps the v4 batch endpoint
(`PATCH /api/v4/work/{id}/data/batch`), where every entry of `fields` is its own `set` op and
all of them are applied together.

**Not every server has this release.** The tool arrives with the v4 batch endpoint, and
environments upgrade independently of marketplace releases. If the server reports no such
tool, read that as *not supported here* — not as a bad id or a permissions problem — and fall
back to per-field `update_work_data` calls, run sequentially.

**Parameters:**
- `id` — work item GUID
- `fields` — `{json_path: value}`, one `set` op per entry, e.g.
  `{"$.displayName": "MV Aurora", "$.capacityMt": 74000}`. Pass **leaf** paths:
  `{"$.vessel.name": "Aurora", "$.vessel.imo": "9321483"}` sets two fields, whereas
  `{"$.vessel": {"name": "Aurora"}}` is a full-value `set` of the whole `vessel` object and
  **drops every sibling it omits**. For the same reason, never pass a parent path and one of
  its own descendants in one call (`{"$.vessel": {…}, "$.vessel.name": "x"}`): the two ops
  overlap, and which one wins depends on the order the backend applies them, which nothing
  guarantees. One op per leaf path, always. An empty or non-dict `fields` fails with code
  `validation`.
- `section_name` — **required** — the task name the data belongs to, the same value
  `update_work_data` takes — obtain it via `get_flow_step` (`get_flow_steps` projects
  `taskName` out). It is resolved client-side to the
  task id the v4 endpoint authorises against, so a name matching no task fails with code
  `section_not_found` and the real task names listed under `tasks`, rather than as an opaque
  backend 403. It is a **required** parameter on both writers — it cannot be omitted, only
  got wrong.
- `response_format` — `"summary"` (default), or `"full"` to add the raw API response under
  `result`.

Other error codes it can return before writing anything: `origin_unresolved` (the work
carries no `flowOriginId` — the usual cause is passing an **entity** id where a work id
belongs, cf. pitfall #7) and `unexpected_response` (the pre-write read of the work came back
as a non-object; retry). Because the batch is one request, a failure at the write itself
normally means **no** field was written — the error hint says so, and `get_work(id)` confirms
it before you retry. An all-`set` batch is idempotent, so re-sending the identical call after
a `transient` error is safe.

`set` is the only operation the batch endpoint offers. For `push` / `pull` / `rename`, use
`update_work_data`. That is why both tools exist — this one is not a replacement.

**It reads the values back**, which `update_work_data` does not. When `changed` is present no
follow-up `get_work` is needed to find out what persisted — when it is absent, one is the only
way to know (see below):
- `changed` — the paths confirmed stored with the requested value.
- `counts` — `{requested, persisted}`.
- `warnings[]` — `dropped_value` for each path the API accepted but did not store (check the
  flow's data schema with `get_flow_data_schema` for the real path);
  `unverified_writes` as a rollup whenever `persisted < requested`; `no_verification` when the
  read-back itself failed. A path can also come back with the general echo-diff codes
  `value_differs`, `dropped_property` or `dropped_item` when the stored value only partly
  matches the request.
- The envelope also carries `workId`, `sectionId` and `originId`.

**A `value_differs` path counts as unpersisted here.** The verifier treats *any* diff as
"did not land": the path is left out of `changed`, `counts.persisted` drops, and the
`unverified_writes` rollup fires. That is deliberately stricter than the `SKILL.md` rule that
`value_differs` is informational — a batch op is a full-value `Set`, so a stored value that
differs is indistinguishable from one that was never applied. The comparison is already
tolerant of harmless normalisation (`4` vs `4.0`, surrounding whitespace, GUID case), so a
`value_differs` here means the server stored something genuinely different — a reformatted
date, a rewritten list. The practical consequence: such a path lowers `counts.persisted` even
though the write was not lost. Read the warning to tell the two apart — `value_differs` carries
`requested` and `actual`, so you can see what the server chose, whereas `dropped_value` means
the old value is still sitting there.

**What to do with each**, since `SKILL.md` rule 1 otherwise reads as "any warning means
failure":

| Warning | What it means | What to do |
|---|---|---|
| `value_differs` | The write landed; the server stored its own form of the value | Accept it. Do **not** retry — a second write normalises identically, so retrying never converges |
| `dropped_value` | The value is not there; the old one still is | Fix the **path**, not the call. Retrying the same path is equally futile |
| `unverified_writes` | Rollup: `persisted < requested` | Read the per-path warnings above it; it adds no information of its own |
| `no_verification` | The read-back failed; nothing is known | Call `get_work(id)` |

Neither `value_differs` nor a `persisted` below `requested` is, on its own, grounds for
reporting failure to the user.

**`changed` absent is not `changed: []`.** When the read-back fails, `changed` is **omitted**
and `counts` carries `requested` only — verification did not run, so nothing is known about
what landed. An empty `changed` is the opposite claim: verification ran and confirmed nothing
persisted. Never read an absent `changed` as "nothing persisted".

Verification is `set`-strict: clearing a field with `""` / `[]` / `{}` is confirmed only if the
stored value really is empty, and a list must read back with the requested number of entries.

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
- `section_name` — **required** — the task name scoping the data location: the task's `taskName`, slash form like `"UntitledTask/Generated-<guid>"` or bare like `"Task_1"`. `get_flow_steps` projects `taskName` **out**, so fetch it with `get_flow_step` (see below). A name matching no task fails as a backend error here; `update_work_data_bulk` catches the same mistake client-side.

One field per call. Writing several fields this way costs one PATCH each, they must run
**sequentially** (parallel calls answer with `Cannot connect to host`), and a drop mid-sequence
leaves the work half-populated with no signal about which fields landed — so prefer
`update_work_data_bulk` for more than one field. This tool remains the only way to `push`,
`pull` or `rename`: the batch endpoint offers `set` alone.

### `submit_work(id, event_name, step_name, invitation=None)`
Submit work to advance it to the next step in its workflow. Leave `invitation`
as `None` unless you are deliberately supplying recipients — a non-null
invitation stops the server deriving recipients from the flow config.

**Parameters:**
- `id` — work item GUID
- `event_name` — the action event triggering the transition (e.g. `"Submit"`, `"Approve"`, `"Reject"`). Get from the work item's `actions` array.
- `step_name` — the current step name (from `activeStepName`)
- `invitation` — optional dict with routing destinations for the next step

## Creating a New Work Item

```
1. create_work(flow_id)                      # start workflow → get workId, stepName, eventName
2. update_work_data_bulk(workId,             # fill in every field in ONE request
     {"$.task.field": "value",
      "$.task.other": 42}, section_name)
3. submit_work(workId, eventName, stepName)  # advance to next step
```

The `flow_id` is the workflow's ID — find it via `get_flow_config` or from the flow creation step. The `section_name` — taken by `update_work_data_bulk` and `update_work_data` alike — is the task's `taskName` (slash form like `UntitledTask/Generated-<guid>` on v3-style flows, or bare like `Task_1` on v4 native flows). `get_flow_steps` projects `taskName` **out** of its response — fetch it via `get_flow_step(flow_id, step_id)` (pass the `id` field from `get_flow_steps` as `step_id`) and read `taskName` from the full step.

## Progressing an Existing Work Item

```
1. list_work(flow_origin_id)                 # find items in a workflow (pass the ORIGIN id, not the published flow id)
2. get_work(id)                              # inspect details, find stepName and actions
3. update_work_data_bulk(id,                 # fill in every field in ONE request
     {"$.task.field": "value"}, section_name)
4. submit_work(id, event_name, step_name)    # advance to next step
```

## Important Notes

- Every path must start with `$` — `json_path` on `update_work_data`, and every key of
  `fields` on `update_work_data_bulk`
- Task-scoped fields follow `$.{taskCamelCase}.{fieldCamelCase}` — e.g.
  `"$.reviewTask.approved"`, `"$.details.description"`. Root-level fields are one segment
  (`"$.displayName"`), so the two-segment pattern is the common case, not the rule
- `event_name` and `step_name` must match exactly what the flow expects — get them from `get_work` response
- Submitting with wrong event/step names will fail

## list_work pagination & filters (server ≥ Trust & Signal)

`list_work` is paginated and filterable: `limit` (default 50), `skip`,
`status="all"|"inProgress"|"completed"|"deleted"`, `active_step_ids`,
`sort_field`/`sort_direction`, `date_from`/`date_to`. The response is an
envelope `{items, returned, skip, limit, hasMore, nextSkip}` — loop on
`nextSkip` until `hasMore` is false. Rows are projected; `get_work(id)` for the summary,
`get_work(id, format="full")` for the full record.
