# Configure the Rise-X endpoints

Load the shared [authoring protocol](../../rise-x-mcp/references/integration-authoring.md)
before configuring or testing. Confirm actual tool schemas and the deployed API
support; local examples are not proof that a deployment has these operations.

## Configure

1. Select the requested deployment/ecosystem and discover existing integrations.
   Reuse a compatible one; read it before editing and preserve unrelated endpoints.
2. Adapt [integration.template.json](../assets/integration.template.json). Replace
   every `<...>` value and `bookings@example.com`. Obtain the client-secret value
   through the authorized secret input channel and use `isSecret: true`. Do not
   save filled-in credentials in this template, a public fixture, or APP.md.
3. The template is a **new-integration upsert body**, not a Postman collection or
   complete import envelope. For updates, use actual returned integration and
   endpoint IDs. In the current API, full-integration creation generates omitted
   endpoint IDs. When using single-endpoint upsert, preserve existing IDs or supply
   a newly generated UUID according to the connected tool schema; never use fake
   IDs to reference existing resources.
4. Run the available read-only validators; resolve errors. Save with the current
   integration upsert tool or editor, then obtain the saved IDs. Integration
   configuration is live immediately; there is no flow-style publish step.
5. Run only the verification operations the user authorized, below. Hand the
   IDs and data contract to [the app builder](apps.md).

| Endpoint | External method | Input data | Extracted items |
| --- | --- | --- | --- |
| QueryCalendar | GET | `from`, `to`: ISO 8601 bounds with Z or offsets | Graph `$.value` array |
| CreateEvent | POST | `subject`, `description`, `start`, `end`, `transactionId` | Root `$` event |
| UpdateEvent | PATCH | `eventId`, `subject`, `start`, `end` | Root `$` event |

Connection fields are fixed; none of these runtime inputs belongs in a static
parameter. Write endpoints have `enableForAssetSearch: false`. All three inherit
integration authentication. Runtime invokes use `{ "data": { ... } }` against
`POST /api/v4/config/integration/{integrationId}/endpoint/{endpointId}/invoke`.
The external method comes from the saved endpoint, not from that POST wrapper.

## Query and paging

The default route is `/users/<mailbox>/calendarView`. For a named calendar use
`/users/<mailbox>/calendars/<calendar-id>/calendarView`, retaining the query string.
calendarView includes recurring occurrences and exceptions. Use `$.value`, not
`$.values`. Check `response['@odata.nextLink']`: the current HTTP helper does not
follow it automatically. If present, report an incomplete result; complete paging
requires additional fixed-host/target continuation handling. Do not silently treat
one page as the full calendar or accept an arbitrary next URL from app input.
[Calendar view](https://learn.microsoft.com/en-us/graph/api/calendar-list-calendarview?view=graph-rest-1.0).

## Create and update

The example uses UTC: supply `start` and `end` as UTC wall-clock strings such as
`2026-10-12T09:00:00`, paired by the template with `timeZone: "UTC"`. Convert user
input from its explicit local time zone before invoking; require end after start.
The template has no attendees. Adding them sends invitations.

Create one `transactionId` for each intended creation and retain it across retries.
Save the returned opaque event `id` and the target binding with the business record.
If saving that link fails after Microsoft succeeds, reconcile before issuing a new
create. Do not promise exactly-once delivery.
[Create event](https://learn.microsoft.com/en-us/graph/api/user-post-events?view=graph-rest-1.0).

For a named calendar change the create/update route to
`/users/<mailbox>/calendars/<calendar-id>/events[/{$.eventId}]`. Keep the calendar ID
fixed and the event ID dynamic. PATCH preserves properties omitted from its body,
but this saved update template always sends subject/start/end: callers must supply
all three. It deliberately omits body and attendees. An arbitrary partial update
requires a different verified template. IDs are Graph strings, not Rise-X GUIDs.
[Update event](https://learn.microsoft.com/en-us/graph/api/event-update?view=graph-rest-1.0).

## Verify and diagnose

In an identified test mailbox, query a small date range; create one no-attendee
appointment; retain its returned ID; update its time; query again and check Outlook.
Use a second distinct input to check that sample values were not frozen into
parameters. Clean up only the recorded test event if deletion is authorized.

Endpoint tests are real external calls even when no Rise-X work record is changed.
Keep query/create/update outcomes separate. A successful outer HTTP response can
contain `success: false`; inspect the result and external status.

| Observation | Check |
| --- | --- |
| Token exchange fails | Tenant/client ID, expired secret, Graph .default scope, correct integration's secret input |
| Graph 403 | Calendar role, mailbox scope, additive grants, permission propagation; do not automatically widen access |
| Graph 404 | Mailbox/calendar binding and event ID from that same mailbox |
| No mapped items | `$.value` for list; `$` for a single event; inspect a sanitized response |
| 429 or transient error | Use provider retry guidance; no unbounded loop, no fresh transactionId on a create retry |
| Conflict or stale data | Refresh and reconcile; a free/busy query does not reserve a slot |

Report whether the check was static or live. Record API/tool/SDK versions when
known; do not invent a minimum supported version or declare success without access.
