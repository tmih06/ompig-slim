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
- `scripts/release_fixture.py` owns the release layout both suites serve: `archive_bytes` builds the member layout `build-binaries.sh` publishes, and `ReleaseServer` exposes GitHub's `releases/download/<tag>/` and `releases/latest/download/` paths plus the `releases/latest` JSON the updater reads. A change to the release contract is therefore proven by both suites at once instead of each drifting separately. `publish` takes pre-encoded archive bytes so a suite that republishes a large payload can cache the encoding.
- `scripts/test-install.py` is the installer's regression suite: it serves fixture releases over loopback with a fake `uname` and a shell-script executable, so it is deterministic on both CI runners and needs no network. Extend it, not a manual smoke run, when the installer's contract changes.
- Install and update share one visual language: the block-letter mark from `assets/ompig-banner.svg`, the `in Go · slim harness` tagline, and a 32-cell progress meter drawn with `━`/`─`. The two renderings are duplicated by necessity — `scripts/install.sh` must run with no Go toolchain and `internal/selfupdate/ui.go` must run with no shell — so the art, the bar glyphs, the bar width, and the tagline must be changed together in both files.
- Presentation is terminal-only. `scripts/install.sh` gates on `[[ -t 1 ]]` and `isTerminal` gates the updater on a character device, so a redirected log or a CI pipe receives plain lines with no escape sequences, no carriage returns, and no banner mid-run. The mark is printed once per run after the platform is settled, so a usage error or an unsupported-platform refusal prints only its own message.
- The shell meter is latency-sensitive in a way the Go meter is not. The poll loop runs every 0.1s for the length of a download, so anything it forks is paid ~10 times a second: formatting the two byte counts with `awk` cost ~6.7ms per iteration, and a `$(...)` helper would still fork a subshell. `human_bytes` therefore assigns `REPLY` through integer arithmetic, and `draw_progress` uses `printf -v` for the bar, so the loop forks nothing except the single `wc -c` that reads the growing file. The tenth is rounded half-to-even to match `printf "%.1f"`, so the shell and Go meters print identical digits; `scripts/test-install.py` and a sampled comparison against `humanBytes` cover that parity. Measure this loop with `strace -f -e trace=execve` (count execs per iteration) and by timing the body in a loop; wall time of a real install is dominated by the network and will not show it.
- Progress is real byte progress, not a spinner. The updater wraps the response body in a counter and compares against `Content-Length`; the installer polls the growing file in a background loop against a length read in a separate `HEAD` request, because neither `curl` nor `wget` will report progress to a pipe without its own meter. Both degrade to a count-only line when the length is unknown (chunked responses, redirects to signed URLs), and both redraw only when the whole percentage changes.
- `scripts/test-install.py` asserts the mark is five rows of equal width with no escape sequences in a pipe; `internal/selfupdate/ui_test.go` asserts the same geometry, that a non-terminal meter writes nothing, and that an unchanged percentage does not redraw.
- A successful install also writes `<config root>/ompig-slim-install` (`kind=archive`, `executable`, `download-base`, `installed-version`, `installer`), mode 0600, following PiG's own config-root resolution (`PIG_HOME`, else `~/.pi`, with a leading `~/` expanded). That marker is what `ompig-slim update` and `ompig-slim upgrade` treat as ownership; a config root that cannot be written warns rather than failing the install that already succeeded, because the executable is already in place by then.

### In-place `update` and `upgrade`

- `ompig-slim upgrade` is a second spelling of `ompig-slim update`, in every form (`upgrade`, `upgrade self`, `upgrade --all`, `upgrade --models`). PiG has no `upgrade` command, so the alias is rewritten before PiG parses arguments; otherwise PiG reads the word as a prompt and fails on the selected model.
- `internal/selfupdate` owns that dispatch; `main.go` calls `selfupdate.Handle` before `cli.Main`. `resolveRequest` is a pure parser of the `update`/`upgrade` argument list and mirrors PiG's package parser only far enough to tell a self-update from a package, model, extension, or help request: `--help`, `--models`, `--extensions` alone, `--extension <source>`, a positional source, and unknown flags all stay PiG's. `update self --extensions` resolves to `--all`, as in PiG.
- A self-update is claimed only when the install marker exists. Otherwise the canonical `update` reaches PiG unchanged and the user gets PiG's own install-method refusal and remediation, so npm, `go install`, Nix, container, and Piglet-Binary installations are never silently replaced.
- Why this exists: PiG's standalone tier requires a signed update manifest and an installation receipt under its own schema. This repository publishes neither a signed manifest nor a trust root, so on a curl-installed binary every `ompig-slim update` refused. `internal/selfupdate` closes that gap with the same trust decision the installer already makes: the release archive plus its published `.sha256`, fetched from the release named by the marker's `download-base`.
- The trust boundary is inherited from `scripts/install.sh` and stated in both places: the checksum detects corruption, a truncated download, or a swapped asset within one release; it is not a signature and does not survive a compromised release. Adding signature verification means publishing a signed manifest and a trust root, which is a separate change.
- Placement contract matches the installer: the executable is extracted to a hidden staging file in the destination directory, chmod 0755, and renamed over the running executable. A checksum mismatch, an archive without `ompig-slim/ompig-slim`, a non-regular or oversized member, an unreachable channel, or a release with no archive for this platform leaves the installed binary byte-for-byte unchanged and exits 1. `--force` reinstalls the current release. `update --all` runs the package phase through a child process first, matching PiG's order.
- `ompig-slim update --models` and `update --extensions` remain PiG's commands; the self-updater never intercepts them, and CI asserts that they do not print its progress lines.
- `scripts/test-self-update.py` proves the contract on the real executable: it installs the binary under test through `scripts/install.sh`, then serves newer fixture releases over loopback through the same `release_fixture` the installer suite uses. `internal/selfupdate/selfupdate_test.go` covers version ordering, argument resolution, checksum and archive rejection, atomic placement, and the no-marker refusal without touching a real home directory.
- `OMPIG_SLIM_DOWNLOAD_BASE` and `OMPIG_SLIM_RELEASES_API` override the marker's download root and the GitHub releases endpoint for mirrors and tests. The running release version comes from `coding.Version`, not from a tag on the executable.

### Codex authentication and imported credentials

- Browser and device-code Codex sign-in use the Codex public client and the native `codex_cli_rs` originator. Email-only accounts can authenticate without a workspace ID; workspace headers are optional, while model access and quotas remain subject to the account plan and OpenAI's backend. See `vendor/github.com/MichaelKinsy/PiG/ai/oauth_openai_codex.go` and `vendor/github.com/MichaelKinsy/PiG/ai/auth_providers.go`.
- `/login` presents a searchable list of account and API-key options. Codex JSON imports use provider ID `codex-json`, through `/login codex-json` or `ompig-slim login codex-json`; imported models are selected under that provider.
- The importer persists only the access token, optional refresh token, account ID, and expiry. It gets the account ID from the token or `providerSpecificData.chatgptAccountId`; `expiresAt` (RFC3339) takes precedence over `expiresIn` (seconds), and JWT expiry caps either lifetime. Missing or expired expiry is rejected. Fields such as `idToken`, labels, and subscription metadata are discarded. See `vendor/github.com/MichaelKinsy/PiG/ai/omp_codex_json.go`.
- Imported accounts use the normal quota-aware account router and refresh-token rotation. Definitive refresh failures remove live secrets and routing blocks while retaining an OMP-compatible disabled row. Transport failures, cancellation, rate limits, server errors, and unclassified failures retain credentials. Cleanup is use-driven, not a background timer; browser-login Codex accounts are unaffected. See `vendor/github.com/MichaelKinsy/PiG/ai/omp_codex_json_storage.go` and the routing tests beside it.
- Sessions using the built-in ModelRuntime stream must also route compaction summaries through it without pre-resolving credentials. Explicit credentials pin a single account and bypass model-plan eligibility and account rotation; caller-supplied Agent streams retain their own auth behavior. See `vendor/github.com/MichaelKinsy/PiG/coding/session_summarization_auth.go`.

### Todo tool and its stop-time reminder

`todo` is OMP's task list, ported as a program-composed extension in `internal/todotool` (registered from `main.go`) rather than as a vendored patch, so `go mod vendor` cannot drop it. `internal/todo` owns the data model and list semantics; the extension owns the tool definition and the settle-boundary handler.

State lives only in each tool result's `details`. Every call re-reads the newest state-changing `todo` result on the session branch (`internal/todotool/snapshot.go`), so resume, rewind and fork need no persistence of their own, and the list can never disagree with the transcript. A `view` result and an errored result are both skipped as state: the first changed nothing, the second persisted nothing.

`view` is also exempt from list normalization. `todo.Apply` runs `normalizeInProgress` for a mutating op to keep at most one task active, but that is a write: running it for a read-only op would return a copy the transcript never accepted, demoting a second `in_progress` task in the rendered list. `Op.readOnly` decides this, and `TestViewDoesNotNormalize`/`TestWriteOpsStillNormalize` pin both directions. The tool short-circuits a view before calling `Apply` at all, so the library-level guarantee backs a path the tool no longer takes.

The harness behavior is the part that makes the tool matter. `SettleExtension` registers `OnAgentBeforeSettle` and returns `Continue` when the list still has open work, injecting a hidden `todo-reminder` custom message. The handler stays silent, deliberately, when: the run did not complete normally; no list exists; every open task is `blocked`; the agent's last message asks the user something; the list is unchanged since the previous reminder; or the per-run budget of 3 reminders is spent. A `blocked` task is open but never actionable, which is the sanctioned way for an agent to stop: it keeps the list honest while letting the turn end.

`sequential` execution serializes the calls but does not make one call's result visible to the next, so two `todo` calls in one batch each read the same prior list and the later one wins. The tool description therefore instructs one op per call beside the next real action; do not treat batching as safe.

The content is a line-for-line port of OMP's `todoToolRenderer` (`packages/tui/src/tools/todo.ts`) in `internal/todotool/render.go`: one `Todo N tasks` line, roman-numeral phase headers when there are several phases, `├─`/`└─` tree gutters, `☑`/`☐` checkboxes, dim `closed/total` progress per phase, and a collapsed walking viewport that leads with the last closed task and caps at 8 rows. The content is reproduced rather than approximated because it is the only place a user watches a plan advance; a layout that differs by a gutter or a windowing rule reads as a different tool.

The content is drawn **unframed**: the tool sets `RenderShell: extension.ToolRenderShellUnframed`, so the rows are the card's whole presentation, with no border, no status bar and no lifecycle background. The list is already a structured block — a title line, phase headers and tree gutters — so the frame would only box it and repeat its title. `ToolRenderShellUnframed` is a pig addition to the `default`/`self` pair (`tool_execution_definition.go` `Unframed`, set from both renderer factories); the frame itself lives in `renderRichFrame`, which only `renderSelfShell` calls for `Rich` cards, so opting out is a routing change rather than a renderer concern. A frame-less card also skips the mouse inset `HandleMouse` applies inside a border.

Two vendor limitations shape the renderer, both handled in it rather than by patching further. First, OMP's `mergeCallAndResult` has no counterpart here, and the card substitutes a name-and-args fallback when a call renderer returns nil, so `renderCall` draws the streaming header and then nothing once `IsPartial` clears. Without that, a settled card shows a stale `● Todo` line above the list. Second, the vendored `Theme` exposes foreground tokens but not OMP's symbol table, so checkbox, tree and tool glyphs are constants in `render.go` holding OMP's default-theme values. A pending todo matched to a live subagent renders dim rather than accent, because no subagent registry is wired into a renderer here.

Because the frame's bottom bar is gone, it carries no `Ctrl+O: Expand` hint. The card is still click- or Ctrl+O-expandable — `definitionComponents` keeps the result region for an unframed card — so the disclosure works; only the footer that advertised it is gone, which `unframed_test.go` pins alongside the absence of border and background.

See `internal/todo/state.go`, `internal/todotool/render.go`, `internal/todotool/settle.go`, and the tests beside them.

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
  - **Unframed** (`todo`): `RenderShell: extension.ToolRenderShellUnframed`
    draws the renderers' rows as the whole card — no bars, no status line, no
    background tint — while the card stays `Rich` and expandable. A pig addition
    (`ToolDefinitionRenderers.Unframed`); a tool opts in when its content is its
    own layout, so the frame would only box it and repeat its title.
  - **Inline** (`read`, `grep`, `find`, `ls`): no frame at all. The collapsed
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
