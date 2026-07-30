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

## Creating a New Flow

### `create_flow(flow_resource_type, update_flow_flags)`

Creates a new empty draft flow from scratch. This is the entry point for creating both workflows and asset types.

**Args:**
- `flow_resource_type` (str, default `"Work"`) — Type of flow:
  - `"Work"` — standard multi-step workflow
  - `"Entity"` — asset type (ThingType)
  - `"User"` — user-related flow
  - `"Company"` — company-related flow
- `update_flow_flags` (str, default `"AddAll"`) — What scaffolding to auto-create:
  - `"AddAll"` — creates a step + layout + default section + default columns (recommended)
  - `"None"` — empty flow, no scaffolding
  - `"AddSteps"` — adds a default step only
  - `"AddLayouts"` — adds layouts to existing steps
  - `"AddDefaultColumns"` — adds default data-grid columns
  - `"AddDefaultLayoutSection"` — adds a default section to layouts

**Returns:** The new draft flow object with its ID, properties, and steps.

**NOTE:** The returned flow is already in draft mode. You do NOT need to call `create_flow_draft` after `create_flow`. Use `create_flow_draft` only when editing an already-published flow.

## Creating an Asset Type — End-to-End Sequence

```
Phase 1: Create the Entity Flow
  flow = create_flow(flow_resource_type="Entity", update_flow_flags="AddAll")
  # Extract: flow ID from response

Phase 2: Set Properties
  update_flow_properties(id=flowId, properties={
    "entityType": "vessel",
    "displayName": "Vessel",
    "description": "Maritime vessel registry"
  })

Phase 3: Configure the Layout
  # Get the auto-created step and layout IDs
  steps = get_flow_steps(flow_id=flowId)
  # Extract: layoutId from the first step

  # Add components to the layout's default section
  # (get_flow_steps returns layoutId; get_layout(..., format="components") returns sections)
  layout = get_layout(layout_id=layoutId, format="components")
  # Find the default section ID

  add_components(layout_id=layoutId, parent_section_id=sectionId, components=[
    {"component": "input-text", "label": "Vessel Name"},
    {"component": "input-text", "label": "IMO Number"},
    {"component": "input-select", "label": "Flag State", "properties": {
      "options": ["Panama", "Liberia", "Marshall Islands"]
    }},
    {"component": "input-text", "label": "Tonnage"}
  ])

Phase 4: Publish
  publish_flow(id=flowId)
```

After publishing, the asset type appears in `search_flows` (flowResourceType=Entity) and `list_asset_types()`, and users can create asset instances with `create_asset(flow_origin_id)`.

## Creating a Workflow — Same Tool

```
flow = create_flow(flow_resource_type="Work", update_flow_flags="AddAll")
# Then: update_flow_properties, add steps/tasks, configure layouts, publish
```

See `references/building-workflows.md` for full workflow construction guide.

## Key Entity Flow Properties

Set these with `update_flow_properties(id, properties)`:

| Property | Type | Description |
|---|---|---|
| `entityType` | string | The ThingType identifier (e.g. `"vessel"`, `"contract"`). This is how the asset type is referenced in search-things components and API calls. |
| `displayName` | string | Human-readable name shown in the UI (e.g. `"Vessel"`) |
| `description` | string | Description of the asset type |
| `icon` | object | Icon configuration: `{"shape": "DirectionsBoatOutlined", "color": "#2196F3"}` |
| `displayNameTemplate` | string | Template for asset instance display names (see below) |

### `displayNameTemplate` — Asset Instance Naming

Controls how each asset instance's display name is generated. Uses **JSONPath expressions** that are resolved against the asset's data at runtime.

**Token format:** `{$.path.to.field}` — each token is a JSONPath starting with `$.` that points into the asset's data object. Optional format specifier may follow a colon: `{$.path:format}` (e.g. `:yyyy-MM-dd` for dates). Literal text (separators, dashes, parentheses) is preserved between tokens.

**Examples (from real flows):**
```
"{$.vessel.displayName}"
    → "Pacific Explorer"

"{$.vessel.displayName} - {$.inquiry.deliveryWindow.from.date:yyyy-MM-dd}"
    → "Pacific Explorer - 2026-04-15"
```

Other valid shapes:
```
"{$.companyName}"                                        → "Acme Corp"
"{$.firstName} {$.lastName} ({$.email})"                 → "Jane Doe (jane@acme.com)"
"{$.product.name} ({$.product.grade})"                   → "Iron Ore (62% Fe)"
```

**Setting it:**
```
update_flow_properties(flow_id, {
  "displayNameTemplate": "{$.vessel.displayName} - {$.imoNumber}"
})
```

**How to find paths:** The path must resolve against the asset's stored data shape, not the component label. Check `get_flow_config` / `get_layout` and any linked `search-things` `dataPaths` to see where values actually land. For example, a `search-things` component for a vessel typically stores data under `$.vessel.displayName`, not `$.vesselName`.

**Without a template:** If `null` (the default), the asset falls back to the static `displayName` on the flow.

## Modifying an Existing Asset Type

Published asset types are modified using the standard draft/publish lifecycle:

```
# 1. Create a draft of the existing flow
draft = create_flow_draft(id=publishedFlowId)
# CRITICAL: use the returned draft ID for all subsequent operations

# 2. Make changes (add/remove/update components, rename steps, etc.)
# Use the draft flow ID and draft layout IDs

# 3. Publish
publish_flow(id=draftFlowId)
```

See `references/managing-flows.md` for the full draft/edit/publish sequence and the Draft ID Rule.

## Deleting an Asset Type

```
delete_flow(id=flowId)
```

**WARNING:** This deletes the asset type definition. Existing asset instances may become orphaned. Use with caution.

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
# Returns: [{"stepDisplayName": "Step", "layoutId": "layout-456", ...}]

# 4. Get layout to find default section
get_layout("layout-456", format="components")
# Find the section ID from the components list

# 5. Rename the step to something meaningful
rename_step(flow_id="abc-123", step_id="step-id", new_name="Customer Details")

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

# 7. Publish
publish_flow("abc-123")

# Now create a customer instance:
# find the "customer" asset type's flowOriginId via search_flows (filter: flowResourceType == Entity)
# create_asset(flow_origin_id)  -> get workId
# update_work_data(workId, "$.customerDetails.companyName", "set", "Acme Corp")
# submit_work(workId, "Submit", "customerDetails")
```

## Common Mistakes

1. **Calling `create_flow_draft` instead of `create_flow`** — `create_flow` creates a brand new flow; `create_flow_draft` creates an editable draft of an *existing* published flow
2. **Forgetting to set `entityType`** — without it, the flow won't appear in `list_asset_types()` and can't be used to create assets
3. **Calling `create_flow_draft` after `create_flow`** — the flow from `create_flow` is already in draft mode; double-drafting creates orphan drafts
4. **Using `create_asset` to create an asset type** — `create_asset` creates an instance (record), not a type (template). Use `create_flow(flow_resource_type="Entity")` for types.
5. **Forgetting to publish** — the asset type won't appear in `search_flows` (flowResourceType=Entity) or `list_asset_types()` until published
