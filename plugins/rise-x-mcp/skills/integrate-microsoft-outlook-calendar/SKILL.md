---
name: integrate-microsoft-outlook-calendar
description: Configure Rise-X integration endpoints to query, create, and update events in a Microsoft 365 shared business calendar through Microsoft Graph, and connect those endpoints to a Rise-X app. Use for shared Outlook calendar integrations in Rise-X; personal delegated sign-in, email ingestion, and calendar-only UI are outside this recipe.
---

# Microsoft Outlook Calendar integration

Connect a shared Exchange Online calendar to Rise-X through an application
identity. The Rise-X server holds Microsoft credentials; apps invoke configured
endpoints with business data.

## Draft status

This is an initial recipe for app-local evaluation, not a live-validated release.
The bundled template covers query/create/update without attendees. Invite attendee
changes, event retrieval/cancellation, and PowerShell automation remain acceptance
work for the test app. See [demo validation](references/demo-validation.md) before
claiming that wider lifecycle is supported. Do not publish this draft until the
app's recorded acceptance supports promotion.

## Choose the work requested

- **Mailbox/application setup:** read [Microsoft setup](references/microsoft-setup.md).
- **Configure or troubleshoot endpoints:** read [Rise-X endpoints](references/endpoints.md).
  The [configuration template](assets/integration.template.json) supplies the three operations.
- **Use the calendar from an app:** read [App consumption](references/apps.md).
- **Validate the connection:** follow the read/create/update check in the endpoint reference.

Before calling any Rise-X MCP tool, load the sibling
[rise-x-mcp skill](../rise-x-mcp/SKILL.md). For configuration writes and endpoint
tests, also load its [integration-authoring protocol](../rise-x-mcp/references/integration-authoring.md).
Use the current tool schema; a tool name in a reference does not prove that the
connected deployment exposes it. Without that connection, prepare the configuration
for the Rise-X editor and identify the missing setup step; do not claim it was applied.

## Inputs and boundaries

Discover existing integrations before creating another. Resolve the deployment,
ecosystem, shared mailbox, default or named calendar, tenant ID, application client
ID, secret input method, and operations required. Ask only for missing choices;
keep supplied secrets out of summaries, files, and app code.

Keep the mailbox/calendar fixed in saved configuration. Leave event IDs, date
ranges, and event content as invocation data. Query-only work does not need write
endpoints or write permission. This recipe's read/create/update configuration uses
mailbox-scoped application calendar access.

An endpoint test makes a real Microsoft request. Respect the user's existing
authorization, identify the target and effect, and obtain missing authorization
before external writes. Creating an event with attendees sends invitations. A
request to prepare configuration does not authorize those calls.

The existing runtime route is available to members of the integration's ecosystem
and qualifying subscription holders. App visibility does not restrict it. Use this
recipe for a shared operational calendar suitable for that audience; narrower
authorization needs a separate platform design.

## Completion evidence

Return the integration and endpoint IDs, operation aliases, required input fields,
response shapes, and what was actually verified. Separate configuration validation
from a live Microsoft check. Record unresolved paging or permission needs. Never
return credential values, raw tokens, or a claim of deployment based on a template.

For the family naming convention and adding another provider, see the
[provider catalog](../rise-x-mcp/references/provider-integrations.md).
