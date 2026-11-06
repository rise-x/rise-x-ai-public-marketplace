BeforeAll {
    $skill = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
}

Describe 'Installed marketplace skill portability' {
    BeforeEach {
        $installed = Join-Path $TestDrive 'installed skill'
        Copy-Item $skill $installed -Recurse
        $entry = Join-Path $installed 'scripts/Start-CalendarSetup.ps1'
    }

    It 'previews from an unrelated working directory without modifying the installed skill' {
        $before = @(Get-ChildItem $installed -Recurse -File | Get-FileHash).Hash
        Push-Location $TestDrive
        try { & $entry -WhatIf | Should -Match 'Preview' }
        finally { Pop-Location }
        $after = @(Get-ChildItem $installed -Recurse -File | Get-FileHash).Hash
        ($after -join ',') | Should -Be ($before -join ',')
        Test-Path (Join-Path $TestDrive '.local') | Should -BeFalse
    }

    It 'requires a chosen setup folder before prompts or provider access' {
        { & $entry -Confirm:$false } | Should -Throw '*Supply -SetupDirectory*'
    }

    It 'resolves relative setup paths against the operator PowerShell location' {
        $folder = Join-Path $TestDrive 'private state'
        New-Item -ItemType Directory -Path $folder | Out-Null
        # Stop at local configuration validation, before any provider is loaded.
        Set-Content (Join-Path $folder 'connection.json') 'invalid json'
        Push-Location $TestDrive
        try {
            { & $entry -SetupDirectory './private state' -Confirm:$false } | Should -Throw '*connection configuration*'
            $status = Get-Content (Join-Path $folder 'setup-status.json') -Raw | ConvertFrom-Json
            $status.stage | Should -Be 'connection configuration'
        } finally { Pop-Location }
    }

    It 'rejects state inside the installed skill before creating any files' {
        { & $entry -SetupDirectory $installed -Confirm:$false } | Should -Throw '*outside the installed skill*'
        $stateFolder = Join-Path $installed 'private state'
        { & $entry -SetupDirectory $stateFolder -Confirm:$false } | Should -Throw '*outside the installed skill*'
        Test-Path $stateFolder | Should -BeFalse
    }

    It 'resolves every local Markdown reference within the standalone skill' {
        foreach ($file in Get-ChildItem $installed -Recurse -Filter '*.md') {
            $content = Get-Content $file.FullName -Raw
            foreach ($match in [regex]::Matches($content, '\[[^\]]+\]\(([^)]+)\)')) {
                $target = $match.Groups[1].Value
                if ($target -match '^(https?://|#)') { continue }
                $relative = ($target -split '#')[0]
                $resolved = [IO.Path]::GetFullPath((Join-Path $file.DirectoryName $relative))
                $resolved.StartsWith($installed + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) | Should -BeTrue
                Test-Path -LiteralPath $resolved | Should -BeTrue -Because "$($file.Name) links to $relative"
            }
        }
    }
}
