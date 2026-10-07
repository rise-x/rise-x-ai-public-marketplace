# Use the calendar from a Rise-X app

Use the installed `rise-x-apps` skill for app design/build work and the installed
SDK README for the current contract. The provider skill owns Microsoft setup;
the app owns user experience and its environment-specific dependency bindings.
This draft's examples cover the three endpoints in the bundled template.

## Manifest

Merge this dependency fragment into `rise-x-app.json`. Replace the synthetic IDs
with saved endpoint IDs; all three may belong to one integration. Declare only
environments whose bindings exist. Never put credentials in the manifest.

```json
{
  "environments": ["test"],
  "dependencies": {
    "calendarQuery": {
      "kind": "integration",
      "ids": { "test": { "integrationId": "11111111-1111-1111-1111-111111111111", "endpointId": "22222222-2222-2222-2222-222222222222" } }
    },
    "calendarCreate": {
      "kind": "integration",
      "ids": { "test": { "integrationId": "11111111-1111-1111-1111-111111111111", "endpointId": "33333333-3333-3333-3333-333333333333" } }
    },
    "calendarUpdate": {
      "kind": "integration",
      "ids": { "test": { "integrationId": "11111111-1111-1111-1111-111111111111", "endpointId": "44444444-4444-4444-4444-444444444444" } }
    }
  }
}
```

## Bound calls

These helpers use public SDK entry points. The generic describes the manifest;
it does not validate missing runtime bindings. Validate the manifest with the
installed SDK CLI before building. Transport failures throw; external failures
can also arrive as a resolved result with `success: false`.

```ts
import {
  getAppDependencies,
  type BoundIntegrationDependency,
} from '@rise-x/apps-sdk/dependencies';

type CalendarDependencies = {
  calendarQuery: BoundIntegrationDependency;
  calendarCreate: BoundIntegrationDependency;
  calendarUpdate: BoundIntegrationDependency;
};

export async function queryCalendar(from: string, to: string) {
  const deps = getAppDependencies<CalendarDependencies>();
  const result = await deps.calendarQuery.integration.invoke({ data: { from, to } });
  if (!result.success) throw new Error(`Calendar query failed (${result.statusCode})`);
  if (result.response?.['@odata.nextLink']) {
    throw new Error('Incomplete calendar results; use a smaller range or configured paging');
  }
  return result.items;
}

export async function createAppointment(input: {
  subject: string; description: string; start: string; end: string; transactionId: string;
}) {
  const deps = getAppDependencies<CalendarDependencies>();
  const result = await deps.calendarCreate.integration.invoke({ data: input });
  if (!result.success) throw new Error(`Calendar create failed (${result.statusCode})`);
  const event = result.items[0];
  if (typeof event?.id !== 'string' || !event.id) {
    throw new Error('Create outcome needs reconciliation: no event ID returned');
  }
  return event;
}

export async function updateAppointment(input: {
  eventId: string; subject: string; start: string; end: string;
}) {
  if (!input.eventId) throw new Error('An existing event ID is required');
  const deps = getAppDependencies<CalendarDependencies>();
  const result = await deps.calendarUpdate.integration.invoke({ data: input });
  if (!result.success) throw new Error(`Calendar update failed (${result.statusCode})`);
  return result.items;
}
```

Before create, persist one transactionId for that intended event. Persist its
returned opaque ID and mailbox/environment binding afterwards. A timeout or
failed business-record save needs reconciliation using the original identity;
do not silently generate a new transactionId and create again. The UI catches
exceptions and displays failed or uncertain outcomes, refreshing after confirmed
writes. Do not log raw provider responses or invite content to explain an error.

Query bounds contain offsets or Z; the saved create/update template pairs UTC
wall-clock strings with `timeZone: "UTC"`. Convert from the user's explicit time
zone, require end after start, and supply all fields required by the saved
update template. Opaque event IDs need verified URI encoding when interpolated.

Integration invocation is network-only. Do not queue writes offline or treat an
overlap query as an atomic booking reservation. Credentials and Graph token
exchange stay in the Rise-X integration; the app does not call Graph directly.
For the expanded invite demo and release criteria, see [demo validation](demo-validation.md).
