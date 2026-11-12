# Project context

Technical orientation and maintenance rulings for coding agents working in this repository. Keep durable, project-specific knowledge here; keep user-facing setup and overview in [README.md](README.md). Verify changeable facts against source, configuration, and scripts rather than treating this document as a substitute for them.

## Identity and runtime

- `ompig-slim` is a Go application (`module ompig-slim`), built with Go 1.27.3 (`go.mod`).
- `main.go` registers PiG providers and the Lisp extension builder, registers the SQLite driver, then delegates arguments and runtime ownership to `github.com/MichaelKinsy/PiG/coding/cli.Main`.
- PiG v0.7.0 is a library dependency, vendored at `vendor/github.com/MichaelKinsy/PiG`. This is not a subprocess wrapper around an installed `pig` binary.
- Project-specific runtime changes live in the vendored PiG tree. Source provenance and the upstream baseline are recorded in `README.md`'s history; check current git history and diffs before relying on a commit reference.
- Build and test commands use `-mod=vendor`. The dependency lock is `go.mod`/`go.sum`; do not run `go mod vendor` without preserving and deliberately reapplying the personal runtime patches. See [CONTRIBUTING.md](CONTRIBUTING.md).

### Repeated tool-call guard

`internal/loopguard` ports OMP's tool-call detector and corrective message.
`main.go` loads it as a hidden inline factory, including with `--no-extensions`,
for the primary session in every CLI mode. It has fixed upstream defaults:
threshold five, exempt tool `wait`; no settings UI or subagent integration.
State is per factory and resets on agent start, failed/aborted turns, no-call
turns and all-exempt turns. Sorted whole-turn call sets ignore call IDs,
argument key order and recursive `i`/`__intent` metadata, but retain duplicates
and array ordering. Changed results do not reset an identical call sequence.
PiG has no OMP `useless` result flag, so even idle exempt waits reset the count.

At and beyond the threshold, the awaited `turn_end` handler proposes a hidden
`tool-call-loop-redirect` custom-message draft, preserving earlier drafts and
continuation decisions. The Session persists it and refreshes model context
before the next request. Arguments/results are bounded diagnostic summaries.
This is a corrective steer, not a tool block or a hard cap: alternating calls,
changed arguments and repetition hidden inside one codemode call are not caught.
The integration test uses a local SSE provider and real read calls to verify
next-request visibility and persistence with extensions disabled.

## Repository map

- `main.go`: executable entry point and library registrations.
- `prompts/system.md`: this program's base system prompt, embedded into the binary at build time. Edit the Markdown, not Go source.
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
make test-broker-container
```

`make check` runs tests and vet for the root and patched library through `scripts/check-go.sh`. `make smoke` builds the executable and runs its codemode, subagent, and web-tool scenarios from that build; `make smoke-built` runs the same scenarios against an executable CI already extracted from a verified release archive. `make race` repeats tests with the race detector. `make test-install` runs `scripts/test-install.py`, which serves fixture releases over loopback and asserts the installer's download, verification, placement, and failure behavior. `make test-self-update` runs `scripts/test-self-update.py` against a built executable, proving that `update` and `upgrade` replace the installed file and that an install without the ownership marker keeps PiG's own refusal. `make test-broker-container` runs `scripts/test-broker-container.py` against the deployable image: identity, missing-secret refusal, UID/private directory, bearer admission, preference CAS, credential redaction, removed legacy routes, SQLite restart durability and two real clients without credential mirrors. `make test-broker-cluster` runs `scripts/test-broker-cluster.py` with one broker and two clients on a private Docker network: preference propagation/deletion/conflicts, live account access, shared quota blocks, restart, local-only connection configuration, late/provisioned clients, and unavailable-broker refusal without local credential fallback. Both need a Docker daemon and skip no check when it is available. Because both suites build their images from the checkout, CI tests the same Dockerfile the release workflow publishes, and the release job reuses that job's result rather than trusting an untested build. `make benchmark` compares an already-built executable with pinned OMP and Pi using a local mock model; CI reuses the verified release artifact and never rebuilds inside the benchmark job. See the scripts and CI configuration for exact coverage and platform-specific checks; do not infer that cross-compilation means a target was executed on that platform.

## Project-specific rulings

- Keep `main.go` a thin registration/delegation layer. The slim product has no sprites (not even an extension API for them), no cosmetic first-run setup, no easter eggs, no promotional announcements, no install/analytics telemetry or provider attribution, no HTML export (`/export`, `--export`, RPC `export_html`, and the `Session`/`rpcclient` methods are gone), no hosted sharing, no changelog presentation, no bundled harness documentation (`docs` CLI, prompt section, and `pigdocs` package are gone), and no built-in llama.cpp router manager. Mermaid, custom theme loading from repository `themes/`, and JSONL export/import remain. Retired commands are absent from menus and the interactive host rejects them locally rather than spend a model turn. The legacy settings keys for the removed features were deleted rather than kept for compatibility, so an old profile simply drops them.
- Keep Pi/OMP-compatible storage and configuration behavior. Avoid creating a separate `.pig` profile or changing shared identity formats without an explicit migration plan.
- **Product identity in the binary is `ompig-slim`, never `pig` or `PiG`.** `codingagent.AppName` (`internal/codingagent/paths.go`, beside `PackageName` and `APP_TITLE`) is the single source for Go; a user-facing string that names the program reads it rather than spelling the name. Source comments and upstream provenance may still say PiG.
  - The vendored tree needs care because Pi's own text is inherited: the help body (`coding/cli/help_upstream.txt`), the `pigCommands` table, `McpCommandOptions`' defaults, `rpcclient` and the extension scaffolds. The help body is rewritten by the `strings.NewReplacer` in `printHelp` (`coding/cli/args.go`), so a new Pi-help line that names the binary must get a replacer entry too; the others spell `ompig-slim` directly.
  - The startup header draws the static product mark from `internal/codingagent/startup_logo.go`. It has no click action, sprite state, animation worker, or cosmetic first-run dialog.
  - Documentation lives in the repository, not the executable or model prompt. No documentation bundle is written into user profiles. Configuration paths remain `PIG_HOME`, else `~/.pi`, project `.pi`; `XDG_CONFIG_HOME` is not consulted and `UsePiDirs()` remains true.
  - Retained deliberately: `PIG_*` env vars and `~/.pi/state/pigsdk`-style on-disk paths (they are interfaces), the `piglet` feature name, `pig-acp`, the `pig-package` keyword, upstream repository URLs and provenance, and the vendored Go import paths.
- When modifying account routing, preserve failure scope, session affinity, refresh behavior, and the rule against replaying a request after streamed output. Consult the implementation and tests for the exact contract.
- Treat provider catalogs, auth flows, and quota protocols as provider-specific. Validate protocol changes with local fixtures and keep network/live-service assumptions out of deterministic tests.
- Update this section when an enduring technical ruling changes; keep transient task notes and details discoverable directly from source out of this file.

## Remote broker

Authentication and credential ownership follow OMP's auth-broker architecture,
not its wire protocol. `internal/broker` owns the canonical SQLite vault and
portable preference sync; `cmd/ompig-slim-broker` runs it without an inference
runtime or gateway. Provider inference and login flows execute on clients;
subsequent OAuth refreshes execute only on the host. Setup, API contracts,
migration and failure semantics are canonical in
[docs/remote-broker.md](docs/remote-broker.md).

The server opens `<data-dir>/agent.db` using PiG's existing OMP-compatible
SQLite store. Broker and standalone mode must keep using this exact `agent.db`
file/schema: a closed database transfers directly in either direction without
logical export/import. Preserve real credentials, selection, tombstones and
quota blocks, not just redacted snapshot contents. Stop all old token owners
before switching modes; use SQLite-aware live backups or close/checkpoint before
copying the main file, and never restore over an open destination or stale WAL.
`vault_portability_test.go` pins the broker → standalone → broker round trip.
Snapshots redact OAuth refresh fields, including opaque retained
JSON, with `__OMPIG_REMOTE_REFRESH__`. Direct clients remain trusted with API
keys/access tokens. Account selection, logout tombstones and quota blocks are
broker-owned. Manual refresh and immediate/one-minute proactive sweeps use the
same SQLite account lease, recheck the observed access-token version and persist
before returning. Sweeps target expiry within five minutes, never overlap, and
continue past account failures. Cancellation cannot discard an in-flight rotated
token. Existing provider-specific definitive-failure classification is reused;
unclassified/transient failures retain credentials. An external provider crash
window still exists between exchange and database commit.

Vendored `ai.CredentialStoreFactory` is installed once before runtimes open and
selects `remoteStore` for exactly the configured profile. It returns the
application-owned account-store interface rather than assuming local SQLite.
Runtime credential overlays and the registry's live credential view are unwrapped
only to find remote OAuth refresh support; the local provider callback must never
see a refresh sentinel. Capability discovery follows the current backing store,
so replacing a registry store cannot retain the previous owner's refresh path. No
credential snapshot is cached on disk and no local credential database is opened
for a broker profile. A down, refusing or invalidly configured broker leaves the
CLI repair commands usable but never silently falls back to a local vault.
Explicit environment/runtime key overrides retain existing precedence.

The hosted credential-document format and client-owned HTTP lease protocol are removed. Local credentials require
`broker migrate --from-local`: it inserts missing enabled identities only, never
overwrites rotations or revives logout, and leaves local recovery copies
untouched. Server and client upgrades must be coordinated; this is not OMP wire
parity.

Credential reads and writes use the broker's account store directly. Benchmarks
in `internal/broker/credential_benchmark_test.go` cover live reads and account
pool scaling. Live snapshot timing remained within noise; its credential codec/SQL
work is deliberately unchanged.

`main.go` starts discovery before PiG reads preferences when a saved
`remote-broker.json` or `OMPIG_BROKER_URL` is configured. Native `/login`
and unattended `broker setup|show|clear` share private device-local bootstrap
configuration. Setup validates an origin and resolves the secret from flags,
mounted file, environment, or stdin; `--verify` contacts the service first.
A changed destination needs `--force`. Restart activates a changed connection.
The bootstrap secret is never uploaded as a provider credential or preference.
Bearer admission remains environment-configured and in-memory hashed,
constant-time compared, not SQLite-backed. TLS is operator-owned.

Preferences retain their separate startup/five-second/normal-exit reconciliation,
allowlist, private first-adoption backups and CAS/common-ancestor conflict rules.
A document failure does not stop other documents; startup preference/network
failures retain the configured client for background retry and exit flush.
Invalid bootstrap configuration still requires repair and restart. File sync does
not hot-apply every runtime setting; session reload/restart remains necessary.
Sessions, project config and arbitrary SQLite tables never synchronize.
Embedded preference API keys remain trusted plaintext. The broker needs outbound
OAuth connectivity and host-specific refresh configuration; client-only custom
provider refresh code cannot run remotely without broker registration.

### The workspace image

`Dockerfile.workspace` publishes `ghcr.io/<owner>/ompig-slim-workspace` for linux/amd64 and
linux/arm64 on the same version tags as the release. It uses Alpine as the runtime base and a
static, stripped `ompig-slim` binary from the Go build stage. The package set is deliberately
small: shell and BusyBox provide the base Unix commands, while `git`, `fd`, `ripgrep`, `jq`,
`curl`, SSH, Python, and archive/patch utilities cover common agent work without bundling
language toolchains. The image runs as an unprivileged `agent` user in `/workspace`; projects
that need compilers or runtimes should layer them separately.

### The broker image

`Dockerfile.broker` builds a separate vault executable, cross-compiling natively
for linux/amd64 and linux/arm64. `VERSION` reaches `--version` through
`-X main.version`. The runtime uses UID/GID 10001 and re-tightens `/data` to
0700. A process lock rejects duplicate owners. Preserve the SQLite WAL along with
the database in backups, or stop the service first. Shutdown drains HTTP and
stops the refresher before closing the vault. Only `/healthz` is public;
never widen the image health check to a credential-bearing route.

The existing container/release gates build the actual checkout. Container HTTP
coverage includes authentication, redaction, SQLite persistence, preference
CAS, restart and real clients without local credential mirrors. The private-network cluster suite covers shared account access/blocks,
preference conflict and deletion, provisioned/late clients, restart and failure
without local credential fallback. Go fixture tests cover server-side refresh,
coalescing, cancellation durability, migration, CAS and direct inference.

## Feature behavior notes

### Skill calls in the composer

The composer offers skill completion after whitespace anywhere in a normal prompt; built-in commands keep the leading-only position. A skill selection leaves the draft open, so one prompt can carry several skills. Registered `/skill:<name>` tokens draw as compact badges instead of raw slash syntax, while the buffer keeps the slash form so history, undo, external editing and submission are unaffected. Cursor navigation and deletion treat a recognized token as one unit, and wrapping and mouse columns are measured in badge cells rather than source cells.

`tui/skill_tokens.go` owns token boundaries and the literal-input exclusions, and `internal/codingagent/skills.go` shares them (D252), so the editor and the expander cannot disagree about what counts as an invocation. On submission every known token is loaded once, in first-appearance order, ahead of the original prompt text. Retaining the source tokens preserves their inline positions and repetitions in persisted transcripts. Already-expanded messages are not expanded again when interactive preflight hands them to Session. Pending queue rows and Alt+Up restoration strip the leading model-only blocks with `ParseExpandedSkillMessage`, retaining the source tokens and prose so editing and resubmitting reloads skills without exposing their bodies. The transcript parses all leading expansion blocks and renders one user bubble, with the same compact badges as the composer in their original positions. The global expand key discloses the loaded bodies within that bubble, not as separate calls. Older messages without retained tokens show their badges before the trailing prose. Badge decoration follows extension Markdown transforms and precedes wrapping, so transforms receive source text and layout measures badge cells. A completed leading skill leaves automatic completion in prose mode; explicit subsequent slash and attachment triggers still work. An unknown token stays literal; a read or parse failure expands nothing at all rather than submitting a partial load. Command arguments and local-execution input remain literal throughout. The expansion is shared by interactive, Session, print and RPC paths, so it is a runtime behavior, not an editor feature.

### `@path` and `@url` mentions in a prompt

Composer attachment search passes an escaped pattern after fd's `--` option boundary, so a filename beginning with `-` remains searchable. Its fd filtering is case-insensitive, matching the ranker's comparisons rather than silently dropping mixed-case matches before ranking.

A prompt's `@path` and `@url` mentions are read before the model runs, so it does not spend a turn calling read. This ports oh-my-pi's mention auto-read, extended with URLs because this runtime's `read` already treats one as its other source. Extraction requires a whitespace or opening-punctuation boundary, so prose such as an email address never becomes a read; quoted mentions may contain spaces. A filesystem mention that is not an existing path is prose and is dropped. A mention that names a URL is fetched through `read`'s own fetch and rendering, so an auto-read and a later `read` of the same URL report the same text; a refused URL keeps a `fetch failed` row because the user named a document. Local content is text only: files truncate with `read`'s limits, directories list as `read` lists them, and non-text files report their size instead of being decoded. See divergence D253.

Delivery is one custom message of type `file-mentions` whose content is the `<file path="…">` blocks the model reads and whose details hold the structured rows, so a resumed session rebuilds the transcript card without re-reading. A new turn sends the user message and that message in one batch, and a steer or follow-up enqueues them as one queue slot: the queues drain one slot per turn by default, so separate enqueues would let the model answer the prompt before reading the files it named (D254). The read is therefore atomic with the prompt, and a refused run consumes neither (D255).

A submission is dispatched on the owner loop, where a fetch would freeze the editor for its whole timeout. A mention set containing a URL is read on a worker and delivered back through the loop; that worker holds the preflight slot until it delivers, so submissions stay in order and a later prompt cannot overtake a slower fetch. A local-only set is read inline. The worker receives the working directory captured on the loop, so a `/cd` or session swap cannot race it.

The transcript draws one indented row per mention under the submitted prompt, replacing oh-my-pi's `fileMention` block, which has no message role here. Rows are one line each and never wrap: a tree rail only stays aligned that way, and a path from the prompt is sanitized to a single display line before it is drawn.

### GitHub issue and pull-request reads

The built-in `read` tool accepts `issue://N` and `pr://N`, resolving the repository from `GITHUB_REPOSITORY` or the current checkout's `origin` remote. Explicit `issue://owner/repo/N` and `pr://owner/repo#N` forms work without a checkout. It reads the GitHub REST API directly, never invoking `gh` or a shell, and includes the title, state, body and conversation comments. `GITHUB_TOKEN` and `GH_TOKEN` are sent as bearer credentials when set; public repositories work anonymously. `GITHUB_API_URL` overrides the API origin for GitHub Enterprise and deterministic tests. Invalid references remain ordinary filesystem paths, while a valid reference without a resolvable repository returns a tool error.

### The base system prompt

The prompt's opening text lives in `prompts/system.md` at the repository root and is embedded into the binary with `go:embed`; nothing reads the file at runtime. Edit the Markdown, not Go source. `prompts.System()` returns it trimmed.

The vendored runtime composes the rest and cannot be bypassed from the root: the prompt-building package is `internal/codingagent/prompts`, and Go forbids an external module from importing a vendored `internal` path. So the seam is an exported field, `cli.MainOptions.SystemPromptPreamble`, next to the existing `ExtensionFactories`. `main.go` passes `prompts.System()`; `cli.Main` assigns it to the package-level `prompts.Preamble` before any session builds a prompt.

Only the preamble is replaceable this way. The tool list, rules, project-context, and skills sections still compose around it, because they depend on runtime facts (selected tools, discovered skills, loaded context files) the root cannot supply. A replaced preamble therefore keeps the stock prompt's structure; it becomes the whole `<preamble>` section and nothing else moves.

Two properties matter for keeping this honest. `prompts.Preamble` is read at render time by both `BuildDefaultPrompt` and `BuildSystemPromptSections`, so startup, resource reload, and per-session rebuild cannot disagree about it; `TestAllBuildersSeeOverride` pins that. An empty or whitespace-only value falls back to the stock paragraph, so a build that never sets the field is byte-identical to before, and clearing it cannot produce a prompt with no opening sentence.

Do not grow this into a prompt file plus a `SYSTEM.md` override layered on top. The runtime already discovers a trusted project `SYSTEM.md` (and `APPEND_SYSTEM.md`) and switches `AppendMode` to `replace` when it exists; that is the user-facing override and it deliberately wins over this embedded default.

### Release distribution and the curl installer

- `scripts/build-binaries.sh` produces four archives (`linux-x64`, `linux-arm64`, `darwin-x64`, `darwin-arm64`), each containing `ompig-slim/ompig-slim` plus README, LICENSE, NOTICE, and vendored license texts, and a `.sha256` sidecar naming that archive.
- CI's `release` job builds and verifies the archives once. The `publish` job runs only for a `refs/tags/v*` push, re-runs `scripts/verify-releases.py` on the downloaded artifact, and attaches the archives and checksums with `gh release create --verify-tag`. Branch and pull-request runs never publish, so the newest non-prerelease release is always a tagged, verified build.
- The `workspace-image` job publishes `ghcr.io/<owner>/ompig-slim-workspace` for linux/amd64 and linux/arm64 on the same release tags. It is a separate Alpine-based, unprivileged runtime image for agent work; it contains the static CLI and selected repository/network/archive tools, not language toolchains.
- `scripts/install.sh` is piped from `raw.githubusercontent.com` and is the first code a user runs. It is POSIX-ish bash with no repository checkout: it selects `linux|darwin` and `x64|arm64` from `uname`, downloads `…/releases/latest/download/<archive>` (or `…/releases/download/<tag>/<archive>` when a version is pinned), and fetches the `.sha256` beside it.
- Placement contract: the executable is copied to a hidden staging file inside the destination, run with `--version` there, and only then renamed over `ompig-slim`. A checksum mismatch, missing member, symlink member, unusable archive, or executable that fails to start leaves any existing installation byte-for-byte unchanged and exits non-zero. Usage errors exit 2. The archive's README, notices, and license tree are intentionally not installed onto `PATH`.
- The checksum is fetched from the same release as the archive, so it detects corruption and truncation, not a compromised release. Do not describe the installer as a signature or supply-chain guarantee without adding one.
- The installer requires `bash`, `tar`, a gzip decompressor where `tar` shells out to one, and `curl` or `wget`; it reports each missing tool by name. It never invokes `sudo` and never edits shell startup files; when the destination is not on `PATH` it prints the export line instead.
- `scripts/release_fixture.py` owns the release layout both suites serve: `archive_bytes` builds the member layout `build-binaries.sh` publishes, and `ReleaseServer` exposes GitHub's `releases/download/<tag>/` and `releases/latest/download/` paths plus the `releases/latest` JSON the updater reads. A change to the release contract is therefore proven by both suites at once instead of each drifting separately. `publish` takes pre-encoded archive bytes so a suite that republishes a large payload can cache the encoding.
- `scripts/test-install.py` is the installer's regression suite: it serves fixture releases over loopback with a fake `uname` and a shell-script executable, so it is deterministic on both CI runners and needs no network. Extend it, not a manual smoke run, when the installer's contract changes.
- Install and update share one visual language: the two-line half-block startup mark from `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/startup_logo.go` (including `in`, `GO`, and the `-slim` tag), plus a 32-cell progress meter drawn with `━`/`─`. The two renderings are duplicated by necessity — `scripts/install.sh` must run with no Go toolchain and `internal/selfupdate/ui.go` must run with no shell — so the art, the bar glyphs, and the bar width must be changed together in both files.
- Presentation is terminal-only. `scripts/install.sh` gates on `[[ -t 1 ]]` and `isTerminal` gates the updater on a character device, so a redirected log or a CI pipe receives plain lines with no escape sequences, no carriage returns, and no banner mid-run. The mark is printed once per run after the platform is settled, so a usage error or an unsupported-platform refusal prints only its own message.
- The shell meter is latency-sensitive in a way the Go meter is not. The poll loop runs every 0.1s for the length of a download, so anything it forks is paid ~10 times a second: formatting the two byte counts with `awk` cost ~6.7ms per iteration, and a `$(...)` helper would still fork a subshell. `human_bytes` therefore assigns `REPLY` through integer arithmetic, and `draw_progress` uses `printf -v` for the bar, so the loop forks nothing except the single `wc -c` that reads the growing file. The tenth is rounded half-to-even to match `printf "%.1f"`, so the shell and Go meters print identical digits; `scripts/test-install.py` and a sampled comparison against `humanBytes` cover that parity. Measure this loop with `strace -f -e trace=execve` (count execs per iteration) and by timing the body in a loop; wall time of a real install is dominated by the network and will not show it.
- Progress is real byte progress, not a spinner. The updater wraps the response body in a counter and compares against `Content-Length`; the installer polls the growing file in a background loop against a length read in a separate `HEAD` request, because neither `curl` nor `wget` will report progress to a pipe without its own meter. Both degrade to a count-only line when the length is unknown (chunked responses, redirects to signed URLs), and both redraw only when the whole percentage changes.
- `scripts/test-install.py` and `internal/selfupdate/ui_test.go` assert the startup mark in a pipe has no escape sequences; the Go tests also assert that a non-terminal meter writes nothing and that an unchanged percentage does not redraw.
- A successful install also writes `<config root>/ompig-slim-install` (`kind=archive`, `executable`, `download-base`, `installed-version`, `installer`), mode 0600, following PiG's own config-root resolution (`PIG_HOME`, else `~/.pi`, with a leading `~/` expanded). That marker is what `ompig-slim update` and `ompig-slim upgrade` treat as ownership; a config root that cannot be written warns rather than failing the install that already succeeded, because the executable is already in place by then.

### In-place `update` and `upgrade`

- `ompig-slim upgrade` is a second spelling of `ompig-slim update`, in every form (`upgrade`, `upgrade self`, `upgrade --all`, `upgrade --models`). PiG has no `upgrade` command, so the alias is rewritten before PiG parses arguments; otherwise PiG reads the word as a prompt and fails on the selected model.
- `internal/selfupdate` owns that dispatch; `main.go` calls `selfupdate.Handle` before `cli.Main`. `resolveRequest` is a pure parser of the `update`/`upgrade` argument list and mirrors PiG's package parser only far enough to tell a self-update from a package, model, extension, or help request: `--help`, `--models`, `--extensions` alone, `--extension <source>`, a positional source, and unknown flags all stay PiG's. `update self --extensions` resolves to `--all`, as in PiG.
- A self-update is claimed only when the install marker exists. Otherwise the canonical `update` reaches PiG unchanged and the user gets PiG's own install-method refusal and remediation, so npm, `go install`, Nix, container, and Piglet-Binary installations are never silently replaced.
- Why this exists: PiG's standalone tier requires a signed update manifest and an installation receipt under its own schema. This repository publishes neither a signed manifest nor a trust root, so on a curl-installed binary every `ompig-slim update` refused. `internal/selfupdate` closes that gap with the same trust decision the installer already makes: the release archive plus its published `.sha256`, fetched from the release named by the marker's `download-base`.
- The trust boundary is inherited from `scripts/install.sh` and stated in both places: the checksum detects corruption, a truncated download, or a swapped asset within one release; it is not a signature and does not survive a compromised release. Adding signature verification means publishing a signed manifest and a trust root, which is a separate change.
- Placement contract matches the installer: the executable is extracted to a hidden staging file in the destination directory, chmod 0755, and its embedded extension SDKs are staged by the replacement binary before it is renamed over the running executable. A checksum mismatch, an archive without `ompig-slim/ompig-slim`, a non-regular or oversized member, an unreachable channel, a failed SDK preparation, or a release with no archive for this platform leaves the installed binary byte-for-byte unchanged and exits 1. `--force` reinstalls the current release. `update --all` runs the package phase through a child process first, matching PiG's order.
- `ompig-slim update --models` and `update --extensions` remain PiG's commands; the self-updater never intercepts them, and CI asserts that they do not print its progress lines.
- `scripts/test-self-update.py` proves the contract on the real executable: it installs the binary under test through `scripts/install.sh`, then serves newer fixture releases over loopback through the same `release_fixture` the installer suite uses. `internal/selfupdate/selfupdate_test.go` covers version ordering, argument resolution, checksum and archive rejection, atomic placement, and the no-marker refusal without touching a real home directory.
- `OMPIG_SLIM_DOWNLOAD_BASE` and `OMPIG_SLIM_RELEASES_API` override the marker's download root and the GitHub releases endpoint for mirrors and tests. The running release version comes from `coding.Version`, not from a tag on the executable.

### Codex authentication and imported credentials

- Browser and device-code Codex sign-in use the Codex public client and the native `codex_cli_rs` originator. Email-only accounts can authenticate without a workspace ID; workspace headers are optional, while model access and quotas remain subject to the account plan and OpenAI's backend. See `vendor/github.com/MichaelKinsy/PiG/ai/oauth_openai_codex.go` and `vendor/github.com/MichaelKinsy/PiG/ai/auth_providers.go`.
- `/login` presents a searchable list of account and API-key options. Codex JSON imports use provider ID `codex-json`, through `/login codex-json` or `ompig-slim login codex-json`; imported models are selected under that provider.
- The importer persists only the access token, optional refresh token, account ID, and expiry. It gets the account ID from the token or `providerSpecificData.chatgptAccountId`; `expiresAt` (RFC3339) takes precedence over `expiresIn` (seconds), and JWT expiry caps either lifetime. Missing or expired expiry is rejected. Fields such as `idToken`, labels, and subscription metadata are discarded. See `vendor/github.com/MichaelKinsy/PiG/ai/omp_codex_json.go`.
- Imported accounts use the normal quota-aware account router and refresh-token rotation. Definitive refresh failures remove live secrets and routing blocks while retaining an OMP-compatible disabled row. Transport failures, cancellation, rate limits, server errors, and unclassified failures retain credentials. Cleanup is use-driven, not a background timer; browser-login Codex accounts are unaffected. Usage probes honor the resolver's retirement immediately: a removed account is omitted from that snapshot rather than queried with its old bearer or shown as unavailable. Temporary refresh failures still probe the retained credential. See `vendor/github.com/MichaelKinsy/PiG/ai/omp_codex_json_storage.go` and the routing tests beside it.
- Same-provider fallback honors persisted quota resets from sibling processes: successfully mirrored local deadlines yield to deleted or shorter stored blocks on the next routing check. Failed or unverified writes retain local protection, and unrelated auth/policy scopes remain blocked. See `vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go` and `account_routing_reset_test.go` beside it.
- Sessions using the built-in ModelRuntime stream must also route compaction summaries through it without pre-resolving credentials. Explicit credentials pin a single account and bypass model-plan eligibility and account rotation; caller-supplied Agent streams retain their own auth behavior. See `vendor/github.com/MichaelKinsy/PiG/coding/session_summarization_auth.go`.

### Todo tool and its stop-time reminder

`todo` is OMP's task list, ported as native runtime code in `vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/todo`. Both halves are native — there is no extension anywhere in the feature, and `main.go` passes no `ExtensionFactories` at all.

The tool is a **built-in tool**: `base_tool.go` registers it with `tools.RegisterExtendedBuiltinTool` from `init()`, exactly as `websearch` and `codemode` do. So `todo` is created with every session, survives `--no-extensions`, is dropped by `--no-builtin-tools`, and never appears in the startup Extensions list. It is listed in `defaultToolNames` (`internal/codingagent/settings_default_tools.go`) and in `DefaultToolSnippets` (`internal/codingagent/prompts/sections.go`) so it is active at startup and named in the prompt.

The stop-time reminder is a **native before-settle hook**, not an extension handler. `agent_before_settle` is otherwise reachable only through `extension.API`, so `internal/codingagent/native_settle_hooks.go` adds a small hook registry (`RegisterNativeSettleHook` / `NativeSettleHooks` / `HasNativeSettleHooks`) that `Session.runBeforeSettleBoundary` consults after the extension handlers. A hook reads `NativeSettleState` (outcome, branch, last assistant text) and calls `Deliver(customType, content)`, which appends a hidden custom message and continues the run; the same `flushPendingCustomMessages` call that carries an extension's message carries a hook's. The boundary pass is skipped only when *neither* an extension handler nor a native hook is registered.

Why the code lives in the vendored tree: a built-in is linked code, and neither direction of the import works from here. A package outside the module cannot import `internal/codingagent/tools` (Go forbids another module's `internal` tree — `use of internal package ... not allowed`), and a package inside the vendored tree cannot import the module that vendors it. `websearch` and `codemode` sit there for the same reason. Consequence: `go mod vendor` will delete the package; reapply it along with the other personal runtime patches, and re-add its `vendor/modules.txt` line and the `todo_on.go` link shim.

State lives only in each tool result's `details`. Every call re-reads the newest state-changing `todo` result on the session branch (`vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/todo/snapshot.go`), so resume, rewind and fork need no persistence of their own, and the list can never disagree with the transcript. A `view` result and an errored result are both skipped as state: the first changed nothing, the second persisted nothing.

`view` is also exempt from list normalization. `todo.Apply` runs `normalizeInProgress` for a mutating op to keep at most one task active, but that is a write: running it for a read-only op would return a copy the transcript never accepted, demoting a second `in_progress` task in the rendered list. `Op.readOnly` decides this, and `TestViewDoesNotNormalize`/`TestWriteOpsStillNormalize` pin both directions. The tool short-circuits a view before calling `Apply` at all, so the library-level guarantee backs a path the tool no longer takes.

An omitted `op` is repaired in `Definition().PrepareArguments`, **not** in `execute`. The harness validates a call's arguments against the tool schema before the tool runs, so `required: ["op"]` rejects the payload first: the repair that used to sit in `execute` was unreachable, and every op-less call died as `Validation failed for tool "todo": op: missing`. The `ArgumentPreparer` hook runs ahead of validation (the same seam `edit` uses for legacy argument shapes), so its output is the payload both validation and `execute` see. Only shapes that pin exactly one op are repaired: a non-empty `list` is `init`; a non-empty `items` beside a `phase` is `append`; a non-empty `items` alone is `init`, because `append` rejects a missing phase; a non-empty `init` member is `init` with its phases moved into `list` (models write `{"init": [...]}` when they mistake the op for a call signature, and the value arrives either as the phase array or as that array's JSON text); an `op` of `null` or `""` counts as absent so a payload that still names its arguments resolves; and an empty object is `view`, the only argument-less op, which is read-only and therefore safe to guess. Anything else is left to the schema so a genuinely ambiguous call still reaches the model as a correctable error. `prepare_arguments_test.go` pins the repaired and rejected shapes, `tool_call_test.go` drives them through `agent.RunToolCall`, and both fail if the hook is removed from the definition.

A call whose own argument text was not JSON reports that parse failure, never a missing member. `ai.ToolCall` keeps `ArgumentsError` beside the empty `Arguments` such a call carries, and `agent.prepareToolCall` reports it before argument preparation and schema validation run. Without that check the model is told `op: missing` about a payload it never sent, which names the wrong problem and hides the text that actually failed. `scripts/smoke-test-todo-arguments.py` proves both fixes end to end on a built binary through a loopback model, and is part of `make smoke`.

The harness behavior is the part that makes the tool matter. `registerSettleHook` installs the native hook, which reports continue when the list still has open work and delivers a hidden `todo-reminder` custom message. It stays silent, deliberately, when: the run did not complete normally; no list exists; every open task is `blocked`; the agent's last message asks the user something; the list is unchanged since the previous reminder; or the per-run budget of 3 reminders is spent. A `blocked` task is open but never actionable, which is the sanctioned way for an agent to stop: it keeps the list honest while letting the turn end.

`sequential` execution serializes the calls but does not make one call's result visible to the next, so two `todo` calls in one batch each read the same prior list and the later one wins. The tool description therefore instructs one op per call beside the next real action; do not treat batching as safe.

The content is a line-for-line port of OMP's `todoToolRenderer` (`packages/tui/src/tools/todo.ts`) in `vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/todo/render.go`: one `Todo N tasks` line, roman-numeral phase headers when there are several phases, `├─`/`└─` tree gutters, `☑`/`☐` checkboxes, dim `closed/total` progress per phase, and a collapsed walking viewport that leads with the last closed task and caps at 8 rows. The content is reproduced rather than approximated because it is the only place a user watches a plan advance; a layout that differs by a gutter or a windowing rule reads as a different tool.

The content is drawn **unframed**: the tool sets `RenderShell: extension.ToolRenderShellUnframed`, so the rows are the card's whole presentation, with no border, no status bar and no lifecycle background. The list is already a structured block — a title line, phase headers and tree gutters — so the frame would only box it and repeat its title. `ToolRenderShellUnframed` is a pig addition to the `default`/`self` pair (`tool_execution_definition.go` `Unframed`, set from both renderer factories); the frame itself lives in `renderRichFrame`, which only `renderSelfShell` calls for `Rich` cards, so opting out is a routing change rather than a renderer concern. A frame-less card also skips the mouse inset `HandleMouse` applies inside a border.

Two vendor limitations shape the renderer, both handled in it rather than by patching further. First, OMP's `mergeCallAndResult` has no counterpart here, and the card substitutes a name-and-args fallback when a call renderer returns nil, so `renderCall` draws the streaming header and then nothing once `IsPartial` clears. Without that, a settled card shows a stale `● Todo` line above the list. Second, the vendored `Theme` exposes foreground tokens but not OMP's symbol table, so checkbox, tree and tool glyphs are constants in `render.go` holding OMP's default-theme values. A pending todo matched to a live subagent renders dim rather than accent, because no subagent registry is wired into a renderer here.

Interactive todo calls retain compact inline transcript receipts, both live and on resume. `todo_tool_card.go` uses the existing lifecycle-colored status row: `Todo: <op> <target>`, completed/total counts, blocked/dropped counts when present, and recorded timing. Successful receipts stay one visible row even under Ctrl+O; the full list is never duplicated there. Failed receipts show a short reason and disclose at most six diagnostic rows when expanded. Both surfaces condense the reason through `todoErrorSummary`: a schema rejection's first line is only its header (`Validation failed for tool "todo":`), and everything from `Received arguments:` on is a copy of the model's own payload, so the summary takes the first `- <path>: <problem>` bullet and drops the echo. The transcript keeps the untouched result text for the model and the session log; only presentation is condensed. The Tasks panel renders that one truncated row directly beneath the plan's summary line and above its tasks: the panel is a twelve-row overlay, so a long list would otherwise push the failure below its collapsed viewport, and keeping the summary first keeps the panel saying which list the call failed to change. With no plan yet the reason is the panel's only row. A tool-level rejection, whose first line is already its own diagnostic (for example `Errors: Missing phase name for append operation`), is left as that line. Nested calls retain the ordinary parent-card behavior while still updating the widget. Persisted results remain unchanged for the model and reminder. `interactive_info_widgets.go` retains the latest successful non-view snapshot, using the registered unframed renderer inside the Tasks widget so task styling stays canonical. Failed calls display an error alongside the last good list; an empty mutation clears it. Branch rebuilds reset the widget and read tool-result records across the entire branch, not just the post-compaction context. Runtime replacement clears the old plan. Ctrl+O still controls transcript tools, not widgets.

### Prompt-anchored information widgets

`tui.InfoWidgets` is an owner-loop component showing exactly one candidate at a time, in priority order. Tasks (including a todo error) take priority over Quota; quota appears only when no todo list/error exists. A completed but uncleared list still counts as a list. Only the selected renderer runs; switching candidates resets its scroll offset. The whole panel opens upward at the bottom-right, attached directly to the prompt's top border. The persistent `▾ Widgets` / `▸ Widgets` control lives beside the existing quota label on that border; it has its own hit box rather than making quota clickable. Alt+W and the border control share the same toggle. Global collapse removes every panel row and transcript hit box, not just task details, so selecting/copying responses cannot hit the hidden widget. Reopening preserves per-section disclosure. The Tasks renderer supplies a width-aware working row: the active task, otherwise the first pending task, otherwise the last task. Opening either disclosure or advancing the working row reveals it with up to two context rows; ordinary repaints preserve manual scrolling. Rejected calls instead reveal the summary/error at the top. Providers without a working row retain their manual offset across collapse. Section headers still toggle their own summary/detail view; wheel scrolling or Alt+PageUp/Alt+PageDown reaches longer content.

There is no top-right relocation, dock fallback, transcript reflow, or space reservation. Panels are capped at 12 rows and the existing visible height above the prompt. Scrolling moves only interior rows: the first header and final border stay fixed, with up/down navigation hints inside the bottom border. Header mouse coordinates account for pinned chrome, and every content row remains reachable. Only the visible interior rows are framed; scrolling never styles offscreen rows or mutates provider-owned slices. Hidden panels and viewports with fewer than three rows skip provider rendering entirely. Fullscreen `InfoWidgetLayout` wraps the complete viewport and finds the allocated editor slot, so status or extension widgets above input cannot separate the panel from its source point. Placement uses only geometry, leaving transcript painting to the frame painter instead of painting and slicing it twice. Regular mode measures only the prompt and rows below it, composites into a copy of existing rows, and registers passive mouse bounds beneath normal modals without claiming modal focus. Prompt text and footer rows remain untouched. Collapsing restores document-only painting and response mouse routing in both modes.

The editor gives the border control priority within its existing trailing-cell budget, then offers the remaining cells to quota; working status and centered scroll hints remain intact. A narrow border can reduce the control to its disclosure glyph. Quota uses the existing minute-refreshed, model/tier-aware cache and never performs credential reads or network work during paint. The full widget-only todo renderer resolves through the extended-builtin registry even without an extension runner. Its private card is not registered for transcript renderer rebinding, which always keeps todo receipts compact.

**Colors follow the palette tokens**, never literal escapes or ad-hoc hex. The frame lines take `border`, the title `accent` in bold with its `Alt+W` hint in `dim`, and the scroll hints inside the bottom border `dim` as well, matching `usage_dashboard.go` and `model_overlay_chrome.go`. The title deliberately does not borrow `border`: the bundled palettes aim that token at lines, so `titanium`'s `#2a3038` would leave the header nearly invisible against its own frame — a regression every width-based assertion would still pass. The border control takes `accent` so it reads as the panel's one interactive affordance rather than another quota readout, and the `▾`/`▸` marker carries the open/collapsed state on its own, so the label still reads on a colorless terminal. Quota rows use `muted` for the window label, `accent` for the provider, `dim` for the reset countdown, and color the remaining percentage by pressure — `error` at exhausted, `warning` at 80% used, `success` below — through the same thresholds as its bar and `tui.StatusColorToken`. A partially reported account is a caveat rather than a failure, so it takes `warning`; only a failed call takes `error`. `TestInfoWidgetPaletteTokens` pins these against the `titanium` palette, since a token that resolves but is the wrong hue still renders and still fits.

This is a native Go adaptation of jcode's margin-widget design, not a Rust compatibility layer: jcode-specific cloud, memory, workspace and swarm backends are not imported. Its observed source revision and MIT notice are recorded in NOTICE and `tui/LICENSE.jcode`. `info_widgets_test.go` and `editor_widget_control_test.go` cover prompt anchoring, palette token use, border quota/control budgeting, zero-space allocation, multiline input, disclosure, complete collapse without transcript hit boxes, Unicode, resize and bounded scrolling; `info_widgets_scroll_test.go` pins both edges at every scroll offset, full content reachability, scrolled header hit targets and tiny-height behavior; `info_widgets_selection_test.go` covers single-candidate priority and scroll reset; `info_widgets_benchmark_test.go` measures host framing with cached 30/300-row lists and complete plain/visible/hidden transcript frames; `interactive_info_widgets_test.go` covers snapshot authority, receipt/widget lifecycle routing, rewind clearing and cached quota windows. `todo_tool_card_test.go` pins compact pending/success/error receipts, the condensed error summary against every failure shape the sessions logged, the bound on expanded diagnostics, dropped argument echoes, renderer rebinding, tiny widths and resumed transcript rendering; `interactive_info_widgets_test.go` pins the panel's one-row failure above the last good list, its twelve-row bound and the clearing that follows a successful call. `scripts/smoke-test-info-widgets.py` drives a loopback model through real fullscreen and regular PTYs under `--no-extensions`, asserting compact transcript receipts, the canonical checkbox renderer, first/last-page scrolling, Alt+W disclosure, and that a payload validation rejects reaches the panel as the rejected member's name rather than the harness's whole message; it runs in `make smoke`.

`native_test.go` pins the contract: both registries hold the feature (`tools.IsExtendedBuiltinTool("todo")`, one native settle hook), and `TestReminderPolicy` covers every continue decision — open work continues and delivers, an unchanged list is nudged once, a final question is a legitimate stop, settled and blocked-only lists never nudge, and the 3-reminder budget is bounded. Removing either registration fails `native_test.go`. Verified end to end on the binary: the Extensions list holds no todo entry, the model calls `todo` by default and reports `NO_TODO_TOOL` under `--no-builtin-tools`, and a run stopped with 3 open tasks logged two `todo-reminder` custom messages in its transcript.

See `vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/todo/state.go`, `vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/todo/render.go`, `vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/todo/settle.go`, `vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/todo/base_tool.go`, and the tests beside them.

### Agent state published to other programs

The binary publishes what each running agent is doing as one JSON document per process:

```text
<PIG_HOME|~/.pi>/state/agent/<pid>/status.json
```

One directory per process, not one shared file, because several agents run at once on a real machine
— two Herdr panes, a pane plus a `-p` run from a script, a subagent host. A single file had them
overwrite each other, and a consumer watching a pane could see a rival session's state; that was a
real defect in the first version of this surface, and `TestConcurrentSessionsKeepSeparateDocuments`
is its regression test. A process id is the right key because one process runs one session at a time;
a session replacement reuses the directory. The publisher fixes `pid` at construction from the
directory name, so a document's `pid` and the directory a consumer enumerates can never disagree.

A consumer either reads one document ([PathForPID], for a tool already embedded in an agent) or
enumerates the live ones ([List], for a status bar or a supervisor). [List] drops a document whose
process is gone and removes its directory, because a killed writer cannot clean up after itself and a
stale `working` in a listing is worse than a missing entry; a pid this process may not signal counts as
alive, so a document is never dropped for lack of permission to check it.

`internal/agentstate` owns that document — its schema, its atomic writer, and its reader — and
`internal/agentstate/ext` is the compiled-in extension that fills it from session events. Everything
else that wants to know what the agent is doing reads the file. Nothing in the binary knows about
Herdr, tmux, a status bar, or a notifier, so supporting another consumer never means a new binary.

**Why a file rather than a socket or a callback.** A lifecycle event is a moment, and a consumer that
starts after it — a status bar launched later, an integration attached to an already-running session —
would miss it. Publishing the *current* state instead of the transitions means a reader only has to
read the file to be correct, and a publish coalesces naturally: two changes faster than one write
leave the newest state on disk. The document is written atomically (temp file in the same directory,
then rename), so a reader never sees a partial file and a crash mid-write leaves the previous document
intact.

**The move, not the moment.** `state` is `idle`, `working`, `blocked`, or `exited`, and the document
also carries `agent` (`ompig-slim`), `session_id`, `session_file`, `cwd`, `mode`, `detail` (the dialog
title for a blocked state, collapsed to one printable line), `blocked_kind`, `pid`, and `updated_at`.
A consumer that only cares about an interactive agent must require `mode == "tui"`: a one-shot
`-p` run in the same directory publishes too, and it is not the agent the user is watching. `exited` is
written once when the *process* ends, so a poller stops instead of trusting the last `working` forever;
`pid` and `updated_at` let a reader tell a live document from a killed writer's leftovers.

**A replaced session is not an exit.** `session_shutdown` carries a reason, and only `quit` ends the
process: `/clear` emits `new`, `/reload` emits `reload`, and resuming or forking emit their own. The
tracker therefore routes the event by reason — `quit` publishes `exited` and stops, every other reason
resets the run and dialog state and republishes — so the next session of the same process keeps
publishing. Treating every shutdown as terminal published `exited` for a live process and ignored
every later event; a consumer reads `exited` as "stop watching this pane", which is one way the adapter
appeared to lose the agent.

**The document is refreshed, not only rewritten.** `Publish` suppresses a write whose content matches
the last one, so an idle agent keeps the timestamp of its last state change — arbitrarily old. The
extension therefore re-stamps the last document every `HeartbeatInterval` (a third of
`HeartbeatAfter`, one minute) through `Publisher.Refresh`, which writes a fresh `updated_at` without
inventing a document or changing `state`. A reader applying `agentstate.Stale` / the adapter's own
`staleAfter` treats anything older as "no information", so one missed beat is tolerated and a writer
that really did stop publishing stops being believed. This matters because Herdr never expires a
reported state on its own: a frozen `working` kept a pane looking busy indefinitely, which was the
other way the adapter appeared to lose the agent. The reader-side rule needs the writer-side refresh;
neither is useful alone.

Derivation, in the order the states win: an open blocking dialog is `blocked`, an in-flight run is
`working`, anything else is `idle`. `blocked` wins because a dialog can open mid-run and is what a
consumer waiting on the user must see. `agent_settled` — not `agent_end` — ends a run, because PiG
fires it only after retries, compaction, and queued continuations stop, so a consumer does not see
`idle` between two attempts of one turn. `session_start` re-reads the session's own idle flag instead
of assuming, because a `/reload` replaces the extension mid-run. A nested dialog increments a depth
counter, so only the outermost close ends `blocked`. The handlers run on more than one goroutine, so
every mutation publishes under the same mutex the state is derived under.

**Failure is silent, by design.** `Publish` reports nothing: a read-only home, a full disk, or an
unresolvable configuration root costs a missing status document, never a failed turn. The last write
error is kept for a caller that wants to log it, and a publisher with no destination drops
everything.

### The Herdr adapter

Herdr decides which agent owns a pane from the **process name** of the pane's foreground job, matched
against a closed alias table (`pi`, `omp`, `claude`, …) in `src/detect/mod.rs`. `ompig-slim` is not in
it, so a Herdr pane running this program looks like a plain shell: no sidebar row, no
`herdr agent wait`, no tab or workspace rollup. Herdr's bundled Pi integration cannot bridge this — it
attaches to a process named `pi` and would report the wrong agent if pointed elsewhere.

`extensions/herdr` is an ordinary **installable extension**, not compiled-in code, and it is a pure
adapter: it polls the state document and calls `herdr pane report-agent`. It cannot find its document
by process id — PiG runs an extension in its own process — so it matches on the session id every
document carries, re-resolved on each poll, which also handles a session replacement without an
explicit re-subscription. It derives no state of its
own, so it cannot disagree with what the binary published, and a bug in it can only misreport a state
that is already visible on disk. Installing it is `ompig-slim install ./extensions/herdr`, or
`-e ./extensions/herdr` for one run.

Because an extension is a separate Go module built by PiG against the SDK staged for the running
version, `make test-extensions` adds a `replace` to the staged SDK, runs `go test`, and restores
`go.mod`. That is why `extensions/herdr/go.mod` carries a placeholder `require … v0.0.0`: the loader
rewrites the version, and a committed `replace` would pin a stale SDK. The `go` directive stays at
1.26 — the version `ompig-slim extension init` scaffolds — because a newer one refuses to build on an
older toolchain.

Transport is `"$HERDR_BIN_PATH" pane report-agent …` rather than the socket in `HERDR_SOCKET_PATH`:
Herdr documents the CLI as the portable form (it reaches the Windows named pipe itself), and the
socket wire protocol, `resume_argv` framing included, stays out of this repository. A call is bounded
by a 2s timeout with both streams discarded, and an unchanged state sends nothing, so a session that
sits idle makes no calls at all.

`--seq` starts from `time.Now().UnixNano()` and only increases, because Herdr drops a report whose seq
is not higher than the last one it accepted from that source and `custom:ompig-slim` keeps that
high-water mark across agent restarts in the same pane.

Resume is reported as `ompig-slim --session <absolute session file>`. Herdr types the command into a
fresh shell and rejects a path as the executable, so `ompig-slim` must be on `PATH` from the installer;
the session is named by file rather than by ID because Herdr restores in the stored cwd and an ID
lookup depends on the original one. Herdr's limits (a plain command name, no control characters or
apostrophes, ≤64 arguments, ≤8 KiB) are enforced in the adapter before sending: a resume command that
breaks one is left out rather than making Herdr reject the whole report, which would lose the state
too. The adapter never releases the pane; Herdr clears the agent and its resume command itself once
the pane is back at its shell prompt, which covers exit and crash alike.

`internal/agentstate/agentstate_test.go` covers the atomic write, duplicate suppression, field
preservation, failure silence, refresh of an unchanged document, the staleness window, and the
reader's missing-versus-corrupt distinction; `internal/agentstate/ext/ext_test.go` drives real session
events through the production factory loader and a real `inproc.Runner`, including each
session-scoped shutdown reason and the heartbeat's start and stop. `extensions/herdr/herdr_test.go`
covers the command line, seq ordering, `exited` filtering, the staleness drop, resume validation, and
the poller. `scripts/smoke-test-agent-state.py` proves both halves on the built executable: one real
model turn moves the document `idle → working → idle`, the idle document is refreshed without a state
change, and the installed extension forwards those states to a recording fake Herdr with the resume
command.

### Theme, palette and colorful Markdown

The theme system is `vendor/github.com/MichaelKinsy/PiG/tui/theme*.go`, a port of `packages/tui/src/theme/` in oh-my-pi. Four things were changed from the stock port; keep them together, because each one only makes sense beside the others.

**Optional palettes, not bundled assets.** The 100 oh-my-pi palettes live as individual JSON files in root `themes/`, with installation instructions and attribution in `themes/README.md`. They are not embedded, decoded, staged, or shipped in release archives. The normal `--theme` and profile theme discovery paths load them only when requested. `NewThemeRegistry` starts with exactly `system`, `dark`, and `light`; the default is `dark` or `light` according to terminal appearance. A saved optional name must be installed as a custom theme to resolve; unknown names retain the existing system fallback. Palette resolution, Markdown colors, schema compatibility, and terminal color-mode conversion still use the same theme loader. Tests load the optional files from the repository through that public loader.

**The status line tokens are real.** `statusLine*` and `pythonMode` are upstream schema tokens that the stock port validated and then dropped. They are now `ThemeColor`/`ThemeBg` values with fields on `Theme`, optional in the schema with entries in `themeColorFallbacks`, so a theme written before the status line existed still renders one instead of panicking on `Unknown theme color`. `internal/codingagent/status_line.go` `renderFooter` draws each segment in its own token — path, branch, session name, spend, output, cost, context, model — instead of the single `dim()` wrapper the stock footer used, which is the second half of the plain-text look. `applyContextColor` follows oh-my-pi's four levels (error >90%, thinkingHigh 70-90%, warning >50%, statusLineContext below). There is deliberately no `statusLineBg`: nothing paints a full-width footer background here, and a token nothing draws is dead configuration.

**The footer's rows are project-and-model, then usage.** Upstream footer.ts puts the project on line 1 and the model at the right edge of line 2 beside the stats. `footerLines` moves the model up to line 1's right edge and leaves line 2 wholly to the stats, so a phone-width terminal shows what it is working in and what it is working with on the row under the prompt. A long project path collapses rather than truncating at the right edge, because an end-truncation would eat exactly the leaf that names the project: `compactFooterPath` elides the middle, keeping the first segment (the `~`, the root, or a Windows drive) and the trailing segments that narrow down to the leaf. `footerProject` gives up the session name first and the branch second before it lets the leaf be cut, since the path is what the row is for. The model name is never cut to fit line 1; when it cannot share the row, `footerStatsWithModel` restores upstream's layout with the model at line 2's right edge, and below that the model takes line 2 alone. `status_line_test.go` sweeps widths 1–220 to prove no row exceeds the terminal, which is what would otherwise wrap and corrupt the rows under it.

**Colorful Markdown.** `Theme.ANSIFg` paints an ANSI palette slot from the theme's `terminal.ansi` (`resolveThemeANSIPalette`), falling back to the terminal's own SGR 30-37/90-97 entry. `tui/markdown_style.go` builds `GetMarkdownTheme` with per-depth `HeadingAt` and the `TableHeader` member, and gives bold, italic, inline code and table headers palette hues: H1 magenta, H2 yellow, H3+ blue, emphasis magenta, inline code green, headers yellow. All hues come from one palette because themes commonly alias several `md*` tokens to one color. A theme with no palette keeps its `md*` tokens, which is the pre-change rendering. A heading's own emphasis is drawn plain: `headingInline` renders through `renderHeadingInline`, which borrows a child Markdown carrying the stock bold and italic, so a `**bold**` inside a heading does not gain a second hue.

Two constraints hold the render path together. Each colorful role is one package-level closure that reads `ActiveTheme()` when called, never a closure built per `GetMarkdownTheme` call: `markdownPureStyles` and `resumeKey` identify a theme by its style functions' code addresses, so a fresh closure per call would disable the streaming resume path and re-render the whole answer every delta. And the colorful roles replace the stock emphasis functions, so `stockSpanTheme` reports false and the inline renderer takes `renderInlineTokensSequential` instead of the one-pass span writer; measured against an assistant-sized answer, that path allocates the same and renders within noise, so no special case is needed.

The theme schema accepts `terminal` and `symbols` because oh-my-pi writes both into every built-in palette and into custom themes saved from one. `ThemeJSON` decodes `terminal` as `ThemeTerminalJSON`; the loader reads only `terminal.ansi` (the colorful Markdown palette) and `terminal.foreground` (the body color adopted when `text` is unset). `symbols` is accepted and ignored — it is only checked to be an object, since this engine renders no glyph table of its own, and deep-validating a section nothing reads would reject themes for no reason. `colors.go` also accepts `#rrggbbaa`, dropping the alpha as Bun does, because 32 of the bundled palettes use it.

Tests: `tui/theme_palettes_test.go` (optional files load, every token resolves, core-only registration, defaults, schema acceptance and rejection, optional-token fallbacks, `ANSIFg` fallback), `tui/markdown_palette_test.go` (role hues, token fallback, heading emphasis, width-and-text parity), `internal/codingagent/status_line_test.go` (segment tokens, context thresholds, the shared project-and-model row, path compaction, the order decorations are dropped in, the narrow fallback, and a width sweep).

**The body color.** `text` is `""` in 96 of the 100 bundled palettes — the token means "the terminal's own foreground" — and every palette records the body color it was designed against in `terminal.foreground` instead. That combination is the rest of the "everything is white" report: assistant prose, dialog descriptions, list rows, login prompts and settings labels all resolve through `Fg("text", …)`, so on a terminal whose default foreground sits close to its background they all render as the same uncolored glyphs, with only headings, code and accents standing out.

`Theme.adoptPaletteForeground` (`theme.go`, called from `resolveThemeWithMode`) closes this at the source rather than at each call site: when a theme's JSON leaves `text` unset, the loader resolves `terminal.foreground` and adopts it as a concrete color, updating `fgAnsi`, the `Text` field, `concreteColors` and `defaultForegroundTokens` together. Every existing `Fg("text", …)` call site then draws the palette's own body color with no call-site change. A theme that writes a real color for `text` is untouched, and a theme assembled in code (no JSON source) keeps whatever it set. Nothing else in the loader depends on `text` staying a default token.

On top of that, `assistantMessageText` (upstream's only `OPTIONAL_THEME_COLOR_RECORD` member) is now a real optional token, and `Theme.ProseFg` returns it when set, else `text`. Assistant prose, `/hotkeys` and any `SlashContext.Append` content go through `InteractiveMode.proseMarkdown`, which applies `ProseFg` via `SetDefaultColor` — the same primitive `UserMessageComponent` uses. `proseMarkdown` is the single seam: a Markdown block built with `tui.NewMarkdownWithOptions(..., nil, nil)` anywhere else deliberately has no prose color.

Prose color is resolved per render, not once at construction. A reply outlives the theme it was built under, so both `proseMarkdown` (via its `proseColoredMarkdown` wrapper) and `AssistantMessageComponent.applyProseColor` re-read `ProseFg` before drawing. Capturing it at construction left the previous theme's escape on screen until the message was rebuilt. `SetDefaultColor` returns early when the color is unchanged, so the per-render cost is one comparison. Thinking runs keep `ThinkingText`, a different token.

Two visual fixes came with it. `renderInlineTokensSequential` no longer writes the context's opening codes twice on a text run that follows an inline token: `writeToken` sets a `pending` flag after writing them, and `applyText` strips a duplicate prefix from the next run. Only `writeToken` raises the flag — a text run ends with closing codes, so it clears it, or escaped punctuation and inline HTML (which render as their own runs) would lose their color. And the settings list's unselected label now uses `th.Text` instead of the terminal default, so the list is not the one surface whose rows have no color at all.

**One role, one color.** `modelroles.Config.RoleInfo` is the single source of a role's display color, and every surface that paints a role reads it: the manager's role rows and fallback rows, the browser's `@role` chips, the Ctrl+P cycle track, and the footer. Routing them all through one token is the point — a role that appeared in two hues read as two roles. Two divergences from OMP are deliberate. The cycle track is colored by role rather than by position, so reordering it does not change a role's hue, and two roles sharing a token (`plan`, `task` and `judge` are all `muted`) share a color. And the color is resolved against `tui.ModelOverlayTheme`, not `ActiveTheme`: on the inherited stock `dark`/`system` themes the catalog paints with OMP's Titanium projection while the transcript keeps PiG's stock palette, so the active theme would give every role a different hue in the footer than in the manager. `Theme.HasFgToken` exists for this path, because `Fg`/`GetFgAnsi` panic on an unknown token and a role color can be any string a `modelTags` override names.

The footer reads its role through `FooterSession.ActiveModelRole`/`ActiveModelRoleColor` on each render rather than through a setter. A role can change while the physical model does not — fallback restoration, or a cycle between two roles that share a model — so no single model-change hook would keep it current.

**Role references are two separate graphs, deliberately.** A primary assignment (`task: "@default"`) and a chain entry (`"review": ["@slow"]`) both name a role, but they resolve through different code and can close different loops, so `modelroles` exposes `AliasReaches` and `ChainReaches` rather than one walk. `Config.assignment` is the single definition of the alias graph, shared by `Resolve` and `AliasReaches`, so the write guard cannot disagree with resolution about which edges exist. `ChainEntries` flattens a chain depth-first and expands each role at most once, which is what makes a hand-edited loop terminate; the write guards in `SettingsManager.SetModelRole`/`SetFallbackChain` refuse the closing write so the manager never creates one. `FallbackRoles` and `AssignableRoles` are the picker's half of that contract: they return exactly the roles the corresponding guard accepts, so every row offered saves successfully.

### Interactive tool cards

- Interactive cards opt into `ToolExecutionOptions.Rich` and follow one of
  OMP's two shapes (`packages/tui/src/tools/renderer.ts` `inline`), plus a third
  shape this project adds for tools whose own rows are already structured:
  - **Framed** (`bash`, `codemode`, `web_search`, unregistered tools): OMP's
    `renderOutputBlock` — `╭─── … ───╮` bars, body rows padded one cell inside
    the borders, a labelled `├─── Output ───┤` section where the tool has one,
    and a `╰─ … ─╯` bar carrying facts and the Ctrl+O hint. Every row including
    the bars is padded to the full card width, and the phase background tint is
    applied over the whole row, borders included, re-applied after embedded SGR
    resets. Every tool is filled, the shell included; this project keeps one
    filled card look instead of oh-my-pi's `applyBg: false` shell exception.
  - **Unframed** (the `todo` renderer embedded in the Tasks widget, not a
    transcript card): `RenderShell: extension.ToolRenderShellUnframed`
    draws the renderers' rows as the whole card — no bars, no status line, no
    background tint — while the card stays `Rich` and expandable. A pig addition
    (`ToolDefinitionRenderers.Unframed`); a tool opts in when its content is its
    own layout, so the frame would only box it and repeat its title.
  - **Inline** (`read`, `grep`, `find`, `ls`, `todo` receipts): no frame at all. The collapsed
    card is the single status-line row — `✔ Read skills/optimize/SKILL.md:70-95`
    — and Ctrl+O discloses the body rows under it. `write` and `edit` return
    framed boxes in oh-my-pi, so they keep the block.
- The status line is `renderStatusLine`: `icon Title<sep>description · meta`.
  `read`/`write`/`edit` separate the target with a space and every other tool
  with `: ` — so a search reads `Web Search: BTC price` and grep reads
  `Grep: frontmatter|^name: in skills`. The subject is recomputed from the
  streaming parser's decoded arguments on every delta, so it appears while the
  call is still arriving rather than only once its arguments complete. The icon is ⏳ before execution, a spinner frame while running,
  then ✔, ✘ (error or abort). Meta carries `exit <code>` for a shell command and
  the duration; Duration prefers the recorded `DurationMs` over the card clock so
  a reload keeps it, and no result body repeats timing.
- Status-line subjects (`tui.ToolDetail`) mirror each OMP renderer: read/write/
  edit show `<path>[:<start>-<end>]` (read.ts `formatReadPathLink`), grep shows
  `<pattern> in <scope>` (grep.ts `formatScopedSubject`), find shows
  `"<query>" [keywords] in <scope>` (find.ts `quoteQuery`), web_search shows the
  query truncated to 80 cells (web-search.ts `renderSearchCall`).
- `write` streams like write.ts. While its `content` argument is still
  arriving the body is `formatStreamingContent`: the newest
  `PREVIEW_LIMITS.EXPANDED_LINES` rows of the file, highlighted by its language,
  numbered from a right-aligned gutter, with a `… N earlier lines` marker and a
  `streaming` footer fact. Once `argsComplete` finalizes the content the body
  becomes `renderContentPreview` — the head of the file capped at
  `WRITE_PREVIEW_LINES` — and the footer clears.
- `edit` streams like edit.ts's `renderStreamingFallback`. Until the computed
  diff lands, a replace-mode edit draws its replacement text as `+` rows
  (`CALL_TEXT_PREVIEW_LINES`, with a `… N more lines` marker), so the edit is
  visible while its arguments arrive rather than appearing only at the end. The
  filesystem-derived diff still takes over once `argsComplete` triggers it.
- A bash exit code comes from the structured result (`exit_code`) because
  `BashDetails` carries none; see `internal/codingagent/tool_shell_exit_code.go`.
- A collapsed or expanded `grep`/`find` result draws oh-my-pi's grouped tree
  (`render/tree-list.ts`): one `├`/`└` branch per file, grep's matched rows
  hanging under it as `*12│text` with the vertical rail, and a trailing
  `… N more matches` row while collapsed that counts hidden *matches*, not
  hidden files (oh-my-pi `fitGroupsToBudget`). The status line carries the result
  counts (`21 matches · 20 files`). Branch glyphs mark siblings, not nesting
  depth: the first N-1 roots draw `├` and the last draws `└`.
- A card body is bounded by **height**: `RichBodyMaxRows` (50) rows while
  collapsed, with the rest behind Ctrl+O. Every card goes through this cap, not
  only searches, because a terminal has a fixed number of rows and an uncapped
  body would push the conversation off screen. The cap appends the
  `… N more` marker and keeps the disclosure reachable.
- A **collapsed** search entry is exactly one row, cut at the card's width with
  an ellipsis. It never wraps: a wrapped entry needs a continuation rail and
  stops reading as a tree, and one row per entry is what makes the 50-row budget
  equal the entry count. Every row is `<glyph><blank><text>` so the text column
  never shifts.
- **Expanding restores the full text.** An expanded entry wraps under its own
  rail rather than showing a wider cut, because a reader who expands wants the
  whole match. A continuation keeps the tree line — `├`/`│` continue with `│`,
  `└` continues with the blank gutter, since that branch has already ended
  (tree_list.go `entryRows`). The card must not re-truncate an expanded row.
- A collapsed inline card has no bottom bar, so its summary row names the key
  (`… 39 more matches ⟨Ctrl+O: Expand⟩`); a framed card names it on the bottom
  bar.
- The card owns this transform (`tui/tree_list.go`), not a tool renderer. Grep and find resolve no registered
  renderers, so a renderer-side implementation never runs in a real session —
  the first attempt was wired that way and drew nothing. Owning it in the card
  makes the tree independent of how a tool's renderers resolved. `SetSearchOutput`
  is optional: the card falls back to its own `Output`, so a result path that
  never records the text still draws the tree instead of an empty body. The
  tools still return flat `path:line: text` rows to the model; the tree is
  display-only.
- `read` covers files, directories and URLs. A directory lists its entries,
  directories first then files (read.ts `formatDirectoryListing`), marked
  `ReadDetails.IsDirectory` so the card keeps the listing as plain text. A URL
  path is fetched through `internal/webtools` and marked `kind: "url"`; the
  retired `web_fetch` tool's model-facing contract is preserved in
  `internal/codingagent/tools/read_url.go`.
- Cards narrower than 10 cells fall back to the plain upstream presentation.
  Images stay outside the frame, and mouse coordinates are translated back into
  renderer space.
- Canonical implementation: `tui/tool_execution_rich.go` (frame, inline row,
  status line, `ToolDetail`), `internal/codingagent/tool_builtin_renderers.go`
  (per-tool bodies), `internal/codingagent/tool_codemode_renderer.go`.

### Saved rate-limit resets

- `/usage reset` lists Codex and Claude reset inventory. The interactive spend picker includes only currently redeemable, unexpired resets and requires confirmation twice. Typed targets use `/usage reset <provider>/<account-key>`, including `codex/active` and `claude/active`.
- Codex spends the available credit expiring soonest; Claude rechecks inventory and spends the grant selected by its endpoint. A confirmed spend invalidates the account's quota cache and clears only matching quota blocks, preserving unrelated auth and policy blocks. A Claude 5h-only reset leaves weekly blocks intact.
- Reset redemption is an irreversible mutation and is not automatically retried. If its outcome is uncertain, check `/usage` before trying again. `Session.DispatchSlash("/usage reset")` returns the inventory without opening a picker; print-mode prompts do not dispatch built-in slash commands. See `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_reset.go`, `vendor/github.com/MichaelKinsy/PiG/ai/usage_reset.go`, and `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_usage_reset.go`.

### Keybindings a legacy terminal cannot report

`app.message.followUp` — queue a message behind the current task — defaults to `ctrl+q` on **every** platform column. Upstream keys it by platform (`alt+enter` outside Windows and WSL, `ctrl+q` inside), which leaves the action on a different key per OS and on none at all in terminals that cannot report Option+Return. `ctrl+q` is the Windows default already, so the change only widens it.

The single source of truth is the `appKeybindingRows` entry in `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/keybindings.go`. Nothing else hardcodes the key: the startup header hint (`startup_header.go`), `/hotkeys` (`interactive_hotkeys.go`) and the `/settings` follow-up row (`settings_selector.go`, which passes `ctrl+q` only as the fallback for a registry that has not loaded) all read that table, so changing the row is enough. `keybindings.schema.json` needs no edit either — it renders each action's description, not its default keys. `keybindings_default_test.go` pins the resolved default on all four platform columns, the three input forms a terminal can send (`\x11`, `\x1b[113;5u`, `\x1b[27;5;113~`), and that those inputs classify as `actionFollowUp` and resolve to `outcomeFollowUp` rather than stopping at the matcher.

`ctrl+q` is the XON flow-control byte. It reaches the process because raw mode clears `IXON` in the same ioctl that clears line editing (`golang.org/x/term` `MakeRaw`), so no terminal capability is being assumed; `ctrl+s` (XOFF) is bound the same way by `app.session.toggleSort`, `app.session.rename` and `app.thinking.save`. A terminal that intercepts `ctrl+q` outright — a multiplexer prefix, or an OS shortcut — never sends the byte, which is the general terminal-mapping problem in [keybindings](vendor/github.com/MichaelKinsy/PiG/internal/pigdocs/content/keybindings.md) and is why the action stays rebindable.

Two consequences are deliberate and recorded as divergence D234: `alt+enter`/ESC+Return now reaches the editor, where the editor inserts a newline, so Apple Terminal (which cannot report Option+Return separately) loses its follow-up key entirely unless the user rebinds; and a `keybindings.json` entry for `app.message.followUp` replaces the default rather than adding to it, so `"app.message.followUp": ["alt+enter"]` restores upstream's macOS/Linux key in one line. Do not add `alt+enter` back as a second default key — a multi-key default is rendered as `ctrl+q/alt+enter` in every hint and row that formats the binding.

The same failure mode applied to four other defaults, which are retargeted with D234. The decisive property of the wire format is that a terminal with **neither** the Kitty keyboard protocol **nor** xterm `modifyOtherKeys` (mode 4) sends `Ctrl+Shift+letter` as the same C0 byte as `Ctrl+letter`. So `shift+ctrl+p` fired `ctrl+p`'s action (cycle **forward**), `shift+ctrl+o` fired `ctrl+o` (expand tools), and `ctrl+shift+g` fired `ctrl+g` (next match) — each binding pointed at its own neighbour. Option-based keys are the other half: with Terminal.app's default "Use Option as Meta key" off, `Option+letter` types a layout character, so those send nothing. The shipped bindings after the fix:

| Action | Was | Now | Why the new key arrives |
|---|---|---|---|
| `app.message.followUp` | `alt+enter` / `ctrl+q` | `ctrl+q` | XON byte; raw mode clears `IXON` |
| `app.model.cycleBackward` | `shift+ctrl+p` | `ctrl+shift+left` | `CSI 1;6D` is a fixed sequence, not extended reporting |
| `app.tree.filter.cycleBackward` | `shift+ctrl+o` | `ctrl+shift+right` | `CSI 1;6C`, as above |
| `app.session.deleteNoninvasive` | `ctrl+backspace` | `ctrl+delete` | `CSI 3;5~` |
| `tui.altScreen.search` | `ctrl+shift+f` | `ctrl+f` | plain byte; already Pi's Windows column |
| `tui.altScreen.searchPrevious` | `shift+enter`, `ctrl+shift+g` | `shift+enter`, `f3` | `f3` is `ESC[13~`; `shift+enter` kept — native modifier detection rewrites Return to `CSI 13;2u` on Apple Terminal and Windows |

`Ctrl+Shift+arrow` is the useful discovery here: it is **not** subject to the Ctrl+Shift+letter collapse, because xterm has emitted `CSI 1;6<final>` for modified arrows since long before the Kitty protocol existed, so it is deliverable on a stock Terminal.app profile. Prefer it over `Ctrl+Shift+letter` for any future binding. `ctrl+home`/`ctrl+end` (`CSI 1;5H`/`1;5F`) and every `ctrl+<single letter>` are equally safe; `Ctrl+Alt+letter` is **not** safe on Apple Terminal with Option-as-Meta off.

Two properties to keep. The `tui.*` rows live in `vendor/github.com/MichaelKinsy/PiG/tui/keybindings.go` and the `app.*` rows in `internal/codingagent/keybindings.go`; nothing else hardcodes a default. And `extensions/sdk` carries a **pinned copy** of the `tui` table for extension-authored UIs — it is a separate module whose `.gz` bundle needs `automation/gen/embed-bundle.go`, which is not vendored, so it is deliberately left at upstream's values. That copy never drives the built-in TUI.

`keybindings_default_test.go` guards the set: `TestDefaultsAreReachableWithoutExtendedKeys` walks every platform column and fails when a listed action has no default reachable from a legacy repertoire (C0 bytes, printable ASCII, `ESC`-prefixed Meta, `CSI 1;N<final>`, `CSI <n>;<m>~`, and `CSI 13;2u`). Reverting any of the five retargeted rows to its upstream key fails the test.
