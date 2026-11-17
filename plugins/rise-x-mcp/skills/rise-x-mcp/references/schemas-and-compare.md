# Schemas & Compare

## Schema lookups

`get_schema(category)` lists the schema names in a category; `get_schema(category, name)`
fetches one. Read a schema before you configure something whose properties you are unsure of:
it defines the enums, required fields, and defaults.

```
get_schema("components", "search-things")   # picker and auto-population options
get_schema("components", "data-grid")       # column configuration
get_schema("activities", "SendEmail")       # an activity's required fields
get_schema("flows", "properties")           # flow-level properties
```

Two traps:

- **No rules under `flows`.** `rules`, `rules/rule`, and `chains/rule` do not exist there and
  return 404. Pipeline and rule shapes are in the `pipelines` category; a chain's shape is
  `get_schema("flows", "chains")`.
- **Component schema names are the names to create with.** `get_schema` answers 400 for
  `select`, `checkbox`, `comments`, and `related-work-select`. The served names are
  `input-select`, `check-box`, `comments-box`, and `relatedWork-select` (camelCase W).
  `switch` and `toggle` are served too, but prefer `product-toggle-switch`. Call
  `get_schema("components")` for the full list. See
  `references/layouts-and-components.md` for the full mapping. There is no `input-number`: a
  numeric field is `input-text` with `inputType: "number"`.

## Cross-ecosystem comparison

`compare(entity_type, source_ecosystem, source_id, target_ecosystem, target_id?)` diffs a flow
or layout between two ecosystems of the **same deployment**; the ecosystem arguments are
workspace names, not `dev` / `test` / `prod`. Omit `target_id` to compare against an empty
entity, which lists everything the source holds.

It returns:

```json
{
  "summary": "Overview of changes",
  "differences": [
    {"path": "steps[0].name", "type": "added|removed|changed",
     "sourceValue": "...", "targetValue": "..."}
  ],
  "patchPlan": [
    {"order": 1, "operation": "AddStep", "description": "Add step 'Review'"}
  ]
}
```

`patchPlan` describes the changes in words; it is not a payload you can send to another tool.
