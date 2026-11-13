# Pi Durable, PiG, and a crash-recoverable software-factory workdesk

> Historical baseline research at the local revision listed below. The subsequent
> implementation adds an opt-in Durable host; see [the current contract](../durable.md).
> Statements about what the binary links refer to that pre-integration baseline.

## Findings

**Pi Durable exists, and PiG v0.7.0 implements it. This `ompig-slim`
executable does not currently run on that implementation.** The appropriate
starting point is to evaluate integrating PiG's existing Durable harness, not
to invent another checkpoint engine around the terminal coding-agent loop.

The earlier conversational assessment was incomplete: it correctly distinguished
saved history from execution recovery, but did not establish that the dependency
already contains a separate durable execution framework. [P1, G1, G2, L1–L4]

| Component | What it actually provides |
|---|---|
| Pi coding agent | Terminal-oriented coding runtime; persisted sessions are not Pi Durable. |
| Pi Durable | Separate experimental framework with storage-backed conversations, tasks, checkpoints, inboxes, and recovery. |
| PiG / Pi in Go | Independent, parity-oriented Go port of Pi; includes a Durable implementation and an additional core/driver implementation. |
| This `ompig-slim` build | Uses PiG `coding/cli` and `coding.Session`; links only a JSON utility from the `durable` tree, not the Durable scheduler. |
| Kubernetes Agent Sandbox | Manages the isolated, stateful pod and optional persistent storage; application execution recovery remains the harness's responsibility. |

Sources: [P1–P3, G1–G4, L1–L4, K1].

## Scope and revision discipline

These are primary-source findings, not a production-readiness certification.
The inspected revisions are:

- Local `ompig-slim`: `7a116e03d4373611233a3cebda0f0bef1ac26d62`,
  with `github.com/MichaelKinsy/PiG v0.7.0` in `go.mod` and a maintained vendor tree.
- PiG v0.7.0: `beb56447c476da70d05d8e1e1ca9e993b367cb8b`.
- Pi behavior pin recorded by that PiG release: Pi v1.5.0,
  `f9a26365568127461c47a07a37dcb42c140a3ac5`.
- Additional upstream Pi comparison: main at
  `ab74d46243e741e20077d244d30d2d673de40b5f`.

The v1.5.0 pin and current Pi main are **not interchangeable**. Current upstream
README examples include signals and tools that park for webhooks/approvals.
Those must not be assumed available in the pinned Go dependency: the inspected
PiG v0.7.0 `durable` Go source has no `DefineSignal` API. Conversely, PiG's root
README describes its scope too narrowly to establish whether Durable exists;
the package source and tests demonstrate that it does. [G1–G3, P2, P3]

## 1. What “Pi Durable” means

Earendil explicitly describes Pi Durable as a **separate agent-application
framework**, not a replacement for the Pi terminal coding agent. The published
package is `@earendil-works/pi-durable`, built on Pi AI for model access and Chord
for document state. It is marked experimental; API stability should not be
assumed. It is not simply “Pi running in a Cloudflare Durable Object.” Node
SQLite, JSONL, memory, and other storage adapters exist. [P1–P3]

### Execution model

A harness opens storage. Conversations contain immutable transcript entries and
mutable typed documents. Work executes as durable state-machine tasks:

```text
submission -> user entry -> pi.generation
                            -> assistant tool calls
                            -> pi.tool tasks
                            -> further generation -> answer / failure
```

Commits can atomically change documents, append entries, and create or advance
tasks. Task state includes checkpoints; recovery reruns the interrupted phase
from its beginning, not from an arbitrary instruction or an in-memory stack.
The same application code, registry, models, and environment must be supplied
again when reopening. Missing or incompatible definitions can block work; code
and credentials do not magically become serialized executable state. [P2, G2]

### What survives and what happens on restart

| Interruption | Pi Durable behavior |
|---|---|
| Input accepted but reply lost | Reuse `requestId` to find the admitted submission rather than admitting another one. This is submission deduplication, not exactly-once external execution. |
| Process stops during model generation | Recover the stored generation state; interrupted requests can be resent under retry policy. A committed partial is retained as an aborted attempt, not resumed at the provider's next token. |
| Tool intent committed, tool result not committed | Rerun only a replay-safe tool; otherwise produce an interrupted error telling the model it may have partially run. |
| Completed nested tool call inside replay-safe caller | Reattach by stable call key and return the stored result instead of starting it again. |
| Durable sleep/backoff | Restore the stored deadline and checkpoint, and resume when due. |
| Repeated crashes without progress | Restart limits bound crash loops rather than retry forever. |

The host must reopen the storage and start scheduling (`resume()`, or an API
that starts scheduling); persisted pending work is not a self-starting process.
Registry availability, retry policy, and terminal failure outcomes still matter.
[P2; G2, G5, G6]

### Safety boundary: tools are not exactly once

PiG's `ToolRegistration.Replay` defaults to unsafe. Tool preparation records an
execution intent before invoking the tool. Recovery checks the recorded policy
and the current registration before replaying a safe tool; unsafe interrupted
calls settle with “may have partially run.” [G5]

This prevents an automatic blind replay, but does **not** prevent a model from
subsequently issuing another equivalent command. A GitHub PR tool, CI trigger,
notification, or deployment still needs its own stable operation identity and
external-state reconciliation. Declaring `ReplaySafe` is a contract the tool
implementation must satisfy, not an idempotency mechanism supplied by the flag.
[P2, G5]

### Durable Code Mode is materially different from ordinary Code Mode

PiG implements `durable/codemode`. It journals delivered results before giving
them to the script, stores start conditions, replays deterministically, and
reattaches nested calls by key. It does not serialize an arbitrary QuickJS heap.
A changed VM fingerprint/version, deadline expiry, or divergent replay can end
the call with an error. An interrupted nested call still follows its own replay
policy. Importing ordinary Code Mode does not opt into these guarantees. [G7]

### Persistence caveats

- SQLite uses WAL and `synchronous = NORMAL` by default in the inspected native
  adapters. This supports process-crash recovery but does not guarantee retention
  of every acknowledged commit across power/host failure. Preserve the WAL with
  the database; use SQLite-aware backup procedures. [P2, P3, G8]
- JSONL offers an explicit fsync option upstream; memory storage is not durable.
  Choosing the backend is part of choosing the guarantee. [P2, P3]
- Deferred progress commits are an explicit exception to commit-before-display:
  eligible document updates can be shown before storage and lost in a crash.
  Task-state changes and transcript entries are not merely deferred progress.
  Do not use displayed streaming progress as an externally durable receipt. [P2, P3]
- Storage is single-writer at the harness ownership level. SQLite transaction
  locking is not a substitute for preventing two independent task schedulers
  from executing the same pending external work. [P1–P3]

## 2. How much PiG implements

There is real implementation, not just a planned feature or a package name:

| PiG package | Role |
|---|---|
| `durable` | Records, interfaces, typed tasks/documents, replay and settings contracts |
| `durable/session` | Transaction/commit line and document state |
| `durable/harness` | Conversations, generation/tool tasks, scheduler, submissions, recovery |
| `durable/storage/sqlite/node` | Native SQLite storage opener for the Go harness |
| `durable/storage/jsonl` | JSONL storage implementation |
| `durable/codemode` | Journaled, deterministic, restart-aware Code Mode |
| `durable/core/driver` | Alternative native core-backed harness with the same `harness.Harness` interface; opens SQLite itself |

The two entry points are `harness.OpenHarness(ctx, storage, options)` and
`driver.OpenHarness(ctx, path, options)`. These are **alternative engines**, not
two layers every host must stack. The driver documents reading Pi Durable
SQLite files and upgrading older schema versions in place. [G2–G4, G7, G9]

PiG includes tool/generation recovery tests, nested-call recovery tests,
Code Mode restart comparisons against Pi, and a differential crash-handoff gate
that describes real-process SIGKILL and Pi-to-candidate store handoffs. The
existence of this infrastructure is stronger evidence than a feature summary,
but is not evidence that every scenario passes in this checkout or deployment.
Its contract README still references an older Pi baseline; the active `.upstream/pin`
and individual evidence must be checked rather than treating all prose as current.
[G1, G6, G7, G10]

Do not infer complete parity from the word “faithful.” PiG's own coverage report
lists Durable source coverage below 100%, and records divergences. That metric
is a source-port/evidence inventory, not a quantified reliability probability.
[G11]

## 3. What this harness actually uses

The local execution path is:

```text
main.go -> coding/cli.Main -> coding.NewRuntime -> coding.Session
        -> internal/codingagent session JSONL + ordinary agent runner
```

`main.go` delegates to `cli.Main`. The CLI session factory constructs a
`coding.NewRuntime` and creates its session with `low.New(start)`. Neither this
path nor the dependency graph opens a `durable/harness` or `durable/core/driver`
instance. [L1, L2]

Verification command:

```sh
go list -mod=vendor -deps .
```

Of packages under `github.com/MichaelKinsy/PiG/durable`, the result contains only:

```text
github.com/MichaelKinsy/PiG/durable/core/jv
```

That package supplies JavaScript-compatible JSON primitives used by provider
encoding and JSON helpers. It is **not** the checkpoint scheduler. The vendor
tree likewise contains only `durable/core/jv` from the Durable subtree. This is
consistent with dependency-based vendoring, not proof the upstream implementation
is absent. [L3, L4]

The ordinary session implementation appends JSONL records; the inspected
`appendSessionLine` writes and closes without an fsync. Loading restores entries,
identity, and branch/context state. This provides useful conversation persistence,
but not the separately implemented Durable task and tool recovery contract.
SQLite credentials and the remote broker do not change the agent engine. [L5]

Consequently, switching to Durable is an integration project, **not a flag**.
Existing coding-extension factories, goal/todo behavior, MCP integration, system
prompt composition, tools, and session formats must be assessed individually.
Shared AI/provider infrastructure does not imply the two extension APIs or
transcript formats are interchangeable. Do not overwrite an existing coding
session or credential database with a new workdesk store. [L1, G2, G5, G7]

## 4. Fit for Kubernetes Agent Sandbox

Agent Sandbox describes itself as a sandbox orchestrator. It manages a singleton
stateful pod, stable identity, lifecycle, and optional persistent storage;
low-level isolation is delegated to runtimes such as gVisor or Kata through
`RuntimeClass`. It does not persist Pi's in-memory scheduler or shell process
state for the application. [K1]

**Recommended architecture (a proposal, not existing behavior):**

```text
Factory controller
  ticket admission, work identity, policy, ownership, retries/approvals
      |
Agent Sandbox pod
  Go workdesk host -> PiG Durable harness -> registered workdesk tools
      |
Persistent volume
  workdesk SQLite (+ WAL), repository/.git, uncommitted files, retained artifacts
```

Start with one ticket/workdesk and one active owner. Reopen the same workdesk
store on container startup, reinstall the pinned registry/environment, and call
`Resume()`. Use the factory assignment/attempt identity as a stable submission
request ID across transport retries; deliberate new work needs a new identity.
Keep the external factory in charge of terminal failure policy rather than
turning every failure into an unbounded continuation loop.

The remaining application work is narrower than implementing durability from
scratch, but still substantial:

1. **Choose the engine deliberately.** Prototype the direct Go harness first;
   compare the core driver if its deployment properties are needed. The source
   shows both exist, not which is best for this product.
2. **Port the workdesk surface.** Register required tools, prompts, and state on
   Durable's APIs; expose submit/status/watch/abort and explicit result artifacts.
3. **Preserve both execution state and workspace.** A transcript checkpoint is
   not an atomic snapshot of Git, arbitrary files, SQLite, and external services.
4. **Own external effects.** Use dedicated reconciling push/PR/CI tools where
   correctness matters. A general `bash` command cannot promise exactly once.
5. **Guarantee exclusive recovery.** Use ownership/locking with storage-appropriate
   fencing; a stable pod name alone is not proof an old worker can no longer act.
6. **Define the failure level.** Process death with the volume intact, node loss,
   volume loss, sandbox deletion, and region failure are different guarantees.
   Review PVC retention, storage locking/fsync behavior, backups, and restore.
7. **Pin upgrades and test migrations.** Durable is experimental and stored
   formats evolve; taking an old binary back into a newly written store is not
   a safe rollback strategy without explicit compatibility evidence.

Acceptance tests should kill the process and replace the pod during generation,
file edits, test execution, Code Mode, push, and PR creation; also disconnect
clients after admission, race duplicate submissions, and test repeated crashes
and incompatible upgrades. Check for preserved files, bounded retries, no duplicate
external effects, and truthful terminal status. These deployment checks have not
been performed by this research.

## 5. Verification performed and limits

Executed against the isolated PiG v0.7.0 checkout, not a modified application:

```sh
GOWORK=off go test -mod=mod ./durable/harness \
  -run '^(TestToolRecovery|TestGenerationRecovery)$' -count=1 -timeout=120s
# PASS

GOWORK=off go test -mod=mod ./durable/core/driver \
  -run '^TestOpenHarnessUpgradesAVersionOneStoreInPlace$' -count=1 -timeout=120s
# PASS
```

The first attempt could not resolve packages omitted by the research sparse
checkout. Expanding that checkout to include its dependencies resolved setup;
the harness recovery tests then passed. They include close/reopen recovery;
this run is not a new SIGKILL or Kubernetes durability certification.

Two additional Code Mode restart tests were attempted:
`TestANestedCallThatFailsBecauseTheHostClosedLeavesTheScriptToALaterAttempt`
and `TestARerunWithoutJournalEntriesGoesOnLiveAfterItsFirstRun`. Both could not
run their paired Pi oracle because `.upstream/current` was not provisioned in
the isolated checkout. Their restart implementation was inspected, but these
tests are **not claimed as passing**. Provision the pinned Pi oracle and its
Node dependencies before using them as an integration gate.

No paid model calls, cluster deployment, full upstream parity gate, or
power-loss tests were run. Only this report is added to the application repo.

## Primary sources

### Pi

- **P1:** [Earendil: Pi Durable announcement](https://earendil.com/posts/pi-durable/), especially “Why Pi Durable?”, “Long runs anywhere”, and “Survives crashes”.
- **P2:** [Pi Durable README at PiG's Pi v1.5.0 pin](https://github.com/earendil-works/pi/blob/f9a26365568127461c47a07a37dcb42c140a3ac5/packages/durable/README.md): concepts, persist/resume, tools, Code Mode, storage, failures.
- **P3:** [Pi Durable README at inspected main](https://github.com/earendil-works/pi/blob/ab74d46243e741e20077d244d30d2d673de40b5f/packages/durable/README.md) and [normative specification](https://github.com/earendil-works/pi/blob/ab74d46243e741e20077d244d30d2d673de40b5f/packages/durable/docs/spec.md). Newer APIs must be separated from the pinned dependency.
- **P4:** [Cloudflare's PiHarness integration](https://developers.cloudflare.com/agents/harnesses/pi/): an independent first-party example of separating Pi Durable storage/execution from host wake-up and lifecycle. Not required for Kubernetes.

### PiG v0.7.0 (immutable release source)

- **G1:** [Upstream behavior pin](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/.upstream/pin).
- **G2:** [Go harness implementation](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/durable/harness/harness.go), notably `OpenHarness`, `Resume`, `Submission`, and `Inspect`.
- **G3:** [Core driver opener](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/durable/core/driver/open.go).
- **G4:** [SQLite storage opener](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/durable/storage/sqlite/node/node.go).
- **G5:** [Tool contracts](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/durable/harness_types.go) and [intent/recovery implementation](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/durable/harness/tool.go), especially preparation and `toolExecute`.
- **G6:** [Tool recovery tests](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/durable/harness/harness_tools_recovery_test.go), [generation recovery tests](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/durable/harness/harness_generation_recovery_test.go), and [nested-tool recovery tests](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/durable/harness/tool_nested_restart_test.go).
- **G7:** [Durable Code Mode implementation and replay contract](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/durable/codemode/tool.go) and [restart scenarios](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/durable/codemode/call_restart_scenarios_pi_test.go).
- **G8:** [Native SQLite adapter pragmas](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/chord/sqlite/node/node.go) and [core driver SQLite pragmas](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/durable/core/sqlhost/db.go).
- **G9:** [In-place store upgrade test](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/durable/core/driver/upgrade_in_place_core_test.go).
- **G10:** [Differential conformance and crash-handoff tooling](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/durable/contract/README.md), including documented known failures and pending cases.
- **G11:** [PiG source-port coverage/evidence inventory](https://github.com/MichaelKinsy/PiG/blob/beb56447c476da70d05d8e1e1ca9e993b367cb8b/test/parity/coverage.md).

### This checkout

Local links refer to the inspected application revision above.

- **L1:** [`main.go`](../../main.go), imports and `cli.Main` delegation; [`go.mod`](../../go.mod), PiG version.
- **L2:** [`coding/cli/cli_session_factory.go`](../../vendor/github.com/MichaelKinsy/PiG/coding/cli/cli_session_factory.go), `create`, runtime construction and session creation around lines 130–141.
- **L3:** [`vendor/modules.txt`](../../vendor/modules.txt), only `durable/core/jv` listed from Durable; verified with `go list -mod=vendor -deps .`.
- **L4:** [`internal/jsonstringify/json.go`](../../vendor/github.com/MichaelKinsy/PiG/internal/jsonstringify/json.go), [`internal/jsparity/jsparity.go`](../../vendor/github.com/MichaelKinsy/PiG/internal/jsparity/jsparity.go), and [`ai/openai_chunk_fast.go`](../../vendor/github.com/MichaelKinsy/PiG/ai/openai_chunk_fast.go), JSON-helper use of `jv`.
- **L5:** [`internal/codingagent/session.go`](../../vendor/github.com/MichaelKinsy/PiG/internal/codingagent/session.go), `planRecordWriteLocked` and `appendSessionLine` around lines 469–573; [`session_restore.go`](../../vendor/github.com/MichaelKinsy/PiG/internal/codingagent/session_restore.go), `newSessionFromEntries`.

### Agent Sandbox

- **K1:** [Official Agent Sandbox README](https://github.com/kubernetes-sigs/agent-sandbox#readme), scope, core Sandbox, persistent storage, lifecycle, and runtime isolation. This source is mutable; pin a release and review its storage/lifecycle configuration before implementation.
