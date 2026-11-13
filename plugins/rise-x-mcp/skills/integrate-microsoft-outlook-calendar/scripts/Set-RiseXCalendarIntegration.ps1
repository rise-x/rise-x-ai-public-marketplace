#requires -Version 7.0
[CmdletBinding(SupportsShouldProcess, ConfirmImpact='High')]
param([Parameter(Mandatory)][string]$ConfigPath,[Parameter(Mandatory)][string]$StatePath,[Parameter(Mandatory)][string]$ManifestPath,[securestring]$RiseXToken,[securestring]$ClientSecret,[switch]$UpgradeHtmlTemplate)
. (Join-Path $PSScriptRoot 'Calendar.Common.ps1')
$config=Read-CalendarConfig $ConfigPath;$state=Read-CalendarState $StatePath $config
$templates=Get-CalendarTemplate $config
if ($WhatIfPreference) { $templates | ForEach-Object { Write-Output "Preview endpoint: $($_.alias) [$($_.endpoint.method)]. Preserve unrelated endpoints. No requests or files written." };if ($UpgradeHtmlTemplate) {Write-Output 'Preview upgrade: recognized calendarCreate body.contentType Text -> HTML only, preserving endpoint ID and credentials. Callers must send escaped/composed HTML. Live differences are checked during apply.'};return }
if (-not $RiseXToken) { $RiseXToken=Read-Host 'Rise-X bearer token (owner/orchestrator in the test ecosystem)' -AsSecureString }
$integration=$null
if ($state.resources.ContainsKey('integration')) {
    try { $integration=Invoke-RiseCalendarApi -Config $config -Token $RiseXToken -Path "/$($state.resources.integration.id)" }
    catch {
        if ($_.Exception.Message -notmatch '\(status 404\)' -or -not $state.resources.integration.created) { throw }
        # Only a previously recorded creation may resume after confirmed absence.
    }
    if ($integration -and $integration.name -ne $config.integrationName) { throw 'Recorded integration was renamed. Resolve the configuration explicitly; ownership and existing settings are preserved.' }
}
if (-not $integration) {
    $existing=@(Invoke-RiseCalendarApi -Config $config -Token $RiseXToken -Path ('?name='+[uri]::EscapeDataString($config.integrationName)))
    if ($existing.Count -gt 1) { throw 'Ambiguous integration name.' }
    if ($existing.Count) { $integration=$existing[0] }
}
$clientId=if ($config.clientId) {$config.clientId} elseif ($state.ContainsKey('clientId')) {$state.clientId} else {throw 'Client ID missing. Run Microsoft setup or supply clientId.'}
if ($integration) {
    if (-not $integration.PSObject.Properties['maxRetries'] -or $null -eq $integration.maxRetries -or $integration.maxRetries -ne 0) { throw 'Integration must explicitly set maxRetries=0; automatic write retries prevent reliable reconciliation. Review its configuration before using this recipe.' }
    if ($integration.baseUri.TrimEnd('/') -ne 'https://graph.microsoft.com/v1.0' -or $integration.authentication.clientId -ne $clientId -or $integration.authentication.tokenUrl -ne "https://login.microsoftonline.com/$($config.tenantId)/oauth2/v2.0/token") { throw 'Existing integration targets a different connection; select a dedicated integration.' }
    if ($integration.authentication.type -ne 'ClientCredentials' -or @($integration.parameters | Where-Object { $_.name -in @('from','to','eventId','subject','description','start','end','transactionId','attendees','comment') }).Count) { throw 'Existing integration authentication or static parameters conflict with the recipe.' }
    if ($ClientSecret) { throw 'Existing credentials are preserved. Rotate through the approved secret-management workflow, not this setup script.' }
    if ($state.resources.ContainsKey('integration') -and $state.resources.integration.id -ne $integration.id) { throw 'Integration differs from recorded identity.' }
    if (-not $state.resources.ContainsKey('integration')) { $state.resources.integration=@{id=$integration.id;created=$false} }
} else {
    if (-not $PSCmdlet.ShouldProcess($config.integrationName,'Create live Rise-X integration')) { return }
    if (-not $ClientSecret) { $ClientSecret=Read-Host 'Microsoft client secret (stored as a Rise-X secret parameter)' -AsSecureString }
    $id=if ($state.resources.ContainsKey('integration')) {$state.resources.integration.id} else {[guid]::NewGuid().ToString()}
    # Save stable identity before posting: reruns reconcile the same object after a lost response.
    $state.resources.integration=@{id=$id;created=$true};Save-CalendarState $state $StatePath
    $body=@{id=$id;name=$config.integrationName;baseUri='https://graph.microsoft.com/v1.0';maxRetries=0;parameters=@(@{name='graphClientSecret';value=(Get-PlainSecret $ClientSecret);isSecret=$true});authentication=@{type='ClientCredentials';tokenUrl="https://login.microsoftonline.com/$($config.tenantId)/oauth2/v2.0/token";clientId=$clientId;secret='{$.graphClientSecret}';grantType='client_credentials';scope='https://graph.microsoft.com/.default';scheme='Bearer'}}
    try { $integration=Invoke-RiseCalendarApi -Config $config -Token $RiseXToken -Path '' -Method POST -Body $body } finally {$body.parameters[0].value=$null}
    if ($integration.id -ne $id) { throw 'Unexpected integration ID returned; stop and reconcile.' }
}
Save-CalendarState $state $StatePath
$dependencies=@{}
foreach ($template in $templates) {
    $found=@($integration.endpoints | Where-Object name -eq $template.endpoint.name)
    if ($found.Count -gt 1) {throw 'Ambiguous endpoint name.'}
    $prior=if ($found.Count) {$found[0]} else {$null}
    if ($prior) {
        $upgrade=$false
        # Refuse to overwrite unfamiliar endpoints, headers/auth or parameters; preserving them is safer than replacing them.
        foreach ($field in @('method','endpointUri','responseValuePath')) { if ([string]$prior.$field -ne [string]$template.endpoint[$field]) {throw "Existing endpoint contract differs: $($template.alias). Review before replacing."} }
        # The API may omit nullable request fields on GET endpoints. Normalize only
        # the absent field to null; a required write template must still match.
        $requestProperty=$prior.PSObject.Properties['request']
        $priorRequest=if ($requestProperty) {$requestProperty.Value} else {$null}
        if (($priorRequest | ConvertTo-Json -Depth 30 -Compress) -ne ($template.endpoint.request | ConvertTo-Json -Depth 30 -Compress)) {
            $candidate=$priorRequest | ConvertTo-Json -Depth 30 | ConvertFrom-Json -AsHashtable
            if ($template.alias -eq 'calendarCreate' -and $candidate -and $candidate.ContainsKey('body') -and $candidate.body -is [System.Collections.IDictionary] -and $candidate.body.contentType -ceq 'Text') {
                $candidate.body.contentType='HTML'
                $upgrade=($candidate | ConvertTo-Json -Depth 30 -Compress) -eq ($template.endpoint.request | ConvertTo-Json -Depth 30 -Compress)
            }
            if (-not $upgrade) {throw 'Existing request template differs. Review before replacing.'}
            if (-not $UpgradeHtmlTemplate) {throw 'Recognized Text create template. Review caller migration, then rerun with -UpgradeHtmlTemplate to switch to HTML without changing IDs or credentials.'}
        }
        if ($prior.headers.Prefer -ne 'outlook.timezone="UTC"') {throw 'Endpoint must request UTC response times.'}
        $parameterProperty=$prior.PSObject.Properties['parameters']
        if ($parameterProperty -and $null -ne $parameterProperty.Value -and @($parameterProperty.Value).Count -gt 0) {throw "Existing endpoint has additional connection settings ($($template.alias): parameters). Review rather than overwrite."}
        $authenticationProperty=$prior.PSObject.Properties['authentication']
        if ($authenticationProperty -and -not (Test-CalendarInheritedAuthentication $authenticationProperty.Value)) {throw "Existing endpoint has additional connection settings ($($template.alias): authentication override). Review rather than overwrite."}
        if (@($prior.headers.PSObject.Properties.Name | Where-Object {$_ -ne 'Prefer'}).Count -gt 0) {throw "Existing endpoint has additional connection settings ($($template.alias): headers). Review rather than overwrite."}
        $endpointId=$prior.id
        if ($state.endpoints.ContainsKey($template.alias) -and $state.endpoints[$template.alias].id -ne $endpointId) {throw 'Endpoint differs from recorded identity.'}
        if (-not $state.endpoints.ContainsKey($template.alias)) {$state.endpoints[$template.alias]=@{id=$endpointId;created=$false}}
        if ($upgrade) {
            if (-not $PSCmdlet.ShouldProcess($template.alias,'Upgrade create body.contentType from Text to HTML; all callers must send composed HTML')) {return}
            # Copy the complete endpoint to preserve unrelated metadata. Never rewrite authentication.
            $replacement=$prior | ConvertTo-Json -Depth 40 | ConvertFrom-Json -AsHashtable
            $replacement.request.body.contentType='HTML'
            $integration=Invoke-RiseCalendarApi -Config $config -Token $RiseXToken -Path "/$($integration.id)/endpoint" -Method POST -Body @{endpoint=$replacement}
            $verified=@($integration.endpoints | Where-Object id -eq $endpointId)
            if ($verified.Count -ne 1 -or -not $verified[0].PSObject.Properties['request'] -or ($verified[0].request | ConvertTo-Json -Depth 30 -Compress) -ne ($template.endpoint.request | ConvertTo-Json -Depth 30 -Compress)) {throw 'HTML upgrade read-back did not match. Retain endpoint identity and reconcile before retrying.'}
        }
    } else {
        if (-not $PSCmdlet.ShouldProcess($template.alias,'Add live integration endpoint')) {return}
        $endpointId=if ($state.endpoints.ContainsKey($template.alias)) {$state.endpoints[$template.alias].id} else {[guid]::NewGuid().ToString()}
        $template.endpoint.id=$endpointId;$state.endpoints[$template.alias]=@{id=$endpointId;created=$true};Save-CalendarState $state $StatePath
        $integration=Invoke-RiseCalendarApi -Config $config -Token $RiseXToken -Path "/$($integration.id)/endpoint" -Method POST -Body @{endpoint=$template.endpoint}
        if (-not @($integration.endpoints | Where-Object id -eq $endpointId).Count) {throw 'Endpoint ID not returned. Reconcile before retrying.'}
    }
    Save-CalendarState $state $StatePath
    $dependencies[$template.alias]=@{kind='integration';ids=@{ $config.environment=@{integrationId=$integration.id;endpointId=$endpointId}}}
}
$manifest=if (Test-Path $ManifestPath) {Get-Content $ManifestPath -Raw | ConvertFrom-Json -AsHashtable} else {@{environments=@();dependencies=@{}}}
foreach ($alias in $dependencies.Keys) {
    if ($manifest.dependencies.ContainsKey($alias)) {
        if ($manifest.dependencies[$alias].kind -ne 'integration') {throw 'Manifest alias is already used by another dependency kind.'}
        $manifest.dependencies[$alias].ids[$config.environment]=$dependencies[$alias].ids[$config.environment]
    } else {$manifest.dependencies[$alias]=$dependencies[$alias]}
}
$manifest.environments=@(@($manifest.environments)+$config.environment | Select-Object -Unique)
if ($PSCmdlet.ShouldProcess($ManifestPath,'Export secret-free dependency bindings')) { $manifest | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $ManifestPath -Encoding utf8 }
Write-Output 'Integration and six endpoints ready. Validate the manifest and rebuild for the configured environment; live acceptance remains pending.'
