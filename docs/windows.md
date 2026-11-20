# Native Windows support

## Install a Windows release

Use 64-bit Windows with Windows Terminal or another console that supports ConPTY.
Releases include native x64 and ARM64 executables. Neither installer nor runtime
needs WSL. A 32-bit Windows build is not supported.

Run these commands in PowerShell 5.1 or later:

```powershell
Invoke-WebRequest -UseBasicParsing https://raw.githubusercontent.com/tmih06/ompig-slim/main/scripts/install.ps1 -OutFile install.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
& "$env:LOCALAPPDATA\ompig-slim\bin\ompig-slim.exe" --version
```

The process-scoped execution policy does not change your saved policy. Review the
downloaded script before running it. Open a new terminal after installation to
use the updated user PATH. To select a release, pass `-Version` with a tag that
contains Windows assets. Use `-Dir 'C:\Tools\ompig-slim'` for a custom destination.
Older releases without Windows ZIPs cannot be installed on Windows.

The default destination is `%LOCALAPPDATA%\ompig-slim\bin`. Installation needs
neither administrator rights nor Go. The installer checks the published SHA-256,
runs the staged executable, then replaces the destination. The checksum detects
corruption, not a compromised release publisher. Close running instances before
reinstalling through PowerShell. Use `ompig-slim update` for an in-process update.

## Run coding tools

Install [Git for Windows](https://git-scm.com/download/win) to use the `bash` tool
and durable shell commands. These tools retain Bash syntax even when you launch
`ompig-slim.exe` from PowerShell or cmd. They run native Git Bash, not WSL.
Automatic shell lookup excludes the Windows WSL `bash.exe` launcher. An explicit
`shellPath` remains an operator override.

The binary, installer, updater, file tools, and redirected CLI modes do not need
Bash to launch. External extensions and tools retain their own prerequisites.
Windows does not support Unix process suspension. Clipboard access requires an
interactive desktop. Container deployment remains Linux-only.

Profiles retain the shared Pi paths under `%USERPROFILE%\.pi\agent` and the
project's `.pi` directory. `PIG_HOME` overrides the library root and install
marker directory. Use an absolute path or a leading `~/`, not `~\`.

## Update an installation

```powershell
ompig-slim update
ompig-slim upgrade --channel beta
ompig-slim update --channel stable
```

Only an executable recorded by the installer is eligible for archive updates.
The updater verifies the ZIP and checksum and prepares the replacement's embedded
extension SDKs before replacing the executable. A Windows update moves the old
image aside because Windows cannot overwrite a mapped executable. Ordinary
replacement failures restore the old image. A hard termination between moves can
leave the old image at a recovery path. Do not delete that file until you have
restored the installed executable. The updater reports the recovery path when
rollback fails. Rerunning the PowerShell installer restores a backup-only install
before staging another release. Updates share a byte-range lock with the installer.
A process whose installed version changed must restart before updating again.
Channel selection is saved only after successful replacement. Release lookup,
downloads, and SDK staging share a five-minute deadline.

Unix atomic replacement and directory-flush guarantees do not apply to Windows.
Durable assignment files are flushed before rename, but Windows does not provide
the Unix directory flush used by this host. Private broker and workdesk
directories use a protected current-user DACL on Windows instead of POSIX mode
bits. Existing state files are secured before use. Windows state directories must
not contain junctions, symlinks, other reparse points, or multiply hard-linked
files. The host refuses them without following external targets. Client connection
secrets receive a private file DACL without changing the whole profile tree.
Existing durable replay and external-effect limits still apply. See
[durable behavior](durable.md) and [broker storage](remote-broker.md).

## Build and check in PowerShell

Install the Go version pinned by `go.mod` and Git for Windows. Use native scripts
rather than Bash or Make:

```powershell
.\scripts\bootstrap-vendor.ps1
go build -mod=vendor -trimpath -ldflags='-s -w' -o bin/ompig-slim.exe .
.\bin\ompig-slim.exe --version
.\scripts\check-go.ps1 -Mode check
```

The bootstrap restores pinned third-party dependencies without replacing the
maintained PiG sources. The check script tests root packages and PiG under their
canonical imports with an isolated profile. Windows uses directory junctions,
which do not need administrator symlink privileges. Use `-Mode race` on x64.
Go's Windows ARM64 target does not support the race detector.

## Verification gates

CI builds all six release archives and validates their executable headers,
checksums, paths, and bundled license texts. Native Windows x64 and ARM64 jobs
run the PowerShell bootstrap, Go tests and vet, loopback installer/update checks,
and executable smoke scenarios. The x64 job also runs the race detector.
Windows smoke scenarios drive terminal input through ConPTY rather than the
Unix-only Python PTY helpers. Stable and beta publication depend on these jobs.

Cross-compilation and PE inspection are not native runtime proof, so the native
jobs are the Windows support gate. [The verification report](windows-verification.md)
records native evidence and review gaps. [The decision trail](windows-support-decisions.tsv)
records implementation decisions.
