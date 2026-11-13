#requires -Version 7.0
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
function Read-CalendarConfig {
    param([Parameter(Mandatory)][string]$Path)
    $json = Get-Content -LiteralPath $Path -Raw
    $schema = Join-Path $PSScriptRoot '../templates/connection.schema.json'
    if (-not (Test-Json -Json $json -SchemaFile $schema -ErrorAction Stop)) { throw 'Invalid calendar configuration.' }
    $config = $json | ConvertFrom-Json -AsHashtable
    $uri = [uri]$config.riseXBaseUrl
    if ($uri.Scheme -ne 'https' -or $uri.UserInfo -or $uri.Query -or $uri.Fragment -or $uri.AbsolutePath -ne '/') { throw 'Rise-X base URL must be an HTTPS origin without credentials, path, query or fragment.' }
    if ($config.mailbox -eq $config.excludedMailbox) { throw 'Allowed and excluded mailboxes must differ.' }
    if (-not $config.ContainsKey('calendarId')) { $config.calendarId='' }
    if (-not $config.ContainsKey('clientId')) { $config.clientId='' }
    return $config
}
function Read-CalendarState {
    param([string]$Path, [hashtable]$Config)
    $binding = "$($Config.tenantId)|$($Config.mailbox)|$($Config.calendarId)|$($Config.riseXBaseUrl)|$($Config.ecosystem)|$($Config.environment)"
    if (Test-Path -LiteralPath $Path) {
        $state = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json -AsHashtable
        if ($state.version -ne 1 -or $state.binding -ne $binding) { throw 'State target does not match configuration. Use the original config or a new state file.' }
        return $state
    }
    return @{ version = 1; binding = $binding; resources = @{}; endpoints = @{}; runs = @(); evidence = @() }
}
function Save-CalendarState {
    param([hashtable]$State,[string]$Path)
    # Atomic replacement; only identifiers, ownership and sanitized outcomes belong here.
    $full = [IO.Path]::GetFullPath($Path)
    [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($full)) | Out-Null
    $temporary = "$full.$([guid]::NewGuid().ToString('N')).tmp"
    try {
        $State | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $temporary -Encoding utf8
        [IO.File]::Move($temporary, $full, $true)
    } finally {
        if ([IO.File]::Exists($temporary)) { [IO.File]::Delete($temporary) }
    }
}
function Get-PlainSecret {
    param([Parameter(Mandatory)][securestring]$Secret)
    return [System.Net.NetworkCredential]::new('', $Secret).Password
}
function Test-CalendarInheritedAuthentication {
    param([AllowNull()][object]$Authentication)
    if ($null -eq $Authentication) {return $true}
    # Rise-X serializes its default endpoint auth as None plus empty fields.
    # Scheme and other nonempty fields can affect requests even with Type=None,
    # so accept only the known default shape, not every None object.
    $settings=@{}
    if ($Authentication -is [System.Collections.IDictionary]) {
        foreach ($key in $Authentication.Keys) {$settings[$key]=$Authentication[$key]}
    } elseif ($Authentication -is [pscustomobject]) {
        foreach ($property in $Authentication.PSObject.Properties) {$settings[$property.Name]=$property.Value}
    } else {return $false}
    if (-not $settings.ContainsKey('type') -or $settings['type'] -cne 'None') {return $false}
    foreach ($key in $settings.Keys) {
        $value=$settings[$key]
        switch ($key) {
            'type' {continue}
            'grantType' {if ($null -ne $value -and $value -cne '' -and $value -cne 'client_credentials') {return $false}}
            'parameters' {
                if ($null -eq $value) {continue}
                if ($value -is [System.Collections.IDictionary]) {if ($value.Count -gt 0) {return $false}}
                elseif ($value -is [pscustomobject]) {if (@($value.PSObject.Properties).Count -gt 0) {return $false}}
                else {return $false}
            }
            {$_ -in @('tokenUrl','clientId','secret','target','scope','scheme')} {if ($null -ne $value -and $value -cne '') {return $false}}
            default {return $false}
        }
    }
    return $true
}
function Get-CalendarOnlineMeetingCapability {
    param([Parameter(Mandatory)][object]$Calendar)
    # An absent/malformed field is unknown, not evidence that Teams is supported.
    $property=$Calendar.PSObject.Properties['allowedOnlineMeetingProviders']
    if (-not $property -or $null -eq $property.Value -or $property.Value -isnot [array]) {
        return @{teamsStatus='unknown';allowedOnlineMeetingProviders=@()}
    }
    $providers=@($property.Value)
    if (@($providers | Where-Object {$_ -isnot [string] -or [string]::IsNullOrWhiteSpace($_)}).Count) {
        return @{teamsStatus='unknown';allowedOnlineMeetingProviders=@()}
    }
    $status=if ($providers -ccontains 'teamsForBusiness') {'supported'} else {'unsupported'}
    return @{teamsStatus=$status;allowedOnlineMeetingProviders=$providers}
}
function Assert-CalendarAppUrl {
    param([Parameter(Mandatory)][string]$AppUrl)
    $uri=$null
    if (-not [uri]::TryCreate($AppUrl,[UriKind]::Absolute,[ref]$uri) -or $uri.Scheme -ne 'https' -or $uri.UserInfo -or $uri.Query -or $uri.Fragment -or $uri.HostNameType -ne [UriHostNameType]::Dns -or $uri.Host -notmatch '\.' -or $uri.Host -match '\.(local|localhost)$') {
        throw 'AppUrl must be a deployed HTTPS app URL without credentials, query parameters or fragment.'
    }
    return $uri.AbsoluteUri
}
function ConvertTo-CalendarHtmlBody {
    param([string]$Subject,[string]$Description,[string]$AppUrl)
    $content='<p><strong>'+[System.Net.WebUtility]::HtmlEncode($Subject)+'</strong></p>'
    foreach ($line in ($Description -split '\r?\n')) {$content+='<p>'+[System.Net.WebUtility]::HtmlEncode($line)+'</p>'}
    if ($AppUrl) {$content+='<p><a href="'+[System.Net.WebUtility]::HtmlEncode((Assert-CalendarAppUrl $AppUrl))+'">Open in Rise-X</a></p>'}
    return $content
}
function Invoke-CalendarHttp {
    param([Parameter(Mandatory)][string]$Uri,[string]$Method = 'GET',[hashtable]$Headers = @{},[object]$Body,[string]$ContentType = 'application/json',[ValidateSet('Standard','ReadOnly','Token')][string]$RequestKind='Standard')
    $requestArguments = @{ Uri=$Uri; Method=$Method; Headers=$Headers; ErrorAction='Stop'; MaximumRedirection=0; TimeoutSec=60 }
    if ($null -ne $Body) { $requestArguments.Body = if ($ContentType -eq 'application/json') { $Body | ConvertTo-Json -Depth 40 -Compress } else { $Body }; $requestArguments.ContentType=$ContentType }
    try { return Invoke-RestMethod @requestArguments } catch {
        $status = 0
        if ($_.Exception.PSObject.Properties['Response'] -and $_.Exception.Response) { $status = [int]$_.Exception.Response.StatusCode }
        # Only translate a known fixed API message; never echo arbitrary response text,
        # headers, URI or payload because these may contain credentials or recipients.
        $hint=''
        if ($_.ErrorDetails -and $_.ErrorDetails.Message) {
            try {
                $problem=$_.ErrorDetails.Message | ConvertFrom-Json -AsHashtable -ErrorAction Stop
                if ($status -eq 400 -and $problem -is [hashtable] -and $problem['message'] -ceq 'Environment header is missing in the request.') {
                    $hint=' Rise-X could not resolve the ecosystem. Check the exact ecosystem name and API environment in connection.json.'
                }
                if ($RequestKind -eq 'Token' -and $problem -is [hashtable]) {
                    if ($problem['error'] -in @('invalid_request','invalid_client','invalid_grant','invalid_scope','unauthorized_client','unsupported_grant_type','invalid_resource','temporarily_unavailable')) {
                        $hint+=' OAuth error: '+$problem['error']+'.'
                    }
                    $codes=@($problem['error_codes'] | Where-Object { ($_ -is [int] -or $_ -is [long]) -and $_ -gt 0 -and $_ -lt 1000000000 } | Select-Object -First 3)
                    if ($codes.Count) { $hint+=' Entra diagnostic codes: '+($codes -join ', ')+'.' }
                }
            } catch { $hint='' } # Unrecognised provider responses remain redacted.
        }
        $recovery=if ($RequestKind -eq 'Token') {'Token acquisition did not modify calendar entries.'} elseif ($RequestKind -eq 'ReadOnly' -or $Method -in @('GET','HEAD')) {'This read request made no changes.'} else {'Reconcile writes before retrying.'}
        throw [InvalidOperationException]::new("Calendar HTTP request failed (status $status).$hint $recovery")
    }
}
function Invoke-RiseCalendarApi {
    param([hashtable]$Config,[securestring]$Token,[string]$Path,[string]$Method='GET',[object]$Body,[switch]$ReadOnlyOperation)
    $headers = @{ Authorization="Bearer $(Get-PlainSecret $Token)"; Environment=$Config.ecosystem }
    $requestKind=if ($ReadOnlyOperation) {'ReadOnly'} else {'Standard'}
    try { return Invoke-CalendarHttp -RequestKind $requestKind -Uri ($Config.riseXBaseUrl.TrimEnd('/') + '/api/v4/config/integration' + $Path) -Method $Method -Headers $headers -Body $Body }
    finally { $headers.Clear() }
}
function Get-CalendarTemplate {
    param([hashtable]$Config)
    $templates = Get-Content (Join-Path $PSScriptRoot '../templates/endpoints.json') -Raw | ConvertFrom-Json -AsHashtable
    $user = [uri]::EscapeDataString($Config.mailbox)
    $calendar = if ($Config.calendarId) { '/calendars/' + [uri]::EscapeDataString($Config.calendarId) } else { '/calendar' }
    foreach ($template in $templates) {
        $template.endpoint.endpointUri = $template.endpoint.endpointUri.Replace('__MAILBOX__',$user).Replace('__CALENDAR__',$calendar)
    }
    return $templates
}
function Invoke-CalendarOperation {
    param([hashtable]$Config,[hashtable]$State,[securestring]$Token,[string]$Alias,[hashtable]$Data)
    if (-not $State.endpoints.ContainsKey($Alias) -or -not $State.resources.ContainsKey('integration')) { throw 'Run endpoint setup first.' }
    $id=$State.resources.integration.id; $endpointId=$State.endpoints[$Alias].id
    $result=Invoke-RiseCalendarApi -Config $Config -Token $Token -Path "/$id/endpoint/$endpointId/invoke" -Method POST -Body @{data=$Data} -ReadOnlyOperation:($Alias -in @('calendarQuery','calendarGet'))
    if (-not $result.success -or $result.statusCode -lt 200 -or $result.statusCode -ge 300) { throw "Calendar operation failed (status $($result.statusCode)); verify actual state before retrying." }
    # Empty provider responses (for example cancellation) omit the nullable response field.
    $responseProperty=$result.PSObject.Properties['response']
    if ($responseProperty -and $responseProperty.Value -and $responseProperty.Value.PSObject.Properties['@odata.nextLink']) { throw 'Incomplete calendar page. Narrow the query; no writes are safe from this result.' }
    return $result
}
function Get-CalendarEventResult {
    param([object]$Result)
    $itemsProperty=$Result.PSObject.Properties['items']
    $responseProperty=$Result.PSObject.Properties['response']
    $calendarEvent=$null
    if ($itemsProperty -and @($itemsProperty.Value).Count) { $calendarEvent=@($itemsProperty.Value)[0] }
    elseif ($responseProperty) { $calendarEvent=$responseProperty.Value }
    if (-not $calendarEvent -or -not $calendarEvent.PSObject.Properties['id'] -or -not $calendarEvent.id) { throw 'Write result has no event ID. Keep the transaction ID and reconcile before retrying.' }
    return $calendarEvent
}
