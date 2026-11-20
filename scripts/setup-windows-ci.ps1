#requires -Version 5.1
$ErrorActionPreference = 'Stop'

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    throw 'This helper requires native Windows.'
}
if (-not $env:GITHUB_PATH -or -not $env:RUNNER_TEMP) {
    throw 'This helper requires a GitHub Actions runner.'
}

$bash = Join-Path $env:ProgramFiles 'Git\bin\bash.exe'
if (-not (Test-Path -LiteralPath $bash -PathType Leaf)) {
    throw 'Git for Windows with native Bash is required for tool integration tests.'
}
& $bash -c 'test -z "$WSL_DISTRO_NAME" && printf native-bash'
if ($LASTEXITCODE -ne 0) { throw 'Native Bash did not start.' }

if (-not (Get-Command rg -ErrorAction SilentlyContinue)) {
    $architecture = $env:PROCESSOR_ARCHITEW6432
    if (-not $architecture) { $architecture = $env:PROCESSOR_ARCHITECTURE }
    # The digests are pinned here, not fetched beside the archive, so a replaced
    # release asset cannot vouch for itself. Update them together with $version.
    $version = '15.2.0'
    switch ($architecture.ToUpperInvariant()) {
        'AMD64' {
            $target = 'x86_64-pc-windows-msvc'
            $expected = '71b2fef860abe467217a538ff31de02f5258807c0129f771846f87bd029aafc5'
        }
        'ARM64' {
            $target = 'aarch64-pc-windows-msvc'
            $expected = 'e4abca10c3a64ebea742667dd7009449d49403db5460dd6873e389fa2945360f'
        }
        default { throw "Unsupported Windows architecture $architecture" }
    }
    $name = "ripgrep-$version-$target.zip"
    $directory = Join-Path $env:RUNNER_TEMP 'ripgrep'
    New-Item -ItemType Directory -Path $directory -Force | Out-Null
    $archive = Join-Path $directory $name
    $url = "https://github.com/BurntSushi/ripgrep/releases/download/$version/$name"
    Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $archive
    if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ine $expected) {
        throw 'ripgrep checksum mismatch.'
    }
    Expand-Archive -LiteralPath $archive -DestinationPath $directory -Force
    $bin = Join-Path $directory "ripgrep-$version-$target"
    if (-not (Test-Path -LiteralPath (Join-Path $bin 'rg.exe') -PathType Leaf)) {
        throw 'The ripgrep archive does not contain rg.exe.'
    }
    Add-Content -LiteralPath $env:GITHUB_PATH -Value $bin -Encoding UTF8
    $env:PATH = "$bin;$env:PATH"
}
& rg --version
if ($LASTEXITCODE -ne 0) { throw 'ripgrep did not start.' }
