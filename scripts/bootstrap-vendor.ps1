[CmdletBinding()]
param()

$callerErrorActionPreference = $ErrorActionPreference
$ErrorActionPreference = 'Stop'
$callerLocation = Get-Location
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$stage = $null
$oldGoFlags = [Environment]::GetEnvironmentVariable('GOFLAGS', 'Process')

function Get-ExistingItem([string]$Path) {
    Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
}

function Assert-NoReparseTree([string]$Path) {
    $item = Get-ExistingItem $Path
    if ($null -eq $item) { return }
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Refusing to replace reparse point $Path"
    }
    if ($item.PSIsContainer) {
        foreach ($child in Get-ChildItem -LiteralPath $Path -Force) {
            Assert-NoReparseTree $child.FullName
        }
    }
}

function Remove-Tree([string]$Path) {
    $item = Get-ExistingItem $Path
    if ($null -eq $item) { return }
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Refusing to remove reparse point $Path"
    }
    if ($item.PSIsContainer) {
        foreach ($child in Get-ChildItem -LiteralPath $Path -Force) {
            Remove-Tree $child.FullName
        }
    }
    Remove-Item -LiteralPath $Path -Force
}

function Remove-TemporaryTree([string]$Path) {
    $item = Get-ExistingItem $Path
    if ($null -eq $item) { return }
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        if ($item.PSIsContainer) { [IO.Directory]::Delete($Path, $false) }
        else { [IO.File]::Delete($Path) }
        return
    }
    if ($item.PSIsContainer) {
        foreach ($child in Get-ChildItem -LiteralPath $Path -Force) {
            Remove-TemporaryTree $child.FullName
        }
    }
    Remove-Item -LiteralPath $Path -Force
}

function Assert-OrdinaryDirectory([string]$Path) {
    $item = Get-ExistingItem $Path
    if ($null -ne $item -and
        (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or -not $item.PSIsContainer)) {
        throw "Refusing to write through non-directory or reparse point $Path"
    }
}

try {
    Set-Location -LiteralPath $repoRoot
    $vendorRoot = Join-Path $repoRoot 'vendor'
    $manifest = Join-Path $vendorRoot 'modules.txt'
    $marker = Join-Path $vendorRoot '.bootstrap-modules.txt'
    if (-not (Test-Path -LiteralPath $vendorRoot -PathType Container) -or
        -not (Test-Path -LiteralPath $manifest -PathType Leaf)) {
        throw 'Expected vendor/modules.txt in the checkout'
    }
    Assert-OrdinaryDirectory $vendorRoot
    Assert-OrdinaryDirectory (Join-Path $vendorRoot 'github.com')
    foreach ($protectedPath in @($manifest, $marker)) {
        $item = Get-ExistingItem $protectedPath
        if ($null -ne $item -and
            (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $item.PSIsContainer)) {
            throw "Refusing to write through non-file or reparse point $protectedPath"
        }
    }
    if ((Test-Path -LiteralPath $marker -PathType Leaf) -and
        (Get-FileHash -LiteralPath $manifest -Algorithm SHA256).Hash -eq
        (Get-FileHash -LiteralPath $marker -Algorithm SHA256).Hash) {
        return
    }

    $stage = Join-Path ([IO.Path]::GetTempPath()) ('ompig-vendor-' + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $stage | Out-Null
    Copy-Item -LiteralPath (Join-Path $repoRoot 'go.mod') -Destination $stage
    Copy-Item -LiteralPath (Join-Path $repoRoot 'go.sum') -Destination $stage

    $imports = @(foreach ($rawLine in Get-Content -LiteralPath $manifest -Encoding UTF8) {
        $package = $rawLine.Trim()
        if ([string]::IsNullOrWhiteSpace($package) -or $package.StartsWith('#')) { continue }
        if ($package -cmatch '^github\.com/MichaelKinsy/PiG(/|$)') { continue }
        if ($package.Contains('"') -or $package.Contains("`n") -or $package.Contains("`r")) {
            throw "Invalid package entry in vendor/modules.txt: $package"
        }
        '    _ "' + $package + '"'
    })
    $mainSource = (@('package main', 'import (') + $imports + @(')', 'func main() {}', '')) -join "`n"
    [IO.File]::WriteAllText((Join-Path $stage 'main.go'), $mainSource, [System.Text.UTF8Encoding]::new($false))

    $goCommand = if ([string]::IsNullOrWhiteSpace($env:GO)) { 'go' } else { $env:GO }
    $resolvedGo = Get-Command -Name $goCommand -ErrorAction Stop
    if ($resolvedGo.CommandType -notin @('Application', 'ExternalScript')) {
        throw "GO must name a native Go command: $goCommand"
    }
    $goPath = $resolvedGo.Source
    [Environment]::SetEnvironmentVariable('GOFLAGS', '', 'Process')
    Set-Location -LiteralPath $stage
    $previousPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & $goPath mod vendor
        $goExitCode = $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $previousPreference
    }
    if ($goExitCode -ne 0) { throw "go mod vendor failed with exit code $goExitCode" }
    Set-Location -LiteralPath $repoRoot

    $stagedVendor = Join-Path $stage 'vendor'
    $stagedManifest = Join-Path $stagedVendor 'modules.txt'
    if (-not (Test-Path -LiteralPath $stagedManifest -PathType Leaf)) {
        throw 'go mod vendor did not produce vendor/modules.txt'
    }
    Remove-Item -LiteralPath $stagedManifest -Force

    $generatedRoots = @(Get-ChildItem -LiteralPath $stagedVendor -Directory -Force)
    foreach ($sourceRoot in $generatedRoots) {
        $targetRoot = Join-Path $vendorRoot $sourceRoot.Name
        if ($sourceRoot.Name -ne 'github.com') {
            Assert-NoReparseTree $targetRoot
            continue
        }
        Assert-OrdinaryDirectory $targetRoot
        foreach ($sourceOwner in Get-ChildItem -LiteralPath $sourceRoot.FullName -Directory -Force) {
            $targetOwner = Join-Path $targetRoot $sourceOwner.Name
            if ($sourceOwner.Name -ne 'MichaelKinsy') {
                Assert-NoReparseTree $targetOwner
                continue
            }
            Assert-OrdinaryDirectory $targetOwner
            foreach ($sourceModule in Get-ChildItem -LiteralPath $sourceOwner.FullName -Force) {
                if ($sourceModule.Name -eq 'PiG') {
                    throw 'Scratch vendor unexpectedly contains maintained PiG sources'
                }
            }
            if (Test-Path -LiteralPath $targetOwner -PathType Container) {
                foreach ($targetModule in Get-ChildItem -LiteralPath $targetOwner -Force) {
                    if ($targetModule.Name -ne 'PiG') { Assert-NoReparseTree $targetModule.FullName }
                }
            }
        }
    }

    foreach ($sourceRoot in $generatedRoots) {
        $targetRoot = Join-Path $vendorRoot $sourceRoot.Name
        if ($sourceRoot.Name -ne 'github.com') {
            Remove-Tree $targetRoot
            Move-Item -LiteralPath $sourceRoot.FullName -Destination $targetRoot
            continue
        }
        if (-not (Test-Path -LiteralPath $targetRoot -PathType Container)) {
            New-Item -ItemType Directory -Path $targetRoot | Out-Null
        }
        foreach ($sourceOwner in Get-ChildItem -LiteralPath $sourceRoot.FullName -Directory -Force) {
            $targetOwner = Join-Path $targetRoot $sourceOwner.Name
            if ($sourceOwner.Name -ne 'MichaelKinsy') {
                Remove-Tree $targetOwner
                Move-Item -LiteralPath $sourceOwner.FullName -Destination $targetOwner
                continue
            }
            if (-not (Test-Path -LiteralPath $targetOwner -PathType Container)) {
                New-Item -ItemType Directory -Path $targetOwner | Out-Null
            }
            foreach ($targetModule in Get-ChildItem -LiteralPath $targetOwner -Force) {
                if ($targetModule.Name -ne 'PiG') { Remove-Tree $targetModule.FullName }
            }
            foreach ($sourceModule in Get-ChildItem -LiteralPath $sourceOwner.FullName -Force) {
                Move-Item -LiteralPath $sourceModule.FullName -Destination $targetOwner
            }
        }
    }

    Copy-Item -LiteralPath $manifest -Destination $marker -Force
}
finally {
    [Environment]::SetEnvironmentVariable('GOFLAGS', $oldGoFlags, 'Process')
    Set-Location -LiteralPath $callerLocation.Path
    try {
        if ($null -ne $stage) { Remove-TemporaryTree $stage }
    }
    finally {
        $ErrorActionPreference = $callerErrorActionPreference
    }
}
