#requires -Version 7.0
[CmdletBinding(SupportsShouldProcess, ConfirmImpact='High')]
param([Parameter(Mandatory)][string]$ConfigPath,[Parameter(Mandatory)][string]$StatePath,[securestring]$RiseXToken,[switch]$RemoveConfiguration,[switch]$RemoveProvisioning)
. (Join-Path $PSScriptRoot 'Calendar.Common.ps1')
$config=Read-CalendarConfig $ConfigPath;$state=Read-CalendarState $StatePath $config
if ($WhatIfPreference) {Write-Output 'Preview: cancel recorded demo events only; optionally remove recorded created configuration/provisioning resources. Reused resources are preserved. No HTTP calls or files written.';return}
if (-not $RiseXToken) {$RiseXToken=Read-Host 'Rise-X bearer token' -AsSecureString}
foreach ($run in $state.runs) {
    if ($run.outcome -eq 'cancelled') {continue}
    if (-not $run.eventId) {throw 'A create has no recorded event ID. Reconcile its transaction ID before cleanup.'}
    try { $calendarEvent=Get-CalendarEventResult (Invoke-CalendarOperation $config $state $RiseXToken 'calendarGet' @{eventId=$run.eventId}) }
    catch {
        if ($_.Exception.Message -match '\(status 404\)' -and $run.operation -eq 'calendarCancel') { $run.outcome='cancelled';Save-CalendarState $state $StatePath;continue }
        throw
    }
    if (-not $calendarEvent.isOrganizer -or $calendarEvent.type -ne 'singleInstance' -or $calendarEvent.transactionId -ne $run.transactionId) {throw 'Event ownership verification failed; nothing was removed.'}
    if ($PSCmdlet.ShouldProcess($run.eventId,'Cancel recorded demo event and notify its attendees')) {
        $run.operation='calendarCancel';$run.outcome='unknown';Save-CalendarState $state $StatePath
        Invoke-CalendarOperation $config $state $RiseXToken 'calendarCancel' @{eventId=$run.eventId;comment='Calendar demo cleanup'} | Out-Null
        $confirmed=$false
        try {
            $after=Get-CalendarEventResult (Invoke-CalendarOperation $config $state $RiseXToken 'calendarGet' @{eventId=$run.eventId})
            $confirmed=$after.isCancelled
        } catch {if ($_.Exception.Message -match '\(status 404\)') {$confirmed=$true} else {throw}}
        if (-not $confirmed) {throw 'Cancellation response received, but resulting state is unconfirmed. Keep the record and reconcile.'}
        $run.outcome='cancelled';Save-CalendarState $state $StatePath
    }
}
if (@($state.runs | Where-Object outcome -ne 'cancelled').Count) {throw 'Unresolved events remain; preserve integration access for reconciliation.'}
if ($RemoveConfiguration -and $state.resources.ContainsKey('integration')) {
    $id=$state.resources.integration.id
    $integration=Invoke-RiseCalendarApi -Config $config -Token $RiseXToken -Path "/$id"
    foreach ($entry in @($state.endpoints.GetEnumerator())) {
        if ($entry.Value.created -and @($integration.endpoints | Where-Object id -eq $entry.Value.id).Count -and $PSCmdlet.ShouldProcess($entry.Key,'Remove recorded demo-created endpoint')) {
            $integration=Invoke-RiseCalendarApi -Config $config -Token $RiseXToken -Path "/$id/endpoint/$($entry.Value.id)" -Method DELETE
            $state.endpoints.Remove($entry.Key);Save-CalendarState $state $StatePath
        }
    }
    if ($state.resources.integration.created -and -not @($integration.endpoints).Count -and $PSCmdlet.ShouldProcess($id,'Remove now-empty demo-created integration')) {
        Invoke-RiseCalendarApi -Config $config -Token $RiseXToken -Path "/$id" -Method DELETE | Out-Null
        $state.resources.Remove('integration');Save-CalendarState $state $StatePath
    }
}
if ($RemoveProvisioning) {
    if ($state.resources.ContainsKey('integration')) {throw 'Integration is retained. Review dependencies before removing its Microsoft access.'}
    Import-Module Microsoft.Graph.Applications;Import-Module ExchangeOnlineManagement
    if ((Get-MgContext).TenantId -ne $config.tenantId -or -not @(Get-ConnectionInformation | Where-Object {$_.TenantID -eq $config.tenantId -and $_.State -eq 'Connected'}).Count) {throw 'Connect both administrator sessions to the configured tenant.'}
    foreach ($kind in @('credential','assignment','scope','exchangePrincipal','servicePrincipal','application','mailbox')) {
        if (-not $state.resources.ContainsKey($kind) -or -not $state.resources[$kind].created) {continue}
        $id=$state.resources[$kind].id
        if ($PSCmdlet.ShouldProcess($id,"Remove recorded demo-created $kind")) {
            switch ($kind) {
                credential {Remove-MgApplicationPassword -ApplicationId $state.resources.application.id -KeyId $id}
                assignment {Remove-ManagementRoleAssignment -Identity $id -Confirm:$false}
                scope {Remove-ManagementScope -Identity $id -Confirm:$false}
                exchangePrincipal {Remove-ServicePrincipal -Identity $id -Confirm:$false}
                servicePrincipal {Remove-MgServicePrincipal -ServicePrincipalId $id}
                application {Remove-MgApplication -ApplicationId $id}
                mailbox {Remove-Mailbox -Identity $id -Confirm:$false}
            }
            $state.resources.Remove($kind);Save-CalendarState $state $StatePath
        }
    }
}
Write-Output 'Selected recorded resources processed; reused resources preserved. Verify residual resources and recipient cancellation messages.'
