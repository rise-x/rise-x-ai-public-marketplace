# Data Pipelines (Flow Rules)

Data pipelines are **flow rules that run automatic data operations when watched
work data changes**. A pipeline watches one or more JSON paths; when they change
and an optional condition is met, it runs a list of operations against the work
item's data (set a value, copy a path, map an array, add days to a date, raise
an alert, pull data from a related entity, …).

Manage them with **`manage_pipeline`** on a draft flow. Its description gives the
pipeline shape and the `get_schema` calls for each condition and operation.

## Pipeline shape

```jsonc
{
  "name": "Escalate critical requests",
  "watchPaths": ["$.priority"],          // paths that trigger a re-run on change
  "if":   { "stepType": "WORK_DATA_PATH_EQUALS",
            "payload": { "dataPath": "$.priority", "equals": "Critical" } },
  "then": [ { "stepType": "SET_JSON_PATH",
              "payload": { "dataPath": "$.slaFlag", "value": "EXPEDITE" } } ]
}
```

- Compose several conditions with `AND` / `OR` (payload `{ "conditions": [ <condition>, … ] }`) and `NOT` (payload `{ "condition": <condition> }`). Omit `if` (or send `null`) to always run `then`.
- Do not invent step ids on `add`; the server assigns them. `manage_pipeline` assigns the pipeline's own `id` so a later `update` or `delete` is unambiguous.
- Condition schemas live under **`pipeline/if/<NAME>`** and operation schemas under **`pipeline/steps/<OP>`**, not `pipeline/then/<OP>`: `get_schema("pipelines", "pipeline/if/WORK_DATA_PATH_EQUALS")`. There is no `get_schema("flows", "rules/rule")` (it 404s). `get_schema("pipelines")` lists every condition and operation.

## Choosing conditions and operations

Conditions: `AND`, `OR`, `NOT`, `WORK_DATA_PATH_EQUALS` (`{dataPath, equals}`), `WORK_DATA_PATH_EXISTS` (`{dataPath}`), `WORK_DATA_CONDITION`, `IS_WORK_DELAYED` (work is past its due date), `WORK_WITH_OPEN_TASK` / `WORK_WITH_COMPLETED_TASK` (`{taskName}`), and the actor checks `USER_IN_ROLE`, `USER_FROM_COMPANY`, `USER_WITH_EMAIL`, `USERS_WITH_EMAILS`.

| Group | Operations |
|---|---|
| **Set / copy / remove data** | `SET_JSON_PATH` (set a path; supports `calculations`), `SET_DATA`, `COPY_DATA_PATH`, `REMOVE_DATA_PATH`, `REMOVE_ARRAY_DATA_PATH`, `DISABLE_DATA_PATH`, `OBJECT_TO_ARRAY`, `ENSURE_ARRAY_ITEMS` |
| **Array mapping** | `MAP_SELECT`, `MAP_FIRST`, `MAP_WHERE`, `MAP_WHERE_NOT` |
| **Dates / SLA** | `ADD_DAYS`, `CALCULATE_TIMELINE_DATES_STATUSES` |
| **Alerts** | `ADD_ALERT`, `REMOVE_ALERT` |
| **Relationships / resources** | `GET_RELATED_ENTITY`, `PULL_DATA_FROM_RELATIONSHIP`, `PULL_DATA_FROM_RESOURCE` |
| **Work** | `SET_WORK_ID`, `TRY_SAVE_DATA` |
| **Domain-specific** | `CALCULATE_VINTAGE_TOTALS`, `INITIALISE_VOLUME_TABLE_ROWS` / `…_COLUMNS`, `SYNCHRONISE_DELIVERY_TABLE`, `ENSURE_DATAGRID_ROW_ID_INTEGRITY` |

Fetch the exact payload with `get_schema` before you author a step.

## End-to-end recipe

```
1. create_flow_draft(published_flow_id)        → draft_flow_id
2. manage_pipeline(draft_flow_id, "add", pipeline={…})
3. manage_pipeline(draft_flow_id, "list")      # confirm it persisted
4. publish_flow(draft_flow_id)                 # rules only run on a PUBLISHED flow
5. create_work(flow_origin_id)                 → workId
6. update_work_data_bulk(workId, {"$.priority": "Critical"}, section_name="<task>")
7. get_work(workId)                            # the pipeline has set $.slaFlag
```

Pipelines run **server-side** when watched work data changes on a **published**
flow — they do not fire while you edit a draft. Verify the effect with
`get_work` — the default `format="summary"` already includes `data`, so
there's no need for the larger `"full"` document.

## Editing

`manage_pipeline(flow_id, "update", rule_id, pipeline={…})` takes only the keys you
change; the tool reads the current rule and merges your keys into it, so omitted keys
(`if` / `then` / `watchPaths`) are kept. A `then` or `else` array you send REPLACES
the whole list, so include every step you want to keep (with their ids from `list`).

## Gotchas

1. **`watchPaths` matter** — an operation only re-runs when a watched path changes.
   Watch the path your condition reads.
2. **Publish to test** — rules don't run on a draft. Publish, then drive a work item.
