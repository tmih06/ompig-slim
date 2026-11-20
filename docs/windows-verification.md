# Windows port verification

Native Windows x64 and ARM64 acceptance passed in
[CI run 44741164513](https://github.com/tmih06/ompig-slim/actions/runs/44741164513),
at source revision `cf19f16`. The complete CI run passed. The development host is
Linux ARM64; the Windows results below come from native GitHub-hosted runners,
not cross-builds.

A later documentation-triggered [run 44736570531](https://github.com/tmih06/ompig-slim/actions/runs/44736570531)
found a transient sharing violation in the x64 runtime updater. The runtime now
retries only sharing and lock violations for up to ten seconds per file mutation,
including backup removal, replacement, and rollback. Permanent errors still fail
immediately. Native regressions cover temporarily locked staged, installed, and
backup files. The complete follow-up CI run passed, including both native
installer/update suites and the x64 race detector.

## Native evidence

| Runner | Gate | Result |
| --- | --- | --- |
| Windows x64 | [Go tests, vet, and PowerShell tooling](https://github.com/tmih06/ompig-slim/actions/runs/44741164513/job/136304893017) | Passed |
| Windows ARM64 | [Go tests, vet, and PowerShell tooling](https://github.com/tmih06/ompig-slim/actions/runs/44741164513/job/136304893277) | Passed |
| Windows x64 | [Go race detector](https://github.com/tmih06/ompig-slim/actions/runs/44741164513/job/136304893188) | Passed |
| Windows x64 | [Release installation, update, and executable smoke](https://github.com/tmih06/ompig-slim/actions/runs/44741164513/job/136305931756) | Passed |
| Windows ARM64 | [Release installation, update, and executable smoke](https://github.com/tmih06/ompig-slim/actions/runs/44741164513/job/136305931670) | Passed |

Each architecture passed all 31 loopback installer/update tests. Each then proved
print mode, JSONL, native codemode writes, RPC, session persistence and resume,
durable recovery, and native Git Bash without WSL. The ConPTY scenario
submitted a prompt, observed its tool write and response, resized the terminal,
required a fresh repaint, and quit successfully. These jobs consume the exact
release ZIP after checking its checksum.

Each Go check job also passed 11 PowerShell tooling tests. These include a native
locked-handle regression that releases the file only after a move encounters a
sharing violation. The installer suite prefers Windows PowerShell 5.1; tooling
checks use PowerShell 7. Go coverage includes private-state ACLs, junction and
hard-link refusal, long paths, running-image updates, stale processes, shared
update locks, rollback, SDK timeouts, and case-insensitive ownership.

Native runs found defects that cross-vet missed. Fixes cover handle rights and
sharing, concurrent status reads, PowerShell module-path inheritance, literal
tilde paths, display separators, and transient executable-file locks. The ConPTY
harness also needed explicit standard-handle isolation from redirected CI
streams. The [decision trail](windows-support-decisions.tsv) records the failures
and changes. Neither native release gate is weakened or skipped on failure.

## Development-host checks

These commands passed on Linux ARM64:

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

The Unix installer suite passed 32 cases, and real-binary self-update passed 15.
The final Linux smoke run covered interactive and headless flows. All six release
archives passed checksum, executable-header, layout, and license verification.
Eight release-verifier tests and five portable Windows-smoke tests passed.
PowerShell tooling passed 11 tests under PowerShell 7.7 on Linux. Those portable
checks do not substitute for the Windows executions above.

Actionlint v1.7.12 accepted the workflow. Shell, Python, and PowerShell syntax
checks passed, as did `git diff --check`.

## Reproduce on Windows

```powershell
.\scripts\bootstrap-vendor.ps1
.\scripts\check-go.ps1 -Mode check
python scripts/test-powershell.py
python scripts/test-install-windows.py bin/native/ompig-slim/ompig-slim.exe
python scripts/smoke-test-windows.py bin/native/ompig-slim/ompig-slim.exe
```

Windows x64 also runs `check-go.ps1 -Mode race`. Stable and beta publication depend
on both native architectures passing their Go and executable gates.

## Limits and review

These results cover the hosted Windows runner images and tested flows, not every
Windows filesystem, terminal, or security policy. Hard termination between update
moves has an explicit backup recovery contract. Backup-only recovery is tested;
a real power-loss test was not performed.

No Windows latency, CPU, memory, or throughput baseline was measured. Release
builds retain `CGO_ENABLED=0`, `-trimpath`, and stripped symbols. This work makes no
Windows speedup or universal compatibility claim.

Laziness Protocol kept the existing PiG terminal and process runtime. Model the
Domain kept release selection typed and filesystem paths separate from display
paths. Prove It Works required native execution and observable tool, persistence,
and terminal results. Independent reviews were completed, but cross-model
diversity was not established.

Changes are committed and pushed in [PR #128](https://github.com/tmih06/ompig-slim/pull/128).
No merge or release was performed. Temporary command logs mentioned in the decision
trail are local diagnostics; the CI links above are the durable native evidence.
