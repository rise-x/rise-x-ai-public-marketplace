# Aggregated Dashboards

Load this file when the dashboard is or becomes aggregated: `data_source` is set, a component carries `reportData`, or a chart must sum values inside a sub-array of each work. The dashboard model and build order are in `references/dashboards.md`; per-chart property gotchas are in `references/dashboards-charts.md`.

In Aggregated mode each tile draws from its **own** server-side dataset, declared on the component as `reportData`. The engine takes the works visible to the dashboard (after `defaultQueryFilters`), runs the recipe, and returns rows. The tile never sees the raw works.

## Setting the dashboard `dataSource`

Pass `data_source={"name": "Aggregate", "properties": {}}` to `create_dashboard` or `update_dashboard_metadata`. Omit it at create for Works mode. On an existing dashboard, omitting `data_source` changes nothing; a `dataSource.name` that is unset or `"works"` means Works mode.

- **Avoid `{"name": "data"}` and other custom names.** They need a matching driver configured on the ecosystem. Without one, the aggregated data fetch returns a generic 400 with no useful message, which is easy to misdiagnose as a permissions, schema, or `reportData` problem.
- **`dataSource.name` is not a report name.** `name` picks the driver. `properties.reportName` is a label that some custom drivers read; with `Aggregate`, each component's own `reportData` drives the rows and `reportName` is unused.
- **If an aggregated dashboard returns 400:** read `get_dashboard(id)`, which shows `dataSource`. If it is anything other than `Aggregate`, patch it with `update_dashboard_metadata(id, data_source={"name": "Aggregate", "properties": {}})`. Tiles and `reportData` stay as they are.

Flipping `dataSource` is half the change. The other half is giving each tile `reportData` and pointing its `*Key` properties at the aggregated rows (see [Render path per component](#render-path-per-component)).

To add a tile with `reportData`, include the block in the component dict for `create_dashboard` or `add_dashboard_components`. To change it later, `update_dashboard_component(draft_id, component_id, {"reportData": [...]})` replaces the whole array; `[]` clears it.

## Render path per component

| Component | In Aggregated mode |
|---|---|
| `total-by-bar-chart`, `total-by-stacked-bar-chart`, `total-by-pie-chart`, `total-by-period-stacked-bar-chart` | Separate aggregated renderer over the tile's rows. |
| `total-by-clustered-bar-chart` | Aggregated only; blank in Works mode. Groups by `groupSeriesByKey` (X-axis) and `groupStackByKey` (clusters). |
| `grouped-metric-card` | Separate renderer: `valueLabel` / `value` / `secondValue` per row. |
| `metric-card`, `rating-metric-card` | `value` resolves against the data the tile's `dataPath` selects. |
| `savings-metric-card` | Unpacks `value` into High / Mid / Low. |
| `multiple-rating-chart` | One path for both modes; reads the first row per group. |
| `data-grid` | Uses the server-aggregated rows. |
| `heatmap-chart` | **No aggregated path.** It reads rows as if they were works and is likely broken. Keep heatmaps on Works dashboards. |

**The value key differs by mode.** Both modes read `groupSeriesByKey`, but for the measure:

| Chart | Works reads | Aggregated reads |
|---|---|---|
| `total-by-bar-chart` | `calculateSeriesKey`, `countSeriesKey` | **`calculateStackKey`**, no fallback |
| `total-by-stacked-bar-chart` | `calculateSeriesKey` | `calculateStackKey` |
| `total-by-period-stacked-bar-chart` | `calculateSeriesKey` | **`calculateSeriesKey`**; `calculateStackKey` is ignored |
| `total-by-pie-chart` | `calculateSeriesKey` | `calculateSeriesKey`, falling back to `calculateStackKey` |

An aggregated bar chart with only `calculateSeriesKey` set multiplies every bar by 0 and shows `NO DATA`. An aggregated period chart with `calculateSeriesKey: ""` counts rows per bucket instead of summing, so bars show small integers or look empty. Set the period chart's `calculateSeriesKey` to the measure's `targetPath`, and keep `calculateStackKey` on the same field.

### Top-metric / average overlays in Aggregated mode

`total.value`, `secondTotal.value`, and `average.value` resolve against the **full aggregated payload** (`fullData.data`), not the tile's rows. So they can read a different field group, for example `"$.totals[0].totalQuantity"`.

- `total` needs `showTotal: true`; `average` needs `showAverage: true`; `secondTotal` renders whenever it is present.
- `title` and `units` are literal strings beside the number. Missing `units` falls back to `unitOfMeasure`, then `""`.
- Charts that show them: `total-by-bar-chart` (average only), `total-by-stacked-bar-chart` (total, secondTotal, average), `total-by-pie-chart` (total, secondTotal), `multiple-rating-chart` (average only). `total-by-period-stacked-bar-chart` shows neither.

### Pie chart: slice total plus an overall total

The donut centre shows the sum of the drawn slices. A header pill can show a broader total from another group in the same payload.

```jsonc
{
  "component": "total-by-pie-chart",
  "label": "Volume by region",
  "dataPath": "$.values[?(@.group == 'region')].metrics",
  "properties": {
    "groupSeriesByKey": "tags.region",
    "calculateSeriesKey": "totalVolume",
    "unitOfMeasure": "MT",
    "showTotal": true,
    "totalLabel": "Selected scope total",
    "total": { "title": "All-data total", "units": "MT",
               "value": "$.values[?(@.group == 'total')].metrics[0].totalVolume" }
  }
}
```

`*Key` accessors stay dotted, against each row. `dataPath` and `total.value` are paths and take `$.` with filters. Do not set `secondTotal` here: it hides the centre number.

## Configuring Aggregated Reports (`reportData`)

Stay in Works mode when the chart is "group works by one of their fields, count or sum another". `reportData` is for values that are not fields on the work, such as per-line quantities inside `work.data.cart[]`, or for large work counts.

### `reportData` Schema

`get_schema` does not serve this shape. `reportData` is an array of independent **field groups**; a tile can read each by `name`.

```jsonc
"reportData": [
  {
    "id": "<guid, optional>",
    "name": "cartRowsByModel",                      // unique within the component
    "dataType": 2,                                  // value-typing hint, see enum
    "expandByDataPath": "$.orderDetails.cart",      // optional: one row per array element
    "groupBy": [{
      "sourcePath": "$.orderDetails.cart.model.displayName",  // FULL path from the work root
      "targetPath": "modelName",                              // the key the chart reads
      "missingValue": "Unknown"                               // bucket for null values
    }],
    "fields": [{
      "dataPaths": ["$.orderDetails.cart.quantity"],  // FULL path, or a = formula
      "operations": [0],                              // see enum
      "targetPath": "quantity",
      "condition": null,                              // per-row filter
      "format": null,
      "useExpand": true,                              // over expanded rows (true) or once per work (false)
      "countNulls": false                             // Count includes nulls when true
    }],
    "calculations": [],                             // derived columns, computed after fields
    "properties": ["$.orderDetails.cart"],          // MongoDB projection, see below
    "condition": ""                                 // NOT applied, see "Filtering rows"
  }
]
```

#### `operations` enum (numeric on the wire)

`0` Sum, `1` Average, `2` Count (`countNulls: true` counts nulls), `3` Max, `4` Min, `5` StdDev, `6` Median, `7` Mode, `8` Calculation (only in `calculations[]`). Send integers: `"operations": [0]`, not `["sum"]`. Sum adds the collected values across rows and works, not across the `dataPaths` entries (see [Multiple `dataPaths`](#multiple-datapaths--first-found-coalesce-schema-fallback)).

#### `dataType` enum (numeric)

`0` String, `1` Decimal, `2` Integer, `3` Date, `4` Boolean. Use `2` for counts and integer sums, `1` for currency and floats, `3` for date bucketing. Send the integer, not `"Integer"`.

#### `expandByDataPath` — flattening sub-arrays

The engine iterates the array and, for each element, **replaces the array in place** with that one element. The root shape does not change. So paths under an expand stay **full from the work root**: write `"$.orderDetails.cart.quantity"`, not `"$.quantity"`. A relative path resolves to null on every row and the whole group drops out of the response (see [pitfall 7](#aggregated-mode-pitfalls)). The same rule applies to tokens in formula `dataPaths` and `condition`.

`useExpand: true` (or `null`) processes every expanded row; `false` fires once per work (on the first row), for a per-work Count or a top-level scalar Sum. Without `expandByDataPath`, each work contributes one row.

#### What `properties` actually does

`properties` lists the paths added to the MongoDB **projection** when the engine fetches works. A path not listed is not in the document, so `groupBy.sourcePath`, `fields[].dataPaths`, and `condition` tokens on it resolve to null. The engine does not derive the list from the rest of the report.

The projection is the union of every report's `properties` on the dashboard, so another report can mask a missing path until it changes. **Declare every path your report uses in its own `properties`.**

| Report shape | Minimum `properties` |
|---|---|
| Count of works (`dataPaths: ["$.work"]`, Count) | `["$.work"]` |
| Sum or average of a form field | `["$.paymentDetails.amountPaid"]` |
| Group by a form field, count works | `["$.work", "$.paymentDetails.paymentMethod"]` |
| Conditional aggregate on `'{$.result}'` | add `"$.result"` |
| Expand over an array | `["$.orderDetails.cart"]`; the parent brings all nested fields |

**Symptom of a missing path:** the chart shows one bucket, every row collapsed into `missingValue` (usually `Unknown`). Check `properties` before suspecting the data.

#### The `$.work.X` namespace — what's accessible at the work level

`dataPaths`, `groupBy.sourcePath`, and `condition` can read a synthetic work object through `$.work.X`. It holds a fixed set of fields, not the full work record:

| Path | Returns |
|---|---|
| `$.work.id` | Work id |
| `$.work.chains` | The chain map with its events |
| `$.work.workState`, `$.work.flowState`, `$.work.status` | Current state values |
| `$.work.workStateName` | State as a string |
| `$.work.statusDisplay` | `displayName`, `color`, `bgColor`, `workType`, `icon`, `stateName`, `dateTimeIso`, `roleName`, `eventIndex`, `value` |
| `$.work.currentState` / `$.work.previousState` | Display name of the latest / previous `currentState` chain event |
| `$.work.createdDate` | `{date, offset, ticks, timezone}`; `ticks` is Unix-epoch **milliseconds** |

Not available: `$.work.created.date` (the nested v3 form), `$.work.displayName`, `$.work.lastModified`, `$.work.flowOriginId`, `$.work.workCode`. They resolve to null and every row lands in `missingValue`; reports ported from v3 often use `$.work.created.date`, so switch them to `$.work.createdDate`.

For dates, use `$.work.createdDate` for cycle time, for example `({$.delivery.actualDeliveryDate.ticks} - {$.work.createdDate.ticks}) / 86400000.0`. Use `$.work.statusDisplay.dateTimeIso` for "when the work entered its current status", which is what most "by month" charts want.

A `condition` interpolates `{$.path}` tokens: `$.work.X` against the synthetic object, other paths against the (expanded) row. Quote each token and each literal in single quotes; comparison is case-sensitive string equality, evaluated per row before the operation.

#### What the aggregator can actually see — projected data, not full works

The engine reads only projected fields from stored works. Two consequences:

1. **Paths into referenced assets, contracts, and POs do not resolve.** A reference field such as `invoice.contract` is stored as a shallow `{ id, displayName, imageUrl }` plus a thin `data` envelope. The fully hydrated payload that `get_work` shows is built on demand and is not in storage. So group by `$.invoice.contract.displayName`, not `$.invoice.contract.data.contractAsset.contractor.displayName`. If a value is not written to `work.data.*` by a flow step, assume the engine cannot see it; denormalise it onto the work or chart something else.
2. **Prefer a stored scalar over expand-and-sum.** When the form stores a total beside the array (`invoice.services_Total` beside `invoice.services[]`), sum the scalar with no expand and `useExpand: false`. It is more reliable than expanding and summing a nested field.

#### `fields[].dataPaths` accepts `dynamicValue` formulas (per-row math)

A `dataPaths` entry can be a `=` formula, evaluated per (expanded) row **before** the operation. Use it for derived measures such as line revenue:

```jsonc
{"dataPaths": ["={$.orderDetails.cart.quantity} * {$.orderDetails.cart.unitPrice}"],
 "operations": [0], "targetPath": "revenue", "useExpand": true}
```

This is also the fix when a data-grid shows a computed column (`column.value`): those values are render-only and never stored, so recompute them here (see `references/layouts-and-components.md` "Computed Columns"). Use `calculations[]` instead for formulas over already-aggregated values.

#### Multiple `dataPaths` — first-found coalesce (schema fallback)

The entries in `dataPaths` are **ordered alternatives, not terms to add**. Per row, the engine uses the first path that resolves, then stops. To add two fields, use one formula entry (`"={$.a} + {$.b}"`).

Use this when different works store the same measure under different paths, for example after a schema change, or array on some works and scalar on others:

```jsonc
{"dataPaths": ["$.deliveryDocuments.meterReadings.deliveredVolumeLiters",
               "$.deliveryDocuments.deliveredVolumeL"],
 "operations": [0], "targetPath": "total", "useExpand": true}
```

- Declare every alternative in `properties`.
- Works that have the expand array give N rows; works without it give one un-expanded row where the scalar resolves. Both feed the same Sum.
- Each missed path logs `"Data path '<path>' not found in data."` in the response's error map before falling through. The value is still right.
- To avoid that noise, declare two fields with the **same** `targetPath`, each gated by a `condition` on its schema. A false condition skips silently, and fields with the same `targetPath` merge before the operation runs.

#### `groupBy.targetPath` must match the chart's `*Key`

Each output row carries the `targetPath` names you set. The chart reads them through its `*Key` properties: `groupBy[].targetPath: "model"` pairs with `groupSeriesByKey: "model"`, and `fields[].targetPath: "quantity"` pairs with `calculateStackKey: "quantity"`. Pointing a `*Key` at a work field instead (for example `"data.courier"`) renders every bar as `Unknown` or `0`.

### Worked Example — `total-by-bar-chart` summing per-cart-line quantities by model

Each work has `orderDetails.cart[]` entries `{ model: <asset link>, quantity: <int> }`. The goal: units sold per model, on a dashboard with `dataSource.name: "Aggregate"`.

```jsonc
{
  "component": "total-by-bar-chart",
  "label": "Top T-shirt Models",
  "name": "topTshirtModels",
  "dataPath": "$.data.topTshirtModels",
  "properties": {
    "horizontal": true,
    "unitOfMeasure": "units",
    "groupSeriesByKey": "model",
    "calculateStackKey": "quantity"
  },
  "reportData": [{
    "name": "topTshirtModels",
    "dataType": 2,
    "expandByDataPath": "$.orderDetails.cart",
    "groupBy": [{"sourcePath": "$.orderDetails.cart.model.displayName",
                 "targetPath": "model", "missingValue": "Unknown"}],
    "fields": [{"dataPaths": ["$.orderDetails.cart.quantity"], "operations": [0],
                "targetPath": "quantity", "useExpand": true}],
    "calculations": [],
    "properties": ["$.orderDetails.cart"]
  }]
}
```

The expand turns each work into one row per cart line. `groupBy` buckets the rows by model name, read by full path, into `model`. `fields[0]` sums each line's quantity into `quantity`. The chart reads `model` and `quantity`, and uses `calculateStackKey` because the aggregated bar chart reads that key.

### Counting works instead of summing a measure

Use Count (`2`) on any always-present path, with `useExpand: false` to count works rather than expanded rows:

```jsonc
"fields": [{"dataPaths": ["id"], "operations": [2], "targetPath": "data.count", "useExpand": false}]
```

The chart then reads `"calculateStackKey": "data.count"`. Set `useExpand: true` to count expanded sub-rows.

### Filtering rows with `condition`

**Put row filters on `fields[].condition`.** The group-level `condition` beside `name` and `groupBy` is **not applied**: every row is counted regardless. The usual symptom is a chart showing all works, or two charts with different group conditions showing identical numbers. To filter a whole group, repeat the condition on every `fields[]` entry. To scope which works reach the report at all, use the dashboard's `defaultQueryFilters`.

Condition tokens use full paths from the work root, and every path must be in `properties` (or be a `$.work.X` path), or it projects to null and the comparison fails on every row.

```jsonc
{"dataPaths": ["$.work"], "operations": [2], "useExpand": false, "targetPath": "total",
 "condition": "'{$.work.statusDisplay.displayName}' == 'Order Rejected'"}
```

### `calculations[]` — derived columns

For a measure built from already-aggregated values. An entry has the shape of a `fields[]` entry plus `calculation`, a formula over earlier `targetPath`s in the same group. Its `operations` must be `[8]`; with `[0]` the engine tries to sum the formula instead of evaluating it.

```jsonc
"calculations": [{"calculation": "={$.data.quantity} / {$.data.totalQuantity}",
                  "operations": [8], "targetPath": "data.share"}]
```

### Multiple Datasets per Component

A component can declare several field groups and read each by name. For example, the bars read a `byModel` group while `total.value: "$.totals[0].volumeTotal"` reads a `totals` group. This is common on `grouped-metric-card`, where `value` and `total.value` come from different recipes.

### Aggregated-Mode Pitfalls

1. **`reportData` without the dashboard `dataSource`.** In Works mode `reportData` is ignored and the tile gets raw works. Set `data_source` first.
2. **`useExpand` must match `expandByDataPath`.** With an expand set, `useExpand: false` on a Sum or Average reads only the first element per work and often renders `0`. v3-imported reports often carry `useExpand: false` on Sum fields; flip them. With no expand, `useExpand: true` operates over an empty expansion and returns `0`.
3. **No `missingValue` drops rows.** Rows whose `sourcePath` is null leave the chart entirely instead of landing in a bucket, which understates totals. Set `missingValue: "Unknown"`.
4. **Enums are integers on the wire.** `operations: ["sum"]` and `dataType: "Integer"` are rejected; use `[0]` and `2`.
5. **`*Path` and `*Key` use different syntax.** `groupBy[].sourcePath` is JSONPath style (leading `$.` optional); `groupSeriesByKey` is a dotted lodash path. Do not copy one into the other.
6. **`{$.works:JSON}` is unreliable in Aggregated mode.** Tile expressions run against the aggregated payload, which may not carry the works array, so `"=count({$.works:JSON})"` usually renders `0`. Build every count, sum, or deep-array measure as a `reportData` field group and read it with, for example, `dataPath: "$.data.<reportName>[0]"` and `value: "$.<targetPath>"`.
7. **Expand groups vanish when nothing is added.** If a group with `expandByDataPath` collects no values on any work, its key is absent from the response, not present with `0`. A metric card reading `"$.data.<name>[0]"` then shows `0` and charts show `No data`, with no error. The usual cause is a relative path (`"$.quantity"`) instead of the full form (`"$.orderDetails.cart.quantity"`). To confirm, point the expand at a known-good path and check the key appears. Other causes: the array is empty on every work, or the works use another schema (pitfall 8). Aggregating a stored scalar avoids the problem.
8. **Historical works keep the old schema.** When a field moves (for example `cart` to `orderDetails.cart`), older works keep the old path and nothing migrates them. Accept partial coverage, list both paths in one field's `dataPaths` (see [Multiple `dataPaths`](#multiple-datapaths--first-found-coalesce-schema-fallback)), or backfill the old works. When existing works do not show up, fetch one and compare its top-level keys to the current form.
9. **A KPI shows a total while the period chart over the same measure is empty.** A period chart also needs a non-empty bucket date on each contributing work. If the works that carry the measure are not the works that carry the date, the period chart drops them. Fetch one contributing work and confirm both paths are set on it. Fix by putting the date on those works, or bucket by `$.work.statusDisplay.dateTimeIso`. Keep the KPI and the period chart on the same `dataPaths` and `expandByDataPath` so their totals agree.
10. **`dataPath: ""` is wrong on aggregated tiles.** The tools set a missing `dataPath` to `""` on every non-section component, so leaving it out is the same mistake. An empty `dataPath` resolves to `""`, so the tile's tokens find nothing and metric cards render `0`. Set `"$"` for the whole payload, or `"$.data"`: the aggregate endpoint wraps the result as `{id, data: {values: [...]}, errors}` and tiles receive the whole envelope.
11. **The response wrapper breaks `..` paths.** A lookup that returns `undefined` retries against `data.data`, but a path containing `..` returns `[]` on no match, so it never retries; `sum([])` is `0`. Set the tile's `dataPath` to `"$.data"`, or start the tokens with `data.` (`{$.data.values[...]..value:JSON}`).
12. **The JSONPath library has no `&&`, `||`, or chained filters.** `[?(@.x == 'a' && @.y == 'b')]` returns empty, and in `[?A][?B]` the second filter is ignored. Single conditions with `==` and `!=` work, and so does `..`. To combine two conditions, put `..` between filters where the shape allows, filter in a grouped tile's per-row expression, or, where an "All" row equals the sum of the per-key rows, isolate it arithmetically. Together with `sum([])` being `0`, this is a common reason an aggregated metric card shows `0` beside a working chart.
13. **Aggregated `grouped-metric-card` needs an array.** It maps over the resolved data. A `dataPath` ending in a filter with no `..` before it returns one object, and the tile throws `.map is not a function`. Include `..` in the path (for example `$.data.values[?(@.group=='offer')]..metrics[?(@.labels.measure=='Sufficient offers')]`).

When an aggregated metric card renders `0`, pitfalls 10 to 12 all look the same. Isolate the cause by simplifying the formula in steps: `"=42"` (does the card render at all?), then `"=count({$.values:JSON})"` (does the path see data?), then the real formula. If the count works and the real formula renders `0`, the filter is the cause.
