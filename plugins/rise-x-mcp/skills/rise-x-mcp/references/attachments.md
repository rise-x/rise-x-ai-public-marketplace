# Work Attachments

Files held against a work item or asset, separate from its form data. The
`attachments` layout component renders the picker; from MCP, the dedicated
attachment tools do the upload — never build the platform's multipart route by
hand.

> **Verification status.** The replace-on-same-filename behaviour and the
> `properties.folder` requirement were exercised live against a test
> deployment.

## Tool inventory

| Tool | Purpose |
|---|---|
| `request_attachment_upload(target, target_id, folder, filename, content_type?)` | Step 1 — mint a one-time upload URL + `uploadId` for one file, bound to the work item or asset and folder |
| *(client)* `PUT <uploadUrl>` | Step 2 — send the file bytes to the URL from step 1 (the envelope's `hint` carries a ready `curl`) |
| `upload_attachment(upload_id)` | Step 3 — forward the staged file to its resource; needs EDIT access on that resource |
| `list_attachments(resource_id, resource_type, folder)` | The attachments a work item or asset holds in one folder — use it to verify an upload landed |
| `update_attachment(attachment_id, title?, expiry?)` | Change title and/or expiry; omitted fields are left alone |
| `delete_attachment(attachment_id)` | Flag deleted and unlink from every work item the caller can edit |

## Uploading a file

```
1. request_attachment_upload(target="work", target_id=workId,
                             folder="evidence", filename="report.pdf")
     → uploadUrl, uploadId         # one file per uploadId — mint again for the next
2. PUT the bytes to uploadUrl      # single use, short-lived
3. upload_attachment(uploadId)     # → attachment id, target, folder, counts
4. list_attachments(workId, "work", "evidence")   # CRITICAL: confirm it appears
```

- `folder` must be one an `attachments` layout component names in its
  `properties.folder` (see § The layout component needs `properties.folder`),
  or one that belongs to a data-grid column. A file in a folder nothing names
  is stored and invisible.
- A data-grid column's folder needs one more step: the returned attachment id
  has to be written to the grid's row path with `update_work_data`, or the
  file is in the folder but in no cell.
- Call `upload_attachment` once per `uploadId` at a time. Two calls racing on
  the same id can both send, and the resource ends up with two copies.

## What to design around

### 1. Same filename + same folder REPLACES the earlier file

A second upload of `report.pdf` into the same folder versions over the first
one. It does not add a second attachment, and nothing in the response says a
file was superseded.

```
# NOTE: to keep every upload, make the filename unique yourself — the server
#   will not disambiguate for you.
request_attachment_upload("work", workId, "evidence", filename=`${workCode}-${timestamp}.pdf`)

# NOTE: to make a retake supersede the previous one, do the opposite.
request_attachment_upload("work", workId, "evidence", filename=`${workCode}.pdf`)
```

### 2. The layout component needs `properties.folder`

An `attachments` component with no `folder` in its `properties` displays **no
upload control at all**. The field renders and does nothing.

```
add_components(layoutId, [{
  "component": "attachments",
  "label": "Evidence",
  "properties": {
    "width": "col-12",
    "folder": "evidence"        # CRITICAL: no folder, no upload control
  }
}], parent_section_id=sectionId)
```

The folder is also the coupling between the component and the upload: a file
uploaded to `evidence` appears in the component whose `folder` is `evidence`,
and nowhere else. Two components may not share a folder unless you intend both
to show the same files.

## Reading attachments back

`get_work(id)` carries an `attachments` key in its summary projection.
`get_asset(entity_id)` drops it — its `omitted` note lists `attachments` among
the keys that carried a value, so pass `format="full"` on an asset. See
`references/managing-assets.md` § `get_asset`.

Relationship sync can copy attachments between related items with
`attachmentOperations` (`sourceFolder` → `destinationFolder`) and the
`includeAttachments` flag — see `references/relationships.md`.

## Letting an agent read an attachment

A platform AI agent reads a work item's attachments through the Rise-X MCP
server: add the server to the agent's `mcp_servers`, mention it in the system
prompt, and pass the work id as context on the run. See
`references/managing-agents.md`.
