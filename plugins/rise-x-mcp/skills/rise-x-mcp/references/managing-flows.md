# Managing Flows

## What is a Flow?

A **flow** is a workflow template defining a business process. It contains steps, tasks, actions, activities, and rules. Work items are instances of flows. Flows with `FlowResourceType = Entity` define **asset types** (ThingTypes).

For building a new flow from scratch, see `references/building-workflows.md`.
For creating asset types, see `references/managing-asset-types.md`.

## Discovering Flows

> **Filtering or sorting flows?** Use **`search_flows`** (filter tree, sort, projection, paging — see `references/advanced-search.md`). `list_flows` is only for the simple "list everything, maybe scoped to Work or Entity" case. Do NOT call `list_flows` and iterate the result client-side to find matches.

### `search_flows(filter, sort, fields, page, page_size, enforce_fields, include_total_count)`
**Advanced flow search.** Use whenever the user describes a filter (e.g. "flows modified this month", "asset types with name starting with X", "flows where publishStatus is Published"), a sort, or wants only specific fields. Full reference in `references/advanced-search.md`.

### `list_flows(flow_resource_type: str | None = None)`
List published flows in the active ecosystem. Use this to discover workflows and asset types you can operate on (e.g. for `create_work`, `list_work`, `create_asset`).

- `flow_resource_type` — optional filter, one of: `"Work"` (workflows), `"Entity"` (asset types), `"User"` (system user flows — rare), `"Company"` (system company flows — rare). Omit for all four. Values are PascalCase from the `FlowResourceType` enum.

Each flow in the response includes: `id`, `flowOriginId`, `displayName`, `flowResourceType`, `entityType`, `status`, `description`, `resourceVersion`.

- Use `id` for `create_flow_draft`, `get_flow_config`, `get_flow_steps`, `create_work`
- Use `flowOriginId` for `list_work`, `create_asset`, and asset-related tools (works span flow versions, so `list_work` filters by the stable `flowOriginId`, not the per-version `id`)
- `flowResourceType` enum: `"Work"` | `"Entity"` | `"User"` | `"Company"`. `"Work"` = workflow; `"Entity"` = asset type. `"User"` / `"Company"` are rare system flows.
- Flow lifecycle enum (mapped from `DianaFlowStatus`, values `"Open"` | `"Active"` | `"Archived"` | `"Deleted"`). ⚠️ Field name differs by endpoint: `list_flows` returns the value under `status` (shown above); `search_flows` exposes the SAME enum under the key `state` — when composing a filter tree for `search_flows`, write `{"field": "state", ...}`, not `"status"`. Distinct from a Work item's `status`. `search_flows` excludes `Deleted` and `Archived` by default — see `references/advanced-search.md` § default soft-delete exclusion.

## Creating a New Flow

### `create_flow(flow_resource_type, update_flow_flags)`
Create a brand new empty draft flow. This is the entry point for creating both workflows and asset types.

- `flow_resource_type` — `"Work"` (default, workflow), `"Entity"` (asset type), `"User"`, `"Company"`
- `update_flow_flags` — `"AddAll"` (default, scaffolds step + layout + section + columns), `"None"`, `"AddSteps"`, `"AddLayouts"`, `"AddDefaultColumns"`, `"AddDefaultLayoutSection"`

Returns the new draft flow. **No need to call `create_flow_draft` after this** — the flow is already in draft mode.

**NOTE:** `create_flow` creates a **new** flow. `create_flow_draft` creates an editable draft of an **existing published** flow. Don't confuse the two.

## Reading Flow Data

### `get_flow_config(id: str)`
Full configuration — steps, actions, activities, properties, pipelines. Use when you need everything.

### `get_flow_config(id, format="model")`
Structured object model representation. Pass `format="model"` to `get_flow_config`.

### `get_flow_config(id, path="…")` — targeted read
Pass `path` to return just one subtree instead of the whole payload — the cheap way to **verify a single nested key landed** without dumping the entire config. Dotted key path from the config root; a leading `$.`/`$` is tolerated; list indices are integers.

```
get_flow_config(flow_id, path="properties.flowFeatures")                        # just the feature flags
get_flow_config(flow_id, path="properties.flowFeatures.createAndDuplicateWork")  # just the duplicate config
```

Returns `{path, value}`. A missing segment returns a typed `path_not_found` error **listing the keys that *are* available at that level** — so a wrong guess self-corrects in one call. `path` takes precedence over `format="summary"` (the summary projection applies only when `path` is omitted).

> **Config nesting gotcha:** the flow's own properties (name, `flowFeatures`, …) live under a top-level **`properties`** object in the config payload, so the read path is `properties.flowFeatures` — even though `update_flow_properties` *writes* them with a bare `{"flowFeatures": …}` body. Root-level keys are `actions, cardLayout, chains, columns, customViews, dataPipelines, export, properties, relatedFlows, relationships, steps, tasks, workGridProperty`.

### `get_flow_steps(flow_id: str)`
Lightweight projected list — **best starting point** for understanding flow structure.

Returns: `[{id, stepName, stepDisplayName, taskDisplayName, stepId, taskId, layoutId, actionSetId}]`

- `id` — composite step identifier used by `get_flow_step`, `rename_task`, `add_action`, `update_action`, `delete_action`.
- `stepName` — internal camelCase name used for `ByStepName` action routing. For v3-style flows (asset types, legacy workflows) this is slash form like `UntitledStep/Generated-<guid>`; for v4 native flows it is bare like `Step_1`.
- `stepDisplayName` / `taskDisplayName` — human-readable labels.
- `stepId` / `taskId` / `actionSetId` / `layoutId` — internal GUIDs.

**Not projected:** `taskName` is omitted from this response. To get it (needed as `section_name` for `update_work_data`), call `get_flow_step(flow_id, step_id)` with the `id` field from this list — see `references/managing-work-items.md`.

### `get_flow_step(flow_id: str, step_id: str)`
Full details of a single step/task.

### `get_flow_versions(id: str)`
Version history with dates. Use before `restore_flow_version`.

## Draft/Edit/Publish Sequence

All structural and property changes require draft mode.

**`publishStatus` enum** (the flow's publication lifecycle, mapped from `DianaPublishStatus`): `"Draft"` | `"Published"` | `"Revised"` | `"Deleted"`. A draft becomes `"Published"` on `publish_flow`. The previous published version transitions to `"Revised"`. A fifth value `"Publishing"` exists as a transient internal state during the publish flip — callers don't normally see it. Filter on `publishStatus` via `search_flows` to find e.g. all `"Draft"` flows in the ecosystem.

**CRITICAL — Draft ID Rule:** `create_flow_draft` returns a **new draft ID**. All subsequent operations MUST use this draft ID, not the original. Editing the original published flow returns 403 Forbidden.

**Stale-id trap after re-publish:** once a draft is published, the previously published id is superseded. Calls against the stale id are inconsistent: some 403 with "is in 'Deleted' mode", but property edits can **silently succeed (200 + `changed`)** — the write lands on the dead version and never appears in the live flow. Always take the current id from the publish envelope or re-resolve via `list_flows` (the `flowOriginId` is the only id stable across republishes).

```
1. draft_result = create_flow_draft(original_id)  # returns NEW draft flow ID
2. [make changes using draft_flow_id — properties, steps, etc.]
3. publish_flow(draft_flow_id)                     # required last — publishes flow + all layouts
```

### `create_flow_draft(id: str)`
Create an editable draft of an **existing published** flow. Returns a new draft ID.

To create a brand-new flow, use `create_flow(...)` instead (see above). Passing a fresh/unknown UUID to `create_flow_draft` returns a 404 — the underlying flow must already exist.

### `publish_flow(id: str)`
Publish draft and all associated layouts. Must use the **draft flow ID**, not the original.

## Editing Flow Properties

### `update_flow_properties(id: str, properties: dict)`
PATCH semantics — only include changed fields.

**Supported keys:**
- `displayName` / `name` — flow name
- `description` — flow description
- `icon` — icon config: `{"shape": "CheckCircleOutlined", "color": "#4CAF50"}`
- `entityType` — ThingType identifier for Entity flows (e.g. `"vessel"`)
- `displayNameTemplate` — JSONPath-based template for instance display names (e.g. `"{$.vessel.displayName} - {$.inquiry.deliveryWindow.from.date:yyyy-MM-dd}"`). Tokens are `{$.path}` or `{$.path:format}`. See `managing-asset-types.md` for details.
- `featureFlags` — feature toggles (e.g. `{"OnlyAssetTypeOwnersCanCreateAsset": true}`)
- `allowedStartSources` — consent allow-list for the `StartCrossEcosystemWork` activity: a list of `{"ecosystemId": "<id>", "flowOriginId": "<id>"}` objects naming the source flows permitted to start THIS flow from another ecosystem. Default-deny (absent/empty ⇒ no cross-ecosystem starts). Set this on the **target** flow. See `actions-and-statuses.md` § Start Cross Ecosystem Work.

Example:
```json
{
  "displayName": "Vessel Inspection",
  "description": "Standard vessel inspection workflow",
  "icon": {"shape": "SearchOutlined", "color": "#2196F3"}
}
```

## Structure Modification (all require draft mode)

### `add_step(flow_id, position, step_name, task_name)`
Add a new step with a task at the specified position.

- `position` — 0-based index. Use `-1` to add at end (before End Step).
- `step_name` — display name for the step (e.g. `"Quality Review"`)
- `task_name` — display name for the task (e.g. `"Review Task"`)
- Returns updated step list with IDs for the new step

**Naming convention:** The server auto-derives camelCase internal names from display names. E.g., `"Quality Review"` becomes step name `qualityReview`, task name `qualityReview/reviewTask`.

### `delete_step(flow_id, step_id)`
Delete a step and all its tasks. Accepts any of the row's GUIDs from `get_flow_steps` (`id` / `stepId` / `taskId`) — resolved server-side.

### `rename_step(flow_id, step_id, new_name)`
Rename a step. Keeps the task name unchanged. Accepts any of the row's GUIDs from `get_flow_steps`.

### `rename_step(flow_id, task_id, new_name, target="task")`
Rename a task. Pass `target="task"` to rename the task instead of the step. Keeps step name unchanged.

**Which ID to use:** any of the row's GUIDs from `get_flow_steps` (`id` / `stepId` / `taskId`) — resolved server-side since 1.2.0. One caveat: in a step with several tasks, `target="task"` needs the specific row's `id` or `taskId` (the shared `stepId` is ambiguous and is rejected with the row list). See the ID mapping table in `building-workflows.md`.

### `move_step(flow_id, step_id, after_step_id)`
Reorder a step. Use `"00000000-0000-0000-0000-000000000000"` to move to front. **Move is the ONE tool keyed on the `stepId` field** (not the record `id`) — pass `stepId` for steps, `taskId` for tasks.

### `move_step(flow_id, task_id, after_task_id, item_type="task", target_step_id=step_id)`
Move task within or between steps. Pass `item_type="task"` and `target_step_id` for the target step (accepts the target's `stepId`, `taskId`, or `actionSetId`). Use null GUID for `after_id` to move to front of step.

## Versioning

### `restore_flow_version(id: str)`
Restore the previous published version as current. Use `get_flow_versions` first to see history.

## Deletion

### `delete_flow(id: str)`
Marks the flow as deleted.

## Flow Features (`flowFeatures`)

The `flowFeatures` property on a flow controls UI features and panel configuration. Set it via `update_flow_properties` — **but only on a draft** (see the two warnings below).

> **⚠️ `flowFeatures` edits need the draft→publish cycle — a write to a *published* flow is silently dropped.** `update_flow_properties(publishedId, {"flowFeatures": …})` against a published flow id returns 200 but persists nothing (`update_flow_properties` surfaces this as `changed: []` + a `dropped_property` warning — trust that signal, don't assume it landed). You must: `create_flow_draft(publishedId)` → `update_flow_properties(draftId, {"flowFeatures": …})` (now `changed: [flowFeatures]`) → `publish_flow(draftId)` → re-resolve the new published id via `list_flows`. Verified end-to-end on test.
>
> **⚠️ `flowFeatures` is a whole-object write — no deep-merge.** Unlike the `manage_*` sub-resource tools, `update_flow_properties` replaces the entire `flowFeatures` object. Sending `{"flowFeatures": {"createAndDuplicateWork": {...}}}` **clobbers** any sibling feature already there (`leftPanel`, `calendarBoard`, `isSingleScreenWorkUpdate`, …). Always **read-modify-write**: `get_flow_config(draftId, path="properties.flowFeatures")` → merge your key into the returned object → send the whole merged object back → `publish_flow`. Confirm with `get_flow_config(newPublishedId, path="properties.flowFeatures")`.

### Duplicate & Create Work (`createAndDuplicateWork`)

Controls the DIANA-native **Duplicate** button (and the Create-new-work action) on a flow's work items, and — critically — **which data is carried into the duplicate**. Set under `flowFeatures.createAndDuplicateWork` **on a draft, then publish** (see warnings above); each of `create` and `duplicate` is an independent action block:

```
draftId = create_flow_draft(published_flow_id)      # → draftId (published id is now read-only)
update_flow_properties(draftId, {
  "flowFeatures": {
    "…existing sibling features…": {},            # resend — whole-object write (see warning above)
    "createAndDuplicateWork": {
      "duplicate": {
        "enabled": true,
        "includePaths": ["$"],                      # allow-all…
        "excludePaths": ["$.cart", "$.paymentMethod", "$.cardLast4",
                         "$.transactionReference", "$.amountPaid"]   # …minus these
      },
      "create": { "enabled": true, "includePaths": [], "excludePaths": [] }
    }
  }
})                                                  # expect changed: [flowFeatures]
publish_flow(draftId)                               # commit; re-resolve new published id via list_flows
```

**Action block shape** — `{enabled, includePaths, excludePaths}`:

| Field | Meaning |
|---|---|
| `enabled` | `false` disables the button (the API returns 404 on `duplicate_work`). Defaults to `true`. |
| `includePaths` | JSONPath **whitelist**, applied first. `["$"]` = the whole work document. Paths are `$.<field>` from the **work-data root** (a form field `orderNo` is `$.orderNo`). |
| `excludePaths` | JSONPath list **removed after** the include step. |

**Semantics (verified against the engine):**
- `includePaths` runs first (keep only these), then `excludePaths` (drop these). The **allow-all-minus** pattern is `includePaths: ["$"]` + `excludePaths: [...]` — clone everything except the listed fields.
- **Defaults are permissive — there is no "copy nothing" via empty lists.** If `createAndDuplicateWork` is absent entirely, OR `duplicate` has **both** path lists empty, the engine forces `includePaths = ["$"]` and the flow **full-clones**. So empty-both does NOT mean "carry no data" — it means "carry everything". To carry only a subset, set an explicit `includePaths` (and/or `excludePaths`); to carry (almost) nothing, use an `includePaths` that selects just the field(s) you want rather than relying on emptiness.
- Legacy `flowFeatures.duplicateWork` (`enableDuplicateWork`/`enableAddNewWork`/`includePaths`/`excludePaths`) is still read for back-compat and mapped onto this shape, but write new config under `createAndDuplicateWork`.

**Do NOT confuse with `copyData` / `cloneData`.** Those are component-level flags whose names *suggest* duplicate control but do **not** drive the Duplicate button at all — see `references/layouts-and-components.md` § copyData / cloneData. Only `flowFeatures.createAndDuplicateWork.duplicate` affects what a duplicate carries.

**Verifying the exclusion:** duplicate a real work with `duplicate_work(sourceWorkId)`, then `get_work(newWorkId)` and confirm the `excludePaths` fields are absent. (Before the `duplicate_work` tool existed this could only be checked in-app.) See `references/managing-work-items.md` § duplicate_work.

`get_schema` does **not** surface `createAndDuplicateWork` — it's a `flowFeatures` sub-key, not a schema-registered resource. This doc is the source of truth for its shape.

### Left Panel Tabs

Controls which tabs appear in the work item side panel:

```
update_flow_properties(flow_id, {
  "flowFeatures": {
    "leftPanel": {
      "tabs": [
        {"tab": "workflow"},
        {"tab": "activities"},
        {"tab": "relatedWork"}
      ]
    }
  }
})
```

**Available tabs:**

| Tab | Description |
|---|---|
| `"workflow"` | Step/task progress view |
| `"activities"` | Activity log |
| `"relatedWork"` | Related work items and assets (required for relationships — see `references/relationships.md`) |

**Note:** The `relatedWork` tab must be enabled for the Related Work panel to appear in the UI. Without it, configured relationships still function at the data level but users can't browse them in the side panel.

### Layout Panel Config

Each step/task can have panel-level settings controlling submission behavior. These are part of the step configuration (visible in `get_flow_config`) rather than `flowFeatures`, but control related UI behavior:

| Property | Type | Description |
|---|---|---|
| `quickSubmit` | boolean | Enable quick submit (submit without opening the full form) |
| `toEmails` | boolean | Show "To" email field on submission |
| `ccEmails` | boolean | Show "CC" email field on submission |
| `toEmailsLabel` | string | Custom label for the "To" email field |
| `ccEmailsLabel` | string | Custom label for the "CC" email field |
| `message` | boolean | Show message field on submission |

## Common Patterns

**Rename a step and its task together:**
```
1. get_flow_steps(flow_id)                        # get current structure with IDs
2. draft = create_flow_draft(flow_id)              # returns NEW draft_flow_id
3. rename_step(draft_flow_id, step_id, "New Step Name")
4. rename_step(draft_flow_id, task_id, "New Task Name", target="task")
5. publish_flow(draft_flow_id)                     # use draft ID, not original
```

**Reorder steps:**
```
1. get_flow_steps(flow_id)                        # get current order with IDs
2. draft = create_flow_draft(flow_id)              # returns NEW draft_flow_id
3. move_step(draft_flow_id, step_to_move, after_this_step)
4. publish_flow(draft_flow_id)                     # use draft ID, not original
```
