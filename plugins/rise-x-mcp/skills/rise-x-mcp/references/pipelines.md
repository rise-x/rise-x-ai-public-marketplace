# Data Pipelines (Flow Rules)

Data pipelines are **flow rules that run automatic data operations when watched
work data changes**. A pipeline watches one or more JSON paths; when they change
and an optional condition is met, it runs a list of operations against the work
item's data (set a value, copy a path, map an array, add days to a date, raise
an alert, pull data from a related entity, …).

Manage them with **`manage_pipeline`** — NOT `update_flow_properties` (the
properties endpoint binds to a typed POCO and silently discards `dataPipelines`).

```
manage_pipeline(flow_id, action, rule_id?, pipeline?, unset?, response_format?)
```

`action` is one of `list` | `add` | `update` | `delete`. The flow must be in
**draft** mode (`create_flow_draft` first if it's published).

## Pipeline shape

```jsonc
{
  "name": "Escalate critical requests",
  "comments": "optional note",
  "watchPaths": ["$.priority"],          // paths that trigger a re-run on change
  "fetchPaths": [],                       // optional: extra paths to load before running
  "if":   { "stepType": "WORK_DATA_PATH_EQUALS",
            "payload": { "dataPath": "$.priority", "equals": "Critical" } },
  "then": [ { "stepType": "SET_JSON_PATH",
              "payload": { "dataPath": "$.slaFlag", "value": "EXPEDITE" } } ],
  "else": []                             // optional: operations to run when `if` is false
}
```

- **`if`** is a **single** condition step `{stepType, payload}` (use `AND`/`OR`/`NOT`
  to compose). It may be omitted/null to always run `then`.
- **`then`** / **`else`** are **ordered lists** of operation steps `{stepType, payload}`.
- `stepType` values are **UPPER_SNAKE_CASE** (`WORK_DATA_PATH_EQUALS`, `SET_JSON_PATH`).
- **The server assigns the `id` of the pipeline and of each `if`/`then` step** — do
  not invent them on `add`. (`manage_pipeline` injects the top-level `id` for you
  so later `update`/`delete` is unambiguous.)

### Discover exact shapes with `get_schema`

| What | Call |
|---|---|
| Top-level pipeline | `get_schema("pipelines", "pipeline")` |
| A condition | `get_schema("pipelines", "pipeline/if/WORK_DATA_PATH_EQUALS")` |
| An operation | `get_schema("pipelines", "pipeline/steps/SET_JSON_PATH")` |
| List everything | `get_schema("pipelines")` |

> The operation namespace is **`pipeline/steps/<OP>`**, not `pipeline/then/<OP>`.
> There is no `get_schema("flows", "rules/rule")` (it 404s).

## Conditions (`pipeline/if/<NAME>`)

| Name | Fires when | payload (key fields) |
|---|---|---|
| `AND` / `OR` | all / any child conditions are true | `{ conditions: [ <condition>, … ] }` |
| `NOT` | child condition is false | `{ condition: <condition> }` |
| `WORK_DATA_PATH_EQUALS` | a path equals a value | `{ dataPath, equals }` |
| `WORK_DATA_PATH_EXISTS` | a path is present | `{ dataPath }` |
| `WORK_DATA_CONDITION` | generic work-data predicate | (see schema) |
| `IS_WORK_DELAYED` | work is past its due date | (see schema) |
| `WORK_WITH_OPEN_TASK` / `WORK_WITH_COMPLETED_TASK` | a task is open / completed | `{ taskName }` |
| `USER_IN_ROLE` / `USER_FROM_COMPANY` / `USER_WITH_EMAIL` / `USERS_WITH_EMAILS` | actor identity checks | (see schema) |

Fetch the exact payload before authoring: `get_schema("pipelines", "pipeline/if/<NAME>")`.

## Operations (`pipeline/steps/<NAME>`)

| Group | Steps |
|---|---|
| **Set / copy / remove data** | `SET_JSON_PATH` (set a path; supports `calculations`), `SET_DATA`, `COPY_DATA_PATH`, `REMOVE_DATA_PATH`, `REMOVE_ARRAY_DATA_PATH`, `DISABLE_DATA_PATH`, `OBJECT_TO_ARRAY`, `ENSURE_ARRAY_ITEMS` |
| **Array mapping** | `MAP_SELECT`, `MAP_FIRST`, `MAP_WHERE`, `MAP_WHERE_NOT` |
| **Dates / SLA** | `ADD_DAYS`, `CALCULATE_TIMELINE_DATES_STATUSES` |
| **Alerts** | `ADD_ALERT`, `REMOVE_ALERT` |
| **Relationships / resources** | `GET_RELATED_ENTITY`, `PULL_DATA_FROM_RELATIONSHIP`, `PULL_DATA_FROM_RESOURCE` |
| **Work** | `SET_WORK_ID`, `TRY_SAVE_DATA` |
| **Domain-specific** | `CALCULATE_VINTAGE_TOTALS`, `INITIALISE_VOLUME_TABLE_ROWS` / `…_COLUMNS`, `SYNCHRONISE_DELIVERY_TABLE`, `ENSURE_DATAGRID_ROW_ID_INTEGRITY` |
| **Example/test** | `HelloWorld` |

`SET_JSON_PATH` payload: `{ dataPath, value, fallbackString?, calculations? }`.

## End-to-end recipe

```
1. create_flow_draft(published_flow_id)        → draft_flow_id
2. manage_pipeline(draft_flow_id, "add", pipeline={…})
3. manage_pipeline(draft_flow_id, "list")      # confirm it persisted
4. publish_flow(draft_flow_id)                 # rules only run on a PUBLISHED flow
5. create_work(flow_origin_id)                 → workId
6. update_work_data(workId, "$.priority", "set", "Critical", section_name="<task>")
7. get_work(workId)                            # the pipeline has set $.slaFlag
```

Pipelines run **server-side** when watched work data changes on a **published**
flow — they do not fire while you edit a draft. Verify the effect with
`get_work` — the default `format="slim"` already includes `data`, so
there's no need for the larger `"full"` document.

## Editing / removing

- **Update:** `manage_pipeline(flow_id, "update", rule_id, pipeline={…})` — partial
  PATCH: send only the keys you change; omitted keys (`if`/`then`/`watchPaths`) are
  preserved (the tool reads the current rule and deep-merges before saving). Sending
  a `then`/`else` array REPLACES that whole list, so include every step you want to
  keep (with their ids from `list` to preserve them). Returns `changed: [...]`.
- **Unset keys:** `manage_pipeline(flow_id, "update", rule_id, pipeline={…}, unset=True)`.
- **Delete:** `manage_pipeline(flow_id, "delete", rule_id)`.

## Gotchas

1. **Wrong tool** — `dataPipelines` sent to `update_flow_properties` are silently
   discarded. Use `manage_pipeline`.
2. **Not drafted** — editing rules on a published flow fails; `create_flow_draft` first.
3. **stepType casing** — UPPER_SNAKE (`SET_JSON_PATH`), not `SetDataPath`/PascalCase.
4. **Schema path** — operations are under `pipeline/steps/<OP>`, not `pipeline/then/<OP>`;
   there is no `flows/rules/rule`.
5. **`watchPaths` matter** — an operation only re-runs when a watched path changes.
   Watch the path your condition reads.
6. **Publish to test** — rules don't run on a draft. Publish, then drive a work item.
