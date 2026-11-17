# Managing Flows

## What is a Flow?

A **flow** is a workflow template defining a business process. It contains steps, tasks, actions, activities, and rules. Work items are instances of flows. Flows with `FlowResourceType = Entity` define **asset types** (ThingTypes).

This file covers finding and changing an existing flow. To build a new flow from scratch, see `references/building-workflows.md`. For asset types, see `references/managing-asset-types.md`.

## Discovering Flows

> **Filtering or sorting flows?** Use **`search_flows`** (filter tree, sort, projection, paging — see `references/advanced-search.md`). Do NOT call `list_flows` and iterate the result client-side to find matches.

`list_flows` returns published **Work** flows only. To find asset types, use `search_flows` with `flowResourceType` equals `"Entity"`.

Which id to use:
- Use `id` for `create_flow_draft`, `get_flow_config`, `get_flow_steps`, `create_work`.
- Use `flowOriginId` for `list_work`, `create_asset`, and asset-related tools. Works span flow versions, so `list_work` filters by the stable `flowOriginId`, not the per-version `id`.

The flow lifecycle enum (`"Open"` | `"Active"` | `"Archived"` | `"Deleted"`) has a different field name per tool: `list_flows` returns it as `status`, and `search_flows` filters it as `state`. Write `{"field": "state", ...}` in a `search_flows` filter, not `"status"`. It is distinct from a Work item's `status`.

## Reading Flow Data

**Targeted read.** `get_flow_config(id, path="…")` returns `{path, value}`. A missing segment returns a `path_not_found` error that lists the keys available at that level, so a wrong guess self-corrects in one call.

> **Config nesting gotcha:** the flow's own properties (name, `flowFeatures`, …) live under a top-level **`properties`** object in the config payload, so the read path is `properties.flowFeatures` — even though `update_flow_properties` *writes* them with a bare `{"flowFeatures": …}` body. Root-level keys are `actions, cardLayout, chains, columns, customViews, dataPipelines, export, properties, relatedFlows, relationships, steps, tasks, workGridProperty`.

**`stepName` from `get_flow_steps`** is the internal camelCase name used for `ByStepName` routing. It can be bare (`orderRequest`) or slash form (`orderRequest/submitRequest`); route by the part before the `/`.

**`taskName` is not projected** by `get_flow_steps`. `update_work_data` needs it as `section_name`, so call `get_flow_step(flow_id, step_id)` with the `id` field from that list. (`update_work_data_bulk` does **not** need the round-trip — it also accepts the `taskDisplayName` that `get_flow_steps` already gives you — see `references/managing-work-items.md`.)

## Draft/Edit/Publish Sequence

All structural and property changes require draft mode. To change a published flow:

```
1. draft = create_flow_draft(published_id)         # NEW draft id; the published id is now read-only
2. get_flow_steps(draft_id)                        # layoutIds
3. get_layout(layoutId, format="summary")          # check publishStatus
   - "Published" → create_layout_draft(layoutId, draft_id), then edit the NEW layout id
   - "Draft"     → edit layoutId as-is
4. [edit properties, steps, actions, components using the draft ids]
5. publish_flow(draft_id)                          # publishes the flow + all linked layouts
```

**`publishStatus` enum** (`DianaPublishStatus`): `"Draft"` | `"Published"` | `"Revised"` | `"Deleted"`. A draft becomes `"Published"` on `publish_flow`, and the previous published version becomes `"Revised"`. A fifth value, `"Publishing"`, is a transient internal state during the publish flip.

**`publishMode` enum** (PascalCase; filterable via `search_flows`): `"Default"` | `"UpdateOpenItems"` | `"DoNotUpdateOpenItems"`. Controls how a publish propagates to already-open items — and it gates **every** published config change (layouts, features, relationships, …), not just UI tabs. Work flows default to `"DoNotUpdateOpenItems"`: open works stay pinned to the flow version they were created on, so a published change is visible **only on works created after the publish** — always verify a published change against a fresh work item, never an existing one. Entity flows default to `"UpdateOpenItems"` (open items migrate to the new version on publish). Read it with `get_flow_config(id, path="properties.publishMode")`; change it with `update_flow_properties(draftId, {"publishMode": "UpdateOpenItems"})` **on a draft** (expect `changed: [publishMode]`), then publish.

**Stale-id trap after re-publish:** once a draft is published, the previously published id is superseded. Calls against the stale id are inconsistent: some 403 with "is in 'Deleted' mode", but property edits can **silently succeed (200 + `changed`)** — the write lands on the dead version and never appears in the live flow. Always take the current id from the `publish_flow` result, or re-resolve it with `search_flows` filtered on `flowOriginId`, the only id stable across republishes.

## Editing Flow Properties

The `update_flow_properties` description lists the keys. It does not say:

- `displayNameTemplate` builds instance display names from tokens `{$.path}` or `{$.path:format}`, for example `"{$.vessel.displayName} - {$.inquiry.deliveryWindow.from.date:yyyy-MM-dd}"`. See `managing-asset-types.md` for details.
- The offline write flag `isOfflineModeEnabled` lives in `featureFlags`. `featureFlags` is an object `{name: bool}`, and a sent object replaces the stored one: read it back first and send it complete.
- `allowedStartSources` is set on the **target** flow. See `actions-and-statuses.md` § Start Cross Ecosystem Work.

**Naming convention:** `add_step` and `rename_step` derive camelCase internal names from display names. `"Quality Review"` becomes step name `qualityReview`; its task `"Review Task"` becomes `qualityReview/reviewTask`. `rename_step` recomputes both internal names.

## Flow Features (`flowFeatures`)

The `flowFeatures` property on a flow controls UI features and panel configuration. Set it with `update_flow_properties` **on a draft**, then publish; a write to a published flow is dropped and reported under `warnings`.

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
publish_flow(draftId)                               # commit; the result carries the new published id
```

**Action block shape** — `{enabled, includePaths, excludePaths}`:

| Field | Meaning |
|---|---|
| `enabled` | `false` disables the button (the API returns 404 on `duplicate_work`). Defaults to `true`. |
| `includePaths` | JSONPath **whitelist**, applied first. `["$"]` = the whole work document. Paths are `$.<field>` from the **work-data root** (a form field `orderNo` is `$.orderNo`). |
| `excludePaths` | JSONPath list **removed after** the include step. |

**Semantics:**
- `includePaths` runs first (keep only these), then `excludePaths` (drop these). The **allow-all-minus** pattern is `includePaths: ["$"]` + `excludePaths: [...]` — clone everything except the listed fields.
- **Defaults are permissive — there is no "copy nothing" via empty lists.** If `createAndDuplicateWork` is absent entirely, OR `duplicate` has **both** path lists empty, the engine forces `includePaths = ["$"]` and the flow **full-clones**. So empty-both does NOT mean "carry no data" — it means "carry everything". To carry only a subset, set an explicit `includePaths` (and/or `excludePaths`); to carry (almost) nothing, use an `includePaths` that selects just the field(s) you want rather than relying on emptiness.
- Legacy `flowFeatures.duplicateWork` (`enableDuplicateWork`/`enableAddNewWork`/`includePaths`/`excludePaths`) is still read for back-compat and mapped onto this shape, but write new config under `createAndDuplicateWork`.

**Do NOT confuse with `copyData` / `cloneData`.** Those are component-level flags whose names *suggest* duplicate control but do **not** drive the Duplicate button at all — see `references/layouts-and-components.md` § copyData / cloneData. Only `flowFeatures.createAndDuplicateWork.duplicate` affects what a duplicate carries.

**Verifying the exclusion:** duplicate a real work with `duplicate_work(sourceWorkId)`, then `get_work(newWorkId)` and confirm the `excludePaths` fields are absent.

`get_schema` does **not** surface `createAndDuplicateWork` — it's a `flowFeatures` sub-key, not a schema-registered resource. This doc is the source of truth for its shape.

### Left Panel (`leftPanel`)

Controls the tab strip in the side panel of a work item or asset. Shape: `{"tabs": [{tab, label?}, …], "defaultTab": "<tab>"}` under `flowFeatures.leftPanel` — set it **on a draft, then publish**, with read-modify-write (see the two ⚠️ warnings at the top of the Flow Features section above).

> **⚠️ `tabs` REPLACES the default set — it does not add to it.** When `leftPanel.tabs` is present, the panel shows exactly those entries, in array order; the defaults are discarded, with no merge. Defaults when `tabs` is absent: `workflow` + `activities` on work items, `contents` on assets. The natural "minimal" edit silently removes tabs:
>
> ```json
> // WRONG — adds Related Work but silently REMOVES Workflow and Activities:
> {"leftPanel": {"tabs": [{"tab": "relatedWork"}]}}
>
> // RIGHT — always list every tab you want visible:
> {"leftPanel": {"tabs": [{"tab": "workflow"}, {"tab": "activities"}, {"tab": "relatedWork"}]}}
> ```

```
draftId = create_flow_draft(published_flow_id)
features = get_flow_config(draftId, path="properties.flowFeatures")   # read-modify-write
update_flow_properties(draftId, {
  "flowFeatures": {
    // Preserve every sibling feature returned by the read above.
    "leftPanel": {
      "tabs": [
        {"tab": "workflow"},
        {"tab": "activities"},
        {"tab": "relatedWork"}
      ],
      "defaultTab": "relatedWork"
    }
  }
})                                                  # expect changed: [flowFeatures]
publish_flow(draftId)                               # commit; the result carries the new published id
```

**Available tabs** (the `FlowTab` enum — exactly five values, camelCase):

| Tab | Description | Renders on |
|---|---|---|
| `"workflow"` | Step/task progress view | work items only |
| `"activities"` | Activity/audit log | work items and assets |
| `"relatedWork"` | Related work items and assets (required for browsing relationships — see `references/relationships.md`) | work items only |
| `"navigation"` | Sibling browser — lists all works/assets of the same flow, with grouping/sorting/search | work items and assets |
| `"contents"` | The entity's data sections | assets only |

**Nothing validates tab values** — neither the API (schemaless `flowFeatures` blob) nor the UI. A typo'd tab name is stored and renders as a broken tab (raw i18n-key caption, empty body); a tab listed on the wrong page type (e.g. `contents` on a workflow) renders an empty body; `"tabs": []` removes every tab. Check your spelling against the table — no error will tell you.

**Per-tab `label`** (optional string) overrides the tab's translated caption; omit it for the default name. An empty string is honoured and renders a blank caption — omit the key instead. The `navigation` tab additionally accepts grouping/sorting/search config on its entry (not covered in this reference).

**`defaultTab`** selects which tab opens first. Resolution order: `leftPanel.defaultTab` if set → else the first entry of `tabs` → else the page default (`workflow` for work items, `contents` for assets). `defaultTab` is **not** validated against `tabs` — pointing it at a tab that isn't listed leaves the panel with no visible selection.

> **⚠️ "Configured the tab but it doesn't show."** On a Work flow with the default `publishMode` (see § Draft/Edit/Publish Sequence), a published `leftPanel` change appears **only on works created after the publish**; the config is not broken. `get_flow_config(newPublishedId, path="properties.flowFeatures")` confirms it persisted; whether the tab *renders* can only be checked in the app. Asset pages always read the latest flow version, so asset-side changes show on existing assets immediately.

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

**Rename or reorder steps:**
```
1. draft = create_flow_draft(flow_id)              # returns NEW draft_flow_id
2. get_flow_steps(draft_flow_id)                   # IDs from the draft
3. rename_step(draft_flow_id, row_id, "New Step Name")
   rename_step(draft_flow_id, row_id, "New Task Name", target="task")
   move_step(draft_flow_id, step_to_move_stepId, after_this_stepId)   # move takes stepId, not id
4. publish_flow(draft_flow_id)                     # use draft ID, not original
```
