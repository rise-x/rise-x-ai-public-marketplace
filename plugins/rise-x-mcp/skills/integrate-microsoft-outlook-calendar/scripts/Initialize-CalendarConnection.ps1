#requires -Version 7.0
[CmdletBinding(SupportsShouldProcess, ConfirmImpact='High')]
param([Parameter(Mandatory)][string]$ConfigPath,[Parameter(Mandatory)][string]$StatePath,[switch]$CreateResources,[switch]$CreateCredential,[string]$VaultName,[string]$VaultEntryName)
. (Join-Path $PSScriptRoot 'Calendar.Common.ps1')
$config=Read-CalendarConfig $ConfigPath; $state=Read-CalendarState $StatePath $config
if ($WhatIfPreference) { Write-Output 'Preview: discover/reuse the shared mailbox and dedicated application; create only explicitly requested missing resources; assign mailbox-scoped calendar access. No credentials or files will be written.'; return }
if ([bool]$VaultName -ne [bool]$VaultEntryName) { throw 'Supply VaultName and VaultEntryName together.' }
if ($VaultName) {
    Import-Module Microsoft.PowerShell.SecretManagement -ErrorAction Stop
    if (-not (Test-SecretVault -Name $VaultName -ErrorAction Stop)) { throw 'Unlock and verify the credential vault before provisioning.' }
}
Import-Module Microsoft.Graph.Applications -ErrorAction Stop
Import-Module ExchangeOnlineManagement -ErrorAction Stop
$context=Get-MgContext
if (-not $context -or $context.TenantId -ne $config.tenantId) { throw 'Connect-MgGraph to the configured tenant before running setup.' }
$exchange=@(Get-ConnectionInformation | Where-Object { $_.TenantID -eq $config.tenantId -and $_.State -eq 'Connected' })
if (-not $exchange.Count) { throw 'Connect-ExchangeOnline to the configured tenant before running setup.' }
function Register-Resource([string]$Kind,[string]$Id,[bool]$Created) {
    if (-not $state.resources.ContainsKey($Kind)) { $state.resources[$Kind]=@{id=$Id;created=$Created} }
    elseif ($state.resources[$Kind].id -ne $Id) { throw 'Discovered resource differs from the saved state. Resolve before continuing.' }
    Save-CalendarState $state $StatePath
}
$stage='excluded mailbox lookup (excludedMailbox must identify a real, different mailbox in this tenant)'
try {
    $excludedMailbox=Get-EXOMailbox -Identity $config.excludedMailbox -ErrorAction Stop
    if (-not $excludedMailbox) { throw 'Excluded mailbox could not be resolved.' }
    $stage='resource discovery'
    $mailbox=Get-EXOMailbox -Identity $config.mailbox -ErrorAction SilentlyContinue
    if (-not $mailbox) {
        if (-not $CreateResources) { throw 'Shared mailbox missing. Supply an existing mailbox or use -CreateResources.' }
        if (-not $PSCmdlet.ShouldProcess($config.mailbox,'Create shared mailbox')) { return }
        $mailbox=New-Mailbox -Shared -Name $config.applicationName -PrimarySmtpAddress $config.mailbox -ErrorAction Stop
        Register-Resource 'mailbox' $config.mailbox $true
    } else {
        if ($mailbox.RecipientTypeDetails -ne 'SharedMailbox') { throw 'This recipe requires a shared mailbox.' }
        Register-Resource 'mailbox' $config.mailbox $false
    }
    if ($config.clientId) { $apps=@(Get-MgApplication -Filter "appId eq '$($config.clientId)'") }
    else { $apps=@(Get-MgApplication -Filter "displayName eq '$($config.applicationName)'") }
    if ($apps.Count -gt 1) { throw 'Multiple matching applications; supply the exact clientId.' }
    if (-not $apps.Count) {
        if (-not $CreateResources -or $config.clientId) { throw 'Application missing. Supply an existing client ID or explicitly create without clientId.' }
        if (-not $PSCmdlet.ShouldProcess($config.applicationName,'Create dedicated Entra application')) { return }
        $app=New-MgApplication -DisplayName $config.applicationName -SignInAudience AzureADMyOrg
        Register-Resource 'application' $app.Id $true
    } else { $app=$apps[0]; Register-Resource 'application' $app.Id $false }
    $state.clientId=$app.AppId;Save-CalendarState $state $StatePath
    $principals=@(Get-MgServicePrincipal -Filter "appId eq '$($app.AppId)'")
    if ($principals.Count -gt 1) { throw 'Ambiguous service principal.' }
    if (-not $principals.Count) {
        if (-not $CreateResources) { throw 'Service principal missing. Use -CreateResources to create it.' }
        if (-not $PSCmdlet.ShouldProcess($app.AppId,'Create enterprise service principal')) { return }
        $sp=New-MgServicePrincipal -AppId $app.AppId;Register-Resource 'servicePrincipal' $sp.Id $true
    } else { $sp=$principals[0];Register-Resource 'servicePrincipal' $sp.Id $false }
    # A dedicated RBAC-only app needs no tenant-wide Entra application grants.
    if (@(Get-MgServicePrincipalAppRoleAssignment -ServicePrincipalId $sp.Id -All).Count) { throw 'Dedicated application already has Entra application grants. Review additive access manually; setup will not broaden or remove grants.' }
    $stage='Exchange service-principal registration (verify the Entra enterprise application exists in the connected tenant; if newly created, allow propagation and rerun with the same state file)'
    $exo=Get-ServicePrincipal -Identity $sp.Id -ErrorAction SilentlyContinue
    if (-not $exo) {
        if (-not $PSCmdlet.ShouldProcess($sp.Id,'Register service principal in Exchange')) { return }
        New-ServicePrincipal -AppId $app.AppId -ObjectId $sp.Id -DisplayName $config.applicationName -ErrorAction Stop | Out-Null
        $exo=Get-ServicePrincipal -Identity $sp.Id -ErrorAction Stop
        if (-not $exo) { throw 'Exchange service-principal registration could not be verified.' }
        Register-Resource 'exchangePrincipal' $sp.Id $true
    } else { Register-Resource 'exchangePrincipal' $sp.Id $false }
    $stage='Exchange mailbox scope and role assignment'
    $filter="PrimarySmtpAddress -eq '$($config.mailbox)'"
    $scope=Get-ManagementScope -Identity $config.scopeName -ErrorAction SilentlyContinue
    if ($scope -and $scope.RecipientFilter.Trim('(',')') -ne $filter) { throw 'Existing scope differs from the exact mailbox filter. Resolve manually; setup will not widen it.' }
    if (-not $scope) {
        if (-not $PSCmdlet.ShouldProcess($config.scopeName,'Create mailbox-only resource scope')) { return }
        New-ManagementScope -Name $config.scopeName -RecipientRestrictionFilter $filter -ErrorAction Stop | Out-Null
        Register-Resource 'scope' $config.scopeName $true
    } else { Register-Resource 'scope' $config.scopeName $false }
    $assignments=@(Get-ManagementRoleAssignment -RoleAssignee $sp.Id -ErrorAction Stop)
    foreach ($assignment in $assignments) {
        if ($assignment.Name -ne $config.assignmentName -or [string]$assignment.Role -ne 'Application Calendars.ReadWrite' -or [string]$assignment.CustomResourceScope -ne $config.scopeName) { throw 'Existing Exchange assignment differs from this dedicated mailbox scope. Review manually.' }
    }
    if (-not $assignments.Count) {
        if (-not $PSCmdlet.ShouldProcess($config.assignmentName,'Grant Application Calendars.ReadWrite for the mailbox scope')) { return }
        New-ManagementRoleAssignment -Name $config.assignmentName -Role 'Application Calendars.ReadWrite' -App $sp.Id -CustomResourceScope $config.scopeName -ErrorAction Stop | Out-Null
        Register-Resource 'assignment' $config.assignmentName $true
    } else { Register-Resource 'assignment' $config.assignmentName $false }
    $stage='allowed mailbox authorization query'
    $allowed=@(Test-ServicePrincipalAuthorization -Identity $sp.Id -Resource $config.mailbox -ErrorAction Stop)
    $stage='allowed mailbox scope verification (expected Application Calendars.ReadWrite with InScope=True)'
    if (-not @($allowed | Where-Object { $_.RoleName -eq 'Application Calendars.ReadWrite' -and [string]$_.InScope -eq 'True' }).Count) { throw 'Allowed mailbox is not confirmed in scope.' }
    $stage='excluded mailbox authorization query (verify excludedMailbox is an existing mailbox in this tenant)'
    $excluded=@(Test-ServicePrincipalAuthorization -Identity $sp.Id -Resource $config.excludedMailbox -ErrorAction Stop)
    $stage='excluded mailbox scope verification (expected calendar role with InScope=False and no in-scope or unknown results)'
    if (-not @($excluded | Where-Object { $_.RoleName -eq 'Application Calendars.ReadWrite' -and [string]$_.InScope -eq 'False' }).Count -or @($excluded | Where-Object { [string]$_.InScope -ne 'False' }).Count) { throw 'Excluded mailbox denial is not confirmed.' }
    $stage='client credential creation'
    $secret=$null
    if ($CreateCredential) {
        if ($state.resources.ContainsKey('credential') -or $state.ContainsKey('credentialAttempt') -or @($app.PasswordCredentials | Where-Object { $null -ne $_ -and $_.DisplayName -eq 'Calendar lab' }).Count) { throw 'A credential is already recorded or its creation is uncertain. Reconcile it; setup will not rotate it.' }
        if ($VaultName -and (Get-SecretInfo -Name $VaultEntryName -Vault $VaultName -ErrorAction Stop)) { throw 'The vault entry already exists; setup will not overwrite it.' }
        if (-not $PSCmdlet.ShouldProcess($app.AppId,'Create a 90-day client credential (capture returned SecureString in memory)')) { return }
        # Persist intent before the non-idempotent write. A lost response must never
        # cause a second password to be issued automatically on the next run.
        $state.credentialAttempt=@{at=[DateTimeOffset]::UtcNow.ToString('o');outcome='unknown'}
        if ($VaultName) { $state.credentialStore=@{vault=$VaultName;name=$VaultEntryName} }
        Save-CalendarState $state $StatePath
        $password=Add-MgApplicationPassword -ApplicationId $app.Id -PasswordCredential @{displayName='Calendar lab';endDateTime=[DateTimeOffset]::UtcNow.AddDays(90)}
        $secret=[securestring]::new()
        foreach ($character in $password.SecretText.ToCharArray()) { $secret.AppendChar($character) }
        $secret.MakeReadOnly()
        Register-Resource 'credential' ([string]$password.KeyId) $true
        $state.resources.credential.expiresAt=([DateTimeOffset]$password.EndDateTime).ToString('o')
        $password=$null
        Save-CalendarState $state $StatePath
        if ($VaultName) {
            $stage='credential vault storage (do not recreate a password; inspect the recorded key and vault entry)'
            Set-Secret -Name $VaultEntryName -Vault $VaultName -SecureStringSecret $secret -NoClobber -ErrorAction Stop
            $stored=Get-Secret -Name $VaultEntryName -Vault $VaultName -ErrorAction Stop
            if ($stored -isnot [securestring] -or (Get-PlainSecret $stored) -cne (Get-PlainSecret $secret)) { throw 'Credential vault read-back failed.' }
            $stored=$null
        }
        $state.credentialAttempt.outcome=if ($VaultName) {'stored'} else {'returned-in-memory'}
        Save-CalendarState $state $StatePath
    }
    [pscustomobject]@{ ClientId=$app.AppId; ServicePrincipalObjectId=$sp.Id; ClientSecret=$secret; StatePath=$StatePath; RuntimeAccess='Pending Graph allowed/excluded tests; RBAC simulation alone is insufficient.' }
} catch { throw "Calendar provisioning stopped during $stage. Retain the state file and inspect resources before rerunning; credentials are never rotated implicitly." }
