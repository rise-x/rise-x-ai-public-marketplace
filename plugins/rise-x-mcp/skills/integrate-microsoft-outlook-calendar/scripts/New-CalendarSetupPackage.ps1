#requires -Version 7.4
<#
.SYNOPSIS
Builds a customer setup ZIP from an explicit, tenant-free file list.
.DESCRIPTION
Release packages require a trusted code-signing certificate on Windows and a
timestamp server. -Unsigned creates a clearly labelled internal test package.
Never includes .local, tenant bindings, build output or development dependencies.
.EXAMPLE
./scripts/New-CalendarSetupPackage.ps1 -OutputPath .local/calendar-setup.zip -Unsigned
#>
[CmdletBinding(SupportsShouldProcess)]
param(
    [Parameter(Mandatory)][string]$OutputPath,
    [Security.Cryptography.X509Certificates.X509Certificate2]$SigningCertificate,
    [string]$TimestampServer,
    [switch]$Unsigned
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if ($Unsigned -and $SigningCertificate) { throw 'Choose either a signed release or an unsigned internal test package.' }
if (-not $Unsigned -and (-not $IsWindows -or -not $SigningCertificate -or -not $SigningCertificate.HasPrivateKey -or -not $TimestampServer)) {
    throw 'Release packaging requires Windows, a code-signing certificate with its private key, and TimestampServer. Use -Unsigned only for internal tests.'
}
$output = [IO.Path]::GetFullPath($OutputPath)
if ([IO.Path]::GetExtension($output) -ne '.zip') { throw 'OutputPath must end in .zip.' }
if ([IO.File]::Exists($output) -or [IO.File]::Exists("$output.sha256")) { throw 'Choose a new output filename; packages are immutable.' }
if (-not $PSCmdlet.ShouldProcess($output, 'Build the calendar setup package and SHA-256 checksum')) { return }
$root = Split-Path $PSScriptRoot -Parent
$files = @(
    'scripts/Start-CalendarSetup.ps1', 'scripts/Calendar.Setup.ps1', 'scripts/Calendar.Common.ps1',
    'scripts/Initialize-CalendarConnection.ps1', 'scripts/Set-RiseXCalendarIntegration.ps1',
    'scripts/Test-CalendarConnection.ps1', 'scripts/Remove-CalendarDemoResources.ps1', 'scripts/setup-modules.json',
    'templates/connection.schema.json', 'templates/connection.example.json', 'templates/endpoints.json', 'references/setup.md'
)
$staging = Join-Path ([IO.Path]::GetTempPath()) ('rise-x-calendar-package-' + [guid]::NewGuid().ToString('N'))
try {
    foreach ($relative in $files) {
        $destination = Join-Path $staging $relative
        [IO.Directory]::CreateDirectory((Split-Path $destination -Parent)) | Out-Null
        Copy-Item -LiteralPath (Join-Path $root $relative) -Destination $destination
        if (-not $Unsigned -and $relative.EndsWith('.ps1')) {
            $signature = Set-AuthenticodeSignature -FilePath $destination -Certificate $SigningCertificate -HashAlgorithm SHA256 -TimestampServer $TimestampServer
            if ($signature.Status -ne 'Valid' -or -not $signature.TimeStamperCertificate) { throw 'Code signing or trusted timestamp validation failed. No release ZIP was produced.' }
            if ((Get-AuthenticodeSignature -FilePath $destination).Status -ne 'Valid') { throw 'Signature verification failed.' }
        }
    }
    $inventory = foreach ($relative in $files) {
        @{path = $relative; sha256 = (Get-FileHash -LiteralPath (Join-Path $staging $relative) -Algorithm SHA256).Hash}
    }
    @{version = '1.0.0'; signed = (-not $Unsigned); purpose = $(if ($Unsigned) {'internal-test-only'} else {'customer-release'}); files = @($inventory)} |
        ConvertTo-Json -Depth 5 | Set-Content (Join-Path $staging 'package.json') -Encoding utf8
    [IO.Directory]::CreateDirectory((Split-Path $output -Parent)) | Out-Null
    [IO.Compression.ZipFile]::CreateFromDirectory($staging, $output)
    $hash = (Get-FileHash -LiteralPath $output -Algorithm SHA256).Hash
    Set-Content -LiteralPath "$output.sha256" -Value $hash -Encoding ascii
    [pscustomobject]@{Path = $output; SHA256 = $hash; Signed = (-not $Unsigned)}
} finally {
    if ([IO.Directory]::Exists($staging)) { [IO.Directory]::Delete($staging, $true) }
}
