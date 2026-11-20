[CmdletBinding()]
param(
    [ValidateSet('check', 'test', 'race', 'vet')]
    [string]$Mode = 'check'
)

$callerErrorActionPreference = $ErrorActionPreference
$ErrorActionPreference = 'Stop'
$callerLocation = Get-Location
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$onWindows = [Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT
$savedEnvironment = @{}

function Set-ProcessEnvironment([string]$Name, [AllowNull()][string]$Value) {
    [Environment]::SetEnvironmentVariable($Name, $Value, 'Process')
}

function Invoke-Go([string]$Go, [string[]]$Arguments) {
    $previousPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & $Go @Arguments
        $exitCode = $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $previousPreference
    }
    if ($exitCode -ne 0) {
        throw "go $($Arguments -join ' ') failed with exit code $exitCode"
    }
}

function New-DirectoryLink([string]$Path, [string]$Target) {
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    if ($null -ne $item) {
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -eq 0) {
            throw "Refusing to replace non-link GOPATH entry $Path"
        }
        if ($item.PSIsContainer) { [IO.Directory]::Delete($Path, $false) }
        else { [IO.File]::Delete($Path) }
    }
    $linkType = if ($onWindows) { 'Junction' } else { 'SymbolicLink' }
    New-Item -ItemType $linkType -Path $Path -Target $Target | Out-Null
}

try {
    Set-Location -LiteralPath $repoRoot
    $selector = if ([string]::IsNullOrWhiteSpace($env:GO)) { 'go' } else { $env:GO }
    $selectorCommand = Get-Command -Name $selector -ErrorAction Stop
    if ($selectorCommand.CommandType -notin @('Application', 'ExternalScript')) {
        throw "GO must name a native Go command: $selector"
    }
    $gorootOutput = @(Invoke-Go $selectorCommand.Source @('env', 'GOROOT'))
    $goroot = [string]::Join([Environment]::NewLine, $gorootOutput).Trim()
    if ([string]::IsNullOrWhiteSpace($goroot)) { throw 'go env GOROOT returned an empty path' }
    $goName = if ($onWindows) { 'go.exe' } else { 'go' }
    $compiler = Join-Path (Join-Path $goroot 'bin') $goName
    if (-not (Test-Path -LiteralPath $compiler -PathType Leaf)) {
        throw "Go compiler not found at $compiler"
    }

    $cacheRoot = $env:OMPIG_CHECK_CACHE
    if ([string]::IsNullOrWhiteSpace($cacheRoot)) {
        $cacheBase = $env:XDG_CACHE_HOME
        if ([string]::IsNullOrWhiteSpace($cacheBase)) {
            $userHome = if ($onWindows) { $env:USERPROFILE } else { $env:HOME }
            if ([string]::IsNullOrWhiteSpace($userHome)) { throw 'Could not resolve a cache directory from HOME' }
            $cacheBase = Join-Path $userHome '.cache'
        }
        $cacheRoot = Join-Path $cacheBase 'ompig-check'
    }
    $cacheRoot = [IO.Path]::GetFullPath($cacheRoot)
    $gopath = Join-Path $cacheRoot 'gopath'
    $gopathSource = Join-Path $gopath 'src'
    $profileRoot = Join-Path $cacheRoot 'profile'
    $agentProfile = Join-Path $profileRoot 'agent'
    $libraryProfile = Join-Path $profileRoot 'library'
    $homeProfile = Join-Path $profileRoot 'home'
    $appDataProfile = Join-Path $profileRoot 'appdata'
    $localAppDataProfile = Join-Path $profileRoot 'localappdata'
    $xdgCacheProfile = Join-Path $profileRoot 'xdg-cache'
    $xdgConfigProfile = Join-Path $profileRoot 'xdg-config'
    $xdgDataProfile = Join-Path $profileRoot 'xdg-data'
    foreach ($directory in @(
        $gopathSource, $agentProfile, $libraryProfile, $homeProfile,
        $appDataProfile, $localAppDataProfile, $xdgCacheProfile,
        $xdgConfigProfile, $xdgDataProfile
    )) {
        New-Item -ItemType Directory -Path $directory -Force | Out-Null
    }

    $isolatedEnvironment = @{
        APPDATA = $appDataProfile
        GOPATH = $gopath
        GO111MODULE = 'on'
        GOFLAGS = ''
        GOTOOLCHAIN = 'local'
        HOME = $homeProfile
        HOMEDRIVE = [IO.Path]::GetPathRoot($homeProfile).TrimEnd('\', '/')
        HOMEPATH = $homeProfile.Substring([IO.Path]::GetPathRoot($homeProfile).Length - 1)
        LOCALAPPDATA = $localAppDataProfile
        OMPIG_SLIM_CODING_AGENT_DIR = $agentProfile
        PIG_CODING_AGENT_DIR = $agentProfile
        PIG_HOME = $libraryProfile
        PI_CODING_AGENT_DIR = $agentProfile
        PI_HOME = $libraryProfile
        PI_SKIP_VERSION_CHECK = '1'
        PI_TELEMETRY = '0'
        USERPROFILE = $homeProfile
        XDG_CACHE_HOME = $xdgCacheProfile
        XDG_CONFIG_HOME = $xdgConfigProfile
        XDG_DATA_HOME = $xdgDataProfile
    }
    foreach ($name in $isolatedEnvironment.Keys) {
        $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
        Set-ProcessEnvironment $name $isolatedEnvironment[$name]
    }

    foreach ($source in Get-ChildItem -LiteralPath (Join-Path $repoRoot 'vendor') -Directory -Force) {
        New-DirectoryLink (Join-Path $gopathSource $source.Name) $source.FullName
    }

    if ($Mode -in @('check', 'test', 'race')) {
        $raceFlag = @(if ($Mode -eq 'race') { '-race' })
        Invoke-Go $compiler (@('test') + $raceFlag + @('-mod=vendor', './...'))

        $packageOutput = @(Invoke-Go $compiler @('list', '-mod=vendor', './vendor/github.com/MichaelKinsy/PiG/...'))
        $packages = @($packageOutput | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ })
        if ($packages.Count -eq 0) { throw 'No PiG packages found' }

        Set-ProcessEnvironment 'GO111MODULE' 'off'
        Set-Location -LiteralPath $gopathSource
        Invoke-Go $compiler (@('test') + $raceFlag + $packages)
        Set-ProcessEnvironment 'GO111MODULE' 'on'
        Set-Location -LiteralPath $repoRoot
    }

    if ($Mode -in @('check', 'vet')) {
        Invoke-Go $compiler @('vet', '-mod=vendor', './...', './vendor/github.com/MichaelKinsy/PiG/...')
    }
}
finally {
    foreach ($name in $savedEnvironment.Keys) {
        Set-ProcessEnvironment $name $savedEnvironment[$name]
    }
    Set-Location -LiteralPath $callerLocation.Path
    $ErrorActionPreference = $callerErrorActionPreference
}
