# Card Layout (`cardConfig`)

How a work item (or asset/entity) renders as a **card** on a board / kanban view.
Managed with the `manage_card_layout` tool (never via `update_flow_properties`,
which silently drops `cardLayout`). Discover the raw schema any time with
`get_schema("flows", "card-layout")`.

## Contents

- [Where the card layout lives](#where-the-card-layout-lives)
- [Two renderers: work card vs entity card](#two-renderers-work-card-vs-entity-card)
- [How values resolve — dataPath vs literal](#how-values-resolve--datapath-vs-literal)
- [The scalar-leaf rule (Critical)](#the-scalar-leaf-rule-critical)
- [`cardConfig` field reference](#cardconfig-field-reference)
- [The status pill — `statusText` + `statusClassName`](#the-status-pill--statustext--statusclassname)
- [`items[]`](#items)
- [`icon` (entity cards only)](#icon-entity-cards-only)
- [Fields accepted but ignored](#fields-accepted-but-ignored)
- [Editing the card layout](#editing-the-card-layout)
- [Worked example](#worked-example)

## Where the card layout lives

The card layout is a **flow sub-resource** (one per flow), not part of a step
layout. Its shape (`CardLayout`):

```yaml
id: <guid>                 # server-managed; leave as 00000000-... 
title: Card Layout         # REQUIRED string — an internal name, NOT the visible card title
displayName: <string?>
selected: <bool>
cardConfig:                # ← everything visible lives here
  card: default
  version: 1
  title: $.newOpportunity.client.displayName
  subTitle: [$.workCode, $.newOpportunity.opportunityTitle]
  secondaryLabel: Final Value
  secondaryValue: $.newOpportunity.estimatedValue
  statusText: $.newOpportunity.opportunityType.displayName
  statusClassName: ''
  statusChain: ''
  items:
    - {label: Partner, value: $.newOpportunity.associatedPartner.displayName, color: currentColor, icon: '', dateFormat: ''}
  icon: {imageUrl: $.data.icon.imageUrl, color: '#6DC360', shape: $.data.icon.icon}
```

⚠️ The top-level `title` (on `CardLayout`) is an internal label — usually the
literal string `"Card Layout"`. The **visible** card heading is
`cardConfig.title`. Setting the top-level `title` alone changes nothing on screen.

## Two renderers: work card vs entity card

The same `cardConfig` schema drives two different card components, chosen by the
board type — not by any field in the config:

| Board | Renderer | Renders `icon`? |
|---|---|---|
| Workboard / kanban of **work items** | work card | **No** — `icon` is resolved but not drawn |
| Board of **assets / people (entities)** | entity card | **Yes** |

So `icon.*` only shows on entity/asset boards. On a work-item workboard it is
inert.

## How values resolve — dataPath vs literal

Every string value in `cardConfig` is passed through the `dynamicValue` engine
(see `references/dynamicValue.md`). Whether a value is a **dataPath** or a
**literal** is inferred from the string, not declared per-field:

- Starts with `$.` → a JSONPath **dataPath**, resolved against the work item
  (both the work root and its `data`). E.g. `$.workCode`, `$.newOpportunity.client.displayName`.
- Contains `{$...}` tokens → interpolated.
- Starts with `=` → a math/formula expression.
- Anything else → rendered **literally** (e.g. the static label `Final Value`).

## The scalar-leaf rule (Critical)

A dataPath slot MUST resolve to a **scalar** (string / number), i.e. a leaf —
never a whole object or array.

A `search-things` / entity-reference field stores the **whole referenced object**
at its dataPath: `{id, displayName, imageUrl, data, …}` (the asset's own fields
are nested under `.data`; see `references/layouts-and-components.md` §
search-things Component Reference and `references/dashboards.md` § "What the
aggregator can actually see"). Pointing a card slot at the bare reference field
therefore hands the renderer an object. Consequences differ by slot — and two of
them are full-board outages, not soft failures.

**Two crash-capable slots — both take the ENTIRE board down.** The error is
thrown during render and there is no card-level error boundary, so one malformed
card collapses the whole view to "Unexpected Application Error":

- **`statusText`** (status-pill label) is rendered **raw** as a React child — an
  object/array throws **React error #31** ("Objects are not valid as a React
  child").
- **`statusClassName`** (status-pill color) is tested with `value.includes('#')`
  — a non-string has no `.includes`, throwing **`TypeError: value.includes is
  not a function`**. (When empty/unset it safely defaults to `muted`; it only
  crashes when set to a path that resolves to an object/array.)

**Soft-degrade slots — no crash, but silently wrong.** A non-string coerces to
`---` (or is dropped): `title`, `subTitle[]`, `secondaryValue`, `items[].value`.
Still always point them at a scalar leaf.

> **Internal provenance note (Rise-X maintainers).** Verified against
> `@diana/core` (rise-x-sdk-core — a private repo, not part of this
> marketplace) at v1.1.0: the status pill renders in `WorkStatus.tsx` (raw
> `statusText` label + the `value.includes('#')` color test), fed by
> `getWorkStatus` / `getWorkStatusColor` in `workUtils.ts`; the soft-degrade
> coercion is `sanitizeText` in `WorkCard.tsx`. This crash-vs-`---` split
> depends on those renderers — re-verify here if they change.

**Fix / rule:** always append the scalar leaf. Use
`$.newOpportunity.opportunityType.displayName`, not
`$.newOpportunity.opportunityType`. For an asset's own field use the `.data`
prefix: `$.picker.data.<field>`.

**ACL trap:** the crash only fires for users whose visible set includes a work
item where the offending field is actually **populated as an object**. Legacy
rows that still store a plain string (e.g. before the field was changed to a
picker) render fine, and a viewer who can't see any affected card sees nothing
wrong — so this class of bug routinely slips past casual testing by an admin or
another company's user.

## `cardConfig` field reference

| Field | Type | Kind | Renders as |
|---|---|---|---|
| `card` | string | — | **Reserved / unused.** Keep `"default"`. Card variant is chosen by board type, not this field. |
| `version` | int | — | **Reserved / unused.** Keep `1`. |
| `title` | string | dataPath | Card heading. Non-string → `---`; HTML stripped. |
| `subTitle` | string[] | dataPath each | One line per entry under the title. Non-string entries are dropped. |
| `secondaryLabel` | string | **literal** | Static label for the secondary value (bottom of card). |
| `secondaryValue` | string | dataPath | Secondary value. `''`/`'0'` → `---` (a legitimately-zero value renders the same as empty — by design); an ISO-date-shaped string is reformatted `dd MMM yyyy`. |
| `statusText` | string | dataPath | **Status pill label — rendered RAW; a non-scalar crashes the board (React #31). See scalar-leaf rule.** Empty → localized "no status". |
| `statusClassName` | string | dataPath or literal token | **Status pill color — `.includes('#')` test; a non-scalar crashes the board (`TypeError`).** See below. |
| `statusChain` | string | — | **Not consumed by the renderer** (legacy; often defaulted to `$.chains.pop`). Pill color/label come from `statusClassName`/`statusText`. |
| `items` | array | — | Extra label/value rows. **Only the first 2 are shown**, plus the synthesized `secondaryLabel`/`secondaryValue` row. See below. |
| `itemMode` | string\|null | — | **Accepted but ignored.** |
| `icon` | object\|null | — | Entity/asset cards only (see below). |

## The status pill — `statusText` + `statusClassName`

- **`statusText`** → the pill's **text**. A dataPath; rendered raw (the slot that
  crashed the board in the incident that prompted this doc — see pitfall #62 in
  `SKILL.md`). Point it at a scalar leaf.
- **`statusClassName`** → the pill's **color**. If the resolved value contains
  `#` it is used as a raw hex color; otherwise it is looked up in a fixed token
  map. Accepted tokens: `primary`, `secondary`, `warning`, `danger`, `error`,
  `muted`, `success`. Unknown or empty → `secondary` (grey). Text color is
  auto-contrasted. It may be a literal token (`"success"`) or a dataPath that
  resolves to one — but that dataPath **must** resolve to a string. The color
  test is `value.includes('#')`, so a value that resolves to an object/array
  throws a `TypeError` and crashes the board exactly like `statusText` (see the
  scalar-leaf rule).

## `items[]`

Each item is `{label, value, color, icon, dateFormat, mode}`. Only the first two
items render on a work card.

| Sub-field | Kind | Notes |
|---|---|---|
| `label` | **literal** | Static row label. If empty, a heuristic guesses from `value` (e.g. contains `name` → `Product:`). |
| `value` | dataPath | Empty → `---`; numbers stringified; an ISO-date (`YYYY-MM-DD`) is reformatted via `dateFormat`. |
| `dateFormat` | literal (Luxon token) | Default `dd MMM yyyy`. Only affects `value` **when `value` parses as an ISO date**; ignored otherwise. |
| `color` | — | **Accepted but ignored** on the work card. |
| `icon` | — | **Accepted but ignored** on the work card. |
| `mode` | — | **Accepted but ignored.** |

## `icon` (entity cards only)

`{color, shape, imageUrl}` — each sub-field is a dataPath. Rendered **only by the
entity/asset card**, not the work-item workboard.

- `shape` → icon name/shape token for the avatar.
- `color` → CSS color or token for the avatar.
- `imageUrl` → resolved to an `<img>` src (e.g. `$.data.icon.imageUrl`), with
  fallbacks to the entity's own image / flow icon.

There is no enforced enum for `shape`/`color` — they are whatever the avatar
component accepts.

## Fields accepted but ignored

The schema accepts these, but no work-card renderer consumes them — do not rely
on them: `card`, `version`, `statusChain`, `itemMode`, `items[].color`,
`items[].icon`, `items[].mode`. `icon.*` is consumed **only** by the entity card.
Only the first two `items[]` render (plus the secondary row).

## Editing the card layout

The flow must be in **draft** mode.

```
create_flow_draft(published_flow_id)          # → draft id (original becomes read-only)
manage_card_layout(flow_id=<draft>, action="get")                 # inspect current
manage_card_layout(flow_id=<draft>, action="update", card_layout={
    "cardConfig": {"statusText": "$.newOpportunity.opportunityType.displayName"}
})                                             # PATCH — deep-merges; send only changed keys
publish_flow(<draft>)                          # commits the card layout
```

- `action="update"` **deep-merges** onto the current layout, so omitted keys
  (`title`, `cardConfig.*` you didn't touch) are preserved — send only what you
  change.
- `unset=True` removes the provided keys instead of setting them.
- Do not pass `cardLayout` to `update_flow_properties` — it is silently dropped
  there.

## Worked example

Bug (crashes the board for anyone who can see a card with `opportunityType` set):

```yaml
cardConfig:
  statusText: $.newOpportunity.opportunityType        # ✗ resolves to {id, displayName, imageUrl, data, …}
```

Fix:

```yaml
cardConfig:
  statusText: $.newOpportunity.opportunityType.displayName   # ✓ scalar
```
