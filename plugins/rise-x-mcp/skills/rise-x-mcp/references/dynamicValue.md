# Dynamic Value Expressions

Many string-valued properties across Rise-X (chart formulas, tooltip values, action conditions, validation rules, computed labels, metric-card `value`, `total: { value }`, etc.) are evaluated at runtime by a function called `dynamicValue`. It supports four input shapes — literal strings, JSONPath references, token-substituted strings, and `=`-prefixed math formulas. Formulas are evaluated by `mathjs` extended with custom Rise-X helpers.

Read this file before authoring any property whose value is itself an expression. Getting the input shape wrong is the single most common cause of blank tiles, `undefined` tooltips, and "No Data" charts.

## Contents

- [Quick Reference](#quick-reference)
- [The Four Input Shapes](#the-four-input-shapes)
- [Referencing the Works Array Inside Formulas](#referencing-the-works-array-inside-formulas)
- [The `:JSON` Token Formatter](#the-json-token-formatter)
- [mathjs Functions Available in Formulas](#mathjs-functions-available-in-formulas)
- [Rise-X Custom Helpers](#rise-x-custom-helpers)
- [Canonical Formulas (Copy-Paste)](#canonical-formulas-copy-paste)
- [Failure Behavior](#failure-behavior)
- [When NOT to Use a Formula](#when-not-to-use-a-formula)
- [Common Pitfalls](#common-pitfalls)

## Quick Reference


| Input shape              | Example                             | Resolved as                                                                                                |
| ------------------------ | ----------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| Literal string           | `"Total Revenue"`                   | Returned verbatim.                                                                                         |
| JSONPath                 | `"$.summary.revenue"`               | Looked up against the data context (for dashboards: `{ works: [...] }`). Returns the value or `undefined`. |
| Token-substituted string | `"Code: {$.workCode} ({$.status})"` | Each `{$.path}` is replaced inline. Unresolved tokens become `""` on dashboards, `0` elsewhere.            |
| Formula                  | `"=sum({$.works..amount})"`         | Tokens substituted first, then mathjs evaluates the result.                                                |


Pick the simplest shape that does the job. Plain JSONPath is preferable to a one-token formula; a token string beats a formula when no math is needed.

## The Four Input Shapes

### 1. Literal string

A string with no `$`, no `=`, and no `{`. Returned as-is.

```jsonc
"title": "Order Volume by Region"
```

### 2. Bare JSONPath

A string starting with `$.`. Resolved against the data context, then against `data.data` (works expose their payload at `data`), then against any provided `variables`. Returns the resolved value or `undefined` if not found.

```jsonc
"value": "$.summary.totalOrders"
"value": "$.amount"          // for a single work, reads work.data.amount
```

Use for: showing a single field as-is, no math, no concatenation.

### 3. Token-substituted string

A string containing one or more `{$.path}` placeholders. Each placeholder is replaced inline with the value at that path. Useful for assembling labels, identifiers, and titles.

```jsonc
"label": "Work {$.workCode} — {$.status}"
"href": "/work/{$.id}"
```

If a token resolves to `undefined`/`null`, the substitution depends on context:

- **Dashboards** (via `dashboardDynamicValue`): unresolved tokens become `""` (empty string).
- **Other contexts**: unresolved tokens become `0` inside formulas, otherwise `""`.

The non-`$` form `{key}` is also recognised and looks up a top-level key in the data context (e.g. `{count}` → `data.count`). It's rarely needed in dashboards.

### 4. Formula

A string starting with `=`. The platform first substitutes any `{...}` tokens, then hands the resulting string to `mathjs` for evaluation.

```jsonc
"value": "=sum({$.works..amount})"
"value": "={$.basePrice} * (1 + {$.taxRate})"
```

Two automatic rewrites happen before evaluation, so authors can write idiomatic JS:

- `&&` → `and`, `||` → `or` (only outside string literals).
- String equality like `'X1' == '0'` collapses to `1` or `0` (case-insensitive). The same for `!=`.

## Referencing the Works Array Inside Formulas

Authors expect to write `=sum(works.*.amount)` or `=works.length` because `works` is "in scope". This does not work, and the failure is silent.

Why: although `works` exists in the data context, the mathjs scope is built by coercing each value with `Number(value)`. A non-empty array becomes `NaN` and is then normalised to `0`. So `works` becomes the literal `0` inside the formula, and `0.length` or `0.*.amount` quietly fail.

The reliable pattern is to pass the array into the formula via a token, where it's substituted as a literal. There are two token forms:

### `{$.works..fieldName}` — recursive descent

JSONPath's recursive descent (`..`) collects every `fieldName` value found **inside each work's `data` payload**. The token is substituted as a comma-separated list, which mathjs accepts as a variadic argument list.

```jsonc
"=sum({$.works..amount})"        // sum of every data.amount
"=mean({$.works..score})"        // average of every data.score
"=max({$.works..deliveryDays})"  // max of every data.deliveryDays
```

Use this when:

- You're aggregating a **form-field value** (lives under `work.data.<field>`).
- Every work has the field (or you want to ignore the ones that don't).

Important — what `..fieldName` does **not** reach:

- **Top-level work fields** like `id`, `displayName`, `createdDate`, `status` are not reachable via `..` from the `works` array as it's exposed to dashboard formulas. Recursive descent only resolves into the `data` payload, so `{$.works..id}` returns nothing useful and `=count({$.works..id})` renders **`0`**.
- For "count of works" use the `:JSON` form below: `=count({$.works:JSON})`. That's the canonical count pattern — see [Canonical Formulas](#canonical-formulas-copy-paste).

Avoid `..fieldName` for string fields with commas/quotes — the comma-separated substitution would be ambiguous. Use the `:JSON` form instead.

### `{$.works:JSON}` — JSON-encoded payload

The `:JSON` formatter encodes the resolved value as a JSON literal so a custom helper that takes an array can parse it. Required for any custom helper that accepts a `works` argument.

```jsonc
"=sumOfFilteredValues({$.works:JSON}, \"status\", \"Done\", \"amount\")"
"=groupedSumOfFilteredValues({$.works:JSON}, \"region\", \"EU\", \"product\", \"qty\")"
```

Without `:JSON` the substituted value is the array's `toString()` — unparseable for anything beyond simple variadic numbers.

## The `:JSON` Token Formatter

`{$.path:JSON}` is a generic formatter. It doesn't have to be `works`; any value that needs to round-trip through a formula as a structured argument should use it.

How it works under the hood (you don't need to call this yourself): the engine stores the resolved value in an internal store keyed by the path, and substitutes a call like `getDynamicValue("$.path")` into the formula. Custom helpers receive the live value, not a stringified one.

Use `:JSON` whenever you're passing arrays or objects to a custom helper.

## mathjs Functions Available in Formulas

The standard mathjs library is loaded with the `all` preset, so most builtins work. The ones authors actually use in dashboards:

- **Aggregations:** `sum`, `mean` (alias `average`), `min`, `max`, `count`, `prod`, `std`, `median`.
- **Arithmetic:** `+`, `-`, `*`, `/`, `%`, `pow`, `sqrt`, `abs`, `round`, `floor`, `ceil`, `log`.
- **Comparison:** `==`, `!=`, `<`, `>`, `<=`, `>=`, `equal`, `unequal`.
- **Boolean:** `and`, `or`, `not`. (Source `&&`, `||`, `!` are auto-rewritten.)
- **Conditional:** ternary `cond ? a : b`.

Don't reach for lodash idioms (`groupBy`, `mapValues`, `pluck`, `chain`, `_.get`, etc.) — none are imported. Use the Rise-X custom helpers below.

## Rise-X Custom Helpers

These are imported into mathjs scope and callable from any `=` formula. Group by purpose:

### Aggregation over works (single number)


| Helper                                                                   | Returns | Notes                                                                           |
| ------------------------------------------------------------------------ | ------- | ------------------------------------------------------------------------------- |
| `sumOfDeepValues(works, "field")`                                        | number  | Sum a field across works. Handles nested values via `getDeepValue`.             |
| `sumOfFilteredValues(works, "filterKey", "filterValue", "sumField")`     | number  | Filter then sum. Filter is strict equality on a top-level work field.           |
| `sumOfDeepFilteredValues(works, "filterKey", "filterValue", "sumField")` | number  | Filtered + deep.                                                                |
| `countOfFilteredValues(works, "filterKey", "filterValue")`               | number  | Count works whose `filterKey` equals `filterValue`.                             |
| `countOfDeepValues(works, "field", flatten?)`                            | number  | Count *distinct* values at a path across works.                                 |
| `countOfItemsInWorkData(data, "field", trimDataPathPrefix?)`             | number  | Length of an array field, or numeric coercion. Useful for single-work contexts. |
| `filterByDeepValue(works, "filterKey", filterValue)`                     | array   | Returns the filtered subset for nesting in another helper.                      |


### Grouped output (returns `{ label: number }` or `{ label: "NN%" }`)

These are the helpers that feed `grouped-metric-card` and grouped-metric chart series — each returns an object whose keys become row labels.


| Helper                                                                                                    | Returns                  | Notes                                                                  |
| --------------------------------------------------------------------------------------------------------- | ------------------------ | ---------------------------------------------------------------------- |
| `groupedCountOfValues("comma,separated,string")`                                                          | `{ label: count }`       | Splits a comma-separated string and counts occurrences.                |
| `groupedCountOfFilteredValues(works, "filterKey", "filterValue", "groupKey")`                             | `{ label: count }`       | Filters works, then splits each `groupKey` value on commas and counts. |
| `groupedSumOfFilteredValues(works, "filterKey", "filterValue", "groupKey", "sumKey")`                     | `{ label: sum }`         | Filters, groups by `groupKey`, sums `sumKey` per group.                |
| `groupedAverageOfFilteredValues(works, "filterKey", "filterValue", "groupKey", "avgKey")`                 | `{ label: avg }`         | Filters, groups, averages.                                             |
| `weightedAverageOfFilteredValues(works, "filterKey", "filterValue", "groupKey", "avgKey", "multiplyKey")` | `{ label: weightedAvg }` | `sum(avg * weight) / sum(avg)` per group.                              |
| `getPercentageFromFixedRatio(works, ratioKey, ratioValue, totalKey, totalValue, groupKey, sumKey)`        | `{ label: "NN%" }`       | Pre-formatted percentages.                                             |
| `getPercentageFromFixedRatioCount(works, ratioKey, ratioValue, totalKey, totalValue, groupKey)`           | `{ label: "NN%" }`       | Same, count-based.                                                     |


### Single-percentage helpers


| Helper                                                                              | Returns        | Notes                                                 |
| ----------------------------------------------------------------------------------- | -------------- | ----------------------------------------------------- |
| `getTotalPercentageFromFilteredValues(works, "filterKey", "filterValue", "sumKey")` | `"NN%"` string | Filtered sum / total sum × 100, formatted as `"NN%"`. |


### Conditional / null-handling


| Helper                                  | Returns | Notes                                                                                                                           |
| --------------------------------------- | ------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `ifNull(a, b)`                          | number  | Returns `a` if `a > 0`, else `b ?? 0`. **Value-based, not strict null.** Use the ternary `cond ? a : b` for true null handling. |
| `isEmpty(value)`                        | boolean | True on null / undefined / empty array / empty object / empty string / unresolved token.                                        |
| `ifNotDefined(value)`                   | boolean | Heuristic: true when the value still looks like an unresolved `{...}` token.                                                    |
| `hasDuplicates(array)`                  | boolean | True if the array contains the same value twice.                                                                                |
| `countUniqueValues(string | array)`     | number  | Distinct items in a comma-separated string or array.                                                                            |
| `countOfValues(string, valueToFilter?)` | number  | Counts comma-separated occurrences (optionally only matching `valueToFilter`).                                                  |


### Dates


| Helper                                                    | Returns           | Notes                                                                                         |
| --------------------------------------------------------- | ----------------- | --------------------------------------------------------------------------------------------- |
| `getDateDiff(d1, d2, "days" \| "hours" \| "minutes" \| ...)` | number             | Luxon difference between two ISO dates in the requested unit. Returns 0 if either is invalid. |
| `shiftDateByDuration(date, "5 days")`                        | ISO string \| null | Add a duration string (`"5 days"`, `"-2 hours"`, etc.) to an ISO date.                        |


### Pricing helpers (used in pricing flows; usually not needed for dashboards)


| Helper         | Returns | Notes                                                                    |
| -------------- | ------- | ------------------------------------------------------------------------ |
| `dTotal(a, b)` | number  | If `b > 0`: `a × ((100 − b) / 100)` (percentage discount). Else `a + b`. |
| `dValue(a, b)` | number  | If `b > 0`: `−a × (b / 100)`. Else `b`.                                  |


## Canonical Formulas (Copy-Paste)

The patterns to reach for first. Tweak the field names; the structure stays the same.

```jsonc
// Single number across works
"=count({$.works:JSON})"         // count of works — canonical pattern
"=sum({$.works..amount})"        // sum of data.amount across works
"=mean({$.works..score})"        // mean of data.score across works
"=max({$.works..deliveryDays})"  // max of data.deliveryDays across works

// Filtered single number
"=sumOfFilteredValues({$.works:JSON}, \"status\", \"Completed\", \"amount\")"
"=countOfFilteredValues({$.works:JSON}, \"status\", \"Completed\")"

// Grouped output for grouped-metric-card
"=groupedCountOfFilteredValues({$.works:JSON}, \"status\", \"Completed\", \"region\")"
"=groupedSumOfFilteredValues({$.works:JSON}, \"status\", \"Completed\", \"region\", \"amount\")"

// Percentage
"=getTotalPercentageFromFilteredValues({$.works:JSON}, \"status\", \"Completed\", \"amount\")"

// Conditional label (use single quotes around tokens for string comparison)
"='{$.code}' == '0' ? '-' : '{$.code}'"

// Date age in days against another field
"=getDateDiff('{$.createdDate}', '{$.dueDate}', 'days')"

// Arithmetic across fields on a single record
"={$.basePrice} * (1 + {$.taxRate})"
```

For an empty filter (i.e. "all works"), pass empty strings: `groupedCountOfFilteredValues({$.works:JSON}, "", "", "region")` returns `{ region: count }` for every distinct region.

## Failure Behavior


| Symptom in UI                             | Likely cause                                                                                                                                            |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Raw `=...` text rendered literally        | Formula failed to parse; the engine returns the unmodified input. Check for unbalanced quotes, missing tokens, lodash-style chaining.                   |
| Tile shows `0` when you expected a number | A token resolved to `undefined`/`null` and was substituted as `0` (the formula default for unresolved tokens). Verify the JSONPath against actual data. |
| Tile blank, no error                      | JSONPath miss returned `undefined`; the chart treats that as no data.                                                                                   |
| `undefined` in tooltip                    | A `tooltipFields[].value` references a key the bucketed item doesn't expose.                                                                            |


In development builds the engine logs failures to the console. On production builds failures are silent — the only signal is the rendered output.

## When NOT to Use a Formula

If the value is a single field on a work, prefer the bare JSONPath:

```jsonc
"value": "$.amount"                 // good
"value": "=sum({$.amount})"         // unnecessary; formula overhead with no aggregation
```

If you only need string interpolation, prefer a token string (no `=`):

```jsonc
"label": "Work {$.workCode}"        // good
"label": "='Work ' + '{$.workCode}'" // unnecessary; adds a parsing step
```

Reach for `=` only when you need arithmetic, aggregation, conditionals, or a custom helper.

## Common Pitfalls

1. **Treating `works` as a free symbol.** `=works.length`, `=sum(works.*.amount)`, `=avg(works.*.score)` — none of these work. The mathjs scope coerces arrays to `0`. Use `=count({$.works:JSON})`, `=sum({$.works..amount})`, `=mean({$.works..score})`.
2. **Reaching for lodash.** `=groupBy(works, 'status').mapValues(...)` is not supported — lodash isn't imported into mathjs. Use `=groupedCountOfFilteredValues({$.works:JSON}, "", "", "status")` (empty filter = all works) for the same effect.
3. **Forgetting `:JSON` when a helper takes an array.** `=sumOfFilteredValues({$.works}, ...)` substitutes the array's `toString()`, which mathjs cannot parse. Always use `{$.works:JSON}` for custom helpers that accept a works argument.
4. **Missing the `=` prefix.** `"sum({$.works..amount})"` is treated as a literal string; the `=` is what switches the engine to formula mode.
5. **Quoting tokens for string comparison.** Wrap them in single quotes so the substituted literal is a valid mathjs string: `"='{$.status}' == 'Done' ? 'OK' : 'X'"`. Unquoted, the substituted value would be parsed as an identifier.
6. **Expecting `null`-aware behavior from `ifNull`.** It tests `a > 0`, not `a != null`. For true null fallback, write `"='{$.maybe}' == '' ? '{$.fallback}' : '{$.maybe}'"`.
7. **Recursive descent on missing fields.** `{$.works..fieldName}` only contributes from works that *have* `fieldName` inside their `data` payload. If the field is sometimes missing, `count` will undercount. For "count of works", do **not** reach for `..id` — top-level work fields (`id`, `displayName`, `createdDate`, etc.) aren't reachable via recursive descent from the dashboard's `works` array. Use `=count({$.works:JSON})` instead — `count` over the JSON-substituted array gives the true work count.

