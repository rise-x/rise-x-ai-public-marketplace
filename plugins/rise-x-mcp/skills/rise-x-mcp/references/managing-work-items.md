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

**Do not** call `list_work` and iterate the result client-side to find matches. If there's a filter in the user's question (e.g. "open works", "works modified last week", "works where data.priority is High"), the right tool is `search_works`, not `list_work`.

## Tools

### `get_work(id: str, format: str = "summary")`

The summary view adds an `omitted` note listing the dropped top-level keys that carried a value (typically `steps`, `users`, `chains`, `cardLayout`, `companies`, `company`, `lastModifiedTicks`, `schemaVersion`, `workStateColor`). Pass `format="full"` when you need one of them.

### `list_work(flow_origin_id, …)`

- Read the origin id from `list_flows()`, from any work's `flowOriginId` field, or from `get_flow_config(flow_id).flowOriginId`. `get_flow_config` and `get_flow_steps` take the published `flow_id`; both ids look like GUIDs, so the tool cannot tell them apart.
- The response is `{items, returned, skip, limit, hasMore, nextSkip}`. Loop on `nextSkip` until `hasMore` is false.
- Each row carries `id`, `workCode`, `displayName`, `flowId`, `flowOriginId`, `flowDisplayName`, `activeStepId`, `status`, `state`, `statusLabel`, `statusColor`, `canEdit`, `canDelete`, `canDelegate`, `roleName`, `createdBy` / `lastModifiedBy` (each `{id, email, displayName}`), `createdDate`, and `lastModifiedDate`. Date field names differ by tool — see `references/common-pitfalls.md` pitfall #65.
- For form data, status and actions, call `get_work(id)`; for `cardLayout`, per-role user lists, `chains`, or `steps`, call `get_work(id, format="full")`.

### `update_work_data_bulk(id, fields, section_name, response_format="summary")`

It wraps the v4 batch endpoint (`PATCH /api/v4/work/{id}/data/batch`): one write request whatever the field count, including the first write to a brand-new work item. The tool also issues two reads: one before the write to resolve ids, one after it to verify.

**Not every server has this release.** Environments upgrade independently of marketplace releases. If the server reports no such tool, read that as *not supported here* — not as a bad id or a permissions problem — and fall back to per-field `update_work_data` calls, run sequentially.

**Never pass a parent path and one of its own descendants in one call** (`{"$.vessel": {…}, "$.vessel.name": "x"}`). The two ops overlap, and the order the backend applies them is unspecified, so the outcome is undefined. One op per leaf path, always.

> **`section_name` is more forgiving here than on `update_work_data`.** This tool resolves the
> name against the work's own step tree — depth-first, case-insensitive, first match wins —
> trying the internal names `name` / `taskName` / `stepName` first, then the labels
> `displayName` / `taskDisplayName` / `stepDisplayName`. Internal names go first deliberately,
> so a label colliding with another task's internal name cannot win.
>
> **So either form works, and the `get_flow_step` round-trip is optional:** the
> `taskDisplayName` that `get_flow_steps` already returns resolves, even though `taskName` is
> projected out of that response. `section_not_found` lists both forms per task, so a wrong
> name is recoverable without a second lookup.
>
> **`update_work_data` has none of this** — it forwards the value to the v3 endpoint verbatim
> and needs the internal `taskName`, which does require `get_flow_step`.

Other error codes it can return before writing anything: `validation` (empty or non-dict
`fields`), `origin_unresolved` (the work carries no `flowOriginId` — the usual cause is
passing an **entity** id where a work id belongs, cf. pitfall #7) and `unexpected_response`
(the pre-write read of the work came back as a non-object; retry). The tool never makes a
second write attempt, so there is no partial state of its own making. An all-`set` batch is
idempotent, so re-sending the identical call after a `transient` error is safe.

> **Older API builds: `400: Object reference not set to an instance of an object` on a new
> work item.** Before a backend fix, the batch endpoint could not create a work item's data
> record, so the **first** bulk write to a work item nothing had been written to failed with
> that bare 400 — every time, on every fresh item. Current builds create the record. If you
> do hit that exact 400 on a fresh item, the environment predates the fix: write **one**
> field with `update_work_data` (which does create the record), then re-send the rest with
> `update_work_data_bulk`. Any other 400 is a real failure — read it.

**Reading the result.** Besides `dropped_value`, `unverified_writes`, and `no_verification`, a
path can come back with the general echo-diff codes `value_differs`, `dropped_property`, or
`dropped_item` when the stored value only partly matches the request. The envelope also
carries `workId`, `sectionId`, and `originId`.

**A `value_differs` path counts as unpersisted here.** The verifier treats *any* diff as
"did not land": the path is left out of `changed`, `counts.persisted` drops, and the
`unverified_writes` rollup fires. That is deliberately stricter than the `SKILL.md` rule that
`value_differs` is informational — a batch op is a full-value `Set`, so a stored value that
differs is indistinguishable from one that was never applied. The comparison is already
tolerant of harmless normalisation (`4` vs `4.0`, surrounding whitespace, GUID case), so a
`value_differs` here means the server stored something genuinely different — a reformatted
date, a rewritten list. Read the warning to tell the two apart — `value_differs` carries
`requested` and `actual`, so you can see what the server chose, whereas `dropped_value` means
the old value is still sitting there.

**What to do with each**, since `SKILL.md` rule 1 otherwise reads as "any warning means
failure":

| Warning | What it means | What to do |
|---|---|---|
| `value_differs` | The write reached the field, but the stored value is not the one you sent | **Compare `requested` against `actual`.** Cosmetic difference (formatting only — e.g. the same date in another format) → accept it. Semantic difference — a date read as a different day, a list that came back reordered or short — → the **value or its type** is wrong, so fix the value. Either way, do not re-send the same value: it will be transformed identically |
| `dropped_value` | The value is not there; the old one still is | Fix the **path**, not the call. Retrying the same path is equally futile |
| `unverified_writes` | Rollup: `persisted < requested` | Read the per-path warnings above it; it adds no information of its own. Expected whenever the call includes a `null`, since a `null` is never counted as persisted |
| `no_verification` (no `path`) | The read-back failed; nothing is known | Call `get_work(id)` |
| `no_verification` (with a `path`) | You wrote `null` there. A stored null and an absent path read back identically, so the write can be neither confirmed nor refuted, and it is **never** counted in `changed` / `counts.persisted` | To clear a field, send `""` / `[]` / `{}` instead — those are verified. If it must be `null`, confirm with `get_work(id)` |

A `persisted` below `requested` is therefore not, on its own, grounds for reporting failure —
but it is never grounds for reporting success either. Look at what each warning names first: the
verifier's tolerances mean a `value_differs` has already survived the harmless cases, so treat
it as a real divergence until you have compared the two values and seen otherwise.

**`changed` absent is not `changed: []`.** When the read-back fails, `changed` is **omitted**
and `counts` carries `requested` only — verification did not run, so nothing is known about
what landed. An empty `changed` with no per-path `no_verification` is the opposite claim: verification ran
and confirmed nothing persisted. Never read an absent `changed` as "nothing persisted".

Verification is `set`-strict: clearing a field with `""` / `[]` / `{}` is confirmed only if the
stored value really is empty, and a list must read back with the requested number of entries.

### `update_work_data(id, json_path, operation, value, section_name)`

- `section_name` is the task's internal `taskName`: slash form like `"UntitledTask/Generated-<guid>"`, or bare like `"Task_1"`. `get_flow_steps` projects `taskName` **out**, so fetch it with `get_flow_step`. A name matching no task fails as a backend error here; `update_work_data_bulk` catches the same mistake client-side.
- Writing several fields this way costs one PATCH each, and they must run **sequentially**: parallel calls answer with `Cannot connect to host`. A drop mid-sequence leaves the work half-populated with no signal about which fields landed.

### `submit_work(id, event_name, step_name, invitation=None)`

Leave `invitation` as `None` unless you are deliberately supplying recipients, or passing the invitation a previous call returned. A non-null invitation stops the server deriving recipients from the flow config.

## Creating a New Work Item

```
1. create_work(flow_id)                      # start workflow → get workId, stepName, eventName
2. update_work_data_bulk(workId,             # fill in every field in ONE request
     {"$.task.field": "value",
      "$.task.other": 42}, section_name)
3. submit_work(workId, eventName, stepName)  # advance to next step
```

Both writers require a `section_name`, but they differ in what they accept: `update_work_data_bulk` resolves either the internal name or the display label (so `taskDisplayName` from `get_flow_steps` is enough), while `update_work_data` needs the internal `taskName` (slash form like `UntitledTask/Generated-<guid>` on v3-style flows, or bare like `Task_1` on v4 native flows). To read `taskName`, call `get_flow_step(flow_id, step_id)` with the `id` field from `get_flow_steps` as `step_id`.

## Progressing an Existing Work Item

```
1. search_works / list_work(flow_origin_id)  # find items (pass the ORIGIN id, not the published flow id)
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
- `event_name` and `step_name` must match exactly what the flow expects — get them from the
  `create_work`, `submit_work`, or `get_work` response
