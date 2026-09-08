#Requires -Version 5.1
<#
.SYNOPSIS
    Installs rise-x-kit on Windows: downloads the latest kit-v* release,
    verifies its checksum, and puts the binary on PATH.
.NOTES
    Env override: $env:RISE_X_KIT_VERSION installs that version instead of
    the latest (e.g. "v0.1.0").
#>

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$Repo = 'rise-x/rise-x-ai-public-marketplace'
$ApiBase = "https://api.github.com/repos/$Repo"
$Headers = @{ 'Accept' = 'application/vnd.github+json'; 'User-Agent' = 'rise-x-kit-installer' }

function Resolve-Tag {
    if ($env:RISE_X_KIT_VERSION) {
        return "kit-$($env:RISE_X_KIT_VERSION)"
    }

    try {
        $latest = Invoke-RestMethod -Uri "$ApiBase/releases/latest" -Headers $Headers -UseBasicParsing
        if ($latest.tag_name -like 'kit-v*') {
            return $latest.tag_name
        }
    } catch {
        # No releases yet, or /releases/latest is unreachable -- fall through
        # to scanning every release below.
    }

    # /releases/latest didn't point at a kit release -- the repo may host
    # other release kinds too. GitHub lists releases newest-first, so the
    # first kit-v* tag is the newest one.
    $releases = Invoke-RestMethod -Uri "$ApiBase/releases?per_page=100" -Headers $Headers -UseBasicParsing
    $found = $releases | Where-Object { $_.tag_name -like 'kit-v*' } | Select-Object -First 1
    if (-not $found) {
        throw "No kit-v* release found in $Repo"
    }
    return $found.tag_name
}

$tag = Resolve-Tag
$version = $tag -replace '^kit-', ''
$asset = "rise-x-kit_${version}_windows_amd64.zip"
$downloadBase = "https://github.com/$Repo/releases/download/$tag"

$installDir = Join-Path $env:LOCALAPPDATA 'Programs\rise-x-kit'
if (Get-Process -Name rise-x-kit -ErrorAction SilentlyContinue) {
    Write-Host "Rise-X Kit is running. Click Quit in the app, then run this installer again."
    exit 1
}

$tempDir = Join-Path $env:TEMP ("rise-x-kit-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempDir | Out-Null

try {
    $zipPath = Join-Path $tempDir $asset
    $checksumsPath = Join-Path $tempDir 'checksums.txt'

    Invoke-WebRequest -Uri "$downloadBase/$asset" -OutFile $zipPath -UseBasicParsing
    Invoke-WebRequest -Uri "$downloadBase/checksums.txt" -OutFile $checksumsPath -UseBasicParsing

    $expectedLine = Get-Content $checksumsPath | Where-Object { $_ -match [regex]::Escape($asset) + '\s*$' } | Select-Object -First 1
    if (-not $expectedLine) {
        throw "$asset is not listed in checksums.txt"
    }
    $expectedHash = ($expectedLine -split '\s+')[0].ToUpperInvariant()

    $actualHash = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash.ToUpperInvariant()
    if ($actualHash -ne $expectedHash) {
        throw "Checksum mismatch for $asset (expected $expectedHash, got $actualHash)"
    }

    if (Test-Path $installDir) {
        Remove-Item -Path $installDir -Recurse -Force
    }
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null

    Expand-Archive -Path $zipPath -DestinationPath $installDir -Force

    $exePath = Join-Path $installDir 'rise-x-kit.exe'
    Unblock-File -Path $exePath

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $pathEntries = @()
    if ($userPath) {
        $pathEntries = $userPath -split ';'
    }
    if ($pathEntries -notcontains $installDir) {
        $newUserPath = if ($userPath) { "$userPath;$installDir" } else { $installDir }
        [Environment]::SetEnvironmentVariable('Path', $newUserPath, 'User')
        $env:Path = "$env:Path;$installDir"
    }

    $startMenuDir = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'
    $shortcutPath = Join-Path $startMenuDir 'Rise-X Kit.lnk'
    $shell = New-Object -ComObject WScript.Shell
    $shortcut = $shell.CreateShortcut($shortcutPath)
    $shortcut.TargetPath = $exePath
    $shortcut.WorkingDirectory = $installDir
    $shortcut.Save()

    Write-Host "Installed rise-x-kit $version."
    Write-Host "Run: rise-x-kit"
} finally {
    Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
}
