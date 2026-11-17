# Dashboard Chart Configuration

Load this file when you configure a chart or metric card beyond `label` and `width`, or when a tile renders blank, `NO DATA`, `0`, `Unknown`, or the word `undefined`. The dashboard model and build order are in `references/dashboards.md`. Aggregated-mode behaviour (`reportData`, which value key each chart reads there) is in `references/dashboards-aggregated.md`. Expression syntax is in `references/dynamicValue.md`.

Each chart and metric card has a served schema: `get_schema("components", "<type>")` lists its properties. The sections that follow keep only what the schema does not say. `heatmap-chart` has no served schema, so its properties are listed in full.

## Configuring Charts & Metric Cards (Non-Aggregated Dashboards)

### Shared Concepts

- **Where data comes from.** A tile reads its data through the top-level `dataPath` field on the component; `properties.dataPath` is ignored. The renderer picks a resolver by the shape of the value:
  - **Empty, `null`, or `"$.works"`**: the full works array. This is the Works-mode default. The create, add, and replace tools set a missing `dataPath` to `""`.
  - **`$.`-prefixed with no `=` and no `{`** (for example `"$.summary"`, `"$.foo[0]"`): a lodash `_.get` lookup. It understands dotted paths and integer indexes, **not** `[*]` or `..`. So `"$.works[*].data.cart[*]"` and `"$.works..cart"` resolve to nothing and the tile renders `NO DATA`.
  - **`$.`-prefixed and containing `=` or `{`**: full JSONPath (filters, wildcards, recursive descent). A filter such as `"$.values[?(@.group == 'supplier')].metrics"` lands here because of the `==`.
  - **`@`-prefixed** (`"@row.foo"`, `"@options.value"`): row or chart context.
- **The API accepts a `dataPath` that is empty or starts with `$` or `@`.** A `=`-prefixed formula is rejected on write, even though the SDK could evaluate one. Put formulas in `properties.value` and similar properties.
- **Sub-array charts in Works mode.** Use the filter form, or switch the dashboard to Aggregated mode and use `reportData.expandByDataPath` (see `references/dashboards-aggregated.md`). Aggregated mode is the cleaner choice for anything beyond one filter.
- **Keys vs. paths.** Properties named `*Key` (`groupSeriesByKey`, `calculateSeriesKey`) are field accessors on each item, resolved like lodash `_.property`. Properties named `*Path` and metric-card `value` take `$.` JSONPaths or `=` expressions. See [pitfall 1](#chart--metric-card-pitfalls).
- **Counting vs. summing.** On charts that support both, leave the calculate key empty to count items per bucket; set it to sum a numeric field.
- **Tooltip fields.** `tooltipFields: [{ name, value, unit? }]`. `value` is a `dynamicValue` expression with two scopes:
  - the works array: `"{$.works..fieldName}"` or `"=sum({$.works..amount})"`;
  - `@options`, the current bar, slice, or row: `{@options.category}` (bucket label), `{@options.value}`, `{@options.percentage}`, `{@options.name}` (series), `{@options.total}`. Token form only, no `$.`.

  A bare string such as `"courier"` is a literal: the tooltip prints the word `courier`. The tooltip is not scoped to one work record.

### `total-by-bar-chart`

Counts or sums works grouped by one field. Beyond the schema:

- `groupSeriesByKey` (required): accessor on the work, usually `"data.<field>"`.
- `calculateSeriesKey`: field to sum per group; `""` counts. In Aggregated mode this chart reads `calculateStackKey` instead.

Example, count of works per courier, with tooltip context tokens:

```jsonc
{
  "component": "total-by-bar-chart",
  "label": "Order Count by Courier",
  "properties": {
    "title": "Order Count by Courier",
    "showTitle": true,
    "hasLabelsAtTheEndOfDataBars": true,
    "groupSeriesByKey": "data.courier",
    "calculateSeriesKey": "",
    "calculateStackKey": "",
    "countSeriesKey": "",
    "unitOfMeasure": " ",
    "tooltipFields": [
      { "name": "Courier", "value": "{@options.category}" },
      { "name": "Orders",  "value": "{@options.value}" }
    ]
  }
}
```

To sum order amount per courier instead, set `"calculateSeriesKey": "data.amount"` and a `unitOfMeasure`.

### `total-by-clustered-bar-chart`

> **Aggregated mode only.** On a Works-mode dashboard the renderer returns nothing and the tile is blank. On Works dashboards use `total-by-bar-chart`.

Side-by-side bars over two groupings. Beyond the schema:

- `groupSeriesByKey`: the X-axis category. `groupStackByKey`: the clusters inside each category; this is a real second grouping on this chart.
- The measure is `calculateStackKey`, falling back to `calculateSeriesKey`. In Aggregated mode a `*Key` may be a `=` formula, for example `"=round({$.savingsUsdPerMt}, 2)"`.
- `allowNegativeValues: true` keeps negative values. Without it they are clipped to 0 with no warning, so a savings or delta chart shows 0.
- `averageLinesFromSeries: true` (with `showAverage`, `hasAverageLine`, and an `average` block) draws one average line per series instead of one overall line.
- For an X-axis-only chart with a target line, set `groupStackByKey` equal to `groupSeriesByKey` or leave it empty. For stacked rather than side-by-side segments, use `total-by-stacked-bar-chart`.

### `total-by-stacked-bar-chart`

X-axis category plus stacked segments. Beyond the schema:

- `groupStackByKey` is required with a real value. Without it the chart throws and the tile shows a render error.
- `calculateStackKey` sums per (category, segment); `""` counts. `calculateSeriesKey` sets the category total and its labels.
- `total` / `secondTotal` header pills work as on the pie chart: `title` is the caption and a `label` key is ignored.

### `total-by-pie-chart`

Beyond the schema:

- `limit` keeps the top N slices (default 10).
- `countSeriesKey` counts items under one key when `calculateSeriesKey` is empty; initialise it to `""`.
- `showTotal: true` with `total: { title, value, units? }` adds a **header pill** above the pie, not text inside the donut. `secondTotal` renders whenever it is present, with no flag. `units` falls back to `unitOfMeasure`, then `""`. A `label` key inside these blocks is ignored; use `title`.
- `totalLabel` (a chart-level property, not in the schema) is the caption under the donut centre, which always shows the sum of the drawn slices. Setting `secondTotal` hides the centre and turns both totals into pills.
- For the aggregated "slice total plus overall total" pattern, see `references/dashboards-aggregated.md`.

### `total-by-period-stacked-bar-chart`

Time-series stacked bars. Beyond the schema:

- `groupSeriesByKey` must point at an ISO 8601 date string on each item; any other field gives an empty chart. `groupStackByKey` is required, as on the stacked chart.
- `calculateSeriesKey` sums per (period, segment); `""` counts. In Aggregated mode this chart reads `calculateSeriesKey`, not `calculateStackKey`.
- `period`: `"Day"` | `"Week"` | `"Month"` | `"Year"`; omit to pick from the date range. Not in the schema.
- `labelNumberFormatterOptions`: `Intl.NumberFormat` options, for example `{ "notation": "compact" }`. Not in the schema.

```jsonc
{
  "component": "total-by-period-stacked-bar-chart",
  "label": "Monthly Volume by Status",
  "properties": {
    "groupSeriesByKey": "data.createdDate",
    "groupStackByKey": "data.status",
    "calculateSeriesKey": "data.volume",
    "calculateStackKey": "data.volume",
    "period": "Month"
  }
}
```

### `multiple-rating-chart`

One star-rating row per group, taking the rating from the first item in each group. Beyond the schema:

- `groupStackByKey`: field whose distinct values become the rows.
- `calculateSeriesKey`: a `$.`-prefixed path to the rating value, for example `"$.performanceRating"`.
- `showAverage: true` with an `average` block adds an average pill above the list.

### `heatmap-chart`

Counts works in a fixed grid. Works mode only: it has no aggregated path. No served schema.

- Required: `xAxis.valuePath` and `yAxis.valuePath` (field names on the work), and `xAxis.items` / `yAxis.items` as `[{ value, displayName }]`. Cells are the cartesian product of the two lists; the chart does not infer cells from data, so empty `items` gives an empty grid.
- Optional: `xAxis.title`, `yAxis.title`; `categoryPath` (tooltip category); `countFlags: [{ valuePath, displayName }]` (tooltip breakdowns); `colors` (2 or more hex values; omit for the default gradient); `validationFilters: [{ valuePath, rules }]` (drops the items that fail a rule).

```jsonc
{
  "component": "heatmap-chart",
  "label": "Status by Region",
  "properties": {
    "xAxis": { "title": "Region", "valuePath": "region",
               "items": [{ "value": "EU", "displayName": "Europe" }, { "value": "NA", "displayName": "North America" }] },
    "yAxis": { "title": "Status", "valuePath": "status",
               "items": [{ "value": "Open", "displayName": "Open" }, { "value": "Closed", "displayName": "Closed" }] }
  }
}
```

### `metric-card`

Single KPI tile. Beyond the schema:

- `value` is a `dynamicValue` expression: a JSONPath, a token string, or a `=` formula. Count works with `"=count({$.works:JSON})"`; sum with `"=sum({$.works..amount})"`. Do not count with `..id`: `..` does not reach top-level work fields and the card renders `0`.
- `format`: a Luxon duration format such as `"hh:mm:ss"` when the value is seconds.

```jsonc
{"component": "metric-card", "label": "Total Orders",
 "properties": {"title": "Total Orders", "showTitle": true, "value": "=count({$.works:JSON})"}}
```

### `grouped-metric-card`

Multi-row tile whose properties mean different things per mode.

- **Works mode:** `value` must return a `{ label: number }` map, one row per key. Use the grouped helpers from `references/dynamicValue.md` with the `:JSON` formatter. `secondValue` is a second map shown beside matching rows. `total.value` evaluates against the works array. `valueLabel` is ignored.
- **Aggregated mode:** the tile receives the aggregated rows. `valueLabel`, `value`, and `secondValue` are per-row expressions, such as `"$.data.modelName"` and `"$.data.quantity"`. `total.value` evaluates against the full aggregated payload. Its `dataPath` must resolve to an array (see `references/dashboards-aggregated.md`).

```jsonc
{
  "component": "grouped-metric-card",
  "label": "Orders by Status",
  "properties": {
    "value": "=groupedCountOfFilteredValues({$.works:JSON}, \"\", \"\", \"status\")",
    "showTotal": true,
    "total": { "title": "Total", "value": "=count({$.works:JSON})" }
  }
}
```

### `rating-metric-card`

Star-rating tile. `value` evaluates to a number, intended range 0 to 5, for example `"=mean({$.works..satisfactionScore})"`.

### `savings-metric-card`

Runs a weighted savings formula. Beyond the schema:

- `calculateFn`: `"savingsByQuantity"` | `"savingsByEfficiency"` | `"savingsTotal"` (Works mode).
- `savingsByQuantity` and `savingsTotal` need `calculateSavingsByQuantityKey` and `calculateSavingsByProductPriceKey`. `savingsByEfficiency` and `savingsTotal` need `calculateSavingsByTimeKey`.
- `ranks: { high, mid, low }` weights the result; `ranksTotal` is needed only for `savingsTotal`. The served schema types the weights as integers.
- `calculateFn` drives Works mode. In Aggregated mode the card reads `value` instead, which must resolve to `[{ name: "high"|"mid"|"low", value: number[] }, ...]`.

### Chart & Metric-Card Pitfalls

1. **`*Key` properties are lodash paths, not JSONPaths.** `"courier"` reads `work.courier`; `"data.courier"` reads `work.data.courier`, the usual place for form values. `"$.courier"` fails in Works mode and puts every item in `Unknown`. Aggregated mode strips a leading `$.`, but stay dotted so the key works in both modes.
2. **Every bar labelled `Unknown` means the grouping path is wrong.** Items whose grouping field is missing, `null`, `"undefined"`, `"null"`, or `"unknown"` go into an `Unknown` bucket. The counts are real; fix the path, not the data.
3. **Aggregated field names do not exist in Works mode.** A key like `"orderCount"` exists only in an aggregated payload. In Works mode, group by a real work field and let the chart count.
4. **`countSeriesKey` is not "what to count".** To count, leave `calculateSeriesKey` empty. Do not put a label such as `"orderCount"` in a `*Key` property to name the axis.
5. **`works` is not a free symbol in expressions.** Reference it through tokens: `{$.works..amount}` (recursive descent into each work's data) or `{$.works:JSON}` (the whole array). Top-level work fields (`id`, `displayName`, `createdDate`) are not reachable with `..`; use `:JSON`.
6. **`dataPath` in Works mode.** Leave `dataPath` out or set `""`, and the tile gets the works array. A path that names no field, such as a label-like `"$.totalOrders"`, renders `NO DATA` on charts and `0` on metric cards. To clear one on an existing tile, `update_dashboard_component(draft_id, component_id, {"dataPath": ""})`. Aggregated tiles need a non-empty `dataPath` (see `references/dashboards-aggregated.md`).
7. **Seed `*Key` properties as empty strings.** The schemas initialise `calculateSeriesKey`, `calculateStackKey`, `countSeriesKey`, `groupSeriesByKey`, `groupStackByKey`, and `unitOfMeasure` to `""`. Match that shape to keep PATCH diffs clean. The exception: stacked and period-stacked charts need a real `groupStackByKey` or they throw. A tile that shows `NO DATA` with keys present usually has a `dataPath` or path problem, not a missing key.
8. **An unset `unitOfMeasure` renders the word `undefined`.** Bar, stacked, period, and clustered charts interpolate the unit with no fallback, so ticks read `2 undefined`. `null` renders `null`. For no unit, set `" "` or `""`. Metric cards are not affected.
