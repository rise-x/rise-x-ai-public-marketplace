---
name: integrate-microsoft-outlook-calendar
description: Help a user set up a Microsoft 365 shared mailbox and connect its Outlook calendar to Rise-X using bundled PowerShell scripts. Use for calendar integration onboarding, Entra application and Exchange mailbox-scoped access, secure credentials, setup recovery, or Rise-X calendar query, HTML invitations, attendee updates and cancellation. Use alongside rise-x-apps when an app needs these endpoints. Excludes delegated personal calendars, Teams meeting creation and general Azure provisioning.
---

# Connect a shared Outlook calendar to Rise-X

This skill includes its own scripts and endpoint templates. No calendar app or
private repository checkout is required. It supports Microsoft 365 commercial
tenants and a dedicated application with Exchange mailbox-scoped calendar access.
The current release is a **pilot recipe**: read-only connection and basic event
lifecycle have live evidence; clean-machine guided installation and live HTML
recipient acceptance remain pending. Never substitute offline tests for customer
acceptance. See [validation and evaluation](references/validation.md).

## Agent workflow

1. Establish the intended tenant, shared mailbox (existing or new), another real
   mailbox to exclude, default or named calendar, Rise-X API origin, exact ecosystem
   name and build environment. Reuse known inputs. Ask only for missing nonsecret
   values. A named calendar requires its actual opaque ID. Confirm the administrator
   has the privileges in [setup](references/setup.md); do not request a Global
   Administrator account as the default solution.
2. Locate this installed skill directory. Read [setup](references/setup.md) before
   provisioning. Use the bundled scripts rather than recreating provisioning
   commands. Keep a separate private setup directory outside the plugin cache and
   source control; preserve it and its vault for reruns. `-WhatIf` is offline and
   does not prompt, authenticate or write files.
3. Prepare the exact command with the resolved script path and selected setup
   directory. Help the user run it in an interactive `pwsh -NoProfile` session for
   Microsoft sign-in/MFA and secure prompts. Use `-InstallPrerequisites` only when
   installing pinned modules is wanted. Do not collect tokens, passwords or client
   secrets in chat, command arguments, transcripts, source files or app manifests.
   A headless agent cannot complete the user's interactive authentication.
4. Run or guide [Start-CalendarSetup.ps1](scripts/Start-CalendarSetup.ps1) within the
   user's authorized setup scope. It reviews the connection, checks Rise-X access
   and the vault, creates/reuses the mailbox and dedicated identity, assigns
   `Application Calendars.ReadWrite` to the mailbox scope, configures six endpoints
   and exports bindings. Its 90-day credential is stored in a protected vault.
   Existing secrets are never rotated implicitly. Do not suppress the review by
   default. There are no invitations or app deployments in guided setup.
5. On failure, inspect only the sanitized `setup-status.json` and follow
   [recovery](references/setup.md#recovery). Retain configuration/state and rerun
   with the same directory. Unknown password or calendar writes must be reconciled
   before another write. Never delete state, broaden grants or bypass a contract
   mismatch to make a rerun pass. Exchange simulation alone is insufficient:
   require an allowed-calendar read, excluded-mailbox 403 and Rise-X query.
6. For app work read [endpoint and app contracts](references/contracts.md) and load
   `rise-x-apps`. Merge the six generated environment bindings into the actual app
   manifest, preserving unrelated dependencies. Build for that environment and
   deploy only within the user's request. Secrets stay in Rise-X, not browser code.
7. For an explicitly requested invitation test, follow
   [live acceptance](references/validation.md#live-acceptance), with two real
   controlled individual recipients and the actual deployed app URL. Describe who
   will receive invitations/updates/cancellations before executing. Query/read before
   changing events, preserve recipient types/metadata, and retain the original
   transaction identity after an uncertain create. Attendees do not gain mailbox
   access. Do not mutate distribution lists or recurring events with this recipe.
8. Report separately: setup verified, organizer API lifecycle verified, recipient
   delivery observed, and HTML/link acceptance observed. Include credential expiry
   and the next action without publishing tenant identifiers. Cleanup is explicit:
   [Remove-CalendarDemoResources.ps1](scripts/Remove-CalendarDemoResources.ps1)
   uses recorded ownership; review dependencies before any teardown.

All new meetings use escaped/composed HTML. Teams creation and separate body
editing are deferred. Existing Text create endpoints require the explicit
`-UpgradeHtmlTemplate` migration after callers are ready; unfamiliar changes stop
setup. Query pagination is detected but not followed, so an incomplete result is
never a complete availability answer.

Before any Rise-X MCP call load `rise-x-mcp`; for integration mutations also read
its `references/integration-authoring.md`. The REST scripts use secure operator
authentication and do not require MCP. Check deployed contracts rather than
inventing unavailable tools. Existing invocation policy includes ecosystem members
and qualifying subscribers; app visibility does not narrow endpoint access. Select
a shared operational calendar appropriate for that audience.
