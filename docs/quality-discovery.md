# Quality discovery

Measured on `feat-better-lintin` at HEAD before any quality-gate change. Every
number below was produced on this checkout; do not copy thresholds from
textbooks.

## Stack

- **Language**: Go 1.27 (toolchain `go1.27.4`), `godebug winsymlink=0` in `go.mod`.
- **Module layout**: single module `ompig-slim`. No `go.work`. The PiG runtime is
  a maintained vendored tree at `vendor/github.com/MichaelKinsy/PiG` (local
  patches, treated as source, linted by `scripts/check-go.sh`). All other
  dependencies are vendored but disposable — restored by `make deps`
  (`scripts/bootstrap-vendor.sh`). **`go mod vendor` is forbidden**; it erases
  the PiG patches.
- **Frameworks**: no HTTP framework, no ORM, no DI. `net/http` (stdlib) for the
  broker. `modernc.org/sqlite` for durable journals. TUI and model access come
  from vendored PiG. External test/runtime deps: `github.com/gofrs/flock`,
  `go.yaml.in/yaml/v3`, `github.com/dlclark/regexp2`.
- **Build/test/CI**: `Makefile` → `scripts/check-go.sh` (modes `check`, `test`,
  `race`, `vet`). `make check` = `go test ./...` + PiG packages via GOPATH
  symlink + `go vet` on both trees. CI: `.github/workflows/ci.yml` (Go
  check+race on ubuntu/macos/windows, PTY smoke shards, broker container
  suites, gitleaks, actionlint, release archives, GHCR images).
- **Generated code**: none. No protobuf, mocks, sqlc, ent, or `go:generate`
  outside vendor. The only code to exclude from linting is `vendor/` (which
  golangci-lint excludes by default) and the Go-build cache GOPATH symlink root.

## Existing tooling

- No `.golangci.yml`, no Semgrep config, no git hooks (`core.hooksPath` unset),
  no `tools.go`. `golangci-lint` is not currently a dependency.
- `scripts/check-go.sh` is the canonical test+vet entry; extend it, do not
  replace it.
- `AGENTS.md` (repo root) and `CONTRIBUTING.md` already encode conventions:
  Conventional Commits, "bug fixes need a meaningful regression test", "keep
  the native core small", "prefer deletion", "fix causes, not symptoms".
- `.gitleaks.toml` (secret scanning) and `.coderabbit.yaml` (AI review) exist.

## Baseline (measured before changes)

| Check | Result | Duration |
|-------|--------|----------|
| `go build -mod=vendor ./...` | pass | ~3 min cold (after `make deps`) |
| `go vet -mod=vendor ./...` | pass | ~39 s |
| `go test -mod=vendor ./...` | pass, all 13 test packages | ~68 s |
| Total coverage | **76.7%** | — |
| Packages with no tests | `cmd/ompig-slim-broker`, `internal/modelcontext` | — |

Per-package coverage: prompts 100%, loopguard 95.8%, streamingrules 95.5%,
configroot 90.9%, goal 89.2%, privatefs 88.9%, agentstate/ext 89.1%,
agentstate 85.7%, goalext 80.6%, workdesk 79.8%, broker 74.6%, selfupdate
70.8%, main package 10.9% (thin entry shim), cmd/ompig-slim-broker and
modelcontext 0%.

## Real conventions (evidence)

- **Errors**: `fmt.Errorf("context: %w", err)` for wrapping (41 `%w` sites),
  `errors.New` for sentinels/messages (33), `errors.Is`/`As` at boundaries
  (15), `errors.Join` for cleanup. Named return `err` used with `defer` +
  `errors.Join` in `workdesk.Run`. See `internal/workdesk/workdesk.go`,
  `internal/broker/client.go`.
- **Logging**: **no `slog`/`log` package**. Diagnostics go to an explicit
  `io.Writer` — `fmt.Fprintln(os.Stderr, err)` / `fmt.Fprintln(stderr, ...)`.
  See `main.go`, `cmd/ompig-slim-broker/main.go`, `internal/broker/migrate.go`.
- **Context**: `context.Context` is the first parameter (11 files).
  `context.Background()` appears only at process root (`main.go`, broker
  `main.go`) or for shutdown/cleanup timeouts (`workdesk.go`,
  `client.go:476`). `signal.NotifyContext` wires SIGINT/SIGTERM.
- **Package layering**: `main` → `internal/*` + `prompts`. `internal/*` may
  import PiG and sibling internal packages. Observed edges:
  `workdesk`→`modelcontext,privatefs`; `broker`→`privatefs`;
  `selfupdate`→`configroot`; `goalext`→`goal`; `agentstate`→`configroot`.
  `cmd/ompig-slim-broker` → `internal/broker`. `extensions/herdr` → PiG SDK.
  No cycles, no internal pkg importing `main`.
- **Concurrency**: channels + `sync.WaitGroup` (5) + `signal.NotifyContext`;
  **no `errgroup`**. `go func` appears 14×, almost all in tests or with a
  done-channel/ctx contract (`main.go:94` conflict pump, `herdr.go:379`
  `go s.watch(watchCtx…)`). `internal/agentstate/ext` heartbeat is a tracked
  `go t.beat(...)`.
- **Tests**: stdlib `testing` only — **no testify**. Helpers take `t` and call
  `t.Helper`. Fixtures use `t.TempDir`, `httptest.NewServer`, fixture
  credentials. `t.Parallel` is unused. Platform splits use `//go:build
  windows|unix` + `_windows.go`/`_unix.go`/`_other.go`. Test files often exceed
  complexity limits — table-driven cases in one function.
- **Approved helpers** (reuse, do not reimplement): `internal/privatefs`
  (`EnsurePrivateDir`, `RestrictFile`, `SyncDir` — all credential-state IO must
  go through it), `internal/configroot` (`Dir`), `internal/agentstate`,
  `internal/goal`, `internal/loopguard`. `docs/` documents durable and broker
  contracts.

## History mining

`git log` over the last ~100 commits: **44 fix, 26 feat, 12 chore, 4 perf, 3
test, 3 docs, 2 refactor, 1 ci** — 44% of commits are fixes. Recurring bug
classes, from commit subjects and bodies:

| Bug class | Example commits | Tool-catchable? |
|---|---|---|
| Credential/account ordering & drift (auth) | `#121 #127 #114 #105 #75 #68` | partly (state-machine rules hard) |
| Goal/durable lifecycle state machines | `#131 #130 #106 #99 #94 #90 #69` | partly; goroutine/context rules help |
| Subagent lifecycle, deadlines, grace periods | `#133 #111 #97 #93 #95` | yes — leaked goroutine/deadline checks |
| Broker sync conflicts/ancestry | `#107`, `b6baba8` | partly (conflict-channel patterns) |
| Streaming/decode edge cases | `#113`, `04a7dbe` | no (semantic) |
| TUI rendering/cost rounding | `#126 #112 #73` | no (visual) |

Sloppy-pattern signal is low: **0 `//nolint`, 1 `t.Skip`, 0 `panic(`, 0
`fmt.Print` in prod except `cmd/.../main.go` (version) and one `goalext` UI
print, 0 TODO/FIXME/HACK**. The real risk this gate addresses is *new*
AI-generated code, not a dirty baseline — so `new-from-rev` gating plus strict
thresholds for new code is the right posture.

## Measurements → proposed thresholds

| Signal | Current | Proposed threshold | Reason |
|---|---|---|---|
| `//nolint` | 0 | 0 on new code; require explanation+specific | keep it that way |
| `t.Skip(` | 1 | 0 on new code (diff-check) | no new skips |
| `, _ :=` ignored errors | 91 (19 prod, mostly `strings.Cut`/`json.Marshal`/type-assert) | errcheck `check-blank`; allow listed idioms | prod uses are idiomatic; ban in new code via errcheck-blank |
| `panic(` | 0 | forbidigo | none exist; keep it |
| `time.Sleep` | 13 (all tests + `replace_windows.go` retry) | forbidigo in prod; allow tests | sleeps are a test/retry smell |
| `fmt.Print*` | 3 (CLI version + one UI) | forbidigo; allow `os.Stdout/Stderr` writes | repo writes via `io.Writer`, not stdout defaults |
| TODO/FIXME/HACK | 0 | godox `TODO,FIXME,HACK,XXX` | keep at 0 on new code |
| `interface{}`/`any` | 129 (mostly `map[string]any` JSON payload) | not gated | legitimate JSON/enum vocabulary |
| pkg-level `var` | 16 | gochecknoglobals on new code | small, controlled |
| `init()` | 0 (outside blank imports) | gochecknoinits | none exist |
| bare `go func` (prod) | ~4, all ctx/done-scoped | Semgrep rule: require ctx/errgroup | prevent leaked goroutines |
| `context.Background()` | 136 (mostly root/cleanup/tests) | Semgrep: flag inside request-path fns only | root use is correct |
| gocognit | p50=5 p90=19 p99=48 max=132 | new-code warn 20, block 40 | p99≈48; block above ~p95 |
| cyclop | >6: p50=10 p90=22 p95=29 p99=42 max=56 | new-code max 30 | p95≈29 |
| nestif | p99=2 max=40 (test) | min-complexity 5 | most code flat; deep nesting rare |
| funlen | all <60 lines, <40 stmts | 80 lines / 50 stmts | headroom above observed max |
| dupl | 19 blocks ≥40 tok in 11 files | threshold 100 tokens, new code | moderate clone debt |
| coverage | 76.7% total | changed-lines ≥70%, no drop below 76% | ratchet from real baseline |
| dead code | unused: clean | `unused` + `unparam` | already clean |
| gosec | not measured | enable, audit findings into baseline | first run will reveal debt |
