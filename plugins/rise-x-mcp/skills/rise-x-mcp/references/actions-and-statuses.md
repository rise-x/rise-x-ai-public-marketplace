# Actions and Status Configuration

## Contents

- [What are Actions?](#what-are-actions)
- [Reading Actions](#reading-actions)
- [Action Properties](#action-properties)
- [Routing with `next`](#routing-with-next)
- [Managing Actions](#managing-actions)
- [Activities (Automation on Actions)](#activities-automation-on-actions)
  - [Managing Activities](#managing-activities)
  - [Start Cross Ecosystem Work](#start-cross-ecosystem-work)
- [Common Action Patterns](#common-action-patterns)
  - [Submit (default — advance to next step)](#submit-default--advance-to-next-step)
  - [Reject (route back to a previous step)](#reject-route-back-to-a-previous-step)
  - [Send Back (return to previous step)](#send-back-return-to-previous-step)
  - [Cancel / Terminate](#cancel--terminate)
  - [Multiple Actions on One Step](#multiple-actions-on-one-step)
- [Complete Example: Approval Workflow with Reject](#complete-example-approval-workflow-with-reject)
- [Status Display in Kanban and Grid Views](#status-display-in-kanban-and-grid-views)
  - [How Statuses Work](#how-statuses-work)
  - [Customizing Status Labels with `completedName`](#customizing-status-labels-with-completedname)
  - [Grid Columns](#grid-columns)
  - [Adding Custom Columns](#adding-custom-columns)
  - [Default Columns](#default-columns)
- [Status Labels: Prefer the Action Over a State Chain](#status-labels-prefer-the-action-over-a-state-chain)
- [Common Mistakes](#common-mistakes)

## What are Actions?

**Actions** are the buttons users click to advance work through a workflow. Every step has at least one action (typically "Submit"), but steps can have **multiple actions** to support branching logic like Reject, Send Back, or Approve.

When a user clicks an action button, the flow engine:
1. Validates form data (unless `skipValidation` is set)
2. Routes the work item to the step specified by the action's `next` configuration
3. Notifies the party responsible for the next step

## Reading Actions

Use `get_flow_step(flow_id, step_id)` to see the full step details including its actions. Each step has an `actions` array containing action objects.

**Note:** `get_flow_step` may return 404 on newly created draft flows. Use `get_flow_config(flow_id)` as a reliable alternative — it returns the complete flow structure including all steps and their actions in the `steps` array.

## Action Properties

| Property | Type | Description |
|---|---|---|
| `name` | string | System name (e.g. `"Submit"`, `"Reject"`) |
| `displayName` | string | Button label shown to users (e.g. `"Approve"`, `"Send Back"`) |
| `eventName` | string | **Unique identifier** used for routing. Must be unique across the ENTIRE FLOW — the engine keys event idempotency per work item by eventName, so a duplicate anywhere in the flow causes runtime 403s on work items. `manage_action` validates this before writing. |
| `actionTypeName` | string | `"Submit"` (advances work) or `"Stop"` (terminates/stops) |
| `color` | string | Button color: `"primary"` (default), `"error"` (red), `"warning"` (orange), `"secondary"` |
| `skipValidation` | bool | If true, skip form validation when this action is clicked |
| `next` | list | Routing destinations (see below) |
| `completedName` | string | Status label after this action completes (shown in kanban/grid) |
| `completedColor` | string | Status color after completion |

## Routing with `next`

The `next` array defines where work goes when the action is executed. Each entry has:

| Property | Type | Description |
|---|---|---|
| `nextStep` | string | Target step identifier (name, display name, or ID depending on type) |
| `nextStepType` | string | How to interpret `nextStep` |

**`nextStepType` values:**

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

## Managing Actions

`step_id` accepts any of the step row's GUIDs from `get_flow_steps` (`id` /
`stepId` / `taskId`) — the server resolves it. An unmatched GUID returns a
`validation` error listing the flow's steps.

### `manage_action(flow_id, step_id, "add", action_data={...})`
Add an action to a step. The flow must be in draft mode.

### `manage_action(flow_id, step_id, "update", action_id=action_id, action_data={...})`
Update an existing action (PATCH semantics).

### `manage_action(flow_id, step_id, "delete", action_id=action_id)`
Remove an action from a step.

## Activities (Automation on Actions)

**Activities** are the automation that runs when a user takes an action — e.g. `StartWork`, `StartMultipleWork`, `StartCrossEcosystemWork`, `PublishData`, `SendEmail`. They hang off an **action**, not the step directly, and run at submit time.

Discover the available activity types and their property schemas:

```
get_schema("activities")                            # list all activity types
get_schema("activities", "StartCrossEcosystemWork")  # one type's property schema
```

### Managing Activities

`manage_activity(flow_id, step_id, action_id, action, activity_type?, activity_id?, properties?, condition?)` — the flow must be in draft mode. `step_id` accepts any step GUID (`id`/`stepId`/`taskId`); `action_id` is the action's `id` from `get_flow_steps` / `get_flow_config`.

#### `manage_activity(flow_id, step_id, action_id, "add", activity_type="StartCrossEcosystemWork", properties={...})`
Creates an activity of `activity_type` (with its schema defaults) on the action, then applies `properties` if given. Returns the action's activity list including the new activity's `id`.

#### `manage_activity(flow_id, step_id, action_id, "update", activity_id=activity_id, properties={...})`
> ⚠️ **`update` REPLACES the entire `properties` object — it is NOT a deep merge.** Always pass the **full** desired property set. Omitting a key drops it: sending just `{"continueOnError": true}` wipes `targetEcosystemId` and the activity becomes invalid (`invalid-configuration` → the submit is blocked with a 403). Re-send every property you want to keep.

#### `manage_activity(flow_id, step_id, action_id, "delete", activity_id=activity_id)`
Removes the activity from the action.

**Note:** a step's default `Submit` action created by `AddAll` may already carry default (empty) `SendEmail` activities. If you want only your activity to run, `delete` those first.

### Conditional activities (`condition`)

Any activity can be gated by an optional `condition` — pass it to `manage_activity` (`add` or `update`) as the top-level `condition` argument (it is **not** a member of `properties`). When set, the engine runs the activity only if the expression is true; empty/omitted means it always runs. Pass `""` to clear a previously set condition.

The expression is a **token-replaced boolean**: `{$.path}` tokens are substituted from the work data, then evaluated. Rules:
- Paths are rooted at the work data (`$.` = the data root — the same root the components' `dataPath` use, e.g. `$.orderRequest.selectProject`), **not** `$.data.…`.
- Wrap string values and string-valued tokens in single quotes; compare with `==` / `!=`; combine with `&&` / `||`.
- To match a picker/asset selection, compare its `displayName` (the selected label).

```
# Run the activity only when the selected delivery terminal is "ESEASA":
condition = "'{$.orderRequest.selectDeliveryTerminal.displayName}' == 'ESEASA'"
```

If a token path does not resolve (wrong path, unset field), it collapses to an empty string and the expression is false — so a mistyped path silently **skips** the activity rather than erroring. Verify the path against `get_flow_data_schema` (drop the leading `data.`).

**Ordering caveat (fail-hard gate):** activity execution order within an action is config-only — it follows the `executeWhen` phase, then array position; there is no priority field. If a `continueOnError: false` activity must gate the others (so nothing non-reversible, e.g. a `SendEmail`, runs when it fails), author it **first** in the action's activity list.

### Start Cross Ecosystem Work

`StartCrossEcosystemWork` creates a work item in a flow that lives in a **different ecosystem** on the same deployment, transferring selected data from the source work. (For same-ecosystem creation use `StartWork` / `StartMultipleWork`.)

**Properties** (`get_schema("activities", "StartCrossEcosystemWork")` for the full schema):

| Property | Type | Description |
|---|---|---|
| `targetEcosystemId` | guid | **Required.** The target ecosystem (environment) id — get it from `list_ecosystems`. |
| `targetFlowOriginId` | guid | **Required.** The target flow's `flowOriginId` (stable across versions) — the latest published version is resolved at run time. |
| `stepName` | string | Optional step in the target flow to start at. |
| `publishData` | list[str] | JSON paths copied from the source work into the new work. Use **root keys** (`"$.fullName"`), matching the component `dataPath` — NOT `"$.data.fullName"`. Each path is read from the source and stored at the same key on the target. |
| `operations` | list | Optional richer source→target steps — field remapping, sub-path scoping, and **entity (asset) lookups**. See "Operations & entity lookups" below. |
| `targetRelationshipName` | string | Relationship label from source → new work. |
| `sourceRelationshipName` | string | Relationship label from new work → source. |
| `continueOnError` | bool | See failure handling below. |
| `notifyOnCreate` | bool | Optional (default `false`). Email the created target work's assignee at creation time. Needed because the target flow's first-step Submit email never fires on mere creation — see "Notify on create" below. |
| `notifyEmailSubject` | string | Optional subject override for the create notification. Defaults to a message naming the target flow. |
| `notifyEmailMessage` | string | Optional body text for the create notification. Defaults to a sentence naming the target flow. |
| `notifyEmailTemplateUri` | string | Optional template URI. When unset, the standard default email template is used. |

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

**Notify on create (`notifyOnCreate`).** Creating the target work does **not** run its first-step Submit action, so the target flow's own "assigned to you" `SendEmail` never fires on creation. Set `notifyOnCreate: true` to email the assignee at creation time instead. The email uses the standard template (with `notifyEmailSubject` / `notifyEmailMessage` / `notifyEmailTemplateUri` overrides) and includes a **View** button that deep-links into the created target work. The button link needs a host: it uses the **target ecosystem's** configured host, falling back to the source app host — so in a properly-configured deployment the button resolves; if no host is configured anywhere the button link will be host-less. Notification failures never block the source submission (the created work is kept regardless).

**Operations & entity lookups (`operations`).** `publishData` copies whole source keys 1:1. Use `operations` when you need to remap fields, scope the mapping to a sub-object, or inject a **fixed asset** the target flow's routing depends on. Each operation is an object; every field is optional, so a lookup-only operation (no `dataMapping`) is valid:

| Field | Type | Description |
|---|---|---|
| `dataMapping` | list | **Optional** source→target field mappings — each `{fromPath, toProperty, excludedDataPaths?}`. Omit for lookup-only operations. |
| `excludeProperties` | list[str] | Properties to leave untouched (partial updates). |
| `updateType` | string | `"Set"` (default, replace) or `"Merge"` (update a subset). |
| `targetPath` | string | Path in the target work data where the mapped data is placed. |
| `rootDataPath` | string | When set, the mapping resolves against **this sub-path** of the source rather than the whole source object. |
| `lookup` | list | Asset injections — see below. |

Each `lookup` entry fetches one asset and merges it into the target work data:

| Field | Type | Description |
|---|---|---|
| `assetId` | string | A literal asset GUID **or** a source-data JSON path resolving to one (e.g. `"$.selectedSite"`). |
| `assetType` | string | Optional asset type hint. |
| `dataPaths` | list[str] | Optional — narrow which asset fields are fetched. |
| `targetPath` | string | Where the fetched asset lands in the target work data. |

> **Lookups resolve as the *triggering user* and are gated on that user's own access to the referenced asset.** A missing or inaccessible asset (or an `assetId` that resolves to neither a GUID nor an accessible asset) **fails loudly** — never a silent skip — which aborts the source submission (or, with `continueOnError: true`, logs and continues). Use a lookup when the target flow needs an entity the source data doesn't carry, e.g. a fixed delivery-location asset the target's routing keys on.

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

### Submit (default — advance to next step)

Every step gets a "Submit" action by default when created with `add_step`. To customize it:

```
get_flow_step(flow_id, step_id)
# Find the action ID in the response's "actions" array

manage_action(flow_id, step_id, "update", action_id=action_id, action_data={
  "displayName": "Approve & Continue",
  "completedName": "Approved"
})
```

### Reject (route back to a previous step)

```
manage_action(flow_id, step_id, "add", action_data={
  "name": "Reject",
  "displayName": "Reject",
  "eventName": "reject",
  "actionTypeName": "Submit",
  "color": "error",
  "next": [{
    "nextStep": "orderRequest",
    "nextStepType": "ByStepName"
  }]
})
```

### Send Back (return to previous step)

```
manage_action(flow_id, step_id, "add", action_data={
  "name": "SendBack",
  "displayName": "Send Back",
  "eventName": "sendBack",
  "actionTypeName": "Submit",
  "color": "warning",
  "next": [{
    "nextStepType": "Previous"
  }]
})
```

### Cancel / Terminate

```
manage_action(flow_id, step_id, "add", action_data={
  "name": "Cancel",
  "displayName": "Cancel Order",
  "eventName": "cancel",
  "actionTypeName": "Stop",
  "color": "error",
  "skipValidation": true,
  "next": [{
    "nextStepType": "TerminateFlow"
  }]
})
```

### Multiple Actions on One Step

```
# Step: "Manager Review" already has default Submit action.
# Add Reject and Send Back:

manage_action(flow_id, step_id, "add", action_data={
  "name": "Reject",
  "displayName": "Reject",
  "eventName": "reject",
  "actionTypeName": "Submit",
  "color": "error",
  "next": [{"nextStep": "orderRequest", "nextStepType": "ByStepName"}]
})

manage_action(flow_id, step_id, "add", action_data={
  "name": "RequestChanges",
  "displayName": "Request Changes",
  "eventName": "requestChanges",
  "actionTypeName": "Submit",
  "color": "warning",
  "next": [{"nextStepType": "Previous"}]
})
```

Users will see three buttons: **Submit**, **Reject** (red), **Request Changes** (orange).

## Complete Example: Approval Workflow with Reject

```
# Build a 3-step flow: Request → Review → Fulfilment
# The Review step should have Approve and Reject buttons

# 1. Create and set up the flow
flow = create_flow("Work", "AddAll")
flow_id = flow["id"]

update_flow_properties(flow_id, {"displayName": "Purchase Order"})

# 2. Rename default step and add more steps
steps = get_flow_steps(flow_id)
rename_step(flow_id, steps[0]["stepId"], "Order Request")
rename_step(flow_id, steps[0]["id"], "Submit Request", target="task")

add_step(flow_id, -1, "Manager Review", "Review Task")
add_step(flow_id, -1, "Fulfilment", "Process Order")

# 3. Get updated steps to find Review step ID
steps = get_flow_steps(flow_id)
review_step_id = steps[1]["stepId"]

# 4. Customize the Review step's default Submit action
review_details = get_flow_step(flow_id, review_step_id)
# Find the default Submit action ID from review_details["actions"]
submit_action_id = review_details["actions"][0]["id"]

manage_action(flow_id, review_step_id, "update", action_id=submit_action_id, action_data={
  "displayName": "Approve",
  "completedName": "Approved",
  "completedColor": "success"
})

# 5. Add Reject action on Review step
manage_action(flow_id, review_step_id, "add", action_data={
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

Actions have `completedName` and `completedColor` properties that override the default status display after execution. This is how you show "Approved" vs "Rejected" instead of just the step name:

```
# Default: status shows "Manager Review" for all work in that step
# With completedName: after Approve, status shows "Approved" (green)
#                     after Reject, status shows "Rejected" (red)

manage_action(flow_id, step_id, "update", action_id=approve_action_id, action_data={
  "completedName": "Approved",
  "completedColor": "success"
})

manage_action(flow_id, step_id, "update", action_id=reject_action_id, action_data={
  "completedName": "Rejected",
  "completedColor": "error"
})
```

### Grid Columns

Grid columns control what fields appear in the data grid (table) view. The **Status column** is added by default when a flow is created with `updateFlowFlags="AddAll"`.

All column operations use `manage_columns(flow_id, action, column_id?, column?)`:

#### `manage_columns(flow_id, "list")`
List all configured columns.

#### `manage_columns(flow_id, "add", column={...})`
Add a custom column. Key properties:

| Property | Type | Description |
|---|---|---|
| `displayName` | string | Column header text |
| `key` | string | Internal reference key |
| `valuePaths` | list[str] | JSON paths to work item data (e.g. `["$.orderDetails.vesselName"]`) |
| `defaultOn` | bool | Visible by default |
| `pinned` | string | `"left"` or `"right"` to pin |
| `colPosition` | int | Column order (0-based) |
| `grouping` | bool | Enable row grouping by this column |
| `isQuickFilter` | bool | Show in quick filter bar |

#### `manage_columns(flow_id, "update", column_id=column_id, column={...})`
Update a column (PATCH semantics).

#### `manage_columns(flow_id, "delete", column_id=column_id)`
Remove a column.

### Adding Custom Columns

```
# Add a "Vessel Name" column to the grid
manage_columns(flow_id, "add", column={
  "displayName": "Vessel Name",
  "key": "vesselName",
  "valuePaths": ["$.orderDetails.vesselName"],
  "defaultOn": true,
  "colPosition": 2
})

# Add a "Total Amount" column pinned to the right
manage_columns(flow_id, "add", column={
  "displayName": "Total Amount",
  "key": "totalAmount",
  "valuePaths": ["$.orderDetails.totalAmount"],
  "defaultOn": true,
  "pinned": "right"
})
```

### Default Columns

When created with `AddAll` flags, flows get these default columns automatically:
- **Display Name** — work item title
- **Work Code** — auto-generated reference code
- **Status** — current step/action status (via `$.statusDisplay.displayName`)
- **Last Modified** — timestamp
- **Created By** — initiator
- **Assigned Users** — current assignees

## Status Labels: Prefer the Action Over a State Chain

A flow can express "what state is this in, and what colour is it" two ways: the
`completedName`/`completedColor` on each action, or a state chain via
`manage_chain`. Prefer the action.

One thing to configure instead of two. It is set in the same place as the action
that produces the state, so the label sits on the thing that caused it. A chain
is a second mapping, keyed by state name, that has to be kept in step with the
actions by hand.

**Read them, then sanity-check them.** These fields are frequently wrong, because
nothing validates them and nobody looks. A live flow inspected recently had, on
one approval task:

```
Submit     → completedName: "Transport Details"   color: primary
Send Back  → completedName: "Submitted"           color: primary
```

A rejected request named "Submitted", and a send-back sharing the primary colour
with an approval. Before an app renders these, check that each `completedName`
describes the state the action *leaves behind* and that a rejection is not
wearing an approval's colour.

**A flow's chain is often unconfigured boilerplate.** `manage_chain(flow_id,
"list")` on that same flow returned one chain, `chainColours`, whose rules had
`displayName: "Display Name"` keyed to `UntitledTask/Generated-…` state names
matching no task in the flow. The chain *feature* works — that flow's chain was
never filled in. Two conclusions to avoid: don't decide the platform has no
state model from one empty chain, and don't build on a chain without checking
its rules resolve against the current task set.

## Common Mistakes

1. **Duplicate `eventName`** — eventName uniqueness is enforced at the FLOW level (not per step): once any step's action fires an event on a work item, a same-named event on another step is blocked as a duplicate (runtime 403, the work gets stuck). `manage_action` rejects collisions at write time with the owning step's name.
2. **Routing to a non-existent step** — when using `ByStepName`, the step name must match the internal camelCase name exactly (e.g. `"orderRequest"`, not `"Order Request"`). **Never use `ByStepDisplayName`** — it is deprecated and will block publishing.
3. **Forgetting to draft before adding actions** — action changes require the flow to be in draft mode
4. **Not publishing after action changes** — changes are invisible until `publish_flow` is called
5. **Using `actionTypeName: "Stop"` for routing actions** — `"Stop"` terminates the step/flow. Use `"Submit"` for actions that route to other steps, even rejection actions.
6. **Missing `next` configuration** — without `next`, the action defaults to advancing to the next sequential step
