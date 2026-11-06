BeforeAll {
    $scripts = Split-Path $PSScriptRoot -Parent
    . (Join-Path $scripts 'Calendar.Setup.ps1')
    # Offline command stand-ins: tests never import providers or touch a real vault.
    function Install-Module { [CmdletBinding()] param($Name,$RequiredVersion,$Repository,$Scope) }
    function Get-PSRepository { [CmdletBinding()] param($Name) }
    function Get-MgContext { [CmdletBinding()] param() }
    function Connect-MgGraph { [CmdletBinding()] param($Scopes,$ContextScope,$TenantId,[switch]$NoWelcome,[switch]$UseDeviceAuthentication) }
    function Get-ConnectionInformation { [CmdletBinding()] param() }
    function Connect-ExchangeOnline { [CmdletBinding()] param($ShowBanner,[switch]$Device) }
    function Get-EXOMailbox { [CmdletBinding()] param($Identity) }
    function New-Mailbox { [CmdletBinding()] param([switch]$Shared,$Name,$PrimarySmtpAddress) }
    function Get-MgApplication { [CmdletBinding()] param($Filter) }
    function New-MgApplication { [CmdletBinding()] param($DisplayName,$SignInAudience) }
    function Get-MgServicePrincipal { [CmdletBinding()] param($Filter) }
    function New-MgServicePrincipal { [CmdletBinding()] param($AppId) }
    function Get-MgServicePrincipalAppRoleAssignment { [CmdletBinding()] param($ServicePrincipalId,[switch]$All) }
    function Get-ServicePrincipal { [CmdletBinding()] param($Identity) }
    function New-ServicePrincipal { [CmdletBinding()] param($AppId,$ObjectId,$DisplayName) }
    function Get-ManagementScope { [CmdletBinding()] param($Identity) }
    function New-ManagementScope { [CmdletBinding()] param($Name,$RecipientRestrictionFilter) }
    function Get-ManagementRoleAssignment { [CmdletBinding()] param($RoleAssignee) }
    function New-ManagementRoleAssignment { [CmdletBinding()] param($Name,$Role,$App,$CustomResourceScope) }
    function Test-ServicePrincipalAuthorization { [CmdletBinding()] param($Identity,$Resource) }
    function Add-MgApplicationPassword { [CmdletBinding()] param($ApplicationId,$PasswordCredential) }
    function Get-SecretVault { [CmdletBinding()] param($Name) }
    function Register-SecretVault { [CmdletBinding()] param($Name,$ModuleName,[switch]$DefaultVault) }
    function Get-SecretStoreConfiguration { [CmdletBinding()] param() }
    function Test-SecretVault { [CmdletBinding()] param($Name) }
    function Get-SecretInfo { [CmdletBinding()] param($Name,$Vault) }
    function Get-Secret { [CmdletBinding()] param($Name,$Vault) }
    function Set-Secret { [CmdletBinding()] param($Name,$Vault,$SecureStringSecret,[switch]$NoClobber) }
    function Remove-Secret { [CmdletBinding()] param($Name,$Vault) }
    function New-SetupHttpError([int]$Status) {
        $response = [Net.Http.HttpResponseMessage]::new([Net.HttpStatusCode]$Status)
        return [Microsoft.PowerShell.Commands.HttpResponseException]::new('PRIVATE-PROVIDER-DATA', $response)
    }
}

Describe 'Guided setup with offline Microsoft, Rise-X and vault providers' {
    BeforeEach {
        $folder = Join-Path $TestDrive ([guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory $folder | Out-Null
        $config = @{
            tenantId = '11111111-1111-4111-8111-111111111111'; mailbox = 'calendar@example.com'; excludedMailbox = 'excluded@example.com'
            riseXBaseUrl = 'https://api.example.com'; ecosystem = 'fixture'; environment = 'test'
            applicationName = 'SetupTest'; scopeName = 'SetupScope'; assignmentName = 'SetupCalendars'; integrationName = 'SetupIntegration'
        }
        Save-CalendarState $config (Join-Path $folder 'connection.json')
        $global:setupVault = @{}
        $global:setupIntegration = $null
        $global:setupPasswords = @()
        $global:setupPosts = 0
        $global:setupFailSave = $false
        $token = ConvertTo-SecureString 'PRIVATE-TOKEN' -AsPlainText -Force
        $versions = Get-Content (Join-Path $scripts 'setup-modules.json') -Raw | ConvertFrom-Json -AsHashtable
        Mock Get-Module { param($Name,[switch]$ListAvailable) if ($ListAvailable) { [pscustomobject]@{Name=[string]$Name;Version=[version]$versions[[string]$Name]} } }
        Mock Import-Module {}
        Mock Install-Module { throw 'No installation expected' }
        Mock Get-MgContext { [pscustomobject]@{TenantId=$config.tenantId;AuthType='Delegated';Environment='Global';Scopes=@('Application.ReadWrite.All')} }
        Mock Connect-MgGraph { throw 'No login expected' }
        Mock Get-ConnectionInformation { [pscustomobject]@{TenantID=$config.tenantId;State='Connected'} }
        Mock Connect-ExchangeOnline { throw 'No login expected' }
        Mock Get-EXOMailbox { [pscustomobject]@{RecipientTypeDetails='SharedMailbox'} }
        Mock Get-MgApplication { [pscustomobject]@{Id='app-object';AppId='22222222-2222-4222-8222-222222222222';PasswordCredentials=$global:setupPasswords} }
        Mock Get-MgServicePrincipal { [pscustomobject]@{Id='enterprise-object'} }
        Mock Get-MgServicePrincipalAppRoleAssignment { @() }
        Mock Get-ServicePrincipal { [pscustomobject]@{Identity='enterprise-object'} }
        Mock Get-ManagementScope { [pscustomobject]@{RecipientFilter="PrimarySmtpAddress -eq '$($config.mailbox)'"} }
        Mock Get-ManagementRoleAssignment { [pscustomobject]@{Name=$config.assignmentName;Role='Application Calendars.ReadWrite';CustomResourceScope=$config.scopeName} }
        Mock Test-ServicePrincipalAuthorization { param($Resource) [pscustomobject]@{RoleName='Application Calendars.ReadWrite';InScope=($Resource -eq $config.mailbox)} }
        Mock Add-MgApplicationPassword {
            $global:setupPasswords = @([pscustomobject]@{DisplayName='Calendar lab';KeyId='password-key'})
            [pscustomobject]@{SecretText='PRIVATE-SECRET';KeyId='password-key';EndDateTime=[DateTimeOffset]::UtcNow.AddDays(90)}
        }
        Mock Get-SecretVault { [pscustomobject]@{ModuleName='Microsoft.PowerShell.SecretStore'} }
        Mock Get-SecretStoreConfiguration { [pscustomobject]@{Authentication='Password'} }
        Mock Test-SecretVault { $true }
        Mock Get-SecretInfo { param($Name) if ($global:setupVault.ContainsKey($Name)) { [pscustomobject]@{Name=$Name} } }
        Mock Get-Secret { param($Name) $global:setupVault[$Name] }
        Mock Set-Secret {
            param($Name,$SecureStringSecret,[switch]$NoClobber)
            if ($NoClobber -and $global:setupVault.ContainsKey($Name)) { throw 'Entry exists' }
            $global:setupVault[$Name] = $SecureStringSecret
        }
        Mock Remove-Secret { param($Name) $global:setupVault.Remove($Name) }
        Mock Read-Host { throw 'Unexpected prompt' }
        Mock Start-Sleep {}
        Mock Invoke-RestMethod {
            param($Uri,$Method,$Body)
            if ($Uri -like 'https://login.microsoftonline.com/*') { return [pscustomobject]@{access_token='PRIVATE-GRAPH-TOKEN'} }
            if ($Uri -match 'excluded%40example.com') { throw (New-SetupHttpError 403) }
            if ($Uri -like 'https://graph.microsoft.com/*') { return [pscustomobject]@{id='calendar'} }
            if ($Uri -match '/invoke$') { return [pscustomobject]@{success=$true;statusCode=200;items=@()} }
            if ($Method -eq 'GET') { if ($global:setupIntegration) { return $global:setupIntegration }; return @() }
            $global:setupPosts++
            $payload = $Body | ConvertFrom-Json
            if ($Uri -match '/endpoint$') {
                $global:setupIntegration.endpoints += @($payload.endpoint)
            } else {
                $global:setupIntegration = [pscustomobject]@{id=$payload.id;name=$payload.name;maxRetries=0;baseUri=$payload.baseUri;authentication=$payload.authentication;parameters=@();endpoints=@()}
            }
            return $global:setupIntegration
        }
    }
    AfterAll { Remove-Variable setupVault,setupIntegration,setupPasswords,setupPosts,setupFailSave -Scope Global }

    It 'previews on a clean machine without files, prompts, modules or network' {
        $empty = Join-Path $TestDrive 'preview-only'
        & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $empty -InstallPrerequisites -WhatIf | Should -Match 'Preview'
        Test-Path $empty | Should -BeFalse
        Should -Invoke Get-Module -Times 0
        Should -Invoke Read-Host -Times 0
        Should -Invoke Invoke-RestMethod -Times 0
        Should -Invoke Set-Secret -Times 0
    }
    It 'loads the pinned provider modules' {
        Import-CalendarSetupModule -Confirm:$false
        Should -Invoke Import-Module -Times 5 -Exactly
    }
    It 'provisions, securely checkpoints a credential, verifies reads and reruns without new credentials or configuration writes' {
        $result = & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false
        $result.Outcome | Should -Be 'read-only-verified'
        $global:setupPosts | Should -Be 7
        $state = Get-Content "$folder/state.json" -Raw | ConvertFrom-Json -AsHashtable
        $state.credentialAttempt.outcome | Should -Be 'stored'
        $state.resources.credential.expiresAt | Should -Not -BeNullOrEmpty
        $global:setupVault.Keys | Should -HaveCount 1
        (Get-Content "$folder/rise-x-app.json" -Raw | ConvertFrom-Json).dependencies.PSObject.Properties | Should -HaveCount 6
        & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false | Out-Null
        $global:setupPosts | Should -Be 7
        Should -Invoke Add-MgApplicationPassword -Times 1 -Exactly
        Should -Invoke Invoke-RestMethod -Times 0 -ParameterFilter { $Method -ne 'GET' -and $Uri -like 'https://graph.microsoft.com/*' }
        Get-ChildItem $folder -Filter '*.json' | ForEach-Object { Get-Content $_.FullName -Raw | Should -Not -Match 'PRIVATE-|SecretText|access_token' }
    }
    It 'creates missing resources using the enterprise object ID and a mailbox-only role' {
        Mock Get-EXOMailbox { $null } -ParameterFilter { $Identity -eq $config.mailbox }
        Mock New-Mailbox { [pscustomobject]@{RecipientTypeDetails='SharedMailbox'} }
        Mock Get-MgApplication { @() }
        Mock New-MgApplication { [pscustomobject]@{Id='app-object';AppId='22222222-2222-4222-8222-222222222222';PasswordCredentials=@()} }
        Mock Get-MgServicePrincipal { @() }
        Mock New-MgServicePrincipal { [pscustomobject]@{Id='enterprise-object'} }
        Mock Get-ServicePrincipal { $null } -ParameterFilter { $ErrorAction -eq 'SilentlyContinue' }
        Mock New-ServicePrincipal {}
        Mock Get-ManagementScope { $null }
        Mock New-ManagementScope {}
        Mock Get-ManagementRoleAssignment { @() }
        Mock New-ManagementRoleAssignment {}
        (& "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false).Outcome | Should -Be 'read-only-verified'
        Should -Invoke New-ServicePrincipal -Times 1 -Exactly -ParameterFilter { $ObjectId -eq 'enterprise-object' -and $AppId -eq '22222222-2222-4222-8222-222222222222' }
        Should -Invoke New-ManagementScope -Times 1 -Exactly -ParameterFilter { $RecipientRestrictionFilter -eq "PrimarySmtpAddress -eq 'calendar@example.com'" }
        Should -Invoke New-ManagementRoleAssignment -Times 1 -Exactly -ParameterFilter { $Role -eq 'Application Calendars.ReadWrite' -and $App -eq 'enterprise-object' -and $CustomResourceScope -eq 'SetupScope' }
        $state = Get-Content "$folder/state.json" -Raw | ConvertFrom-Json
        $state.resources.application.created | Should -BeTrue
        $state.resources.mailbox.created | Should -BeTrue
    }
    It 'treats an omitted password collection as no existing credentials' {
        Mock Get-MgApplication { [pscustomobject]@{Id='app-object';AppId='22222222-2222-4222-8222-222222222222';PasswordCredentials=$null} }
        (& "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false).Outcome | Should -Be 'read-only-verified'
        Should -Invoke Read-Host -Times 0
        Should -Invoke Add-MgApplicationPassword -Times 1 -Exactly
    }
    It 'rejects the wrong Graph tenant before cloud or vault mutations' {
        Mock Get-MgContext { [pscustomobject]@{TenantId='33333333-3333-4333-8333-333333333333';AuthType='Delegated';Environment='Global';Scopes=@('Application.ReadWrite.All')} }
        { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } | Should -Throw '*Microsoft Graph sign-in*'
        Should -Invoke Add-MgApplicationPassword -Times 0
        Should -Invoke Set-Secret -Times 0
        Should -Invoke Invoke-RestMethod -Times 0
    }
    It 'rejects any other active Exchange tenant before provisioning' {
        Mock Get-ConnectionInformation {
            [pscustomobject]@{TenantID=$config.tenantId;State='Connected'}
            [pscustomobject]@{TenantID='other';State='Connected'}
        }
        { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } | Should -Throw '*Exchange sign-in*'
        Should -Invoke Add-MgApplicationPassword -Times 0
        Should -Invoke Set-Secret -Times 0
    }
    It 'checks Rise-X access before Microsoft provisioning and redacts provider details' {
        Mock Invoke-RestMethod { throw 'PRIVATE-TOKEN PRIVATE-SECRET PRIVATE-PROVIDER-DATA' }
        $caught = try { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } catch { $_ }
        $caught.Exception.Message | Should -Match 'Rise-X access preflight'
        $caught.Exception.Message | Should -Not -Match 'PRIVATE-'
        Get-Content "$folder/setup-status.json" -Raw | Should -Not -Match 'PRIVATE-'
        Should -Invoke Add-MgApplicationPassword -Times 0
    }
    It 'refuses an unprotected vault without changing its policy or provisioning' {
        Mock Get-SecretStoreConfiguration { [pscustomobject]@{Authentication='None'} }
        { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } | Should -Throw '*credential vault preflight*'
        Should -Invoke Set-Secret -Times 0
        Should -Invoke Add-MgApplicationPassword -Times 0
    }
    It 'registers the default vault on the actual provider not-found error' {
        $global:setupVaultRegistered = $false
        Mock Get-SecretVault {
            if (-not $global:setupVaultRegistered) {
                throw [Management.Automation.ErrorRecord]::new([InvalidOperationException]::new('Vault missing'), 'GetSecretVaultNotFound,Microsoft.PowerShell.SecretManagement.GetSecretVaultCommand', [Management.Automation.ErrorCategory]::ObjectNotFound, $null)
            }
            [pscustomobject]@{ModuleName='Microsoft.PowerShell.SecretStore'}
        }
        Mock Register-SecretVault { $global:setupVaultRegistered = $true }
        Initialize-CalendarSetupVault -Name RiseXCalendarSetup -Confirm:$false
        Should -Invoke Register-SecretVault -Times 1 -Exactly -ParameterFilter { $Name -eq 'RiseXCalendarSetup' -and $ModuleName -eq 'Microsoft.PowerShell.SecretStore' -and -not $DefaultVault }
        $global:setupVault.Keys | Should -HaveCount 0
        Remove-Variable setupVaultRegistered -Scope Global
    }
    It 'fails before provisioning if vault write/read is unavailable' {
        Mock Get-Secret { $null }
        { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } | Should -Throw '*credential vault preflight*'
        Should -Invoke Add-MgApplicationPassword -Times 0
        $global:setupVault.Count | Should -Be 0
    }
    It 'does not reissue a password after a lost credential-creation response' {
        Mock Add-MgApplicationPassword { throw 'PRIVATE lost response' }
        { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } | Should -Throw '*Microsoft provisioning*'
        $state = Get-Content "$folder/state.json" -Raw | ConvertFrom-Json -AsHashtable
        $state.credentialAttempt.outcome | Should -Be 'unknown'
        Mock Read-Host { ConvertTo-SecureString 'recovered-secret' -AsPlainText -Force }
        { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } | Should -Throw '*uncertain credential creation*'
        Should -Invoke Add-MgApplicationPassword -Times 1 -Exactly
        $global:setupPosts | Should -Be 0
    }
    It 'retains a successfully vaulted credential when Rise-X saving fails, then resumes' {
        $global:setupFailSave = $true
        Mock Invoke-RestMethod {
            param($Body)
            if ($global:setupFailSave) { throw 'PRIVATE write failure' }
            $payload = $Body | ConvertFrom-Json
            $global:setupIntegration = [pscustomobject]@{id=$payload.id;name=$payload.name;maxRetries=0;baseUri=$payload.baseUri;authentication=$payload.authentication;parameters=@();endpoints=@()}
            return $global:setupIntegration
        } -ParameterFilter { $Method -eq 'POST' -and $Uri -match '/integration$' }
        Mock Invoke-RestMethod { if ($global:setupIntegration) { return $global:setupIntegration }; throw (New-SetupHttpError 404) } -ParameterFilter { $Method -eq 'GET' -and $Uri -match '/integration/[0-9a-f-]+$' }
        { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } | Should -Throw '*Rise-X endpoint setup*'
        (Get-Content "$folder/state.json" -Raw | ConvertFrom-Json).credentialAttempt.outcome | Should -Be 'stored'
        $global:setupVault.Keys | Should -HaveCount 1
        $global:setupFailSave = $false
        (& "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false).Outcome | Should -Be 'read-only-verified'
        Should -Invoke Add-MgApplicationPassword -Times 1
    }
    It 'records the issued key when vault storage fails and refuses implicit rotation' {
        Mock Set-Secret { throw 'PRIVATE vault failure' } -ParameterFilter { $Name -notlike '*probe*' }
        { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } | Should -Throw '*Microsoft provisioning*'
        $state = Get-Content "$folder/state.json" -Raw | ConvertFrom-Json
        $state.resources.credential.id | Should -Be 'password-key'
        $state.resources.credential.expiresAt | Should -Not -BeNullOrEmpty
        $state.credentialAttempt.outcome | Should -Be 'unknown'
        Mock Read-Host { [securestring]::new() }
        { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } | Should -Throw '*existing credential recovery*'
        Should -Invoke Add-MgApplicationPassword -Times 1 -Exactly
        $global:setupPosts | Should -Be 0
    }
    It 'does not save an invalid imported credential to the vault or Rise-X' {
        $global:setupPasswords = @([pscustomobject]@{DisplayName='Existing';KeyId='existing-key'})
        Mock Read-Host { ConvertTo-SecureString 'invalid-secret' -AsPlainText -Force }
        Mock Invoke-RestMethod { throw (New-SetupHttpError 401) } -ParameterFilter { $Uri -like 'https://login.microsoftonline.com/*' }
        { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } | Should -Throw '*existing credential recovery*'
        Should -Invoke Add-MgApplicationPassword -Times 0
        Should -Invoke Set-Secret -Times 0 -ParameterFilter { $Name -notlike '*probe*' }
        $global:setupPosts | Should -Be 0
    }
    It 'only retries allowed-mailbox 403 reads and leaves acceptance incomplete on exhaustion' {
        Mock Invoke-RestMethod { throw (New-SetupHttpError 403) } -ParameterFilter { $Uri -match 'calendar%40example.com' }
        { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } | Should -Throw '*read-only acceptance*'
        Should -Invoke Start-Sleep -Times 2 -Exactly
        Should -Invoke Invoke-RestMethod -Times 3 -Exactly -ParameterFilter { $Uri -match 'calendar%40example.com' }
        (Get-Content "$folder/setup-status.json" -Raw | ConvertFrom-Json).outcome | Should -Be 'incomplete'
        $global:setupPosts | Should -Be 7
        Should -Invoke Add-MgApplicationPassword -Times 1
    }
    It 'never retries an excluded mailbox that is accessible' {
        Mock Invoke-RestMethod { [pscustomobject]@{id='forbidden-calendar'} } -ParameterFilter { $Uri -match 'excluded%40example.com' }
        { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } | Should -Throw '*read-only acceptance*'
        Should -Invoke Start-Sleep -Times 0
        (Get-Content "$folder/setup-status.json" -Raw | ConvertFrom-Json).outcome | Should -Be 'incomplete'
    }
    It 'will not overwrite a running installer status or acquire its setup folder lock' {
        Set-Content "$folder/setup-status.json" 'unchanged'
        $held = [IO.File]::Open("$folder/setup.lock", [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
        try {
            { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } | Should -Throw '*local setup folder*'
            Get-Content "$folder/setup-status.json" -Raw | Should -Match '^unchanged'
            Should -Invoke Get-Module -Times 0
        } finally { $held.Dispose() }
    }
    It 'fails on missing prerequisites without silently installing modules' {
        Mock Get-Module { @() }
        { & "$scripts/Start-CalendarSetup.ps1" -SetupDirectory $folder -RiseXToken $token -Confirm:$false } | Should -Throw '*PowerShell prerequisites*'
        Should -Invoke Install-Module -Times 0
    }
    It 'refuses a replaced PSGallery source even when installation is requested' {
        Mock Get-Module { @() }
        Mock Get-PSRepository { [pscustomobject]@{SourceLocation='https://untrusted.example.com/api/v2'} }
        { Import-CalendarSetupModule -InstallPrerequisites -Confirm:$false } | Should -Throw '*unexpected source*'
        Should -Invoke Install-Module -Times 0
    }
    It 'keeps module installation scoped to the current user and pinned versions' {
        Mock Get-Module { @() }
        Mock Get-PSRepository { [pscustomobject]@{SourceLocation='https://www.powershellgallery.com/api/v2'} }
        Mock Install-Module {}
        Import-CalendarSetupModule -InstallPrerequisites -Confirm:$false
        Should -Invoke Install-Module -Times 5 -Exactly -ParameterFilter { $Scope -eq 'CurrentUser' -and $Repository -eq 'PSGallery' -and $RequiredVersion -eq $versions[$Name] }
    }
}

Describe 'Guided input validation' {
    It 're-prompts invalid fields and generates stable resource names without client IDs' {
        $script:answers = [Collections.Generic.Queue[string]]::new()
        @('bad','shared@example.com','shared@example.com','excluded@example.com','http://bad','https://api.example.com','workspace','test') | ForEach-Object { $script:answers.Enqueue($_) }
        Mock Read-Host { $script:answers.Dequeue() }
        $config = Read-CalendarSetupConfig -TenantId '11111111-1111-4111-8111-111111111111'
        $config.mailbox | Should -Be 'shared@example.com'
        $config.excludedMailbox | Should -Be 'excluded@example.com'
        $config.riseXBaseUrl | Should -Be 'https://api.example.com'
        $config.applicationName | Should -Match '^RiseXCalendar-[0-9A-F]{12}$'
        $config.ContainsKey('clientId') | Should -BeFalse
        Should -Invoke Read-Host -Times 8 -Exactly
    }
}

Describe 'Customer setup packaging' {
    It 'requires signing for a customer release and makes no files on preview' {
        { & "$scripts/New-CalendarSetupPackage.ps1" -OutputPath "$TestDrive/release.zip" } | Should -Throw '*code-signing*'
        & "$scripts/New-CalendarSetupPackage.ps1" -OutputPath "$TestDrive/preview.zip" -Unsigned -WhatIf
        Test-Path "$TestDrive/preview.zip" | Should -BeFalse
    }
    It 'packages only the allowlisted files with valid hashes and an explicit unsigned label' {
        $package = & "$scripts/New-CalendarSetupPackage.ps1" -OutputPath "$TestDrive/internal.zip" -Unsigned
        $package.Signed | Should -BeFalse
        $package.SHA256 | Should -Be (Get-FileHash "$TestDrive/internal.zip" -Algorithm SHA256).Hash
        Get-Content "$TestDrive/internal.zip.sha256" | Should -Be $package.SHA256
        $expanded = Join-Path $TestDrive 'expanded'
        Expand-Archive "$TestDrive/internal.zip" $expanded
        $inventory = Get-Content "$expanded/package.json" -Raw | ConvertFrom-Json
        $inventory.signed | Should -BeFalse
        $inventory.purpose | Should -Be 'internal-test-only'
        $inventory.files | Should -HaveCount 12
        foreach ($entry in $inventory.files) {
            (Get-FileHash (Join-Path $expanded $entry.path) -Algorithm SHA256).Hash | Should -Be $entry.sha256
            $entry.path | Should -Not -Match '\.local|state\.json|node_modules|dist/'
        }
        # Exercise the extracted package, not the source checkout.
        & "$expanded/scripts/Start-CalendarSetup.ps1" -WhatIf | Should -Match 'Preview'
        { & "$scripts/New-CalendarSetupPackage.ps1" -OutputPath "$TestDrive/internal.zip" -Unsigned } | Should -Throw '*immutable*'
    }
}
