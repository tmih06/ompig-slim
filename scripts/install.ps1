[CmdletBinding()]
param(
    [string]$Version,
    [string]$Dir,
    [switch]$NoModifyPath,
    [switch]$Help
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

function Assert-NoNewline([string]$Value, [string]$Name) {
    if ($Value -match '[\r\n]') {
        throw "$Name must not contain a newline."
    }
}

function Invoke-InstallFileMutation([scriptblock]$Mutation) {
    $stopwatch = [Diagnostics.Stopwatch]::StartNew()
    while ($true) {
        try {
            & $Mutation
            return
        } catch [IO.IOException] {
            $errorCode = $_.Exception.HResult -band 0xFFFF
            if (($errorCode -ne 32 -and $errorCode -ne 33) -or
                $stopwatch.Elapsed.TotalSeconds -ge 10) {
                throw
            }
            Start-Sleep -Milliseconds 50
        }
    }
}

function Get-NormalizedPath([string]$Path) {
    $value = [Environment]::ExpandEnvironmentVariables($Path.Trim().Trim('"'))
    try {
        return [IO.Path]::GetFullPath($value)
    } catch {
        return $value
    }
}

function Add-PathEntry([string]$PathValue, [string]$Entry) {
    $wanted = Get-NormalizedPath $Entry
    foreach ($existing in ($PathValue -split ';')) {
        if (-not [string]::IsNullOrWhiteSpace($existing) -and
            [StringComparer]::OrdinalIgnoreCase.Equals((Get-NormalizedPath $existing), $wanted)) {
            return $PathValue
        }
    }
    if ([string]::IsNullOrEmpty($PathValue)) {
        return $Entry
    }
    return "$PathValue;$Entry"
}

function Test-SafeArchiveName([string]$Name) {
    if ([string]::IsNullOrEmpty($Name) -or $Name.Contains('\') -or $Name.Contains(':') -or
        $Name.Contains([char]0) -or $Name.StartsWith('/')) {
        return $false
    }
    foreach ($part in $Name.TrimEnd('/').Split('/')) {
        if ([string]::IsNullOrEmpty($part) -or $part.EndsWith('.') -or $part.EndsWith(' ') -or
            $part -match '[\x00-\x1f<>"|?*]' -or
            $part -match '^(?i:CON|PRN|AUX|NUL|COM[1-9\u00B9\u00B2\u00B3]|LPT[1-9\u00B9\u00B2\u00B3])(?:\.|$)') {
            return $false
        }
    }
    return $true
}

function Test-ArchiveLink($Entry) {
    $unixMode = ($Entry.ExternalAttributes -shr 16) -band 0xFFFF
    $dosMode = $Entry.ExternalAttributes -band 0xFFFF
    $fileType = $unixMode -band 0xF000
    return ($fileType -eq 0xA000) -or (($dosMode -band 0x0400) -ne 0)
}

function Test-ArchiveRegularFile($Entry) {
    $unixMode = ($Entry.ExternalAttributes -shr 16) -band 0xFFFF
    $dosMode = $Entry.ExternalAttributes -band 0xFFFF
    $fileType = $unixMode -band 0xF000
    return (-not (Test-ArchiveLink $Entry)) -and
        (-not $Entry.FullName.EndsWith('/')) -and
        (($dosMode -band 0x0010) -eq 0) -and
        ($fileType -eq 0 -or $fileType -eq 0x8000)
}

function Copy-LimitedStream([IO.Stream]$Source, [IO.Stream]$Destination, [long]$MaximumBytes, [string]$LimitMessage) {
    $buffer = New-Object byte[] 65536
    [long]$copied = 0
    while (($read = $Source.Read($buffer, 0, $buffer.Length)) -gt 0) {
        $copied += $read
        if ($copied -gt $MaximumBytes) { throw $LimitMessage }
        $Destination.Write($buffer, 0, $read)
    }
    return $copied
}

function Save-Download([string]$Uri, [string]$Path, [long]$MaximumBytes) {
    $request = [Net.HttpWebRequest]::Create($Uri)
    $request.Method = 'GET'
    $request.UserAgent = 'ompig-slim-installer'
    $request.Timeout = 120000
    $request.ReadWriteTimeout = 120000
    $response = $null
    $inputStream = $null
    $outputStream = $null
    try {
        $response = $request.GetResponse()
        $limitMessage = "Download exceeds the $MaximumBytes-byte limit."
        if ($response.ContentLength -gt $MaximumBytes) { throw $limitMessage }
        $inputStream = $response.GetResponseStream()
        $outputStream = [IO.File]::Create($Path)
        $copied = Copy-LimitedStream $inputStream $outputStream $MaximumBytes $limitMessage
        if ($response.ContentLength -ge 0 -and $copied -ne $response.ContentLength) {
            throw 'Download ended before its Content-Length.'
        }
    } finally {
        if ($null -ne $outputStream) { $outputStream.Dispose() }
        if ($null -ne $inputStream) { $inputStream.Dispose() }
        if ($null -ne $response) { $response.Dispose() }
    }
}

function Write-InstallMarker([string]$Root, [string]$Text) {
    [void][IO.Directory]::CreateDirectory($Root)
    $marker = Join-Path $Root 'ompig-slim-install'
    $temporary = Join-Path $Root ('.ompig-slim-install.' + [Guid]::NewGuid().ToString('N') + '.tmp')
    try {
        [IO.File]::WriteAllText($temporary, $Text, [Text.UTF8Encoding]::new($false))
        if ([IO.File]::Exists($marker)) {
            Invoke-InstallFileMutation { [IO.File]::Replace($temporary, $marker, [NullString]::Value) }
        } else {
            Invoke-InstallFileMutation { [IO.File]::Move($temporary, $marker) }
        }
    } finally {
        if ([IO.File]::Exists($temporary)) {
            Invoke-InstallFileMutation { [IO.File]::Delete($temporary) }
        }
    }
}

if ($Help) {
    @'
Install ompig-slim from a published Windows release.

Usage: install.ps1 [-Version TAG] [-Dir DIRECTORY] [-NoModifyPath]

Defaults: the latest stable release and $env:LOCALAPPDATA\ompig-slim\bin.
Environment: OMPIG_SLIM_VERSION, OMPIG_SLIM_INSTALL_DIR,
OMPIG_SLIM_DOWNLOAD_BASE.
The installer adds its directory to the current process and user PATH. Other
already-running processes see the user PATH change when they next start.
Use -NoModifyPath to leave PATH unchanged.
'@
    return
}

$work = $null
$staged = $null
$zip = $null
$updateLock = $null
$lockHeld = $false
try {
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
        throw 'Windows is required.'
    }

    if (-not $PSBoundParameters.ContainsKey('Version')) {
        $Version = $env:OMPIG_SLIM_VERSION
    }
    if (-not $PSBoundParameters.ContainsKey('Dir')) {
        $Dir = $env:OMPIG_SLIM_INSTALL_DIR
    }
    $downloadBase = $env:OMPIG_SLIM_DOWNLOAD_BASE
    Assert-NoNewline $downloadBase 'OMPIG_SLIM_DOWNLOAD_BASE'
    Assert-NoNewline $Version 'Version'
    Assert-NoNewline $Dir 'Directory'
    if ([string]::IsNullOrWhiteSpace($downloadBase)) {
        $downloadBase = 'https://github.com/tmih06/ompig-slim/releases'
    }
    $downloadBase = $downloadBase.Trim().TrimEnd('/')
    $baseUri = $null
    if (-not [Uri]::TryCreate($downloadBase, [UriKind]::Absolute, [ref]$baseUri) -or
        $baseUri.Scheme -notin @('http', 'https')) {
        throw 'OMPIG_SLIM_DOWNLOAD_BASE must be an absolute HTTP or HTTPS URL.'
    }
    if ([string]::IsNullOrWhiteSpace($Dir)) {
        if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
            throw 'LOCALAPPDATA must be set, or pass -Dir DIRECTORY.'
        }
        $Dir = Join-Path $env:LOCALAPPDATA 'ompig-slim\bin'
    }
    $installDir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Dir)
    Assert-NoNewline $installDir 'Directory'

    $homeDirectory = $env:USERPROFILE
    if ([string]::IsNullOrEmpty($homeDirectory)) {
        $homeDirectory = [Environment]::GetFolderPath([Environment+SpecialFolder]::UserProfile)
    }
    $configRoot = $env:PIG_HOME
    if ([string]::IsNullOrEmpty($configRoot)) {
        $configRoot = Join-Path $homeDirectory '.pi'
    } elseif ($configRoot -ceq '~') {
        $configRoot = $homeDirectory
    } elseif ($configRoot.StartsWith('~/', [StringComparison]::Ordinal)) {
        $configRoot = Join-Path $homeDirectory $configRoot.Substring(2)
    }
    Assert-NoNewline $homeDirectory 'User profile path'
    Assert-NoNewline $configRoot 'PIG_HOME'

    $nativeArchitecture = $env:PROCESSOR_ARCHITEW6432
    if ([string]::IsNullOrWhiteSpace($nativeArchitecture)) {
        $nativeArchitecture = $env:PROCESSOR_ARCHITECTURE
    }
    switch ($nativeArchitecture.ToUpperInvariant()) {
        { $_ -in @('AMD64', 'X64', 'X86_64') } { $architecture = 'x64'; break }
        { $_ -in @('ARM64', 'AARCH64') } { $architecture = 'arm64'; break }
        default { throw "Unsupported Windows CPU '$nativeArchitecture'; supported CPUs are x64 and arm64." }
    }

    $archiveName = "ompig-slim-windows-$architecture.zip"
    if ([string]::IsNullOrWhiteSpace($Version) -or $Version -eq 'latest') {
        $archiveUrl = "$downloadBase/latest/download/$archiveName"
    } else {
        $archiveUrl = "$downloadBase/download/$Version/$archiveName"
    }
    Assert-NoNewline $archiveUrl 'Release URL'

    $work = Join-Path ([IO.Path]::GetTempPath()) ('ompig-slim-install-' + [Guid]::NewGuid().ToString('N'))
    [void][IO.Directory]::CreateDirectory($work)
    $archivePath = Join-Path $work $archiveName
    $checksumPath = "$archivePath.sha256"
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    Save-Download $archiveUrl $archivePath (512MB)
    Save-Download "$archiveUrl.sha256" $checksumPath 4096

    $checksumText = [IO.File]::ReadAllText($checksumPath, [Text.Encoding]::UTF8).TrimStart([char]0xFEFF)
    $checksumMatch = [Regex]::Match($checksumText, '^\s*([0-9a-fA-F]{64})(?:\s+.*)?\s*$')
    if (-not $checksumMatch.Success) {
        throw "$archiveName.sha256 is not a valid checksum file."
    }
    $actualChecksum = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash
    $expectedChecksum = $checksumMatch.Groups[1].Value
    if (-not [StringComparer]::OrdinalIgnoreCase.Equals($actualChecksum, $expectedChecksum)) {
        throw "Checksum mismatch for $archiveName."
    }

    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead($archivePath)
    $seenNames = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    $executableEntry = $null
    foreach ($entry in $zip.Entries) {
        if (-not (Test-SafeArchiveName $entry.FullName)) {
            throw "Unsafe ZIP member name '$($entry.FullName)'."
        }
        if (-not $seenNames.Add($entry.FullName)) {
            throw "Duplicate ZIP member '$($entry.FullName)'."
        }
        if (Test-ArchiveLink $entry) {
            throw "ZIP member '$($entry.FullName)' is a symbolic link."
        }
        if ([StringComparer]::Ordinal.Equals($entry.FullName, 'ompig-slim/ompig-slim.exe')) {
            $executableEntry = $entry
        }
    }
    if ($null -eq $executableEntry) {
        throw "$archiveName must contain exactly one ompig-slim/ompig-slim.exe member."
    }
    if (-not (Test-ArchiveRegularFile $executableEntry)) {
        throw "$archiveName does not contain a regular ompig-slim/ompig-slim.exe file."
    }
    $maximumExecutableSize = 256MB
    if ($executableEntry.Length -le 0 -or $executableEntry.Length -gt $maximumExecutableSize) {
        throw "ompig-slim/ompig-slim.exe has an invalid size ($($executableEntry.Length) bytes)."
    }

    [void][IO.Directory]::CreateDirectory($installDir)
    $target = Join-Path $installDir 'ompig-slim.exe'
    $updateLockPath = "$target.update.lock"
    $updateLock = [IO.File]::Open($updateLockPath, [IO.FileMode]::OpenOrCreate,
        [IO.FileAccess]::ReadWrite, [IO.FileShare]::ReadWrite)
    try {
        $updateLock.Lock(0, 1)
        $lockHeld = $true
    } catch {
        throw "Another install or update is in progress for $target."
    }
    $backup = Join-Path $installDir 'ompig-slim.previous.exe'
    if (-not [IO.File]::Exists($target) -and [IO.File]::Exists($backup)) {
        $backupAttributes = [IO.File]::GetAttributes($backup)
        if (($backupAttributes -band [IO.FileAttributes]::Directory) -ne 0 -or
            ($backupAttributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "Update backup $backup is not a regular file."
        }
        Invoke-InstallFileMutation { [IO.File]::Move($backup, $target) }
    }
    $staged = Join-Path $installDir ('.ompig-slim.new.' + [Guid]::NewGuid().ToString('N') + '.exe')
    $inputStream = $executableEntry.Open()
    $outputStream = [IO.File]::Create($staged)
    try {
        $copied = Copy-LimitedStream $inputStream $outputStream $maximumExecutableSize 'The executable exceeds the 256 MiB size limit.'
        if ($copied -ne $executableEntry.Length) {
            throw 'The executable ZIP member length did not match its contents.'
        }
    } finally {
        $outputStream.Dispose()
        $inputStream.Dispose()
    }
    $zip.Dispose()
    $zip = $null

    $versionOutput = (& $staged --version 2>&1 | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) {
        throw "$staged did not run on this machine: $versionOutput"
    }
    if ([string]::IsNullOrWhiteSpace($versionOutput)) {
        throw "$staged did not report a version."
    }
    Assert-NoNewline $versionOutput 'Executable version'
    Assert-NoNewline $target 'Executable path'

    if ([IO.File]::Exists($target)) {
        Invoke-InstallFileMutation { [IO.File]::Replace($staged, $target, [NullString]::Value) }
    } elseif ([IO.Directory]::Exists($target)) {
        throw "Cannot replace $target because it is a directory."
    } else {
        Invoke-InstallFileMutation { [IO.File]::Move($staged, $target) }
    }
    $staged = $null
    Write-Output "Installed ompig-slim $versionOutput to $target"

    $markerText = "kind=archive`nexecutable=$target`ndownload-base=$downloadBase`ninstalled-version=$versionOutput`ninstaller=$archiveUrl`nchannel=stable`n"
    try {
        $markerRoot = [IO.Path]::GetFullPath([IO.Path]::Combine(
            $ExecutionContext.SessionState.Path.CurrentFileSystemLocation.Path, $configRoot))
        Write-InstallMarker $markerRoot $markerText
    } catch {
        [Console]::Error.WriteLine("warning: could not record the install under $configRoot; ompig-slim update will refuse to replace this binary")
    }

    if (-not $NoModifyPath) {
        $env:Path = Add-PathEntry $env:Path $installDir
        try {
            $userPath = [Environment]::GetEnvironmentVariable('Path', [EnvironmentVariableTarget]::User)
            $updatedUserPath = Add-PathEntry $userPath $installDir
            if ($updatedUserPath -cne $userPath) {
                [Environment]::SetEnvironmentVariable('Path', $updatedUserPath, [EnvironmentVariableTarget]::User)
            }
        } catch {
            [Console]::Error.WriteLine("warning: could not add $installDir to the user PATH")
        }
        Write-Output 'Added the install directory to the current and user PATH.'
    }
} catch {
    [Console]::Error.WriteLine("ompig-slim installer: $($_.Exception.Message)")
    exit 1
} finally {
    if ($null -ne $zip) {
        $zip.Dispose()
    }
    if ($null -ne $updateLock) {
        if ($lockHeld) {
            try { $updateLock.Unlock(0, 1) } catch { }
        }
        $updateLock.Dispose()
    }
    if ($null -ne $staged -and [IO.File]::Exists($staged)) {
        Invoke-InstallFileMutation { [IO.File]::Delete($staged) }
    }
    if ($null -ne $work -and [IO.Directory]::Exists($work)) {
        [IO.Directory]::Delete($work, $true)
    }
}
