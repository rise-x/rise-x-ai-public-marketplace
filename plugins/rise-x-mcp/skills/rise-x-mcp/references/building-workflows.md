# Building Workflows

This is the comprehensive guide for constructing a flow from scratch, including orchestrated agent workflow for building from user requirements.

## Contents

- [Orchestration Flow](#orchestration-flow)
- [Two Flow Types](#two-flow-types)
  - [Work Flow (multi-step process)](#work-flow-multi-step-process)
  - [Asset Flow (single-step entity)](#asset-flow-single-step-entity)
- [Multi-Party Workflow Design Principles](#multi-party-workflow-design-principles)
  - [Task = Party Handover](#task--party-handover)
  - [Step = Process Phase](#step--process-phase)
  - [Section = Logical Grouping](#section--logical-grouping)
  - [Auto-Notifications](#auto-notifications)
  - [Example — Marine Bunkering (3-party workflow)](#example--marine-bunkering-3-party-workflow)
  - [Example — Asset Type: Vessel](#example--asset-type-vessel)
  - [Component Selection Strategy](#component-selection-strategy)
- [End-to-End Construction Sequence](#end-to-end-construction-sequence)
  - [Phase 1: Flow Structure](#phase-1-flow-structure)
  - [Phase 2: Layout & Components (for each task)](#phase-2-layout--components-for-each-task)
  - [Phase 3: Actions (for multi-party or branching flows)](#phase-3-actions-for-multi-party-or-branching-flows)
  - [Phase 4: Grid Columns (optional but recommended)](#phase-4-grid-columns-optional-but-recommended)
  - [Phase 5: Publish](#phase-5-publish)
- [Component Hierarchy Rule (Critical)](#component-hierarchy-rule-critical)
- [Minimum Component Dict](#minimum-component-dict)
- [Building an Asset Flow](#building-an-asset-flow)
- [Discovering Component Properties](#discovering-component-properties)
- [Tips](#tips)

## Orchestration Flow

When the user asks to **build a new workflow or asset flow from scratch**, follow this phased approach:

```
User: "build a new inspection workflow" or "create an asset flow for vessels"
        |
        +-- Phase 1: ANALYZE
        |     Gather requirements from the user:
        |       - Flow purpose and description
        |       - Steps needed (for work flows) or single-step structure (for asset flows)
        |       - Fields per step/task (what data needs to be captured)
        |       - Parties involved (who does what)
        |       - Decision points: which steps need Reject/Send Back/Approve?
        |       - Key data to surface in grid view (columns)
        |     Present structured outline to the user for approval.
        |     GATE: Wait for user approval before proceeding.
        |
        +-- Phase 2: BUILD STRUCTURE
        |     1. create_flow_draft(flow_id)
        |     2. update_flow_properties(flow_id, {displayName, description, icon})
        |     3. add_step(flow_id, position, step_name, task_name)  -- repeat per step
        |     4. get_flow_steps(flow_id)  -- capture stepIds, taskIds, layoutIds
        |
        +-- Phase 3: BUILD LAYOUTS
        |     For each task (using layoutId from get_flow_steps in Phase 2):
        |     5. update_component(layoutId, sectionId, {label, properties})  -- label the section
        |     6. add_components(layoutId, [{fields...}], parent_section_id)  -- add fields
        |     Repeat for each task's layout.
        |     NOTE: DO NOT call create_layout_draft here. create_flow_draft already
        |     created layout drafts for all tasks. The layoutIds from get_flow_steps
        |     ARE the draft layout IDs. Calling create_layout_draft creates orphan
        |     drafts that won't be published by publish_flow.
        |
        +-- Phase 4: CONFIGURE ACTIONS (if multi-party or branching)
        |     For steps that need more than a simple "Submit":
        |     7. get_flow_step(flow_id, step_id)  -- find default action ID
        |     8. manage_action(flow_id, step_id, "update", action_id=..., action_data={...})  -- customize Submit
        |     9. manage_action(flow_id, step_id, "add", action_data={...})  -- add Reject/Send Back/etc.
        |     See "Designing Actions for Multi-Party Steps" below.
        |
        +-- Phase 5: CONFIGURE GRID COLUMNS (optional)
        |     Expose key data fields in the work grid/kanban view:
        |     10. manage_columns(flow_id, "add", column={displayName, key, valuePaths, ...})
        |     See "Configuring Grid Columns" below.
        |
        +-- Phase 6: PUBLISH
        |     11. publish_flow(flow_id)  -- publishes flow + all layouts
        |     12. get_flow_config(flow_id)  -- verify published state
```

## Two Flow Types

### Work Flow (multi-step process)
Standard business process with multiple steps, each representing a phase.
Example: Inspection -> Review -> Approval -> Closure

### Asset Flow (single-step entity)
Single step for creating/editing master data entities (companies, vessels, products).
After building and publishing, the flow appears in `search_flows` (flowResourceType=Entity) and `list_asset_types()`.

## Multi-Party Workflow Design Principles

Rise-X workflows are **multi-party permissioned forms**. Tasks represent discrete units of work assigned to specific parties. Steps group related, consecutive tasks into high-level phases. Understanding these concepts is critical for well-designed workflows.

### Task = Party Handover

A task is a unit of work that can be completed by the same party (or set of parties) before handing off to the next. All components in a task can be completed before submission triggers a handover.

**Golden rule:** Don't create consecutive tasks intended for the same party. Merge them into a single task with multiple sections instead.

```
GOOD: task1(PartyA) → task2(PartyB) → task3(PartyA)  # alternating parties
BAD:  task1(PartyA) → task2(PartyA) → task3(PartyB)  # merge tasks 1&2

GOOD: task1(PartyA, 2 sections) → task2(PartyB)       # sections group PartyA's work
BAD:  task1(PartyA, 1 section) → task2(PartyA, 1 section) → task3(PartyB)
```

### Step = Process Phase

Steps are Kanban columns that represent distinct phases. Tasks within a step must be consecutive.

**Guidelines:**
- Prefer fewer than 4–6 steps unless clearly justified by distinct process phases
- Prefer fewer than 4 tasks per step
- Step and task names must be unique within the flow

### Section = Logical Grouping

Sections group related components within a task (e.g. "Order Details", "Shipping Details"). Use sections instead of extra tasks when the same party fills in logically distinct groups of fields.

### Auto-Notifications

After each task submission, the platform automatically sends an email to the party responsible for the next task. You do not need to design explicit notification steps or tasks.

### Example — Marine Bunkering (3-party workflow)

```
Parties: Ship Owner, Bunker Supplier, Port Authority

Step 1: Pre-Arrival Planning
  Task: Ship Owner arranges fuel delivery (Ship Owner)
    Actions: Submit, Cancel Order (→ TerminateFlow, red)
  Task: Supplier coordinates with Port Authority (Bunker Supplier)
    Actions: Confirm (→ Next), Decline (→ "Pre-Arrival Planning", red)

Step 2: Arrival at Port
  Task: Ship arrives at bunkering location (Ship Owner)
    Actions: Submit
  Task: Port Authority inspects and verifies documentation (Port Authority)
    Actions: Approve (→ Next, green status "Cleared"),
             Reject (→ "Pre-Arrival Planning", red status "Documentation Rejected")

Step 3: Bunkering Operation
  Task: Supplier delivers fuel (Bunker Supplier)
    Actions: Submit
  Task: Ship crew monitors transfer and safety (Ship Owner)
    Actions: Confirm Complete (→ Next),
             Report Issue (→ "Bunkering Operation", warning, stays in same step)

Step 4: Post-Bunkering
  Task: Ship crew completes bunkering log (Ship Owner)
    Actions: Submit
  Task: Supplier issues invoice (Bunker Supplier)
    Actions: Submit Invoice (→ Next)

Grid Columns (key data to surface):
  - Vessel Name ($.preArrivalPlanning.vesselName)
  - Supplier ($.preArrivalPlanning.supplierName)
  - Port ($.preArrivalPlanning.portName)
  - Fuel Type ($.preArrivalPlanning.fuelType)
  - Quantity ($.preArrivalPlanning.quantity)
  - Delivery Date ($.preArrivalPlanning.deliveryDate)
```

**Action design rationale:** Port Authority's Approve/Reject on documentation inspection is the critical gate — rejection routes all the way back to planning. The Ship Owner's "Report Issue" during bunkering uses `Self` routing so the step stays active while the issue is handled. Cancel Order is available early to abort before resources are committed.

### Example — Asset Type: Vessel

```
Asset: Vessel
Purpose: Represents a ship or marine vessel in operational workflows.

Sections:
  1. Identification
     - Vessel Name (text, required)
     - IMO Number (text, required)
     - Call Sign (text)
     - Flag State (select, required)
  2. Vessel Characteristics
     - Vessel Type (select: tanker, bulk carrier, container ship)
     - Deadweight Tonnage (number)
     - Length Overall (number, meters)
  3. Ownership & Contacts
     - Owner Company Name (text, required)
     - Technical Manager (text)
     - Primary Contact Email (email, required)
```

### Component Selection Strategy

Always choose the best-fit component type for each field:

| Component | Use for |
|---|---|
| `input-text` | Short text or numeric inputs (set `inputType: "number"` for numbers) |
| `input-select` | Single/multi-select from predefined options |
| `date-picker` | Date, date-range, or date-time inputs |
| `user-invitation` | Selecting users or groups (use `defaultValue: "initiator"` for current user) |
| `search-things` | Searching and selecting assets/entities |
| `attachments` | File uploads and documents |
| `data-grid` | Tabular data with multiple rows and columns |
| `richtext-input` | Long-form text with formatting |
| `signature` | Handwritten signature capture |
| `finance-section` | Financial data with automatic calculations |
| `banner` | Informational/instructional content (should not be sole component in section) |
| `product-toggle-switch` | Boolean on/off inputs |

See `references/layouts-and-components.md` for full components list

**Prefer data-grid over repeated fields.** Instead of "Name 1", "Address 1", "Name 2", "Address 2", use a single data-grid with "Name" and "Address" columns.

## End-to-End Construction Sequence

### Phase 1: Flow Structure

**New flow:** Use `create_flow()` to create from scratch — it returns a draft flow already in draft mode.
**Existing flow:** Use `create_flow_draft(published_flow_id)` — it returns a **new draft flow ID**.

**CRITICAL:** All subsequent operations MUST use the draft flow ID returned by `create_flow` or `create_flow_draft`.

```
1. flow = create_flow("Work", "AddAll")                # new flow — already in draft mode
   # OR: draft = create_flow_draft(existing_id)         # existing flow — capture draft ID
2. update_flow_properties(flow_id, {
     "displayName": "Vessel Inspection",
     "description": "Standard vessel inspection workflow",
     "icon": {"shape": "SearchOutlined", "color": "#2196F3"}
   })
3. add_step(flow_id, 0, "Inspection", "Inspection Details")
4. add_step(flow_id, 1, "Review", "Review Task")
5. add_step(flow_id, 2, "Approval", "Approval Task")
6. get_flow_steps(flow_id)   # capture layoutIds for each task
```

Response from `get_flow_steps` provides the layoutId for each task — you need these for Phase 2.

**ID Mapping Table — Which ID to use for each tool:**

Every `get_flow_steps` row carries several GUIDs (`id`, `stepId`, `taskId`,
`actionSetId`, `layoutId`). As of server 1.2.0 most step tools accept ANY of
the row's GUIDs and resolve it for you:

| Tool | ID parameter | Where to find it |
|---|---|---|
| `manage_action`, `manage_task_invites`, `delete_step`, `rename_step` | any of `id` / `stepId` / `taskId` (resolved server-side) | `get_flow_steps` |
| `rename_step(..., target="task")` in a step with several tasks | the row's `id` or `taskId` (a shared `stepId` is ambiguous — the server rejects it with the row list) | `get_flow_steps` |
| `get_flow_step` | the record `id` field specifically | `get_flow_steps` → `id` |
| `move_step` | `stepId` field (steps) / `taskId` field (tasks) — move is the ONE endpoint keyed on these | `get_flow_steps` |
| Layout operations | `layoutId` | `get_flow_steps` → `layoutId` |

If a step tool can't match the GUID you passed, the error now lists the
flow's steps (`steps: [{id, stepName, stepDisplayName}]`) — pick from there
instead of re-fetching.

### Phase 2: Layout & Components (for each task)

**CRITICAL — Do NOT call `create_layout_draft` here.** When `create_flow_draft` runs, it automatically creates draft copies of all associated layouts. The `layoutId` values from `get_flow_steps` ARE already draft IDs. Calling `create_layout_draft` on them creates **orphan drafts** that are NOT linked to the flow — `publish_flow` will publish the flow's own (empty) layout drafts, not your orphans.

```
# Each layout already has a default empty section. Get its ID from the response
# of get_flow_steps or get_layout(layoutId, format="components").

# Update the existing section label:
7.  update_component(layoutId_from_step1, sectionId, {
      "label": "Vessel Information",
      "properties": {"showTitle": true}
    })

# Add field components into the section (use appropriate col widths for a presentable layout):
8.  add_components(layoutId_from_step1, [
      {"component": "input-text", "label": "Vessel Name", "properties": {"width": "col-6"}},
      {"component": "input-text", "label": "IMO Number", "properties": {"width": "col-6"}},
      {"component": "date-picker", "label": "Inspection Date", "properties": {"width": "col-6"}},
      {"component": "input-select", "label": "Vessel Type", "properties": {"width": "col-6"}},
      {"component": "richtext-input", "label": "Findings"},
      {"component": "attachments", "label": "Photos"}
    ], parent_section_id=section_id)

# Repeat steps 7-8 for each task's layout
```

**NOTE:** The dropdown component is `input-select`, NOT `select`. Using `select` creates a component with null properties. Always use `"options": ["A", "B"]` (string array only). Do NOT use `selectOptions` or put objects in `options` — see `references/layouts-and-components.md` for details.

### Phase 3: Actions (for multi-party or branching flows)

Steps with decision points need more than the default "Submit" button. Think about where a party might reject, send back, or cancel work.

**When to add custom actions:**
- A reviewer can approve or reject → add Reject action routing back to the requester's step
- A party can request changes → add Send Back action routing to the previous step
- Any step where work can be cancelled → add Cancel action with `TerminateFlow`
- You want distinct status labels in the grid → set `completedName`/`completedColor` on actions

**How to configure:**

```
# 1. Get the step details to find the default Submit action ID
step_details = get_flow_step(draft_flow_id, step_id)
submit_action_id = step_details["actions"][0]["id"]

# 2. Customize the default Submit to show "Approve" with a green status
manage_action(draft_flow_id, step_id, "update", action_id=submit_action_id, action_data={
  "displayName": "Approve",
  "completedName": "Approved",
  "completedColor": "success"
})

# 3. Add a Reject action that routes back to the requester step
manage_action(draft_flow_id, step_id, "add", action_data={
  "name": "Reject",
  "displayName": "Reject",
  "eventName": "reject",
  "actionTypeName": "Submit",
  "color": "error",
  "completedName": "Rejected",
  "completedColor": "error",
  "next": [{"nextStep": "orderRequest", "nextStepType": "ByStepName"}]
})
```

**Action design guidelines for multi-party workflows:**

| Step pattern | Recommended actions |
|---|---|
| Requester submits to reviewer | Submit (default) + Cancel (`TerminateFlow`) |
| Reviewer evaluates submission | Approve (`Next`) + Reject (`ByStepName` back to requester's step camelCase name) |
| Reviewer with partial concerns | Approve + Request Changes (`Previous`) + Reject |
| Final sign-off / closure | Complete (`Next`) only |
| Any step where the initiator might abort | Add Cancel with `skipValidation: true` and `color: "error"` |

See `references/actions-and-statuses.md` for full action property reference and routing types.

### Phase 4: Grid Columns (optional but recommended)

**IMPORTANT — Auto-generated columns:** When components are added to layouts, columns are **automatically generated** for most field types. Before manually adding columns:
1. Call `manage_columns(flow_id, "list")` to see what already exists
2. Check for your field's `key` — it's likely already there
3. Only use `manage_columns(flow_id, "add", column={...})` for columns not auto-generated, or to customize `valuePaths`
4. Use `manage_columns(flow_id, "update", ...)` to adjust visibility (`defaultOn: false`) or position on existing columns
5. Adding a column with a duplicate `key` will fail with a 400 error

In practice, the main column work is **hiding secondary columns** (rich text, notes, attachments) and **reordering** the important ones — not creating new ones.

The work grid is the primary view where users track all active work items. By default it shows Status, Display Name, Work Code, Created By, Last Modified, and Assigned Users. Adding **custom columns** that surface key business data makes the grid immediately useful.

**What makes a good column:** data that users need to scan across many work items without opening each one — identifiers, amounts, dates, statuses, names.

```
# Add columns that surface key data from the form fields
manage_columns(draft_flow_id, "add", column={
  "displayName": "Vessel Name",
  "key": "vesselName",
  "valuePaths": ["$.inspectionDetails.vesselName"],
  "defaultOn": true,
  "colPosition": 2
})

manage_columns(draft_flow_id, "add", column={
  "displayName": "Inspection Date",
  "key": "inspectionDate",
  "valuePaths": ["$.inspectionDetails.inspectionDate"],
  "defaultOn": true,
  "colPosition": 3
})
```

**Column design guidelines:**

| Data type | Column tip |
|---|---|
| Key identifiers (order #, vessel name, ref code) | `defaultOn: true`, pin left if critical |
| Dates (due date, inspection date) | `defaultOn: true`, helps users sort by urgency |
| Amounts / totals | `defaultOn: true`, consider pinning right |
| Party names (supplier, buyer) | Useful for filtering in multi-party flows |
| Secondary details (notes, descriptions) | `defaultOn: false` — available but not shown by default |

**`valuePaths` format:** JSON paths starting with `$` that match the form field's `dataPath`. The path follows the pattern `$.{stepCamelCase}.{fieldCamelCase}`. For example, a field "Vessel Name" in step "Inspection Details" has path `$.inspectionDetails.vesselName`.

### Phase 5: Publish

```
10. publish_flow(draft_flow_id)    # publishes flow + all layouts — use DRAFT ID
```

## Component Hierarchy Rule (Critical)

```
Layout
  -> Section  (parentId = "00000000-0000-0000-0000-000000000000")
     -> Fields (parentId = section's ID)
```

- Every non-section component **MUST** have a `parentId` pointing to a section
- Sections use the null GUID as their parentId
- Two ways to handle parenting:
  1. Add section first, then fields with `parent_section_id` parameter
  2. Include section + fields in one `add_components` call (fields auto-parent to first section in batch)

## Minimum Component Dict

```json
{"component": "input-text", "label": "Field Name"}
```
IDs are auto-generated. Use `get_schema("components", "input-text")` to discover all available properties.

## Building an Asset Flow

Asset flows are single-step. After publishing, they appear in `search_flows` (flowResourceType=Entity) and `list_asset_types()`:

```
1. flow = create_flow("Entity", "AddAll")        # already in draft mode
2. update_flow_properties(flow_id, {
     "displayName": "Vessel",
     "description": "Vessel master data",
     "icon": {"shape": "DirectionsBoatOutlined", "color": "#00BCD4"}
   })
3. add_step(draft_flow_id, 0, "Vessel Details", "Vessel Form")
4. get_flow_steps(draft_flow_id)                 # get layoutId (already a draft)
5. update_component(layoutId, sectionId, {"label": "General Information", "properties": {"showTitle": true}})
6. add_components(layoutId, [
     # Use layoutId directly — do NOT call create_layout_draft
     {"component": "input-text", "label": "Vessel Name", "properties": {"width": "col-6"}},
     {"component": "input-text", "label": "IMO Number", "properties": {"width": "col-6"}},
     {"component": "input-select", "label": "Flag State", "properties": {"width": "col-6"}},
     {"component": "date-picker", "label": "Built Date", "properties": {"width": "col-6"}}
   ], parent_section_id=sectionId)
7. publish_flow(draft_flow_id)                   # use DRAFT ID
```

Assets can then be managed via `create_asset`, `edit_asset`, etc. See `references/managing-assets.md`.

## Discovering Component Properties

Before adding complex components, check their schemas:

```
get_schema("components")                       # list all available types
get_schema("components", "search-things")      # see all properties for search-things
get_schema("components", "data-grid")          # see all properties for data-grid
```

See `references/schemas-and-compare.md` for full schema tool details.

## Tips

- **Naming convention:** Display names like `"Quality Review"` auto-derive camelCase names: `qualityReview`
- **Position for add_step:** 0 = first, -1 = last (before End Step)
- **Multiple sections per task:** You can add multiple sections to create logical groupings
- **After building:** Use `get_flow_config(flow_id)` to verify the complete published structure
- **User-invitation default:** Use `"initiator"` (NOT `"@me"`) to auto-populate with the current user. See `references/layouts-and-components.md` for details.
- **Conditional fields:** Use `isHiddenDataPath` with expressions like `"='{$.field}' !== 'Value'"` to show/hide fields based on other field values. See `references/layouts-and-components.md` for the full conditional rendering guide.
- **Actions beyond Submit:** For review, approval, or any branching step, add Reject/Send Back actions. Set `completedName` for distinct status labels in the grid. See `references/actions-and-statuses.md`.
- **Grid columns:** Always add columns for the most-queried fields (identifiers, dates, amounts, party names). Use `valuePaths` matching the form field `dataPath` pattern `$.{stepCamelCase}.{fieldCamelCase}`. See `references/actions-and-statuses.md`.
