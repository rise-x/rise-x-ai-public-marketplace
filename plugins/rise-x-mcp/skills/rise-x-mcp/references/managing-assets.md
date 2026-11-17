# Managing Assets (Instances)

> **Asset Type vs Asset:** An **asset type** is a template/schema (e.g. "Vessel") — created with `create_flow(flow_resource_type="Entity")` and managed with flow tools. An **asset** is a record/instance (e.g. "Pacific Explorer") — created with `create_asset` and managed with asset + work tools. For creating or modifying asset types, see `references/managing-asset-types.md`.

## What is an Asset?

An **asset** is a managed entity instance (a specific company, person, product, vessel, contract) created and edited through a **3-step workflow pattern**. The form fields come from the asset type's flow layout.

## Discovering Asset Types

Asset types are flows with `flowResourceType == "Entity"`. **Prefer `search_flows`** over the obsolete `list_asset_types`:

```
search_flows(filter={"field": "flowResourceType", "operator": "equals", "values": ["Entity"]})
```

Each row gives `displayName`, `flowOriginId`, `id` (the `assetId`), and `entityType` (the `thingType`).

## Identifier Mapping (Critical)

| Identifier | What it is | Source | Used in |
|---|---|---|---|
| `flowOriginId` | Identifies the asset type | `search_flows` / `list_asset_types()` | `create_asset`, `list_assets`, `search_assets` pin |
| `assetId` | Internal flow ID | `search_flows` (as `id`) / `list_asset_types()` | `get_asset_type_properties` (as `flow_id`), `get_flow_steps` |
| `entityId` | Specific asset instance | `create_asset` response, `list_assets`, `get_asset` | `get_asset`, `edit_asset`, `delete_asset` |
| `workId` | Draft work item for in-progress create/edit | `create_asset`/`edit_asset` response | `update_work_data_bulk`, `update_work_data`, `submit_work` |

## The 3-Step Pattern

### Creating a New Asset

```
Step 1: Initiate
  response = create_asset(flow_origin_id)
  # Returns workId plus the resolved stepName, eventName and, when the step
  # routes to a party, invitation. Use them verbatim in Step 3.

Step 2: Set every field value in ONE call
  # ALWAYS include $.displayName. Without it the asset shows an empty name
  # in the UI list/grid.
  update_work_data_bulk(
    id=workId,
    fields={
      "$.displayName": "Pacific Explorer",
      "$.vesselDetails.vesselName": "Pacific Explorer",  # paths from get_asset_type_properties
      "$.vesselDetails.imoNumber": "9876543",
    },
    section_name=sectionName  # bulk accepts the internal taskName OR the task's
                              # displayName, so taskDisplayName from get_flow_steps
                              # is enough. Only update_work_data needs get_flow_step.
  )
  # Storing a value is not proof the path is one the flow reads
  # (pitfall #67), so get the paths from get_asset_type_properties first.

Step 3: Finalize
  submit_work(id=workId, event_name=eventName, step_name=stepName,
              invitation=invitation)   # omit invitation only when none was returned
  # Without the returned invitation the submit may leave the asset in Draft.
  # If Step 1 returned no eventName, its nextSteps says so: call get_work(workId)
  # and read actions[].events[].eventName and actions[].invitation.
```

### Editing an Existing Asset

Same pattern, but Step 1 uses `edit_asset(asset_id=entityId)`. Then `update_work_data_bulk` with the returned `workId`, and `submit_work` with the returned `stepName`, `eventName`, and `invitation`.

## Writing Asset Data

Both writers require `section_name`, and they accept different things: `update_work_data_bulk`
resolves the task's `displayName` as well as its internal `taskName`, `update_work_data` only the
internal name. Use `update_work_data_bulk` by default, and `update_work_data` for a single field
or a `push` / `pull`. Documented in full in `references/managing-work-items.md`.

## Reading and Listing Assets

### Choosing the right tool for retrieval

| You need to… | Use |
|---|---|
| Find assets matching ANY filter (status, `entityType`, date range, a `data.*` field value), server-side sort, paging, or a projected field set | **`search_assets`** — see `references/advanced-search.md` § Asset Search |
| Look up a single asset by its entity GUID | `get_asset(entity_id)` |
| List one asset type's assets with simple paging only | `list_assets(flow_origin_id, …)` |

**Do not** call `list_assets` and then filter or sort the result client-side.

### `get_asset(entity_id: str, format: str = "summary")`

Summary-view details the tool description does not give:

- `resourceId` is the id of the resource this entity record was created from; the record's `id` derives from it. It is not the app-manifest `resourceId` in `managing-apps.md`.
- `publishStatus` is lifted from the document's `dianaVersion`. It uses the same `DianaPublishStatus` enum as a flow's (see `managing-flows.md`), but an asset record is only ever written as `Draft` or `Published`, and deletion is tracked on `status`. It is not the asset type's status.
- The date fields are `created` and `lastModified`; other tools spell them differently (see `references/common-pitfalls.md` pitfall #65).
- An `omitted` note lists which heavy keys (`workDraft`, `steps`, `flowProperties`, `users`, `companies`, `cardLayout`, `dataMap`, `actions`, `chains`, `attachments`, `layoutProperties`) carried a value. Unlike `get_work`'s summary, this view drops `actions` and `attachments`.

## Complete Example: Create a Vessel

```
# 1. Discover the vessel asset type
search_flows(filter={"field": "flowResourceType", "operator": "equals", "values": ["Entity"]})
# Find: {"displayName": "Vessel", "flowOriginId": "abc-123", "id": "def-456", "entityType": "vessel"}

# 2. Get field paths
get_asset_type_properties(flow_id="def-456")
# Returns: [{"Vessel Name": {"id": "...", "dataPath": "$.vesselDetails.vesselName", ...}}, ...]

# 3. Get the task name for section_name
steps = get_flow_steps("def-456")  # use assetId (the flow's id), NOT flowOriginId
# steps[0]["taskDisplayName"] is enough for update_work_data_bulk.
# Only update_work_data needs the internal taskName:
#   get_flow_step("def-456", steps[0]["id"])["taskName"]
#   (slash form like "UntitledTask/Generated-<guid>" on v3-style asset flows)

# 4. Create the asset
r = create_asset("abc-123")
# Returns: {workId: "work-789", stepName: ..., eventName: ..., invitation: ...}

# 5. Set every field value in one request — ALWAYS include $.displayName
update_work_data_bulk(
    "work-789",
    fields={
        "$.displayName": "Pacific Explorer",
        "$.vesselDetails.vesselName": "Pacific Explorer",
        "$.vesselDetails.imoNumber": "9876543",
        "$.vesselDetails.flagState": "Panama",
    },
    section_name=steps[0]["taskDisplayName"],
)
# Expect changed: [...all four paths...] and counts: {requested: 4, persisted: 4}.
# Anything less, and the warnings name each path and why — not all of them mean
# the value was lost (see managing-work-items.md § What to do with each).

# 6. Finalize with the values step 4 returned
submit_work("work-789", event_name=r["eventName"], step_name=r["stepName"],
            invitation=r.get("invitation"))
```

## Common Mistakes

1. **Using `assetId` where `flowOriginId` is expected**: `create_asset` and `list_assets` take `flowOriginId`, not `assetId`. A `create_asset` 403 almost always means this, not a permissions problem.
2. **Forgetting to call `submit_work`**: the asset stays in draft forever.
3. **Confusing `entityId` with `workId`**: `entityId` is the permanent asset ID; `workId` is the temporary draft work item.
4. **Missing `$.displayName`**: UI shows assets by `$.displayName`. If you only set domain fields (like `legalEntityName`), the list view shows a blank name.
5. **Guessing the submit `event_name`**: `"Submit"` is a display label, not an event name. Pass the `stepName` and `eventName` that `create_asset` / `edit_asset` returned.
6. **Dropping the returned `invitation`**: pass it verbatim to `submit_work`; without it the submit may not finalize and the asset stays in Draft.
7. **Passing a `section_name` that matches no task**: `update_work_data_bulk` reports `section_not_found` with the real task names under `tasks`; `update_work_data` surfaces it as a backend error. Asset type flows typically have a single task.
8. **Writing fields one at a time**: a 20-field asset costs 20 sequential `update_work_data` calls that cannot run in parallel. Send one `update_work_data_bulk` call instead.
