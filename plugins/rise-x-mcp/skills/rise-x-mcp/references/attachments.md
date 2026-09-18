# Work Attachments

Files held against a work item, separate from its form data. The `attachments`
layout component renders the picker; the upload itself goes to a dedicated
route that none of the layout or work tools cover.

> **Verification status.** The route, the multipart shape, the
> replace-on-same-filename behaviour, and the `properties.folder` requirement
> were exercised live against a test deployment. The absence of a size cap and
> of a malware scan is **read from source, not exercised** (the attribute and
> the missing Kestrel cap are visible in the handler; no oversized upload was
> actually pushed through). Which codebase implements the deployed
> `upload_attachment` / `request_attachment_upload` MCP tools is **not
> established** — see § MCP tools at the end. The wire contract below holds
> regardless of which server serves it.

## The upload route

```
POST /api/v4/attachments/work/{workId}/{folder}

Content-Type: multipart/form-data
Form field:   files          # repeated once per file, same field name
```

- `{workId}` is the work item's GUID, the same id `get_work` takes.
- `{folder}` is the folder name the file lands in. It must match the
  `properties.folder` of the `attachments` component that is meant to display
  it (see § 3. The layout component needs `properties.folder`), or the file
  uploads and nothing shows it.
- The field name is `files` for every file. Two files means the field appears
  twice, not `files[0]` / `files[1]`.

```
# CRITICAL: never set the multipart Content-Type header by hand. The boundary
#   token is generated with the body — a hand-written header carries the wrong
#   boundary and the server parses zero parts. Hand a FormData to fetch and let
#   the browser write the header.
form = new FormData()
form.append("files", file)             # repeat per file
fetch(url, {method: "POST", body: form})   # no Content-Type header
```

### From inside a federated app

An app reaches the same route through the SDK's API accessor rather than
building the URL itself:

```
1. api = getApiV4("attachment")          # the attachment-scoped v4 client
2. api.post(`work/${workId}/${folder}`, form)
```

Authoring a federated app is otherwise out of scope here — see the
`rise-x-apps` skill in this marketplace for the SDK, the shell accessors, and
the app lifecycle.

## What to design around

Three behaviours shape how an app should name and bound its uploads.

### 1. Same filename + same folder REPLACES the earlier file

A second upload of `report.pdf` into the same folder versions over the first
one. It does not add a second attachment. This is worth using deliberately:
name a file after the work item and a retake supersedes the first attempt
instead of leaving two files for a reader to choose between. It also means an
app that uploads under a fixed name cannot accumulate a history.

```
# NOTE: to keep every upload, make the filename unique yourself — the server
#   will not disambiguate for you.
upload(workId, "evidence", file named `${workCode}-${timestamp}.pdf`)

# NOTE: to make a retake supersede the previous one, do the opposite.
upload(workId, "evidence", file named `${workCode}.pdf`)
```

### 2. No size limit and no malware scan

The endpoint carries `[DisableRequestSizeLimit]` and there is no Kestrel cap in
front of it, so the server enforces **no** upload size. Nor does it run a
malware scan. Validation is **filename-extension only**.

> ⚠️ **The caller's own UI is the only bound that exists.** An app that wants a
> size ceiling, a content-type check, or a scan must implement it before the
> POST — nothing downstream will. Treat a stored attachment as untrusted
> content, the same as any other user-supplied file.

### 3. The layout component needs `properties.folder`

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

The folder is also the coupling between the component and the route: a file
posted to `/evidence` appears in the component whose `folder` is `evidence`,
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

## An agent cannot read a work attachment

A platform AI agent has **no** path to a file stored on a work item. This is an
architectural boundary, not a missing parameter: an app that wants an agent to
read a document holds the file once and sends it twice. The mechanics, and the
two ways a file does reach an agent, are in
`references/managing-agents.md` § Feeding a file to an agent.

## MCP tools

Some deployments expose `upload_attachment`, `request_attachment_upload`,
`list_attachments`, `update_attachment`, and `delete_attachment` as MCP tools.
Their signatures and behaviour are **not documented here** because they were
not exercised, and which codebase implements them is still being established —
do not assume they mirror the route above. If they are absent on the server you
are talking to, read that as not supported there, not as a permissions
problem. The route in § The upload route is the contract that was verified.
