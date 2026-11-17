# Managing Vector Stores

A **vector store** is a searchable file corpus that an agent's `file_search` tool queries at run
time. Five tools cover its lifecycle; their descriptions give the arguments, the formats, and
the return shapes. Files sit on the MCP server only until `add_vector_store_files` consumes
them; embeddings never live there.

**Not every server has this release.** On a server without it, all five tools are absent and a
call returns tool-not-found, not a permissions or id error. Tell the user the deployment
predates file search, and do not retry with another ecosystem or id.

## Before you create a store

A store expires `expires_after_days` days after its **last use**. Omit the argument to take
the platform default of 90 days; the deployment caps the value (365 by default), and an
explicit value above the cap is a 400. Every **agent run** that searches the store resets the
countdown, so a store in use never expires and an abandoned one does. A `file_search` call
alone does not move the clock (the platform re-applies the policy once per run, after the
reply), so a store attached to an agent nobody runs still expires on schedule.

Tell the user this rule and confirm the number of days before calling `create_vector_store`.
Renewing an active store is easy; rebuilding an expired one costs a new id and an update of
every record that saved the old one.

OpenAI bills storage per GB per day. The sliding expiry is the cost control: a store nobody
runs against stops costing money without anyone deleting it.

## Saving the id

There is no `list_vector_stores` tool and no index to browse. The `id` from
`create_vector_store` is the only handle, so save it on the record it belongs to in the same
turn: `update_work_data` for a work item, `edit_asset` for an asset. An unsaved id cannot be
recovered, and the store keeps billing until it expires. The store has no access control of its
own; the owning record's permissions decide who can use it. Save the new id the same way after
`manage_vector_store(action="rebuild")`.

`purpose`, `resource_type`, and `resource_id` are free-text labels with no enum behind them.
Use plain values that help support staff and a later session find the owning record; they do
not replace saving the id on that record. `expiry.notice` is server-written text for the user:
show it, never parse it.

## Adding files

`request_vector_store_upload` → PUT one file, or one zip of files, to the `uploadUrl` →
`add_vector_store_files(vector_store_id, upload_id, filename)`.

- `filename` is required and must carry the right extension (`report.pdf`, `batch.zip`): the
  gateway reads the type from it. A zip is expanded, each member keeps its own name, and a
  folder inside the zip becomes that file's `folder` attribute.
- One upload holds up to 500 files and 100 MB in total.
- Spreadsheets convert to Markdown: one file per non-empty sheet, split every 2,000 rows so
  each chunk keeps its column names. CSV becomes one Markdown file with no row split. An `.eml`
  becomes headers plus body, and its attachments become their own files.

## Checking status before a run

Indexing is asynchronous. **Poll `get_vector_store` until `fileCounts.inProgress` is 0** before
you run an agent against the store; a run during ingestion searches only what has landed.
The store-level `status` can read `completed` while files added later are still in progress, so
check the count, not only the status. Pass `include_files=True` to see each file's `status` and
`lastError` when a count does not add up.

## Using a store in a run

For a corpus scoped to one conversation or one work item, pass `vector_store_ids` (up to 5) on
`POST /api/v1/agent/run`; the apps SDK exposes it as `vectorStoreIds`. Do not put such an id on
the agent's own configuration: every user of that agent would search it. The agent's
`file_search` config (`references/managing-agents.md`) is for a corpus every user of the agent
shares.

A run against an expired or missing store fails with **422**, naming the store and pointing at
the rebuild step.

## Renewing, renaming, rebuilding, and file retention

- **Renew** works only before expiry (409 after). After expiry, **rebuild**: it returns a new
  id, so update every record that held the old one. Rebuild on a live store is a 409.
- To relabel a store, **rename** it. It keeps the id, files, and expiry. Never rebuild or create
  a replacement just for a new name.
- After a store expires, its original files stay in the Files API; nothing deletes them yet. A
  rebuild reattaches whatever is still there and returns 410 only once every file of that
  lineage is gone.
- A rebuild leaves the old and the new store sharing one set of files. **Deleting** the
  superseded store keeps those files for the live sibling, so usage does not drop until the
  last store of the lineage is deleted.
