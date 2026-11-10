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
```

`make check` runs tests and vet for the root and patched library through `scripts/check-go.sh`. `make smoke` builds the executable and runs its codemode, subagent, and web-tool scenarios from that build; `make smoke-built` runs the same scenarios against an executable CI already extracted from a verified release archive. `make race` repeats tests with the race detector. `make test-install` runs `scripts/test-install.py`, which serves fixture releases over loopback and asserts the installer's download, verification, placement, and failure behavior. `make test-self-update` runs `scripts/test-self-update.py` against a built executable, proving that `update` and `upgrade` replace the installed file and that an install without the ownership marker keeps PiG's own refusal. `make benchmark` compares an already-built executable with pinned OMP and Pi using a local mock model; CI reuses the verified release artifact and never rebuilds inside the benchmark job. See the scripts and CI configuration for exact coverage and platform-specific checks; do not infer that cross-compilation means a target was executed on that platform.

## Project-specific rulings

- Preserve the complete PiG CLI and runtime; `main.go` should remain a thin registration/delegation layer unless a concrete requirement needs otherwise.
- Keep Pi/OMP-compatible storage and configuration behavior. Avoid creating a separate `.pig` profile or changing shared identity formats without an explicit migration plan.
- When modifying account routing, preserve failure scope, session affinity, refresh behavior, and the rule against replaying a request after streamed output. Consult the implementation and tests for the exact contract.
- Treat provider catalogs, auth flows, and quota protocols as provider-specific. Validate protocol changes with local fixtures and keep network/live-service assumptions out of deterministic tests.
- Update this section when an enduring technical ruling changes; keep transient task notes and details discoverable directly from source out of this file.

## Feature behavior notes

### The base system prompt

The prompt's opening text lives in `prompts/system.md` at the repository root and is embedded into the binary with `go:embed`; nothing reads the file at runtime. Edit the Markdown, not Go source. `prompts.System()` returns it trimmed.

The vendored runtime composes the rest and cannot be bypassed from the root: the prompt-building package is `internal/codingagent/prompts`, and Go forbids an external module from importing a vendored `internal` path. So the seam is an exported field, `cli.MainOptions.SystemPromptPreamble`, next to the existing `ExtensionFactories`. `main.go` passes `prompts.System()`; `cli.Main` assigns it to the package-level `prompts.Preamble` before any session builds a prompt.

Only the preamble is replaceable this way. The tool list, rules, docs, project-context, and skills sections still compose around it, because they depend on runtime facts (selected tools, discovered skills, loaded context files) the root cannot supply. A replaced preamble therefore keeps the stock prompt's structure; it becomes the whole `<preamble>` section and nothing else moves.

Two properties matter for keeping this honest. `prompts.Preamble` is read at render time by both `BuildDefaultPrompt` and `BuildSystemPromptSections`, so startup, resource reload, and per-session rebuild cannot disagree about it; `TestAllBuildersSeeOverride` pins that. An empty or whitespace-only value falls back to the stock paragraph, so a build that never sets the field is byte-identical to before, and clearing it cannot produce a prompt with no opening sentence.

Do not grow this into a prompt file plus a `SYSTEM.md` override layered on top. The runtime already discovers a trusted project `SYSTEM.md` (and `APPEND_SYSTEM.md`) and switches `AppendMode` to `replace` when it exists; that is the user-facing override and it deliberately wins over this embedded default.

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
- Same-provider fallback honors persisted quota resets from sibling processes: successfully mirrored local deadlines yield to deleted or shorter stored blocks on the next routing check. Failed or unverified writes retain local protection, and unrelated auth/policy scopes remain blocked. See `vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go` and `account_routing_reset_test.go` beside it.
- Sessions using the built-in ModelRuntime stream must also route compaction summaries through it without pre-resolving credentials. Explicit credentials pin a single account and bypass model-plan eligibility and account rotation; caller-supplied Agent streams retain their own auth behavior. See `vendor/github.com/MichaelKinsy/PiG/coding/session_summarization_auth.go`.

### Todo tool and its stop-time reminder

`todo` is OMP's task list, ported as native runtime code in `vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/todo`. Both halves are native — there is no extension anywhere in the feature, and `main.go` passes no `ExtensionFactories` at all.

The tool is a **built-in tool**: `base_tool.go` registers it with `tools.RegisterExtendedBuiltinTool` from `init()`, exactly as `websearch` and `codemode` do. So `todo` is created with every session, survives `--no-extensions`, is dropped by `--no-builtin-tools`, and never appears in the startup Extensions list. It is listed in `defaultToolNames` (`internal/codingagent/settings_default_tools.go`) and in `DefaultToolSnippets` (`internal/codingagent/prompts/sections.go`) so it is active at startup and named in the prompt.

The stop-time reminder is a **native before-settle hook**, not an extension handler. `agent_before_settle` is otherwise reachable only through `extension.API`, so `internal/codingagent/native_settle_hooks.go` adds a small hook registry (`RegisterNativeSettleHook` / `NativeSettleHooks` / `HasNativeSettleHooks`) that `Session.runBeforeSettleBoundary` consults after the extension handlers. A hook reads `NativeSettleState` (outcome, branch, last assistant text) and calls `Deliver(customType, content)`, which appends a hidden custom message and continues the run; the same `flushPendingCustomMessages` call that carries an extension's message carries a hook's. The boundary pass is skipped only when *neither* an extension handler nor a native hook is registered.

Why the code lives in the vendored tree: a built-in is linked code, and neither direction of the import works from here. A package outside the module cannot import `internal/codingagent/tools` (Go forbids another module's `internal` tree — `use of internal package ... not allowed`), and a package inside the vendored tree cannot import the module that vendors it. `websearch` and `codemode` sit there for the same reason. Consequence: `go mod vendor` will delete the package; reapply it along with the other personal runtime patches, and re-add its `vendor/modules.txt` line and the `todo_on.go` link shim.

State lives only in each tool result's `details`. Every call re-reads the newest state-changing `todo` result on the session branch (`vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/todo/snapshot.go`), so resume, rewind and fork need no persistence of their own, and the list can never disagree with the transcript. A `view` result and an errored result are both skipped as state: the first changed nothing, the second persisted nothing.

`view` is also exempt from list normalization. `todo.Apply` runs `normalizeInProgress` for a mutating op to keep at most one task active, but that is a write: running it for a read-only op would return a copy the transcript never accepted, demoting a second `in_progress` task in the rendered list. `Op.readOnly` decides this, and `TestViewDoesNotNormalize`/`TestWriteOpsStillNormalize` pin both directions. The tool short-circuits a view before calling `Apply` at all, so the library-level guarantee backs a path the tool no longer takes.

The harness behavior is the part that makes the tool matter. `registerSettleHook` installs the native hook, which reports continue when the list still has open work and delivers a hidden `todo-reminder` custom message. It stays silent, deliberately, when: the run did not complete normally; no list exists; every open task is `blocked`; the agent's last message asks the user something; the list is unchanged since the previous reminder; or the per-run budget of 3 reminders is spent. A `blocked` task is open but never actionable, which is the sanctioned way for an agent to stop: it keeps the list honest while letting the turn end.

`sequential` execution serializes the calls but does not make one call's result visible to the next, so two `todo` calls in one batch each read the same prior list and the later one wins. The tool description therefore instructs one op per call beside the next real action; do not treat batching as safe.

The content is a line-for-line port of OMP's `todoToolRenderer` (`packages/tui/src/tools/todo.ts`) in `vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/todo/render.go`: one `Todo N tasks` line, roman-numeral phase headers when there are several phases, `├─`/`└─` tree gutters, `☑`/`☐` checkboxes, dim `closed/total` progress per phase, and a collapsed walking viewport that leads with the last closed task and caps at 8 rows. The content is reproduced rather than approximated because it is the only place a user watches a plan advance; a layout that differs by a gutter or a windowing rule reads as a different tool.

The content is drawn **unframed**: the tool sets `RenderShell: extension.ToolRenderShellUnframed`, so the rows are the card's whole presentation, with no border, no status bar and no lifecycle background. The list is already a structured block — a title line, phase headers and tree gutters — so the frame would only box it and repeat its title. `ToolRenderShellUnframed` is a pig addition to the `default`/`self` pair (`tool_execution_definition.go` `Unframed`, set from both renderer factories); the frame itself lives in `renderRichFrame`, which only `renderSelfShell` calls for `Rich` cards, so opting out is a routing change rather than a renderer concern. A frame-less card also skips the mouse inset `HandleMouse` applies inside a border.

Two vendor limitations shape the renderer, both handled in it rather than by patching further. First, OMP's `mergeCallAndResult` has no counterpart here, and the card substitutes a name-and-args fallback when a call renderer returns nil, so `renderCall` draws the streaming header and then nothing once `IsPartial` clears. Without that, a settled card shows a stale `● Todo` line above the list. Second, the vendored `Theme` exposes foreground tokens but not OMP's symbol table, so checkbox, tree and tool glyphs are constants in `render.go` holding OMP's default-theme values. A pending todo matched to a live subagent renders dim rather than accent, because no subagent registry is wired into a renderer here.

Interactive todo calls retain compact inline transcript receipts, both live and on resume. `todo_tool_card.go` uses the existing lifecycle-colored status row: `Todo: <op> <target>`, completed/total counts, blocked/dropped counts when present, and recorded timing. Successful receipts stay one visible row even under Ctrl+O; the full list is never duplicated there. Failed receipts show a short reason and disclose at most six diagnostic rows when expanded. Nested calls retain the ordinary parent-card behavior while still updating the widget. Persisted results remain unchanged for the model and reminder. `interactive_info_widgets.go` retains the latest successful non-view snapshot, using the registered unframed renderer inside the Tasks widget so task styling stays canonical. Failed calls display an error alongside the last good list; an empty mutation clears it. Branch rebuilds reset the widget and read tool-result records across the entire branch, not just the post-compaction context. Runtime replacement clears the old plan. Ctrl+O still controls transcript tools, not widgets.

### Prompt-anchored information widgets

`tui.InfoWidgets` is an owner-loop component showing exactly one candidate at a time, in priority order. Tasks (including a todo error) take priority over Quota; quota appears only when no todo list/error exists. A completed but uncleared list still counts as a list. Only the selected renderer runs; switching candidates resets its scroll offset. The whole panel opens upward at the bottom-right, attached directly to the prompt's top border. The persistent `▾ Widgets` / `▸ Widgets` control lives beside the existing quota label on that border; it has its own hit box rather than making quota clickable. Alt+W and the border control share the same toggle. Global collapse removes every panel row and transcript hit box, not just task details, so selecting/copying responses cannot hit the hidden widget. Reopening preserves per-section disclosure. Section headers still toggle their own summary/detail view; wheel scrolling or Alt+PageUp/Alt+PageDown reaches longer content.

There is no top-right relocation, dock fallback, transcript reflow, or space reservation. Panels are capped at 12 rows and the existing visible height above the prompt. Scrolling moves only interior rows: the first header and final border stay fixed, with up/down navigation hints inside the bottom border. Header mouse coordinates account for pinned chrome, and every content row remains reachable. Only the visible interior rows are framed; scrolling never styles offscreen rows or mutates provider-owned slices. Hidden panels and viewports with fewer than three rows skip provider rendering entirely. Fullscreen `InfoWidgetLayout` wraps the complete viewport and finds the allocated editor slot, so status or extension widgets above input cannot separate the panel from its source point. Placement uses only geometry, leaving transcript painting to the frame painter instead of painting and slicing it twice. Regular mode measures only the prompt and rows below it, composites into a copy of existing rows, and registers passive mouse bounds beneath normal modals without claiming modal focus. Prompt text and footer rows remain untouched. Collapsing restores document-only painting and response mouse routing in both modes.

The editor gives the border control priority within its existing trailing-cell budget, then offers the remaining cells to quota; working status and centered scroll hints remain intact. A narrow border can reduce the control to its disclosure glyph. Quota uses the existing minute-refreshed, model/tier-aware cache and never performs credential reads or network work during paint. The full widget-only todo renderer resolves through the extended-builtin registry even without an extension runner. Its private card is not registered for transcript renderer rebinding, which always keeps todo receipts compact.

**Colors follow the palette tokens**, never literal escapes or ad-hoc hex. The frame lines take `border`, the title `accent` in bold with its `Alt+W` hint in `dim`, and the scroll hints inside the bottom border `dim` as well, matching `usage_dashboard.go` and `model_overlay_chrome.go`. The title deliberately does not borrow `border`: the bundled palettes aim that token at lines, so `titanium`'s `#2a3038` would leave the header nearly invisible against its own frame — a regression every width-based assertion would still pass. The border control takes `accent` so it reads as the panel's one interactive affordance rather than another quota readout, and the `▾`/`▸` marker carries the open/collapsed state on its own, so the label still reads on a colorless terminal. Quota rows use `muted` for the window label, `accent` for the provider, `dim` for the reset countdown, and color the remaining percentage by pressure — `error` at exhausted, `warning` at 80% used, `success` below — through the same thresholds as its bar and `tui.StatusColorToken`. A partially reported account is a caveat rather than a failure, so it takes `warning`; only a failed call takes `error`. `TestInfoWidgetPaletteTokens` pins these against the `titanium` palette, since a token that resolves but is the wrong hue still renders and still fits.

This is a native Go adaptation of jcode's margin-widget design, not a Rust compatibility layer: jcode-specific cloud, memory, workspace and swarm backends are not imported. Its observed source revision and MIT notice are recorded in NOTICE and `tui/LICENSE.jcode`. `info_widgets_test.go` and `editor_widget_control_test.go` cover prompt anchoring, palette token use, border quota/control budgeting, zero-space allocation, multiline input, disclosure, complete collapse without transcript hit boxes, Unicode, resize and bounded scrolling; `info_widgets_scroll_test.go` pins both edges at every scroll offset, full content reachability, scrolled header hit targets and tiny-height behavior; `info_widgets_selection_test.go` covers single-candidate priority and scroll reset; `info_widgets_benchmark_test.go` measures host framing with cached 30/300-row lists and complete plain/visible/hidden transcript frames; `interactive_info_widgets_test.go` covers snapshot authority, receipt/widget lifecycle routing, rewind clearing and cached quota windows. `todo_tool_card_test.go` pins compact pending/success/error receipts, expanded error bounds, renderer rebinding, tiny widths and resumed transcript rendering. `scripts/smoke-test-info-widgets.py` drives a loopback model through real fullscreen and regular PTYs under `--no-extensions`, asserting compact transcript receipts, the canonical checkbox renderer, first/last-page scrolling and Alt+W disclosure; it runs in `make smoke`.

`native_test.go` pins the contract: both registries hold the feature (`tools.IsExtendedBuiltinTool("todo")`, one native settle hook), and `TestReminderPolicy` covers every continue decision — open work continues and delivers, an unchanged list is nudged once, a final question is a legitimate stop, settled and blocked-only lists never nudge, and the 3-reminder budget is bounded. Removing either registration fails `native_test.go`. Verified end to end on the binary: the Extensions list holds no todo entry, the model calls `todo` by default and reports `NO_TODO_TOOL` under `--no-builtin-tools`, and a run stopped with 3 open tasks logged two `todo-reminder` custom messages in its transcript.

See `vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/todo/state.go`, `vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/todo/render.go`, `vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/todo/settle.go`, `vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/todo/base_tool.go`, and the tests beside them.

### Theme, palette and colorful Markdown

The theme system is `vendor/github.com/MichaelKinsy/PiG/tui/theme*.go`, a port of `packages/tui/src/theme/` in oh-my-pi. Four things were changed from the stock port; keep them together, because each one only makes sense beside the others.

**The bundled palettes.** `tui/theme_palettes.json` is a verbatim copy of oh-my-pi's `packages/tui/src/theme/defaults.json` (100 palettes, MIT; see NOTICE), embedded with `go:embed` and decoded by `paletteDocuments`. `NewThemeRegistry` lists every name, and `ThemeRegistry.Get` resolves a palette into ANSI sequences on first use, caching it in `paletteResolved`. Resolving all of them eagerly cost ~35ms of the first frame, which is why resolution is lazy; the bundle decode itself is ~2.5ms once. Re-copying the file on an oh-my-pi upgrade is deliberate maintenance, not a generated artifact.

**The default is a palette, not `system`.** `DefaultDarkTheme` is `titanium` and `DefaultLightTheme` is `light`, matching oh-my-pi's `theme.dark`/`theme.light` defaults. `SetThemeSettingPresence`, `presentationTheme.resolveThemeName`, the startup theme in `startup_ui.go`, the settings UI's current theme and the first-run dialog all resolve through `tui.DefaultThemeName`. The system theme is still selectable and still the fallback for a name nothing knows, but it is no longer what an unconfigured install selects. The reason matters: `system` is generated from the terminal's *reported* colors (`system_theme.go`), and a terminal that answers none of the OSC 10/11/4 queries gets `systemIndexedColors`, which leaves 70 of 71 tokens at the terminal default foreground. That is the "everything is white" report — the theme was working as designed, on a terminal with nothing to generate from. A bundled palette is complete before any color arrives, so `MarkTerminalColorsPending` no longer leaves a fresh install grayscale.

**The status line tokens are real.** `statusLine*` and `pythonMode` are upstream schema tokens that the stock port validated and then dropped. They are now `ThemeColor`/`ThemeBg` values with fields on `Theme`, optional in the schema with entries in `themeColorFallbacks`, so a theme written before the status line existed still renders one instead of panicking on `Unknown theme color`. `internal/codingagent/status_line.go` `renderFooter` draws each segment in its own token — path, branch, session name, spend, output, cost, context, model — instead of the single `dim()` wrapper the stock footer used, which is the second half of the plain-text look. `applyContextColor` follows oh-my-pi's four levels (error >90%, thinkingHigh 70-90%, warning >50%, statusLineContext below). There is deliberately no `statusLineBg`: nothing paints a full-width footer background here, and a token nothing draws is dead configuration.

**Colorful Markdown.** `Theme.ANSIFg` paints an ANSI palette slot from the theme's `terminal.ansi` (`resolveThemeANSIPalette`), falling back to the terminal's own SGR 30-37/90-97 entry. `tui/markdown_style.go` builds `GetMarkdownTheme` with per-depth `HeadingAt` and the `TableHeader` member, and gives bold, italic, inline code and table headers palette hues: H1 magenta, H2 yellow, H3+ blue, emphasis magenta, inline code green, headers yellow. All hues come from one palette because themes commonly alias several `md*` tokens to one color. A theme with no palette keeps its `md*` tokens, which is the pre-change rendering. A heading's own emphasis is drawn plain: `headingInline` renders through `renderHeadingInline`, which borrows a child Markdown carrying the stock bold and italic, so a `**bold**` inside a heading does not gain a second hue.

Two constraints hold the render path together. Each colorful role is one package-level closure that reads `ActiveTheme()` when called, never a closure built per `GetMarkdownTheme` call: `markdownPureStyles` and `resumeKey` identify a theme by its style functions' code addresses, so a fresh closure per call would disable the streaming resume path and re-render the whole answer every delta. And the colorful roles replace the stock emphasis functions, so `stockSpanTheme` reports false and the inline renderer takes `renderInlineTokensSequential` instead of the one-pass span writer; measured against an assistant-sized answer, that path allocates the same and renders within noise, so no special case is needed.

The theme schema accepts `terminal` and `symbols` because oh-my-pi writes both into every built-in palette and into custom themes saved from one. `ThemeJSON` decodes `terminal` as `ThemeTerminalJSON`; the loader reads only `terminal.ansi` (the colorful Markdown palette) and `terminal.foreground` (the body color adopted when `text` is unset). `symbols` is accepted and ignored — it is only checked to be an object, since this engine renders no glyph table of its own, and deep-validating a section nothing reads would reject themes for no reason. `colors.go` also accepts `#rrggbbaa`, dropping the alpha as Bun does, because 32 of the bundled palettes use it.

Tests: `tui/theme_palettes_test.go` (bundle loads, every token resolves, registration, defaults, schema acceptance and rejection, optional-token fallbacks, `ANSIFg` fallback), `tui/markdown_palette_test.go` (role hues, token fallback, heading emphasis, width-and-text parity), `internal/codingagent/status_line_test.go` (segment tokens, context thresholds).

**The body color.** `text` is `""` in 96 of the 100 bundled palettes — the token means "the terminal's own foreground" — and every palette records the body color it was designed against in `terminal.foreground` instead. That combination is the rest of the "everything is white" report: assistant prose, dialog descriptions, list rows, login prompts and settings labels all resolve through `Fg("text", …)`, so on a terminal whose default foreground sits close to its background they all render as the same uncolored glyphs, with only headings, code and accents standing out.

`Theme.adoptPaletteForeground` (`theme.go`, called from `resolveThemeWithMode`) closes this at the source rather than at each call site: when a theme's JSON leaves `text` unset, the loader resolves `terminal.foreground` and adopts it as a concrete color, updating `fgAnsi`, the `Text` field, `concreteColors` and `defaultForegroundTokens` together. Every existing `Fg("text", …)` call site then draws the palette's own body color with no call-site change. A theme that writes a real color for `text` is untouched, and a theme assembled in code (no JSON source) keeps whatever it set. Nothing else in the loader depends on `text` staying a default token.

On top of that, `assistantMessageText` (upstream's only `OPTIONAL_THEME_COLOR_RECORD` member) is now a real optional token, and `Theme.ProseFg` returns it when set, else `text`. Assistant prose, the changelog, `/hotkeys` and any `SlashContext.Append` content go through `InteractiveMode.proseMarkdown`, which applies `ProseFg` via `SetDefaultColor` — the same primitive `UserMessageComponent` uses. `proseMarkdown` is the single seam: a Markdown block built with `tui.NewMarkdownWithOptions(..., nil, nil)` anywhere else deliberately has no prose color.

Prose color is resolved per render, not once at construction. A reply outlives the theme it was built under, so both `proseMarkdown` (via its `proseColoredMarkdown` wrapper) and `AssistantMessageComponent.applyProseColor` re-read `ProseFg` before drawing. Capturing it at construction left the previous theme's escape on screen until the message was rebuilt. `SetDefaultColor` returns early when the color is unchanged, so the per-render cost is one comparison. Thinking runs keep `ThinkingText`, a different token.

Two visual fixes came with it. `renderInlineTokensSequential` no longer writes the context's opening codes twice on a text run that follows an inline token: `writeToken` sets a `pending` flag after writing them, and `applyText` strips a duplicate prefix from the next run. Only `writeToken` raises the flag — a text run ends with closing codes, so it clears it, or escaped punctuation and inline HTML (which render as their own runs) would lose their color. And the settings list's unselected label now uses `th.Text` instead of the terminal default, so the list is not the one surface whose rows have no color at all.

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

### The follow-up key

`app.message.followUp` — queue a message behind the current task — defaults to `ctrl+q` on **every** platform column. Upstream keys it by platform (`alt+enter` outside Windows and WSL, `ctrl+q` inside), which leaves the action on a different key per OS and on none at all in terminals that cannot report Option+Return. `ctrl+q` is the Windows default already, so the change only widens it.

The single source of truth is the `appKeybindingRows` entry in `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/keybindings.go`. Nothing else hardcodes the key: the startup header hint (`startup_header.go`), `/hotkeys` (`interactive_hotkeys.go`) and the `/settings` follow-up row (`settings_selector.go`, which passes `ctrl+q` only as the fallback for a registry that has not loaded) all read that table, so changing the row is enough. `keybindings.schema.json` needs no edit either — it renders each action's description, not its default keys. `keybindings_default_test.go` pins the resolved default on all four platform columns, the three input forms a terminal can send (`\x11`, `\x1b[113;5u`, `\x1b[27;5;113~`), and that those inputs classify as `actionFollowUp` and resolve to `outcomeFollowUp` rather than stopping at the matcher.

`ctrl+q` is the XON flow-control byte. It reaches the process because raw mode clears `IXON` in the same ioctl that clears line editing (`golang.org/x/term` `MakeRaw`), so no terminal capability is being assumed; `ctrl+s` (XOFF) is bound the same way by `app.session.toggleSort`, `app.session.rename` and `app.thinking.save`. A terminal that intercepts `ctrl+q` outright — a multiplexer prefix, or an OS shortcut — never sends the byte, which is the general terminal-mapping problem in [keybindings](vendor/github.com/MichaelKinsy/PiG/internal/pigdocs/content/keybindings.md) and is why the action stays rebindable.

Two consequences are deliberate and recorded as divergence D234: `alt+enter`/ESC+Return now reaches the editor, where the editor inserts a newline, so Apple Terminal (which cannot report Option+Return separately) loses its follow-up key entirely unless the user rebinds; and a `keybindings.json` entry for `app.message.followUp` replaces the default rather than adding to it, so `"app.message.followUp": ["alt+enter"]` restores upstream's macOS/Linux key in one line. Do not add `alt+enter` back as a second default key — a multi-key default is rendered as `ctrl+q/alt+enter` in every hint and row that formats the binding.
