# Building Workflows

The guide for building a flow from scratch: the build order, the design rules, and the traps between calls. To change an existing flow, see `references/managing-flows.md`.

## Contents

- [Orchestration Flow](#orchestration-flow)
- [Two Flow Types](#two-flow-types)
- [Multi-Party Workflow Design Principles](#multi-party-workflow-design-principles)
  - [Task = Party Handover](#task--party-handover)
  - [Step = Process Phase](#step--process-phase)
  - [Section = Logical Grouping](#section--logical-grouping)
  - [Auto-Notifications](#auto-notifications)
  - [Example — Marine Bunkering (3-party workflow)](#example--marine-bunkering-3-party-workflow)
- [End-to-End Construction Sequence](#end-to-end-construction-sequence)
  - [Phase 1: Flow Structure](#phase-1-flow-structure)
  - [Phase 2: Layout & Components (for each task)](#phase-2-layout--components-for-each-task)
  - [Phase 3: Actions (for multi-party or branching flows)](#phase-3-actions-for-multi-party-or-branching-flows)
  - [Phase 4: Grid Columns (optional but recommended)](#phase-4-grid-columns-optional-but-recommended)
  - [Phase 5: Publish](#phase-5-publish)
- [Building an Asset Flow](#building-an-asset-flow)

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
        |     1. create_flow(flow_resource_type, update_flow_flags)  -- new flow, already in draft mode
        |     2. update_flow_properties(flow_id, {displayName, description, icon})
        |     3. rename_step the scaffolded first step, add_step for the rest
        |     4. get_flow_steps(flow_id)  -- capture ids, layoutIds
        |
        +-- Phase 3: BUILD LAYOUTS
        |     For each task (using layoutId from get_flow_steps in Phase 2):
        |     5. update_component(layoutId, sectionId, {label, properties})  -- label the section
        |     6. add_components(layoutId, [{fields...}], parent_section_id)  -- add fields
        |     DO NOT call create_layout_draft here (see "Phase 2: Layout & Components" under End-to-End Construction Sequence).
        |
        +-- Phase 4: CONFIGURE ACTIONS (if multi-party or branching)
        |     7. get_flow_step(flow_id, id)  -- find default action ID
        |     8. manage_action(..., "update", ...)  -- customize Submit
        |     9. manage_action(..., "add", ...)  -- add Reject/Send Back/etc.
        |
        +-- Phase 5: CONFIGURE GRID COLUMNS (optional)
        |     10. manage_columns(flow_id, "list"), then adjust
        |
        +-- Phase 6: PUBLISH
        |     11. publish_flow(flow_id)  -- publishes flow + all layouts
        |     12. get_flow_config(<new id from publish_flow>)  -- verify published state
```

## Two Flow Types

- **Work flow (multi-step process):** a business process with several steps, each a phase. Example: Inspection -> Review -> Approval -> Closure.
- **Asset flow (single-step entity):** one step for creating/editing master data entities (companies, vessels, products). After publishing, it appears in `search_flows` (flowResourceType=Entity) and `list_asset_types()`.

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

Sections group related components within a task (e.g. "Order Details", "Shipping Details"). Use sections instead of extra tasks when the same party fills in logically distinct groups of fields. To pick component types, see `references/layouts-and-components.md` § Component Types Reference.

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

## End-to-End Construction Sequence

### Phase 1: Flow Structure

`create_flow()` returns a flow already in draft mode; do not call `create_flow_draft` on it. All later calls use the id `create_flow` returned.

With `update_flow_flags="AddAll"` (the default), `create_flow` already creates one step with a task, a layout, and a default section. Rename that step for the first phase and add the others with `add_step`:

```
1. flow = create_flow("Work", "AddAll", display_name="Vessel Inspection")
2. update_flow_properties(flow_id, {
     "description": "Standard vessel inspection workflow",
     "icon": {"shape": "SearchOutlined", "color": "#2196F3"}
   })
3. steps = get_flow_steps(flow_id)
   rename_step(flow_id, steps[0]["id"], "Inspection")
   rename_step(flow_id, steps[0]["id"], "Inspection Details", target="task")
4. add_step(flow_id, -1, "Review", "Review Task")
5. add_step(flow_id, -1, "Approval", "Approval Task")
6. get_flow_steps(flow_id)   # capture layoutIds for each task
```

Display names derive camelCase internal names: `"Quality Review"` becomes `qualityReview`. `ByStepName` routing and column `valuePaths` use these names.

**ID Mapping Table — Which ID to use for each tool:**

Every `get_flow_steps` row carries several GUIDs (`id`, `stepId`, `taskId`,
`actionSetId`, `layoutId`). Most step tools accept ANY of the row's GUIDs and
resolve it for you:

| Tool | ID parameter | Where to find it |
|---|---|---|
| `manage_action`, `manage_task_invites`, `delete_step`, `rename_step` | any of `id` / `stepId` / `taskId` (resolved server-side) | `get_flow_steps` |
| `rename_step(..., target="task")` in a step with several tasks | the row's `id` or `taskId` (a shared `stepId` is ambiguous — the server rejects it with the row list) | `get_flow_steps` |
| `get_flow_step` | the record `id` field specifically | `get_flow_steps` → `id` |
| `move_step` | `stepId` field (steps) / `taskId` field (tasks) — move is the ONE endpoint keyed on these | `get_flow_steps` |
| Layout operations | `layoutId` | `get_flow_steps` → `layoutId` |

If a step tool can't match the GUID you passed, the error lists the
flow's steps (`steps: [{id, stepName, stepDisplayName}]`) — pick from there
instead of re-fetching.

### Phase 2: Layout & Components (for each task)

**CRITICAL — Do NOT call `create_layout_draft` here.** `create_flow` (new flow) creates its layouts as drafts, so the `layoutId` values from `get_flow_steps` ARE already draft IDs. Calling `create_layout_draft` on a layout that is already a draft creates an **orphan draft** that is NOT linked to the flow — `publish_flow` will publish the flow's own (empty) layout draft, not your orphan. For an existing flow, after `create_flow_draft`, check each layout's `publishStatus` first: see `references/managing-flows.md`.

```
# Each layout already has a default empty section. Get its ID from
# get_layout(layoutId, format="summary"); add_step returns it as defaultSectionId.

# Label the existing section:
update_component(layoutId, sectionId, {
  "label": "Vessel Information",
  "properties": {"showTitle": true}
})

# Add fields into the section (use col widths for a presentable layout):
add_components(layoutId, [
  {"component": "input-text", "label": "Vessel Name", "properties": {"width": "col-6"}},
  {"component": "input-text", "label": "IMO Number", "properties": {"width": "col-6"}},
  {"component": "date-picker", "label": "Inspection Date", "properties": {"width": "col-6"}},
  {"component": "input-select", "label": "Vessel Type", "properties": {"width": "col-6", "options": ["Tanker", "Bulk Carrier"]}},
  {"component": "richtext-input", "label": "Findings"},
  {"component": "attachments", "label": "Photos"}
], parent_section_id=sectionId)
```

Repeat for each task's layout. Widths, visibility rules, and component traps are in `references/layouts-and-components.md`.

### Phase 3: Actions (for multi-party or branching flows)

Steps with decision points need more than the default "Submit" button. Think about where a party might reject, send back, or cancel work.

**When to add custom actions:**
- A reviewer can approve or reject → add Reject action routing back to the requester's step
- A party can request changes → add Send Back action routing to the previous step
- Any step where work can be cancelled → add Cancel action with `TerminateFlow`
- You want distinct status labels in the grid → set `completedName`/`completedColor` on actions

**Action design guidelines for multi-party workflows:**

| Step pattern | Recommended actions |
|---|---|
| Requester submits to reviewer | Submit (default) + Cancel (`TerminateFlow`) |
| Reviewer evaluates submission | Approve (`Next`) + Reject (`ByStepName` back to requester's step camelCase name) |
| Reviewer with partial concerns | Approve + Request Changes (`Previous`) + Reject |
| Final sign-off / closure | Complete (`Next`) only |
| Any step where the initiator might abort | Add Cancel with `skipValidation: true` and `color: "error"` |

The calls, a worked approval example, and the routing types are in `references/actions-and-statuses.md`.

### Phase 4: Grid Columns (optional but recommended)

Adding components auto-generates most columns, so list them before adding any. The main column work is hiding secondary columns and reordering the important ones. See `references/actions-and-statuses.md` § Grid Columns.

### Phase 5: Publish

`publish_flow(draft_flow_id)` publishes the flow and all its layouts. Use the draft id; the result carries the new published id.

## Building an Asset Flow

Asset flows are single-step. After publishing, they appear in `search_flows` (flowResourceType=Entity) and `list_asset_types()`:

```
1. flow = create_flow("Entity", "AddAll", display_name="Vessel")   # already in draft mode
2. update_flow_properties(flow_id, {
     "description": "Vessel master data",
     "icon": {"shape": "DirectionsBoatOutlined", "color": "#00BCD4"}
   })
3. steps = get_flow_steps(flow_id)               # the scaffolded step; its layoutId is already a draft
   rename_step(flow_id, steps[0]["id"], "Vessel Details")
4. update_component(layoutId, sectionId, {"label": "General Information", "properties": {"showTitle": true}})
5. add_components(layoutId, [
     # Use layoutId directly — do NOT call create_layout_draft
     {"component": "input-text", "label": "Vessel Name", "properties": {"width": "col-6"}},
     {"component": "input-text", "label": "IMO Number", "properties": {"width": "col-6"}},
     {"component": "input-select", "label": "Flag State", "properties": {"width": "col-6", "options": ["Panama", "Liberia"]}},
     {"component": "date-picker", "label": "Built Date", "properties": {"width": "col-6"}}
   ], parent_section_id=sectionId)
6. publish_flow(flow_id)                         # use the draft id
```

Assets can then be managed via `create_asset`, `edit_asset`, etc. See `references/managing-assets.md`.
