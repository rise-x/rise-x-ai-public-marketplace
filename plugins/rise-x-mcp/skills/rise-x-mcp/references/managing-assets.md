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
Get field labels and dataPaths for an asset type. Takes the asset type's flow id (the `assetId` from `search_flows`/`list_asset_types`, not flowOriginId). `asset_id` still works as a deprecated alias — it logs a warning and is not an asset instance id.

Response: `[{"Vessel Name": {"id": "comp-guid", "dataPath": "$.vesselDetails.vesselName"}}]`

## Identifier Mapping (Critical)

| Identifier | What it is | Source | Used in |
|---|---|---|---|
| `flowOriginId` | Identifies the asset type | `search_flows` / `list_asset_types()` | `create_asset`, `list_assets` |
| `assetId` | Internal flow ID | `search_flows` (as `id`) / `list_asset_types()` | `get_asset_type_properties` (as `flow_id`) |
| `entityId` | Specific asset instance | `create_asset` response, `list_assets`, `get_asset` | `get_asset`, `edit_asset`, `delete_asset` |
| `workId` | Draft work item for in-progress create/edit | `create_asset`/`edit_asset` response | `update_work_data`, `submit_work` |

## The 3-Step Pattern

### Creating a New Asset

```
Step 1: Initiate
  response = create_asset(flow_origin_id)
  # Extract: workId, stepName from response
  # NOTE: the eventName returned by create_asset ("Submit") is the DISPLAY name,
  # NOT the actual event_name to pass to submit_work. See Step 3.

Step 2: Set field values (repeat per field)
  # CRITICAL: ALWAYS set $.displayName first — this is the label shown in the UI
  # list/grid. Without it the asset appears with an empty name.
  update_work_data(
    id=workId,
    json_path="$.displayName",
    operation="set",
    value="Pacific Explorer",
    section_name=taskName  # e.g. "UntitledTask/Generated-..." from get_flow_steps
  )

  # Then set all other fields
  update_work_data(
    id=workId,
    json_path="$.vesselDetails.vesselName",   # from get_asset_type_properties
    operation="set",
    value="Pacific Explorer",
    section_name=taskName
  )

Step 3: Finalize
  # Before submitting, call get_work(workId, format="full") to find the
  # actual submit step & event — summary's actions[] already covers most flows (see
  # Common Mistakes #8), but this fallback still needs steps[], which is dropped
  # by the default summary view.
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
  update_work_data(id=workId, json_path="$.vesselDetails.vesselName",
    operation="set", value="New Name")

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
  "nextSteps": "Use update_work_data with id='...' to set field values, then call submit_work..."
}
```

### `edit_asset(asset_id: str)`
Initiates an edit workflow for an existing asset. Same response structure as `create_asset`.

- `asset_id` — the entity ID of the asset to edit (from `get_asset` or `list_assets`)

## update_work_data Operations

| Operation | Description |
|---|---|
| `"set"` | Create or overwrite a value at the path |
| `"push"` | Append to an array at the path |
| `"pull"` | Remove the field at the path |
| `"rename"` | Rename the field (value = new name) |

## Reading and Listing Assets

### Choosing the right tool for retrieval

| You need to… | Use |
|---|---|
| Find assets matching ANY filter (status, `entityType`, date range, a `data.*` field value), server-side sort, paging, or a projected field set | **`search_assets`** — see `references/advanced-search.md` § Asset Search |
| Look up a single asset by its entity GUID | `get_asset(entity_id)` |
| List one asset type's assets with simple paging only | `list_assets(flow_origin_id, …)` |

**Do not** call `list_assets` and then filter or sort the result client-side — `search_assets` is a real filter / sort / projection engine, including dynamic `data.*` fields. Every `search_assets` query needs a `flowOriginId` pin on the AND-spine (the asset type's `flowOriginId` from `search_flows` or `list_asset_types`), not just `data.*` ones; for a `data.*` query, discover the valid paths first with `get_flow_data_schema`. `contains` / `endsWith` are Flow- and Company-only — use `startsWith`. Asset `status` is `DianaEntityStatus` (`Open` / `Closed` / `Deleted`), not Work's states.

### `get_asset(entity_id: str, format: str = "summary")`
- `format="summary"` (default) — identity (`id`, `entityId`, `displayName`, `name`, `code`, `workCode`, `entityType`), state (`currentState`, `previousState`, `workState`, `status`), the active step and flow ids (`activeStepId`, `activeStepName`, `flowId`, `flowOriginId`, `flowDisplayName`), the `canEdit` / `canDelete` / `canClone` / `canDelegate` flags, `revision`, `draftWorkId`, `created`, `lastModified`, `relationships`, and the asset's `data`. Heavy keys that carried a value (`workDraft`, `steps`, `flowProperties`, `users`, `companies`, `cardLayout`, `dataMap`, `actions`, `chains`, `attachments`, `layoutProperties`) are listed in an `omitted` note, the same mechanism as `get_work`, though this view drops `actions` and `attachments` where `get_work`'s keeps them.
- `format="full"` — the raw document, returned directly rather than wrapped in a mutation envelope. Pass this when you need something the `omitted` note flagged.
- `format="slim"` still works as a deprecated alias for `summary` and logs a warning.

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
# Use step["taskName"] as section_name in every update_work_data call below.

# 4. Create the asset
create_asset("abc-123")
# Returns: {workId: "work-789", stepName: "vesselDetails/...", eventName: "Submit"}
# NOTE: eventName "Submit" is a DISPLAY label — don't use it for submit_work.

# 5. Set field values — ALWAYS set $.displayName first, and pass section_name
update_work_data("work-789", "$.displayName", "set", "Pacific Explorer", section_name="UntitledTask/Generated-xxx")
update_work_data("work-789", "$.vesselDetails.vesselName", "set", "Pacific Explorer", section_name="UntitledTask/Generated-xxx")
update_work_data("work-789", "$.vesselDetails.imoNumber", "set", "9876543", section_name="UntitledTask/Generated-xxx")
update_work_data("work-789", "$.vesselDetails.flagState", "set", "Panama", section_name="UntitledTask/Generated-xxx")

# 6. Find the real submit step + event name, and the invitation payload
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
9. **Not passing `section_name` to `update_work_data` for asset fields** — asset type flows typically have a single task (e.g. `UntitledTask/Generated-...`). Without `section_name`, updates may return 500 errors. Get the task name from `get_flow_steps(assetId)` (the flow's `id`, not its `flowOriginId`).
10. **Running update_work_data calls in parallel** — causes concurrency/connection errors. Always call sequentially.
11. **Reading a `create_asset` 403 as a permissions problem** — it almost always means a flow id (or a stale id) was passed instead of a `flowOriginId`. Get the `flowOriginId` from `search_flows` (flowResourceType=Entity) or `list_asset_types`. The server error hint says this too (since 1.2.0).
