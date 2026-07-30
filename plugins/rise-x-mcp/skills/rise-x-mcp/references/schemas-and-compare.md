# Schemas & Compare

## Schema Tool

All schema operations use a single tool: `get_schema(category, schema_name?, flow_id?)`

- **Omit `schema_name`** to list all available schemas in a category
- **Provide `schema_name`** to fetch that specific schema
- **`flow_id`** is optional, only applies when `category="flows"`

**Categories:** `"flows"`, `"components"`, `"activities"`, `"pipelines"`, `"relationships"`

## Schemas by Category

### Flows: `get_schema("flows", schema_name, flow_id?)`
Flow-related JSON schemas. Optional `flow_id` for flow-specific customization.

**Schema names:** `action`, `card-layout`, `chains`, `column`, `custom-views`, `export`, `printer-context`, `properties`, `publish-acl-operation`, `publish-attachment-operation`, `publish-data-operation`, `related-flow`, `reportable-field`, `step`

> There is **no** `rules`, `rules/rule`, or `chains/rule` under `flows` (they 404). Pipeline/rule shapes live in the **`pipelines`** category (below); a chain's shape is `get_schema("flows", "chains")`.

### Components: `get_schema("components", schema_name)`
Component JSON schema by type name. Returns all configurable properties.

**Schema names (component types):** `input-text`, `richtext-input`, `date-picker`, `check-box`, `select`, `switch`, `toggle`, `signature`, `section`, `subsection`, `container`, `banner`, `data-grid`, `attachments`, `comments-box`, `rating`, `finance-section`, `link-list`, `step-slider-v1`, `user-invitation`, `search-things`, `related-work-select`, `product-ordering`, `image-readonly`, `icon-selector`, and more. (No `input-number` — numeric fields are `input-text` with `inputType: "number"`.)

> **Note — schema name vs. recommended component name.** Some schemas above (e.g. `select`, `switch`, `toggle`, `related-work-select`) are reachable via `get_schema`, but they are **not** the names you should pass to `add_components`. When *creating* components on a layout, use the recommended names: `input-select` (not `select`), `product-toggle-switch` (not `switch`/`toggle`), `relatedWork-select` (camelCase W, not `related-work-select`). See the component table and the **Component Name Gotchas** section in `references/layouts-and-components.md` for the full mapping. Passing a non-recommended name (e.g. `select`) is **rejected** by the MCP server with the canonical-name suggestion before anything is sent — fix the name and resend.

### Activities: `get_schema("activities", schema_name)`
Activity JSON schemas for workflow automation actions.

**Schema names:** `SendEmail`, `PublishData`, `PublishDataV2`, `StartWork`, `StartWorkV3`, `SubmitWork`, `CreateUpdateEntityData`, `SearchEntityData`, `RelateEntity`, `DocumentGeneration`, `FlagRisk`, and more.

### Pipelines: `get_schema("pipelines", schema_name)`
Pipeline JSON schemas: the top-level pipeline plus its conditions and operations.

**Format:** `pipeline` (top-level shape), `pipeline/if/{CONDITION}`, or `pipeline/steps/{OPERATION}`. Names are **UPPER_SNAKE_CASE** — e.g. `WORK_DATA_PATH_EQUALS`, **not** `WorkDataPathEquals`; the operation namespace is `steps`, **not** `then`.

**Conditions** (`pipeline/if/…`): `AND`, `OR`, `NOT`, `WORK_DATA_PATH_EQUALS`, `WORK_DATA_PATH_EXISTS`, `WORK_DATA_CONDITION`, `IS_WORK_DELAYED`, `WORK_WITH_OPEN_TASK`, `WORK_WITH_COMPLETED_TASK`, `USER_IN_ROLE`, `USER_FROM_COMPANY`, `USER_WITH_EMAIL`, `USERS_WITH_EMAILS`
**Operations** (`pipeline/steps/…`): `SET_JSON_PATH`, `SET_DATA`, `COPY_DATA_PATH`, `REMOVE_DATA_PATH`, `MAP_SELECT`, `MAP_FIRST`, `MAP_WHERE`, `ADD_DAYS`, `ADD_ALERT`, `GET_RELATED_ENTITY`, `PULL_DATA_FROM_RELATIONSHIP`, … (full catalog + authoring guide in `references/pipelines.md`)

## When to Use Schemas

- **Before adding components:** Check what properties a component type supports
  ```
  get_schema("components", "search-things")   # see auto-population options
  get_schema("components", "data-grid")       # see column configuration
  ```
- **Before configuring flow activities:** Check required fields for an activity
  ```
  get_schema("activities", "SendEmail")
  ```
- **Before setting flow properties:** Check the properties schema
  ```
  get_schema("flows", "properties")
  ```
- **When unsure about valid values:** Schemas define enums, required fields, and defaults

## Cross-Ecosystem Comparison

### `compare(entity_type, source_ecosystem, source_id, target_ecosystem, target_id?)`
Compare flow or layout configurations across workspaces. Returns structured diff.

**Parameters:**
- `entity_type` — `"flow"` or `"layout"`
- `source_ecosystem` / `target_ecosystem` — **ecosystem/workspace names** (e.g. `"MyEcosystem"`), NOT deployment env names (dev/test/prod)
- `source_id` / `target_id` — flow or layout GUIDs
- Omit `target_id` to compare against an empty flow/layout (audit what exists)

**Returns:**
```json
{
  "summary": "Overview of changes",
  "differences": [
    {"type": "added|removed|changed", "path": "steps[0].name", "source": "...", "target": "..."}
  ],
  "patchPlan": [
    {"operation": "add|remove|update", "path": "...", "value": "..."}
  ]
}
```

## Common Patterns

**Audit a flow's configuration:**
```
compare("flow", "my-workspace", "flow-guid", "my-workspace")
# Omitting target_id compares against empty — shows everything in the flow
```

**Compare same flow across workspaces:**
```
compare("flow", "staging-workspace", "flow-guid", "prod-workspace", "flow-guid")
# See what's different between staging and production
```

**Check component schema before adding:**
```
get_schema("components")                       # list all available types
get_schema("components", "search-things")      # discover all properties
```
