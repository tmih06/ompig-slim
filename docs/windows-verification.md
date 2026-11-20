# Windows port verification

The implementation adds native x64 and ARM64 release artifacts and Windows-only
regression gates. Native Windows execution remains unverified in this session.
The development host is Linux ARM64. Cross-compilation does not prove Windows
ACLs, mapped-image replacement, PowerShell 5.1 installation, or ConPTY behavior.

## Executed checks

These commands passed on the development host:

```sh
make check
make race
make smoke
make test-install
make test-self-update
make release
python3 scripts/verify-releases.py bin/releases
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 make vet
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 make vet
python3 -m unittest discover -s scripts -p 'test_*.py'
python3 scripts/test-powershell.py
python3 scripts/smoke-test-windows.py --self-test
```

The PowerShell tests used PowerShell 7.7 on Linux. Nine tests exercised dependency
bootstrap, test-command failure propagation, profile isolation, directory links,
installer name validation, and actual .NET file and marker replacement. They do
not substitute for installation on Windows. Eight release-verifier tests covered
all six archive layouts, unsafe members, Windows aliases, checksums, PE headers,
license contents, and unexpected release assets. Five portable smoke tests checked
fixture and protocol logic without launching Windows processes.

The existing installer suite passed 32 cases. The existing real-binary self-update
suite passed 15 cases. The full Linux smoke suite passed before the final
Windows-only API and test corrections. Final shared-code changes then passed
`make check`, race checks, and real-binary self-update checks. All six final
release archives must be rebuilt and verified after any further source edit.

The pinned actionlint v1.7.12 accepted `.github/workflows/ci.yml`. All shell scripts
passed Bash syntax checks, and all Python scripts parsed. Native PowerShell scripts
passed PowerShell parsing. An initial combined check exceeded the tool timeout
while vet was running. Separate tests and vet passed, followed by a successful
full `make check`.

## Native release gates

The workflow runs native Go checks on `windows-latest` and `windows-11-arm`.
Windows x64 also runs the race detector. Both architectures execute the exact
release ZIP through loopback installation and update tests, then run CLI, RPC,
Bash, durable-state, and ConPTY smoke scenarios. Publishing depends on these jobs.
No Windows CI result was available for this unpushed working tree.

Run these commands on each native Windows target:

```powershell
.\scripts\bootstrap-vendor.ps1
.\scripts\check-go.ps1 -Mode check
python scripts/test-powershell.py
python scripts/test-install-windows.py bin/native/ompig-slim/ompig-slim.exe
python scripts/smoke-test-windows.py bin/native/ompig-slim/ompig-slim.exe
```

The Windows-only Go tests cover existing private-state ACLs, junction and hard-link
refusal, long paths, running-image updates, stale processes, shared update locks,
rollback, SDK timeouts, and case-insensitive ownership. They compiled and passed
cross-vet, but did not execute here. Hard termination between replacement moves
has an explicit backup recovery contract. The loopback installer tests exercise
backup-only recovery. A real power-loss test was not performed.

No Windows latency, CPU, memory, or throughput baseline was measured. Release
builds retain `CGO_ENABLED=0`, `-trimpath`, and stripped symbols. This work makes
no Windows speedup or complete compatibility claim.

## Review attention

The independent reviewer reported `codex-json/gpt-6.2-sol` as its harness model
identity. Reviews identified stale updater state, explicit ACL grants on existing
children, PowerShell null binding, relative path resolution, and native handle
access assumptions. Fixes and regression tests are present. Final API changes
still require native Windows execution. The reviewer had no independent backend
identity, so cross-model diversity was not established.

The [decision trail](windows-support-decisions.tsv) includes transient `/tmp`
command logs and two temporary design sketches. Those files are local evidence,
not durable links. This document records the reproducible commands and observed
results. The first trail timestamp was a placeholder and is corrected by a later
append-only row. No commit, push, release, or pull request was performed.
