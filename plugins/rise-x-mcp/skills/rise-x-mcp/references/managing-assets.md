# Managing Assets (Instances)

> **Asset Type vs Asset:** An **asset type** is a template/schema (e.g. "Vessel") — created with `create_flow(flow_resource_type="Entity")` and managed with flow tools. An **asset** is a record/instance (e.g. "Pacific Explorer") — created with `create_asset` and managed with asset + work tools. For creating or modifying asset types, see `references/managing-asset-types.md`.

## What is an Asset?

An **asset** is a managed entity instance (a specific company, person, product, vessel, contract) created and edited through a **3-step workflow pattern**. The form fields come from the asset type's flow layout.

## Discovering Asset Types

Asset types are flows with `flowResourceType == "Entity"`. **Prefer `search_flows`** to discover them — it is the general path and supports filtering, sorting, and projection:

```
search_flows(filter={"field": "flowResourceType", "operator": "equals", "values": ["Entity"]})
```

Each row gives `displayName`, `flowOriginId`, `id` (the `assetId`), and `entityType` (the `thingType`).

### `list_asset_types()` *(obsolete)*
Prefer `search_flows` (above), which returns the same fields (including `entityType`/`thingType`). Kept as a deduped fallback.

Response: `[{"thingType": "vessel", "displayName": "Vessel", "assetId": "flow-guid", "flowOriginId": "origin-guid"}]`

### `get_asset_type_properties(flow_id: str)`
Get field labels and dataPaths for an asset type. Takes the asset type's flow id (the `id` of a `search_flows` row or the `assetId` from `list_asset_types`, not `flowOriginId`). Do not pass an asset instance id.

Response: `[{"Vessel Name": {"id": "comp-guid", "dataPath": "$.vesselDetails.vesselName"}}]`

## Identifier Mapping (Critical)

| Identifier | What it is | Source | Used in |
|---|---|---|---|
| `flowOriginId` | Identifies the asset type | `search_flows` / `list_asset_types()` | `create_asset`, `list_assets` |
| `assetId` | Internal flow ID | `search_flows` (as `id`) / `list_asset_types()` | `get_asset_type_properties` (as `flow_id`) |
| `entityId` | Specific asset instance | `create_asset` response, `list_assets`, `get_asset` | `get_asset`, `edit_asset`, `delete_asset` |
| `workId` | Draft work item for in-progress create/edit | `create_asset`/`edit_asset` response | `update_work_data_bulk`, `update_work_data`, `submit_work` |

## The 3-Step Pattern

### Creating a New Asset

```
Step 1: Initiate
  response = create_asset(flow_origin_id)
  # Extract: workId, stepName from response
  # NOTE: the eventName returned by create_asset ("Submit") is the DISPLAY name,
  # NOT the actual event_name to pass to submit_work. See Step 3.

Step 2: Set every field value in ONE call
  # CRITICAL: ALWAYS include $.displayName — it is the label shown in the UI
  # list/grid. Without it the asset appears with an empty name.
  update_work_data_bulk(
    id=workId,
    fields={
      "$.displayName": "Pacific Explorer",
      "$.vesselDetails.vesselName": "Pacific Explorer",  # paths from get_asset_type_properties
      "$.vesselDetails.imoNumber": "9876543",
    },
    section_name=taskName  # the task's taskName — see step 3 of the worked
                           # example below; get_flow_steps projects it OUT,
                           # so it comes from get_flow_step
  )
  # Read `changed` and `counts` on the response — they say which paths actually
  # persisted, so no follow-up get_work is needed. A `dropped_value` warning
  # means that path did not land (usually an unmodeled dataPath).
  # For push / pull / rename, use update_work_data instead, one field per call.

Step 3: Finalize
  # create_asset already returned the resolved stepName, eventName and
  # invitation (Common Mistakes #7 and #8) — pass those straight to submit_work.
  # Only if that response lacked them, call get_work(workId, format="full"):
  # this fallback reads steps[], which the default summary view drops.
  # Look inside steps[] for a nested step with displayName: "Submitted" and
  # name: "SubmitUntitledStep/Generated-..." — this full name is what you need
  # for BOTH step_name AND event_name (they are identical).
  #
  # Also build an invitation payload from the "actions" array of the work item
  # so the flow routes correctly. Without invitation, the submit call may
  # silently leave the asset in Draft state.
  submit_work(
    id=workId,
    event_name="SubmitUntitledStep/Generated-...",   # full name, NOT "Submit"
    step_name="SubmitUntitledStep/Generated-...",    # same as event_name
    invitation={
      "destinations": [{
        "actionId": "00000000-0000-0000-0000-000000000000",
        "sourceStep": "Submit",
        "targetStep": "End",
        "description": "Close flow.",
        "toDataPath": [{"partyName": "Owner", "dataPath": "$.invites.owner"}],
        "ccDataPath": [{"partyName": "FlowOwner", "dataPath": "$.invites.flowOwner"}],
        "to": [{"email": "<owner-email>", "partyName": "Owner", "inviteType": "To"}],
        "cc": [],
        "none": [],
        "hasInvites": true,
        "isLocked": false,
        "sourceStepName": "SubmitUntitledStep/Generated-...",
        "targetStepName": "End"
      }]
    }
  )
```

### Editing an Existing Asset

Same pattern, but Step 1 uses `edit_asset`:

```
Step 1: Initiate edit
  response = edit_asset(asset_id=entityId)
  # Extract: workId, stepName, eventName from response

Step 2: Modify fields
  # section_name is the task's taskName — same lookup as the create flow
  # (get_flow_steps → get_flow_step → taskName); it is required here too.
  update_work_data_bulk(id=workId, section_name=taskName,
    fields={"$.vesselDetails.vesselName": "New Name"})

Step 3: Finalize
  submit_work(id=workId, event_name=eventName, step_name=stepName)
```

### `create_asset(flow_origin_id: str)`
Initiates a draft workflow for a new asset. Returns:
```json
{
  "action": "create",
  "entityId": "new-entity-guid",
  "workId": "draft-work-guid",
  "stepName": "vesselDetails",
  "eventName": "Submit",
  "nextSteps": "Use update_work_data_bulk with id='<workId>' to set the field values in one call (include $.displayName so the asset list shows a name; update_work_data for one field, or push/pull/rename), then call submit_work with id='<workId>', step_name='<step>', event_name='<event>'[, invitation=<the returned invitation>] to finalize."
}
```

### `edit_asset(asset_id: str)`
Initiates an edit workflow for an existing asset. Same response structure as `create_asset`.

- `asset_id` — the entity ID of the asset to edit (from `get_asset` or `list_assets`)

## Writing Asset Data

Both writers take `section_name` (the task's `taskName`) and are documented in full in
`references/managing-work-items.md`.

| Tool | Use it for |
|---|---|
| `update_work_data_bulk(id, fields, section_name)` | **The default.** Any number of fields, all `set`, applied in ONE request; reads the values back and reports `changed` / `counts` |
| `update_work_data(id, json_path, operation, value, section_name)` | A single field, or any operation other than `set` |

`update_work_data` operations:

| Operation | Description |
|---|---|
| `"set"` | Create or overwrite a value at the path (also available in bulk) |
| `"push"` | Append to an array at the path (bulk cannot express this) |
| `"pull"` | Remove the field at the path (bulk cannot express this) |
| `"rename"` | Rename the field (value = new name; bulk cannot express this) |

## Reading and Listing Assets

### Choosing the right tool for retrieval

| You need to… | Use |
|---|---|
| Find assets matching ANY filter (status, `entityType`, date range, a `data.*` field value), server-side sort, paging, or a projected field set | **`search_assets`** — see `references/advanced-search.md` § Asset Search |
| Look up a single asset by its entity GUID | `get_asset(entity_id)` |
| List one asset type's assets with simple paging only | `list_assets(flow_origin_id, …)` |

**Do not** call `list_assets` and then filter or sort the result client-side — `search_assets` is a real filter / sort / projection engine, including dynamic `data.*` fields. Every `search_assets` query needs a `flowOriginId` pin on the AND-spine (the asset type's `flowOriginId` from `search_flows` or `list_asset_types`), not just `data.*` ones; for a `data.*` query, discover the valid paths first with `get_flow_data_schema`. `contains` / `endsWith` are Flow- and Company-only — use `startsWith`. Asset `status` is `DianaEntityStatus` (`Open` / `Closed` / `Deleted`), not Work's states.

### `get_asset(entity_id: str, format: str = "summary")`
- `format="summary"` (default) — identity (`id`, `entityId`, `resourceId` — the id of the resource this entity record was created from, from which its `id` is derived; not the app-manifest `resourceId` in `managing-apps.md` — `displayName`, `name`, `code`, `workCode`, `entityType`), state (`currentState`, `previousState`, `workState`, `status`), the active step and flow ids (`activeStepId`, `activeStepName`, `flowId`, `flowOriginId`, `flowDisplayName`), the `canEdit` / `canDelete` / `canClone` / `canDelegate` flags, `revision`, `draftWorkId`, `created`, `lastModified`, `createdBy` / `lastModifiedBy`, `publishStatus` (lifted from the document's `dianaVersion`; the same `DianaPublishStatus` enum as a flow's, see `managing-flows.md`, but an asset record is only ever written as `Draft` or `Published`, and deletion is tracked on `status`, not here; not the asset type's), `relationships`, and the asset's `data`. An `omitted` note lists which of the heavy keys (`workDraft`, `steps`, `flowProperties`, `users`, `companies`, `cardLayout`, `dataMap`, `actions`, `chains`, `attachments`, `layoutProperties`) carried a value; unlike `get_work`'s note it covers only those keys, and this view drops `actions` and `attachments` where `get_work`'s keeps them.
- `format="full"` — the raw document. Pass this when you need something the `omitted` note flagged.

Date fields here are `created` and `lastModified`; other tools spell them differently — see `references/common-pitfalls.md` pitfall #65.

The create/edit flow does not need `full`: `create_asset` / `edit_asset` return the resolved `stepName`, `eventName`, and `invitation` (Common Mistakes #7 and #8).

### `list_assets(flow_origin_id: str, skip: int = 0, limit: int = 50)`
Simple paginated list of one asset type's assets — `flowOriginId` from `search_flows` or `list_asset_types`. Returns the paginated envelope `{items, returned, skip, limit, hasMore, nextSkip}` (loop on `nextSkip` while `hasMore`). For any filter / sort / projection, use `search_assets`, not this.

## Deleting Assets

### `delete_asset(entity_id: str)`
Permanent deletion. Cannot be undone.

## Complete Example: Create a Vessel

```
# 1. Discover the vessel asset type
search_flows(filter={"field": "flowResourceType", "operator": "equals", "values": ["Entity"]})
# Find: {"displayName": "Vessel", "flowOriginId": "abc-123", "id": "def-456", "entityType": "vessel"}

# 2. Get field paths
get_asset_type_properties(flow_id="def-456")
# Returns: [{"Vessel Name": {"dataPath": "$.vesselDetails.vesselName"}}, ...]

# 3. Find the step id, then fetch the full step to get its taskName
steps = get_flow_steps("def-456")  # use assetId (the flow's id), NOT flowOriginId
# Returns: [{id, stepName, stepDisplayName, taskDisplayName,
#            stepId, taskId, layoutId, actionSetId}, ...]
# Note: taskName is NOT in this projection — it's the field we actually need.

step = get_flow_step("def-456", steps[0]["id"])
# Returns the full FlowPocoStep_v4, including taskName.
# For v3-style asset flows, taskName is slash form like "UntitledTask/Generated-<guid>".
# Use step["taskName"] as section_name in the data write below.

# 4. Create the asset
create_asset("abc-123")
# Returns: {workId: "work-789", stepName: "vesselDetails/...", eventName: "Submit"}
# NOTE: eventName "Submit" is a DISPLAY label — don't use it for submit_work.

# 5. Set every field value in one request — ALWAYS include $.displayName
update_work_data_bulk(
    "work-789",
    fields={
        "$.displayName": "Pacific Explorer",
        "$.vesselDetails.vesselName": "Pacific Explorer",
        "$.vesselDetails.imoNumber": "9876543",
        "$.vesselDetails.flagState": "Panama",
    },
    section_name="UntitledTask/Generated-xxx",
)
# Response carries changed: [...all four paths...] and counts: {requested: 4,
# persisted: 4}. Anything less, and the warnings name the paths that did not land.

# 6. Fallback only — step 4's response normally carries the resolved stepName,
#    eventName and invitation (Common Mistakes #7 and #8); use those. If it did not:
get_work("work-789", format="full")
# Inside steps[] find the nested step with displayName: "Submitted" and
# name: "SubmitUntitledStep/Generated-yyy" — that is BOTH step_name and event_name.
# Also grab actions[0].invitation to route to End.

# 7. Finalize
submit_work(
    "work-789",
    event_name="SubmitUntitledStep/Generated-yyy",
    step_name="SubmitUntitledStep/Generated-yyy",
    invitation={...}  # from get_work.actions[0].invitation
)
```

## Common Mistakes

1. **Using `assetId` where `flowOriginId` is expected** — `create_asset` and `list_assets` take `flowOriginId`, not `assetId`
2. **Forgetting to call `submit_work`** — the asset stays in draft forever
3. **Wrong `json_path` format** — must start with `$` (e.g. `"$.vesselDetails.name"`)
4. **Not extracting `workId`/`stepName`/`eventName`** from the create/edit response
5. **Confusing `entityId` with `workId`** — `entityId` is the permanent asset ID; `workId` is the temporary draft work item
6. **Missing `$.displayName`** — UI shows assets by `$.displayName`. If you only set domain fields (like `legalEntityName`), the list view will show a blank name. Always set `$.displayName` explicitly.
7. **Re-deriving the submit `event_name` by hand** — `create_asset`/`edit_asset` now return the **resolved** `stepName` and `eventName` (the nested submit step's full name, e.g. `SubmitUntitledStep/Generated-...`). Pass those straight to `submit_work` for both `event_name` and `step_name`. The `eventName: "Submit"` display label is only a fallback — you no longer need a `get_work()` round-trip to discover the real name.
8. **Dropping the returned `invitation`** — `create_asset`/`edit_asset` also return the `invitation` payload (when the step requires one). Pass it verbatim to `submit_work`; without it the submit may not finalize and the asset stays in Draft. Only fall back to `get_work(workId).actions[0].invitation` if the create/edit response didn't include one.
9. **Passing a `section_name` that matches no task** — asset type flows typically have a single task (e.g. `UntitledTask/Generated-...`), and `section_name` is a **required** parameter on both writers, so it cannot be left out. Getting it *wrong* is the real failure: `update_work_data_bulk` reports code `section_not_found` with the real task names under `tasks`, while `update_work_data` surfaces it as a backend error. Get the name from `get_flow_step` (`get_flow_steps` projects `taskName` out), using the flow's `id`, not its `flowOriginId`.
10. **Writing fields one at a time when `update_work_data_bulk` would do** — a 20-field asset costs 20 sequential PATCHes, and they cannot be parallelised (concurrent writes answer `Cannot connect to host`). Send one `update_work_data_bulk` call instead; fall back to sequential `update_work_data` only for `push` / `pull` / `rename`.
11. **Reading a `create_asset` 403 as a permissions problem** — it almost always means a flow id (or a stale id) was passed instead of a `flowOriginId`. Get the `flowOriginId` from `search_flows` (flowResourceType=Entity) or `list_asset_types`. The server error hint says this too (since 1.2.0).
