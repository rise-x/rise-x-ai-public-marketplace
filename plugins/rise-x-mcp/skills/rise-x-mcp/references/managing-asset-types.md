# Managing Asset Types (and Creating Flows)

## Asset Type vs Asset — Key Distinction

| | Asset Type (ThingType) | Asset (Thing / Entity Instance) |
|---|---|---|
| **What it is** | A **template** that defines the structure and fields for a category of entities | A **record** created from an asset type |
| **Analogy** | Class / Schema / Blueprint | Instance / Record / Object |
| **Example** | "Vessel" asset type with fields: IMO Number, Flag State, Tonnage | "Pacific Explorer" — a specific Vessel with IMO 9876543 |
| **Under the hood** | A Flow with `FlowResourceType = Entity` | A CompanyEntity linked to the asset type's flow |
| **Created with** | `create_flow(flow_resource_type="Entity")` | `create_asset(flow_origin_id)` |
| **Managed with** | Flow tools (draft, properties, steps, layouts, components, publish) | Asset tools (create, edit, get, delete, list) + Work tools |
| **Reference doc** | This document | `references/managing-assets.md` |

**Rule of thumb:** If you're defining *what fields exist*, use flow tools. If you're filling *values into those fields*, use asset + work tools.

`create_flow` creates both workflows (`"Work"`) and asset types (`"Entity"`); see `references/building-workflows.md` for building a workflow. The flow it returns is already a draft, so do not call `create_flow_draft` on it. Set the asset type's properties (`entityType`, `displayName`, `description`, `icon`, `displayNameTemplate`) with `update_flow_properties`; the full key list is `get_schema("flows", "properties")`.

## `displayNameTemplate` — Asset Instance Naming

Controls how each asset instance's display name is generated. Uses **JSONPath expressions** that are resolved against the asset's data at runtime.

**Token format:** `{$.path.to.field}` — each token is a JSONPath starting with `$.` that points into the asset's data object. Optional format specifier may follow a colon: `{$.path:format}` (e.g. `:yyyy-MM-dd` for dates). Literal text (separators, dashes, parentheses) is preserved between tokens.

**Examples (from real flows):**
```
"{$.vessel.displayName}"
    → "Pacific Explorer"

"{$.vessel.displayName} - {$.inquiry.deliveryWindow.from.date:yyyy-MM-dd}"
    → "Pacific Explorer - 2026-04-15"

"{$.firstName} {$.lastName} ({$.email})"
    → "Jane Doe (jane@acme.com)"
```

**How to find paths:** The path must resolve against the asset's stored data shape, not the component label. Check `get_flow_config` / `get_layout` and any linked `search-things` `dataPaths` to see where values actually land. For example, a `search-things` component for a vessel typically stores data under `$.vessel.displayName`, not `$.vesselName`.

**Without a template:** If `null` (the default), the asset falls back to the static `displayName` on the flow.

## Modifying an Existing Asset Type

Published asset types follow the standard draft/publish lifecycle: `create_flow_draft(id=publishedFlowId)`, make every change against the returned draft id and its draft layout ids, then `publish_flow(id=draftFlowId)`. See `references/managing-flows.md` for the full sequence and `SKILL.md` § Draft/Publish Lifecycle for the Draft ID Rule.

## Deleting an Asset Type

`delete_flow(id=flowId)` deletes the asset type definition. Existing asset instances may become orphaned. Use with caution.

## Complete Example: Create a "Customer" Asset Type

```
# 1. Create the entity flow with scaffolding
create_flow(flow_resource_type="Entity", update_flow_flags="AddAll")
# Response includes: {"id": "abc-123", "steps": [{...}], ...}

# 2. Set the entity type and display name
update_flow_properties("abc-123", {
  "entityType": "customer",
  "displayName": "Customer",
  "description": "Customer registry",
  "icon": {"shape": "BusinessOutlined", "color": "#4CAF50"}
})

# 3. Get auto-created step/layout structure
get_flow_steps("abc-123")
# Returns: [{"id": "step-row-id", "stepDisplayName": "Step", "layoutId": "layout-456", ...}]

# 4. Get layout to find default section
get_layout("layout-456", format="components")
# Find the section ID from the components list

# 5. Rename the step to something meaningful
rename_step(flow_id="abc-123", id="step-row-id", new_name="Customer Details")

# 6. Add form fields
add_components(layout_id="layout-456", parent_section_id="section-id", components=[
  {"component": "input-text", "label": "Company Name"},
  {"component": "input-text", "label": "Contact Person"},
  {"component": "input-text", "label": "Email"},
  {"component": "input-text", "label": "Phone"},
  {"component": "input-select", "label": "Industry", "properties": {
    "options": ["Energy", "Maritime", "Agriculture"]
  }},
  {"component": "input-location", "label": "Address"}
])

# 7. Publish — this issues a NEW flow id; "abc-123" is now superseded
publish_flow("abc-123")
#   Re-resolve both ids before using them. A superseded id can still ACCEPT writes,
#   which then land on the dead version (managing-flows.md, the stale-id trap). search_flows with
#   flowResourceType == Entity gives the live row: `id` for step reads,
#   `flowOriginId` for create_asset.
# liveFlowId, flowOriginId = <the live row's id and flowOriginId>

# Now create a customer instance:
# response = create_asset(flowOriginId)     -> workId, and the RESOLVED stepName /
#                                              eventName / invitation — use those verbatim
# sectionName = get_flow_steps(liveFlowId)[0]["taskDisplayName"]
#   update_work_data_bulk resolves the task's displayName as well as its internal
#   taskName, so get_flow_steps alone is enough.
# update_work_data_bulk(workId, {
#     "$.displayName": "Acme Corp",          # ALWAYS set this — the list/grid
#                                            # shows the asset by it
#     "$.customerDetails.companyName": "Acme Corp",
# }, section_name=sectionName)
# submit_work(workId, event_name=response["eventName"], step_name=response["stepName"],
#             invitation=response.get("invitation"))
#   never guess "Submit" — it is a display label (pitfall #21). Pass the returned
#   invitation verbatim; dropping it can leave the asset in Draft (Common Mistakes #6
#   in references/managing-assets.md)
```

## Common Mistakes

1. **Calling `create_flow_draft` after `create_flow`**: the flow from `create_flow` is already in draft mode; double-drafting creates orphan drafts. Use `create_flow_draft` only to edit an existing published flow.
2. **Forgetting to set `entityType`**: without it, the flow won't appear in `list_asset_types()` and can't be used to create assets
3. **Using `create_asset` to create an asset type**: `create_asset` creates an instance (record), not a type (template). Use `create_flow(flow_resource_type="Entity")` for types.
4. **Forgetting to publish**: the asset type won't appear in `search_flows` (flowResourceType=Entity) or `list_asset_types()` until published
