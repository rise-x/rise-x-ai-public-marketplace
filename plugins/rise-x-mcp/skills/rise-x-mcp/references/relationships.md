# Configuring Relationships

## What are Relationships?

**Relationships** connect work items to other work items or to assets. They are configured in the `relatedFlows` array on each flow's configuration. When a work item is submitted, the flow engine evaluates the `relatedFlows` config and establishes links to matching target work items or assets.

**Key concept:** relationship links are **two-sided by default**. When a work is submitted, the engine evaluates the `relatedFlows` entries on **that work's own flow**; a matching entry writes relationship records onto **both** items — source and target — unless `isOneWayRelationship: true`. So a single side's config is enough to create both ends of a link. Mirror the config on both flows anyway (see below): links are only established/refreshed when the **configured** side submits, and named related-work lookups only resolve on a flow that has its own matching entry.

**Not every relationship comes from `relatedFlows`.** Runtime activities — e.g. `StartCrossEcosystemWork` with `sourceRelationshipName`/`targetRelationshipName` set — create relationship records directly when they run, with no `relatedFlows` config on either flow. A populated Related Work tab therefore does not imply any `relatedFlows` entries exist.

## The Bidirectional Rule

When Flow A needs to relate to Flow B via `relatedFlows`, configure **both sides**, so the link is established no matter which side submits, and related-work lookups by relationship name resolve from either flow:

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
- `sourceName` on Flow A is the relationship name stored on Flow A's work items; `targetName` on Flow A is the name stored on Flow B's. So Flow A's `sourceName` is Flow B's `targetName`.
- `workFilters` are mirrored — `targetPath` and `valuePath` swap between sides
- Exception: if `isOneWayRelationship: true`, only one side is created

**How the engine finds targets (priority order):**
1. **Data path** — `options.targetWorkIdDataPath` extracts a work ID directly from source data
2. **Existing relationship** — checks if a relationship by name already exists
3. **Work filters** — queries target flow's work items matching `targetPath = valuePath`
4. **Create new** — if `createWorkIfNotFound: true`, creates a new work item in the target flow

## Relationship Types

### Work ↔ Asset

The most common pattern — linking a workflow to an asset type (e.g. an invoice workflow to a contract asset). The worked example is [Complete Example: Configure Work ↔ Asset Relationship](#complete-example-configure-work--asset-relationship).

### Work ↔ Work

Linking two workflow types — e.g. a monthly summary workflow to daily work items. This entry, on the monthly side, matches daily works still `InProgress`, copies data and attachments across, and invites the submitting user to the daily work:

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

When a relationship also needs to push data to the related entity, add `operations` with `dataMapping`. This entry pushes selected fields from a Quotes asset into `$.contracts` on the monthly work item:

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
        { "fromPath": "$.ratesTable", "toProperty": "rates" },
        { "fromPath": "$.contractAsset.contractor.displayName", "toProperty": "contractor.displayName" }
      ],
      "updateType": "Set",
      "targetPath": "$.contracts",
      "dataType": "Object"
    }
  ],
  "options": { "includeData": true, "addRelationship": true }
}
```

`fromPath` reads the source work's data; `toProperty` names the property on the object written at `targetPath`.

## `relatedFlows` Schema Reference

The full entry shape, including `options`: `get_schema("flows", "related-flow")`. The `operations` entry shape: `get_schema("flows", "publish-data-operation")`. The `attachmentOperations` entry shape: `get_schema("flows", "publish-attachment-operation")`; each entry copies attachments from `sourceFolder` to `destinationFolder`. The rules below are the ones the schema does not explain.

### `options` Object

The schema types every option as a nullable boolean and gives no default or meaning:

| Property | Default | Effect |
|---|---|---|
| `addRelationship` | true | Create the relationship link |
| `isOneWayRelationship` | false | Create source→target only (skip the reverse record) |
| `includeData` | false | Include work data when publishing |
| `includeAttachments` | false | Sync attachments between the related items |
| `includeAcl` | false | Sync access control lists |
| `removeExisting` | false | Remove previous relationships before creating the new one |
| `closePrevious` | false | Close previously related work items |
| `inviteCurrentUserToTargetWork` | false | Invite the submitting user to the target work |

### `publishDataDirection` Values

- `ToRelatedEntity`: push data from source to target (most common)
- `FromRelatedEntity`: pull data from target back to source
- `ToSelf`: apply data within the same work item
- `BetweenRelatedEntities`: sync data both ways between already-related entities

### `stepStatuses` Values

Step states of the target work item that count as a match, from `DianaStepState`: `NotStarted`, `Created`, `New`, `InProgress`, `Rework`, `Complete`, `Skipped`, `Cancelled`, `Declined`, `Deleted`. Omitting the property is not the same as listing every state: the config initialiser (`RelatedWorkConfig`) supplies `["InProgress"]`. Pass it explicitly, as the examples in this file do.

### `stepNames` Matching

Entries are internal `stepName` values, not display labels: `["supplierOffer"]`, never `["Supplier Offer"]`. Read them from `get_flow_steps(flow_id)` on the **target** flow; its `stepName` column is the value to use. The related-work lookup uses the **first** entry only; the others are delegation entries (next section). To match without delegating beyond the matched step, pass one entry.

The first entry matches the target work's step names as a case-insensitive **suffix** (Mongo regex `.*<name>$`, flag `i`). For a slash-form name pass the trailing segment: `"Generated-<guid>"` matches `"UntitledStep/Generated-<guid>"`, while a leading segment alone (`"UntitledStep"`) does not. A suffix can match more than one step (`"Step_1"` is a suffix of `"Step_11"`) and the lookup returns at most one work item, so when you have the full name, pass it and set `exactStepNameMatch: true`. This rule is separate from `ByStepName` action routing (`actions-and-statuses.md`, Common Mistakes #2), which needs the exact name.

### `stepNames` and Step Delegation

Every entry, the first included, also grants the target flow access on the source work item for the steps of **this** flow whose `stepName` ends with the entry (case-insensitive). `"*"` grants it for every step. Entries after the first do only this; they never widen the match.

| `stepNames` | Match | Access granted on this work item |
|---|---|---|
| `["supplierOffer"]` | target work at a step ending `supplierOffer` | this flow's steps ending `supplierOffer` |
| `["supplierOffer", "buyerReview"]` | same as above, not `buyerReview` | steps ending `supplierOffer` or `buyerReview` |
| `["supplierOffer", "*"]` | same as above | every step of this flow |

Neither example in this file passes `stepNames`; both rely on `workFilters` or `stepStatuses` alone.

### `workFilters` — Finding Target Work

Each filter is a `{ targetPath, valuePath }` pair: `targetPath` is read on the **target** work item, `valuePath` on the **source** work item. `{ "targetPath": "$.id", "valuePath": "$.invoice.quote.id" }` finds the target work whose `$.id` equals the source's `$.invoice.quote.id`.

## Enabling the Related Work Panel

For the "Related Work" tab to appear in the work item side panel, the flow's `flowFeatures.leftPanel.tabs` must include `{"tab": "relatedWork"}`. Without the tab, configured relationships still function at the data level (links, data sync, invitations) — only the side-panel browsing UI is missing. `relatedWork` is work-item-only: for a Work↔Asset relationship, put the browsing tab on the work flow and keep the asset flow's `contents` tab. `references/managing-flows.md` § Left Panel is the reference for `leftPanel`; its traps apply here: `tabs` replaces the default set, so list every tab you want; `flowFeatures` writes need a draft; and `flowFeatures` is a whole-object write, so read it first.

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
publish_flow(draft_id)                       # the envelope carries the new published id
```

**If the tab doesn't appear after publishing:** work flows default to `publishMode: "DoNotUpdateOpenItems"`, so already-open works stay pinned to their creation-time flow version — the new tab shows only on works created **after** the publish. Verify with a fresh work item.

The `relatedWork-select` form component lets users pick related work items by hand. Its schema is `get_schema("components", "relatedWork-select")`; `multiSelect` takes the string `"single"` (default) or `"multi"`, not a boolean; the component name to add is in `references/layouts-and-components.md`.

## How to Read Relationships

`manage_related_flow(flow_id, "list")` or `get_flow_config(flow_id)` returns the configured `relatedFlows` entries.

The `relationships` array (separate from `relatedFlows`) contains runtime relationship instances on work items — these are populated by the engine (from `relatedFlows` evaluation at submit, or directly by runtime activities such as `StartCrossEcosystemWork`), not configured manually. The Related Work tab renders from this per-work array, which is why it can be populated even on flows with no `relatedFlows` entries.

## Complete Example: Configure Work ↔ Asset Relationship

Goal: Link an "Invoice" workflow to a "Contracts" asset type so each invoice references its contract.

```
# 1. Read both flows' full names (flowName needs the path, not the display name)
get_flow_config(invoice_flow_id)       # "MyEcosystem/Finance/Invoices"
get_flow_config(contracts_asset_id)    # "MyEcosystem/Finance/Contracts"

# 2. Draft both flows
invoice_draft = create_flow_draft(invoice_flow_id)
contracts_draft = create_flow_draft(contracts_asset_id)

# 3. Invoice side
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
# Then add the relatedWork tab to the invoice flow (§ Enabling the Related Work Panel).

# 4. Contracts side (mirror: names swapped, workFilters swapped)
manage_related_flow(contracts_draft, action="add", related_flow={
      "name": "ContractToInvoice",
      "flowName": "MyEcosystem/Finance/Invoices",
      "sourceName": "Contracts",
      "targetName": "Invoices",
      "publishDataDirection": "ToRelatedEntity",
      "workFilters": [
        { "targetPath": "$.invoice.contractId", "valuePath": "$.id" }
      ],
      "options": { "addRelationship": true, "isOneWayRelationship": false }
})

# 5. Publish both
publish_flow(invoice_draft)
publish_flow(contracts_draft)
```

To change an entry later, call `manage_related_flow(draft, "update", related_flow_id, related_flow={...})` with only the keys you change. The tool merges them into the stored entry; a list you send (`workFilters`, `operations`) replaces the stored list.

## Common Mistakes

1. **Configuring only one side** (`relatedFlows` mechanism) — a single entry does create both ends of the link, but only when the **configured** side's work is submitted, and related-work lookups by relationship name return empty on a flow without its own matching entry. Mirror the config on both flows unless one-sided establishment is intentional. (Relationships created at runtime by activities such as `StartCrossEcosystemWork` are exempt — they need no `relatedFlows` entries at all.)
2. **Mismatched `sourceName`/`targetName`** — the `sourceName` on Flow A should correspond to the `targetName` on Flow B. Swapping them breaks the bidirectional link.
3. **Forgetting to mirror `workFilters`** — if Flow A filters `targetPath: $.id, valuePath: $.ref.id`, Flow B must filter `targetPath: $.ref.id, valuePath: $.id`.
4. **Missing `relatedWork` tab** — the Related Work panel won't appear unless `flowFeatures.leftPanel.tabs` includes `{"tab": "relatedWork"}` on the flow.
5. **Wrong `flowName`** — must be the full flow path (e.g. `"Ecosystem/Category/FlowName"`), not the display name.
6. **Forgetting to publish both flows** — both flows must be published after configuration changes.
