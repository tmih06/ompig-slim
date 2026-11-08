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
- `scripts/`: Go checks, native builds, smoke tests, startup measurement, CLI benchmarking, and release verification.
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
```

`make check` runs tests and vet for the root and patched library through `scripts/check-go.sh`. `make smoke` builds the executable and runs its codemode, subagent, and web-tool scenarios from that build; `make smoke-built` runs the same scenarios against an executable CI already extracted from a verified release archive. `make race` repeats tests with the race detector. `make benchmark` compares an already-built executable with pinned OMP and Pi using a local mock model; CI reuses the verified release artifact and never rebuilds inside the benchmark job. See the scripts and CI configuration for exact coverage and platform-specific checks; do not infer that cross-compilation means a target was executed on that platform.

## Project-specific rulings

- Preserve the complete PiG CLI and runtime; `main.go` should remain a thin registration/delegation layer unless a concrete requirement needs otherwise.
- Keep Pi/OMP-compatible storage and configuration behavior. Avoid creating a separate `.pig` profile or changing shared identity formats without an explicit migration plan.
- When modifying account routing, preserve failure scope, session affinity, refresh behavior, and the rule against replaying a request after streamed output. Consult the implementation and tests for the exact contract.
- Treat provider catalogs, auth flows, and quota protocols as provider-specific. Validate protocol changes with local fixtures and keep network/live-service assumptions out of deterministic tests.
- Update this section when an enduring technical ruling changes; keep transient task notes and details discoverable directly from source out of this file.

## Feature behavior notes

### Codex authentication and imported credentials

- Browser and device-code Codex sign-in use the Codex public client and the native `codex_cli_rs` originator. Email-only accounts can authenticate without a workspace ID; workspace headers are optional, while model access and quotas remain subject to the account plan and OpenAI's backend. See `vendor/github.com/MichaelKinsy/PiG/ai/oauth_openai_codex.go` and `vendor/github.com/MichaelKinsy/PiG/ai/auth_providers.go`.
- `/login` presents a searchable list of account and API-key options. Codex JSON imports use provider ID `codex-json`, through `/login codex-json` or `ompig-slim login codex-json`; imported models are selected under that provider.
- The importer persists only the access token, optional refresh token, account ID, and expiry. It gets the account ID from the token or `providerSpecificData.chatgptAccountId`; `expiresAt` (RFC3339) takes precedence over `expiresIn` (seconds), and JWT expiry caps either lifetime. Missing or expired expiry is rejected. Fields such as `idToken`, labels, and subscription metadata are discarded. See `vendor/github.com/MichaelKinsy/PiG/ai/omp_codex_json.go`.
- Imported accounts use the normal quota-aware account router and refresh-token rotation. Definitive refresh failures remove live secrets and routing blocks while retaining an OMP-compatible disabled row. Transport failures, cancellation, rate limits, server errors, and unclassified failures retain credentials. Cleanup is use-driven, not a background timer; browser-login Codex accounts are unaffected. See `vendor/github.com/MichaelKinsy/PiG/ai/omp_codex_json_storage.go` and the routing tests beside it.

### Interactive tool cards

- Interactive cards opt into `ToolExecutionOptions.Rich`: a theme-colored
  frame adds tool-specific labels, preparation/running/final lifecycle status,
  live elapsed time, separate Input/Result sections, and an expansion hint.
  Input contains the call renderer; Result contains streamed or final output,
  with a waiting placeholder before results arrive. Timing appears only in
  the header, not again in built-in result footers. The kit card factory also opts in;
  direct TUI component callers retain the upstream presentation by default.
- The frame wraps existing call/result renderers, including self-framed edits
  and extension renderers, without parsing their output. Images stay outside
  the frame. Mouse coordinates are translated back into renderer space.
- Animation uses owner-loop render ticks, not per-card goroutines. Recorded
  `DurationMs` wins over locally measured elapsed time on completed cards;
  restored cards without timing metadata do not invent a duration.
- Canonical implementation: `tui/tool_execution_rich.go` in the vendored PiG
  tree. Frame widths use terminal cells and ANSI-aware truncation.

### Saved rate-limit resets

- `/usage reset` lists Codex and Claude reset inventory. The interactive spend picker includes only currently redeemable, unexpired resets and requires confirmation twice. Typed targets use `/usage reset <provider>/<account-key>`, including `codex/active` and `claude/active`.
- Codex spends the available credit expiring soonest; Claude rechecks inventory and spends the grant selected by its endpoint. A confirmed spend invalidates the account's quota cache and clears only matching quota blocks, preserving unrelated auth and policy blocks. A Claude 5h-only reset leaves weekly blocks intact.
- Reset redemption is an irreversible mutation and is not automatically retried. If its outcome is uncertain, check `/usage` before trying again. `Session.DispatchSlash("/usage reset")` returns the inventory without opening a picker; print-mode prompts do not dispatch built-in slash commands. See `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_reset.go`, `vendor/github.com/MichaelKinsy/PiG/ai/usage_reset.go`, and `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_usage_reset.go`.
