#requires -Version 7.0
[CmdletBinding(SupportsShouldProcess, ConfirmImpact='High')]
param([Parameter(Mandatory)][string]$ConfigPath,[Parameter(Mandatory)][string]$StatePath,[ValidateSet('ReadOnly','Lifecycle')][string]$Scenario='ReadOnly',[securestring]$RiseXToken,[securestring]$ClientSecret,[string]$RecipientA,[string]$RecipientB,[string]$CapabilitiesPath,[string]$AppUrl,[string]$Description='Temporary calendar integration acceptance item')
. (Join-Path $PSScriptRoot 'Calendar.Common.ps1')
$config=Read-CalendarConfig $ConfigPath;$state=Read-CalendarState $StatePath $config
if ($WhatIfPreference) { Write-Output "Preview: $Scenario connection checks. Lifecycle sends invitations, attendee updates and cancellations only when explicitly selected. No HTTP requests, credentials, or files written.";return }
if ($CapabilitiesPath -and ([IO.Path]::GetFullPath($CapabilitiesPath) -in @([IO.Path]::GetFullPath($ConfigPath),[IO.Path]::GetFullPath($StatePath)))) {throw 'Use a separate capabilities output file; configuration and recovery state must be preserved.'}
if ($Scenario -eq 'Lifecycle') {
    if ($AppUrl) {
        $AppUrl=Assert-CalendarAppUrl $AppUrl
        if (([uri]$AppUrl).Host -match '(^|\.)example\.(com|net|org)$') {throw 'Supply the real deployed app URL for this ecosystem, not an example URL.'}
    }
    if (-not $RecipientA -or -not $RecipientB -or $RecipientA -eq $RecipientB) {throw 'Supply two different controlled individual recipients for live lifecycle testing.'}
    foreach ($recipient in @($RecipientA,$RecipientB)) {
        if ($recipient -notmatch '^[^\s@]+@[^\s@]+\.[^\s@]+$') {throw 'Invalid recipient address.'}
        if ($recipient -match '@(?:[^@]+\.)?example\.(com|net|org)$') {throw 'Replace example-domain placeholders with two real test inboxes you control before running Lifecycle.'}
    }
    if (@($state.runs | Where-Object outcome -eq 'unknown').Count) {throw 'A previous write is uncertain. Inspect its event or transaction ID and reconcile the state before another lifecycle run.'}
    if (-not $PSCmdlet.ShouldProcess($config.mailbox,'Run invite lifecycle and send meeting notifications to the supplied test recipients')) {return}
}
if (-not $ClientSecret) {$ClientSecret=Read-Host 'Microsoft application client secret' -AsSecureString}
if (-not $RiseXToken) {$RiseXToken=Read-Host 'Rise-X bearer token' -AsSecureString}
$clientId=if ($config.clientId) {$config.clientId} else {$state.clientId}
$stage='Microsoft token acquisition'
Write-Information ('Checking: '+$stage) -InformationAction Continue
$body=@{client_id=$clientId;client_secret=(Get-PlainSecret $ClientSecret);grant_type='client_credentials';scope='https://graph.microsoft.com/.default'}
try {$token=Invoke-CalendarHttp -Uri "https://login.microsoftonline.com/$($config.tenantId)/oauth2/v2.0/token" -Method POST -ContentType 'application/x-www-form-urlencoded' -Body $body -RequestKind Token} catch {throw "Connection check failed during ${stage}: $($_.Exception.Message)"} finally {$body.Clear()}
$headers=@{Authorization="Bearer $($token.access_token)"};$token=$null
try {
    $stage='Microsoft allowed mailbox read'
    Write-Information ('Checking: '+$stage) -InformationAction Continue
    $calendarPath=if ($config.calendarId) {'/calendars/'+[uri]::EscapeDataString($config.calendarId)} else {'/calendar'}
    $allowed=Invoke-CalendarHttp -Uri ('https://graph.microsoft.com/v1.0/users/'+[uri]::EscapeDataString($config.mailbox)+$calendarPath) -Headers $headers
    if (-not $allowed.id) {throw 'Allowed mailbox check returned no calendar.'}
    $capability=Get-CalendarOnlineMeetingCapability $allowed
    $stage='Microsoft excluded mailbox denial check'
    Write-Information ('Checking: '+$stage) -InformationAction Continue
    $denied=$false
    try {Invoke-CalendarHttp -Uri ('https://graph.microsoft.com/v1.0/users/'+[uri]::EscapeDataString($config.excludedMailbox)+'/calendar') -Headers $headers | Out-Null}
    catch {if ($_.Exception.Message -match '\(status 403\)') {$denied=$true} else {throw 'Excluded mailbox check did not return the expected 403. Verify that the mailbox exists and retry after propagation.'}}
    if (-not $denied) {throw 'Excluded mailbox is accessible. Stop: review additive grants.'}
} catch {throw "Connection check failed during ${stage}: $($_.Exception.Message)"} finally {$headers.Clear()}
[DateTimeOffset]$from=[DateTime]::UtcNow.Date.AddDays(1);$to=$from.AddDays(1)
$data=@{from=$from.ToString('o');to=$to.ToString('o')}
$stage='Rise-X calendarQuery endpoint invocation'
Write-Information ('Checking: '+$stage) -InformationAction Continue
try {Invoke-CalendarOperation $config $state $RiseXToken 'calendarQuery' $data | Out-Null}
catch {throw "Connection check failed during ${stage}: $($_.Exception.Message)"}
Write-Information ('Selected calendar Teams capability: '+$capability.teamsStatus+'. This is a capability check, not proof of meeting creation or delivery.') -InformationAction Continue
if ($CapabilitiesPath -and $PSCmdlet.ShouldProcess($CapabilitiesPath,'Export secret-free selected-calendar capability snapshot')) {
    $endpointBindings=@{}
    foreach ($alias in $state.endpoints.Keys) {$endpointBindings[$alias]=$state.endpoints[$alias].id}
    $snapshot=@{version=1;binding=$state.binding;clientId=$clientId;environment=$config.environment;ecosystem=$config.ecosystem;calendarId=$allowed.id;integrationId=$state.resources.integration.id;endpoints=$endpointBindings;checkedAt=[DateTimeOffset]::UtcNow.ToString('o');teamsStatus=$capability.teamsStatus;allowedOnlineMeetingProviders=$capability.allowedOnlineMeetingProviders}
    Save-CalendarState $snapshot $CapabilitiesPath
}
if ($Scenario -eq 'ReadOnly') {Write-Output 'Allowed mailbox accessible, excluded mailbox denied, Rise-X query succeeded. No writes performed to Microsoft or Rise-X; recipient delivery not tested.';return}
function Invoke-RecordedWrite([string]$Alias,[hashtable]$Data,[hashtable]$Run) {
    $Run.operation=$Alias;$Run.outcome='unknown';Save-CalendarState $state $StatePath
    $result=Invoke-CalendarOperation $config $state $RiseXToken $Alias $Data
    $state.evidence+=@{operation=$Alias;statusCode=$result.statusCode;at=[DateTimeOffset]::UtcNow.ToString('o');outcome='api-success-recipient-unverified'}
    $state.evidence=@($state.evidence | Select-Object -Last 100)
    Save-CalendarState $state $StatePath
    return $result
}
foreach ($kind in @('appointment','invite')) {
    $run=@{transactionId=[guid]::NewGuid().ToString();eventId=$null;outcome='unknown';operation='calendarCreate';kind=$kind}
    $state.runs+=,$run;Save-CalendarState $state $StatePath
    $attendees=if ($kind -eq 'invite') {@(@{emailAddress=@{address=$RecipientA};type='required'})} else {@()}
    $create=@{subject='Rise-X calendar controlled test';description=(ConvertTo-CalendarHtmlBody -Subject 'Rise-X calendar controlled test' -Description $Description -AppUrl $AppUrl);start=$from.AddHours(9).ToString('o');end=$from.AddHours(9.5).ToString('o');transactionId=$run.transactionId;attendees=@($attendees)}
    $calendarEvent=Get-CalendarEventResult (Invoke-RecordedWrite 'calendarCreate' $create $run)
    $run.eventId=$calendarEvent.id;Save-CalendarState $state $StatePath
    $calendarEvent=Get-CalendarEventResult (Invoke-CalendarOperation $config $state $RiseXToken 'calendarGet' @{eventId=$calendarEvent.id})
    if (-not $calendarEvent.isOrganizer -or $calendarEvent.type -ne 'singleInstance') {throw 'Unexpected organizer/event type. Stop and inspect recorded event.'}
    if (-not $calendarEvent.PSObject.Properties['body'] -or -not $calendarEvent.body -or $calendarEvent.body.contentType -ine 'html') {throw 'Created event is not HTML. Retain its recorded ID for cleanup; migrate the create endpoint before another lifecycle run.'}
    $originalBody=$calendarEvent.body.content
    if ($kind -eq 'invite') {
        # Every attendee mutation starts from the latest organizer event and preserves complete recipient objects.
        $all=@($calendarEvent.attendees)
        if (@($all | Where-Object {$_.emailAddress.address -eq $RecipientB}).Count) {throw 'Recipient B already present; inspect before changing.'}
        $all+=@{emailAddress=@{address=$RecipientB};type='required'}
        Invoke-RecordedWrite 'calendarSetAttendees' @{eventId=$calendarEvent.id;attendees=$all} $run | Out-Null
        $calendarEvent=Get-CalendarEventResult (Invoke-CalendarOperation $config $state $RiseXToken 'calendarGet' @{eventId=$calendarEvent.id})
        if (-not @($calendarEvent.attendees | Where-Object {$_.emailAddress.address -eq $RecipientB}).Count) {throw 'Recipient B not present after update.'}
        $remaining=@($calendarEvent.attendees | Where-Object {$_.emailAddress.address -ne $RecipientA})
        Invoke-RecordedWrite 'calendarSetAttendees' @{eventId=$calendarEvent.id;attendees=$remaining} $run | Out-Null
        $calendarEvent=Get-CalendarEventResult (Invoke-CalendarOperation $config $state $RiseXToken 'calendarGet' @{eventId=$calendarEvent.id})
        if (@($calendarEvent.attendees | Where-Object {$_.emailAddress.address -eq $RecipientA}).Count) {throw 'Recipient A remains after removal.'}
        Invoke-RecordedWrite 'calendarUpdate' @{eventId=$calendarEvent.id;subject='Rise-X calendar rescheduled test';start=$from.AddHours(10).ToString('o');end=$from.AddHours(10.5).ToString('o')} $run | Out-Null
        $calendarEvent=Get-CalendarEventResult (Invoke-CalendarOperation $config $state $RiseXToken 'calendarGet' @{eventId=$calendarEvent.id})
        if ($calendarEvent.subject -ne 'Rise-X calendar rescheduled test') {throw 'Event edit not confirmed.'}
        if ($calendarEvent.body.content -cne $originalBody) {throw 'Event body changed during attendee or subject/time updates. Retain event ID and inspect before continuing.'}
    }
    Invoke-RecordedWrite 'calendarCancel' @{eventId=$calendarEvent.id;comment='Controlled calendar lab cleanup'} $run | Out-Null
    $after=Invoke-CalendarOperation $config $state $RiseXToken 'calendarQuery' $data
    if (@($after.items | Where-Object id -eq $calendarEvent.id).Count) {throw 'Cancelled event still appears in the queried range. Reconcile before marking cleanup complete.'}
    $run.outcome='cancelled';Save-CalendarState $state $StatePath
}
Write-Output 'API lifecycle confirmed and created events removed. Recipient invitation/update/removal/cancellation delivery remains UNVERIFIED: inspect both controlled inboxes and record evidence separately.'
