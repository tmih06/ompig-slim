# Install and troubleshoot ompig-slim

ompig-slim publishes release archives on [GitHub Releases](https://github.com/tmih06/ompig-slim/releases). The archive names on this page are the ones the release workflow produces. To build `ompig-slim` from source instead, see [Install](install.md).

## ompig-slim: command not found

The terminal runs a command only when the command's folder is on `PATH`, the list of folders the terminal searches. Print the list with `echo $PATH`.

- **Install script.** `install.sh` puts `ompig-slim` in `~/.local/bin`. When that folder is not on `PATH`, the script prints the `export PATH=...` line to add; run it. Add the same line to the end of `~/.zshrc` (macOS) or `~/.bashrc` (most Linux systems) so that new terminals have it, then open a new terminal.
- **Build from source.** `make install` writes to `~/.local/bin`, and `make build` leaves the binary at `./bin/ompig-slim`. Run it by path, or add that directory to `PATH`.

Run `ompig-slim --version` again to check.

## Change what the install script does

`install.sh` reads these environment variables. Set them on the `sh` side of the
pipe, for example:

```bash
curl -fsSL https://raw.githubusercontent.com/tmih06/ompig-slim/main/scripts/install.sh \
  | OMPIG_SLIM_VERSION=v0.1.3 bash
```

| Variable | Meaning |
|---|---|
| `OMPIG_SLIM_VERSION` | Install this release tag instead of the latest stable. |
| `OMPIG_SLIM_INSTALL_DIR` | Directory that receives `ompig-slim`. The default is `$HOME/.local/bin`. |
| `OMPIG_SLIM_DOWNLOAD_BASE` | Release download root, for a mirror or a test server. The default is this repository's GitHub Releases URL. |

The script verifies the archive's SHA-256 before it writes anything to the
install directory, whichever mirror these variables select.

## Verify what you installed

`ompig-slim verify` prints the binary's identity and the SHA-256 of its bytes. Verification starts from that digest; the version string, file name, and download URL prove nothing on their own.

```bash
ompig-slim verify                                   # this binary: identity and SHA-256
ompig-slim verify --checksums ompig-slim-linux-x64.tar.gz.sha256 ompig-slim-linux-x64.tar.gz
ompig-slim verify --provenance ./ompig-slim         # GitHub build provenance, via gh
ompig-slim verify --packages                        # installed npm and git Packages
ompig-slim verify ./my-extension ./agent.yaml       # extension, Package, or Piglet
```

`--checksums` requires each file's digest to match its entry in the release `SHA256SUMS`. `--provenance` runs `gh attestation verify`, which checks the Sigstore bundle, its transparency-log entry, and that the signer is ompig-slim's release workflow. A binary you built yourself has no attestation and reports as unverified. `--packages` runs `npm audit signatures` for installed npm Packages and checks that each git Package's working tree matches its commit. A directory or Piglet file is loaded through the same validators ompig-slim uses at startup. The design follows `pi verify` in [dimetron/pi-go](https://github.com/dimetron/pi-go).

## macOS

Browsers, AirDrop, and messaging apps add the `com.apple.quarantine` attribute to downloaded files. Gatekeeper then blocks an unsigned binary with a message that the developer cannot be verified. The release workflow does not sign binaries with an Apple Developer ID and does not notarize them. Files downloaded with `curl` or `wget` are not quarantined.

After you verify the SHA-256, remove the attribute from the binary or from the extracted archive directory:

```bash
xattr -l ./ompig-slim
xattr -d com.apple.quarantine ./ompig-slim
```

Apple silicon Macs use the `darwin-arm64` archive and Intel Macs use `darwin-x64`. `uname -m` prints `arm64` or `x86_64`.

## Windows

Files downloaded with a browser carry the Mark of the Web, and SmartScreen can warn that the app is unrecognized. After you verify the SHA-256, run `Unblock-File .\ompig-slim.exe` in PowerShell. Microsoft Defender can flag unsigned Go binaries heuristically; verify the checksum and report a false positive at https://www.microsoft.com/wdsi/filesubmission.

Add the directory that contains `ompig-slim.exe` to `PATH`, then open a new terminal. This repository publishes Linux and macOS archives; a Windows build is not part of the release workflow.

## Linux

The release workflow builds binaries with cgo disabled, so they do not depend on the system C library. Use `linux-x64` when `uname -m` prints `x86_64` and `linux-arm64` when it prints `aarch64`. If a download fails with `permission denied`, run `chmod +x ompig-slim`; if the file sits on a `noexec` mount such as /tmp, move it to `~/.local/bin`.

## Android (Termux)

This repository's release workflow publishes Linux and macOS archives only;
there is no Android archive. Build from a source checkout with Termux's Go
package instead: `pkg install golang`, then `make build`.

`ompig-slim setup go` cannot install Go on Android, because Go publishes no
archive for `android/arm64`. It stops and tells you to run `pkg install golang`.
ompig-slim then uses the `go` on `PATH`, and `ompig-slim setup status` shows it.
`PIG_CELL_BUILD_TIMEOUT` (default 10 minutes) also bounds
`ompig-slim install <dir> --validate-only`, so a slow Rust build on a phone can
be given more time.

## Proxies and certificates

ompig-slim's HTTP clients honor `HTTPS_PROXY`, `HTTP_PROXY`, and `NO_PROXY`. On macOS and Windows ompig-slim trusts the system certificate store. On Linux, set `SSL_CERT_FILE` or `SSL_CERT_DIR` when a corporate proxy re-signs TLS traffic. Node extensions read additional certificates from `NODE_EXTRA_CA_CERTS`.

## Toolchains for extensions and Piglet builds

ompig-slim runs without any toolchain. Extensions and Piglet builds need the tool for their language:

| Need | Tool | Set up |
|---|---|---|
| Go extensions, native Piglet builds | Go | `ompig-slim setup go` installs a verified Go under `~/.pi/toolchains/go`; ompig-slim uses it when `go` is not on `PATH` |
| Container Piglet builds | Docker or Podman | `ompig-slim setup container` shows the steps for your system |
| Rust extensions | cargo | https://rustup.rs |
| Node and TypeScript extensions | Node.js 22.13 or newer | https://nodejs.org |
| Python extensions | Python 3 | https://www.python.org/downloads/ |

`ompig-slim setup` shows which tools are present. Native Piglet builds also need an ompig-slim source checkout that matches the binary; set `PIG_SOURCE_ROOT` to it.

## Terminal display

A real change of terminal width or height redraws the whole transcript, as Pi does. If the view jumps without a size change, set `PI_TUI_DEBUG_REDRAW=1`, reproduce the problem, and read `pi-tui-debug.log` in the configured agent directory (normally `~/.pi/agent`): each full redraw appends its reason. For tmux settings, see [tmux](https://pi-in-go.dev/docs/latest/tmux/).

## Extensions that fail to load

The error names the extension's runtime log, for example `/tmp/pig-ext-<name>-<id>.log`. Read it first: a missing module or export usually means the extension needs an API that ompig-slim does not provide yet. Report it with the log and the extension's source. To force ompig-slim to rebuild compiled extensions, delete `~/.pi/cache/ext`.

A failed extension stops ompig-slim at startup. `ompig-slim -ne` starts ompig-slim without extensions. A Go extension written for an older SDK reports `Extension "<name>" was written for an older SDK`, lists each changed call, and prints a `ompig-slim extension upgrade <folder>` command. Run that command on your own extension's source folder. For an installed Package, remove it with `ompig-slim remove <source>` and ask its author for an update.

## Restart after an update

A session keeps running the executable it started from. If an update replaces or removes `ompig-slim`, or another `ompig-slim` prunes an extension cell this session uses, the next error shows `The pig installation this session runs from was removed or replaced` (or `The pig extension files this session runs from were removed or replaced`) once. Restart with the `ompig-slim --session <id>` command it prints to continue the session. See [TUI](tui.md) for when ompig-slim checks.

## Report a problem

Include the output of `ompig-slim verify`, your operating system and terminal, and the steps that reproduce the problem. For a difference from Pi, include the Pi version you compared against; ompig-slim pins the release shown by `ompig-slim verify`.
