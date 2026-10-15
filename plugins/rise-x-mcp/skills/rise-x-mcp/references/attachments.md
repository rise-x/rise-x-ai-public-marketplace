# Attachments

Files on a work item or asset. Five tools: `request_attachment_upload`, `upload_attachment`,
`list_attachments`, `update_attachment`, `delete_attachment`.

## Contents

- [Upload flow](#upload-flow)
- [Folders](#folders)
- [What the platform does not check](#what-the-platform-does-not-check)
- [Other tools](#other-tools)

## Upload flow

```
1. request_attachment_upload(target, target_id, folder, filename)   # target = "work" | "asset"
                                                                    # → uploadUrl, uploadId
2. curl -X PUT --data-binary @report.pdf '<uploadUrl>'              # stage the bytes
3. upload_attachment(upload_id)                                     # forward to the resource
4. list_attachments(target_id, target, folder)                      # verify the file is there
```

- One file per `uploadId`. Mint again for the next file.
- `upload_attachment` needs EDIT access on the resource. A read-only caller gets a 403 at step 3,
  not at the staging URL.
- A failed `upload_attachment` keeps the staged bytes, so a retry can reuse the same `uploadId`
  within its TTL. Call it once at a time for an id: two racing calls can both send and leave two
  copies.
- The platform route behind step 3 is `POST /api/v4/attachments/work/{workId}/{folder}` (or
  `/asset/{assetId}/{folder}`), multipart, with the file under a repeated `files` field. An app
  calls it through the SDK, not by hand-building the body.
- `list_attachments` takes `resource_type` and an unmatched type returns an **empty list**, not
  an error. Re-check the type before concluding the folder is empty.

## Folders

- **Filename is identity within a folder.** Uploading the same filename into the same folder
  **replaces** the existing file, and nothing in the response says so. Useful on purpose (name the
  file after the work item so a retake supersedes the first). A trap when two callers choose the
  same obvious name.
- **A file is only reachable from a form if something points at its folder.** Either an
  `attachments` component names the folder in `properties.folder`, or a `data-grid` column holds
  it and the returned attachment id is also written to the grid's row path with
  `update_work_data`. A file in a folder neither names is stored and invisible.
- **`properties.folder` is required for an `attachments` component to render an upload control.**
  Without it the component persists and shows nothing. `add_components` and
  `replace_section_components` accept it but return a `known_pitfall` warning on
  `properties.folder`. `update_component` patches are not linted for this, because a patch omits
  every key it does not change.
- Folder names are matched case-insensitively and passed through verbatim. They can contain
  spaces and non-ASCII characters. Get them from the layout or from `list_attachments`.
- `system` is refused as an upload folder.

## What the platform does not check

The route enforces no size limit and runs no malware scan, and validates only the filename
extension (read from the route definition, not probed). The MCP server's own staging cap
(100 MB by default) is the only size bound on this path. Weigh that before pointing an external
party at an attachment folder. The accepted extension list is not discoverable through any tool
(`references/common-pitfalls.md` #71).

## Other tools

- `update_attachment(attachment_id, title?, expiry?)` changes only the fields you pass, and the
  change shows on every work item linking the file.
- `delete_attachment(attachment_id)` flags the file deleted and unlinks it from each work item the
  caller can edit. A work item they cannot edit keeps a dead link, and the call still reports
  success.
