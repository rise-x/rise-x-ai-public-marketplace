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
| `add_vector_store_files(vector_store_id, upload_id, filename)` | Step 3: attaches the uploaded file(s) to the store |

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

An app that lets several users create a store for the same owning record concurrently needs a
single-flight guard around the create call — two near-simultaneous creates both succeed and return
different ids, and whichever one loses the race to be saved on the record is orphaned: no
`list_vector_stores` will ever surface it again, and it keeps billing until it expires on its own.

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

**Not the same upload as an app bundle.** `request_vector_store_upload` stages a document for this
ingestion flow; `request_bundle_upload` (`references/managing-apps.md`) stages a federated app's
bundle zip for `deploy_app`. Same staged-upload shape, different consumer — don't cross them.

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
       accepted/converted file's stored NAME (not an id — see § File identity below), a reason for
       a rejection, and `lastError` for a failure
```

**`filename` is always required, zip included.** It's what makes the multipart part a file at all;
omit it and the server has nothing to hang the upload on. For a single non-zip file it also has to
be the real name, or the server can't recognize the file type and stores it as `upload.bin`, which
is rejected outright. A zip is different only in that its *contents* are recognized from its own
bytes, not from the name's extension — the value you pass for a zip's `filename` is otherwise
unconstrained, and the server expands the archive keeping each member's own name. Either way, pass
`filename`. An upload holds up to 500 files and 100 MB total.

### File identity: `indexedAs` is a name, not an id

`indexedAs` in an `add_vector_store_files` result is the filename the OpenAI Files API stored the
file under: the name you gave it, a zip member's own name, or — for a converted file — the
converted output's name (e.g. `notes.eml` → `notes.eml.md`). It is never a `file-…` id. Don't keep
it as a delete or reconciliation handle.

The real file id only comes from `get_vector_store(vector_store_id, include_files=True)`, where each
file entry carries `fileId`, or from the SDK's `vectorStores.listFiles`. To delete or reconcile
files, resolve ids from that listing at the time you act, matching on `indexedAs` — and refuse the
match if it's ambiguous (two files landed under the same name), rather than guessing which one the
caller meant.

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
`POST /api/v1/agent/run`; the apps-sdk exposes the same parameter as `vectorStoreIds` **from
`@rise-x/apps-sdk` 0.14.0** — that release is also where the SDK's own `vectorStores` connector
(create/get/addFiles/listFiles/renew/rename/rebuild/deleteFile/delete) landed. 0.12.0 has neither;
check the resolved version (`node -p "require('./node_modules/@rise-x/apps-sdk/package.json').version"`)
before telling an app author the parameter or the connector exists. Don't put an id like this on
the agent's own configuration: every user of that agent would end up searching the same store,
whether or not the corpus is theirs.

Agent-config `file_search` (`references/managing-agents.md`) remains the right place for a store
that's genuinely meant to be shared: the same corpus for every user of that agent.

A run against an expired or missing store fails with **422**, naming the store and pointing at the
rebuild step below.

**No citation metadata comes back.** Neither an MCP-driven agent run nor the SDK's `AgentReplyState`
/ `AgentRunEvent` / `AgentToolCall` types carry a file id, page, or span from `file_search` — the
tool call surfaces only `tool_name`, a plain-string `result`, and `status`. An app or agent can tell
the user a document was searched, and open it, but can't deep-link a citation to a source location
or render an evidence snippet. Don't design a feature that assumes otherwise.

**MCP vs. SDK surface.** The MCP server exposes five vector-store tools (this reference); the
`vectorStores` connector above exposes nine, including per-file `listFiles`/`deleteFile` that MCP
has no equivalent for. See `references/tool-inventory.md` § MCP surface vs SDK surface for the full
comparison before assuming a capability is missing from the platform rather than just from MCP.

## Renewing, renaming, rebuilding, and file retention

`rebuild` carries three constraints that matter more than what it does:

1. **Post-expiry only.** Calling it on a store that hasn't expired yet is rejected with **409**
   (`still active; renew it instead of rebuilding`) — it is never a cleanup fallback for a live
   store, only the recovery path once one has already lapsed.
2. **No file filter.** It reattaches every surviving file in the whole lineage; there's no argument
   to drop or select files. **410** if none survive. You cannot use `rebuild` to selectively remove
   files from a corpus — that isn't what it's for.
3. **Returns a NEW store id.** The old id is superseded, not reused. Every saved reference to the
   old id — the owning work item, the asset, the agent config — must be updated to the new one.

| `action` | When | Arguments | Effect |
|---|---|---|---|
| `renew` | Before expiry only | `expires_after_days` **required**, at least 1 | Extends `expiresAt` by that many days. **409** once the store already expired; rebuild instead |
| `rename` | Any time | `name` **required**, 1-128 characters; `expires_after_days` rejected | Changes the display name only. The id, files, and expiry are untouched, so this is the way to relabel a store — never rebuild for a new name |
| `rebuild` | After expiry only — **409** otherwise | `expires_after_days` optional | Builds a **new** store, with a **new id**, from every surviving file in the same lineage (no per-file selection). **410** once no original file of that lineage remains. Update every saved reference to the old id |
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
4. **Omitting `filename`, zip included.** `filename` is required on every `add_vector_store_files`
   call — it's what makes the multipart part a file. For a single non-zip file it also has to be
   the real name, or the server can't tell the file's type and stores it as `upload.bin`, which is
   rejected outright. A zip's contents are recognized from its own bytes rather than the filename's
   extension, but a `filename` must still be given, and each member keeps its own name once expanded.
5. **Treating `indexedAs` as a file id.** It's the filename the OpenAI Files API stored the file
   under, never a `file-…` id. Get the real id from `get_vector_store(include_files=True)`
   (`fileId`) or the SDK's `vectorStores.listFiles`, and resolve it at delete/reconciliation time —
   refuse an ambiguous filename match rather than guessing.
6. **Expecting an image to be searchable.** Images are rejected outright, along with msg, rtf, and
   odt: there's no conversion path for them the way there is for spreadsheets and email.
7. **Not confirming the expiry days before creating.** The store expires `expires_after_days` days
   after its last use, sliding forward on every agent run that searches it — not on a `file_search`
   call by itself, and not at all for a store nobody runs against. Tell the user this rule and
   confirm the number before the first `create_vector_store` call (§ Before you create a store).
8. **Renewing after expiry.** `manage_vector_store(action="renew")` only works before the store
   expires. Once it has, the store needs `action="rebuild"` instead, which returns a new id.
9. **Rebuilding or recreating a store just to relabel it.** `action="rename"` changes the display
   name in place and keeps the id, so nothing saved on the owning record has to change. A rebuild
   on a live store fails with 409 anyway, and creating a replacement orphans the corpus.
10. **Expecting `delete` on a rebuilt store's predecessor to free the storage.** A rebuild leaves
    both stores sharing one set of files. Deleting the superseded store keeps those files for the
    live sibling, so usage doesn't drop until the last store of that lineage is deleted.
11. **Rebuilding to selectively drop files.** `rebuild` reattaches every surviving file in the
    lineage — there's no argument to exclude one. It is not a way to remove a bad file from a
    corpus; the only lever there is `delete` (§ Renewing, renaming, rebuilding, and file retention).
