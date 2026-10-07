# Microsoft setup

This recipe targets commercial Microsoft 365 / Exchange Online shared mailboxes.
Personal accounts, Microsoft 365 group calendars, sovereign clouds, and delegated
user connections require different recipes. Check current Microsoft documentation
when configuring a tenant; the references below were reviewed on 2026-10-07.

## Mailbox

Use an existing shared mailbox or have an administrator create one. Its calendar
is created with it. Give staff the appropriate Outlook access and leave direct
mailbox sign-in blocked. Rise-X uses an application credential, not the mailbox
password. Inbox forwarding and email polling are unnecessary for calendar calls.
[Microsoft shared mailbox setup](https://learn.microsoft.com/en-us/microsoft-365/admin/email/create-a-shared-mailbox?view=o365-worldwide).

Use the default calendar initially. For another calendar, retrieve its actual ID
from `GET /users/<mailbox-id-or-upn>/calendars` with the configured connection.
Use the owner's mailbox IDs consistently, not a share recipient's calendar copy.
[List calendars](https://learn.microsoft.com/en-us/graph/api/user-list-calendars?view=graph-rest-1.0).

## Application and permission scope

Create a dedicated Entra application and a client secret. Record tenant ID, client
ID, secret expiry, and the enterprise application's service principal object ID.
The object ID of the app registration is a different value.

For mailbox-scoped access, the Exchange administrator:

1. Registers a pointer to the existing Entra service principal in Exchange using
   `New-ServicePrincipal`.
2. Creates a management scope matching only the intended mailbox and verifies its
   recipient filter.
3. Assigns `Application Calendars.ReadWrite` with `New-ManagementRoleAssignment`
   and that scope. A read-only connection can use `Application Calendars.Read`.
4. Uses `Test-ServicePrincipalAuthorization` for allowed and excluded mailboxes,
   then verifies actual Graph access after permission propagation.

Exchange RBAC and Entra grants are additive. An unrestricted Entra calendar grant
would defeat this scope; remove or separately restrict it. The test cmdlet does
not evaluate separately granted Entra permissions. Follow Microsoft's complete
[application RBAC instructions](https://learn.microsoft.com/en-us/exchange/permissions-exo/application-rbac)
for current commands, administrator roles, and propagation behaviour.

## Rise-X token exchange settings

| Field | Value |
| --- | --- |
| type | `ClientCredentials` |
| tokenUrl | `https://login.microsoftonline.com/<tenant-id>/oauth2/v2.0/token` |
| clientId | The application client ID |
| secret | `{$.graphClientSecret}` referencing a secret parameter |
| grantType | `client_credentials` |
| scope | `https://graph.microsoft.com/.default` |
| scheme | `Bearer` |

The supported recipe uses a client secret, not certificate signing or an
interactive authorization-code flow. No `/me` routes: there is no signed-in user.
Rotate the secret through the existing integration secret input, with an expiry
reminder owned by the operator; never copy ciphertext from another integration.
[Microsoft token flow](https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-client-creds-grant-flow).
