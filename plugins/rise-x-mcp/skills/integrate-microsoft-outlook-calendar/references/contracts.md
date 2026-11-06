# Endpoint and app contracts

The executable contract is [templates/endpoints.json](../templates/endpoints.json).
The setup script fixes the mailbox/calendar in saved URIs. Runtime data cannot
choose a different mailbox. Authentication is Microsoft OAuth client credentials
against the configured tenant with Graph `.default`; server credentials belong in
Rise-X. The dedicated identity must not have additive Entra application grants.

| Alias | Runtime input | Result |
| --- | --- | --- |
| calendarQuery | from/to, ISO 8601 with offset | calendarView items; reject nextLink |
| calendarGet | raw opaque eventId | organizer event |
| calendarCreate | subject, composed HTML description, start/end UTC, transactionId, attendees | HTML event with nonempty id |
| calendarUpdate | eventId, subject, start/end UTC | event; body and attendees omitted |
| calendarSetAttendees | eventId, full desired typed attendees array | event; body contains only attendees |
| calendarCancel | eventId, comment | 202 empty body allowed; verify resulting state |

Use actual calendar IDs, not display names. Do not pre-encode runtime event IDs;
the endpoint renderer encodes them. Send attendees as native JSON, including `[]`,
not a JSON string. Templates use bare `$.attendees` for native substitution;
`{$.attendees}` is string interpolation and is incorrect here. Preserve optional
and resource recipient metadata when changing the complete list. Re-read the
organizer event before replacement; this alone is not atomic concurrency control.
Use a single operator for acceptance and reject stale snapshots in an app.

## Consume generated app bindings

Setup writes `rise-x-app.json` in the private setup directory. It contains six
`kind: integration` dependencies, each with
`ids[environment].integrationId` and `ids[environment].endpointId`. Merge these
into the real app manifest; do not replace its other properties or invent IDs.
Keep the deployment environment aligned with the exported environment. Follow the
installed `rise-x-apps` skill and SDK README for manifest validation and building.

Inspect the installed SDK before coding. SDK versions supporting integration
dependencies expose `getAppDependencies` / `useAppDependencies` from
`@rise-x/apps-sdk/dependencies`, with `integration.invoke({data})` on each bound
alias. Do not call Graph directly from a browser or add secrets to app source.
Disable live actions if bindings are absent. Do not queue calendar writes offline.

Check transport errors **and** the external invocation envelope: `success`,
`statusCode`, `items` or `response`. The scripts' `Invoke-CalendarOperation` and
`Get-CalendarEventResult` demonstrate normalization, including absent response
fields and empty cancellation bodies. A query containing `@odata.nextLink` is
incomplete: narrow the range or implement reviewed pagination before making an
availability claim. No automatic write retries; persist transactionId before a
create and retain it for reconciliation. Re-read after mutation.

## HTML and migrations

All creates use `body.contentType: HTML`. Compose ordinary text using
`ConvertTo-CalendarHtmlBody` in [Calendar.Common.ps1](../scripts/Calendar.Common.ps1)
or an equivalent escaping helper in the app. Validate the optional app link as the
actual deployed HTTPS URL for this environment, without credentials, query or
fragment. Sanitize returned HTML before display. Update and attendee endpoints
omit body, preserving descriptions. Teams fields are omitted entirely.

For an existing Text create endpoint, prepare all callers to send composed HTML,
review outstanding transactions, then run the same guided setup command with
`-UpgradeHtmlTemplate`. Preview first with `-WhatIf`; preview does not read live
differences. Apply checks the recognized Text-to-HTML change, preserves endpoint
IDs, credentials and metadata, and reads back the result. Missing GET request
bodies and known default endpoint authentication are accepted; missing write
bodies, custom authentication, extra headers or parameters require review.

Do not advertise Teams support based on a capability snapshot. This recipe has
no Teams create endpoint, separate body editor or recurring-event mutation path.
