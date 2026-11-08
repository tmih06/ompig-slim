# Project context

Technical orientation and maintenance rulings for coding agents working in this repository. Keep durable, project-specific knowledge here; keep user-facing setup and overview in [README.md](README.md). Verify changeable facts against source, configuration, and scripts rather than treating this document as a substitute for them.

## Identity and runtime

- `ompig-slim` is a Go application (`module ompig-slim`), built with Go 1.27.3 (`go.mod`).
- `main.go` registers PiG providers and the Lisp extension builder, registers the SQLite driver, then delegates arguments and runtime ownership to `github.com/MichaelKinsy/PiG/coding/cli.Main`.
- PiG v0.7.0 is a library dependency, vendored at `vendor/github.com/MichaelKinsy/PiG`. This is not a subprocess wrapper around an installed `pig` binary.
- Project-specific runtime changes live in the vendored PiG tree. Source provenance and the upstream baseline are recorded in `README.md`'s history; check current git history and diffs before relying on a commit reference.
- Build and test commands use `-mod=vendor`. The dependency lock is `go.mod`/`go.sum`; do not run `go mod vendor` without preserving and deliberately reapplying the personal runtime patches. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Repository map

- `main.go`: executable entry point and library registrations.
- `vendor/github.com/MichaelKinsy/PiG/`: PiG runtime plus personal patches; inspect this tree for CLI, coding runtime, provider, and TUI behavior.
- `scripts/`: Go checks, native builds, smoke tests, startup measurement, CLI benchmarking, release verification, and the user-facing curl installer.
- `benchmarks/`: pinned OMP/Pi npm manifests and lockfiles plus retained local measurement evidence used by CI comparisons.
- `docs/benchmarks.md`: benchmark build chain, mock workload, metric definitions, limitations, and measured optimization record.
- `Makefile`: canonical developer targets.
- `docs/cost-audit.md`: rationale and findings for subagent cost behavior.
- `AGENTS.md`: repository-wide change, comment, and commit rules.
- `CONTRIBUTING.md`, `SECURITY.md`, `NOTICE`, `LICENSE`: contributor, security, and legal guidance.

The root currently has very little application code outside the vendored library. For behavior changes, locate the actual implementation and existing tests in the library before adding a parallel implementation at the root.

## Working rules

- Keep the native core small. Prefer PiG's existing extension/skill mechanisms where they meet the need; modify runtime code when the behavior requires it.
- Treat vendored PiG as maintained source, not disposable generated output. Preserve unrelated local and vendored edits, and review exact diffs before staging.
- Keep dependencies pinned and changes focused. Preserve upstream notices and licenses.
- Add regression tests for bug fixes. Tests should use temporary profiles, fixture credentials, and local HTTP servers; do not rely on real credentials or paid API calls.
- Shared user state is intentional: the default agent directory is `~/.pi/agent`, with project settings under `.pi`. Be careful not to read, copy, or commit user credentials or profile data.
- Codemode executes with the user's OS permissions; it is not a security sandbox.

## Validation

Use the Makefile targets as the canonical interface:

```sh
make check
make race
make smoke
make build
make test-install
make test-self-update
```

`make check` runs tests and vet for the root and patched library through `scripts/check-go.sh`. `make smoke` builds the executable and runs its codemode, subagent, and web-tool scenarios from that build; `make smoke-built` runs the same scenarios against an executable CI already extracted from a verified release archive. `make race` repeats tests with the race detector. `make test-install` runs `scripts/test-install.py`, which serves fixture releases over loopback and asserts the installer's download, verification, placement, and failure behavior. `make test-self-update` runs `scripts/test-self-update.py` against a built executable, proving that `update` and `upgrade` replace the installed file and that an install without the ownership marker keeps PiG's own refusal. `make benchmark` compares an already-built executable with pinned OMP and Pi using a local mock model; CI reuses the verified release artifact and never rebuilds inside the benchmark job. See the scripts and CI configuration for exact coverage and platform-specific checks; do not infer that cross-compilation means a target was executed on that platform.

## Project-specific rulings

- Preserve the complete PiG CLI and runtime; `main.go` should remain a thin registration/delegation layer unless a concrete requirement needs otherwise.
- Keep Pi/OMP-compatible storage and configuration behavior. Avoid creating a separate `.pig` profile or changing shared identity formats without an explicit migration plan.
- When modifying account routing, preserve failure scope, session affinity, refresh behavior, and the rule against replaying a request after streamed output. Consult the implementation and tests for the exact contract.
- Treat provider catalogs, auth flows, and quota protocols as provider-specific. Validate protocol changes with local fixtures and keep network/live-service assumptions out of deterministic tests.
- Update this section when an enduring technical ruling changes; keep transient task notes and details discoverable directly from source out of this file.

## Feature behavior notes

### Release distribution and the curl installer

- `scripts/build-binaries.sh` produces four archives (`linux-x64`, `linux-arm64`, `darwin-x64`, `darwin-arm64`), each containing `ompig-slim/ompig-slim` plus README, LICENSE, NOTICE, and vendored license texts, and a `.sha256` sidecar naming that archive.
- CI's `release` job builds and verifies the archives once. The `publish` job runs only for a `refs/tags/v*` push, re-runs `scripts/verify-releases.py` on the downloaded artifact, and attaches the archives and checksums with `gh release create --verify-tag`. Branch and pull-request runs never publish, so the newest non-prerelease release is always a tagged, verified build.
- `scripts/install.sh` is piped from `raw.githubusercontent.com` and is the first code a user runs. It is POSIX-ish bash with no repository checkout: it selects `linux|darwin` and `x64|arm64` from `uname`, downloads `…/releases/latest/download/<archive>` (or `…/releases/download/<tag>/<archive>` when a version is pinned), and fetches the `.sha256` beside it.
- Placement contract: the executable is copied to a hidden staging file inside the destination, run with `--version` there, and only then renamed over `ompig-slim`. A checksum mismatch, missing member, symlink member, unusable archive, or executable that fails to start leaves any existing installation byte-for-byte unchanged and exits non-zero. Usage errors exit 2. The archive's README, notices, and license tree are intentionally not installed onto `PATH`.
- The checksum is fetched from the same release as the archive, so it detects corruption and truncation, not a compromised release. Do not describe the installer as a signature or supply-chain guarantee without adding one.
- The installer requires `bash`, `tar`, a gzip decompressor where `tar` shells out to one, and `curl` or `wget`; it reports each missing tool by name. It never invokes `sudo` and never edits shell startup files; when the destination is not on `PATH` it prints the export line instead.
- `scripts/test-install.py` is the regression suite: it serves fixture releases over loopback with a fake `uname` and a shell-script executable, so it is deterministic on both CI runners and needs no network. Extend it, not a manual smoke run, when the installer's contract changes.
- A successful install also writes `<config root>/ompig-slim-install` (`kind=archive`, `executable`, `download-base`, `installed-version`, `installer`), mode 0600, following PiG's own config-root resolution (`PIG_HOME`, else `~/.pi`, with a leading `~/` expanded). That marker is what `ompig-slim update` and `ompig-slim upgrade` treat as ownership; a config root that cannot be written warns rather than failing the install that already succeeded, because the executable is already in place by then.

### In-place `update` and `upgrade`

- `ompig-slim upgrade` is a second spelling of `ompig-slim update`, in every form (`upgrade`, `upgrade self`, `upgrade --all`, `upgrade --models`). PiG has no `upgrade` command, so the alias is rewritten before PiG parses arguments; otherwise PiG reads the word as a prompt and fails on the selected model.
- `internal/selfupdate` owns that dispatch; `main.go` calls `selfupdate.Handle` before `cli.Main`. `resolveRequest` is a pure parser of the `update`/`upgrade` argument list and mirrors PiG's package parser only far enough to tell a self-update from a package, model, extension, or help request: `--help`, `--models`, `--extensions` alone, `--extension <source>`, a positional source, and unknown flags all stay PiG's. `update self --extensions` resolves to `--all`, as in PiG.
- A self-update is claimed only when the install marker exists. Otherwise the canonical `update` reaches PiG unchanged and the user gets PiG's own install-method refusal and remediation, so npm, `go install`, Nix, container, and Piglet-Binary installations are never silently replaced.
- Why this exists: PiG's standalone tier requires a signed update manifest and an installation receipt under its own schema. This repository publishes neither a signed manifest nor a trust root, so on a curl-installed binary every `ompig-slim update` refused. `internal/selfupdate` closes that gap with the same trust decision the installer already makes: the release archive plus its published `.sha256`, fetched from the release named by the marker's `download-base`.
- The trust boundary is inherited from `scripts/install.sh` and stated in both places: the checksum detects corruption, a truncated download, or a swapped asset within one release; it is not a signature and does not survive a compromised release. Adding signature verification means publishing a signed manifest and a trust root, which is a separate change.
- Placement contract matches the installer: the executable is extracted to a hidden staging file in the destination directory, chmod 0755, and renamed over the running executable. A checksum mismatch, an archive without `ompig-slim/ompig-slim`, a non-regular or oversized member, an unreachable channel, or a release with no archive for this platform leaves the installed binary byte-for-byte unchanged and exits 1. `--force` reinstalls the current release. `update --all` runs the package phase through a child process first, matching PiG's order.
- `ompig-slim update --models` and `update --extensions` remain PiG's commands; the self-updater never intercepts them, and CI asserts that they do not print its progress lines.
- `scripts/test-self-update.py` proves the contract on the real executable: it installs the binary under test through `scripts/install.sh`, then serves newer fixture releases over loopback. `internal/selfupdate/selfupdate_test.go` covers version ordering, argument resolution, checksum and archive rejection, atomic placement, and the no-marker refusal without touching a real home directory.
- `OMPIG_SLIM_DOWNLOAD_BASE` and `OMPIG_SLIM_RELEASES_API` override the marker's download root and the GitHub releases endpoint for mirrors and tests. The running release version comes from `coding.Version`, not from a tag on the executable.

### Codex authentication and imported credentials

- Browser and device-code Codex sign-in use the Codex public client and the native `codex_cli_rs` originator. Email-only accounts can authenticate without a workspace ID; workspace headers are optional, while model access and quotas remain subject to the account plan and OpenAI's backend. See `vendor/github.com/MichaelKinsy/PiG/ai/oauth_openai_codex.go` and `vendor/github.com/MichaelKinsy/PiG/ai/auth_providers.go`.
- `/login` presents a searchable list of account and API-key options. Codex JSON imports use provider ID `codex-json`, through `/login codex-json` or `ompig-slim login codex-json`; imported models are selected under that provider.
- The importer persists only the access token, optional refresh token, account ID, and expiry. It gets the account ID from the token or `providerSpecificData.chatgptAccountId`; `expiresAt` (RFC3339) takes precedence over `expiresIn` (seconds), and JWT expiry caps either lifetime. Missing or expired expiry is rejected. Fields such as `idToken`, labels, and subscription metadata are discarded. See `vendor/github.com/MichaelKinsy/PiG/ai/omp_codex_json.go`.
- Imported accounts use the normal quota-aware account router and refresh-token rotation. Definitive refresh failures remove live secrets and routing blocks while retaining an OMP-compatible disabled row. Transport failures, cancellation, rate limits, server errors, and unclassified failures retain credentials. Cleanup is use-driven, not a background timer; browser-login Codex accounts are unaffected. See `vendor/github.com/MichaelKinsy/PiG/ai/omp_codex_json_storage.go` and the routing tests beside it.
- Sessions using the built-in ModelRuntime stream must also route compaction summaries through it without pre-resolving credentials. Explicit credentials pin a single account and bypass model-plan eligibility and account rotation; caller-supplied Agent streams retain their own auth behavior. See `vendor/github.com/MichaelKinsy/PiG/coding/session_summarization_auth.go`.

### Saved rate-limit resets

- `/usage reset` lists Codex and Claude reset inventory. The interactive spend picker includes only currently redeemable, unexpired resets and requires confirmation twice. Typed targets use `/usage reset <provider>/<account-key>`, including `codex/active` and `claude/active`.
- Codex spends the available credit expiring soonest; Claude rechecks inventory and spends the grant selected by its endpoint. A confirmed spend invalidates the account's quota cache and clears only matching quota blocks, preserving unrelated auth and policy blocks. A Claude 5h-only reset leaves weekly blocks intact.
- Reset redemption is an irreversible mutation and is not automatically retried. If its outcome is uncertain, check `/usage` before trying again. `Session.DispatchSlash("/usage reset")` returns the inventory without opening a picker; print-mode prompts do not dispatch built-in slash commands. See `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_reset.go`, `vendor/github.com/MichaelKinsy/PiG/ai/usage_reset.go`, and `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_usage_reset.go`.
