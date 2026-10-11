# Demo validation and promotion

This draft supplies a starting three-endpoint appointment recipe. There is no
bundled PowerShell implementation or completed calendar demo yet. Do not report
those deliverables or live validation as complete.

## Develop with the test app

Copy/adapt the provider skill into the test app's local skill directory and link
it from the app's agent instructions. Verify discovery from the actual working
directory; use an explicit local path if needed. Keep scripts/templates beside
the app while evaluating them, and record the source revision to avoid competing
copies. The installed generic Rise-X MCP and app-building skills still own their
shared authoring and UI workflows.

The expanded demo needs three additional fixed-target operations: GetEvent,
SetAttendees, and CancelEvent, plus an attendee-aware CreateEvent variant. Give
each saved operation its own environment-bound app alias. Verify typed array
substitution, including an empty attendee array, and cancellation's empty success
response against the deployed Rise-X engine before shipping templates for them.

For attendee changes, read the organizer event, compute the complete desired
recipient collection, preserve recipient types, and deduplicate addresses. Send
only the attendees property. Detect changes since the editor opened and ask the
operator to refresh; this preflight is not atomic concurrency control. Adding an
invite recipient does not grant mailbox/calendar access. Use individual controlled
test recipients; keep distribution-list and recurring-series edits out of the
first demo. Microsoft's update documentation describes notification differences
for attendee-only updates and distribution lists.
[Update event](https://learn.microsoft.com/en-us/graph/api/event-update?view=graph-rest-1.0).

Cancellation uses the organizer event and can succeed without returning an event
object. Verify the resulting calendar state separately from recipient message
delivery. Restrict cleanup to recorded demo items.
[Cancel event](https://learn.microsoft.com/en-us/graph/api/event-cancel?view=graph-rest-1.0).

## PowerShell work to validate locally

Implement setup/reuse of Microsoft resources and scoped permissions, Rise-X
integration upserts, secret-free app binding export, connection checks, and
ownership-limited cleanup. Support PowerShell 7, WhatIf with no mutation, secure
secret input, redacted errors, idempotent reruns, and recovery after partial
failure. Preserve reused resources and unrelated endpoints; never rotate secrets
or widen permissions implicitly. Test these behaviours with mocked Pester cases
before running the scripts against a tenant.

## Evidence before promotion

- Record tested SDK, deployed API/MCP, PowerShell/module versions, environment,
  authorized operator/mailbox, and controlled recipient inputs privately.
- Verify range query, no-attendee appointment, invite to A, adding B, removing A,
  rescheduling, cancellation, and denied access to an excluded mailbox.
- Check organizer event state and recipient invitation/update/cancellation
  messages separately; API success alone does not prove delivery.
- Exercise incomplete pages, failed result envelopes, transport failures,
  unknown create outcome, missing ID, duplicate recipients, stale edits, empty
  attendee array, and empty cancellation response with fixtures.
- Record local skill outcomes for reuse, missing inputs/tools, secure secret
  handling, changed invocation inputs, and real-write authorization.

Absent tenant access leaves live acceptance pending. Promote only after the app
acceptance evidence and AI tool findings have been reviewed: remove lab-only
assumptions, bundle reusable resources, validate links, and rerun the skill outside
the app checkout. Then make the public skill the reusable authority and retain
only app-specific guidance plus a released-version reference in the lab.
