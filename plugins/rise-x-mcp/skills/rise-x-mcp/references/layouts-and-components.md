# Layouts & Components

## Contents

- [What is a Layout?](#what-is-a-layout)
- [Reading Layouts](#reading-layouts)
- [Draft/Publish](#draftpublish)
- [Component Hierarchy (Critical)](#component-hierarchy-critical)
- [input-select Options (Critical)](#input-select-options-critical)
- [Adding Components](#adding-components)
- [Updating a Component](#updating-a-component)
- [Replacing Section Components (Destructive)](#replacing-section-components-destructive)
- [Changing Component Type](#changing-component-type)
- [Deleting Components](#deleting-components)
- [Grid System & Column Widths](#grid-system--column-widths)
- [Conditional Rendering & Dynamic Visibility](#conditional-rendering--dynamic-visibility)
  - [`isHiddenDataPath` — Data-Driven Visibility](#mechanism-1-ishiddendatapath--data-driven-visibility)
  - [`renderWhen` — Step-State Visibility](#mechanism-2-renderwhen--step-state-visibility)
  - [`readOnlyWhen` — Conditional Editability](#mechanism-3-readonlywhen--conditional-editability)
  - [`condition` — Section and Component Gating](#mechanism-4-condition--section-and-component-gating)
  - [Expression Engine Reference (`dynamicValue()`)](#expression-engine-reference-dynamicvalue)
- [Component Types Reference](#component-types-reference)
- [date-picker Properties (Critical)](#date-picker-properties-critical)
- [AI Import (for data-grid and timesheet-table)](#ai-import-for-data-grid-and-timesheet-table)
- [search-things Component Reference](#search-things-component-reference)
- [data-grid Component Reference](#data-grid-component-reference)
  - [Computed Columns](#computed-columns)

## What is a Layout?

A **layout** is the UI form definition attached to a task in a flow. It contains a tree of **components** (form fields) organized inside **sections** (containers).

## Reading Layouts

### `get_layout(id: str)`
Full layout config including the complete component tree.

### `get_layout(id, format="model")`
Structured navigable model representation of the layout.

### `get_layout(layout_id, format="components")`
Components array only, with null values stripped. Lighter than the default format.

## Draft/Publish

Layouts are drafted and published as part of the flow lifecycle. The pattern is:

1. `create_flow_draft(flow_id)` → draft flow
2. `get_flow_steps(draft_flow_id)` → get layoutId
3. Check layout status: `get_layout(layoutId)` → check `dianaVersion.publishStatus`
   - If **"Published"** → call `create_layout_draft(layoutId, draft_flow_id)` → returns a new draft layout ID linked to the flow
   - If **"Draft"** → use layoutId as-is
4. Edit components on the draft layout
5. `publish_flow(draft_flow_id)` → publishes flow AND all linked layouts

### `create_layout_draft(layout_id: str, flow_id: str | None = None)`
Creates a draft of a layout and links it to the flow's publish chain.
- `flow_id` — the DRAFT flow GUID. Required to link the layout to the flow so `publish_flow` auto-publishes it.

**Do NOT** call this for new flows (`create_flow`) — their layouts are already in draft mode.

**CRITICAL — Draft ID Rule:** Returns a **new draft layout ID**. All subsequent component operations MUST use this draft layout ID.

**Do NOT** call `publish_layout` — it has been removed. `publish_flow` handles layout publishing automatically.

## Component Hierarchy (Critical)

```
Layout
  -> Section (parentId = "00000000-0000-0000-0000-000000000000")
     -> Components (parentId = section's ID)
```

- **Sections** are top-level containers. Their `parentId` is always the null GUID.
- **Non-section components** (input-text, select, etc.) MUST have a `parentId` pointing to a section.
- Orphan components (no parentId) will be rejected with an error.

## input-select Options (Critical)

**Only use `options` as a plain `string[]`. Do NOT use `selectOptions`. Do NOT put objects in `options`.**

```
CORRECT:
{"component": "input-select", "label": "Incoterms", "properties": {
  "width": "col-6",
  "options": ["CIF", "FOB", "CFR", "FCA", "DAP"]
}}

WRONG (objects in options — renders [object Object] in UI):
{"component": "input-select", "label": "Incoterms", "properties": {
  "options": [{"label": "CIF", "value": "CIF"}]
}}

WRONG (selectOptions is not supported):
{"component": "input-select", "label": "Incoterms", "properties": {
  "selectOptions": [{"value": "CIF", "label": "CIF"}]
}}
```

## Adding Components

### `add_components(layout_id, components, parent_section_id=None, after_component_id=None)`

**Parameters:**
- `components` — list of dicts. Minimum: `{"component": "input-text", "label": "Field Name"}`
- `parent_section_id` — section ID to add components into. Required for non-section components.
- `after_component_id` — position control. Null GUID = add at start. Omit = add at end.

**IDs are auto-generated** if missing from component dicts.

**Two approaches for proper parenting:**

**Option A: Separate calls**
```
# First: add section
add_components(layout_id, [
  {"component": "section", "label": "Details"}
])
# Note the section ID from response

# Then: add fields into section
add_components(layout_id, [
  {"component": "input-text", "label": "Name"},
  {"component": "date-picker", "label": "Date"}
], parent_section_id="section-guid-here")
```

**Option B: Single call (section first)**
```
add_components(layout_id, [
  {"component": "section", "label": "Details"},
  {"component": "input-text", "label": "Name"},
  {"component": "date-picker", "label": "Date"}
])
# Non-sections auto-parent to the first section in the same call
```

**Partial Failure Warning:** `add_components` batch calls may partially succeed before returning an error. If you get a 400 or other error, always call `get_layout(layout_id, format="components")` to check what was actually created before retrying. Delete unwanted duplicates with `delete_components`.

## Updating a Component

### `update_component(layout_id, component_id, component_data)`
PATCH semantics — only include changed fields. Unchanged fields are preserved.

Ensure `parentId` is correct:
- Section: `"00000000-0000-0000-0000-000000000000"`
- Non-section: the section's GUID

## Replacing Section Components (Destructive)

### `replace_section_components(layout_id, section_id, components)`

**WARNING:** Replaces ALL children of the specified section. Any component NOT in the list is permanently deleted.

Rules:
- Must include every component you want to keep
- Cannot include section-type components in the replacement list
- `section_id` cannot be the null GUID
- **Prefer `update_component` for single changes — it's safer**

## Changing Component Type

### Using `update_component(layout_id, component_id, {"component": "new_type"})`
Use `update_component(layout_id, component_id, {"component": "new_type"})` to change a component's type. Retains label, properties, and position. Only changes the type string.

Example types: `"input-text"`, `"date-picker"`, `"input-select"`, `"richtext-input"`, `"attachments"`, `"search-things"`, `"data-grid"`

## Deleting Components

### `delete_components(layout_id, component_ids: list[str])`
Delete one or more components by their GUIDs.

## Grid System & Column Widths

Rise-X uses a **12-column grid** (Bootstrap/MUI-style). Components sit inside a `Grid container` with `spacing={2}`. Each component's width is set via `properties.width`.

### Width Values

Format: `col-{1-12}` where the number is how many of 12 columns the component spans.

| Width | Span | Use for |
|---|---|---|
| `col-12` | 100% | Full-width: richtext, attachments, data-grid, signature, banner, sections |
| `col-6` | 50% | Half-width: text inputs, selects, date pickers, toggles — **this is the platform default** |
| `col-4` | 33% | Compact fields: status, priority, short codes, ratings |
| `col-3` | 25% | Very compact: toggles, small numeric fields, checkboxes |
| `col-8` | 67% | Main field paired with a col-4 side field |

### Default Widths by Component Type

The platform assigns these defaults when creating components:

| Default | Components |
|---|---|
| **`col-6`** | `input-text`, `input-select`, `date-picker`, `search-things`, `product-toggle-switch`, `check-box`, `rating` |
| **`col-12`** | `section`, `subsection`, `banner`, `richtext-input`, `attachments`, `signature`, `data-grid`, `finance-section`, `comments-box` |

### Setting Width

Pass `properties.width` when adding components:
```
{"component": "input-text", "label": "First Name", "properties": {"width": "col-6"}}
```

### Layout Design Guidelines

**Always aim for multi-column layouts** — don't default everything to `col-12`. Forms look better and are more usable when related short fields sit side-by-side.

**Row planning:** Components flow left-to-right within a section. Column values in a row should sum to 12 (or less — remaining space stays empty on the right).

**Recommended patterns:**

**Two-column (most common for data entry):**
```
[col-6: First Name]  [col-6: Last Name]
[col-6: Email]       [col-6: Phone]
[col-12: Description (richtext)]
```

**Asymmetric (main + sidebar):**
```
[col-8: Title]                [col-4: Priority]
[col-8: Assigned To]          [col-4: Due Date]
[col-12: Description (richtext)]
```

**Three-column (compact status fields):**
```
[col-4: Status]  [col-4: Priority]  [col-4: Severity]
```

**Mixed (logical grouping):**
```
[col-6: Category]       [col-6: Sub-Category]
[col-4: Priority]       [col-4: Severity]    [col-4: Impact]
[col-12: Description (richtext)]
[col-12: Attachments]
```

**Rules of thumb:**
- Short text fields (names, codes, IDs): `col-6` or `col-4`
- Dropdowns/selects: `col-6` or `col-4`
- Date pickers: `col-6` or `col-4`
- Rich text / descriptions: `col-12`
- Attachments, data grids, signatures: `col-12`
- Checkboxes, toggles, ratings: `col-3` or `col-4`
- Group related fields on the same row (e.g., First Name + Last Name)
- Put the most important field first and/or wider

### Section Properties

Sections support these layout-relevant properties:
- `collapsible`: `"none"` | `"collapsed"` | `"expanded"` — collapsible sections save vertical space
- `description`: section description text
- `showTitle`: show/hide section title

### Useful Component Properties

Properties are split between **top-level** (on the component dict) and **nested** (inside `properties`).

#### Top-Level Properties

| Property | Type | Effect |
|---|---|---|
| `required` | `boolean` | Mark field as required |
| `readOnly` | `boolean` | Make field read-only |
| `defaultValue` | `string` | Default value — supports expressions (see below) |

#### copyData / cloneData (inert flags — NOT duplicate control)

Components carry two boolean flags, `copyData` and `cloneData`, that appear in the component schema and default to `false`. Their names *suggest* they control what gets copied when a work item is duplicated — **they do not.** There is no runtime code (in `@diana/core` or the API) that reads them to drive the **Duplicate** button, so toggling them and republishing changes nothing about duplication.

If you want to control which fields a duplicate carries, configure `flowFeatures.createAndDuplicateWork.duplicate` (`includePaths`/`excludePaths`) at the **flow** level — see `references/managing-flows.md` § Duplicate & Create Work. Do not spend time on `copyData`/`cloneData` for this.

#### Properties Object (`properties.X`)

**Layout & Display:**

| Property | Type | Effect |
|---|---|---|
| `width` | `"col-{1-12}"` | Column span in 12-column grid |
| `placeholder` | `string` | Placeholder text inside input |
| `tooltip` | `string` | Help tooltip on hover |
| `showTitle` | `boolean` | Show/hide the component label |
| `showInReport` | `boolean` | Include in generated reports |

**Input Behavior:**

| Property | Type | Effect |
|---|---|---|
| `inputType` | `string` | Field type: `"text"` (default), `"number"`, `"password"`, `"link"`. `"email"` is NOT valid — the backend enum is text/number/password/link (email format is enforced via a validation rule, not inputType). |
| `decimals` | `number` | Decimal places for number inputs |
| `unitOfMeasure` | `string` | Unit label for numeric fields (e.g., "kg", "USD") |
| `readOnly` | `boolean` | Read-only (via properties) |

**Dynamic Visibility & State:**

| Property | Type | Effect |
|---|---|---|
| `hidden` | `boolean` | Static hide — always hidden |
| `isHiddenDataPath` | `string` | Dynamic hide — evaluated against work data. Hidden when truthy |
| `readOnlyWhen` | `string` | Dynamic read-only — expression evaluated at runtime |
| `renderWhen` | `string` | Step-state visibility: `"StepIsEditing"`, `"StepIsNotEditing"`, `"StepIsComplete"`, `"StepIsNotComplete"`, `"StepIsEditingAndIsComplete"` |

**Data Binding & Scoping:**

| Property | Type | Effect |
|---|---|---|
| `dataKey` | `string` | Which data object to bind to (default: `"data"`) |
| `defaultValueDataKey` | `string` | Data *context* the `defaultValue` expression resolves against — **not the value itself**. With an empty `defaultValue` it does nothing. Rarely needed: a `defaultValue` of `$.path` already resolves against the whole work. |
| `workId` | `string` | Bind to different work item. Supports `$.path` or literal ID |
| `onChange` | `string` | URL to call when value changes (supports `{$.field}` tokens) |
| `recalculateDefaultValueWhenDataPathChange` | `string[]` | Re-evaluate `defaultValue` when any watched path changes. **Only fires when `defaultValue` is a dynamic expression** (`$.…`, `=…`, or `{$.…}`) — a literal or empty default is never recalculated. |

**Component Linking (No-Code):**

| Property | Type | Effect |
|---|---|---|
| `sourceComponentId` | `string` | Link to another component |
| `sourceComponentRelation` | `"copy"` / `"sync"` | `copy` = default value from source, `sync` = shared data path |

#### Default Value Expressions

The `defaultValue` field supports:
- **Literal values:** `"Draft"`, `"0"`, `"true"`
- **Data path references:** `$.someField` — resolves to the value at that path
- **Mathematical expressions:** `={$.price} * {$.quantity}` — computed at runtime
- **Token substitution:** `{$.field}` — replaced with field value in strings
- **Special values:** `"utcNow"` — current UTC timestamp (useful for date-picker defaults)
- **Special value for user-invitation:** `"initiator"` — auto-populates with the work initiator/task assignee (see below)

#### User-Invitation Default Value: `"initiator"` (IMPORTANT)

For `user-invitation` (People Search) components, use `"initiator"` as the `defaultValue` to auto-populate with the current user. **There is NO `@me` token** in Rise-X.

**How it works:**
- The string `"initiator"` is a special token defined as `DEFAULT_INVITATION_VALUE` in the SDK
- At runtime, it resolves via `work.users[work.roleName]?.[0]?.email` — the first user in the current step's role
- This effectively populates the field with the person who initiated or is assigned to the task

**Correct usage:**
```json
{
  "component": "user-invitation",
  "label": "Requirement Initiator",
  "required": true,
  "defaultValue": "initiator",
  "dataPath": "$.requirement.initiator",
  "properties": {
    "width": "col-6",
    "multiSelect": "single"
  }
}
```

**All supported default value types for user-invitation:**
| Type | Example | Description |
|---|---|---|
| `"initiator"` | `"initiator"` | Current work initiator/task assignee |
| FromThisFlow | `$.someField` | Links to another component's data path |
| AssetProperty | (nested path) | Links to an asset search component's property |
| CustomValue | `"user@example.com"` | Fixed hardcoded email |
| Calculation | `={$.expr}` | Dynamic calculated value |

#### Populating a Field from a Selected Asset

To copy values out of an asset chosen in a `search-things` field into other fields, you configure the **target** fields — `search-things` itself has no copy/populate property. (`autoPopulate` does **not** exist; a `{from, to}` array on the picker is silently ignored.)

When an asset is selected, the **whole asset object the picker fetched** is stored at the picker's `dataPath`, with the asset's own fields nested under `.data`. So if the picker binds to `$.cmo`, the asset's `sites` field is at `$.cmo.data.sites` — **not** `$.cmo.sites`.

On each target field, set `defaultValue` to that path and watch the picker's path for changes:

```json
{
  "component": "search-things",
  "label": "CMO",
  "dataPath": "$.cmo",
  "properties": { "thingType": "sanofi-cmo", "width": "col-4" }
}
```
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
- **Empty `defaultValue` disables it.** Recalculation only runs when `defaultValue` is a dynamic expression (`$.…`, `=…`, or `{$.…}`). A blank or literal default is never recomputed — so setting `defaultValueDataKey` with an empty `defaultValue` populates nothing, because `defaultValueDataKey` is only the *context* a `defaultValue` expression resolves against, never the value itself. Put the `$.cmo.data.<field>` path in `defaultValue` and leave `defaultValueDataKey` unset.
- **Missing `.data`.** Use `$.cmo.data.<field>`; `$.cmo.<field>` resolves to nothing.
- **Field not fetched.** The path only resolves if the field is actually present in what the picker stored. Leave the picker's `dataPaths` unset/`null` (the default returns the full asset object), or, if you do set `dataPaths`, include the field you reference (e.g. `$.data.sites`).

## Conditional Rendering & Dynamic Visibility

Rise-X supports four mechanisms for conditionally showing/hiding or making fields read-only. The first three use the `dynamicValue()` engine — math.js expression syntax with token replacement. The fourth, `condition`, is what no-code-builder flows carry: a different syntax, the opposite polarity, and the only one that gates a whole **section**. Expect to meet it in any flow you did not author yourself.

### Mechanism 1: `isHiddenDataPath` — Data-Driven Visibility

**Purpose:** Hide/show a component based on work data values.
**Property location:** `properties.isHiddenDataPath`
**Evaluation:** Expression is evaluated; **truthy result = component IS hidden**.

**Expression syntax:**
```
"='{$.path.to.field}' === 'SomeValue'"     # String equality
"='{$.path.to.field}' !== 'SomeValue'"     # String inequality
"={$.numericField} > 100"                   # Numeric comparison
"='{$.type}' === 'A' ? true : false"        # Ternary conditional
"=size('{$.items}')"                        # Function call (truthy if > 0)
"=equalText('{$.field}', 'Value')"          # Case-insensitive comparison
```

**CRITICAL — Logic is inverted:** The expression returns `true` to **HIDE** the field. To **SHOW** a field only when a condition is met, use `!==` or negate the logic.

**Common pattern — Show field only when another field equals a specific value:**
```json
{
  "component": "input-text",
  "label": "Counter-Offer Price",
  "dataPath": "$.review.counterOfferPrice",
  "properties": {
    "width": "col-6",
    "isHiddenDataPath": "='{$.review.decision}' !== 'Counter-Offer'"
  }
}
```
This field is **visible only** when `$.review.decision` equals `"Counter-Offer"`. For all other values, it's hidden.

**More examples:**
```json
// Show only when price type is "Floating"
"isHiddenDataPath": "='{$.order.priceType}' !== 'Floating'"

// Show only when trade type is "Contract Nomination" AND price type is "Floating"
"isHiddenDataPath": "='{$.order.priceType}' === 'Floating' ? equalText('{$.inquiry.tradeType}','Contract Nomination') : true"

// Hide when a boolean flag is true
"isHiddenDataPath": "='{$.config.enableFeature}' === 'true'"

// Show only when a field has a value (hide when empty)
"isHiddenDataPath": "=size('{$.someField}') === 0"
```

**Available data context in expressions:**
- `{$.fieldName}` — Access any work data path
- `{@row.fieldName}` — Current row data, **only inside data-grid contexts that wrap the row into the data scope**: `isHiddenDataPath` and `readOnlyWhen` on a column, and any component nested in a row-detail panel. The token form has **no `$.` prefix** (the row lives at `data['@row']`, which `lodash.has` resolves correctly only when written as `{@row.field}`).
- `{@work.fieldName}` — Full work object, same wrapping rule and same no-`$.` shape.

> ⚠️ Data-grid **computed columns** (`column.value`) do *not* see `@row` in scope. The row there is passed as mathjs `variables`, not as `data['@row']`. Use bare symbols like `=quantity * unitPrice` instead — see [Computed Columns](#computed-columns).

### Mechanism 2: `renderWhen` — Step-State Visibility

**Purpose:** Show/hide a component based on the workflow step's state (editing vs. complete).
**Property location:** `properties.renderWhen`
**Values:** Enum string — no expression evaluation needed.

| Value | Shows when |
|---|---|
| `"StepIsEditing"` | Step is actively being edited |
| `"StepIsNotEditing"` | Step is NOT being edited (read-only view) |
| `"StepIsComplete"` | Step has been completed/submitted |
| `"StepIsNotComplete"` | Step has NOT yet been completed |
| `"StepIsEditingAndIsComplete"` | Step is being re-edited after completion |

**Examples:**
```json
// Show input field only when actively editing
{"properties": {"renderWhen": "StepIsEditing"}}

// Show read-only summary only after step is complete
{"properties": {"renderWhen": "StepIsComplete"}}

// Show re-edit warning when editing a completed step
{"properties": {"renderWhen": "StepIsEditingAndIsComplete"}}
```

### Mechanism 3: `readOnlyWhen` — Conditional Editability

**Purpose:** Make a field read-only based on data values, while keeping it visible.
**Property location:** `properties.readOnlyWhen`
**Evaluation:** Same expression engine as `isHiddenDataPath`; **truthy = read-only**.

```json
{
  "component": "input-text",
  "label": "Approved Price",
  "dataPath": "$.order.approvedPrice",
  "properties": {
    "readOnlyWhen": "='{$.order.status}' === 'Approved'"
  }
}
```

### Combining Mechanisms

You can combine multiple mechanisms on a single component:

```json
{
  "component": "input-text",
  "label": "Counter-Offer Price",
  "dataPath": "$.review.counterOfferPrice",
  "properties": {
    "width": "col-6",
    "inputType": "number",
    "unitOfMeasure": "USD/MT",
    "isHiddenDataPath": "='{$.review.decision}' !== 'Counter-Offer'",
    "renderWhen": "StepIsEditing",
    "readOnlyWhen": "='{$.review.status}' === 'Submitted'"
  }
}
```
This field: appears only when decision is "Counter-Offer", only during editing, and becomes read-only once submitted.

### Expression Engine Reference (`dynamicValue()`)

All `isHiddenDataPath` and `readOnlyWhen` expressions are evaluated by the `dynamicValue()` engine:

| Feature | Syntax | Example |
|---|---|---|
| Token replacement | `{$.path}` | `{$.order.status}` → actual value |
| String equality | `='{$.path}' === 'value'` | `='{$.status}' === 'Active'` |
| String inequality | `='{$.path}' !== 'value'` | `='{$.status}' !== 'Draft'` |
| Numeric comparison | `={$.path} > number` | `={$.quantity} > 100` |
| Ternary | `=cond ? a : b` | `='{$.type}' === 'A' ? true : false` |
| Functions | `=func(args)` | `=size('{$.items}')`, `=equalText('{$.a}','{$.b}')` |
| Math | `={$.a} * {$.b}` | `={$.price} * {$.qty}` |
| Null check | `=ifNull('{$.val}', 'default')` | Fallback value |
| DataGrid row (in `isHiddenDataPath` / `readOnlyWhen` / row-detail children) | `{@row.fieldName}` | No `$.` prefix. **Not for `column.value`** — see [Computed Columns](#computed-columns). |

**Example — well-structured component:**
```json
{
  "component": "input-text",
  "label": "Email Address",
  "required": true,
  "properties": {
    "width": "col-6",
    "placeholder": "name@company.com",
    "tooltip": "Primary contact email"
  }
}
```

**Example — dynamic visibility and default value:**
```json
{
  "component": "input-text",
  "label": "Overtime Rate",
  "defaultValue": "={$.baseRate} * 1.5",
  "properties": {
    "width": "col-4",
    "isHiddenDataPath": "='{$.isStandardShift}' === 'true'",
    "recalculateDefaultValueWhenDataPathChange": ["$.baseRate"]
  }
}
```

**Example — conditional form section (show counter-offer fields only when "Counter-Offer" selected):**
```json
[
  {
    "component": "input-select",
    "label": "Decision",
    "dataPath": "$.review.decision",
    "properties": {
      "width": "col-6",
      "options": ["Accept Offer", "Reject Offer", "Counter-Offer"]
    }
  },
  {
    "component": "input-text",
    "label": "Counter-Offer Price",
    "dataPath": "$.review.counterPrice",
    "properties": {
      "width": "col-6",
      "inputType": "number",
      "isHiddenDataPath": "='{$.review.decision}' !== 'Counter-Offer'"
    }
  },
  {
    "component": "input-text",
    "label": "Counter-Offer Volume",
    "dataPath": "$.review.counterVolume",
    "properties": {
      "width": "col-6",
      "inputType": "number",
      "isHiddenDataPath": "='{$.review.decision}' !== 'Counter-Offer'"
    }
  }
]
```

### Mechanism 4: `condition` — Section and Component Gating

**Purpose:** show or hide a component, **or a whole section**, based on work data.
**Property location:** top-level `condition` on the component or section record —
not under `properties`.
**Evaluation:** truthy result = the component or section **IS shown**. This is the
opposite polarity to `isHiddenDataPath`.

This is what flows authored in the no-code builder carry, and its expression
syntax differs from the `dynamicValue()` mechanisms above — no leading `=`, and
single quotes are doubled by the serialiser:

```
'{$.deliveryIncluded}' == 'true'                                   # toggle is on
'{$.request.typeOfDelivery}' == 'Entrega General - General Delivery'
'{$.a}' == 'X' or '{$.a}' == 'Y'                                   # `or`, not ||
false                                                              # never rendered
```

Three things to know before you read or write one:

- **The compared value is the whole option label**, not a code or an index. For
  an `input-select` that means the entire `Spanish - English` string, verbatim.
  Get one character wrong and the section silently never appears.
- **A section's `condition` gates every component inside it.** This is the
  structural mechanism; `isHiddenDataPath` and `renderWhen` are per-component.
  A real flow used three toggles and three selects, all with section conditions,
  to decide which of 24 attachment slots a person had to fill.
- **`condition: false` is deliberate configuration, not dead weight.** It is a
  common way to carry role bindings — a section of `user-invitation` components
  populated from an asset — on the layout without ever rendering them. Do not
  surface such a section, and do not delete it.

**`get_layout(format="summary")` does not return `condition`.** Summary gives
component, label and dataPath, which is the right way to orient in a large
layout. Read `format="components"` before you conclude anything about what a
layout asks for, or you will design against a flat field list that does not
exist. One layout read this way looked like a handful of fields and was in fact
98 components across 26 sections, almost all of them conditional.

**How the reveal happens at runtime.** Components carrying a `condition`
typically also set:

```
onChange: /api/v3/flow/execute/activity/{$.id}?refreshLayout=true
```

The platform re-executes the activity and returns a fresh layout, so visibility
is resolved server-side. A client rendering the layout itself must either make
that round trip on change or evaluate the same conditions locally — but it has
to know which, because the two behave differently on a slow connection.

## Component Types Reference

| Component | Default Width | Use for |
|---|---|---|
| `input-text` | `col-6` | Short text, names, numbers, IDs |
| `richtext-input` | `col-12` | Long-form text with formatting |
| `date-picker` | `col-6` | Date/datetime fields. Use `showRange: true` for date range selection |
| `input-select` | `col-6` | Single dropdown selection |
| `check-box` | `col-6` | Boolean yes/no |
| `product-toggle-switch` | `col-6` | On/off toggles. **Preferred.** `switch` and `toggle` schemas also exist on the server but may behave differently — use `product-toggle-switch` when creating components. |
| `attachments` | `col-12` | File uploads |
| `comments-box` | `col-12` | Comment threads |
| `data-grid` | `col-12` | Tabular data entry |
| `search-things` | `col-6` | Entity lookup / reference to assets |
| `relatedWork-select` | `col-6` | Link to other work items |
| `section` | `col-12` | Top-level container for grouping fields |
| `subsection` | `col-12` | Nested container within a section |
| `banner` | `col-12` | Display-only information/instructions |
| `signature` | `col-12` | Signature capture |
| `rating` | `col-6` | Star/numeric rating |
| `finance-section` | `col-12` | Financial data entry |
| `user-invitation` | `col-6` | User/role assignment |
| `input-location` | `col-6` | Location/address input with map |
| `timesheet-table` | `col-12` | Time & expense tracking table (see `references/timesheet-table.md`) |
| `image-readonly` | `col-12` | Read-only image display |
| `link-list` | `col-12` | List of links |
| `step-slider-v1` | `col-12` | Step progress indicator |

**Component Name Gotchas:**
- `input-select` NOT `select` — the server rejects `select` with the canonical suggestion
- `relatedWork-select` NOT `related-work-select` — note the camelCase `W`
- `richtext-input` NOT `rich-text-input` — no second hyphen
- `input-location` NOT `location` — needs the `input-` prefix
- `timesheet-table` NOT `timesheet` — needs the `-table` suffix

> The MCP server validates these on every component write, with two distinct
> behaviours. Known-wrong **names** (`select`, `rich-text`, `timesheet`, …) and
> `input-select` object-options / `selectOptions` are **rejected** with the
> canonical suggestion before anything is sent — trust the validation error, fix
> and resend. Fixable-but-wrong **properties** are instead **auto-corrected and
> reported as a warning** (the write still goes through): `dateRange` → `showRange`
> on a `date-picker`, and `input-number` → `input-text` + `inputType: number`.
> Check the returned warnings to see what was changed.

Use `get_schema("components", "type-name")` to discover all configurable properties for any type. See `references/schemas-and-compare.md`.

## date-picker Properties (Critical)

**Date range mode:** Use `showRange: true` to enable date range selection (start + end date). The correct property is `showRange`, NOT `dateRange`. On a `date-picker` the MCP server auto-corrects `dateRange` → `showRange` and returns an `auto_corrected` warning, so the field still works — but author it as `showRange` rather than relying on the fix-up.

```
CORRECT (date range):
{"component": "date-picker", "label": "Delivery Window", "properties": {
  "width": "col-6",
  "showRange": true
}}

WRONG (dateRange — server auto-corrects to showRange and warns):
{"component": "date-picker", "label": "Delivery Window", "properties": {
  "width": "col-6",
  "dateRange": true
}}
```

**Other useful date-picker properties:**

| Property | Type | Effect |
|---|---|---|
| `showRange` | `boolean` | Enable date range selection (start + end date). Default: `false` |
| `showTime` | `boolean` | Include time selection. Default: `false` |
| `dateFormat` | `string` | Display format (e.g. `"dd/MM/yyyy"`, `"MM/dd/yyyy"`) |
| `disablePastDates` | `boolean` | Prevent selecting past dates |
| `disableFutureDates` | `boolean` | Prevent selecting future dates |

## AI Import (for data-grid and timesheet-table)

AI Import enables intelligent data extraction from files (PDF, CSV, XLS/XLSX, JPG, PNG, WebP) into table components.

### Enabling AI Import

Add to the component's `toolbarActions` property:
```json
{
  "component": "data-grid",
  "label": "Line Items",
  "properties": {
    "toolbarActions": [{"name": "aiImport"}]
  }
}
```

Same for timesheet-table:
```json
{
  "component": "timesheet-table",
  "label": "Time & Expenses",
  "properties": {
    "toolbarActions": [{"name": "aiImport"}]
  }
}
```

### Import Modes

Users get three options when importing:
- **Set Data** — replace all existing rows
- **Append Data** — add new rows to existing data
- **Fill Data** — fill empty values in existing rows

### Accepted File Types
`.pdf, .csv, .xls, .xlsx, .jpg, .jpeg, .png, .webp`

### How It Works

The AI import system:
1. User clicks "Import with AI" button in the table toolbar
2. Selects import mode (Set/Append/Fill)
3. Uploads a file
4. AI extracts structured data from the file, matching it to the table's column definitions
5. For timesheet tables, it can also match extracted data to configured service options
6. Data is validated and populated into the table

### Timesheet-Specific Import

For timesheet tables, AI import automatically maps these columns:
- Name, Type, Rate, Quantity, Total
- Tax Rate (if `timesheetShowTax` is enabled)

It can also match imported items to service options using:
- `timesheetServiceOptionsDataPath` — path to load service options
- `timesheetServiceOptionsFilter` — filter rules for matching
- `timesheetServiceOptionsDataPathMapping` — field mapping for service options

## search-things Component Reference

Component type: `search-things` | Default width: `col-6`

An asset lookup component that lets users search and select assets (entities) from published asset types. Supports single or multi-select, auto-population of related fields, and custom search result columns.

### Key Properties

| Property | Type | Default | Description |
|---|---|---|---|
| `thingType` | string | (required) | The asset type identifier to search (e.g. `"vessel"`, `"customer"`). Must match an `entityType` from a published asset type. |
| `assetTypeOriginId` | GUID | null | Flow origin ID of the asset type. Optional — if provided, narrows the search scope. |
| `multiSelect` | `"single"` / `"multi"` | `"single"` | Allow selecting one or multiple assets. |
| `dataPaths` | string[] | `["$.displayName"]` | Data paths to fetch from the selected asset. Prefix asset data paths with `$.data` (e.g. `["$.data.vesselDetails.imoNumber", "$.displayName"]`). `null` returns the entire object. |
| `addNewItem` | boolean | `true` | Show option to create a new asset from the search bar. |
| `alwaysAddNewItem` | boolean | `false` | Show create option even when similar results are found. |
| `returnAllEntitiesWhenEmptySearch` | boolean | `false` | Auto-fetch all assets without typing. Makes it behave like a dropdown select. |
| `field` | string | null | Field on the target asset to search (e.g. `"imoNumber"`). Omit the `data.` prefix. |
| `searchBy` | string | null | Value to search by. Can be a fixed value or a data path like `$.path.to.value`. |
| `contains` | boolean | `true` | Whether search uses "contains" matching (vs exact). |
| `entityDataFilter` | object | null | Filter entities by data values. Format: `{"fieldName": ["value1", "value2"]}`. |
| `subscriptionNames` | string[] | null | Filter by subscription names. |
| `entityPropertiesToShowInSearchResults` | array | null | Custom columns in search results (see below). |
| `customLabelPath` | string | null | Custom data path for the display label. |
| `useEnvironmentAssets` | boolean | `false` | Search environment-level assets. |

### Custom Search Result Columns

Show additional asset data in the search dropdown:

```json
{
  "entityPropertiesToShowInSearchResults": [
    {"label": "IMO Number", "dataPath": "$.data.vesselDetails.imoNumber", "position": 1},
    {"label": "Flag State", "dataPath": "$.data.vesselDetails.flagState", "position": 2}
  ]
}
```

### Basic Example

```json
{
  "component": "search-things",
  "label": "Select Vessel",
  "properties": {
    "width": "col-6",
    "thingType": "vessel",
    "multiSelect": "single",
    "addNewItem": true,
    "dataPaths": ["$.displayName", "$.data.vesselDetails.imoNumber"]
  }
}
```

### Dropdown-Style (show all assets)

```json
{
  "component": "search-things",
  "label": "Customer",
  "properties": {
    "width": "col-6",
    "thingType": "customer",
    "returnAllEntitiesWhenEmptySearch": true,
    "multiSelect": "single",
    "addNewItem": false
  }
}
```

### Filtered Search with Custom Results

```json
{
  "component": "search-things",
  "label": "Active Vessels",
  "properties": {
    "width": "col-6",
    "thingType": "vessel",
    "entityDataFilter": {"status": ["active"]},
    "entityPropertiesToShowInSearchResults": [
      {"label": "IMO", "dataPath": "$.data.vesselDetails.imoNumber", "position": 1},
      {"label": "Type", "dataPath": "$.data.vesselDetails.vesselType", "position": 2}
    ]
  }
}
```

### Common Mistakes

1. **Wrong `thingType`** — must exactly match the `entityType` set on the asset type flow via `update_flow_properties`. Case-sensitive.
2. **Missing `$.data` prefix** — asset field data paths must be prefixed with `$.data` (e.g. `$.data.vesselDetails.name`), while top-level properties like `$.displayName` and `$.id` don't need the prefix.
3. **Using `autoPopulate` or `dataPathMapping`** — these properties don't exist on `search-things` and are silently ignored. `dataPaths` controls which asset fields the search backend returns (and are thus available under `.data` on the stored selection); it is **not** a copy-to-other-fields mechanism, and **not** the dropdown-column control (that's `entityPropertiesToShowInSearchResults`). To **auto-populate other fields** from the selected asset, configure those target fields with `defaultValue: "$.<picker>.data.<field>"` + `recalculateDefaultValueWhenDataPathChange` — see § Populating a Field from a Selected Asset.

## data-grid Component Reference

Component type: `data-grid` | Default width: `col-12`

A tabular data entry component for capturing structured, multi-row data. Supports column types, validation, sorting, grouping, pinning, totals, toolbar actions, and AI import.

### Grid-Level Properties

| Property | Type | Default | Description |
|---|---|---|---|
| `columns` | array | `[]` | Column definitions (see below). Required. |
| `editable` | boolean | `true` | Enable editing in the grid. |
| `addRows` | boolean | `true` | Allow adding new rows. |
| `autoHeight` | boolean | `true` | Auto-adjust grid height to content. |
| `sortable` | boolean | `true` | Default sortable value for all columns. |
| `groupable` | boolean | `true` | Default groupable value for all columns. |
| `reorderRows` | boolean | `true` | Allow drag-to-reorder rows. |
| `reorderColumns` | boolean | `true` | Allow drag-to-reorder columns. |
| `rowActions` | string[] | `["duplicate", "delete"]` | Row action buttons. Options: `"duplicate"`, `"delete"`, `"new-inquiry"`. |
| `disableColumnMenu` | boolean | `false` | Hide column context menus. |
| `disableColumnPinning` | boolean | `false` | Prevent column pinning. |
| `toolbarActions` | array | null | Toolbar buttons (see below). |
| `currencyCode` | string | null | ISO currency code for currency columns. |

### Column Definition

Each entry in the `columns` array defines a table column:

| Property | Type | Default | Description |
|---|---|---|---|
| `name` | string | (required) | Column data key (used in data storage). |
| `displayName` | string | (required) | Column header text. |
| `type` | string | `"string"` | Column type (see types below). |
| `description` | string | null | Header tooltip text. |
| `required` | boolean | null | Required when editing. |
| `editable` | boolean | `true` | Column is editable. |
| `sortable` | boolean | `true` | Column can be sorted. |
| `filterable` | boolean | `true` | Column can be filtered. |
| `groupable` | boolean | `true` | Column can be grouped. |
| `hide` | boolean | null | Hide column from view. |
| `flex` | int | null | Flex width factor. |
| `columnWidth` | int | null | Fixed width in pixels. |
| `minWidth` | int | null | Minimum width in pixels. |
| `defaultValue` | string | null | Default cell value (literal, `$.path`, or `=`-prefixed formula). Used to seed an editable cell on row creation. **Not the place to put a computed-column formula** — use `value` (below). |
| `value` | string | null | **Computed-column formula.** When set, the cell is non-editable and re-evaluated on every render against the current row. Bare mathjs symbols matching sibling column `name`s are resolved against the row scope (e.g. `"=quantity * unitPrice"`). See [Computed Columns](#computed-columns) below. |
| `placeholder` | string | null | Placeholder text. |
| `options` | string[] | null | Options for `select` type columns. |
| `decimals` | int | null | Decimal places for number/currency/percentage. |
| `format` | string | null | Date format string for `date` columns. |
| `showRange` | boolean | null | Date range mode for `date` columns. |
| `showTime` | boolean | null | Time selection for `date` columns. |
| `thingType` | string | null | Asset type for `thing` columns. |
| `dataPaths` | string[] | null | Data paths for `thing` columns. |
| `link` | string | null | URL template for `link` columns. |
| `linkLabel` | string | null | Display label for `link` columns. |
| `operation` | string | null | Default aggregation: `"sum"`, `"average"`, `"count"`, `"min"`, `"max"`. |
| `readOnlyWhen` | string | null | Dynamic read-only expression. |
| `validation` | array | null | Validation rules (same format as component validation). |

### Column Types

| Type | Use for |
|---|---|
| `string` | Plain text (default) |
| `number` | Numeric values |
| `date` | Date/datetime values |
| `boolean` | Checkbox |
| `booleanSwitch` | Toggle switch |
| `select` | Dropdown from options |
| `selectTags` | Multi-tag selection |
| `currency` | Monetary values (uses `currencyCode`) |
| `percentage` | Percentage values |
| `thing` | Asset search (uses `thingType`) |
| `work` | Work item reference |
| `link` | Hyperlink |
| `user-link` | User profile link |
| `rating` | Star rating |
| `status` | Status badge |
| `badge` | Label badge |
| `toggleGroup` | Toggle button group |
| `text` | Multi-line text |
| `array` | Array of values |
| `inlineActions` | Action buttons in row |

### Toolbar Actions

Add toolbar buttons above the grid:

```json
{
  "toolbarActions": [
    {"name": "aiImport"},
    {"name": "exportAsCSV"}
  ]
}
```

Available actions: `"aiImport"` (AI-powered data import), `"createWork"`, `"openWork"`, `"createWorkFromRelated"`, `"exportAsCSV"`.

### Basic Example

```json
{
  "component": "data-grid",
  "label": "Line Items",
  "properties": {
    "width": "col-12",
    "columns": [
      {"name": "description", "displayName": "Description", "type": "string", "required": true, "flex": 2},
      {"name": "quantity", "displayName": "Qty", "type": "number", "required": true, "decimals": 0, "columnWidth": 100},
      {"name": "unitPrice", "displayName": "Unit Price", "type": "currency", "required": true, "decimals": 2},
      {"name": "total", "displayName": "Total", "type": "currency", "decimals": 2,
       "value": "=quantity * unitPrice"}
    ]
  }
}
```

### With Asset Search Column

```json
{
  "component": "data-grid",
  "label": "Cargo Details",
  "properties": {
    "columns": [
      {"name": "product", "displayName": "Product", "type": "thing", "thingType": "product", "required": true, "flex": 2},
      {"name": "quantity", "displayName": "Quantity (MT)", "type": "number", "decimals": 2},
      {"name": "loadPort", "displayName": "Load Port", "type": "select", "options": ["Singapore", "Rotterdam", "Houston"]}
    ]
  }
}
```

### Computed Columns

A column with a `value` property is **computed**: the cell renders the formula's result and is force-disabled for editing — no need to set `editable: false` or omit `defaultValue`.

The formula runs through `dynamicValue` in a per-row scope. The reliable shape uses **bare mathjs symbols** matching sibling column `name`s:

```json
{
  "type": "currency",
  "name": "lineTotal",
  "displayName": "Line Total",
  "value": "=quantity * unitPrice",
  "decimals": 2,
  "currencyCode": "USD"
}
```

Symbols inside the formula (`quantity`, `unitPrice`) are looked up against the row directly, because the data-grid passes the row as `variables` (i.e. as the mathjs scope). This is the canonical pattern: reference sibling column `name` values directly with bare mathjs symbols such as `=quantity * unitPrice`.

**Common broken forms — all of these silently render `0`:**

| Broken | Why it fails | Use instead |
|---|---|---|
| `"defaultValue": "=quantity * unitPrice"` | `defaultValue` only seeds new rows; it isn't a per-render formula. The cell stays empty / 0. | Move the formula to `value`. |
| `"value": "={@row.quantity} * {@row.unitPrice}"` | `column.value` evaluation puts the row in `variables`, not `data['@row']`. The `{@row.x}` token resolver looks for `data['@row']['x']`, doesn't find it, substitutes literal `'0'`. | `"=quantity * unitPrice"` |
| `"value": "={quantity} * {unitPrice}"` | Bare `{name}` tokens look up `data.name` (form root), not the row. Substituted as `0`. | `"=quantity * unitPrice"` |
| `"value": "={$.quantity} * {$.unitPrice}"` | Works (the `$.`-prefixed tokens fall back to variables when not in data) but is verbose; not used in any SDK fixture. | Prefer bare symbols. |

Computed columns can reference any column declared **earlier in the same `columns` array** — symbols resolve against the full row record, not just preceding columns, but ordering still matters for human readability.

> ⚠️ **Computed columns are render-only — the result is NOT persisted to the row record.** `column.value` is evaluated at render time for display only, so it recalculates whenever the cell paints and does not write back to `row[column.name]`. Downstream consumers (dashboards reading `work.data.cart[i]`, exports, action conditions, other formulas) see `undefined` for the computed field — only `quantity` and `unitPrice` are stored, not the computed `lineTotal`. *How to apply:* if a value needs to be readable outside the grid (e.g. summed by a chart, exported, used in a validation rule on another component), persist it via an action that writes the computed result back to the row, or compute it at the consumer (e.g. an aggregated chart's `fields[].dataPaths: "={$.quantity} * {$.unitPrice}"`).

### With Validation

```json
{
  "columns": [
    {
      "name": "sku",
      "displayName": "SKU",
      "type": "string",
      "validation": [
        {"type": "error", "rules": "required|regExp:^[A-Z]{3}-\\d{4}$", "message": "SKU must be AAA-0000 format"}
      ]
    },
    {
      "name": "quantity",
      "displayName": "Qty",
      "type": "number",
      "validation": [
        {"type": "error", "rules": "required|numeric|min:1", "message": "Qty must be at least 1"}
      ]
    }
  ]
}
```

### Common Mistakes

1. **Missing `name` on columns** — every column must have a unique `name` key for data storage.
2. **Using `headerName` instead of `displayName`** — the property is `displayName` (not `headerName`).
3. **Forgetting `type` on numeric columns** — without `type: "number"`, values are stored as strings.
4. **`options` format for select columns** — use a string array `["A", "B"]`, same as `input-select`.
