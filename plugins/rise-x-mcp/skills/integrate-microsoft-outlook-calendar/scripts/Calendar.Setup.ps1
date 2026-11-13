#requires -Version 7.4
# Helpers for the guided entry point. Cloud provisioning remains in the existing scripts.
. (Join-Path $PSScriptRoot 'Calendar.Common.ps1')

function Read-CalendarSetupValue {
    param([string]$Prompt, [scriptblock]$Validate, [string]$Hint, [string]$Default)
    while ($true) {
        $answer = (Read-Host $(if ($Default) { "$Prompt [$Default]" } else { $Prompt })).Trim()
        if (-not $answer -and $Default) { $answer = $Default }
        if (& $Validate $answer) { return $answer }
        Write-Information $Hint -InformationAction Continue
    }
}

function Read-CalendarSetupConfig {
    param([string]$TenantId)
    $email = { param($value) $value -match '^[A-Za-z0-9._+%-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$' }
    $mailbox = Read-CalendarSetupValue 'Shared mailbox email (existing or to create)' $email 'Enter a full email address.'
    $excluded = Read-CalendarSetupValue 'Existing mailbox that must NOT be accessible' {
        param($value) (& $email $value) -and $value -ine $mailbox
    } 'Enter a different, real mailbox in the same tenant.'
    $origin = Read-CalendarSetupValue 'Rise-X API origin supplied for your environment' {
        param($value)
        $uri = $null
        [uri]::TryCreate($value, [UriKind]::Absolute, [ref]$uri) -and $uri.Scheme -eq 'https' -and
            $uri.HostNameType -eq [UriHostNameType]::Dns -and -not $uri.UserInfo -and
            -not $uri.Query -and -not $uri.Fragment -and $uri.AbsolutePath -eq '/'
    } 'Enter only the HTTPS API origin, without a workspace path, credentials or query.'
    $ecosystem = Read-CalendarSetupValue 'Exact Rise-X ecosystem name' {
        param($value) -not [string]::IsNullOrWhiteSpace($value) -and $value -notmatch '[\r\n]'
    } 'Enter the ecosystem name shown in Rise-X.'
    $environment = Read-CalendarSetupValue 'App build environment (for example test or production)' {
        param($value) $value -match '^[A-Za-z][A-Za-z0-9_-]{0,63}$'
    } 'Use letters, digits, underscores or hyphens; start with a letter.'
    # Stable names avoid manual object IDs and keep separate connections distinct.
    $binding = "$TenantId|$mailbox|$origin|$ecosystem|$environment".ToLowerInvariant()
    $hash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($binding))).Substring(0, 12)
    $name = "RiseXCalendar-$hash"
    return @{
        tenantId = $TenantId; mailbox = $mailbox; excludedMailbox = $excluded
        riseXBaseUrl = $origin.TrimEnd('/'); ecosystem = $ecosystem; environment = $environment
        applicationName = $name; scopeName = "$name-Scope"; assignmentName = "$name-Calendars"
        integrationName = $name; calendarId = ''
    }
}

function Import-CalendarSetupModule {
    [CmdletBinding(SupportsShouldProcess)]
    param([switch]$InstallPrerequisites)
    $versions = Get-Content (Join-Path $PSScriptRoot 'setup-modules.json') -Raw | ConvertFrom-Json -AsHashtable
    foreach ($name in $versions.Keys) {
        $version = [version]$versions[$name]
        $loaded = @(Get-Module -Name $name)
        if (@($loaded | Where-Object Version -NE $version).Count) {
            throw "A different $name version is loaded. Close this window and start pwsh -NoProfile."
        }
        if (-not @(Get-Module -ListAvailable -Name $name | Where-Object Version -EQ $version).Count) {
            if (-not $InstallPrerequisites) { throw "Missing $name $version. Rerun with -InstallPrerequisites or ask IT to install the versions in setup-modules.json." }
            $gallery = Get-PSRepository -Name PSGallery -ErrorAction Stop
            if ($gallery.SourceLocation.TrimEnd('/') -ne 'https://www.powershellgallery.com/api/v2') { throw 'PSGallery points to an unexpected source. Ask IT to review it; setup will not change repository trust.' }
            if (-not $PSCmdlet.ShouldProcess("$name $version", 'Install from official PSGallery for the current OS user')) { throw 'Required module installation declined.' }
            Install-Module -Name $name -RequiredVersion $version -Repository PSGallery -Scope CurrentUser -ErrorAction Stop
        }
    }
    # Exchange first reduces assembly conflicts in a fresh PowerShell process.
    foreach ($name in @('ExchangeOnlineManagement','Microsoft.Graph.Authentication','Microsoft.Graph.Applications','Microsoft.PowerShell.SecretManagement','Microsoft.PowerShell.SecretStore')) {
        Import-Module -Name $name -RequiredVersion $versions[$name] -ErrorAction Stop
    }
}

function Connect-CalendarSetupTenant {
    param([string]$TenantId, [switch]$DeviceLogin)
    $graph = Get-MgContext
    if ($graph -and ($graph.AuthType -ne 'Delegated' -or $graph.Environment -ne 'Global' -or
        ($TenantId -and $graph.TenantId -ne $TenantId) -or $graph.Scopes -notcontains 'Application.ReadWrite.All')) {
        throw 'The existing Graph session has the wrong tenant, cloud, authentication type or scopes. Use a fresh pwsh -NoProfile window; setup will not replace another session.'
    }
    if (-not $graph) {
        $arguments = @{Scopes = @('Application.ReadWrite.All'); ContextScope = 'Process'; NoWelcome = $true; ErrorAction = 'Stop'}
        if ($TenantId) { $arguments.TenantId = $TenantId }
        if ($DeviceLogin) { $arguments.UseDeviceAuthentication = $true }
        Connect-MgGraph @arguments | Out-Null
        $graph = Get-MgContext
    }
    if (-not $graph -or $graph.AuthType -ne 'Delegated' -or $graph.Environment -ne 'Global' -or
        ($TenantId -and $graph.TenantId -ne $TenantId) -or $graph.Scopes -notcontains 'Application.ReadWrite.All' -or
        $graph.TenantId -notmatch '^[0-9a-fA-F-]{36}$') { throw 'Graph sign-in did not confirm the required commercial tenant and delegated application-management scope.' }
    return $graph.TenantId
}

function Connect-CalendarSetupExchange {
    param([string]$TenantId, [switch]$DeviceLogin)
    $connections = @(Get-ConnectionInformation | Where-Object State -EQ Connected)
    # Remote cmdlets can bind to a different active connection; fail closed.
    if (@($connections | Where-Object TenantID -NE $TenantId).Count) { throw 'An Exchange session for another tenant is active. Use a fresh pwsh -NoProfile window.' }
    if (-not $connections.Count) {
        $arguments = @{ShowBanner = $false; ErrorAction = 'Stop'}
        if ($DeviceLogin) { $arguments.Device = $true }
        Connect-ExchangeOnline @arguments | Out-Null
        $connections = @(Get-ConnectionInformation | Where-Object State -EQ Connected)
    }
    if (-not $connections.Count -or @($connections | Where-Object TenantID -NE $TenantId).Count) {
        throw 'Exchange sign-in did not match the selected Microsoft tenant. No provisioning has started. Use a fresh window and sign into the correct organization.'
    }
    foreach ($command in @('Get-EXOMailbox','New-Mailbox','Get-ServicePrincipal','New-ServicePrincipal','Get-ManagementScope','New-ManagementScope','Get-ManagementRoleAssignment','New-ManagementRoleAssignment','Test-ServicePrincipalAuthorization')) {
        if (-not (Get-Command $command -ErrorAction SilentlyContinue)) { throw "Exchange command $command is unavailable. Activate the required Exchange roles, then reconnect." }
    }
}

function Initialize-CalendarSetupVault {
    [CmdletBinding(SupportsShouldProcess)]
    param([string]$Name)
    $vault = $null
    try { $vault = Get-SecretVault -Name $Name -ErrorAction Stop }
    catch { if ($_.FullyQualifiedErrorId -notlike 'GetSecretVaultNotFound,*') { throw } }
    if (-not $vault) {
        if ($Name -ne 'RiseXCalendarSetup') { throw 'Register your approved SecretManagement vault before using -SecretVault.' }
        if (-not $PSCmdlet.ShouldProcess($Name, 'Register a password-protected local SecretStore vault')) { throw 'Vault registration declined.' }
        Register-SecretVault -Name $Name -ModuleName Microsoft.PowerShell.SecretStore -DefaultVault:$false -ErrorAction Stop
        $vault = Get-SecretVault -Name $Name -ErrorAction Stop
    }
    if ($vault.ModuleName -eq 'Microsoft.PowerShell.SecretStore' -and
        (Get-SecretStoreConfiguration).Authentication -ne 'Password') {
        throw 'SecretStore must require a password. Ask IT to review its user-wide configuration; setup will not weaken or reset it.'
    }
    Write-Information 'Unlock your vault when prompted. On first use, choose and retain a vault password in your approved password manager.' -InformationAction Continue
    if (-not (Test-SecretVault -Name $Name -ErrorAction Stop)) { throw 'Vault is not ready. Unlock it and rerun; no cloud provisioning has started.' }
    # Prove write/read access before issuing a non-recoverable client credential.
    $probeName = 'RiseX-setup-probe-' + [guid]::NewGuid().ToString('N')
    $probe = [securestring]::new()
    foreach ($character in [guid]::NewGuid().ToString().ToCharArray()) { $probe.AppendChar($character) }
    $probe.MakeReadOnly()
    try {
        Set-Secret -Name $probeName -Vault $Name -SecureStringSecret $probe -NoClobber -ErrorAction Stop
        $readBack = Get-Secret -Name $probeName -Vault $Name -ErrorAction Stop
        if ($readBack -isnot [securestring] -or (Get-PlainSecret $readBack) -cne (Get-PlainSecret $probe)) { throw 'Vault read-back failed.' }
    } finally {
        Remove-Secret -Name $probeName -Vault $Name -ErrorAction Stop
        $probe = $null; $readBack = $null
    }
}

function Get-CalendarSetupIntegration {
    param([hashtable]$Config, [hashtable]$State, [securestring]$Token)
    if ($State.resources.ContainsKey('integration')) {
        try { return Invoke-RiseCalendarApi -Config $Config -Token $Token -Path "/$($State.resources.integration.id)" }
        catch { if ($_.Exception.Message -notmatch '\(status 404\)' -or -not $State.resources.integration.created) { throw } }
    }
    $found = @(Invoke-RiseCalendarApi -Config $Config -Token $Token -Path ('?name=' + [uri]::EscapeDataString($Config.integrationName)))
    if ($found.Count -gt 1) { throw 'Multiple integrations have this name. Select a dedicated integration before continuing.' }
    if ($found.Count) { return $found[0] }
    return $null
}

function Test-CalendarSetupCredential {
    param([string]$TenantId, [string]$ClientId, [securestring]$Secret)
    $body = @{client_id = $ClientId; client_secret = (Get-PlainSecret $Secret); grant_type = 'client_credentials'; scope = 'https://graph.microsoft.com/.default'}
    try {
        $response = Invoke-CalendarHttp -Uri "https://login.microsoftonline.com/$TenantId/oauth2/v2.0/token" -Method POST -ContentType 'application/x-www-form-urlencoded' -Body $body -RequestKind Token
        if (-not $response.access_token) { throw 'Microsoft did not return an access token.' }
    } finally { $body.Clear(); $response = $null }
}

function Invoke-CalendarSetupReadCheck {
    param([hashtable]$Arguments, [ValidateRange(1,10)][int]$Attempts = 3, [ValidateRange(0,60)][int]$DelaySeconds = 20)
    for ($attempt = 1; $attempt -le $Attempts; $attempt++) {
        try {
            & (Join-Path $PSScriptRoot 'Test-CalendarConnection.ps1') @Arguments -Scenario ReadOnly -Confirm:$false | Out-Null
            return
        } catch {
            # Only a denial of the allowed mailbox can be a propagation delay.
            # Never retry writes, invalid credentials or unexpected excluded access.
            if ($attempt -eq $Attempts -or $_.Exception.Message -notmatch '^Connection check failed during Microsoft allowed mailbox read:.*\(status 403\)') { throw }
            Write-Information "Mailbox permission may still be propagating. Read-only check $($attempt + 1)/$Attempts in $DelaySeconds seconds." -InformationAction Continue
            Start-Sleep -Seconds $DelaySeconds
        }
    }
}
