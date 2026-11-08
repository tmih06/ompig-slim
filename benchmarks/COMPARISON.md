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
| ompig-slim 0.1.0 | 121.3 | 21.0 | 32.9 | 46.42 | 179.3 | 1.46 |
| OMP 18.18.0 | 2,175.5 | 43.6 | 63.7 | 18.64 | 499.4 | 10.05 |
| Pi 0.73.1 | 1,128.7 | 35.9 | 65.2 | 25.64 | 446.3 | 6.15 |

### Resumed session

| CLI | Ready (ms) | Turn median (ms) | Turn p95 (ms) | Turns/s | Peak RSS (MiB) | CPU (user + system, s) |
|---|---:|---:|---:|---:|---:|---:|
| ompig-slim 0.1.0 | 161.2 | 33.6 | 37.9 | 29.38 | 83.1 | 0.35 |
| OMP 18.18.0 | 2,596.7 | 75.9 | 262.2 | 10.55 | 454.6 | 5.11 |
| Pi 0.73.1 | 1,123.9 | 60.3 | 150.1 | 14.81 | 271.4 | 2.27 |

In these runs, ompig-slim had the lowest measured startup time, turn latency,
CPU use, and peak RSS in both session modes. Long-session median turns were
about 2.1x faster than OMP and 1.7x faster than Pi; resumed-session turns were
about 2.3x and 1.8x faster, respectively. The resumed-session p95 for OMP and
Pi was notably higher than ompig-slim's in this workload.

### Installed footprint

The footprint measurement counts the entrypoint, installed dependencies, and
required interpreter; it excludes shared system libraries. Compressed sizes
are a single gzip level-6 stream over sorted files, not release archive sizes.

| CLI | Entrypoint (MB) | Dependencies (MB) | Runtime (MB) | Total installed (MB) | Total gzip (MB) |
|---|---:|---:|---:|---:|---:|
| ompig-slim | 72.6 | 0 | 0 | 72.6 | 32.3 |
| OMP 18.18.0 | 18.6 | 293.0 | 107.3 | 418.9 | 142.7 |
| Pi 0.73.1 | 0.001 | 149.6 | 121.7 | 271.3 | 75.5 |

On this host, the self-contained ompig-slim executable uses about 5.8x less
installed space than OMP and 3.7x less than Pi. Its compressed total is about
4.4x smaller than OMP's and 2.3x smaller than Pi's. Entrypoint sizes alone are
not an equivalent distribution comparison: OMP and Pi also require their
runtime and dependency trees.

## Go-side optimizations

The remaining evidence isolates two changes inside ompig-slim; it should not
be read as additional comparisons against OMP or Pi.

### Canonical JSON scan

Five `-benchmem` samples of the canonical-session benchmark are retained in
[`canonical-before.txt`](evidence/canonical-before.txt) and
[`canonical-after.txt`](evidence/canonical-after.txt). The table reports the
median sample for each case.

| Payload | Before (ns/op) | After (ns/op) | Speedup | Allocations/op (before → after) |
|---|---:|---:|---:|---:|
| Short frame | 412.6 | 255.8 | 1.61x | 0 → 0 |
| Source file | 15,608 | 4,881 | 3.20x | 0 → 0 |
| Large history | 2,712,432 | 674,740 | 4.02x | 0 → 0 |

This demonstrates a substantial improvement in the targeted canonical JSON
scanner, especially for larger payloads. It is a microbenchmark result, not
proof that the change alone improved whole-CLI performance.

### Session allocation profile

The allocation profile compared two runs per binary over 200 turns in one
persisted RPC session. Values are medians from
[`session-allocations.json`](evidence/session-allocations.json).

| Metric | Before | After | Change |
|---|---:|---:|---:|
| Allocated bytes | 508,769,012 | 314,804,852 | -38.1% |
| GC cycles | 18 | 14 | -22.2% |
| Mapped bytes | 80,939,288 | 68,385,032 | -15.5% |
| Live heap at exit | 10,666,800 | 9,620,240 | -9.8% |
| Peak RSS | 120,217,600 | 108,349,440 | -9.9% |

The allocation reduction is the clearest result here. The accompanying
whole-session timing measurements were close enough to be within run-to-run
noise, so they do not support a claim of an end-to-end speed improvement from
this allocation change.

## Evidence and caveats

- [`local-metrics.json`](evidence/local-metrics.json) retains the per-run CLI
  metrics, versions, executable hashes, and benchmark parameters.
- [`footprint.json`](evidence/footprint.json) records footprint counts and the
  separate short command used to collect them. Its entry lists the measured
  date as 2026-11-07; it is not the 120-turn timing workload.
- [`docs/benchmarks.md`](../docs/benchmarks.md) describes the workload,
  measurement definitions, limitations, and commands to reproduce the runs.
- Results cover a synthetic headless workload. They do not compare model
  quality, provider latency, interactive terminal behavior, or every feature
  and extension workload.
