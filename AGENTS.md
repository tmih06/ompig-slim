# AGENTS.md

Repository-wide guidance for working on `ompig-slim`.

## Project context

`ompig-slim` is a native Go coding agent built on the vendored PiG runtime,
with selected features from `omp`. It is a personal project, not an official
PiG, Pi, or `omp` release. The executable supports interactive TUI, print,
JSONL, RPC, and opt-in durable headless modes. A separate broker binary stores
synchronized profiles and credentials. Inference and ordinary tools stay on
the client.

The module targets Go 1.27.4 and depends on PiG v0.7.0. The maintained PiG
source is checked in under `vendor/github.com/MichaelKinsy/PiG`. Treat that
tree as source, not disposable generated output. Project behavior may live in
the root package, `internal/`, `prompts/`, or the vendored runtime. Read the
implementation and its tests before adding a parallel solution.

Important product boundaries are the following.

- Keep the native core small. Prefer an existing PiG extension or skill when it
  provides the needed behavior. Change runtime code when the behavior belongs
  in the executable or requires a runtime patch.
- Preserve Pi and PiG compatible profile and session paths unless an explicit
  migration is part of the change. User state commonly lives under
  `~/.pi/agent` and `.pi` in a project.
- Codemode and native tools run with the user's OS permissions. They are not a
  security sandbox.
- Do not read, copy, log, or commit credentials, profile databases, tokens, or
  other private user state. Tests must use temporary directories and fixtures.

## Repository map

- `main.go` owns executable registration, startup, broker setup, durable
  dispatch, and delegation to the PiG CLI.
- `internal/` contains project-owned agent state, broker, durable workdesk,
  loop guard, goals, self-update, and related integrations.
- `cmd/ompig-slim-broker/` contains the broker executable entry point.
- `vendor/github.com/MichaelKinsy/PiG/` contains the PiG runtime and maintained
  local patches. Inspect it for inherited CLI, model, tool, session, and TUI
  behavior.
- `prompts/system.md` is the embedded base system prompt. Edit the Markdown,
  not Go source, when changing prompt prose.
- `scripts/` contains dependency bootstrap, checks, builds, release tooling,
  smoke tests, and installer tests.
- `docs/` contains durable behavior, broker contracts, model roles, tools,
  benchmarks, and other technical documentation.
- `themes/` contains profile theme palettes.
- `Makefile` contains the canonical developer commands.
- `README.md` is user-facing. `CONTRIBUTING.md` contains contributor and
  dependency rules. `SECURITY.md` contains security-reporting guidance.

## How to work in this project

1. Read the relevant README, documentation, implementation, and tests before
   changing behavior. Search the vendored PiG tree too.
2. State the user-visible outcome and the boundary that owns it before coding.
   For stateful or branching work, define the data shape and invariants first.
3. Keep one change focused on one behavior. Reuse the existing owner of that
   behavior instead of adding duplicate helpers, state, formats, or settings.
4. For a bug, reproduce the failure before changing code. Trace it to the first
   wrong decision, then fix that cause rather than adding a guard at the place
   where the symptom appears.
5. Add or update a regression test for bug fixes and boundary behavior. Drive
   tests through the user's or caller's interface and assert observable
   behavior, not private call order or helper structure.
6. Use the Makefile targets rather than ad-hoc Go commands. Run the narrowest
   relevant check after each coherent change, then broaden validation.
7. Preserve unrelated local and vendored changes. Review `git status` and the
   exact diff before staging. Stage explicit paths and never force-push.
8. Update user-facing README text and durable technical docs when behavior or
   contracts change. Keep comments and documentation synchronized with code.

### Boundaries and state

Validate and parse external data at the boundary. This includes CLI arguments,
configuration, environment variables, network payloads, RPC messages, database
rows, and serialized files. Convert it to typed domain data once, then trust
the type inside the system. Keep business logic independent of framework and
transport code when the separation is useful.

Operations that can be retried, restarted, or resumed must converge to the same
state after repeated execution. Ask what happens after a crash at each step.
Use atomic writes, ownership, locks, or compare-and-swap when concurrent actors
really share state. Prefer separate write targets when they do not need to share
one mutable object.

### Skills and user-facing verification

Use the repository's skills when they match the work. Read the relevant
`SKILL.md` before following it rather than inventing a weaker process.

- Use `.agents/skills/verify-ompig-slim/SKILL.md` after changing the TUI,
  startup, model roles, or durable user flows. It drives the real binary in an
  isolated PTY and records observable evidence.
- Use the project's verification helpers and `make smoke` for runtime behavior.
  Do not treat compilation, a unit test of an internal helper, or a final
  terminal frame as proof of a user flow.
- For changes that cross function or subsystem boundaries, design the data
  shape and ownership before implementation. Keep the shell thin and the
  domain logic testable.
- For a multi-step migration or sweep, make each unit verifiable before moving
  to the next. Do not build further on a failing intermediate state.

### Common commands

```sh
make check                 # Go tests and vet
make race                  # tests with the race detector
make build                 # build bin/ompig-slim
make smoke                 # build and run PTY and codemode smoke scenarios
make test-install          # installer tests against a loopback server
make test-self-update      # update and upgrade behavior against a built binary
make test-broker-container # broker image and client integration; needs Docker
make test-broker-cluster   # multi-client broker integration; needs Docker
```

CI also checks scripts, release archives, notices, secrets, and Linux and
macOS build behavior. Cross-compilation does not prove runtime behavior on a
platform that was not executed.

### Dependencies and vendored code

`go.mod`, `go.sum`, and `vendor/modules.txt` are the dependency records.
`make deps` restores unmodified dependencies while preserving the maintained
PiG tree and license notices. Do not run `go mod vendor`, because it can erase
maintained PiG patches. Do not run dependency restores concurrently in one
checkout.

If a PiG upgrade is necessary, inspect the upstream diff and deliberately port
local changes. Do not replace the vendor tree blindly. Keep dependency updates
separate from unrelated behavior changes.

### Durable and broker changes

Treat `docs/durable.md` and `docs/remote-broker.md` as the canonical contracts
for those subsystems. Preserve crash recovery, locking, credential ownership,
profile synchronization, and failure semantics described there. Do not claim
exactly-once external effects or silently fall back to a local credential vault
without an explicit contract and tests. Broker tests must use isolated data and
must verify that secrets do not appear in logs or snapshots.

## Engineering principles

- **Prefer deletion.** Remove dead paths before adding abstractions. Add a
  layer, hook, parameter, or compatibility path only for a current requirement.
- **Keep the data model explicit.** Make illegal states hard to represent. Put
  shared invariants in types, tables, registries, or state machines instead of
  repeating conditionals across callers.
- **Fix causes, not symptoms.** A guard is a fix only when the state it handles
  is valid. Otherwise find what produced the invalid state.
- **Keep one source of truth.** Consolidate repeated decisions and derive
  dependent values from authoritative state.
- **Minimize reader load.** Collapse one-caller wrappers, avoid hidden mutable
  state, and keep a boundary meaningful enough to hide real decisions.
- **Preserve compatibility deliberately.** Keep established paths, formats,
  flags, and workflows unless the change includes a migration and its checks.
- **Make security part of correctness.** Minimize secret exposure, validate
  trust boundaries, and report errors without credentials.
- **Prove the real artifact.** Read the resulting file, run the built binary,
  or inspect the persisted state. Do not claim success from compilation or an
  indirect proxy.
- **Keep comments rare and useful.** Write a comment only for a non-obvious
  reason, contract, invariant, ownership rule, or concurrency constraint that
  the code cannot express. Do not narrate steps or restate syntax.
- **Encode recurring lessons.** When a rule needs repeating, prefer a test,
  type, lint, metadata flag, runtime check, or script over another paragraph
  of instructions.

## Quality gate

Every change must pass the gate before it is done. It is one command:

```sh
make hooks       # configure git core.hooksPath=.githooks (also auto-wired by make build/check/gate)
make gate        # full gate: build, lint(new code), vet, slop, diff, ratchet,
                 # tests -race -shuffle, govulncheck, manifest drift
make gate-quick  # cheap subset run by .githooks/pre-commit (build, lint, vet, slop, diff, ratchet)
```

Run a single linter when iterating:

```sh
golangci-lint run --new-from-rev=origin/main ./...   # lint only new/changed code
golangci-lint run ./internal/broker/                  # one package
semgrep --config .semgrep/slop.yml path/to/file.go    # slop rules on one file
python3 scripts/quality/semgrep-diff.py --base origin/main
python3 scripts/quality/ratchet.py lint.json          # per-rule debt ratchet
```

### Gate rules

**Never bypass the gate.** Never add `//nolint`, `t.Skip`, or a disabled rule
to make it pass. Never edit `.golangci.yml`, `.semgrep/`, `scripts/quality/`,
`.github/workflows/`, `docs/quality-baseline.*`, `CODEOWNERS`, `.githooks/`, or
this file to loosen a check — those paths are CODEOWNER-protected and the
diff-check fails the PR without owner approval. If a rule seems wrong, stop
and ask a human; do not weaken the gate.

New and changed code is gated strictly; existing debt lives in
`docs/quality-baseline.json` and may only shrink (`scripts/quality/ratchet.py`).

### Conventions (measured in docs/quality-discovery.md)

- **Errors**: return `fmt.Errorf("context: %w", err)`; `errors.New` for
  sentinels; `errors.Is`/`As` at boundaries; `errors.Join` for cleanup. See
  `internal/workdesk/workdesk.go`.
- **Logging/diagnostics**: write to an explicit `io.Writer`
  (`fmt.Fprintln(os.Stderr, …)`), never a logger or `fmt.Print*`. See
  `internal/broker/migrate.go`.
- **Context**: `context.Context` is the first parameter. `context.Background()`
  only at process root or in cleanup after a cancelled ctx; derive children
  with `WithCancel`/`WithTimeout`. See `internal/broker/client.go`.
- **Private state**: all credential/profile filesystem writes go through
  `internal/privatefs` (`EnsurePrivateDir`, `RestrictFile`, `SyncDir`).
- **Concurrency**: channels + `sync.WaitGroup` + `signal.NotifyContext`; no
  errgroup. Every `go func` has a ctx/done contract. See `main.go`.
- **Tests**: stdlib `testing` only — no testify. `t.TempDir`, `httptest`,
  fixture credentials, `t.Helper` in helpers. Platform splits via
  `//go:build` + `_unix.go`/`_windows.go`.
- **Imports**: `main`/`cmd` → `internal/*` + `prompts` + PiG; `internal/*`
  may import PiG and siblings, never `main`.

### Root-cause fixes must leave a guardrail
Whenever you fix a bug or defect by finding its root cause, you MUST also add a
rule that prevents the same class of problem from recurring, in the same PR.
Do this in order of preference:

1. **Mechanical enforcement**: a new or tightened golangci-lint setting, a
   Semgrep rule (with positive and negative fixtures), a forbidigo/depguard
   entry, or a regression test that fails without the fix.
2. **A written rule** in the "Learned rules" section below, when it cannot be
   enforced mechanically.

Every entry in "Learned rules" must follow this format, be specific and
testable, and be checked for duplicates first (merge with or sharpen an
existing rule instead of adding near-copies):

- **Rule**: one imperative sentence.
- **Why**: the incident or bug that caused it (PR, commit, or issue link).
- **Enforced by**: the lint rule, Semgrep id, or test name, or "manual" if none.

A fix PR that adds no rule must say why in its description (for example,
"one-off typo, no recurring class"). Do not add vague rules like "be careful
with errors". If a rule can't be made specific enough to check, don't add it.

### Learned rules

- **Rule**: Scope `context.Background()`/`TODO()` to the process root or
  cleanup-after-cancel; a function that takes a `ctx` must derive children
  from it and never accumulate nested contexts in a loop.
  **Why**: durable/goal lifecycle fixes (`#106`, `#99`, `#94`) traced to a
  cancellation that never reached the work; a fresh root context would have
  detached it.
  **Enforced by**: `contextcheck`, `fatcontext`, `containedctx`, semgrep `slop.context-in-request`.

- **Rule**: Give every goroutine a cancellation and a completion signal owned
  by its caller (and call `wg.Add` before `go func`); no bare `go func(){...}()`.
  **Why**: subagent/heartbeat lifecycle bugs (`#97`, `#95`, `a62bd87`) where a
  worker outlived its run or missed its deadline.
  **Enforced by**: semgrep `slop.bare-goroutine`, `slop.waitgroup-add-in-goroutine`, `go vet`, race tests.

- **Rule**: Never discard an error with `_` on a result you must act on;
  deliberate discards (cleanup, encode-after-headers) get a comment.
  **Why**: credential-sync and migration paths (`#127`, `b6baba8`) where a
  swallowed error could corrupt account or sync state.
  **Enforced by**: `errcheck` (`check-blank`, `check-type-assertions`), `nilerr`, `nilnesserr`, `forcetypeassert`, semgrep `slop.blank-error-assign`, `slop.empty-err-check`.

- **Rule**: Route all credential and profile filesystem writes through
  `internal/privatefs` using explicit `0o` octal modes; do not hand-roll
  `os.MkdirAll(0o700)`/`WriteFile(0o600)`.
  **Why**: credential files must be 0700/0600 and atomic; a hand-rolled path
  (`#127`, `4f1c7d5`) drifted from the enforced permission/atomicity contract.
  **Enforced by**: semgrep `slop.reimplement-privatefs`, `slop.decimal-file-mode`.

- **Rule**: Make enum/state switches exhaustive over every member; do not rely
  on `default` to absorb a new state.
  **Why**: goal/goalOp state machines (`#131`, `4caf517`) added operations that
  a non-exhaustive switch silently dropped.
  **Enforced by**: `exhaustive` linter.

- **Rule**: Pass `context.Context` to `os/exec` and `net/http` using an
  explicit `http.Client` with a timeout and `http.NewServeMux()`; never use
  `exec.Command`, `http.DefaultClient`, or `http.DefaultServeMux`.
  **Why**: a runaway child or request without a deadline blocked the update/
  durable path.
  **Enforced by**: `noctx`, `bodyclose`, `forbidigo`, semgrep `slop.http-default-client-or-mux`.

- **Rule**: Do not place `defer` or `time.After` inside loops; close resources
  per iteration (or via a helper) and use `time.NewTicker`/`time.NewTimer`.
  **Why**: function-scoped `defer` holds open file handles (blocking Windows
  rename/remove in `privatefs` and `selfupdate`) and `time.After` leaks timers
  in long-running heartbeat/poll loops.
  **Enforced by**: semgrep `slop.defer-in-loop`, `slop.time-after-in-loop`.

Maintenance: when a learned rule becomes mechanically enforced, update its
"Enforced by". Prune rules that no longer apply. Keep this file scannable — if
it grows past ~200 lines, move detail to `docs/rules/` and link it.

## Commits

Use Conventional Commits for every commit.

```text
<type>[optional scope]: <subject>

<body>

[optional footer]
```

Allowed types are `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `build`,
`ci`, `chore`, `style`, and `revert`. Write an imperative subject of at most
72 characters without a trailing period. The body is required. Explain why the
change exists, what changed, the approach and trade-offs, and anything a
reviewer needs. Wrap text at 72 columns. Use footers for issue references
(`Refs: #123`, `Closes: #123`). Document breaking changes with a `BREAKING
CHANGE:` footer. Before committing, reread the staged diff and ensure the
message describes exactly what is staged.
