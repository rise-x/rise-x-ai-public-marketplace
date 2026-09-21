# Managing Vector Stores

A **vector store** is a searchable file corpus that an agent's `file_search` tool queries at run
time. Five tools cover its whole lifecycle: create the store, stage and add files, check ingestion
status, and renew, rename, rebuild, or delete it. All five are thin server-side clients of
the Rise-X AI gateway; files sit on the MCP server only transiently, staged until
`add_vector_store_files` consumes them — embeddings never live there.

> **Scope:** these tools manage the store and its files only. Wiring a store's id into an agent or
> a run is covered here in § Using a store in a run, and in more depth by
> `references/managing-agents.md` for the agent-config path.

**Not every server has this release.** On a server without it, all five tools are simply absent —
a call returns tool-not-found, not a permissions or id error. Read that absence as *not supported
here*, tell the user the deployment predates file search, and don't retry with a different
ecosystem or id.

## Before you create a store

A store expires `expires_after_days` days (default 90, capped at 365 — the server rejects anything
above that with a 400) after its **last use**: every **agent run** that searches it resets the
countdown, so a store an agent keeps using never expires and an abandoned one eventually does. A
`file_search` call does not move the clock by itself — the platform re-applies the expiry policy
once per run, after the reply — so a store attached to an agent nobody runs still expires on
schedule. **Tell the user this rule and confirm the number of days before calling
`create_vector_store`.** Getting it wrong is awkward to fix later: renewing an active store is
easy, but rebuilding an expired one costs a new id and a record update everywhere the old one was
saved (see § Renewing, renaming, rebuilding, and file retention).

## Tool inventory

| Tool | What it does |
|---|---|
| `create_vector_store(name, expires_after_days=90, purpose?, resource_type?, resource_id?)` | Creates the store; returns its `id` (an OpenAI `vs_…` id), the store view, and an `expiry` block |
| `get_vector_store(vector_store_id, include_files=False)` | Status, file counts, usage, and expiry; per-file status and `lastError` with `include_files=True` |
| `manage_vector_store(vector_store_id, action, expires_after_days?, name?)` | `action` is required, one of `"renew"` \| `"rebuild"` \| `"rename"` \| `"delete"` — see § Renewing, renaming, rebuilding, and file retention for each one's arguments |
| `request_vector_store_upload()` | Step 1 of adding files: a one-time `uploadUrl` and `uploadId` |
| `add_vector_store_files(vector_store_id, upload_id, filename?)` | Step 3: attaches the uploaded file(s) to the store |

## Saving the id

There is no `list_vector_stores` tool. The `id` in `create_vector_store`'s response is the only
handle to the store; the server keeps no per-user index to browse later. Save it on the record it
belongs to right away, with `update_work_data` (work item) or `edit_asset` (asset), not only in the
conversation. That record's own permissions then decide who can use the store, since the store
itself carries no access control of its own. This is pitfall #66 in
`references/common-pitfalls.md` — losing the id is the one mistake here with no recovery.

`purpose`, `resource_type`, and `resource_id` are optional free-text metadata on
`create_vector_store` (§ Creating a store). They help support staff diagnose an issue and help a
later session find the owning record, but they don't replace saving the id on that record.

## Creating a store

```yaml
create_vector_store(
  name: "Q3 vessel inspection reports"
  expires_after_days: 90              # 1-365; outside that range the server returns a 400
  resource_type: "work"               # free text, max 512 chars, like purpose and resource_id
  resource_id: "<the owning work id>"
)
# → ok: true, id: "vs_abc123...", expiry: {days: 90, expiresAt: "2026-12-08", daysUntilExpiry: 90,
#     notice: "Expires on 2026-12-08 if unused; every agent run resets the 90-day window.
#              Renew before then; after expiry, rebuild."}
```

`purpose`, `resource_type`, and `resource_id` are free-text labels of up to 512 characters each
with no enum behind them, so pick plain values the next reader can act on and don't invent a
taxonomy. `expiry.notice` is server-composed prose meant for the user as-is: surface it, never
parse it.

Confirm the expiry days with the user first (§ Before you create a store), then save `id` on the
owning work item or asset before doing anything else.

## Uploading files

Files travel to the store through a three-step stage-then-attach flow, the same shape as the
app-bundle deploy in `references/managing-apps.md`:

```
1. request_vector_store_upload()
     → uploadUrl          (single-use, short TTL)
     → id                 (the uploadId, keep it for step 3)
     → expiresInSeconds   (the upload URL's TTL)
     → maxUploadMb        (100)

2. curl -X PUT --data-binary @file '<uploadUrl>'
     # one file, or one zip of files (folders inside become each file's `folder` attribute)

3. add_vector_store_files(vector_store_id, upload_id, filename="report.pdf")
     → each file listed as accepted | converted | rejected | failed, with `indexedAs` naming the
       accepted/converted file's stored name, a reason for a rejection, and `lastError` for a failure
```

**Pass `filename` when the upload is a single non-zip file.** Without it, the server has nothing to
recognize the file type from and stores it as `upload.bin`, which is rejected outright. A zip needs
no `filename`: it's detected from its own bytes, and the server expands it keeping each member's own
name. An upload holds up to 500 files and 100 MB total.

### Formats

| Handling | Extensions |
|---|---|
| Searchable as-is | pdf, doc, docx, pptx, txt, md, json, html, css, tex, and code (c, cpp, cs, go, java, js, php, py, rb, sh, ts) |
| Converted to Markdown | xlsx, xls (one Markdown file per non-empty sheet, split every 2,000 rows so every chunk keeps its column names), csv (one Markdown file, no row split), eml (headers plus body; attachments become their own files) |
| Rejected | images, msg, rtf, odt, and anything else not listed above |

## Checking status before a run

```yaml
get_vector_store(vector_store_id: "vs_abc123...")
# → status: "in_progress" | "completed" | "expired"
#   fileCounts: {inProgress, completed, failed, cancelled, total}
#   usageBytes, expiry: {days, expiresAt, daysUntilExpiry, notice}
```

**Poll until `fileCounts.inProgress` reaches 0 before running an agent against the store.** A run
against a store still ingesting files only searches whatever has landed so far, not the full
corpus. Pass `include_files=True` to see each file's own `status` and `lastError` when a count
doesn't add up.

Mention `expiry.daysUntilExpiry` to the user whenever it drops under 7 days — the platform's own
near-expiry notice — so they have time to renew before the store expires and the rebuild path
becomes the only way back.

## Using a store in a run

For a corpus scoped to one conversation or one work item, pass `vector_store_ids` (up to 5) on
`POST /api/v1/agent/run`; the apps-sdk exposes the same parameter as `vectorStoreIds`. Don't put an
id like this on the agent's own configuration: every user of that agent would end up searching the
same store, whether or not the corpus is theirs.

Agent-config `file_search` (`references/managing-agents.md`) remains the right place for a store
that's genuinely meant to be shared: the same corpus for every user of that agent.

A run against an expired or missing store fails with **422**, naming the store and pointing at the
rebuild step below.

## Renewing, renaming, rebuilding, and file retention

| `action` | When | Arguments | Effect |
|---|---|---|---|
| `renew` | Before expiry only | `expires_after_days` **required**, at least 1 | Extends `expiresAt` by that many days. **409** once the store already expired; rebuild instead |
| `rename` | Any time | `name` **required**, 1-128 characters; `expires_after_days` rejected | Changes the display name only. The id, files, and expiry are untouched, so this is the way to relabel a store — never rebuild for a new name |
| `rebuild` | After expiry | `expires_after_days` optional | Builds a **new** store from the same lineage's surviving files and returns a **new id**. Update every record that held the old one. **409** if the store hasn't expired yet — renew it instead; **410** once no original file of that lineage remains |
| `delete` | Any time | none; `expires_after_days` and `name` both rejected | Removes the store, and its files with it — unless a rebuilt sibling still shares that lineage, in which case the files stay behind for the sibling and only the store goes |

Each rule is enforced: `name` on anything but `rename`, or `expires_after_days` on `rename`/
`delete`, comes back as a validation error rather than being ignored.

After a store expires, its original files stay in the Files API — nothing deletes them
automatically yet. A rebuild reattaches whatever's still there; `rebuild` only 410s once every
file from that lineage is gone, whether from a prior `delete` or a future retention sweep (not
built today).

## Cost

OpenAI bills vector-store storage per GB per day. The sliding expiry (every agent run that
searches the store resets the countdown) is the cost control: a store nobody runs against stops
accruing that cost on its own, without anyone having to remember to delete it.

## Pitfalls

1. **Forgetting to save the id.** There's no `list_vector_stores` tool: the id `create_vector_store`
   returns is the only handle to the store. Save it on the owning work item or asset in the same
   turn you create it, before anything else.
2. **Running an agent before ingestion finishes.** `get_vector_store` can report
   `status: "completed"` at the store level while `fileCounts.inProgress` is still non-zero for
   files added later. Poll `fileCounts.inProgress == 0` before the run, not only the top-level
   status.
3. **Putting a per-conversation or per-work-item id on the agent's own config.** `file_search`'s
   `config.vector_store_ids` on an agent is shared by every user of that agent. Use
   `vector_store_ids` on the run call (`vectorStoreIds` in the apps-sdk) for anything scoped
   narrower than "every user of this agent."
4. **Uploading a single non-zip file without `filename`.** Without it, the server can't tell the
   file's type from bytes alone and stores it as `upload.bin`, which is rejected outright. A zip
   needs no `filename`: it's detected from its own bytes, and each member keeps its own name.
5. **Expecting an image to be searchable.** Images are rejected outright, along with msg, rtf, and
   odt: there's no conversion path for them the way there is for spreadsheets and email.
6. **Not confirming the expiry days before creating.** The store expires `expires_after_days` days
   after its last use, sliding forward on every agent run that searches it — not on a `file_search`
   call by itself, and not at all for a store nobody runs against. Tell the user this rule and
   confirm the number before the first `create_vector_store` call (§ Before you create a store).
7. **Renewing after expiry.** `manage_vector_store(action="renew")` only works before the store
   expires. Once it has, the store needs `action="rebuild"` instead, which returns a new id.
8. **Rebuilding or recreating a store just to relabel it.** `action="rename"` changes the display
   name in place and keeps the id, so nothing saved on the owning record has to change. A rebuild
   on a live store fails with 409 anyway, and creating a replacement orphans the corpus.
9. **Expecting `delete` on a rebuilt store's predecessor to free the storage.** A rebuild leaves
   both stores sharing one set of files. Deleting the superseded store keeps those files for the
   live sibling, so usage doesn't drop until the last store of that lineage is deleted.
