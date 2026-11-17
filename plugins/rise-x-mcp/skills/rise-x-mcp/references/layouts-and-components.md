# Layouts & Components

## Contents

- [What is a Layout?](#what-is-a-layout)
- [Component Hierarchy (Critical)](#component-hierarchy-critical)
- [Adding Components](#adding-components)
- [Grid System & Column Widths](#grid-system--column-widths)
- [Conditional Rendering & Dynamic Visibility](#conditional-rendering--dynamic-visibility)
  - [`isHiddenDataPath` — Data-Driven Visibility](#mechanism-1-ishiddendatapath--data-driven-visibility)
  - [`renderWhen` — Step-State Visibility](#mechanism-2-renderwhen--step-state-visibility)
  - [`readOnlyWhen` — Conditional Editability](#mechanism-3-readonlywhen--conditional-editability)
- [Component Types Reference](#component-types-reference)
- [AI Import (for data-grid and timesheet-table)](#ai-import-for-data-grid-and-timesheet-table)
- [search-things Component Reference](#search-things-component-reference)
- [data-grid Component Reference](#data-grid-component-reference)
  - [Computed Columns](#computed-columns)

## What is a Layout?

A **layout** is the UI form definition attached to a task in a flow. It contains a tree of **components** (form fields) organized inside **sections** (containers). Edit a layout only through a flow draft; the build order and the layout-draft traps are in `references/building-workflows.md`.

To see every property a component type accepts, call `get_schema("components", "<type>")`. This file covers behaviour the schema does not state.

## Component Hierarchy (Critical)

```
Layout
  -> Section (parentId = "00000000-0000-0000-0000-000000000000")
     -> Components (parentId = section's ID)
```

- **Sections** are top-level containers. Their `parentId` is always the null GUID.
- **Non-section components** (input-text, select, etc.) MUST have a `parentId` pointing to a section.
- Orphan components (no parentId) will be rejected with an error.

## Adding Components

To add a section and its fields in one call, put the section first; the non-section components that follow auto-parent to the first section in the same call:

```
add_components(layout_id, [
  {"component": "section", "label": "Details"},
  {"component": "input-text", "label": "Name"},
  {"component": "date-picker", "label": "Date"}
])
```

**Partial Failure Warning:** `add_components` batch calls may partially succeed before returning an error. If you get a 400 or other error, always call `get_layout(layout_id, format="components")` to check what was actually created before retrying. Delete unwanted duplicates with `delete_components`.

## Grid System & Column Widths

Rise-X uses a **12-column grid** (Bootstrap/MUI-style). Components sit inside a `Grid container` with `spacing={2}`. Each component's width is set via `properties.width`, as `col-{1-12}`. The platform default for most inputs is `col-6`; each type's default is in [Component Types Reference](#component-types-reference).

### Layout Design Guidelines

**Always aim for multi-column layouts** — don't default everything to `col-12`. Forms look better and are more usable when related short fields sit side-by-side.

**Row planning:** Components flow left-to-right within a section. Column values in a row should sum to 12 (or less — remaining space stays empty on the right).

**Recommended patterns:**

```
Two-column (most common):      [col-6: First Name]   [col-6: Last Name]
Asymmetric (main + sidebar):   [col-8: Title]        [col-4: Priority]
Three-column (status fields):  [col-4: Status]  [col-4: Priority]  [col-4: Severity]
Full width:                    [col-12: Description (richtext)]
```

**Rules of thumb:**
- Short text fields, dropdowns, date pickers: `col-6` or `col-4`
- Checkboxes, toggles, ratings: `col-3` or `col-4`
- Rich text, attachments, data grids, signatures: `col-12`
- Group related fields on the same row (e.g., First Name + Last Name)
- Put the most important field first and/or wider
- Use `collapsible` on a section (`"collapsed"` / `"expanded"`) to save vertical space

### Useful Component Properties

The schema lists these properties but not how they behave:

| Property | Effect |
|---|---|
| `inputType` | `"text"` (default), `"number"`, `"password"`, `"link"`. `"email"` is NOT valid; enforce email format with a validation rule. |
| `defaultValueDataKey` | Data *context* the `defaultValue` expression resolves against — **not the value itself**. With an empty `defaultValue` it does nothing. Rarely needed: a `defaultValue` of `$.path` already resolves against the whole work. |
| `recalculateDefaultValueWhenDataPathChange` | `string[]` of paths. Re-evaluates `defaultValue` when any watched path changes. **Only fires when `defaultValue` is a dynamic expression** (`$.…`, `=…`, or `{$.…}`) — a literal or empty default is never recalculated. |
| `sourceComponentRelation` | `"copy"` = default value from the source component, `"sync"` = shared data path. Pairs with `sourceComponentId`. |
| `onChange` | URL to call when the value changes (supports `{$.field}` tokens). |
| `workId` | Binds the component to a different work item. Takes a literal id or a `$.path`. |

#### copyData / cloneData (inert flags — NOT duplicate control)

Components carry two boolean flags, `copyData` and `cloneData`, that default to `false`. Their names *suggest* they control what gets copied when a work item is duplicated — **they do not.** No runtime code reads them to drive the **Duplicate** button, so toggling them and republishing changes nothing about duplication.

To control which fields a duplicate carries, configure `flowFeatures.createAndDuplicateWork.duplicate` (`includePaths`/`excludePaths`) at the **flow** level — see `references/managing-flows.md` § Duplicate & Create Work.

#### Default Value Expressions

The `defaultValue` field supports:
- **Literal values:** `"Draft"`, `"0"`, `"true"`
- **Data path references:** `$.someField` — resolves to the value at that path
- **Mathematical expressions:** `={$.price} * {$.quantity}` — computed at runtime
- **Token substitution:** `{$.field}` — replaced with field value in strings
- **Special values:** `"utcNow"` — current UTC timestamp (useful for date-picker defaults)
- **Special value for user-invitation:** `"initiator"` — auto-populates with the work initiator/task assignee (see below)

Expression syntax is in `references/dynamicValue.md`.

#### User-Invitation Default Value: `"initiator"` (IMPORTANT)

For `user-invitation` (People Search) components, use `"initiator"` as the `defaultValue` to auto-populate with the current user. **There is NO `@me` token** in Rise-X. At runtime `"initiator"` resolves to `work.users[work.roleName]?.[0]?.email`, the first user in the current step's role.

```json
{
  "component": "user-invitation",
  "label": "Requirement Initiator",
  "required": true,
  "defaultValue": "initiator",
  "dataPath": "$.requirement.initiator",
  "properties": {"width": "col-6", "multiSelect": "single"}
}
```

Other `defaultValue` forms for user-invitation: `$.someField` (another component's data path), a nested asset-property path from an asset search component, a fixed email (`"user@example.com"`), or a calculation (`={$.expr}`).

#### Populating a Field from a Selected Asset

To copy values out of an asset chosen in a `search-things` field into other fields, you configure the **target** fields — `search-things` itself has no copy/populate property. (`autoPopulate` does **not** exist; a `{from, to}` array on the picker is silently ignored.)

When an asset is selected, the **whole asset object the picker fetched** is stored at the picker's `dataPath`, with the asset's own fields nested under `.data`. So if the picker binds to `$.cmo`, the asset's `sites` field is at `$.cmo.data.sites` — **not** `$.cmo.sites`.

On each target field, set `defaultValue` to that path and watch the picker's path for changes:

```json
{
  "component": "input-text",
  "label": "Site",
  "dataPath": "$.site",
  "defaultValue": "$.cmo.data.sites",
  "properties": {
    "width": "col-4",
    "recalculateDefaultValueWhenDataPathChange": ["$.cmo"]
  }
}
```

**Three traps that silently break this:**
- **Empty `defaultValue` disables it.** A blank or literal default is never recomputed, and `defaultValueDataKey` is only the *context*, never the value. Put the `$.cmo.data.<field>` path in `defaultValue` and leave `defaultValueDataKey` unset.
- **Missing `.data`.** Use `$.cmo.data.<field>`; `$.cmo.<field>` resolves to nothing.
- **Field not fetched.** The path only resolves if the field is actually present in what the picker stored. Leave the picker's `dataPaths` unset/`null` (the default returns the full asset object), or, if you do set `dataPaths`, include the field you reference (e.g. `$.data.sites`).

## Conditional Rendering & Dynamic Visibility

Rise-X supports three mechanisms for conditionally showing/hiding or making fields read-only. The expression-based ones use the `dynamicValue()` engine (math.js syntax with token replacement; see `references/dynamicValue.md`).

### Mechanism 1: `isHiddenDataPath` — Data-Driven Visibility

**Property location:** `properties.isHiddenDataPath`. The expression is evaluated; **truthy result = component IS hidden**.

**CRITICAL — Logic is inverted:** The expression returns `true` to **HIDE** the field. To **SHOW** a field only when a condition is met, use `!==` or negate the logic.

```json
// Visible only when decision is "Counter-Offer"
"isHiddenDataPath": "='{$.review.decision}' !== 'Counter-Offer'"

// Show only when trade type is "Contract Nomination" AND price type is "Floating"
"isHiddenDataPath": "='{$.order.priceType}' === 'Floating' ? equalText('{$.inquiry.tradeType}','Contract Nomination') : true"

// Hide when a boolean flag is true
"isHiddenDataPath": "='{$.config.enableFeature}' === 'true'"

// Show only when a field has a value (hide when empty)
"isHiddenDataPath": "=size('{$.someField}') === 0"
```

`equalText(a, b)` compares case-insensitively; `size(x)` is truthy when non-empty.

**Available data context in expressions:**
- `{$.fieldName}` — Access any work data path
- `{@row.fieldName}` — Current row data, **only inside data-grid contexts that wrap the row into the data scope**: `isHiddenDataPath` and `readOnlyWhen` on a column, and any component nested in a row-detail panel. The token form has **no `$.` prefix** (the row lives at `data['@row']`, which `lodash.has` resolves correctly only when written as `{@row.field}`).
- `{@work.fieldName}` — Full work object, same wrapping rule and same no-`$.` shape.

> ⚠️ Data-grid **computed columns** (`column.value`) do *not* see `@row` in scope. The row there is passed as mathjs `variables`, not as `data['@row']`. Use bare symbols like `=quantity * unitPrice` instead — see [Computed Columns](#computed-columns).

### Mechanism 2: `renderWhen` — Step-State Visibility

**Property location:** `properties.renderWhen`. An enum string, no expression:

| Value | Shows when |
|---|---|
| `"StepIsEditing"` | Step is actively being edited |
| `"StepIsNotEditing"` | Step is NOT being edited (read-only view) |
| `"StepIsComplete"` | Step has been completed/submitted |
| `"StepIsNotComplete"` | Step has NOT yet been completed |
| `"StepIsEditingAndIsComplete"` | Step is being re-edited after completion |

### Mechanism 3: `readOnlyWhen` — Conditional Editability

**Property location:** `properties.readOnlyWhen`. Same expression engine as `isHiddenDataPath`; **truthy = read-only**, and the field stays visible.

### Combining Mechanisms

```json
{
  "component": "input-text",
  "label": "Counter-Offer Price",
  "dataPath": "$.review.counterOfferPrice",
  "properties": {
    "width": "col-6",
    "inputType": "number",
    "isHiddenDataPath": "='{$.review.decision}' !== 'Counter-Offer'",
    "renderWhen": "StepIsEditing",
    "readOnlyWhen": "='{$.review.status}' === 'Submitted'"
  }
}
```
This field appears only when decision is "Counter-Offer", only during editing, and becomes read-only once submitted.

## Component Types Reference

| Component | Default Width | Use for |
|---|---|---|
| `input-text` | `col-6` | Short text, names, IDs. For numbers set `inputType: "number"` |
| `richtext-input` | `col-12` | Long-form text with formatting |
| `date-picker` | `col-6` | Date/datetime fields. Use `showRange: true` for a date range |
| `input-select` | `col-6` | Dropdown. `options` is a plain `string[]` only |
| `check-box` | `col-6` | Boolean yes/no |
| `product-toggle-switch` | `col-6` | On/off toggles. **Preferred.** `switch` and `toggle` schemas also exist on the server but may behave differently |
| `attachments` | `col-12` | File uploads |
| `comments-box` | `col-12` | Comment threads |
| `data-grid` | `col-12` | Tabular data entry. **Prefer it over repeated fields** ("Name 1", "Name 2", …) |
| `search-things` | `col-6` | Entity lookup / reference to assets |
| `relatedWork-select` | `col-6` | Link to other work items |
| `section` | `col-12` | Top-level container for grouping fields |
| `subsection` | `col-12` | Nested container within a section |
| `banner` | `col-12` | Display-only information/instructions. Should not be the only component in a section |
| `signature` | `col-12` | Signature capture |
| `rating` | `col-6` | Star/numeric rating |
| `finance-section` | `col-12` | Financial data with automatic calculations |
| `user-invitation` | `col-6` | User/role assignment (`defaultValue: "initiator"` for the current user) |
| `input-location` | `col-6` | Location/address input with map |
| `timesheet-table` | `col-12` | Time & expense tracking table (see `references/timesheet-table.md`) |
| `image-readonly` | `col-12` | Read-only image display |
| `link-list` | `col-12` | List of links |
| `step-slider-v1` | `col-12` | Step progress indicator |

**Component Name Gotchas:**
- `input-select` NOT `select`. Its `options` is a plain `string[]`: objects render `[object Object]`, and `selectOptions` is not supported
- `relatedWork-select` NOT `related-work-select` — note the camelCase `W`
- `richtext-input` NOT `rich-text-input` — no second hyphen
- `input-location` NOT `location` — needs the `input-` prefix
- `timesheet-table` NOT `timesheet` — needs the `-table` suffix
- `date-picker` range mode is `showRange`, NOT `dateRange`

> The MCP server validates these on every component write, with two distinct
> behaviours. Known-wrong **names** (`select`, `rich-text`, `timesheet`, …) and
> `input-select` object-options / `selectOptions` are **rejected** with the
> canonical suggestion before anything is sent — trust the validation error, fix
> and resend. Fixable-but-wrong **properties** are instead **auto-corrected and
> reported as a warning** (the write still goes through): `dateRange` → `showRange`
> on a `date-picker`, and `input-number` → `input-text` + `inputType: number`.
> Check the returned warnings to see what was changed.

## AI Import (for data-grid and timesheet-table)

AI Import extracts data from a file into a table component. To enable it, add `{"name": "aiImport"}` to the component's `properties.toolbarActions`:

```json
{
  "component": "data-grid",
  "label": "Line Items",
  "properties": {"toolbarActions": [{"name": "aiImport"}]}
}
```

The user clicks **Import with AI** in the table toolbar, picks a mode, and uploads a file. The AI matches the extracted data to the table's column definitions, then validates and fills the table.

- **Import modes:** **Set Data** replaces all rows, **Append Data** adds rows, **Fill Data** fills empty values in existing rows.
- **Accepted files:** `.pdf, .csv, .xls, .xlsx, .jpg, .jpeg, .png, .webp`
- Timesheet-specific mapping is in `references/timesheet-table.md` § AI Import.

## search-things Component Reference

Component type: `search-things` | Default width: `col-6`

An asset lookup component that lets users search and select assets (entities) from published asset types. For every property, call `get_schema("components", "search-things")`. A new `search-things` shows the "create new asset" option in the search bar; set `addNewItem: false` to hide it.

### Basic Example

```json
{
  "component": "search-things",
  "label": "Select Vessel",
  "properties": {
    "width": "col-6",
    "thingType": "vessel",
    "multiSelect": "single",
    "returnAllEntitiesWhenEmptySearch": true,
    "dataPaths": ["$.displayName", "$.data.vesselDetails.imoNumber"],
    "entityDataFilter": {"status": ["active"]},
    "entityPropertiesToShowInSearchResults": [
      {"label": "IMO Number", "dataPath": "$.data.vesselDetails.imoNumber", "position": 1}
    ]
  }
}
```

`returnAllEntitiesWhenEmptySearch: true` makes the picker behave like a dropdown. `entityPropertiesToShowInSearchResults` adds columns to the search dropdown.

### Common Mistakes

1. **Wrong `thingType`** — must exactly match the `entityType` set on the asset type flow via `update_flow_properties`. Case-sensitive.
2. **Missing `$.data` prefix** — asset field data paths must be prefixed with `$.data` (e.g. `$.data.vesselDetails.name`), while top-level properties like `$.displayName` and `$.id` don't need the prefix.
3. **Using `autoPopulate` or `dataPathMapping`** — these properties don't exist on `search-things` and are silently ignored. `dataPaths` only controls which asset fields are fetched and stored under `.data`; it neither copies values to other fields nor sets the dropdown columns (`entityPropertiesToShowInSearchResults` does). To fill other fields, see § Populating a Field from a Selected Asset.

## data-grid Component Reference

Component type: `data-grid` | Default width: `col-12`

A tabular data entry component for multi-row data. For every grid property, column property, column `type`, and toolbar action, call `get_schema("components", "data-grid")`. Column validation uses the same rules as components; see `references/validation.md` § Data-Grid Column Validation.

### Basic Example

```json
{
  "component": "data-grid",
  "label": "Line Items",
  "properties": {
    "width": "col-12",
    "columns": [
      {"name": "product", "displayName": "Product", "type": "thing", "thingType": "product", "required": true, "flex": 2},
      {"name": "quantity", "displayName": "Qty", "type": "number", "required": true, "decimals": 0, "columnWidth": 100},
      {"name": "unitPrice", "displayName": "Unit Price", "type": "currency", "required": true, "decimals": 2},
      {"name": "loadPort", "displayName": "Load Port", "type": "select", "options": ["Singapore", "Rotterdam"]},
      {"name": "total", "displayName": "Total", "type": "currency", "decimals": 2,
       "value": "=quantity * unitPrice"}
    ]
  }
}
```

### Computed Columns

A column with a `value` property is **computed**: the cell renders the formula's result and is force-disabled for editing — no need to set `editable: false` or omit `defaultValue`. A column's `defaultValue` only seeds an editable cell on row creation; it is not a per-render formula.

The formula runs through `dynamicValue` in a per-row scope. The data-grid passes the row as `variables` (the mathjs scope), so the reliable shape uses **bare mathjs symbols** matching sibling column `name`s: `"=quantity * unitPrice"`.

**Common broken forms — all of these silently render `0`:**

| Broken | Why it fails | Use instead |
|---|---|---|
| `"defaultValue": "=quantity * unitPrice"` | `defaultValue` only seeds new rows; it isn't a per-render formula. The cell stays empty / 0. | Move the formula to `value`. |
| `"value": "={@row.quantity} * {@row.unitPrice}"` | `column.value` evaluation puts the row in `variables`, not `data['@row']`. The `{@row.x}` token resolver looks for `data['@row']['x']`, doesn't find it, substitutes literal `'0'`. | `"=quantity * unitPrice"` |
| `"value": "={quantity} * {unitPrice}"` | Bare `{name}` tokens look up `data.name` (form root), not the row. Substituted as `0`. | `"=quantity * unitPrice"` |
| `"value": "={$.quantity} * {$.unitPrice}"` | Works (the `$.`-prefixed tokens fall back to variables when not in data) but is verbose; not used in any SDK fixture. | Prefer bare symbols. |

Symbols resolve against the full row record, so a computed column can reference any sibling column. Declare referenced columns earlier in the array for readability.

> ⚠️ **Computed columns are render-only — the result is NOT persisted to the row record.** `column.value` is evaluated at render time for display only and does not write back to `row[column.name]`. Downstream consumers (dashboards reading `work.data.cart[i]`, exports, action conditions, other formulas) see `undefined` for the computed field. *How to apply:* if a value needs to be readable outside the grid, persist it via an action that writes the computed result back to the row, or compute it at the consumer (e.g. an aggregated chart's `fields[].dataPaths: "={$.quantity} * {$.unitPrice}"`).

### Common Mistakes

1. **Missing `name` on columns** — every column must have a unique `name` key for data storage.
2. **Using `headerName` instead of `displayName`** — the property is `displayName` (not `headerName`).
3. **Forgetting `type` on numeric columns** — without `type: "number"`, values are stored as strings.
4. **`options` format for select columns** — use a string array `["A", "B"]`, same as `input-select`.
