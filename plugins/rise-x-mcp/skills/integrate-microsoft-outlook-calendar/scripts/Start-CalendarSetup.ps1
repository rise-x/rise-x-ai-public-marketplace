#requires -Version 7.4
<#
.SYNOPSIS
Guides a Microsoft 365 administrator through shared calendar connection setup.
.DESCRIPTION
Creates or reuses the dedicated identity, mailbox scope and Rise-X endpoints.
Stores generated credentials in a vault and resumes using the same setup folder.
Uses interactive administrator sign-in. Never sends invitations or deploys an app.
.PARAMETER SetupDirectory
Private local folder containing connection.json, state.json, rise-x-app.json and
setup-status.json. Keep this folder and the credential vault for future reruns.
.PARAMETER InstallPrerequisites
Install missing pinned modules from official PSGallery for the current OS user.
No machine elevation, repository trust change or execution-policy bypass is used.
.PARAMETER SecretVault
Existing approved SecretManagement vault, or the default password-protected local
RiseXCalendarSetup vault. Secrets are never written into the setup folder.
.PARAMETER DeviceLogin
Use Microsoft device-code sign-in when permitted by the customer's tenant policy.
.PARAMETER UpgradeHtmlTemplate
Explicitly upgrade only the recognized Text create template to HTML.
.EXAMPLE
./scripts/Start-CalendarSetup.ps1 -SetupDirectory ~/rise-x-calendar -InstallPrerequisites
.EXAMPLE
./scripts/Start-CalendarSetup.ps1 -WhatIf
#>
[CmdletBinding(SupportsShouldProcess, ConfirmImpact = 'High')]
param(
    [string]$SetupDirectory,
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9 _.-]{0,100}$')][string]$SecretVault = 'RiseXCalendarSetup',
    [securestring]$RiseXToken,
    [switch]$InstallPrerequisites,
    [switch]$DeviceLogin,
    [switch]$UpgradeHtmlTemplate
)
. (Join-Path $PSScriptRoot 'Calendar.Setup.ps1')

if ($WhatIfPreference) {
    Write-Output 'Preview: check/install pinned modules if requested; sign into Microsoft; collect or validate connection settings; verify Rise-X access and secure vault; review changes; create/reuse the dedicated mailbox-scoped identity and six endpoints; export bindings; run read-only checks. No sign-in, prompts, HTTP, module changes, credentials or files in preview.'
    return
}

if ([string]::IsNullOrWhiteSpace($SetupDirectory)) {
    throw 'Supply -SetupDirectory with a private folder outside the installed skill. Reuse that folder on subsequent runs.'
}
$folder = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($SetupDirectory)
$skillRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if ($folder.Equals($skillRoot, [StringComparison]::OrdinalIgnoreCase) -or
    $folder.StartsWith($skillRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'SetupDirectory must be outside the installed skill so plugin updates cannot replace connection state.'
}
$configPath = Join-Path $folder 'connection.json'
$statePath = Join-Path $folder 'state.json'
$manifestPath = Join-Path $folder 'rise-x-app.json'
$statusPath = Join-Path $folder 'setup-status.json'
$lock = $null
$secret = $null
$stage = 'local setup folder'
$recovery = 'Use a writable private local folder. Only run one installer for this connection at a time.'
$report = @{version = 1; installerVersion = '1.0.0'; outcome = 'incomplete'; stage = $stage}
try {
    if (-not $PSCmdlet.ShouldProcess($folder, 'Use this private folder for resumable calendar setup (no plaintext credentials)')) { return }
    [IO.Directory]::CreateDirectory($folder) | Out-Null
    # Keep the lock file; deleting it can let a third process lock a different inode.
    $lock = [IO.File]::Open((Join-Path $folder 'setup.lock'), [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    $stage = 'connection configuration'
    $recovery = 'Validate connection.json against templates/connection.schema.json. Use real identifiers (omit clientId for a new application), an HTTPS API origin and different allowed/excluded mailbox addresses. Keep the original config with its state file.'
    $config = if (Test-Path -LiteralPath $configPath) { Read-CalendarConfig $configPath } else { $null }
    if (-not $config -and (Test-Path -LiteralPath $statePath)) { throw 'Existing state requires its original connection.json.' }
    if ($config) { $null = Read-CalendarState $statePath $config }

    $stage = 'PowerShell prerequisites'
    $recovery = 'Start pwsh -NoProfile. Install the versions in scripts/setup-modules.json through IT or rerun with -InstallPrerequisites. Do not bypass execution policy or certificate checks.'
    Import-CalendarSetupModule -InstallPrerequisites:$InstallPrerequisites -Confirm:$false

    $stage = 'Microsoft Graph sign-in'
    $recovery = 'Sign into the intended commercial tenant as an application administrator with consent for Application.ReadWrite.All. Activate required PIM roles first. For session conflicts use a fresh pwsh -NoProfile window; try -DeviceLogin only if permitted by tenant policy.'
    $tenant = Connect-CalendarSetupTenant -TenantId $(if ($config) { $config.tenantId } else { '' }) -DeviceLogin:$DeviceLogin
    if (-not $config) { $config = Read-CalendarSetupConfig -TenantId $tenant }

    $stage = 'connection review'
    $recovery = 'Review connection.json locally: the API origin, exact ecosystem name and build environment are separate values. Keep the original target when resuming; use a different setup folder for another connection.'
    Write-Information ("Tenant: {0}`nShared mailbox: {1}`nExcluded mailbox: {2}`nRise-X: {3} / {4}`nBuild environment: {5}`nApplication/integration: {6} / {7}" -f
        $config.tenantId, $config.mailbox, $config.excludedMailbox, $config.riseXBaseUrl, $config.ecosystem, $config.environment, $config.applicationName, $config.integrationName) -InformationAction Continue
    if (-not $PSCmdlet.ShouldProcess("$($config.tenantId) / $($config.mailbox) / $($config.riseXBaseUrl) / $($config.ecosystem)",
        'Create/reuse a dedicated application, shared mailbox, mailbox-only calendar role, 90-day credential if needed, and six Rise-X endpoints; no invitations')) { return }
    if (-not (Test-Path -LiteralPath $configPath)) { Save-CalendarState $config $configPath }
    $config = Read-CalendarConfig $configPath
    $state = Read-CalendarState $statePath $config

    $stage = 'Rise-X access preflight'
    $recovery = 'Verify the API origin and exact ecosystem spelling in connection.json. Supply a current owner/orchestrator bearer token from that same Rise-X environment at the secure prompt; do not paste it into a command, ticket or chat.'
    if (-not $RiseXToken) { $RiseXToken = Read-Host 'Rise-X owner/orchestrator bearer token for the displayed environment' -AsSecureString }
    if (-not $RiseXToken.Length) { throw 'A token is required.' }
    $integration = Get-CalendarSetupIntegration -Config $config -State $state -Token $RiseXToken
    if ($integration) {
        # Discover the real existing application ID instead of creating a second app.
        if ($integration.name -ne $config.integrationName -or $integration.baseUri.TrimEnd('/') -ne 'https://graph.microsoft.com/v1.0' -or
            $integration.authentication.tokenUrl -ne "https://login.microsoftonline.com/$tenant/oauth2/v2.0/token" -or
            $integration.authentication.type -ne 'ClientCredentials' -or $integration.maxRetries -ne 0 -or
            ($config.clientId -and $config.clientId -ne $integration.authentication.clientId)) { throw 'The existing integration does not match this dedicated connection.' }
        $config.clientId = $integration.authentication.clientId
        Save-CalendarState $config $configPath
        $config = Read-CalendarConfig $configPath
    }

    $stage = 'Exchange sign-in and role preflight'
    $recovery = 'Sign into the same Microsoft tenant in Exchange. Activate Exchange Organization Management/delegating roles, then reconnect. A wrong-tenant connection must be closed before proceeding.'
    Connect-CalendarSetupExchange -TenantId $tenant -DeviceLogin:$DeviceLogin

    $stage = 'dedicated application discovery'
    $recovery = 'Use a dedicated application. If multiple applications share the configured name, set clientId to the intended application ID in connection.json. Retrieve existing credentials from your password manager; setup does not replace them.'
    $filter = if ($config.clientId) { "appId eq '$($config.clientId)'" } else { "displayName eq '$($config.applicationName)'" }
    $applications = @(Get-MgApplication -Filter $filter -ErrorAction Stop)
    if ($applications.Count -gt 1) { throw 'Multiple matching applications.' }
    $hasExistingPassword = $applications.Count -eq 1 -and @($applications[0].PasswordCredentials | Where-Object { $null -ne $_ }).Count -gt 0

    $stage = 'credential vault preflight'
    $recovery = 'Unlock the selected vault and verify read/write access. Retain its password in your approved password manager. Existing SecretStore settings are user-wide and are never reset by this installer.'
    if ($state.ContainsKey('credentialStore')) {
        if ($PSBoundParameters.ContainsKey('SecretVault') -and $SecretVault -ne $state.credentialStore.vault) { throw 'The selected vault differs from recorded state.' }
        $SecretVault = $state.credentialStore.vault
    }
    Initialize-CalendarSetupVault -Name $SecretVault -Confirm:$false
    $secretName = if ($state.ContainsKey('credentialStore')) { $state.credentialStore.name } else {
        'RiseX-calendar-' + [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($state.binding)))
    }
    $secretInfo = Get-SecretInfo -Name $secretName -Vault $SecretVault -ErrorAction Stop
    if ($secretInfo) {
        $secret = Get-Secret -Name $secretName -Vault $SecretVault -ErrorAction Stop
        if ($secret -isnot [securestring] -or -not $secret.Length) { throw 'The vault entry must contain a nonempty SecureString.' }
    }
    if ($state.ContainsKey('credentialAttempt') -and $state.credentialAttempt.outcome -eq 'unknown' -and -not $state.resources.ContainsKey('credential')) {
        $stage = 'uncertain credential creation'
        $recovery = 'The password-creation response was lost. Reconcile the application password keys in Entra and the credential vault before continuing. Do not remove credentialAttempt or issue another password until the original write is accounted for; see references/setup.md.'
        throw 'A credential write needs reconciliation.'
    }
    if (-not $secret -and ($integration -or $hasExistingPassword -or $state.resources.ContainsKey('credential') -or $state.ContainsKey('credentialAttempt'))) {
        $stage = 'existing credential recovery'
        $recovery = 'Use the existing client secret from your approved password manager. If it is lost, reconcile the recorded Entra credential and rotate it explicitly with your administrator; rerunning setup never issues a replacement.'
        Write-Information $recovery -InformationAction Continue
        $secret = Read-Host 'Existing Microsoft client secret (blank stops setup)' -AsSecureString
        if (-not $secret.Length) { throw 'Existing credential unavailable.' }
        if ($applications.Count -ne 1) { throw 'The application must exist before recovering its credential.' }
        Test-CalendarSetupCredential -TenantId $tenant -ClientId $applications[0].AppId -Secret $secret
        Set-Secret -Name $secretName -Vault $SecretVault -SecureStringSecret $secret -NoClobber -ErrorAction Stop
    }
    if ($secret -and $applications.Count -ne 1) { throw 'A saved credential cannot be associated with a new application. Restore the original application/configuration.' }
    $state.credentialStore = @{vault = $SecretVault; name = $secretName}
    Save-CalendarState $state $statePath

    $stage = 'Microsoft provisioning'
    $recovery = 'Keep this setup folder and rerun the same command. Check that the excluded mailbox exists, the allowed mailbox is shared, and the dedicated application has no separate Entra grants. Newly created service principals and role assignments can require propagation. See references/setup.md for reconciliation; never delete state to force a retry.'
    $connection = & (Join-Path $PSScriptRoot 'Initialize-CalendarConnection.ps1') -ConfigPath $configPath -StatePath $statePath -CreateResources -CreateCredential:($null -eq $secret) -VaultName $SecretVault -VaultEntryName $secretName -Confirm:$false
    if (-not $connection) { throw 'Microsoft setup did not complete.' }
    if (-not $secret) { $secret = Get-Secret -Name $secretName -Vault $SecretVault -ErrorAction Stop }
    if ($secret -isnot [securestring] -or -not $secret.Length) { throw 'Credential is unavailable.' }
    $stage = 'Microsoft credential verification'
    $recovery = 'Verify the existing application secret and expiry in Entra. Correct the recorded vault entry through your approved secret-management procedure, then rerun. No Rise-X credential is overwritten and no new password is issued automatically.'
    Test-CalendarSetupCredential -TenantId $tenant -ClientId $connection.ClientId -Secret $secret
    $connection = $null

    $stage = 'Rise-X endpoint setup'
    $recovery = 'Keep state.json and rerun with a current Rise-X token. Existing connection or endpoint differences need review. Use -UpgradeHtmlTemplate only for an intentional Text-to-HTML migration. Unknown write results are reconciled by saved resource IDs; writes are not automatically retried.'
    $arguments = @{ConfigPath = $configPath; StatePath = $statePath; ManifestPath = $manifestPath; RiseXToken = $RiseXToken; UpgradeHtmlTemplate = $UpgradeHtmlTemplate; Confirm = $false}
    if (-not $integration) { $arguments.ClientSecret = $secret }
    & (Join-Path $PSScriptRoot 'Set-RiseXCalendarIntegration.ps1') @arguments | Out-Null

    $stage = 'read-only acceptance'
    $recovery = 'For allowed-mailbox 403, wait for Microsoft permission propagation and rerun this command (it can take up to two hours). A successful read from the excluded mailbox requires immediate review of additive grants. For 400/401, verify the environment/token/credential. No invitation test is run by setup.'
    Invoke-CalendarSetupReadCheck -Arguments @{ConfigPath = $configPath; StatePath = $statePath; RiseXToken = $RiseXToken; ClientSecret = $secret}
    $state = Read-CalendarState $statePath $config
    $report.outcome = 'read-only-verified'
    $report.stage = 'complete'
    $report.checkedAt = [DateTimeOffset]::UtcNow.ToString('o')
    $report.recipientDelivery = 'not-tested'
    if ($state.resources.ContainsKey('credential') -and $state.resources.credential.ContainsKey('expiresAt')) {
        $report.credentialExpiresAt = ([DateTimeOffset]$state.resources.credential.expiresAt).ToString('o')
        Write-Information "Credential expires $($report.credentialExpiresAt). Schedule renewal with your administrator; setup never rotates it automatically." -InformationAction Continue
    }
    Save-CalendarState $report $statusPath
    Write-Information "Connection verified. Bindings: $manifestPath. Build/deploy the app separately, then explicitly test invitations with controlled recipients." -InformationAction Continue
    [pscustomobject]@{Outcome = $report.outcome; ManifestPath = $manifestPath; StatusPath = $statusPath}
} catch {
    # Never serialize provider exceptions, request bodies, tokens or environment data
    # into the support report. Detailed errors may contain confidential information.
    $report.stage = $stage
    $report.recovery = $recovery
    # Translate only known failure categories, never copy raw provider messages.
    $detail = switch -Regex ($_.Exception.Message) {
        'during excluded mailbox lookup' { 'The excluded mailbox could not be resolved.'; break }
        'during Exchange service-principal registration' { 'The enterprise service principal could not be registered/read back in Exchange; check tenant visibility and propagation.'; break }
        'during Exchange mailbox scope and role assignment' { 'The mailbox scope or application role could not be verified; review existing assignments and administrator roles.'; break }
        'during allowed mailbox (scope verification|authorization query)' { 'Exchange did not confirm access to the allowed mailbox.'; break }
        'during excluded mailbox (scope verification|authorization query)' { 'Exchange did not confirm that the excluded mailbox is out of scope.'; break }
        'during credential vault storage' { 'A credential was issued but vault storage was not confirmed. Retain its recorded key ID; do not issue another.'; break }
        'during client credential creation' { 'Credential creation was refused or its result is uncertain. Inspect credentialAttempt and the recorded key before any retry.'; break }
        'Excluded mailbox is accessible' { 'The excluded mailbox was accessible: review additive permissions before using this integration.'; break }
        '\(status (400|401|403|404|409|429|500|502|503|504)\)' { "HTTP $($Matches[1]) was returned. Follow the recovery guidance for this stage."; break }
        default { 'This stage did not complete; no success is recorded.' }
    }
    $report.detail = $detail
    $report.checkedAt = [DateTimeOffset]::UtcNow.ToString('o')
    if ($lock) {
        try { Save-CalendarState $report $statusPath } catch { Write-Warning 'Could not write setup-status.json. Keep the existing state file.' }
    }
    $exception = [InvalidOperationException]::new("Calendar setup stopped at ${stage}. $detail $recovery")
    $PSCmdlet.ThrowTerminatingError([Management.Automation.ErrorRecord]::new($exception, 'CalendarSetupFailed', [Management.Automation.ErrorCategory]::InvalidOperation, $null))
} finally {
    $secret = $null; $RiseXToken = $null
    if ($lock) { $lock.Dispose() }
}
