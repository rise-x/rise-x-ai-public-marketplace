# Actions and Status Configuration

## Contents

- [What are Actions?](#what-are-actions)
- [Reading Actions](#reading-actions)
- [Action Properties](#action-properties)
- [Routing with `next`](#routing-with-next)
- [Activities (Automation on Actions)](#activities-automation-on-actions)
  - [Conditional activities (`condition`)](#conditional-activities-condition)
  - [Start Cross Ecosystem Work](#start-cross-ecosystem-work)
- [Common Action Patterns](#common-action-patterns)
- [Complete Example: Approval Workflow with Reject](#complete-example-approval-workflow-with-reject)
- [Status Display in Kanban and Grid Views](#status-display-in-kanban-and-grid-views)
  - [How Statuses Work](#how-statuses-work)
  - [Customizing Status Labels with `completedName`](#customizing-status-labels-with-completedname)
  - [Grid Columns](#grid-columns)
- [Common Mistakes](#common-mistakes)

## What are Actions?

**Actions** are the buttons users click to advance work through a workflow. Every step has at least one action (typically "Submit"), but steps can have **multiple actions** to support branching logic like Reject, Send Back, or Approve.

When a user clicks an action button, the flow engine:
1. Validates form data (unless `skipValidation` is set)
2. Routes the work item to the step specified by the action's `next` configuration
3. Notifies the party responsible for the next step

## Reading Actions

`get_flow_step(flow_id, id)` returns one step with its `actions` array; pass the `id` field from `get_flow_steps`, not `stepId`. `get_flow_config(flow_id)` returns every step's actions in its `steps` array.

## Action Properties

The `manage_action` description lists the keys it takes; `get_schema("flows", "action", flow_id=...)` returns the full action schema. Neither states:

| Property | Behaviour |
|---|---|
| `color` | Button color: `"primary"` (default), `"error"` (red), `"warning"` (orange), `"secondary"` |
| `completedName` | Status label shown in kanban/grid after this action completes |
| `completedColor` | Status color after completion (for example `"success"`, `"error"`) |
| `actionTypeName` | `"Submit"` routes the work, rejections included. `"Stop"` terminates the step or flow |

## Routing with `next`

The `next` array defines where work goes when the action is executed. Each entry is `{nextStep, nextStepType}`; `nextStepType` says how to read `nextStep`:

| Value | Behavior |
|---|---|
| `"Next"` | Move to the next sequential step (default for Submit) |
| `"Previous"` | Move back to the previous step |
| `"Self"` | Stay on the current step |
| `"ByStepName"` | Route to a step by its internal camelCase name (e.g. `"supplierOffer"`) — **preferred** |
| `"ByStepId"` | Route to a step by its GUID |
| `"StopStep"` | Stop/complete the current step |
| `"TerminateFlow"` | End the entire flow |
| `"EndFlow"` | End flow (variant) |
| `"Close"` | Close the work item |

## Activities (Automation on Actions)

**Activities** are the automation that runs when a user takes an action — e.g. `StartWork`, `StartMultipleWork`, `StartCrossEcosystemWork`, `PublishData`, `SendEmail`. They hang off an **action**, not the step directly, and run at submit time. Manage them with `manage_activity`; list the types with `get_schema("activities")` and one type's properties with `get_schema("activities", "<Type>")`.

- A step's default `Submit` action created by `AddAll` may already carry default (empty) `SendEmail` activities. If you want only your activity to run, `delete` those first.
- An `update` that drops a required property leaves the activity invalid (`invalid-configuration`), and every submit through that action then fails with a 403.

### Conditional activities (`condition`)

The `manage_activity` description covers the `condition` argument. Rules it does not state:

- Paths are rooted at the work data (`$.` = the data root — the same root the components' `dataPath` use, e.g. `$.orderRequest.selectProject`), **not** `$.data.…`.
- Combine conditions with `&&` / `||`.
- To match a picker/asset selection, compare its `displayName` (the selected label).
- If a token path does not resolve (wrong path, unset field), it collapses to an empty string and the expression is false — so a mistyped path silently **skips** the activity rather than erroring. Verify the path against `get_flow_data_schema` (drop the leading `data.`).
- Execution order within an action follows the `executeWhen` phase, then array position; there is no priority field.

```
# Run the activity only when the selected delivery terminal is "ESEASA":
condition = "'{$.orderRequest.selectDeliveryTerminal.displayName}' == 'ESEASA'"
```

### Start Cross Ecosystem Work

`StartCrossEcosystemWork` creates a work item in a flow that lives in a **different ecosystem** on the same deployment, transferring selected data from the source work. (For same-ecosystem creation use `StartWork` / `StartMultipleWork`.) The `manage_activity` description lists its properties. What it does not say:

- **`targetEcosystemId`:** resolve it **by name** with `resolve_ecosystem(ecosystem_name)`. Never take it from a flow's or work item's `environmentId`.
- **`targetFlowOriginId`:** the latest published version of the target flow is resolved at run time.
- **`publishData`:** use **root keys** (`"$.fullName"`), matching the component `dataPath` — NOT `"$.data.fullName"`. Each path is read from the source and stored at the same key on the target.
- **`stepName`:** optional step in the target flow to start at.
- **`targetRelationshipName` / `sourceRelationshipName`:** the relationship label from source to new work, and from new work to source.
- **`operations[].dataMapping`:** each entry is `{fromPath, toProperty, excludedDataPaths?}`.
- **`operations[].excludeProperties`:** properties to leave untouched, for partial updates.
- **`operations[].updateType`:** `"Set"` (default) replaces; `"Merge"` updates a subset.
- **`lookup[].assetType`:** an optional asset type hint.

**Consent — the target flow's allow-list (default-deny).** A cross-ecosystem start only succeeds if the **target flow** has allow-listed the source. This is a flow-to-flow (system) relationship — there is no per-user permission check. Set it on the target flow's `allowedStartSources` property with `update_flow_properties` (from the target ecosystem, on a target-flow draft):

```
set_active_ecosystem("<target ecosystem>")
draft = create_flow_draft("<target published flow id>")
update_flow_properties(draft["id"], {
  "allowedStartSources": [
    {"ecosystemId": "<SOURCE ecosystem id>", "flowOriginId": "<SOURCE flow origin id>"}
  ]
})
publish_flow(draft["id"])
```

With no matching entry the start is denied — the submit fails with `source-not-allow-listed`.

**Failure handling — `continueOnError`:**
- `false` (default) — **fail mode.** Any failure (bad config, not allow-listed, target-creation error) aborts the whole source submission: nothing is persisted and the user gets the error (a 403 on submit). Use when the cross-ecosystem work is mandatory.
- `true` — **fire-and-forget.** The failure is logged on the source work's event log and the source submission continues; no target work is created. Use when the cross-ecosystem work is best-effort.

**Other behavior:** the created work's `createdBy`/assignee is the source submitter; the source and target works are related **both ways** (Related Work tab), and the relationship record carries the other work's ecosystem id when it differs (the "different ecosystem" marker).

**Notify on create (`notifyOnCreate`).** Creating the target work does **not** run its first-step Submit action, so the target flow's own "assigned to you" `SendEmail` never fires on creation. Set `notifyOnCreate: true` to email the assignee at creation time instead. Without overrides, the email uses the standard template and names the target flow. Its **View** button links into the created work on the **target ecosystem's** host, falling back to the source app host. Notification failures never block the source submission.

**Operations & entity lookups (`operations`).** `publishData` copies whole source keys 1:1. Use `operations` when you need to remap fields, scope the mapping to a sub-object, or inject a **fixed asset** the target flow's routing depends on:

```
# Operation that injects a fixed delivery-location asset into the target work:
"operations": [
  {
    "lookup": [
      {"assetId": "<delivery-location asset guid>", "targetPath": "$.deliveryLocation"}
    ]
  }
]
```

**Example — start a T-shirt order in another ecosystem on submit:**

```
# On the source flow's Submit action (source flow in draft):
manage_activity(source_flow_id, step_id, submit_action_id, "add",
  activity_type="StartCrossEcosystemWork",
  properties={
    "targetEcosystemId": "<target ecosystem id>",
    "targetFlowOriginId": "<target flow origin id>",
    "publishData": ["$.fullName", "$.deliveryAddress"],
    "targetRelationshipName": "Started Order",
    "sourceRelationshipName": "Source Request",
    "continueOnError": False
  })
publish_flow(source_flow_id)
# Then allow-list the source on the TARGET flow (see Consent above) — without it, submits 403.
```

## Common Action Patterns

Every step gets a "Submit" action by default when created with `add_step`. Add the others with `manage_action(flow_id, step_id, "add", action_data=...)`:

```
# Reject: route back to an earlier step by its camelCase name
{"name": "Reject", "displayName": "Reject", "eventName": "reject",
 "actionTypeName": "Submit", "color": "error",
 "next": [{"nextStep": "orderRequest", "nextStepType": "ByStepName"}]}

# Send Back / Request Changes: return to the previous step
{"name": "SendBack", "displayName": "Send Back", "eventName": "sendBack",
 "actionTypeName": "Submit", "color": "warning",
 "next": [{"nextStepType": "Previous"}]}

# Cancel: end the flow without validating the form
{"name": "Cancel", "displayName": "Cancel Order", "eventName": "cancel",
 "actionTypeName": "Stop", "color": "error", "skipValidation": true,
 "next": [{"nextStepType": "TerminateFlow"}]}
```

A step with the default Submit plus Reject and Send Back shows three buttons: **Submit**, **Reject** (red), **Send Back** (orange).

## Complete Example: Approval Workflow with Reject

```
# Build a 3-step flow: Request → Review → Fulfilment
# The Review step should have Approve and Reject buttons

# 1. Create and set up the flow
flow = create_flow("Work", "AddAll", display_name="Purchase Order")
flow_id = flow["id"]

# 2. Rename default step and add more steps
steps = get_flow_steps(flow_id)
rename_step(flow_id, steps[0]["id"], "Order Request")
rename_step(flow_id, steps[0]["id"], "Submit Request", target="task")

add_step(flow_id, -1, "Manager Review", "Review Task")
add_step(flow_id, -1, "Fulfilment", "Process Order")

# 3. Get updated steps; get_flow_step needs the record "id", not "stepId"
steps = get_flow_steps(flow_id)
review_id = steps[1]["id"]

# 4. Customize the Review step's default Submit action
review_details = get_flow_step(flow_id, review_id)
submit_action_id = review_details["actions"][0]["id"]

manage_action(flow_id, review_id, "update", action_id=submit_action_id, action_data={
  "displayName": "Approve",
  "completedName": "Approved",
  "completedColor": "success"
})

# 5. Add Reject action on Review step
manage_action(flow_id, review_id, "add", action_data={
  "name": "Reject",
  "displayName": "Reject",
  "eventName": "reject",
  "actionTypeName": "Submit",
  "color": "error",
  "completedName": "Rejected",
  "completedColor": "error",
  "next": [{"nextStep": "orderRequest", "nextStepType": "ByStepName"}]
})

# 6. Publish
publish_flow(flow_id)
```

---

## Status Display in Kanban and Grid Views

### How Statuses Work

**Status is automatic.** When work moves through steps, the platform computes a status display from the current step's `stepDisplayName`. Each step name becomes a kanban column header and a status label in the grid view.

The status display object (`$.statusDisplay`) on a work item includes:
- `displayName` — the status label (derived from step display name)
- `color` — status color
- `stateName` — internal step name reference

### Customizing Status Labels with `completedName`

An action's `completedName` and `completedColor` override the default status display after it runs. Without them, all work in the "Manager Review" step shows "Manager Review". With them, the status shows "Approved" (green) after Approve and "Rejected" (red) after Reject. The [Complete Example](#complete-example-approval-workflow-with-reject) sets both.

### Grid Columns

Grid columns control what fields appear in the work grid (table) and kanban views. Manage them with `manage_columns`.

**Default columns.** A flow created with `AddAll` gets these automatically:
- **Display Name** — work item title
- **Work Code** — auto-generated reference code
- **Status** — current step/action status (via `$.statusDisplay.displayName`)
- **Last Modified** — timestamp
- **Created By** — initiator
- **Assigned Users** — current assignees

**Auto-generated columns.** When components are added to layouts, columns are **automatically generated** for most field types. Before adding columns:
1. Call `manage_columns(flow_id, "list")` to see what already exists. Your field's `key` is likely already there.
2. Use `"add"` only for columns not auto-generated, or to customize `valuePaths`. Adding a column with a duplicate `key` fails with a 400 error.
3. Use `"update"` to adjust visibility (`defaultOn: false`) or position on existing columns.

In practice, the main column work is **hiding secondary columns** (rich text, notes, attachments) and **reordering** the important ones — not creating new ones.

**`valuePaths` format:** JSON paths starting with `$` that match the form field's `dataPath`. The path follows the pattern `$.{stepCamelCase}.{fieldCamelCase}`. For example, a field "Vessel Name" in step "Inspection Details" has path `$.inspectionDetails.vesselName`.

```
manage_columns(draft_flow_id, "add", column={
  "displayName": "Vessel Name",
  "key": "vesselName",
  "valuePaths": ["$.inspectionDetails.vesselName"],
  "defaultOn": true,
  "colPosition": 2
})
```

**What makes a good column:** data that users need to scan across many work items without opening each one.

| Data type | Column tip |
|---|---|
| Key identifiers (order #, vessel name, ref code) | `defaultOn: true`, pin left if critical |
| Dates (due date, inspection date) | `defaultOn: true`, helps users sort by urgency |
| Amounts / totals | `defaultOn: true`, consider pinning right |
| Party names (supplier, buyer) | Useful for filtering in multi-party flows |
| Secondary details (notes, descriptions) | `defaultOn: false` — available but not shown by default |

## Common Mistakes

1. **Duplicate `eventName`**: uniqueness is flow-wide, not per step; a duplicate gets the work stuck with runtime 403s. `manage_action` rejects collisions at write time with the owning step's name.
2. **Routing to a non-existent step** — when using `ByStepName`, the step name must match the internal camelCase name exactly (e.g. `"orderRequest"`, not `"Order Request"`). **Never use `ByStepDisplayName`** — it is deprecated and will block publishing.
3. **Forgetting to draft before adding actions** — action changes require the flow to be in draft mode
4. **Not publishing after action changes** — changes are invisible until `publish_flow` is called
5. **Using `actionTypeName: "Stop"` for routing actions** — `"Stop"` terminates the step/flow. Use `"Submit"` for actions that route to other steps, even rejection actions.
6. **Missing `next` configuration** — without `next`, the action defaults to advancing to the next sequential step
