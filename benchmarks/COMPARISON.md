# Benchmark comparison

This report summarizes the checked-in measurements in [`evidence/`](evidence/).
For the full harness definition, limitations, and reproduction commands, see
[`docs/benchmarks.md`](../docs/benchmarks.md).

## CLI comparison

The CLI results use a synthetic loopback OpenAI-compatible model, not a real
model or provider. The workload is the same for each implementation: a
120-turn persisted session followed by a fresh process resuming that session
for 12 more turns. Values below are medians of five measured runs after one
warmup on Linux arm64 with four reported CPUs. Timing and memory are workload-
and host-specific, not universal performance guarantees.

### Long session

| CLI | Ready (ms) | Turn median (ms) | Turn p95 (ms) | Turns/s | Peak RSS (MiB) | CPU (user + system, s) |
|---|---:|---:|---:|---:|---:|---:|
| ompig-slim 0.1.5 + local optimizations | 121.3 | 20.6 | 30.9 | 47.36 | 177.7 | 1.48 |
| OMP 18.18.0 | 2055.7 | 41.4 | 59.9 | 19.62 | 502.1 | 9.20 |
| Pi 1.5.2 | 1135.1 | 35.8 | 59.4 | 25.97 | 439.4 | 6.30 |

### Resumed session

| CLI | Ready (ms) | Turn median (ms) | Turn p95 (ms) | Turns/s | Peak RSS (MiB) | CPU (user + system, s) |
|---|---:|---:|---:|---:|---:|---:|
| ompig-slim 0.1.5 + local optimizations | 200.9 | 32.5 | 36.2 | 30.49 | 81.0 | 0.37 |
| OMP 18.18.0 | 2443.2 | 71.1 | 243.9 | 11.11 | 452.0 | 4.75 |
| Pi 1.5.2 | 1136.4 | 64.0 | 226.9 | 12.97 | 283.5 | 2.52 |

These 2026-11-12 measurements use isolated local profiles, not the machine's
configured remote broker. Fresh HOME/profile directories and an environment
allowlist exclude saved broker configuration, broker URL/token and credentials;
only the loopback model is contacted. All native features remain enabled.

In this workload the optimized build has the lowest ready time, turn latency,
CPU and sampled peak RSS in both modes. Long-session turns are about 2.0x faster
than OMP and 1.8x faster than Pi; resumed turns are about 2.1x and 2.0x faster.

A five-run local before/after comparison, both sides built from the same base,
measures ready time 582.8 → 121.3 ms, long turns 53.7 → 20.6 ms, resumed turns
191.8 → 32.5 ms, and long peak RSS 570.4 → 177.7 MiB. The changes remove eager
unrelated catalog decoding, full-history loop-guard previews, todo serialization
of irrelevant answers, and repeated resumed-context decoding. See
[`docs/benchmarks.md`](../docs/benchmarks.md#local-mode-optimization) for contracts
and trade-offs, and [`local-optimization.json`](evidence/local-optimization.json)
for both sides' runs and allocation counters.

### Installed footprint

The footprint measurement counts the entrypoint, installed dependencies, and
required interpreter; it excludes shared system libraries. Compressed sizes
are a single gzip level-6 stream over sorted files, not release archive sizes.

| CLI | Entrypoint (MB) | Dependencies (MB) | Runtime (MB) | Total installed (MB) | Total gzip (MB) |
|---|---:|---:|---:|---:|---:|
| ompig-slim | 74.8 | 0 | 0 | 74.8 | 32.6 |
| OMP 18.18.0 | 18.6 | 296.3 | 107.3 | 422.2 | 143.5 |
| Pi 1.5.2 | 0.0002 | 115.9 | 121.7 | 237.7 | 68.3 |

On this host, the self-contained ompig-slim executable uses about 5.7x less
installed space than OMP and 3.2x less than Pi. Its compressed total is about
4.4x smaller than OMP's and 2.1x smaller than Pi's. Entrypoint sizes alone are
not an equivalent distribution comparison: OMP and Pi also require their
runtime and dependency trees.

## Go-side optimizations

The remaining evidence concerns internal paths, not additional OMP/Pi comparisons.
The scanner table is a historical optimization retest; the allocation table
compares the pre-optimization 0.1.5 build with the current optimized build.

### Canonical JSON scan

Five `-benchmem` samples of the canonical-session benchmark are retained in
[`canonical-before.txt`](evidence/canonical-before.txt) and
[`canonical-after.txt`](evidence/canonical-after.txt). The table reports the
median sample for each case.

| Payload | Before (ns/op) | After (ns/op) | Speedup | Allocations/op (before → after) |
|---|---:|---:|---:|---:|
| Short frame | 412.6 | 223.5 | 1.85x | 0 → 0 |
| Source file | 15,608 | 4,863 | 3.21x | 0 → 0 |
| Large history | 2,712,432 | 671,374 | 4.04x | 0 → 0 |

This demonstrates a substantial improvement in the targeted canonical JSON
scanner, especially for larger payloads. It is a microbenchmark result, not
proof that the change alone improved whole-CLI performance.

### Session allocation profile

Two runs per binary over 200 turns in one persisted local RPC session, with
`--profile heap allocs metrics`. Values are medians from
[`session-allocations.json`](evidence/session-allocations.json); before is the
pre-optimization 0.1.5 build, after includes the local-mode optimizations.

| Metric | Before | After | Change |
|---|---:|---:|---:|
| Allocated bytes | 2,035,209,252 | 342,866,792 | -83.2% |
| GC cycles | 17 | 15 | -11.8% |
| Mapped bytes | 310,982,984 | 66,291,992 | -78.7% |
| Live heap at exit | 49,834,728 | 8,866,632 | -82.2% |
| Peak RSS | 353,699,840 | 105,918,464 | -70.1% |

The loop guard now explicitly requests a turn summary, so the host need not
project/decode the entire transcript for it. Ordinary extensions keep their
full-context contract. The todo reminder skips non-todo decoded messages before
serializing them, and resumed active-context reads retain decoded entries rather
than repeatedly reading and parsing the same file-backed prefix. No credentials
or remote-broker responses are cached by these changes.

## Evidence and caveats

- [`local-metrics.json`](evidence/local-metrics.json) retains the per-run CLI
  metrics, versions, executable hashes, and benchmark parameters from the
  120-turn comparison above.
- [`footprint.json`](evidence/footprint.json) records footprint counts, binary
  sections, compression alternatives, and the command that produced them.
  Distribution sizes describe the optimized build; section/compression and x64
  archive experiments are explicitly labeled historical pre-optimization data.
- [`docs/benchmarks.md`](../docs/benchmarks.md) describes the workload,
  measurement definitions, limitations, and commands to reproduce the runs.
- Results cover a synthetic headless workload. They do not compare model
  quality, provider latency, interactive terminal behavior, or every feature
  and extension workload.
