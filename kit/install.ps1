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
    # other release kinds too, and it answers 404 while every release is a
    # prerelease. GitHub lists releases newest-first, so the first match is
    # the newest one.
    #
    # Prereleases and drafts are skipped: this fallback is the one path
    # reachable while no stable release exists, so without the filter a
    # partner running the documented one-liner gets an RC and stays on the RC
    # track, while selfupdate offers a prerelease only to a binary already on
    # one. $env:RISE_X_KIT_VERSION above is how you ask for one on purpose.
    $releases = Invoke-RestMethod -Uri "$ApiBase/releases?per_page=100" -Headers $Headers -UseBasicParsing
    $found = $releases |
        Where-Object { -not $_.prerelease -and -not $_.draft -and $_.tag_name -match '^kit-v\d+\.\d+\.\d+$' } |
        Select-Object -First 1
    if (-not $found) {
        throw "No stable kit-v* release found in $Repo. Prereleases are not installed by default; to install one, set `$env:RISE_X_KIT_VERSION (e.g. 'v0.1.0-rc.1')."
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

    $assetPattern = '^\s*([0-9a-f]{64})\s+\*?' + [regex]::Escape($asset) + '\s*$'
    $expectedMatch = Get-Content $checksumsPath | Select-String -Pattern $assetPattern | Select-Object -First 1
    if (-not $expectedMatch) {
        throw "$asset is not listed in checksums.txt"
    }
    $expectedHash = $expectedMatch.Matches[0].Groups[1].Value.ToUpperInvariant()

    $actualHash = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash.ToUpperInvariant()
    if ($actualHash -ne $expectedHash) {
        throw "Checksum mismatch for $asset (expected $expectedHash, got $actualHash)"
    }

    $parentDir = Split-Path $installDir -Parent
    New-Item -ItemType Directory -Path $parentDir -Force | Out-Null

    # Sweep leftovers an earlier run could not remove -- both cleanups here are
    # best-effort, and nothing else ever looks in this directory: the kit's own
    # Cleanup sweeps only inside the install directory. Without this every
    # interrupted reinstall leaves another full copy behind for good. Runs
    # before this run's own scratch directory exists, so it cannot take it.
    foreach ($stale in @('rise-x-kit.old-*', 'rise-x-kit.new-*')) {
        Get-ChildItem -Path $parentDir -Directory -Filter $stale -ErrorAction SilentlyContinue |
            ForEach-Object { Remove-Item -Path $_.FullName -Recurse -Force -ErrorAction SilentlyContinue }
    }

    # Extract beside $installDir rather than under $tempDir, so both swaps
    # below are same-volume renames. $env:TEMP is redirected to another volume
    # on plenty of corporate images, and across volumes Move-Item is
    # copy-then-delete: a failure midway would leave a partial $installDir and
    # no way to put the old one back. Same reason the Go self-updater
    # downloads into the install directory rather than into a temp dir.
    $extractDir = Join-Path $parentDir ('rise-x-kit.new-' + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $extractDir -Force | Out-Null

    try {
        Expand-Archive -Path $zipPath -DestinationPath $extractDir -Force

        # The existing install is renamed aside rather than deleted, and only
        # removed once the new one is in place. Move-Item can still fail here
        # -- antivirus holding a handle, a permission change, a full disk --
        # and deleting first would leave the partner with no kit at all and no
        # copy to put back.
        $backupDir = Join-Path $parentDir ('rise-x-kit.old-' + [Guid]::NewGuid().ToString('N'))
        $movedAside = $false
        if (Test-Path $installDir) {
            Move-Item -Path $installDir -Destination $backupDir
            $movedAside = $true
        }
        try {
            Move-Item -Path $extractDir -Destination $installDir
        } catch {
            $installError = $_
            if (-not $movedAside) { throw }
            # Clear whatever the failed move left at the destination first, or
            # the restore fails too and the partner is shown that error rather
            # than this one. The throws are outside the try on purpose: inside,
            # the success message would be caught by its own catch block.
            $restored = $false
            try {
                if (Test-Path $installDir) {
                    Remove-Item -Path $installDir -Recurse -Force
                }
                Move-Item -Path $backupDir -Destination $installDir
                $restored = $true
            } catch {
                $restored = $false
            }
            if ($restored) {
                throw "Could not install the new version; the previous one is still in place. $installError"
            }
            throw ("Could not install the new version, and could not put the previous one back. " +
                "Your install is at '$backupDir' -- rename that folder to '$installDir' to recover. $installError")
        }
        if ($movedAside) {
            Remove-Item -Path $backupDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    } finally {
        Remove-Item -Path $extractDir -Recurse -Force -ErrorAction SilentlyContinue
    }

    $exePath = Join-Path $installDir 'rise-x-kit.exe'
    # Strips the mark of the web, which is the only reason an unsigned exe
    # runs past SmartScreen. Once Azure Trusted Signing is wired up
    # (kit-release.yml still signs with a no-op), this hides a failed
    # signature instead of an absent one. Remove it with the signing.
    Unblock-File -Path $exePath

    # [Environment]::SetEnvironmentVariable always writes REG_SZ, which would
    # silently drop the %VAR% expansion on an existing REG_EXPAND_SZ Path --
    # read and write the registry value directly so its kind is preserved.
    $envKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
    try {
        $pathKind = $envKey.GetValueKind('Path')
    } catch [System.Management.Automation.MethodInvocationException] {
        $pathKind = [Microsoft.Win32.RegistryValueKind]::String
    }
    $userPath = $envKey.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
    $pathEntries = @()
    if ($userPath) {
        $pathEntries = $userPath -split ';'
    }
    if ($pathEntries -notcontains $installDir) {
        $newUserPath = if ($userPath) { "$userPath;$installDir" } else { $installDir }
        $envKey.SetValue('Path', $newUserPath, $pathKind)
        $env:Path = "$env:Path;$installDir"
    }
    $envKey.Close()

    $startMenuDir = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'
    $shortcutPath = Join-Path $startMenuDir 'Rise-X Kit.lnk'
    $shell = New-Object -ComObject WScript.Shell
    $shortcut = $shell.CreateShortcut($shortcutPath)
    $shortcut.TargetPath = $exePath
    $shortcut.WorkingDirectory = $installDir
    $shortcut.Save()

    # Teach Claude Code how to open the app, so "open Rise-X Kit" works in a
    # session. Idempotent, and the previous CLAUDE.md is backed up first. This
    # edits a global file, so say so, and let it be declined.
    if ($env:RISE_X_KIT_CLAUDE_MD -eq '0') {
        Write-Host "Skipped the CLAUDE.md note (RISE_X_KIT_CLAUDE_MD=0)."
    } else {
        Write-Host "Adding a `"how to open Rise-X Kit`" note to the global CLAUDE.md:"
        try {
            & $exePath -write-claude-md
        } catch {
            Write-Warning "Could not update the global CLAUDE.md: $_"
        }
    }

    Write-Host "Installed rise-x-kit $version."
    Write-Host "Run: rise-x-kit"
} finally {
    Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
}
