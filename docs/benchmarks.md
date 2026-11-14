# CLI performance benchmarks

## CI build chain

`.github/workflows/ci.yml` builds and verifies the four release archives once in `release`.
Linux and macOS `check` jobs download the appropriate archive and call `make smoke-built` instead of rebuilding.
The Linux `benchmark` job waits for `release`, `check`, and `hygiene`, downloads the same artifact, verifies its checksum, and extracts it.
The benchmark job does not install Go or invoke a build command.
Compiling test binaries in `check` remains necessary; those are not additional release executable builds.

The runner installs upstream Pi (`@earendil-works/pi-coding-agent` 1.5.2), OMP (`@oh-my-pi/pi-coding-agent` 18.18.0), Node 24.18.0, and Bun 1.4.4 through `actions/setup-node` and `oven-sh/setup-bun`.
Pi here means the original Node CLI, not the Go PiG port.
OMP and Pi have separate npm manifests and integrity-checked lockfiles under `benchmarks/`.
Separate dependency trees make installed-footprint comparisons meaningful and avoid sharing one baseline's dependencies with another.
No globally installed CLI is used.

Pi uses its maintained package scope because the deprecated `@mariozechner` package has no security-fixed release. OMP retains its CLI pin with a `puppeteer-core` 25.17.0 override: this upgrades Chromium BiDi to a version that supports patched Zod 4 instead of forcing Zod 4 into a Zod 3 consumer. Both dependency trees must pass `npm audit`. The measurements below were retested on 2026-11-12 against these current baselines.

## Workload and assertions

`scripts/benchmark.py` uses a loopback streaming OpenAI-compatible model, not a paid API or real model.
Each implementation receives a fresh temporary HOME, profile, project, and session directory.
An environment allowlist excludes credentials, auth brokers, inherited extension settings, and real user profiles.
The model URL is local; offline settings disable startup network work where supported.
This is configuration isolation, not an OS network sandbox.

The default run has:

- One complete warmup per CLI, excluded from reported medians.
- Five measured repetitions, executed serially in seeded shuffled order.
- 120 sequential prompts in one persisted RPC process, with growing conversation history.
- A fresh process that resumes the same session and completes 12 additional prompts.
- 100 generated Python source files with 256 functions each.
- Alternating actual `read`, `write`, and `bash` calls, followed by streamed 4 KiB assistant responses in 128-character chunks.
- Two model requests per prompt: one tool call and one final answer.
- Unique numbered answer lines so OMP's repeated-content detector does not mistake fixture output for a model loop.

The fixture checks offered tools, read/shell output, written file contents, prior assistant history (including after resume), request counts, and exact completed turn order.
The driver checks RPC errors, failed tools, assistant failures, final answer markers, persisted session existence, and process exit codes.
A failure invalidates the comparison and fails CI; it never becomes a deceptively fast sample.
The driver closes stdin and reaps processes, with process-group cleanup on timeout or failure.
Partial results and stderr logs survive in the uploaded artifact.

The comparison selects the common `read,write,bash` tools and disables extension/context discovery, thinking, automatic compaction and retries.
OMP-specific flags also disable LSP, automatic titles, rules, and PTY shell execution.
These are benchmark settings, not changes to production defaults or removed features.
Different default system prompts and tool implementations remain part of each CLI's cost.
The fixture reports synthetic token usage, not an independent tokenizer measurement.

## Measurements and interpretation

`summary.md` contains medians across complete measured runs.
The p95 column is the median of each run's nearest-rank turn p95, not a pooled percentile.
`results.json` preserves each run's counters, turn latencies, first-text latencies, turn-boundary RSS, sampled resource timeline, execution order, versions, executable hashes, dependency-lock hashes, and machine metadata.
CI appends the summary to the job summary and retains the JSON, Markdown, and stderr logs for 30 days.

Measurements include:

| Area | Metrics |
|---|---|
| Startup | Launch to successful `get_state`, both fresh and resumed |
| Speed | Turn median/p95, first assistant text including the tool round-trip, turns/second, total wall time |
| CPU | Wait-accounted user/system CPU, average percent of one core, sampled cumulative live-tree CPU, idle CPU percent |
| Memory | Sampled peak process-tree RSS, per-turn RSS, first-to-last turn RSS growth, settled idle RSS and idle change |
| OS resources | Sampled peak threads and file descriptors, context switches, page faults, sampled physical disk read/write bytes |
| Persistence | Total session JSONL bytes after the long session and resumed session |
| Distribution | Entrypoint bytes and deterministic gzip bytes; installed dependency payload raw plus gzip; interpreter executable raw plus gzip; combined totals |

Sampling uses Linux `/proc` every 20 ms and includes descendants from every thread's children list.
The Python fixture, sampler, and report generator are outside the measured tree.
RSS sums each process's resident pages, including shared pages counted per process; it is not PSS, managed heap size, or a leak proof.
Short-lived children and transient peaks can fall between samples.
CPU totals use `RUSAGE_CHILDREN` deltas after reaping; descendant CPU is included when descendants are reaped by their parent.
Sampled live-tree CPU can decrease as children exit and is diagnostic, not a replacement for those totals.
Disk counters are physical kernel I/O and may be zero with warm page caches; they are not logical bytes processed.
The idle interval is 250 ms, without forced garbage collection.

The entrypoints are not equivalent standalone binaries: Pi's tiny launcher needs Node and its dependency tree; OMP needs Bun and its dependencies.
Installed payload excludes the interpreter, which has its own raw and gzip columns, and does not attempt to charge shared system libraries to a CLI.
Totals add entrypoint, installed payload, and interpreter, so they answer "what does this CLI cost on disk and over the wire" rather than "how large is the file users start".
gzip figures are one level-6 stream over sorted files, which approximates a downloaded archive; they are not the same bytes as a release tarball.
Results describe a synthetic headless coding session, not model quality, provider latency, interactive terminal repainting, extension/subagent workloads, or cold-cache performance.
The existing startup-input check and codemode/subagent/web-tool smoke tests remain separate feature gates.
Shared GitHub-hosted runners are noisy; comparisons do not enforce absolute timing thresholds or claim statistical significance from five samples.

## Run locally

Use Linux with Node and Bun on PATH.
Install the baselines and build once:

```sh
npm ci --prefix benchmarks/omp
npm ci --prefix benchmarks/pi
# Bun 1.4.4 must be on PATH for OMP; Node is used for Pi.
make build
make benchmark
```

Reuse a release executable without building:

```sh
python3 scripts/benchmark.py --slim /path/to/ompig-slim --runs 5 --turns 120
```

For a longer stress run:

```sh
make benchmark ARGS='--turns 1000 --resume-turns 100 --runs 7 --output benchmark-results/stress'
```

For a quick infrastructure check:

```sh
python3 -m unittest discover -s scripts -p 'test_benchmark.py'
make benchmark ARGS='--runs 1 --warmups 0 --turns 3 --resume-turns 3 --answer-bytes 256'
```

`--only ompig-slim` does not require installed baseline packages.
Use `--help` for event deadlines, sampling interval, answer size, and output paths.
Changing workload sizes changes the question being measured; compare identical settings.

## Binary and footprint comparison

Measured on the same Linux arm64 host with the harness above.
Raw bytes sum regular files, skip symlinks, and deduplicate resolved paths.
gzip is a single level-6 stream over the same sorted files; it approximates a downloaded archive but is not byte-identical to a release tarball.
"Runtime" is the interpreter executable the launcher needs (`bun` or `node`).
Shared system libraries are excluded for every CLI.

| CLI | Entrypoint B | Entrypoint gzip B | Dependencies B | Dependencies gzip B | Runtime B | Runtime gzip B | Total B | Total gzip B |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| ompig-slim | 74,776,839 | 32,553,561 | 0 | 0 | 0 | 0 | 74,776,839 | 32,553,561 |
| OMP 18.18.0 | 18,601,469 | 6,404,768 | 296,266,075 | 85,467,211 | 107,339,768 | 51,644,283 | 422,207,312 | 143,516,262 |
| Pi 1.5.2 | 169 | 148 | 115,934,602 | 24,435,944 | 121,723,800 | 43,845,646 | 237,658,571 | 68,281,738 |

Reading the table:

- `ompig-slim` is one self-contained native executable: no interpreter, no dependency tree, nothing to resolve at startup. Nothing is downloaded at run time.
- OMP's disk cost is dominated by its platform native addon (`pi_natives.linux-arm64.node`) and Bun itself. Pi's is dominated by its JavaScript dependency tree and Node.
- Compared as total installed bytes, `ompig-slim` is 17.7% of OMP's footprint and 31.4% of Pi's.
- Compared as compressed download bytes, it is 22.7% of OMP's and 47.7% of Pi's.
- The 74.7 MB native binary is larger than OMP's 18.6 MB JavaScript bundle and Pi's 169-byte launcher. A fair single-file comparison would only be against another self-contained binary; neither baseline ships one.
- The interpreter numbers are the exact Node and Bun executables on this host and are not charged to a CLI a second time elsewhere.
- Two other views are worth stating explicitly: total installed bytes is 5.7x smaller than OMP and 3.2x smaller than Pi; compressed download bytes is 4.4x smaller than OMP and 2.1x smaller than Pi.
- The compressed gap against Pi is narrow because a stripped Go binary, dominated by machine code, type metadata, and pclntab, compresses less than JavaScript text does. Raw disk footprint is where the native build wins clearly.

Historical pre-optimization, locally cross-compiled `linux/amd64` release artifact: `ompig-slim-linux-x64.tar.gz` is 35,092,196 bytes, containing an 80,150,688-byte binary plus licenses.
The optimized arm64 development build above is 74,711,303 bytes; architecture, source revision and linker inputs differ, so compare matching release archives to release archives.

Historical pre-optimization contents of the stripped arm64 executable (section/compression experiments were not rerun for this optimization):

| Section | Bytes |
|---|---:|
| `.text` | 26,860,660 |
| `.gopclntab` | 21,178,557 |
| `.rodata` | 16,041,945 |
| `.go.type` | 6,055,888 |
| `.noptrdata` | 3,027,905 |
| `.data` | 1,406,210 |

The 34.1 MB FIPS DRBG scratch buffer is `.noptrbss` and is virtual address space, not file bytes.
Unstripped builds are 100,969,552 bytes; the existing `-s -w` flags already remove the symbol table and DWARF data.
A further `strip -s` recovered only 17,111 bytes, so external stripping would add a toolchain dependency for no meaningful gain.
xz reaches 24,690,828 bytes and zstd-19 reaches 27,396,204 bytes on this binary; release archives stay on gzip because it is universally available and already verified.
Largest embedded payloads are the codemode/QuickJS runtime, syntax-highlighting data, and the Lisp extension builder.

`benchmarks/evidence/footprint.json` records these numbers with the command that produced them.
The distribution columns and the 120-turn timing numbers above come from one build measured on 2026-11-12.

## Go 1.27.4 rebenchmark

The project now builds with Go 1.27.4, the newest stable Go toolchain at the
latest retest. Paired five-run local workload measurements built the same source
with Go 1.27.3 and Go 1.27.4. The results showed no meaningful patch-level gain:
long-session turn median was 21.3 → 21.3 ms, peak RSS 176.9 → 173.8 MiB, and
CPU 1.51 → 1.52 s. The canonical JSON microbenchmark was 223.1 → 223.9 ns for
a short frame, 4,866 → 4,858 ns for a source file, and 674,186 → 670,097 ns
for large history. These are noisy same-host observations, not evidence of a
regression in Go itself. Full provenance and commands are in
[`go1.27.4-rebenchmark.md`](../benchmarks/evidence/go1.27.4-rebenchmark.md).

## Model catalog and browser paths

[Model catalog performance](../benchmarks/MODEL-CATALOG.md) records the
profile-driven optimization of full-catalog composition, compatibility copies,
lookup and browser ranking. It includes interleaved before/after microbenchmarks,
allocation/CPU attribution, whole-process startup checks, correctness oracles
and raw evidence. Those results do not imply faster inference or lower RSS.

## Session memory trace

`PIG_PROFILE=heap,allocs,metrics` writes whole-process profiles on exit. The harness now exposes this through `--profile`, with a separate directory per process, and refuses mixed-CLI profiling comparisons. Profiling is opt-in: ordinary timing runs remain uninstrumented.
Two 200-turn local sessions per binary, medians reported on 2026-11-12. Before is the pre-optimization 0.1.5 build; after includes the local-mode changes below. `benchmarks/evidence/session-allocations.json` retains each run.

| Metric | Before | After | Change |
|---|---:|---:|---:|
| Allocated bytes | 2,035,209,252 | 342,866,792 | -83.2% |
| GC cycles | 17 | 15 | -11.8% |
| Mapped runtime bytes | 310,982,984 | 66,291,992 | -78.7% |
| Live heap at exit | 49,834,728 | 8,866,632 | -82.2% |
| Peak RSS from metrics | 353,699,840 | 105,918,464 | -70.1% |

The original single-text-block optimization in `convertCompletionsMessages` remains intact. The new reduction removes history projection and JSON decoding from the loop guard's ordinary no-draft turn, and avoids serializing unrelated messages to find a todo snapshot. A no-todo-history regression test asserts zero allocations across 600 assistant entries. The transcript and provider conversion memo still retain history; these figures are not a leak proof.

## Local-mode optimization

All CLI comparisons here measure isolated local mode, even on a host configured for a remote broker. The allowlisted environment excludes `OMPIG_BROKER_URL` and `OMPIG_BROKER_TOKEN`; the temporary HOME/profile has no saved broker configuration. `--offline` also bypasses broker startup. No real profile, credential or broker setting is modified. Broker HTTP/RTT benchmarks below are a separate workload.

Five measured runs after one warmup per binary, each 120 turns plus a fresh 12-turn resume, serial on the same Linux arm64 host:

| Metric | Before optimization | After optimization |
|---|---:|---:|
| Ready, long session | 582.8 ms | 121.3 ms |
| Turn median, long session | 53.7 ms | 20.6 ms |
| Turn median, resumed session | 191.8 ms | 32.5 ms |
| Peak RSS, long session | 570.4 MiB | 177.7 MiB |
| CPU, long session | 4.61 s | 1.48 s |
| CPU, resumed session | 2.74 s | 0.37 s |

`benchmarks/evidence/local-optimization.json` retains both binaries' hashes, toolchain/runtime provenance, per-run counters and turn arrays, plus the raw process allocation metrics behind the table below. Timelines are omitted. The optimized full comparison also reruns OMP and Pi, rather than combining old baseline timings with new native timings.

The changes remove work, not features:

- `builtinTypedBase` installs lazy provider callbacks using embedded catalog ownership, without decoding every provider's rows. `ResolveCliModel` uses the existing exact `GetModel` path for an explicit provider/id; misses, suffixes and fuzzy selection still use the full resolver. Saved defaults/roles also use exact lookup. Provider overlays, stripping and virtual models stay authoritative. Cost: the full catalog is still decoded when genuinely requested; no new generated index or stale cache.
- `RequestAuthRuntime` composes auth without decoding model lists until requested; dynamic Radius rows are still snapshotted during composition. Available-model listing checks auth before loading a plain provider catalog. A real local PTY with a saved default (no explicit `--model`) becomes editable in 597 → 135 ms and initializes in 712 → 247 ms, with one warmup and five fresh measured profiles per binary. Cost: catalog work is deferred to actual list/menu use, not removed.
- The loop guard requests `OnTurnEndSummary`, whose context is explicitly empty. The runner skips projection/decoding when it has no drafts; actual drafts still run validation/repair, and ordinary Go/JSON handlers receive full previews in snapshot order. The guard itself, goals and agent-state remain enabled. Cost: a small explicit host registration contract rather than guessing which fields a callback reads.
- `phasesOf` rejects decoded non-todo messages before serialization. The reminder still reads the authoritative latest successful non-view todo snapshot, with no cached state to go stale after rewind/resume. Cost: none beyond a type/role check.
- `GetContextBranch` retains entries decoded for the current context, as `BuildSessionProjection` already does. Resumed prompts stop reparsing a cold prefix on every selection/usage check. Cost: resident entries bounded by active context, not a new unbounded cache; leaf moves/compaction preserve existing eviction and unrelated branches stay file-backed.

Regression coverage pins exact-model resolution against the full resolver, snapshot ordering and unsubscribe behavior, draft validation/repair, normal-handler previews, unchanged retained JSON records, and repeat context reads with no further disk decoding. `make check`, targeted race tests and `make smoke-built` pass. There are no absolute timing thresholds.

Reproduce with the existing harness (profiling affects timing, so keep those runs separate):

```sh
python3 scripts/benchmark.py --slim ./bin/ompig-slim --runs 5 --warmups 1 --turns 120 --resume-turns 12
python3 scripts/benchmark.py --slim ./bin/ompig-slim --only ompig-slim \
  --runs 2 --warmups 0 --turns 200 --resume-turns 1 \
  --profile heap allocs metrics --output benchmark-results/allocations
```

Not applied: new catalog wire formats and broader fuzzy/default-selection indexes (*hypothesis*); exact selection no longer needs them, and they add generated-data/compatibility cost. Longer broker credential/quota caching remains deliberately out of scope because it weakens live sibling logout/block visibility. Results measure synthetic harness overhead, not provider latency or universal performance.

### Resume-to-work latency

`scripts/measure-resume.py` measures a real terminal opening an old session,
rendering an edit, and sending its first continuation to a loopback model that
checks every saved turn marker. `--via-picker` instead measures `/resume` in
an already-running TUI, including listing and automated selection. Profiles,
credentials and broker settings are isolated through the benchmark allowlist.
The PTY reader searches a bounded tail so draining long regular-mode scrollback
does not itself throttle the binary.

Resume construction uses the cached immutable projection for message-existence
checks instead of decoding and cloning the whole context just to count it.
`BuildSessionProjectionView` retains decoded active-context entries under the
session mutex, as `GetContextBranch` does, so constructor checks, restored
settings, agent hydration and the transcript share one read. Projection uses
the retained entries, not temporary disk-decode buffers. No history is hidden,
no model context is deferred past submission, and leaf moves/compaction still
evict unrelated entries. Cost: the already-needed active context becomes
resident earlier; the projection and retained entries share its messages.

A 1,000-turn, 4,000-message fixture on a contended Linux arm64 host measured
fullscreen startup-to-request **2,731 → 1,779 ms**, and in-app picker-to-request
**2,834 → 1,682 ms** (medians of three interleaved runs). Regular-mode medians
were **2,715 → 2,326 ms** and **2,355 → 1,735 ms** respectively; regular mode
still emits the full transcript into native terminal scrollback. These are
harness timings, not a latency guarantee for another terminal or provider.
Raw runs, provenance and timing boundaries are retained in
[`session-resume-readiness.json`](../benchmarks/evidence/session-resume-readiness.json).
`TestResumeReadsContextOnce` fails on duplicate cold-prefix decoding and also
checks that the UI's subsequent context read does no additional disk decoding.

```sh
python3 scripts/measure-resume.py ./bin/ompig-slim --turns 1000 --runs 3
python3 scripts/measure-resume.py ./bin/ompig-slim --turns 1000 --runs 3 --via-picker
# Add --mode regular for native scrollback; --max-ms bounds resume-to-request.
```

A separate **compacted** workload is essential: the earlier uncompacted test
did not catch reads of history outside the model's kept range. Add
`--kept-turns 12` to compact the fixture while checking exactly those retained
turns in the first continuation request. Two interleaved runs per variant on a
3,000-turn, roughly 19.7 MB log measured:

| Resume-to-request | Before (6add1d6) | After filtered history reads |
|---|---:|---:|
| Fullscreen startup | 3,127 ms | 693 ms |
| Fullscreen `/resume` picker | 2,748 ms | 530 ms |
| Regular startup | 2,880 ms | 666 ms |
| Regular `/resume` picker | 2,726 ms | 519 ms |

Cache notices previously forced both a full branch decode and another full
file decode, despite needing only assistant usage, cache-warming records and
summary boundaries. They now filter the slot index before reading bodies,
preserving file order across forks and resets. Transcript hydration reads the
context branch even when no first-kept entry exists; its compaction trim still
shows only the summary in that case. Subagent restoration also uses the context
branch, which retains every state-only entry, including custom records from
before compaction, without decoding discarded conversation messages. Neither
feature is disabled, and no new cache or session format is introduced.

`TestCompactedResumeLeavesToolHistoryCold` guards proper, empty, self-referencing
and missing kept-entry boundaries. `TestCacheNoticeIndexPreservesFileOrder`
compares notices against the original full-history scan, including orphan
responses, cache warming and both summary resets. `TestResumeReadsContextOnce`
also verifies compacted constructor reads and pre-compaction subagent records.
Raw runs and the separate tool-heavy component benchmark are retained in
[`session-resume-compacted.json`](../benchmarks/evidence/session-resume-compacted.json).
The picker fixture contains one saved session, so these results do not measure
listing hundreds of unrelated logs before a session is selected.

Not changed: terminal scrollback replay and initial parsing of the necessary
active context. Fullscreen avoids the regular terminal's full-scrollback write,
but the optimization does not override a user's chosen TUI mode.

## Remote client round-trip latency

`internal/broker/remote_latency_test.go` exercises real authenticated HTTP and
SQLite through a loopback handler that adds one cancellable delay per request.
A 50 ms delay models 50 ms total RTT, not 50 ms in each direction. Independent
requests overlap. Fixtures use temporary profiles and fake API keys; no paid
provider or real credential is needed.

The comparison below ran on Linux arm64, Go 1.27.3, four reported CPUs. Each
entry is the benchmark mean of five operations, not a percentile or confidence
interval. Before uses broker client sources identical to `33e540a`; after applies this branch's
preference concurrency and quota-read invalidation fixes. Both use the same test
harness and invocation. Raw output is retained in
`benchmarks/evidence/remote-client-{before,after}.txt`.

| Path | 50 ms RTT before → after (ms) | 150 ms RTT before → after (ms) | Requests/op before → after |
| --- | ---: | ---: | ---: |
| Empty-profile preference startup | 284.2 → 60.8 | 781.1 → 162.0 | 5 → 5 |
| Idle preference sync / exit-flush path | 264.1 → 52.0 | 753.8 → 153.9 | 5 → 5 |
| Push one changed preference | 319.7 → 109.8 | 912.5 → 311.8 | 6 → 6 |
| Pull one changed preference | 264.2 → 60.9 | 760.7 → 161.0 | 5 → 5 |
| Service readiness | 385.0 → 169.3 | 1077.0 → 468.6 | 7 → 7 |
| Two auth/quota passes | 204.1 → 154.0 | 604.9 → 453.1 | 4 → 3 |
| Select an account | 55.1 → 55.4 | 154.7 → 155.1 | 1 → 1 |
| Log out one account | 54.4 → 55.4 | 155.2 → 155.7 | 1 → 1 |

At zero injected RTT, startup measured 19.8 → 8.7 ms and service readiness
45.0 → 15.3 ms; these small samples include filesystem/scheduler noise. WAN
results are the target, with the critical path and request counts explaining
why the latency moves:

- `Client.SyncFiles`: five serial document GETs → five concurrent GETs, under
  the same local/process lock. Startup and idle polling pay roughly one RTT,
  not five; a push still requires GET then CAS PUT. Cost: up to five concurrent
  HTTP exchanges/goroutines and simultaneous document buffers, bounded by the
  fixed allowlist and existing per-response limits. No ordering between different
  documents was transactional before or after this change.
- `remoteStore.GetAccountBlockedUntil`: mutation helper → direct read-only
  request. Each quota read stays live but no longer evicts unchanged credentials;
  two auth/quota passes fetch one snapshot instead of two. Cost: none beyond the
  existing one-second credential freshness bound. Successful mutations still
  invalidate the view, expired views never mask fetch failures, and metadata is
  not cached.

Readiness includes preference sync, session-service/registry construction,
OpenAI model availability and credential resolution. It does **not** measure a
rendered terminal, extensions, model first-token latency, DNS/TLS handshakes,
packet loss, bandwidth limits or actual OAuth-provider latency. Startup here
means the broker preference stage on an empty profile, not executable spawn.
Selection/logout deliberately retain one live durable round trip: no speedup
is claimed. All credential samples start cold; the two-pass auth/quota path then
uses the existing TTL. The same idle sync code runs on exit, but process shutdown
itself is not timed.

Regression tests assert overlapping requests and snapshot counts rather than
fragile wall-clock thresholds. Delayed safety cases also cover sibling block/
clear visibility, selection/logout invalidation, TTL expiry, unavailable-host
refusal, adoption backups, conflicts, independent progress, deletion and canceled
sync recovery. The overlap and snapshot regressions were run red against the
pre-fix sources and green after restoring the fix.

Run the comparison workload and regressions:

```sh
go test -mod=vendor ./internal/broker -run '^TestRemoteLatency' -v
go test -mod=vendor ./internal/broker -run '^$' \
  -bench '^BenchmarkRemoteLatency$' -benchtime=5x -count=1
```

For a new before/after comparison, run the same benchmark test file in both
checkouts, using the same host, Go version and command. The test file is
self-contained apart from existing broker test fixtures and works against the
pre-fix sources. Prefer larger/repeated samples for small loopback differences.

Not applied: batching live routing metadata/quota into a new request is a
*hypothesis* for reducing the remaining readiness/auth round trips; it needs an
explicit request-scoped consistency design. Longer-lived credential or quota
caching was not applied because it would weaken sibling logout/block visibility.
The landing review found no additional measured optimization worth that tradeoff.

## Measured CPU optimization

Local evidence is retained under `benchmarks/evidence/`.
The host was Linux arm64 with four reported CPUs, Go 1.27.3, Node 24.18.0, and Bun 1.4.4.
These measurements are not results from the GitHub x64 runner.

A CPU profile of `BenchmarkCanonicalSession` attributed approximately 85% of sampled CPU to the canonical JSON scanner's string-token scan.
That scanner checked source-text bytes one at a time even though the serializer already had a word-at-a-time `PlainEnd` scan.
The optimization shares that scan through string and byte-slice forms, without unsafe memory access, per-token copies, new pools, retained buffers, or changes to JSON semantics.
UTF-8 validity, escape spelling, surrogate-pair handling, member ordering, and the full-rewrite fallback remain unchanged.
An initially tested string conversion allocated; it was rejected in favor of a shared generic scan with a byte-slice entry point.

Medians of five `-benchmem` samples:

| Canonical payload | Before ns/op | After ns/op | Speedup | Allocations before/after |
|---|---:|---:|---:|---:|
| Short frame | 412.6 | 223.5 | 1.85x | 0 / 0 |
| Source file | 15,608 | 4,863 | 3.21x | 0 / 0 |
| Large history | 2,712,432 | 671,374 | 4.04x | 0 / 0 |

Both versions also report zero allocated bytes per operation with a reused destination.
`canonical-before.txt` retains the original pre-change samples; `canonical-after.txt` was regenerated on 2026-11-12 against this build.
The tests compare both scan forms with an independent scalar oracle and compare fast JSON canonicalization against the full rewrite, including Unicode, controls, escapes, surrogate pairs, and boundary offsets.
A bounded fuzz run exercised over 323,000 inputs without a mismatch.

Five-run, 120-turn full-session medians, retested on 2026-11-12 against the current build:

| CLI | Ready ms | Turn ms | Peak RSS MiB | User + system CPU seconds |
|---|---:|---:|---:|---:|
| ompig-slim | 121.3 | 20.6 | 177.7 | 1.48 |
| OMP | 2055.7 | 41.4 | 502.1 | 9.20 |
| Pi | 1135.1 | 35.8 | 439.4 | 6.30 |

These are the same optimized local-mode runs reported in [the CLI comparison](../benchmarks/COMPARISON.md).
The scanner microbenchmark above is a separate historical retest, not attribution for the full-session improvements; those come from the [local-mode optimization](#local-mode-optimization).
`local-metrics.json` retains per-run metrics, versions, executable hashes, and the run order for these observations; it omits the large sample timelines, which CI retains in its artifacts.

To reproduce the scanner profiles without rebuilding the application:

```sh
go test -mod=vendor ./vendor/github.com/MichaelKinsy/PiG/internal/jsonstringify \
  -run '^$' -bench BenchmarkCanonicalSession -benchmem -count=5 \
  -cpuprofile=/tmp/canonical.cpu -memprofile=/tmp/canonical.mem -o /tmp/canonical.test
go tool pprof -top /tmp/canonical.cpu
go tool pprof -alloc_space -top /tmp/canonical.mem
```
