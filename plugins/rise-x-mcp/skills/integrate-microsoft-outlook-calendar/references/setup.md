# Guided shared-calendar setup

This installer configures Microsoft 365 and the six Rise-X calendar endpoints.
It does not send invitations, deploy the app, enable Teams meetings, or change
people's mailbox membership. Use a Microsoft 365 **commercial** tenant and a
dedicated shared calendar. Government and China clouds need a different recipe.

## Before starting

Use a trusted administrator workstation with PowerShell **7.4 or newer**. Open
`pwsh -NoProfile`; do not use Windows PowerShell 5.1. The administrator needs Entra
application-management privileges and consent for the delegated
`Application.ReadWrite.All` setup scope, plus the Entra Exchange Administrator role and Exchange Organization Management /
the roles needed to delegate application permissions. Activate any PIM roles
before starting. The runtime application receives only mailbox-scoped Exchange
`Application Calendars.ReadWrite`, with no tenant-wide Entra calendar grant.

Have these values available:

- Shared mailbox email, either existing or to be created. Keep shared-mailbox
  interactive sign-in blocked. Licensing/retention requirements remain your
  Microsoft 365 administrator's responsibility.
- Another **existing** mailbox in the same tenant to prove that access is denied.
- Rise-X API origin, exact ecosystem name, and app build environment from your
  Rise-X environment details. The API origin and ecosystem are checked before
  Microsoft resources are provisioned.
- A current Rise-X owner/orchestrator bearer token from that environment. Enter
  it only at the masked prompt. This version still requires that token; it does
  not implement a new Rise-X browser login flow.

No application, service-principal or endpoint IDs need to be copied. The installer
discovers IDs, generates stable names and checks both Microsoft sign-ins target
the same tenant. It asks for Microsoft sign-in/MFA and shows a connection summary
before provisioning. A custom/named calendar can be selected using `calendarId`
in an existing configuration; the guided questionnaire uses the default calendar.

## Run one command

Resolve this installed skill directory, open a fresh `pwsh -NoProfile`, and invoke its bundled entry point. Use an absolute private setup directory outside the installed skill and outside source control. For example, after changing to the skill root:

```powershell
./scripts/Start-CalendarSetup.ps1 -SetupDirectory ~/rise-x-calendar -InstallPrerequisites
```

The installer offers installation of the exact module versions in
`scripts/setup-modules.json` into your user profile. It never changes execution
policy, disables TLS validation or marks a repository trusted. If your organization
blocks PSGallery, have IT install those versions from its approved mirror and omit
`-InstallPrerequisites`. Use `-DeviceLogin` only where tenant policy permits it.

For an entirely offline preview, including on a machine without those modules:

```powershell
./scripts/Start-CalendarSetup.ps1 -WhatIf
```

The explicitly selected setup directory contains configuration, resource IDs, ownership,
the generated `rise-x-app.json`, and `setup-status.json`. Keep this folder private
and backed up. It contains tenant metadata but **no client secrets or tokens**.
Use a different absolute `-SetupDirectory` for each separate connection. There is no default setup directory; plugin installation paths must stay immutable. Do not use a symlink into the plugin cache. Never reuse
one directory for different tenants, ecosystems or calendars. Do not run the
individual maintenance scripts concurrently with the installer.

## Credential storage and resuming

By default the installer registers `RiseXCalendarSetup` using Microsoft's
SecretManagement/SecretStore modules. On first use, choose a vault password and
keep it in your approved password manager. SecretStore data is encrypted on disk;
the vault is local to this OS user and machine. SecretStore registrations share
that user's backing store/settings: the installer does not reset them, remove
password protection or change the default vault. An existing approved
SecretManagement vault can be selected with `-SecretVault YourVault`.

The vault is tested before cloud provisioning. Each new client credential lasts
90 days and is saved to the vault before Rise-X registration starts. The bearer
token is held only in the current run and is never saved. The scripts do not start
a transcript or print HTTP bodies; don't run a general-purpose HTTP/debug trace
while provisioning credentials.

Rerun the **same command with the same setup directory** after a failure. It
rediscovers resources, checks contracts and retrieves the stored client secret.
There is no automatic credential rotation or repeated password-creation request.
Onboarding an existing integration may ask once for its current client secret,
which is then saved in the selected vault. It does not replace the secret in Rise-X.
Imported credentials are checked with Microsoft before they are saved. Close the
dedicated setup PowerShell window when finished; Graph sign-in uses process-local
authentication. Existing administrator sessions are not silently replaced or
disconnected by the installer.

If an existing create endpoint still uses Text, first confirm all callers now
compose escaped HTML, then explicitly add `-UpgradeHtmlTemplate` when rerunning.
Unrecognized endpoint differences are rejected rather than overwritten.

## What success means

`Outcome: read-only-verified` means Microsoft allowed-calendar access succeeded,
the excluded mailbox returned 403, and Rise-X `calendarQuery` succeeded. The
generated manifest contains all six actual bindings. Build and deploy the app
with those bindings separately. Invitation, HTML rendering, recipient changes
and cancellation delivery still require the explicitly selected lifecycle test
with controlled recipients; setup never runs it automatically.

Schedule credential renewal before `credentialExpiresAt` in `setup-status.json`.
Credentials adopted from an external password manager may not have a known expiry
in local state: check Entra directly. Renewal is an explicit administrator
procedure: create the replacement, securely update the existing Rise-X secret,
verify reads, then revoke the old credential. The installer does not implement
rotation or grant itself ongoing administrator permissions.

## Recovery

The sanitized `setup-status.json` gives the failed stage and next action. Share
that file with support if needed; do not send the entire setup folder, tokens,
vault, raw exceptions or HTTP traces. A failed run throws `CalendarSetupFailed`
(a nonzero exit when invoked through `pwsh -File`).

| Stage / symptom | Action |
| --- | --- |
| Prerequisites | Start a fresh `pwsh -NoProfile`; use the pinned versions. Do not hot-swap already loaded provider assemblies. |
| Wrong tenant / missing admin command | Sign into the intended organization, activate admin roles and reconnect in a fresh window. No session is silently replaced. |
| Rise-X 400 / 401 / 403 | Check API origin, ecosystem spelling, token environment, expiry and owner/orchestrator access. |
| Excluded mailbox lookup | Supply a real, different mailbox in this tenant. A nonexistent mailbox is not a valid denial test. |
| Scope or existing grant mismatch | Review the dedicated identity's Entra grants and Exchange assignments. Setup neither broadens nor removes them. |
| New principal not visible / allowed mailbox 403 | Keep state and rerun after propagation. Runtime access can take up to two hours. Setup retries only allowed-mailbox 403 reads, at most three times; it never loops over provisioning writes. |
| Excluded mailbox is accessible | Stop and review additive permissions. Exchange scope simulation alone does not account for Entra grants. |
| Vault unavailable | Unlock the same vault/user profile. Do not reset the store or delete state to force credential creation. |
| Credential issued but vault storage failed | The key ID is in `state.json`. Check whether that exact secret reached the vault. Recover it from the approved store or explicitly revoke/replace that key; don't create another blindly. |
| Password response lost (`credentialAttempt.outcome=unknown`, no recorded key) | Compare Entra password keys and creation times with the recorded attempt. An administrator must account for the original request, revoke any orphaned key, and update recovery state only after confirming the outcome. Setup stops before another password request. |
| Rise-X save response lost | Keep the recorded integration/endpoint IDs. Rerun to read the actual resources; the setup scripts reconcile saved identities. |

No automatic rollback deletes resources after a partial failure. The existing
`Remove-CalendarDemoResources.ps1` previews and removes only recorded resources
created by this recipe when its explicit removal flags are selected; reused
resources remain. Review other apps' dependencies before teardown. Vault cleanup
is a separate explicit operation after remote credentials are revoked. Never
delete an entire shared SecretStore as part of calendar cleanup.

## Packaging and maintenance (Rise-X maintainers)

The marketplace distributes this source skill; it does not assert an Authenticode signature. The optional ZIP builder below is a separate distribution path.

`New-CalendarSetupPackage.ps1` uses an explicit file allowlist, includes per-file
hashes and writes a ZIP SHA-256 sidecar. It cannot include tenant configuration,
vault data, node_modules or build artifacts. Unsigned internal test packages use:

```powershell
./scripts/New-CalendarSetupPackage.ps1 -OutputPath .local/calendar-setup-internal.zip -Unsigned
```

Customer releases must be built on Windows with the organization's trusted
code-signing certificate supplied as `-SigningCertificate` and its approved
`-TimestampServer`. All shipped PowerShell files are signed and verified before
the ZIP is created. Publish the ZIP hash through the authenticated release
channel. Before extraction, compare `Get-FileHash ./calendar-setup.zip -Algorithm
SHA256` with that published value. On Windows verify the extracted entry point
using `Get-AuthenticodeSignature`. A checksum from the same untrusted download
does not establish publisher identity. Never recommend `iex` downloads or an
execution-policy bypass.

Run Pester and PSScriptAnalyzer after changes; the dedicated CI workflow runs
offline tests on Windows, macOS and Linux. Review pinned module updates explicitly,
then repeat fresh-tenant and interrupted-run acceptance before releasing.
SecretManagement is feature-complete with security fixes supported; the vault
adapter keeps a future replacement independent of calendar provisioning.

**Release acceptance is still required:** trusted signing, clean-machine module
installation/MFA/vault initialization on each supported OS, a customer-tenant
pilot, interrupted-run recovery and credential-renewal rehearsal. Offline test
results do not claim these live checks passed.

References: [Exchange application RBAC](https://learn.microsoft.com/en-us/exchange/permissions-exo/application-rbac),
[Graph session authentication](https://learn.microsoft.com/en-us/powershell/microsoftgraph/authentication-commands),
[Exchange connection requirements](https://learn.microsoft.com/en-us/powershell/exchange/connect-to-exchange-online-powershell),
[SecretStore](https://learn.microsoft.com/en-us/powershell/utility-modules/secretmanagement/get-started/using-secretstore),
[SecretManagement maintenance status](https://github.com/PowerShell/SecretManagement).
