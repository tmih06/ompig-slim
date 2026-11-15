# Durable headless workdesks

`ompig-slim durable` is an opt-in headless host for PiG Durable. Running
`ompig-slim` normally still uses the daily coding agent, with the same TUI,
settings, extensions, tools, RPC interface, and session files as before.

## Commands

```sh
ompig-slim durable run \
  --state /data/workdesk \
  --cwd /work/repo \
  --model anthropic/claude-sonnet-4-5 \
  --request-id ticket-123 \
  'Implement the ticket, run its checks, and review the diff'

ompig-slim durable resume --state /data/workdesk
ompig-slim durable status --state /data/workdesk
ompig-slim durable run --state /data/workdesk --cwd /work/repo \
  --model provider/model --request-id ticket-123 --events-file /data/events.jsonl \
  'Implement the ticket'
ompig-slim durable --help
```

Choose an exact configured `provider/model` identity. Credentials and provider
configuration use the existing profile, including configured broker-backed
credentials. No credentials are copied into the workdesk. Project settings,
model roles, skills, MCP servers, ordinary coding-agent extensions, Code Mode,
and interactive confirmations are **not automatically loaded** in this mode.
The initial instruction tells the agent to inspect repository instructions,
implement the objective, verify it, inspect its diff, and report blockers.
Its native Durable tools are `read`, `write`, `edit`, and `bash`; they can perform
multiple tool rounds without another human prompt. The sandbox supplies any
language runtimes, Git identity, dependencies, and network permissions needed.

A state directory belongs to **one immutable assignment**: request ID, prompt,
absolute working directory, model, and turn budget. Repeating the same `run`
returns the existing submission; a changed assignment is refused. Use a new
state directory for another ticket or an intentional new attempt. `resume`
uses the original assignment and requires no repeated prompt or model flags.
`--max-turns N` on the first `run` bounds tool rounds (default 100). Resume does
not reset this budget or automatically retry a terminal failure. PiG's default
model retry policy and restart limit bound repeated crashes; a terminal
`unanswered` result needs an operator/factory decision, not endless restarts.

Commands write one JSON submission receipt to stdout, for example. The
optional `--events-file` writes committed text, tool-call, and tool-result
records as JSONL while the conversation advances. The optional
`--control-file` reads JSONL objects such as
`{"content":"Please also inspect the failing test"}` and admits them as
follow-up Durable inputs while the run is active. These are adapter streams,
not a second Durable journal; the SQLite/WAL state remains authoritative.


```json
{"request_id":"ticket-123","status":"done","answer":"Implemented ..."}
```

`status` reads committed records without starting a scheduler or resolving model
credentials. It may briefly wait for a SQLite transaction; it is not a live
stream. Before admission it reports `pending`; an admitted input may be `placed`
or `queued`. Terminal statuses are `done` and `unanswered`, with `reason` for a
failure. An execution command exits nonzero on an error or `unanswered`; `status`
returns zero when inspection succeeds, including when inspecting failed work.
Diagnostics go to stderr in the executable. SIGINT/SIGTERM stop the current host
and leave pending work recoverable rather than aborting the assignment.

**`done` means the agent produced an answer, not that the ticket is proven
correct, tests passed, a PR exists, or review is approved.** The answer may report
a blocker. Verification and factory acceptance policy are caller-owned. This is
not a ticket queue, GitHub workflow controller, approval service, or a daemon
that restarts itself.

## Recovery contract

- `job.json` is an atomic, fsynced admission intent. It is written before an input
  can execute. A crash before SQLite admission is bridged by replaying this intent
  with the same Durable `requestId`; a response lost after admission likewise
  does not admit the objective twice.
- `session.sqlite` and its WAL are PiG's canonical Durable store. Conversation
  entries, tool intents/results, task checkpoints, and retry state belong to
  PiG, not a second application execution journal. A host reopens this storage,
  reinstalls the same registry/environment, and submitting the original request
  resumes scheduling. The implementation uses `durable/harness.OpenHarness`.
- Interrupted model calls count as failed attempts and may be resent under the
  retry policy. A committed partial is an aborted attempt, not an exact-token
  continuation. Model usage and charges can repeat after interruption.
- Tools follow PiG's replay policies. Unsafe interrupted calls are **not**
  automatically rerun: the next model request receives an interruption result
  saying they may have partially executed. Completed calls remain completed.
  This is not an exactly-once guarantee for commands or external services.
- `owner.lock` is a nonblocking OS advisory lock held for the writer's entire
  lifetime, including shutdown. Another writer fails immediately. The file may
  remain after a crash; the kernel lock, not file existence, determines ownership.
  Never delete/recreate the lock file to bypass an owner.
- State directories are private (`0700`); assignment and database files are
  private (`0600`). SQLite WAL uses `synchronous=FULL` for task persistence;
  durability still depends on the filesystem and storage honoring flushes.
  Deferred streaming progress is not an acknowledged task checkpoint.
- Host assignment format version 1 is checked before resume. PiG also checks and
  migrates its storage schema. Pin images and back up before upgrades: an older
  image may not understand a store written by a newer one.

A SIGKILL does not roll back file edits, Git actions, API calls, or external
jobs. It also does not necessarily terminate shell descendants if only the
harness process dies. Contain the entire process tree in the sandbox; before
recovering from a host-only crash, terminate or reconcile surviving commands.
An unsafe-tool error informs the agent but cannot prevent it from choosing to
issue an equivalent command again. Use external idempotency keys and reconciliation
for push/PR/CI/deploy effects. This mode does not claim distributed fencing.

`status` does not execute tasks, but opening the SQLite adapter performs schema
checks/migrations. Do not use a different binary version as a live viewer during
an upgrade. Model/provider configuration must be restored for execution/resume;
status needs only the state directory.

## Progress checkpoint

Durable coding runs expose a bounded `ompig.progress` conversation document. The
model can replace it through the `update_progress` tool with criteria, completed
work, current step, constraints, decisions, blockers, and the next action. Each
committed revision is tied to the immutable assignment request ID. Completed work
is an agent claim and does not mean that tests passed or that factory acceptance
is complete.

The status receipt includes `progress.present` and the latest committed
checkpoint. A missing document is reported as `present: false`, including for
state created by an older binary. Status only opens the canonical SQLite store.
The checkpoint is committed in the same Durable transaction as the tool result,
so a crash before that commit leaves the previous revision visible. Resumed model
context includes only committed progress, never transient streamed prose. The
checkpoint uses latest-document semantics and caps each list at 16 items, each
text value at 500 characters, and revisions at 1,000,000.

## Progress precedence

The immutable assignment remains binding. The latest committed checkpoint
governs reported progress over conflicting summaries and older updates.
After compaction, PiG rebuilds the prompt sections from current committed state
and omits retained older `pi.system` entries. The next prepared request carries
the current `ompig_progress` section beside the summary. The section states
these rules to the model:

- **The assignment is immutable.** Its request ID binds the checkpoint, and a
  host refuses a checkpoint written by another assignment.
- **The newest committed revision governs reported state.** It replaces the
  revision before it.
- **A generated summary describes history.** It cannot drop a criterion,
  constraint, blocker, or next action the checkpoint commits, and cannot reopen
  work the checkpoint supersedes. The summarizer reads the transcript without
  system messages, so it may never have seen the checkpoint.
- **Only `update_progress` changes the checkpoint.** Prose in an answer, a
  summary, or an older message changes nothing.

A completed-work entry stays an agent claim, and `done` still means the agent
answered. Between compactions, recent context can contain older section updates
and tool arguments. They remain history, not competing checkpoints. Compaction
folds that history and rebuilds one current baseline instead of pinning all
revisions. PiG's recent-context and token-reserve behavior is unchanged.

These are instructions to the model, not a semantic validator for summaries or
progress updates. The 16-item and 500-character caps bound the checkpoint. An
unmet criterion omitted from a later explicit progress update is no longer in
that document. The immutable assignment does not change.

Summary generation and publication still use PiG's compaction task. Its pinned
range and retry state survive a crash. Summary placement and task completion
share one Durable transaction, so a response lost before that commit can be
regenerated without creating another authoritative checkpoint.

## Agent Sandbox deployment

Persist **both** the state directory (entire directory, WAL included) and the
repository (`.git`, untracked files, and uncommitted edits). Use the same absolute
workspace path after restart. Put retained artifacts on persistent storage too;
PiG shell-output spill files in a temporary filesystem are not recoverable
artifacts. Database transactions are not atomic snapshots of arbitrary files or
external systems.

Use one sandbox/process owner per workdesk. Agent Sandbox manages the pod;
configure its container command to `durable resume --state ...` after initial
admission. The orchestrator must restart the process when pending work remains,
and distinguish terminal errors from recoverable infrastructure interruption.
The volume must support SQLite locking/WAL and OS advisory file locks correctly;
not every network filesystem does. Test your chosen CSI/storage class. A PVC is
not a backup: configure retention and SQLite-aware backup/restore separately.
This change does not ship a Kubernetes controller or cluster manifest.

## Maintenance and verification

The additional vendored packages are unmodified production Go source from PiG
v0.7.0, commit `beb56447c476da70d05d8e1e1ca9e993b367cb8b`, selected from the
transitive closure of `durable/harness`, `durable/tools`,
`durable/storage/sqlite/node`, and `durable/env/node`. Existing vendored packages
and personal patches were not replaced. No module version was changed. The
separate core driver, extra storage backends, upstream oracle fixtures, and
Durable Code Mode were not added: they are not used by this host.

```sh
make check
make race
make build
python3 scripts/smoke-test-durable.py ./bin/ompig-slim
go test -mod=vendor ./internal/workdesk -coverprofile=/tmp/workdesk.cover
```

The Go tests use isolated profiles, temporary workspaces, a local OpenAI SSE
fixture, and actual child processes. They cover autonomous edits/checks,
compaction precedence and recovery across a crash, close/reopen and SIGKILL
recovery, unsafe shell interruption, repeated crash limits, writer exclusion,
submission deduplication, cancellation, immutable assignment conflicts, turn
budgets, admission intent before database creation, and private storage. The
compaction tests force automatic compaction with a small-context model and a
stale generated summary, then read the real outgoing request. The compiled-binary
smoke test covers CLI dispatch, SIGKILL/resume, status, admission deduplication,
preserved workspace data, a compaction followed by SIGKILL and resume, and the
ordinary version command. The smoke test is part of `make smoke`.
Kubernetes pod replacement, CSI failure, node/power loss, and remote-service
idempotency must be verified in the deployment environment; local process tests
do not certify those guarantees.
