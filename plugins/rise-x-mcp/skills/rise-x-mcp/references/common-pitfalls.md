# Common Pitfalls

Traps that take more than one tool call, or a tool plus platform behaviour, to explain. A trap
that one tool's description already states lives in that description; a trap that belongs to
one area lives in that area's reference file. The symptom index at the end points to both.

Entries keep their original numbers because other references cite them; the gaps are retired
entries.

### 7. ID confusion

Each id kind goes to different tools, they all look like GUIDs, and a wrong kind often fails
silently or with a misleading error.

- `flowOriginId` is stable across every version of a flow or asset type. It goes to
  `list_work`, `list_assets`, `create_asset`, `get_flow_data_schema`, and every search pin. Get
  it from `list_flows` or `search_flows`.
- The published flow `id` changes on every publish. It goes to `create_work`,
  `get_flow_config`, `get_flow_steps`, `get_asset_type_properties`, and `create_flow_draft`.
  Re-read it from the `publish_flow` envelope after each publish.
- Draft ids come back from `create_flow_draft` and `create_layout_draft`; every edit and the
  publish take the draft id.
- An asset has an `entityId` (the record) and a `workId` (the draft work that creates or edits
  it). Data writes and `submit_work` take the `workId`; an `entityId` passed to
  `update_work_data_bulk` fails with `origin_unresolved`.
- A **403 from `create_asset`** usually means a flow id or a stale id was passed instead of the
  `flowOriginId`. It is not a permissions problem.

### 10. A layout that is already `Draft` must not be drafted again

After `create_flow_draft`, check each step layout with `get_layout`. Call
`create_layout_draft(layoutId, draftFlowId)` only when it reads `Published`. When it is already
`Draft` (the normal case, because `create_flow_draft` clones step layouts as editable drafts),
edit that layout id directly. Drafting a `Draft` layout spawns a detached draft the step never
points to: `publish_flow` republishes the unchanged layout, your edits vanish, and the publish
envelope still lists the old layout id. After publishing, confirm the change with
`get_layout(stepLayoutId, format="components")`.

### 17. "Cannot connect to host"

This is usually transient: retry the same call once. The exception is concurrent work-data
writes: the API answers parallel `update_work_data` calls on one work item this way, so
retrying the same fan-out reproduces it. Send one `update_work_data_bulk` instead, or run the
per-field calls one at a time.

### 21. Submitting an asset

`create_asset` and `edit_asset` return the real `stepName`, `eventName`, `steps`, and
`invitation`. Pass them to `submit_work` verbatim.

- Never guess `"Submit"`: it is a display label, not an event name. If `eventName` is missing
  from the response, call `get_work(workId)` to read the actions.
- Some asset flows need a second submit (`ActionSet_1`) to reach Complete. Check the returned
  `steps` list.

### 39. Works keep the flow version they were created on

Publishing a flow changes nothing for existing works in two ways:

- **Data paths.** Moving a field (for example `cart` to `orderDetails.cart`) leaves works created
  on the older version storing data at the old path. Searches, exports, and dashboard reports
  that target the new path skip them. Fetch one historical work and check its keys before you
  assume the current form matches storage; then backfill or accept partial coverage.
- **Configuration.** Work flows default to `publishMode: "DoNotUpdateOpenItems"`, so open works
  stay on their creation-time version and show a published change (left-panel tabs, card
  layout, actions) only on works created after the publish. Verify on a fresh work item, or set
  `publishMode` to `UpdateOpenItems` on the draft when existing works must change.

### 65. Date field names differ by tool

- `get_work`: `createdDate` / `lastModified`.
- `list_work` rows: `createdDate` / `lastModifiedDate`.
- `search_works` and `search_assets` filter and sort keys: `created` / `lastModified`.
- `get_asset`: `created` / `lastModified`.

Copy the pair from the tool you are calling; the spellings are not interchangeable.

### 67. A write to an unmodeled work-data path may drop or persist

Flow, component, and entity mutations report a dropped property as `dropped_property` /
`dropped_item`. Work **data** is not that consistent:

- **The path is dropped.** `update_work_data_bulk` catches it: the path is missing from
  `changed`, `counts.persisted` falls, and a `dropped_value` warning names it.
- **The path persists** (seen on the test API). Verification passes: the value is stored, so
  `changed` lists the path and `counts` reports full success, while the data sits at a path
  nothing in the flow reads. Bulk verification cannot detect this, because it confirms storage,
  not correctness.

Production behaviour is unconfirmed; do not assume it matches either case. Get the path right
before writing: `get_flow_data_schema(flow_origin_id)` for a work flow,
`get_asset_type_properties(flow_id)` for an asset type. `update_work_data` never re-reads, so
even the drop case needs a `get_work` there.

## Symptom index

| Symptom | Where to look |
|---|---|
| 403 on edit or publish right after drafting | `SKILL.md` § Draft/Publish Lifecycle: use the draft id |
| 403 "is in 'Deleted' mode", or a 200 write that never shows in the live flow | `references/managing-flows.md`, the stale-id trap after re-publish: re-resolve the id from the publish envelope, or from `search_flows` on `flowOriginId` |
| 403 on `create_asset` | #7 |
| Edits gone after `publish_flow` | #10 |
| "Cannot connect to host" | #17 |
| `add_components` errors after part of the batch landed | `references/layouts-and-components.md`, the partial-failure warning: check the layout, delete what landed, retry |
| 400 "duplicate column" from `manage_columns` | A column with that `key` exists: `manage_columns(flow_id, "list")`, then `"update"` it |
| A warning from `update_work_data_bulk` (`section_not_found`, `origin_unresolved`, `dropped_value`, `value_differs`, `unverified_writes`, `no_verification`), a 400 "Object reference not set" on a brand-new work item, or the tool missing on the server | `references/managing-work-items.md` |
| A bulk write reports success but the UI shows nothing | #67 |
| A published change is missing on an existing work item | #39 |
| Dashboard tile shows `NO DATA`, `0`, a single "Unknown" bucket, `undefined` labels, or a blank chart; `.map is not a function` in the console | `references/dashboards-charts.md`; `references/dashboards-aggregated.md` when the tile has `reportData` |
| Blank workboard with React error #31 or `value.includes is not a function` | `references/card-layout.md`, the scalar-leaf rule |
| Left-panel tab missing, empty, or other tabs gone | `references/managing-flows.md` § Left Panel |
| Integration test returns 401 at the token exchange | `references/integrations.md` |
| `list_work` or `list_attachments` returns an empty list with no error | The tool's description: wrong id kind (`list_work`) or wrong `resource_type` (`list_attachments`). Then `whoami` for the bound user's visibility |
