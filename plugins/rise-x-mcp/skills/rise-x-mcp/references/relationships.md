# Configuring Relationships

## Contents

- [What are Relationships?](#what-are-relationships)
- [The Bidirectional Rule](#the-bidirectional-rule)
- [Relationship Types](#relationship-types)
  - [Work ↔ Asset](#work--asset)
  - [Work ↔ Work](#work--work)
  - [Work ↔ Asset with Data Sync](#work--asset-with-data-sync)
- [`relatedFlows` Schema Reference](#relatedflows-schema-reference)
  - [`publishDataDirection` Values](#publishdatadirection-values)
  - [`stepStatuses` Values](#stepstatuses-values)
  - [`stepNames` Matching](#stepnames-matching)
  - [`workFilters` — Finding Target Work](#workfilters--finding-target-work)
  - [`options` Object](#options-object)
- [Data Operations (PublishDataOperation)](#data-operations-publishdataoperation)
- [`relatedWork-select` Component](#relatedwork-select-component)
- [Enabling the Related Work Panel](#enabling-the-related-work-panel)
- [How to Read Relationships](#how-to-read-relationships)
- [Complete Example: Configure Work ↔ Asset Relationship](#complete-example-configure-work--asset-relationship)
- [Common Mistakes](#common-mistakes)

## What are Relationships?

**Relationships** connect work items to other work items or to assets. They are configured in the `relatedFlows` array on each flow's configuration. When a work item is submitted, the flow engine evaluates the `relatedFlows` config and establishes links to matching target work items or assets.

**Key concept:** relationship links are **two-sided by default**. When a work is submitted, the engine evaluates the `relatedFlows` entries on **that work's own flow**; a matching entry writes relationship records onto **both** items — source and target — unless `isOneWayRelationship: true`. So a single side's config is enough to create both ends of a link. Mirror the config on both flows anyway (see below): links are only established/refreshed when the **configured** side submits, and named related-work lookups only resolve on a flow that has its own matching entry.

**Not every relationship comes from `relatedFlows`.** Runtime activities — e.g. `StartCrossEcosystemWork` with `sourceRelationshipName`/`targetRelationshipName` set — create relationship records directly when they run, with no `relatedFlows` config on either flow. A populated Related Work tab therefore does not imply any `relatedFlows` entries exist.

## The Bidirectional Rule

When Flow A needs to relate to Flow B via `relatedFlows`, configure **both sides** — not because one entry can't create the link (it can, on both items — see Key concept above), but so the link is established no matter which side submits, and so related-work lookups by relationship name resolve from either flow:

```
Flow A relatedFlows:                     Flow B relatedFlows:
┌──────────────────────────┐             ┌──────────────────────────┐
│ name: "AtoB"             │             │ name: "BtoA"             │
│ flowName: "Flow B"       │◄───────────►│ flowName: "Flow A"       │
│ sourceName: "Flow A"     │             │ sourceName: "Flow B"     │
│ targetName: "Flow B"     │             │ targetName: "Flow A"     │
│ workFilters:             │             │ workFilters:             │
│   targetPath: $.id       │   MIRRORED  │   targetPath: $.ref.id   │
│   valuePath: $.ref.id    │◄───────────►│   valuePath: $.id        │
└──────────────────────────┘             └──────────────────────────┘
```

**Rules:**
- Give both flows a `relatedFlows` entry pointing to each other (one side alone works only when that side submits, and only that side can query by name)
- `sourceName` on Flow A corresponds to the relationship name stored on Flow A's work items
- `targetName` on Flow A corresponds to the relationship name stored on Flow B's work items
- `workFilters` are mirrored — `targetPath` and `valuePath` swap between sides
- Exception: if `isOneWayRelationship: true`, only one side is created

**How the engine finds targets (priority order):**
1. **Data path** — `options.targetWorkIdDataPath` extracts a work ID directly from source data
2. **Existing relationship** — checks if a relationship by name already exists
3. **Work filters** — queries target flow's work items matching `targetPath = valuePath`
4. **Create new** — if `createWorkIfNotFound: true`, creates a new work item in the target flow

## Relationship Types

### Work ↔ Asset

The most common pattern — linking a workflow to an asset type (e.g. an invoice workflow to a contract/quote asset).

**Example (anonymized from a real deployment): "Check Quotes (AI)" workflow ↔ "Quotes" asset**

On the **workflow side** (Check Quotes):
```json
{
  "name": "RelatedInvoices",
  "flowName": "MyEcosystem/Logistics/CostManagement/Quotes",
  "sourceName": "Check Quotes (AI)",
  "targetName": "Quotes",
  "workFilters": [
    { "targetPath": "$.id", "valuePath": "$.invoice.quote.id" }
  ],
  "publishDataDirection": "ToRelatedEntity",
  "options": {
    "includeData": false,
    "includeAttachments": false,
    "addRelationship": true,
    "isOneWayRelationship": false,
    "inviteCurrentUserToTargetWork": false
  }
}
```

On the **asset side** (Quotes):
```json
{
  "name": "RelatedQuoteInvoices",
  "flowName": "MyEcosystem/Logistics/Work/MonthlyWorkFlow~copy001~copy002~copy002",
  "sourceName": "Quotes",
  "targetName": "Check Quotes (AI)",
  "workFilters": [
    { "targetPath": "$.invoice.quote.id", "valuePath": "$.id" }
  ],
  "publishDataDirection": "ToRelatedEntity",
  "options": {
    "includeData": false,
    "includeAttachments": false,
    "addRelationship": true,
    "isOneWayRelationship": false,
    "inviteCurrentUserToTargetWork": false
  }
}
```

Notice the `workFilters` are **mirrored** — `targetPath` and `valuePath` swap.

### Work ↔ Work

Linking two workflow types — e.g. a monthly summary workflow to daily work items.

**Example (anonymized from a real deployment): Monthly workflow ↔ Daily workflow**

On the **monthly side**:
```json
{
  "name": "MonthlyToDailyWorkRelationship",
  "flowName": "MyEcosystem-sandbox/Logistics/Work/DailyWorkFlow",
  "sourceName": "MyEcosystem-sandboxMonthlytoDaily",
  "targetName": "Target",
  "publishDataDirection": "ToRelatedEntity",
  "stepStatuses": ["InProgress"],
  "options": {
    "includeData": true,
    "includeAttachments": true,
    "addRelationship": true,
    "isOneWayRelationship": false,
    "inviteCurrentUserToTargetWork": true
  }
}
```

### Work ↔ Asset with Data Sync

When a relationship also needs to push data to the related entity, use `operations` with `dataMapping`.

**Example (anonymized from a real deployment): Quotes asset → Monthly workflow (with data mapping)**

```json
{
  "name": "MonthlyToContractAssetRelationship",
  "flowName": "MyEcosystem/Logistics/Work/MonthlyWorkFlow",
  "sourceName": "MyEcosystemMonthlyFlow",
  "targetName": "Target",
  "publishDataDirection": "ToRelatedEntity",
  "stepStatuses": ["InProgress"],
  "operations": [
    {
      "sectionName": "creatework/startwork",
      "dataMapping": [
        { "fromPath": "$.displayName", "toProperty": "displayName" },
        { "fromPath": "$.id", "toProperty": "id" },
        { "fromPath": "$.ratesTable", "toProperty": "rates" },
        { "fromPath": "$.contractAsset.contractor.displayName", "toProperty": "contractor.displayName" }
      ],
      "updateType": "Set",
      "targetPath": "$.contracts",
      "dataType": "Object"
    }
  ],
  "options": {
    "includeData": true,
    "includeAttachments": true,
    "addRelationship": true,
    "isOneWayRelationship": false,
    "inviteCurrentUserToTargetWork": true
  }
}
```

This pushes selected fields from the asset into `$.contracts` on the target work item.

## `relatedFlows` Schema Reference

Each entry in the `relatedFlows` array has these properties:

| Property | Type | Description |
|---|---|---|
| `name` | string | Unique name for this relationship config |
| `flowName` | string | Full path of the target flow (e.g. `"Ecosystem/Category/FlowName"`) |
| `sourceName` | string | Relationship name stored on **this** flow's work items |
| `targetName` | string | Relationship name stored on the **target** flow's work items |
| `publishDataDirection` | enum | Direction data flows (see below) |
| `workFilters` | array | Matching rules to find target work items |
| `stepStatuses` | array | Step states the target work must be in (see below) |
| `stepNames` | array | Internal `stepName` values of the target steps, not display labels (see below) |
| `exactStepNameMatch` | bool | Compare the whole step name instead of a suffix (default `false`; see below) |
| `createWorkIfNotFound` | bool | Create a new work item if no match found |
| `operations` | array | Data mapping operations (see Data Operations) |
| `attachmentOperations` | array | Attachment sync rules (`sourceFolder` → `destinationFolder`) |
| `options` | object | Relationship behavior options (see below) |

### `publishDataDirection` Values

| Value | Behavior |
|---|---|
| `ToRelatedEntity` | Push data from source to target (most common) |
| `FromRelatedEntity` | Pull data from target back to source |
| `ToSelf` | Apply data within the same work item |
| `BetweenRelatedEntities` | Bidirectional data sync between already-related entities |

### `stepStatuses` Values

Step states of the target work item that count as a match, from `DianaStepState`: `NotStarted`, `Created`, `New`, `InProgress`, `Rework`, `Complete`, `Skipped`, `Cancelled`, `Declined`, `Deleted`. Omitting the property is not the same as listing every state: the config initialiser (`RelatedWorkConfig`) supplies `["InProgress"]`. Pass it explicitly, as the examples in this file do.

### `stepNames` Matching

Entries are internal `stepName` values (see `managing-flows.md`), not display labels: `["supplierOffer"]`, never `["Supplier Offer"]`. The related-work lookup uses the **first** entry only; further entries only widen the access the target flow is granted on this flow's steps, where `"*"` means every step.

The first entry matches the target work's step names as a case-insensitive **suffix** (Mongo regex `.*<name>$`, flag `i`). For a slash-form name pass the trailing segment: `"Generated-<guid>"` matches `"UntitledStep/Generated-<guid>"`, while a leading segment alone (`"UntitledStep"`) does not. Set `exactStepNameMatch: true` to compare the whole name instead. This rule is separate from `ByStepName` action routing (`actions-and-statuses.md`, Common Mistakes #2), which needs the exact name.

### `workFilters` — Finding Target Work

Each filter is a `{ targetPath, valuePath }` pair:
- `targetPath` — JSON path on the **target** work item's data to match against
- `valuePath` — JSON path on the **source** work item's data to get the match value

```json
{ "targetPath": "$.id", "valuePath": "$.invoice.quote.id" }
```
Means: find a target work item where its `$.id` equals the source's `$.invoice.quote.id`.

### `options` Object

| Property | Type | Default | Description |
|---|---|---|---|
| `addRelationship` | bool | true | Create the relationship link |
| `isOneWayRelationship` | bool | false | Only create source→target (skip reverse) |
| `includeData` | bool | false | Include work data when publishing |
| `includeAttachments` | bool | false | Sync attachments between related items |
| `includeAcl` | bool | false | Sync access control lists |
| `removeExisting` | bool | false | Remove previous relationships before creating new |
| `closePrevious` | bool | false | Close previous related work items |
| `inviteCurrentUserToTargetWork` | bool | false | Auto-invite the submitting user to the target work |

## Data Operations (PublishDataOperation)

When `operations` are configured, data is mapped from source to target during relationship establishment.

| Property | Type | Description |
|---|---|---|
| `dataMapping` | array | Field mappings: `{ fromPath, toProperty }` |
| `targetPath` | string | JSON path on target where mapped data is written |
| `updateType` | enum | How to write: `Set`, `Push`, `Merge`, `Replace`, `Pull`, `Unset`, `AddToSet`, `Increment`, `Decrement` |
| `dataType` | enum | Type of data being written: `Object`, `Array`, `String`, `Integer`, etc. |
| `sectionName` | string | Optional: task section name for the update |
| `excludeProperties` | array | Fields to exclude from mapping |

**`dataMapping` entries:**
- `fromPath` — JSON path on source work data (e.g. `"$.displayName"`)
- `toProperty` — property name on the target object (e.g. `"displayName"`)

## `relatedWork-select` Component

The `relatedWork-select` component renders a UI widget that lets users manually select related work items from within a form.

**Component name:** `relatedWork-select` (camelCase W — NOT `related-work-select`)

### Key Properties

| Property | Type | Description |
|---|---|---|
| `flowName` | string | Target flow path to search for related work |
| `targetName` | string | Relationship target name |
| `sourceName` | string | Relationship source name |
| `targetPath` | string | Path to match on target work data |
| `valuePath` | string | Path to match on current work data |
| `targetStoragePath` | string | Where source work data is stored on target |
| `targetStorageKey` | string | Key for matching within the storage path |
| `numberOfDays` | int | Days to look back for related work (default: 1) |
| `multiSelect` | string | `"single"` (default) or `"multi"` |
| `extendedRelatedWork` | bool | Show additional columns in the selector |
| `extendedRelatedWorkProperties` | array | Column definitions: `[{ label, value }]` |

## Enabling the Related Work Panel

For the "Related Work" tab to appear in the work item side panel, the flow's `flowFeatures.leftPanel.tabs` must include `{"tab": "relatedWork"}`. Without the tab, configured relationships still function at the data level (links, data sync, invitations) — only the side-panel browsing UI is missing. `relatedWork` is work-item-only: for a Work↔Asset relationship, put the browsing tab on the work flow and keep the asset flow's `contents` tab. The canonical reference for `leftPanel` (all five tab values, `defaultTab`, per-tab `label`, publish-mode caveat) is `references/managing-flows.md` § Left Panel — three traps from there apply directly here:

- **`tabs` REPLACES the default set** (`workflow` + `activities`) — sending only `{"tab": "relatedWork"}` silently removes the Workflow and Activities tabs. Always list every tab you want visible.
- **`flowFeatures` writes need the draft→publish cycle** — a write to a published flow id returns 200 but persists nothing.
- **`flowFeatures` is a whole-object write** — read-modify-write or you clobber sibling features.

```
draft_id = create_flow_draft(flow_id)
features = get_flow_config(draft_id, path="properties.flowFeatures")  # read existing
update_flow_properties(draft_id, {
  "flowFeatures": {
    # Preserve every sibling feature returned by the read above.
    "leftPanel": {
      "tabs": [
        {"tab": "workflow"},
        {"tab": "activities"},
        {"tab": "relatedWork"}
      ]
    }
  }
})                                                                    # expect changed: [flowFeatures]
publish_flow(draft_id)                       # re-resolve the new published id via list_flows
```

**If the tab doesn't appear after publishing:** work flows default to `publishMode: "DoNotUpdateOpenItems"`, so already-open works stay pinned to their creation-time flow version — the new tab shows only on works created **after** the publish. Verify with a fresh work item.

## How to Read Relationships

Use `get_flow_config(flow_id)` — the response includes a `relatedFlows` array with all configured relationships. Each entry shows the target flow, filter rules, data operations, and options.

The `relationships` array (separate from `relatedFlows`) contains runtime relationship instances on work items — these are populated by the engine (from `relatedFlows` evaluation at submit, or directly by runtime activities such as `StartCrossEcosystemWork`), not configured manually. The Related Work tab renders from this per-work array, which is why it can be populated even on flows with no `relatedFlows` entries.

## Complete Example: Configure Work ↔ Asset Relationship

Goal: Link an "Invoice" workflow to a "Contracts" asset type so each invoice references its contract.

**Step 1: Identify the flows**
```
# Get the Invoice workflow config
get_flow_config(invoice_flow_id)
# Note the flow name: "MyEcosystem/Finance/Invoices"

# Get the Contracts asset type config
get_flow_config(contracts_asset_id)
# Note the flow name: "MyEcosystem/Finance/Contracts"
```

**Step 2: Draft both flows**
```
invoice_draft = create_flow_draft(invoice_flow_id)
contracts_draft = create_flow_draft(contracts_asset_id)
```

> The writes in Steps 3–4 are shown against flows with **no existing config**. On real flows, **read-modify-write `flowFeatures`** (§ Enabling the Related Work Panel above) so sibling features are preserved. Add or update each relationship with `manage_related_flow`; its updates are partial PATCHes, so omit fields you are not changing.

**Step 3: Configure the Invoice side**
```
manage_related_flow(invoice_draft, action="add", related_flow={
      "name": "InvoiceToContract",
      "flowName": "MyEcosystem/Finance/Contracts",
      "sourceName": "Invoices",
      "targetName": "Contracts",
      "publishDataDirection": "ToRelatedEntity",
      "workFilters": [
        { "targetPath": "$.id", "valuePath": "$.invoice.contractId" }
      ],
      "options": {
        "addRelationship": true,
        "isOneWayRelationship": false,
        "includeData": false,
        "includeAttachments": false,
        "inviteCurrentUserToTargetWork": false
      }
})

update_flow_properties(invoice_draft, {
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

**Step 4: Configure the Contracts side (mirror)**
```
manage_related_flow(contracts_draft, action="add", related_flow={
      "name": "ContractToInvoice",
      "flowName": "MyEcosystem/Finance/Invoices",
      "sourceName": "Contracts",
      "targetName": "Invoices",
      "publishDataDirection": "ToRelatedEntity",
      "workFilters": [
        { "targetPath": "$.invoice.contractId", "valuePath": "$.id" }
      ],
      "options": {
        "addRelationship": true,
        "isOneWayRelationship": false,
        "includeData": false,
        "includeAttachments": false,
        "inviteCurrentUserToTargetWork": false
      }
})
```

**Step 5: Publish both**
```
publish_flow(invoice_draft)
publish_flow(contracts_draft)
```

## Common Mistakes

1. **Configuring only one side** (`relatedFlows` mechanism) — a single entry does create both ends of the link, but only when the **configured** side's work is submitted, and related-work lookups by relationship name return empty on a flow without its own matching entry. Mirror the config on both flows unless one-sided establishment is intentional. (Relationships created at runtime by activities such as `StartCrossEcosystemWork` are exempt — they need no `relatedFlows` entries at all.)
2. **Mismatched `sourceName`/`targetName`** — the `sourceName` on Flow A should correspond to the `targetName` on Flow B. Swapping them breaks the bidirectional link.
3. **Forgetting to mirror `workFilters`** — `targetPath` and `valuePath` must be swapped between the two sides. If Flow A filters `targetPath: $.id, valuePath: $.ref.id`, Flow B must filter `targetPath: $.ref.id, valuePath: $.id`.
4. **Missing `relatedWork` tab** — the Related Work panel won't appear unless `flowFeatures.leftPanel.tabs` includes `{"tab": "relatedWork"}` on the flow.
5. **Wrong `flowName`** — must be the full flow path (e.g. `"Ecosystem/Category/FlowName"`), not the display name.
6. **Using `update_flow_properties` for `relatedFlows`** — it rejects that key before sending the request. Use `manage_related_flow` instead; its updates are partial PATCHes, so omitted fields are preserved.
7. **Forgetting to publish both flows** — both flows must be published after configuration changes.
8. **Using `related-work-select` instead of `relatedWork-select`** — the component name uses camelCase W.
