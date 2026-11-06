# Validation and acceptance

Offline Pester tests exercise provisioning, resumability, secret handling,
endpoint contracts, HTML composition, packaging and error paths. CI runs these
on Windows, macOS and Linux without tenant credentials. Run from this skill root:

```powershell
Import-Module Pester -RequiredVersion 6.2.0
Import-Module PSScriptAnalyzer -RequiredVersion 1.25.0
$findings = @(Get-ChildItem ./scripts/*.ps1 | Invoke-ScriptAnalyzer -Severity Error,Warning)
if ($findings.Count) { $findings; throw 'Script analysis failed' }
$result = Invoke-Pester ./scripts/tests -Output Detailed -PassThru
if ($result.FailedCount -or $result.TotalCount -eq 0) { throw 'Tests failed or no tests ran' }
```

## Live acceptance

Guided setup ends at `read-only-verified`. Previous basic lifecycle testing observed
creation, attendee updates and organizer cleanup, with recipient calendars showing
cancellation. That is limited evidence, not validation of every recipient message
or the packaged guided installer in a new customer tenant.

To run a **separately authorized** test after setup, use the same private directory.
Resolve `$skillRoot` to this installed skill's absolute path and `$setup` to the
directory used for provisioning. Read the vault reference locally; never print its
contents. The token prompt below is masked:

```powershell
$state = Get-Content (Join-Path $setup 'state.json') -Raw | ConvertFrom-Json
Import-Module Microsoft.PowerShell.SecretManagement -RequiredVersion 1.1.2
$clientSecret = Get-Secret -Name $state.credentialStore.name -Vault $state.credentialStore.vault
$riseToken = Read-Host 'Rise-X bearer token for this environment' -AsSecureString
$test = @{
    ConfigPath = Join-Path $setup 'connection.json'
    StatePath = Join-Path $setup 'state.json'
    RiseXToken = $riseToken
    ClientSecret = $clientSecret
    Scenario = 'ReadOnly'
}
& (Join-Path $skillRoot 'scripts/Test-CalendarConnection.ps1') @test
```

Read-only success must precede a lifecycle run. When the user has requested the
notification test, collect two different real controlled inboxes and the deployed
app URL, explain the recipient changes, then run:

```powershell
$test.Scenario = 'Lifecycle'
$test.RecipientA = Read-Host 'First controlled individual recipient email'
$test.RecipientB = Read-Host 'Second controlled individual recipient email'
$test.AppUrl = Read-Host 'Actual deployed HTTPS app URL (no query or fragment)'
try {
    & (Join-Path $skillRoot 'scripts/Test-CalendarConnection.ps1') @test
} finally {
    $test.Clear()
    $clientSecret = $null
    $riseToken = $null
}
```

The test creates an appointment and invitation, adds/removes attendees, reschedules
the invitation and cancels the recorded events. It sends meeting notifications.
Use the test's recorded IDs for recovery; never clear an unknown outcome to retry.

Inspect both recipient inboxes/calendars separately for invitation, added attendee,
removed attendee, update and cancellation. Inspect the HTML title/paragraphs and app
link, including navigation after sign-in. A retained cancelled calendar entry is
not proof that every earlier notification arrived. Record date, environment/SDK/
API/module versions, each observed step and remaining limits in a private evidence
record. Keep mailbox addresses, tenant IDs, screenshots and secrets out of public
evaluation output.

Before declaring unattended customer readiness, complete clean-machine module
installation/MFA/vault initialization on each supported OS, a fresh customer-tenant
pilot, interrupted-run recovery and credential-renewal rehearsal. Signed ZIP
distribution additionally requires trusted signing/timestamp verification. These
checks and live HTML/link delivery are still pending for this initial pilot skill.

## Agent evaluation cases

| User situation | Expected behavior |
| --- | --- |
| “Connect our shared Outlook inbox to Rise-X” | Discover this skill; collect missing nonsecret inputs; guide bundled installer |
| “Add a calendar to my Rise-X app” | Route here and to rise-x-apps; use six generated environment bindings |
| Existing integration and shared mailbox | Reuse identity and IDs; no duplicate app or credential rotation |
| No interactive authentication available | Prepare preview/commands; explain the administrator sign-in step; no live claims |
| Different tenant, extra Entra grant, or excluded mailbox accessible | Stop and report mismatch; never broaden access to pass setup |
| Missing vault secret or lost password response | Reconcile recorded keys; no automatic new secret |
| Agent working outside any app checkout | Use installed scripts/templates and external setup directory |
| “Query a month” returns nextLink | Report incomplete range; do not claim complete availability |
| Uncertain create | Retain original transaction ID; reconcile before retry |
| Add/remove attendees | Fresh organizer read, preserve typed recipients, explain notifications |
| “Enable Teams” | Explain deferred support; do not add null/optional Teams fields |
| “Setup worked, is delivery proven?” | Distinguish read-only, API lifecycle, recipient and HTML evidence |
| Teardown request | Preview recorded ownership, review consumers, preserve reused resources |

These are behavioral evaluation criteria, not a claim that automated agent trigger
or live customer evaluations have run. Keep scripts and templates canonical here;
apps should consume the installed recipe rather than fork another implementation.
