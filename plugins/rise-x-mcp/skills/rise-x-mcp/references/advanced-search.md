# Advanced Search

The descriptions of `search_works`, `search_assets`, `search_flows`, `search_companies`, and `get_flow_data_schema` carry the field keys, operators, enum rules, the `flowOriginId` pin, and the default status filters. This file holds the recipes that combine calls, and the search behaviour those descriptions do not state.

## When to use these tools, and when not to

If the user names a filter, condition, sort, range, or a set of fields, use `search_*`. Never call `list_work` / `list_flows` / `list_assets` and filter the rows in your own loop: that ignores paging, cannot sort, and costs more. `list_*` is only for the unfiltered "give me everything" case.

## Recipe: find the flow, read its schema, filter and project in one call

```python
# 1. Find the flow (an asset type is flowResourceType "Entity")
flows = await search_flows(filter={"and": [
    {"field": "flowResourceType", "operator": "equals", "values": ["Work"]},
    {"field": "displayName", "operator": "contains", "values": ["invoice"]},
]})
origin = flows["items"][0]["flowOriginId"]

# 2. Read the data.* paths
schema = await get_flow_data_schema(flow_origin_id=origin)

# 3. Filter, sort, and project in ONE call
await search_works(
    filter={"and": [
        {"field": "flowOriginId", "operator": "equals", "values": [origin]},
        {"field": "data.invoice.amount", "operator": "greaterThanOrEqual", "values": [1000]},
    ]},
    sort=[{"field": "data.invoice.dueDate", "order": "desc"}],
    fields=["id", "displayName", "data.invoice.amount", "data.invoice.dueDate"],
    enforce_fields=True,
)
```

- Use each path exactly as the schema returns it.
- A data-grid or repeater column has `isArray: true`. In `filter` and `sort`, write it without an index: the server strips any index, and the leaf matches when any row matches. In `fields`, the un-indexed path projects every row's value, and `data.grid[0].col` selects one row.

## Counts & pagination

- To page, loop while `hasMore` is true and increase `page`.
- `page_size` above 100 is clamped to 100, and `page_size` below 1 is raised to 1. Both return 200 with the clamped `pageSize` and a `warnings[]` entry. A page of 100 rows after you asked for 500 is the clamp, not the whole result.
- To count by a field, run one search per value with `page_size=1` and `include_total_count=True`, and read `totalCount`. Add a `status` leaf, or the default status filter hides rows from the count.

```python
for status in ["Open", "Closed", "Completed"]:
    page = await search_works(
        filter={"and": [
            {"field": "flowOriginId", "operator": "equals", "values": [origin]},
            {"field": "status", "operator": "equals", "values": [status]},
        ]},
        page_size=1, include_total_count=True,
    )
    counts[status] = page["totalCount"]
```

## Recipe: date-range filter

- Pass dates as ISO 8601 strings. A value with no offset is read as UTC. A value that does not parse is a 400.
- `between` takes exactly two values and includes both ends.
- On a `data.*` date, write the path the schema returns (`data.invoice.dueDate`). Do not append `.date`: the server compares the stored `.date` value for you. A row whose date is missing or does not parse never matches, so it does not leak into a `lessThan` result.

```python
filter={"and": [
    {"field": "flowOriginId", "operator": "equals", "values": [origin]},
    {"field": "lastModified", "operator": "between",
     "values": ["2026-11-01T00:00:00Z", "2026-11-30T23:59:59Z"]},
    {"field": "data.invoice.dueDate", "operator": "lessThan", "values": ["2026-12-01"]},
]}
```

## Default status exclusion

- To count every non-deleted work, add `status in ["Open", "Closed", "Completed", "Ok"]` on the AND-spine. Add `"Deleted"` to include deleted works.
- The Work default matches only the string form of `status`. Older works that store `status` as a number stay hidden until the filter has a `status` leaf. A caller `equals` / `in` on `status` matches both forms.

## Recipe: match a word inside a Work or Asset value

`search_works` and `search_assets` reject `contains` and `endsWith`, and there is no wildcard: `equals` with `"*fire*"` matches the literal string. For a prefix, use `startsWith`. For a word anywhere in the value, project the field with `fields` and `enforce_fields=True`, page through, and match on your side.

## Operators

Rules the tool descriptions do not state:

- Filter groups nest at most 5 levels deep.
- Range operators (`greaterThan`, `greaterThanOrEqual`, `lessThan`, `lessThanOrEqual`, `between`) work on number and date fields only. On a string or enum field they are a 400.
- `exists` and `notExists` take no values. Use them for "has any value" and "has no value".
- `in` and `notIn` are a 400 on a boolean field. Boolean `equals` matches both `true` and `"true"`.
- A guid field takes `equals`, `notEquals`, `in`, `notIn`, `exists`, and `notExists`.
- Every `flowOriginId` pin value must be a GUID. A flow name is a 400.
- On an array-valued field (Work `environments`, `assignedRoles`, `entities`; Company `domains`), `in` matches any element. Work `entities` holds the asset ids a work is linked to.
- Flow `publishMode` is one of `Default`, `UpdateOpenItems`, `DoNotUpdateOpenItems`. Like `flowResourceType`, it and `resourceType` compare the exact string, so a wrong-cased value matches nothing.
- Flow `resourceType` does not tell a workflow from an asset type; filter on `flowResourceType` for that.
- `search_flows` returns only the current non-draft version, so `publishStatus` `Revised` also returns an empty page.
- Flow `flowId` is an alias for `id`. Flow `environment` is the environment name label; the server already scopes every search to the active environment, so you rarely need it.
- Asset `status` `Closed` is reserved and not in use; an asset is `Open` or `Deleted`.

## `enforce_fields` Behaviour

- `enforce_fields=True` with `fields` empty or omitted still returns the default projection.
- A `fields` entry that is null, empty, or has a segment starting with `$` is a 400.
- An unknown projection path is not a 400; the response carries a `warnings[]` entry (a plain string). An unknown whitelist key is left out of the rows. An unknown `data.*` path is projected as sent and may come back empty. A `data.*` path that matches the schema only by case gets a warning that names the declared spelling; run the search again with that spelling.
- Every response carries `id` and `status` (Work, Asset), `id`, `state` and `flowResourceType` (Flow), or `id` (Company), whatever `fields` says.
- The default Asset projection is `displayName`, `entityType`, `code`, `flowId`, `flowOriginId`, `companyId`, the dates, `createdBy`, and `lastModifiedBy`. The default Flow projection is `name`, `normalisedName`, `displayName`, `description`, `uniqueName`, `group`, `entityType`, the ids, the dates, `createdBy`, and `lastModifiedBy`. Any other Flow key, `publishStatus` included, needs `fields` with `enforce_fields=True`.
- `statusDisplay`, `acl`, `stepEvents`, `attachments`, `relationships`, and `integrationReceipts`: list the bare key. A sub-path such as `statusDisplay.color` still projects the whole object. `acl` comes back trimmed to `principals`, `userRoles`, and `rolePermissions`.
- `assignedUsers`: the bare key returns every member of each user. A sub-path such as `assignedUsers.email` returns only that member; the others come back `null`, or the all-zero GUID for `id` and `companyId`.
- `data.*` sub-paths trim the payload to those paths. The bare `"data"` key returns the whole tree and wins over any `data.*` path in the same request. On Work it is the data of every section merged in order, a later section overriding an earlier one.
- In the response, `data` is a nested object (`{"data": {"step2": {"toggle1": true}}}`), not dotted keys. A `data.*` date comes back as a `{date, ticks, offset}` object.

## Asset Search and `data.*` Fields

`search_assets` projects like `search_works`, with two shape differences:

- A data-grid column path such as `data.equipments.serialNumber` returns a flat array of that column's values on Work, and the stored rows on Asset. To get whole rows on either, project the container path (`data.equipments`).
- A requested `data.*` path the asset does not have is left out (all absent gives `data: {}`). Work returns it as `null`.

## Multi-flow searches

`flowOriginId in [a, b]` merges the flows' schemas.

- An unknown flow id is a 400 only when the request names a specific `data.*` path. Without one, the search returns an empty page.
- A path whose type differs between the flows is a 400 only when a filter or sort uses it. Narrow the search to one flow, or project the bare `"data"` key.
