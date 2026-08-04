---
name: rise-x-mcp
description: "You MUST invoke this skill before calling any Rise-X MCP server tool. Use when the user explicitly mentions Rise-X, rise-x, EOP, or a Rise-X ecosystem, or when established conversation context makes the request a Rise-X operation. Covers workflows, layouts/components, dashboards, work items, assets, search/data schemas, actions/approvals, pipelines/exports, integrations/external APIs, and app deployment. Do not trigger for generic workflow or software requests without Rise-X context, or when only editing the Rise-X MCP server Python implementation."
metadata:
  author: rise-x
---

# Rise-X MCP Server Skill

For background on Rise-X, the EOP platform, value proposition, and target industries, see `references/about-rise-x.md`.

## Domain Model

**Two-tier setup:**
- **Deployment** (dev/test/prod) — fixed at MCP server startup, determines API URL
- **Ecosystem** (workspace name like `MyEcosystem`) — set per-session, determines data scope. An ecosystem is a collection of workflows, work items, assets, and users with access controls for collaboration.

**Entity hierarchy:**
```
Flow (workflow template)
  -> Steps (high-level phases — Kanban columns, e.g. "Order Request", "Fulfilment")
     -> Tasks (discrete units of work assigned to specific parties)
        -> Layout (UI form definition)
           -> Sections (logical groupings of fields)
              -> Components (form fields — text, dates, signatures, etc.)
```

- **Work Item** = running instance of a flow (a live transaction). Created with `create_work(flow_id)`, progresses through steps via `submit_work`. After each task, the platform automatically notifies the party responsible for the next task.
- **Asset Type** (ThingType) = a **template** that defines the structure and fields for a category of entities. Think of it as a *class* or *schema*. Example: a "Vessel" asset type defines fields like IMO number, flag state, tonnage. Under the hood, an asset type is a Flow with `FlowResourceType = Entity`. Created with `create_flow(flow_resource_type="Entity")`, configured with flow/layout/component tools, then published.
- **Asset** (Thing / entity instance) = a **record** created from an asset type. Think of it as an *instance* of the class. Example: "Pacific Explorer" is a Vessel asset with IMO 9876543. Created/edited via the 3-step workflow pattern (`create_asset` → `update_work_data` → `submit_work`).
- **Activities** = workflow automations that run when an action fires (e.g. `StartWork`, `StartCrossEcosystemWork`, `PublishData`, `SendEmail`). They hang off an **action**, configured with `manage_activity` (add/update/delete); discover types + schemas via `get_schema("activities")`. See `references/actions-and-statuses.md` § Activities.

## Multi-Party Workflow Design (Summary)

Rise-X workflows are **multi-party permissioned forms** — each task is a handover between parties. The golden rule: **never create consecutive tasks for the same party** — use sections within one task instead. After each task, the next party is automatically notified.

For the full design guide with examples, component selection strategy, and asset flow patterns, see `references/building-workflows.md`.

## Pre-Flight Checklist

Before any operation, always:
1. `get_active_ecosystem()` — check if ecosystem is set
2. If null: `list_ecosystems()` then `set_active_ecosystem("name")`
3. Every tool except session tools requires an active ecosystem

## Response Envelopes & Verification (server ≥ Trust & Signal release)

Every mutation tool returns a compact YAML **envelope** instead of echoing the
full entity:

```yaml
ok: true|false
action: <tool name>
entity: flow|layout|component|work|asset|dashboard|integration|…
id: <primary id>                  # e.g. the DRAFT id for create_*_draft
ids: {flowOriginId: …, draftId: …, layoutId: …}   # ids for follow-up calls
…summary fields…                  # added components, steps, status, context
changed: [keys confirmed applied] # PATCH-style tools
counts: {requested: N, persisted: M}
warnings: [{code, path, message}] # ← READ THIS EVERY TIME
hint: <one-liner>
error: {code, message, hint}      # failures (code like http_403, validation)
```

**Rules:**
1. **Always check `warnings[]` after every mutation.** The API silently drops
   unknown properties (200 OK, nothing persisted); the server now diffs your
   request against the saved entity and reports drops as
   `dropped_property` / `dropped_item` / `component_missing`. A warning means
   that part of your request did NOT land — do not report success to the user.
2. `value_differs` warnings are informational (server-side normalisation).
3. Need the raw payload? Pass `response_format="full"` — it arrives under
   `result:` with the warnings still attached.
4. Error envelopes carry a `code` (`http_403`, `validation`, `transient`, …)
   and usually a `hint` with the fix. A **request-side** `validation` error
   (caught before sending) means nothing was sent — fix and retry. An
   **API-side** 4xx can still *partially* persist (pitfall #16, the
   `add_components` partial-failure case, in `references/common-pitfalls.md`;
   likewise the attachments-column "400 that commits"): when
   a write error is ambiguous, follow the hint and verify the target entity
   (`get_layout` / `get_work`) before assuming nothing changed.
5. Paginated lists (`list_work`, `list_assets`) return
   `{items: […], returned, skip, limit, hasMore, nextSkip}` — pass `nextSkip`
   as `skip` for the next page.

## Flow Sub-Resources — use the dedicated manage_* tools

`update_flow_properties` REJECTS these keys up front (the API used to discard
them silently): `dataPipelines`/`rules` → **`manage_pipeline`**,
`export(s)` → **`manage_export`**, `chains` → **`manage_chain`**,
`cardLayout` → **`manage_card_layout`**, `relatedFlows` →
**`manage_related_flow`**, `columns` → `manage_columns`, `steps`/`actions`/
`userInvites` → step tools. Each manage_* tool follows the same pattern:
`action="list"|"add"|"update"|"delete"` (+ `unset=True` to REMOVE the provided
keys on update) and verifies the saved item in its response.

## Draft/Publish Lifecycle

### Editing a published flow or asset type:
1. `create_flow_draft(original_id)` → envelope `id` is the draft flow ID
2. `get_flow_steps(draft_flow_id)` → get layoutId
3. Check if layout needs drafting:
   - `get_layout(layoutId, format="summary")` → check `publishStatus`
   - If **"Published"** → `create_layout_draft(layoutId, draft_flow_id)` → envelope `id` is the draft layout ID and `sections` carries the section GUIDs. Use the draft ID for all component operations.
   - If **"Draft"** → use the layoutId as-is
4. Edit components on the (draft) layout ID
5. `publish_flow(draft_flow_id)` → publishes flow AND all linked layouts; returns the new published id + `ids.flowOriginId`

### Creating a new flow:
1. `create_flow(...)` → flow + layouts already in draft mode; response carries the steps with layout ids
2. `add_step(...)` → response includes `newStep.defaultSectionId` — pass it straight to `add_components` as `parent_section_id` (no `get_layout` round-trip)
3. Edit components directly
4. `publish_flow(flow_id)`

**Do NOT** call `publish_layout` — it does not exist. `publish_flow` handles layout publishing automatically.

**Draft ID Rule:** `create_flow_draft` returns a new draft flow ID, and `create_layout_draft` returns a new draft layout ID. Use these draft IDs for all subsequent edits and publish — the original IDs are read-only and respond with 403 Forbidden.

## Component Type Names

The server validates component types on every write: known-wrong names
(`select`→`input-select`, `rich-text`/`textarea`→`richtext-input`,
`location`→`input-location`, `timesheet`→`timesheet-table`,
`datepicker`→`date-picker`, `text-input`→`input-text`, `checkbox`→`check-box`,
`step-slider`→`step-slider-v1`, `comments`→`comments-box`, …) are **rejected
with the canonical name** before anything is sent, and unknown types produce a
warning. Canonical names you'll use most: `input-text`, `input-select`,
`richtext-input`, `date-picker`, `check-box`, `relatedWork-select` (camelCase W),
`input-location`, `timesheet-table`, `product-toggle-switch`, `data-grid`,
`attachments`, `comments-box`, `search-things`, `section`. Full list:
`get_schema("components")`.

**There is no `input-number`.** A numeric field is `input-text` with
`properties.inputType: "number"` — the server auto-corrects `input-number` (and
`number`) to that for you. Valid `inputType` values: `text` (default),
`number`, `password`, `link` (NOT `email`).

## Routing Table

| User wants to... | Load reference |
|---|---|
| Understand what Rise-X / the EOP is | `references/about-rise-x.md` |
| Set up ecosystem/workspace | `references/session-and-environment.md` |
| View, edit, version, or delete a flow | `references/managing-flows.md` |
| Build a new workflow or asset flow from scratch | `references/building-workflows.md` |
| Create a new asset type or workflow from scratch | `references/managing-asset-types.md` |
| Create, edit, list, or delete asset instances | `references/managing-assets.md` |
| Create, get, submit, or update work items | `references/managing-work-items.md` |
| Add/edit/remove UI form fields or layouts | `references/layouts-and-components.md` |
| Configure how a work item / asset renders as a card on a board (`cardConfig` — title, subtitle, status pill, items, icon) | `references/card-layout.md` |
| Add actions (Reject, Send Back), attach activities (StartWork, Start Cross Ecosystem Work), or configure grid columns/statuses | `references/actions-and-statuses.md` |
| Add validation rules to fields or data-grid columns | `references/validation.md` |
| Author or debug expression-valued properties (chart formulas, tooltip values, action conditions, validation rules, computed labels) | `references/dynamicValue.md` |
| Configure timesheet-table components | `references/timesheet-table.md` |
| Configure relationships between work items or assets | `references/relationships.md` |
| Look up schemas or compare across ecosystems | `references/schemas-and-compare.md` |
| Create, edit, or delete performance dashboards (sections, containers, charts, metric-cards, aggregated `reportData`) | `references/dashboards.md` |
| **Find / filter / search / sort works, flows, companies, or assets by ANY condition** (status, date, assignee, `data.*` field value, etc.) — never use `list_*` + client-side filtering | `references/advanced-search.md` |
| Discover a flow's / asset type's `data.*` paths before composing a Work or Asset search | `references/advanced-search.md` |
| Configure CSV / report exports of work data (incl. expanding data-grid rows into one row each) | `references/exports.md` |
| Configure data pipelines / flow rules (auto data operations on watched-data change — set/copy/map paths, alerts, dates) | `references/pipelines.md` |
| Deploy a federated-app bundle, release a new app version, or list/update/delete apps in the registry (`request_bundle_upload` → PUT zip → `deploy_app`; `list_apps`, `get_app`, `update_app`, `delete_app`) | `references/managing-apps.md` |
| Configure, import, inspect, or test an integration (external API + endpoints called from `JsonEndPoint` activities) | `references/integrations.md` for read-only inspection (domain shape, lifecycle, pitfalls). For any *mutating* integration call (`update_integration`, `update_integration_endpoint`, `import_integrations`, `delete_integration`, `delete_integration_endpoint`, `test_integration_endpoint`, `test_integration_endpoint_in_flow`), load `references/integration-authoring.md` BEFORE calling the tool — it enforces the slot-filling + secrets protocol. Vendor/auth worked recipes (API key, Bearer, OAuth2 client credentials, webhooks, Postman imports) live in `references/integration-patterns.md`. |
| Look up a tool's exact name, signature, or per-tool caveats | `references/tool-inventory.md` |
| Diagnose an error, a warning, or an edit that didn't take effect | `references/troubleshooting.md` |
| Check the numbered pitfall list before a first write, or after a surprising result | `references/common-pitfalls.md` |

## Common Pitfalls

56 traps with the fix for each, in 63 numbered slots (7 are retired stubs kept so
the numbering stays stable — other references cite these entries by number) —
draft/publish lifecycle, ID confusion, component and dashboard authoring,
search/filter semantics, exports, card layouts:
`references/common-pitfalls.md`. Read it before the first write of a session;
re-check the cited entry whenever a mutation warns or a tile renders empty.

**Irreversible — read the entry before you call either:**
`replace_section_components` permanently deletes every component you omit, so
send the full keep-set (pitfall #6); `delete_integration` is a physical delete
with no undo.

## Troubleshooting

Symptom → cause → fix table for every known error (403 after drafting, "Step not
found", `no_ecosystem`, empty `list_work`, `NO DATA` charts, blank workboard):
`references/troubleshooting.md`. Go there the moment a call fails or a change
doesn't show up in the UI.

## Tool Inventory

All 85 tools grouped by category (Session, Flow, Flow Structure, Flow Config,
Columns, Layout, Component, Schema, Compare, Work, Search, Asset, Apps,
Dashboard, Integration), with signatures and per-tool caveats:
`references/tool-inventory.md`. Load it when you need an exact tool name or
argument shape.
