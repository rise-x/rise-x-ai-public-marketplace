BeforeAll {
    $scripts=Split-Path $PSScriptRoot -Parent
    . (Join-Path $scripts 'Calendar.Common.ps1')
    function New-TestConfig {
        foreach ($name in @('state.json','manifest.json','capabilities.json')) { Remove-Item (Join-Path $TestDrive $name) -Force -ErrorAction SilentlyContinue }
        $config=@{tenantId='11111111-1111-4111-8111-111111111111';clientId='22222222-2222-4222-8222-222222222222';mailbox='bookings@example.com';excludedMailbox='excluded@example.com';calendarId='';riseXBaseUrl='https://api.example.com';ecosystem='fixture';environment='test';applicationName='Calendar Lab';scopeName='LabScope';assignmentName='LabAssignment';integrationName='CalendarLab'}
        $config | ConvertTo-Json | Set-Content "$TestDrive/config.json"
        return $config
    }
}
Describe 'Preview and configuration safety' {
    BeforeEach { $config=New-TestConfig; Mock Invoke-RestMethod { throw 'Network must not run' } }
    It 'performs zero network calls or state writes for every WhatIf script' {
        & "$scripts/Initialize-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -CreateResources -CreateCredential -WhatIf
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -UpgradeHtmlTemplate -WhatIf
        & "$scripts/Test-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -CapabilitiesPath "$TestDrive/capabilities.json" -Scenario Lifecycle -WhatIf
        & "$scripts/Remove-CalendarDemoResources.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -RemoveConfiguration -RemoveProvisioning -WhatIf
        Test-Path "$TestDrive/state.json" | Should -BeFalse
        Test-Path "$TestDrive/manifest.json" | Should -BeFalse
        Test-Path "$TestDrive/capabilities.json" | Should -BeFalse
        Should -Invoke Invoke-RestMethod -Times 0
    }
    It 'requires HTTPS origins and rejects hidden secret configuration keys' {
        $config.riseXBaseUrl='http://api.example.com';$config | ConvertTo-Json | Set-Content "$TestDrive/config.json"
        {Read-CalendarConfig "$TestDrive/config.json"} | Should -Throw
        $config.riseXBaseUrl='https://api.example.com';$config.secret='never';$config | ConvertTo-Json | Set-Content "$TestDrive/config.json"
        {Read-CalendarConfig "$TestDrive/config.json"} | Should -Throw
    }
    It 'rejects a state file belonging to another mailbox' {
        $state=Read-CalendarState "$TestDrive/state.json" $config;Save-CalendarState $state "$TestDrive/state.json";$config.mailbox='another@example.com'
        {Read-CalendarState "$TestDrive/state.json" $config} | Should -Throw
    }
    It 'preserves typed attendee tokens and opaque event ID placeholders' {
        $templates=Get-CalendarTemplate $config
        ($templates | Where-Object alias -eq calendarSetAttendees).endpoint.request.attendees | Should -Be '$.attendees'
        ($templates | Where-Object alias -eq calendarCreate).endpoint.request.attendees | Should -Be '$.attendees'
        ($templates | Where-Object alias -eq calendarGet).endpoint.endpointUri | Should -Match '\{\$\.eventId\}'
        $config.calendarId='opaque/A+B=='
        (Get-CalendarTemplate $config | Where-Object alias -eq calendarQuery).endpoint.endpointUri | Should -Match '/calendars/opaque%2FA%2BB%3D%3D/calendarView'
    }
    It 'explains a known ecosystem error without exposing provider data' {
        Mock Invoke-RestMethod {
            $response=[System.Net.Http.HttpResponseMessage]::new([System.Net.HttpStatusCode]::BadRequest)
            $exception=[Microsoft.PowerShell.Commands.HttpResponseException]::new('SECRET', $response)
            $record=[System.Management.Automation.ErrorRecord]::new($exception,'HttpFailure',[System.Management.Automation.ErrorCategory]::InvalidOperation,$null)
            $record.ErrorDetails=[System.Management.Automation.ErrorDetails]::new('{"message":"Environment header is missing in the request.","user":"SECRET"}')
            throw $record
        }
        $caught=$null;try {Invoke-CalendarHttp -Uri 'https://example.com' } catch {$caught=$_.Exception.Message}
        $caught | Should -Match 'exact ecosystem name'
        $caught | Should -Match 'read request made no changes'
        $caught | Should -Not -Match 'SECRET|Reconcile writes'
    }
    It 'redacts provider errors and never leaks token or payload' {
        Mock Invoke-RestMethod { throw 'Bearer SECRET recipient@example.com' }
        $caught=$null;try {Invoke-CalendarHttp -Uri 'https://example.com' -Headers @{Authorization='Bearer SECRET'}} catch {$caught=$_.Exception.Message}
        $caught | Should -Not -Match 'SECRET|recipient@|Bearer'
        $caught | Should -Match 'status 0'
    }
}
Describe 'Rise-X setup and resume' {
    BeforeEach {
        $config=New-TestConfig
        $global:calendarTestIntegration=$null;$global:calendarTestPosts=0
        Mock Invoke-RestMethod {
            param($Uri,$Method,$Body)
            if ($Method -eq 'GET') { if ($global:calendarTestIntegration) {return @($global:calendarTestIntegration)};return @() }
            $global:calendarTestPosts++
            $payload=$Body | ConvertFrom-Json
            if ($Uri -match '/endpoint$') {$global:calendarTestIntegration.endpoints=@($global:calendarTestIntegration.endpoints | Where-Object id -ne $payload.endpoint.id)+@($payload.endpoint);return $global:calendarTestIntegration}
            $global:calendarTestIntegration=[pscustomobject]@{id=$payload.id;name=$payload.name;maxRetries=$payload.maxRetries;baseUri=$payload.baseUri;authentication=$payload.authentication;parameters=@();endpoints=@();headers=[pscustomobject]@{}}
            return $global:calendarTestIntegration
        }
        $token=ConvertTo-SecureString 'fixture-token' -AsPlainText -Force
        $secret=ConvertTo-SecureString 'fixture-secret' -AsPlainText -Force
    }
    It 'sets up once, retains credential and endpoint identities on rerun, and exports actual bindings' {
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -ClientSecret $secret -Confirm:$false
        $global:calendarTestPosts | Should -Be 7
        $original=Get-Content "$TestDrive/manifest.json" -Raw
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -Confirm:$false
        $global:calendarTestPosts | Should -Be 7
        (Get-Content "$TestDrive/manifest.json" -Raw | ConvertFrom-Json).dependencies.calendarQuery.ids.test.integrationId | Should -Be $global:calendarTestIntegration.id
        $original | Should -Not -Match 'fixture-secret|fixture-token'
        Get-Content "$TestDrive/state.json" -Raw | Should -Not -Match 'fixture-secret|fixture-token'
    }
    It 'preserves unrelated endpoints on a reused integration' {
        $global:calendarTestIntegration=[pscustomobject]@{id=[guid]::NewGuid().ToString();name=$config.integrationName;maxRetries=0;baseUri='https://graph.microsoft.com/v1.0';parameters=@();authentication=[pscustomobject]@{type='ClientCredentials';clientId=$config.clientId;tokenUrl="https://login.microsoftonline.com/$($config.tenantId)/oauth2/v2.0/token"};endpoints=@([pscustomobject]@{id='existing';name='Unrelated';endpointUri='/unrelated'})}
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -Confirm:$false
        $global:calendarTestIntegration.endpoints | Where-Object name -eq Unrelated | Should -HaveCount 1
        (Get-Content "$TestDrive/state.json" -Raw | ConvertFrom-Json).resources.integration.created | Should -BeFalse
    }
    It 'requires an explicit upgrade and preserves the existing endpoint ID, metadata and credentials across reruns' {
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -ClientSecret $secret -Confirm:$false
        $create=$global:calendarTestIntegration.endpoints | Where-Object name -eq CreateEvent
        $id=$create.id;$create.request.body.contentType='Text'
        $create | Add-Member NoteProperty description 'Retain metadata'
        $auth=$global:calendarTestIntegration.authentication | ConvertTo-Json -Depth 30
        $before=Get-Content "$TestDrive/state.json" -Raw | ConvertFrom-Json
        {& "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -Confirm:$false} | Should -Throw '*-UpgradeHtmlTemplate*'
        $global:calendarTestPosts | Should -Be 7
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -UpgradeHtmlTemplate -Confirm:$false
        $global:calendarTestPosts | Should -Be 8
        $after=$global:calendarTestIntegration.endpoints | Where-Object name -eq CreateEvent
        $after.id | Should -Be $id
        $after.request.body.contentType | Should -Be 'HTML'
        $after.description | Should -Be 'Retain metadata'
        ($global:calendarTestIntegration.authentication | ConvertTo-Json -Depth 30) | Should -Be $auth
        (Get-Content "$TestDrive/state.json" -Raw | ConvertFrom-Json).endpoints.calendarCreate.created | Should -Be $before.endpoints.calendarCreate.created
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -UpgradeHtmlTemplate -Confirm:$false
        $global:calendarTestPosts | Should -Be 8
        $global:calendarTestIntegration.endpoints | Should -HaveCount 6
    }
    It 'does not overwrite an unfamiliar create template even with the upgrade flag' {
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -ClientSecret $secret -Confirm:$false
        $create=$global:calendarTestIntegration.endpoints | Where-Object name -eq CreateEvent
        $create.request.body.contentType='Text';$create.request.body.content='$.custom'
        {& "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -UpgradeHtmlTemplate -Confirm:$false} | Should -Throw '*differs*'
        $global:calendarTestPosts | Should -Be 7
    }
    It 'upgrades and reruns when GET endpoints omit the nullable request property' {
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -ClientSecret $secret -Confirm:$false
        $ids=@($global:calendarTestIntegration.endpoints.id | Sort-Object)
        foreach ($endpoint in @($global:calendarTestIntegration.endpoints | Where-Object method -eq GET)) {
            $endpoint.PSObject.Properties.Remove('request')
        }
        ($global:calendarTestIntegration.endpoints | Where-Object name -eq CreateEvent).request.body.contentType='Text'
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -UpgradeHtmlTemplate -Confirm:$false
        $global:calendarTestPosts | Should -Be 8
        ($global:calendarTestIntegration.endpoints | Where-Object name -eq CreateEvent).request.body.contentType | Should -Be 'HTML'
        @($global:calendarTestIntegration.endpoints.id | Sort-Object) | Should -Be $ids
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -UpgradeHtmlTemplate -Confirm:$false
        $global:calendarTestPosts | Should -Be 8
        (Get-Content "$TestDrive/manifest.json" -Raw | ConvertFrom-Json).dependencies.PSObject.Properties.Name | Should -HaveCount 6
    }
    It 'accepts API-default None authentication during upgrade and preserves it across reruns' {
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -ClientSecret $secret -Confirm:$false
        foreach ($endpoint in $global:calendarTestIntegration.endpoints) {
            $endpoint | Add-Member NoteProperty authentication ('{"type":"None","tokenUrl":"","clientId":"","secret":"","grantType":"client_credentials","parameters":{}}' | ConvertFrom-Json)
            if ($endpoint.method -eq 'GET') {$endpoint.PSObject.Properties.Remove('request')}
        }
        $create=$global:calendarTestIntegration.endpoints | Where-Object name -eq CreateEvent
        $id=$create.id;$create.request.body.contentType='Text'
        $auth=$create.authentication | ConvertTo-Json -Compress
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -UpgradeHtmlTemplate -Confirm:$false
        $global:calendarTestPosts | Should -Be 8
        $create=$global:calendarTestIntegration.endpoints | Where-Object name -eq CreateEvent
        $create.id | Should -Be $id
        $create.request.body.contentType | Should -Be 'HTML'
        ($create.authentication | ConvertTo-Json -Compress) | Should -Be $auth
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -UpgradeHtmlTemplate -Confirm:$false
        $global:calendarTestPosts | Should -Be 8
    }
    It 'still blocks custom endpoint <Setting> without disclosing values or posting an upgrade' -ForEach @(
        @{Setting='authentication';Value='{"type":"Bearer","secret":"PRIVATE"}'},
        @{Setting='authentication';Value='{"type":"None","scheme":"PRIVATE"}'},
        @{Setting='authentication';Value='{"type":"None","secret":"PRIVATE"}'},
        @{Setting='parameters';Value='[{"name":"custom","value":"PRIVATE"}]'},
        @{Setting='headers';Value='{"Prefer":"outlook.timezone=\"UTC\"","X-Custom":"PRIVATE"}'}
    ) {
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -ClientSecret $secret -Confirm:$false
        $create=$global:calendarTestIntegration.endpoints | Where-Object name -eq CreateEvent
        $create.request.body.contentType='Text'
        $create | Add-Member NoteProperty $Setting ($Value | ConvertFrom-Json) -Force
        $message=$null
        try {& "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -UpgradeHtmlTemplate -Confirm:$false} catch {$message=$_.Exception.Message}
        $message | Should -Match 'additional connection settings'
        $message | Should -Not -Match 'PRIVATE'
        $global:calendarTestPosts | Should -Be 7
    }
    It 'rejects an omitted required write request without rewriting the endpoint' {
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -ClientSecret $secret -Confirm:$false
        ($global:calendarTestIntegration.endpoints | Where-Object name -eq CreateEvent).PSObject.Properties.Remove('request')
        {& "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -UpgradeHtmlTemplate -Confirm:$false} | Should -Throw '*Existing request template differs*'
        $global:calendarTestPosts | Should -Be 7
    }
    It 'reports an omitted upgrade read-back request as unverified and retains endpoint identity' {
        & "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -ClientSecret $secret -Confirm:$false
        $create=$global:calendarTestIntegration.endpoints | Where-Object name -eq CreateEvent
        $id=$create.id;$create.request.body.contentType='Text'
        Mock Invoke-RestMethod {
            param($Body)
            $replacement=($Body | ConvertFrom-Json).endpoint
            $replacement.PSObject.Properties.Remove('request')
            $global:calendarTestIntegration.endpoints=@($global:calendarTestIntegration.endpoints | Where-Object id -ne $replacement.id)+@($replacement)
            return $global:calendarTestIntegration
        } -ParameterFilter {$Method -eq 'POST' -and $Uri -match '/endpoint$'}
        {& "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -UpgradeHtmlTemplate -Confirm:$false} | Should -Throw '*HTML upgrade read-back did not match*'
        (Get-Content "$TestDrive/state.json" -Raw | ConvertFrom-Json).endpoints.calendarCreate.id | Should -Be $id
    }
    It 'preserves ownership and refuses a recorded integration renamed outside the script' {
        $state=Read-CalendarState "$TestDrive/state.json" $config
        $state.resources.integration=@{id=[guid]::NewGuid().ToString();created=$false};Save-CalendarState $state "$TestDrive/state.json"
        $global:calendarTestIntegration=[pscustomobject]@{id=$state.resources.integration.id;name='Renamed externally'}
        {& "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -ClientSecret $secret -Confirm:$false} | Should -Throw '*renamed*'
        $global:calendarTestPosts | Should -Be 0
        (Get-Content "$TestDrive/state.json" -Raw | ConvertFrom-Json).resources.integration.created | Should -BeFalse
    }
    It 'rejects reused integrations that can automatically retry writes' {
        $global:calendarTestIntegration=[pscustomobject]@{id=[guid]::NewGuid().ToString();name=$config.integrationName;maxRetries=1}
        {& "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -Confirm:$false} | Should -Throw '*maxRetries=0*'
        $global:calendarTestPosts | Should -Be 0
        Test-Path "$TestDrive/manifest.json" | Should -BeFalse
    }
    It 'retains partial identities when a request fails' {
        Mock Invoke-RestMethod {throw 'provider failure SECRET'} -ParameterFilter {$Method -eq 'POST' -and $Uri -match '/endpoint$'}
        {& "$scripts/Set-RiseXCalendarIntegration.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -ManifestPath "$TestDrive/manifest.json" -RiseXToken $token -ClientSecret $secret -Confirm:$false} | Should -Throw
        $state=Get-Content "$TestDrive/state.json" -Raw | ConvertFrom-Json
        $state.resources.integration.id | Should -Not -BeNullOrEmpty
        $state.endpoints.calendarQuery.id | Should -Not -BeNullOrEmpty
        Test-Path "$TestDrive/manifest.json" | Should -BeFalse
    }
}
Describe 'Ownership-limited cleanup' {
    It 'preserves reused integration and endpoints' {
        $config=New-TestConfig;$state=Read-CalendarState "$TestDrive/state.json" $config
        $state.resources.integration=@{id='recorded';created=$false};$state.endpoints.calendarQuery=@{id='reused';created=$false};Save-CalendarState $state "$TestDrive/state.json"
        Mock Invoke-RestMethod {return [pscustomobject]@{id='recorded';endpoints=@([pscustomobject]@{id='reused'})}}
        $token=ConvertTo-SecureString 'fixture-token' -AsPlainText -Force
        & "$scripts/Remove-CalendarDemoResources.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -RiseXToken $token -RemoveConfiguration -Confirm:$false
        Should -Invoke Invoke-RestMethod -Times 0 -ParameterFilter {$Method -eq 'DELETE' -or $Method -eq 'POST'}
    }
}

Describe 'Microsoft provisioning stops on Exchange failures' {
    BeforeAll {
        # Offline stand-ins for module and remote-session commands.
        function Get-MgContext { [CmdletBinding()] param() }
        function Get-ConnectionInformation { [CmdletBinding()] param() }
        function Get-EXOMailbox { [CmdletBinding()] param($Identity) }
        function Get-MgApplication { [CmdletBinding()] param($Filter) }
        function Get-MgServicePrincipal { [CmdletBinding()] param($Filter) }
        function Get-MgServicePrincipalAppRoleAssignment { [CmdletBinding()] param($ServicePrincipalId,[switch]$All) }
        function Get-ServicePrincipal { [CmdletBinding()] param($Identity) }
        function New-ServicePrincipal { [CmdletBinding()] param($AppId,$ObjectId,$DisplayName) }
        function Get-ManagementScope { [CmdletBinding()] param($Identity) }
        function Get-ManagementRoleAssignment { [CmdletBinding()] param($RoleAssignee) }
        function New-ManagementRoleAssignment { [CmdletBinding()] param($Name,$Role,$App,$CustomResourceScope) }
        function Test-ServicePrincipalAuthorization { [CmdletBinding()] param($Identity,$Resource) }
        function Add-MgApplicationPassword { [CmdletBinding()] param($ApplicationId,$PasswordCredential) }
    }
    BeforeEach {
        $config=New-TestConfig
        Mock Import-Module {}
        Mock Get-MgContext { [pscustomobject]@{TenantId=$config.tenantId} }
        Mock Get-ConnectionInformation { [pscustomobject]@{TenantID=$config.tenantId;State='Connected'} }
        Mock Get-EXOMailbox { [pscustomobject]@{RecipientTypeDetails='SharedMailbox'} }
        Mock Get-MgApplication { [pscustomobject]@{Id='app-object';AppId=$config.clientId;PasswordCredentials=@()} }
        Mock Get-MgServicePrincipal { [pscustomobject]@{Id='enterprise-object';AppId=$config.clientId} }
        Mock Get-MgServicePrincipalAppRoleAssignment { @() }
        Mock Get-ServicePrincipal { $null }
        Mock New-ServicePrincipal {}
        Mock Get-ManagementScope { [pscustomobject]@{RecipientFilter="PrimarySmtpAddress -eq '$($config.mailbox)'"} }
        Mock Get-ManagementRoleAssignment { @() }
        Mock New-ManagementRoleAssignment {}
        Mock Test-ServicePrincipalAuthorization { param($Resource) [pscustomobject]@{RoleName='Application Calendars.ReadWrite';InScope=($Resource -eq $config.mailbox)} }
        Mock Add-MgApplicationPassword { throw 'Credential creation must not run' }
    }
    It 'stops a nonterminating registration error without recording success or attempting roles or credentials' {
        Mock New-ServicePrincipal { Write-Error 'AADServicePrincipalNotFound' }
        { & "$scripts/Initialize-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -CreateResources -CreateCredential -Confirm:$false } | Should -Throw '*Exchange service-principal registration*'
        $state=Get-Content "$TestDrive/state.json" -Raw | ConvertFrom-Json -AsHashtable
        $state.resources.ContainsKey('servicePrincipal') | Should -BeTrue
        $state.resources.ContainsKey('exchangePrincipal') | Should -BeFalse
        Should -Invoke New-ServicePrincipal -Times 1 -ParameterFilter { $ObjectId -eq 'enterprise-object' -and $AppId -eq $config.clientId -and $ErrorAction -eq 'Stop' }
        Should -Invoke Get-ManagementScope -Times 0
        Should -Invoke New-ManagementRoleAssignment -Times 0
        Should -Invoke Test-ServicePrincipalAuthorization -Times 0
        Should -Invoke Add-MgApplicationPassword -Times 0
    }
    It 'requires registration read-back before recording success' {
        { & "$scripts/Initialize-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -CreateResources -Confirm:$false } | Should -Throw '*Exchange service-principal registration*'
        (Get-Content "$TestDrive/state.json" -Raw | ConvertFrom-Json -AsHashtable).resources.ContainsKey('exchangePrincipal') | Should -BeFalse
        Should -Invoke Get-ServicePrincipal -Times 1 -ParameterFilter { $ErrorAction -eq 'Stop' }
        Should -Invoke Get-ManagementScope -Times 0
    }
    It 'does not treat a role lookup error as an empty assignment list' {
        Mock Get-ServicePrincipal { [pscustomobject]@{Identity='enterprise-object'} }
        Mock Get-ManagementRoleAssignment { Write-Error 'Lookup failed' }
        { & "$scripts/Initialize-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -CreateResources -Confirm:$false } | Should -Throw '*role assignment*'
        Should -Invoke New-ManagementRoleAssignment -Times 0
        Should -Invoke Test-ServicePrincipalAuthorization -Times 0
    }
    It 'checks that the excluded mailbox exists before provisioning or writing state' {
        Mock Get-EXOMailbox { Write-Error 'Mailbox not found' } -ParameterFilter { $Identity -eq $config.excludedMailbox }
        { & "$scripts/Initialize-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -CreateResources -CreateCredential -Confirm:$false } | Should -Throw '*excluded mailbox lookup*'
        Should -Invoke Get-MgApplication -Times 0
        Should -Invoke New-ServicePrincipal -Times 0
        Test-Path "$TestDrive/state.json" | Should -BeFalse
    }
    It 'accepts explicit string True and False scope results without treating False as truthy' {
        Mock Get-ServicePrincipal { [pscustomobject]@{Identity='enterprise-object'} }
        Mock Test-ServicePrincipalAuthorization { param($Resource) [pscustomobject]@{RoleName='Application Calendars.ReadWrite';InScope=if ($Resource -eq $config.mailbox) {'True'} else {'False'}} }
        $result=& "$scripts/Initialize-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -CreateResources -Confirm:$false
        $result.ClientId | Should -Be $config.clientId
    }
    It 'distinguishes allowed mailbox scope failure and does not create a credential' {
        Mock Get-ServicePrincipal { [pscustomobject]@{Identity='enterprise-object'} }
        Mock Test-ServicePrincipalAuthorization { [pscustomobject]@{RoleName='Application Calendars.ReadWrite';InScope=$false} }
        { & "$scripts/Initialize-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -CreateResources -CreateCredential -Confirm:$false } | Should -Throw '*allowed mailbox scope verification*'
        Should -Invoke Add-MgApplicationPassword -Times 0
    }
    It 'rejects an excluded result of <ScopeResult> before creating credentials' -ForEach @(
        @{ScopeResult='True'}, @{ScopeResult='Not Run'}, @{ScopeResult='Empty'}
    ) {
        Mock Get-ServicePrincipal { [pscustomobject]@{Identity='enterprise-object'} }
        Mock Test-ServicePrincipalAuthorization {
            param($Resource)
            if ($Resource -eq $config.mailbox) { return [pscustomobject]@{RoleName='Application Calendars.ReadWrite';InScope=$true} }
            if ($ScopeResult -ne 'Empty') { [pscustomobject]@{RoleName='Application Calendars.ReadWrite';InScope=$ScopeResult} }
        }
        { & "$scripts/Initialize-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -CreateResources -CreateCredential -Confirm:$false } | Should -Throw '*excluded mailbox scope verification*'
        Should -Invoke Add-MgApplicationPassword -Times 0
    }
    It 'rechecks legacy saved steps and resumes after registration becomes visible' {
        $state=Read-CalendarState "$TestDrive/state.json" $config
        $state.resources.exchangePrincipal=@{id='enterprise-object';created=$true}
        $state.resources.assignment=@{id=$config.assignmentName;created=$true}
        Save-CalendarState $state "$TestDrive/state.json"
        Mock Get-ServicePrincipal { [pscustomobject]@{Identity='enterprise-object'} } -ParameterFilter { $ErrorAction -eq 'Stop' }
        $result=& "$scripts/Initialize-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -CreateResources -Confirm:$false
        $result.ClientId | Should -Be $config.clientId
        Should -Invoke New-ServicePrincipal -Times 1
        Should -Invoke New-ManagementRoleAssignment -Times 1
        Should -Invoke Test-ServicePrincipalAuthorization -Times 2
        (Get-Content "$TestDrive/state.json" -Raw | ConvertFrom-Json).resources.exchangePrincipal.created | Should -BeTrue
    }
}

Describe 'Read-only connection diagnostics' {
    BeforeAll {
        function New-TestHttpError([int]$Status,[string]$Detail) {
            $response=[System.Net.Http.HttpResponseMessage]::new([System.Net.HttpStatusCode]$Status)
            $exception=[Microsoft.PowerShell.Commands.HttpResponseException]::new('SECRET provider text', $response)
            $record=[System.Management.Automation.ErrorRecord]::new($exception,'HttpFailure',[System.Management.Automation.ErrorCategory]::InvalidOperation,$null)
            $record.ErrorDetails=[System.Management.Automation.ErrorDetails]::new($Detail)
            return $record
        }
    }
    BeforeEach {
        $config=New-TestConfig
        $state=Read-CalendarState "$TestDrive/state.json" $config
        $state.resources.integration=@{id='integration';created=$true}
        $state.endpoints.calendarQuery=@{id='query';created=$true}
        Save-CalendarState $state "$TestDrive/state.json"
        $token=ConvertTo-SecureString 'fixture-token' -AsPlainText -Force
        $secret=ConvertTo-SecureString 'fixture-secret' -AsPlainText -Force
        Mock Invoke-RestMethod {
            param($Uri,$Body,$ContentType)
            if ($Uri -match 'login.microsoftonline.com') {
                $ContentType | Should -Be 'application/x-www-form-urlencoded'
                $Body.client_id | Should -Be $config.clientId
                $Body.client_secret | Should -Be 'fixture-secret'
                return [pscustomobject]@{access_token='fixture-access-token'}
            }
            if ($Uri -match 'excluded%40example.com') {throw (New-TestHttpError 403 '{"error":{"code":"ErrorAccessDenied"}}')}
            if ($Uri -match 'graph.microsoft.com') {return [pscustomobject]@{id='calendar'}}
            return [pscustomobject]@{success=$true;statusCode=200;response=[pscustomobject]@{value=@()};items=@()}
        }
    }
    It 'completes token, allowed, excluded and Rise-X reads without changing saved state' {
        $before=Get-Content "$TestDrive/state.json" -Raw
        & "$scripts/Test-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -RiseXToken $token -ClientSecret $secret | Should -Match 'No writes performed'
        Get-Content "$TestDrive/state.json" -Raw | Should -Be $before
        Should -Invoke Invoke-RestMethod -Times 4
        Should -Invoke Invoke-RestMethod -Times 1 -ParameterFilter { $Uri -match 'login.microsoftonline.com' -and $ContentType -eq 'application/x-www-form-urlencoded' }
    }
    It 'checks the selected named calendar and exports only bound non-secret capability metadata' {
        $config.calendarId='opaque/A+B=='
        $config | ConvertTo-Json | Set-Content "$TestDrive/config.json"
        Remove-Item "$TestDrive/state.json"
        $state=Read-CalendarState "$TestDrive/state.json" $config
        $state.resources.integration=@{id='integration';created=$false}
        $state.endpoints.calendarQuery=@{id='query';created=$false}
        Save-CalendarState $state "$TestDrive/state.json"
        $before=Get-Content "$TestDrive/state.json" -Raw
        Mock Invoke-RestMethod { [pscustomobject]@{id='opaque/A+B==';allowedOnlineMeetingProviders=@('teamsForBusiness');name='PRIVATE';owner='PRIVATE'} } -ParameterFilter { $Uri -match '/calendars/' }
        & "$scripts/Test-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -RiseXToken $token -ClientSecret $secret -CapabilitiesPath "$TestDrive/capabilities.json" -Confirm:$false
        $export=Get-Content "$TestDrive/capabilities.json" -Raw | ConvertFrom-Json
        $export.teamsStatus | Should -Be 'supported'
        $export.binding | Should -Be $state.binding
        $export.calendarId | Should -Be $config.calendarId
        $export.endpoints.calendarQuery | Should -Be 'query'
        $export.clientId | Should -Be $config.clientId
        $export.checkedAt | Should -Not -BeNullOrEmpty
        Get-Content "$TestDrive/capabilities.json" -Raw | Should -Not -Match 'fixture-secret|fixture-token|fixture-access-token|PRIVATE'
        Get-Content "$TestDrive/state.json" -Raw | Should -Be $before
        Should -Invoke Invoke-RestMethod -Times 1 -Exactly -ParameterFilter { $Uri -like '*/calendars/opaque%2FA%2BB%3D%3D' }
    }
    It 'exports unknown rather than enabling Teams when the provider field is absent' {
        & "$scripts/Test-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -RiseXToken $token -ClientSecret $secret -CapabilitiesPath "$TestDrive/capabilities.json" -Confirm:$false
        (Get-Content "$TestDrive/capabilities.json" -Raw | ConvertFrom-Json).teamsStatus | Should -Be 'unknown'
    }
    It 'does not export capabilities when the Rise-X read fails' {
        Mock Invoke-RestMethod {throw (New-TestHttpError 400 '{}')} -ParameterFilter {$Uri -match '/invoke$'}
        {& "$scripts/Test-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -RiseXToken $token -ClientSecret $secret -CapabilitiesPath "$TestDrive/capabilities.json" -Confirm:$false} | Should -Throw
        Test-Path "$TestDrive/capabilities.json" | Should -BeFalse
    }
    It 'refuses to overwrite recovery state with a capability export before making calls' {
        $before=Get-Content "$TestDrive/state.json" -Raw
        {& "$scripts/Test-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -RiseXToken $token -ClientSecret $secret -CapabilitiesPath "$TestDrive/state.json" -Confirm:$false} | Should -Throw '*separate capabilities*'
        Get-Content "$TestDrive/state.json" -Raw | Should -Be $before
        Should -Invoke Invoke-RestMethod -Times 0
    }
    It 'identifies token failure and exposes only allowlisted OAuth diagnostics' {
        Mock Invoke-RestMethod {throw (New-TestHttpError 400 '{"error":"invalid_client","error_codes":[7000215,"SECRET"],"error_description":"SECRET recipient@example.com"}')} -ParameterFilter {$Uri -match 'login.microsoftonline.com'}
        $caught=$null
        try { & "$scripts/Test-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -RiseXToken $token -ClientSecret $secret } catch {$caught=$_.Exception.Message}
        $caught | Should -Match 'Microsoft token acquisition.*status 400.*invalid_client.*7000215'
        $caught | Should -Not -Match 'SECRET|recipient@|Reconcile writes'
        Should -Invoke Invoke-RestMethod -Times 1
    }
    It 'identifies a Rise-X query transport failure as read-only' {
        Mock Invoke-RestMethod {throw (New-TestHttpError 400 '{"message":"SECRET"}')} -ParameterFilter {$Uri -match '/invoke$'}
        $caught=$null
        try { & "$scripts/Test-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -RiseXToken $token -ClientSecret $secret } catch {$caught=$_.Exception.Message}
        $caught | Should -Match 'Rise-X calendarQuery endpoint invocation.*status 400'
        $caught | Should -Match 'read request made no changes'
        $caught | Should -Not -Match 'SECRET|Reconcile writes'
    }
    It 'continues to flag uncertain write failures for reconciliation' {
        Mock Invoke-RestMethod {throw (New-TestHttpError 400 '{"message":"SECRET"}')}
        {Invoke-CalendarHttp -Uri 'https://api.example.com' -Method POST -Body @{value='fixture'}} | Should -Throw '*Reconcile writes before retrying*'
    }
}

Describe 'Optional provider response fields' {
    AfterEach { Remove-Variable -Name calendarCleanupReads -Scope Global -ErrorAction SilentlyContinue }
    BeforeEach {
        $config=New-TestConfig
        $state=Read-CalendarState "$TestDrive/state.json" $config
        $state.resources.integration=@{id='integration';created=$false}
        foreach ($alias in @('calendarGet','calendarCancel')) {$state.endpoints[$alias]=@{id=$alias;created=$false}}
        $token=ConvertTo-SecureString 'fixture-token' -AsPlainText -Force
    }
    It 'accepts a successful empty cancellation envelope with status <Code>' -ForEach @(@{Code=202},@{Code=204}) {
        Mock Invoke-RiseCalendarApi { [pscustomobject]@{success=$true;statusCode=$Code;items=@()} }
        $result=Invoke-CalendarOperation $config $state $token 'calendarCancel' @{eventId='recorded'}
        $result.statusCode | Should -Be $Code
    }
    It 'extracts an event from items without a response field' {
        $result='{"success":true,"statusCode":201,"items":[{"id":"recorded"}]}' | ConvertFrom-Json
        (Get-CalendarEventResult $result).id | Should -Be 'recorded'
    }
    It 'extracts an event from response without an items field' {
        $result='{"success":true,"statusCode":201,"response":{"id":"recorded"}}' | ConvertFrom-Json
        (Get-CalendarEventResult $result).id | Should -Be 'recorded'
    }
    It 'keeps the reconciliation error when no event payload is returned' {
        $result='{"success":true,"statusCode":201}' | ConvertFrom-Json
        {Get-CalendarEventResult $result} | Should -Throw '*no event ID*reconcile*'
    }
    It 'continues to reject incomplete pages' {
        Mock Invoke-RiseCalendarApi { '{"success":true,"statusCode":200,"response":{"@odata.nextLink":"https://example.com/next"},"items":[]}' | ConvertFrom-Json }
        {Invoke-CalendarOperation $config $state $token 'calendarGet' @{eventId='recorded'}} | Should -Throw '*Incomplete calendar page*'
    }
    It 'does not mask external failures with an absent response' {
        Mock Invoke-RiseCalendarApi { [pscustomobject]@{success=$false;statusCode=403;items=@()} }
        {Invoke-CalendarOperation $config $state $token 'calendarCancel' @{eventId='recorded'}} | Should -Throw '*status 403*'
    }
    It 'reconciles a recorded cancellation already returning 404 without another write' {
        $state.runs=@(@{kind='appointment';operation='calendarCancel';outcome='unknown';eventId='recorded';transactionId='transaction'})
        Save-CalendarState $state "$TestDrive/state.json"
        Mock Invoke-RiseCalendarApi { [pscustomobject]@{success=$false;statusCode=404;items=@()} }
        & "$scripts/Remove-CalendarDemoResources.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -RiseXToken $token -Confirm:$false
        (Read-CalendarState "$TestDrive/state.json" $config).runs[0].outcome | Should -Be 'cancelled'
        Should -Invoke Invoke-RiseCalendarApi -Times 1 -Exactly -ParameterFilter {$Path -like '*/calendarGet/invoke'}
        Should -Invoke Invoke-RiseCalendarApi -Times 0 -Exactly -ParameterFilter {$Path -like '*/calendarCancel/invoke'}
    }
    It 'confirms cleanup after a successful cancellation with no response field' {
        $state.runs=@(@{kind='appointment';operation='calendarCancel';outcome='unknown';eventId='recorded';transactionId='transaction'})
        Save-CalendarState $state "$TestDrive/state.json"
        $global:calendarCleanupReads=0
        Mock Invoke-RiseCalendarApi {
            param($Path)
            if ($Path -like '*/calendarCancel/invoke') {return [pscustomobject]@{success=$true;statusCode=202;items=@()}}
            $global:calendarCleanupReads++
            if ($global:calendarCleanupReads -gt 1) {return [pscustomobject]@{success=$false;statusCode=404;items=@()}}
            return [pscustomobject]@{success=$true;statusCode=200;items=@([pscustomobject]@{id='recorded';transactionId='transaction';isOrganizer=$true;type='singleInstance'})}
        }
        & "$scripts/Remove-CalendarDemoResources.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -RiseXToken $token -Confirm:$false
        (Read-CalendarState "$TestDrive/state.json" $config).runs[0].outcome | Should -Be 'cancelled'
        Should -Invoke Invoke-RiseCalendarApi -Times 1 -Exactly -ParameterFilter {$Path -like '*/calendarCancel/invoke'}
        Should -Invoke Invoke-RiseCalendarApi -Times 2 -Exactly -ParameterFilter {$Path -like '*/calendarGet/invoke'}
    }
    It 'rejects documentation recipient placeholders before network calls or state changes' {
        Save-CalendarState $state "$TestDrive/state.json"
        $before=Get-Content "$TestDrive/state.json" -Raw
        Mock Invoke-RestMethod {throw 'Network must not run'}
        {& "$scripts/Test-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -Scenario Lifecycle -RecipientA 'recipient-a@example.com' -RecipientB 'recipient-b@example.com' -Confirm:$false} | Should -Throw '*Replace example-domain placeholders*'
        Get-Content "$TestDrive/state.json" -Raw | Should -Be $before
        Should -Invoke Invoke-RestMethod -Times 0
    }
}

Describe 'Selected-calendar online meeting capability' {
    It 'classifies <Json> as <Expected> without guessing from the default provider' -ForEach @(
        @{Json='{"allowedOnlineMeetingProviders":["teamsForBusiness"]}';Expected='supported'},
        @{Json='{"allowedOnlineMeetingProviders":["skypeForBusiness"],"defaultOnlineMeetingProvider":"teamsForBusiness"}';Expected='unsupported'},
        @{Json='{"allowedOnlineMeetingProviders":[]}';Expected='unsupported'},
        @{Json='{"defaultOnlineMeetingProvider":"teamsForBusiness"}';Expected='unknown'},
        @{Json='{"allowedOnlineMeetingProviders":null}';Expected='unknown'},
        @{Json='{"allowedOnlineMeetingProviders":"teamsForBusiness"}';Expected='unknown'},
        @{Json='{"allowedOnlineMeetingProviders":["teamsForBusiness",null]}';Expected='unknown'}
    ) {
        (Get-CalendarOnlineMeetingCapability ($Json | ConvertFrom-Json)).teamsStatus | Should -Be $Expected
    }
}

Describe 'HTML-only content contracts' {
    It 'composes escaped text, paragraphs and an optional app link' {
        $html=ConvertTo-CalendarHtmlBody -Subject '<Meeting>' -Description "A & B`n<script>" -AppUrl 'https://rise.example.com/test/apps/lab'
        $html | Should -Be '<p><strong>&lt;Meeting&gt;</strong></p><p>A &amp; B</p><p>&lt;script&gt;</p><p><a href="https://rise.example.com/test/apps/lab">Open in Rise-X</a></p>'
        (ConvertTo-CalendarHtmlBody -Subject 'Meeting' -Description '') | Should -Not -Match '<a '
    }
    It 'rejects unsafe invitation URL <Url>' -ForEach @(
        @{Url='http://rise.example.com'}, @{Url='https://u:p@rise.example.com'},
        @{Url='https://rise.example.com/?token=x'}, @{Url='https://rise.example.com/#x'},
        @{Url='javascript:alert(1)'}, @{Url='https://localhost'}, @{Url='https://127.0.0.1'}, @{Url='https://app.local'}
    ) { {Assert-CalendarAppUrl $Url} | Should -Throw }
    It 'keeps Teams fields and body out of ordinary updates and requires HTML creation' {
        $config=New-TestConfig;$templates=Get-CalendarTemplate $config
        $create=($templates | Where-Object alias -eq calendarCreate).endpoint.request
        $create.body.contentType | Should -Be 'HTML'
        ($create | ConvertTo-Json -Depth 20) | Should -Not -Match 'isOnlineMeeting|onlineMeetingProvider'
        foreach ($alias in @('calendarUpdate','calendarSetAttendees')) {
            $request=($templates | Where-Object alias -eq $alias).endpoint.request
            $request.ContainsKey('body') | Should -BeFalse
            ($request | ConvertTo-Json -Depth 20) | Should -Not -Match 'isOnlineMeeting|onlineMeetingProvider'
        }
    }
}

Describe 'HTML lifecycle through the script' {
    BeforeEach {
        $config=New-TestConfig;$state=Read-CalendarState "$TestDrive/state.json" $config
        $state.resources.integration=@{id='integration';created=$false}
        foreach ($alias in @('calendarQuery','calendarGet','calendarCreate','calendarUpdate','calendarSetAttendees','calendarCancel')) {$state.endpoints[$alias]=@{id=$alias;created=$false}}
        Save-CalendarState $state "$TestDrive/state.json"
        $token=ConvertTo-SecureString 'fixture-token' -AsPlainText -Force
        $secret=ConvertTo-SecureString 'fixture-secret' -AsPlainText -Force
        $global:htmlTestEvents=@{};$global:htmlTestWrites=@();$global:htmlTestCounter=0
        Mock Invoke-RestMethod {
            param($Uri,$Method,$Body)
            if ($Uri -match 'login.microsoftonline.com') {return [pscustomobject]@{access_token='fixture-access'}}
            if ($Uri -match 'excluded%40example.com') {
                $response=[System.Net.Http.HttpResponseMessage]::new([System.Net.HttpStatusCode]::Forbidden)
                throw [Microsoft.PowerShell.Commands.HttpResponseException]::new('denied',$response)
            }
            if ($Uri -match 'graph.microsoft.com') {return [pscustomobject]@{id='calendar';allowedOnlineMeetingProviders=@()}}
            $alias=($Uri -split '/')[-2];$data=($Body | ConvertFrom-Json).data
            $event=$null
            if ($alias -eq 'calendarQuery') {return [pscustomobject]@{success=$true;statusCode=200;items=@($global:htmlTestEvents.Values)}}
            if ($alias -ne 'calendarGet') {$global:htmlTestWrites+=@{alias=$alias;data=$data}}
            if ($alias -eq 'calendarCreate') {
                $global:htmlTestCounter++;$id='event-'+$global:htmlTestCounter
                $event=[pscustomobject]@{id=$id;subject=$data.subject;body=[pscustomobject]@{contentType='html';content=$data.description};transactionId=$data.transactionId;isOrganizer=$true;type='singleInstance';attendees=@($data.attendees)}
                $global:htmlTestEvents[$id]=$event
            } else {$event=$global:htmlTestEvents[$data.eventId]}
            if ($alias -eq 'calendarSetAttendees') {$event.attendees=@($data.attendees)}
            if ($alias -eq 'calendarUpdate') {$event.subject=$data.subject}
            if ($alias -eq 'calendarCancel') {
                $global:htmlTestEvents.Remove($data.eventId)
                return [pscustomobject]@{success=$true;statusCode=202;items=@()}
            }
            return [pscustomobject]@{success=$true;statusCode=200;items=@($event)}
        }
    }
    AfterEach {Remove-Variable -Name htmlTestEvents,htmlTestWrites,htmlTestCounter -Scope Global -ErrorAction SilentlyContinue}
    It 'creates two HTML entries, preserves content across edits and cleans up the same recorded IDs' {
        & "$scripts/Test-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -RiseXToken $token -ClientSecret $secret -Scenario Lifecycle -RecipientA 'a@controlled.test' -RecipientB 'b@controlled.test' -AppUrl 'https://rise.controlled.test/test/apps/lab' -Description "First & second`n<literal>" -Confirm:$false
        $creates=@($global:htmlTestWrites | Where-Object alias -eq calendarCreate)
        $creates | Should -HaveCount 2
        foreach ($create in $creates) {
            $create.data.description | Should -Match '<p>First &amp; second</p><p>&lt;literal&gt;</p>'
            $create.data.description | Should -Match 'Open in Rise-X'
            ($create.data | ConvertTo-Json -Depth 20) | Should -Not -Match 'isOnlineMeeting|onlineMeetingProvider'
        }
        $global:htmlTestEvents.Count | Should -Be 0
        $saved=Read-CalendarState "$TestDrive/state.json" $config
        $saved.runs | Should -HaveCount 2
        @($saved.runs | Where-Object outcome -ne cancelled) | Should -HaveCount 0
        @($global:htmlTestWrites | Where-Object alias -eq calendarCancel | ForEach-Object {$_.data.eventId}) | Should -Be @('event-1','event-2')
    }
    It 'rejects a placeholder deployment URL before any live request' {
        {& "$scripts/Test-CalendarConnection.ps1" -ConfigPath "$TestDrive/config.json" -StatePath "$TestDrive/state.json" -RiseXToken $token -ClientSecret $secret -Scenario Lifecycle -RecipientA 'a@controlled.test' -RecipientB 'b@controlled.test' -AppUrl 'https://rise.example.com/test/apps/lab' -Confirm:$false} | Should -Throw '*real deployed app URL*'
        Should -Invoke Invoke-RestMethod -Times 0
    }
}
