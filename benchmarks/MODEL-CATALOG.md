# Model catalog performance

Measured against `342fa69` on Linux arm64, Go 1.27.3, four reported CPUs,
`GOMAXPROCS=4`. Five interleaved before/after runs, 500ms per benchmark case.
The test binaries were compiled once per revision and run serially, before then
after in each repetition. Raw samples, medians, profile summaries, startup
observations and binary hashes are in [`evidence/model-catalog/`](evidence/model-catalog/).
These are host/workload-specific medians, not statistical or universal claims.

## Measurements

| Path | Before | After | Allocated bytes/op before → after | Allocations/op before → after |
| --- | ---: | ---: | ---: | ---: |
| Full browser scope assignment/sort | 438.08 ms | 29.06 ms | 42,504,768 → 2,640,974 | 811,620 → 16,215 |
| OpenAI browser assignment/sort | 2.48 ms | 0.164 ms | 46,034 → 32,923 | 831 → 130 |
| Full typed catalog listing | 640.17 ms | 114.75 ms | 143,960,144 → 27,225,032 | 1,413,253 → 391,701 |
| Exact composed OpenAI lookup | 145.14 µs | 17.30 µs | 34,248 → 5,696 | 310 → 84 |
| Startup-style runtime model identities | 1.022 ms | 0.652 ms | 625,504 → 594,816 | 2,052 → 972 |
| Full browser alternating search | 62.12 ms | 30.49 ms | 2,954,939 → 2,922,842 | 47,448 → 35,156 |
| OpenAI alternating search | 0.404 ms | 0.168 ms | 12,432 → 22,563 | 323 → 311 |
| Full browser 120×40 rendering | 0.332 ms | 0.328 ms | 85,631 → 85,631 | 1,205 → 1,205 |

The complete browser benchmark uses `ai.ListModels("")`, including retained
native-only providers. The registry benchmark walks every provider through the
actual runtime composition paths. The OpenAI case has the same 61 chat rows in
both binaries. Roles, duplicate role bindings and MRU rankings are enabled;
search alternates `gpt`/`claude`. Decoding and fixture construction occur outside
the timed warmed loops. Profile runs include one-time setup and are attribution,
not the source of the per-operation medians above.

## Attribution and changes

- `tui/model_browser_order.go:rankBrowserItems`: profiles showed repeated regex
  parsing and role/MRU scans inside sort comparisons; `ModelSelectorItem.FQ`
  accounted for about 54% of allocated profile bytes. Precompute row keys once
  per pass, then stable-sort those keys. This reduces the full-list sort to
  6.6% of its prior time and 6.2% of its allocated bytes. No persistent cache
  or stale-role invalidation scheme is introduced. Cost: temporary row-key
  storage; small-provider searches allocate about 10 KB more per query.
- `ai/model_compat_clone.go:cloneScalarCompat`: listing profiles attributed about
  81% of allocated bytes cumulatively to `cloneCompat`, largely JSON round trips
  caused by opaque `Extra` fields. Copy modeled scalars and normalize opaque
  JSON values individually instead of re-encoding every known member. Literal
  `true`/`false`/`null` need only a detached whitespace-trimmed copy. Complex typed
  fields, colliding/non-ASCII names, invalid raw values and serializer-sensitive
  inputs retain the previous whole-object fallback. Cost: guarded fast-path
  code; no shared mutable data and no weakened normalization/error behavior.
- `ai/omp_catalog.go:loadOMPCatalog`: warmed reads first use the existing map's
  installed once-loader instead of reopening an embedded file and allocating a
  losing `OnceValue` closure. Unknown IDs still never enter the cache, and
  concurrent first load/panic semantics remain unchanged. Warm low-level exact
  lookup went from six allocations to zero in the initial five-run baseline.

Render-only allocation counts did not change; no rendering optimization is
claimed. The main gains concern list construction, ranking and model cloning.

## Whole-process check and limitations

The existing `scripts/measure-startup.py` was run against isolated offline HOME,
agent directory and project fixtures, with one loopback model and no prompts.
One warmup per binary, then five interleaved measured runs:

- First editable render: median **621.49 → 611.36 ms**.
- Initialized: median **735.57 → 725.10 ms**.

Those small differences are within observed run-to-run noise. **No whole-CLI
startup improvement is claimed.** Neither live-provider latency, inference
throughput, steady-state RSS nor retained heap was measured by these warmed
microbenchmarks. Allocated bytes per operation are not retained memory.

Not applied:

- Persistent sort/render caching (*hypothesis*): could avoid more repeated work,
  but adds invalidation/retention state for live priorities, roles and catalogs.
- General deep-cloning framework (*measured hotspot, deferred expansion*):
  remaining nested typed compat values still take the proven JSON fallback;
  changing all serialization semantics is unnecessary for these gains.
- Rendering rewrites (*measured*): visible-window rendering is around 0.33ms
  here, not the dominant all-catalog sort/composition cost.

## Correctness and validation

- Full-catalog optimized ordering is compared with the frozen pre-optimization
  comparator, including provider locale ties, duplicate MRU, custom/built-in
  roles, queries, separators and virtual-role rows.
- Every bundled compat object is compared with the original JSON-copy result.
  Edge cases cover nested/invalid JSON, HTML, nil raw values, empty fallback
  lists, colliding/Unicode keys, NaN and caller mutation ownership.
- A bounded five-second fuzz run compared 10,272 opaque-key/value inputs against
  the serialization oracle without a mismatch.
- `make check`, `make race`, `make smoke`, and `git diff --check` pass.
- Real OMP 18.14.2 PTY comparison: zero differing text/style cells in all seven
  unfiltered OpenAI/local/web views at both 120×40 and 80×24, using the shared
  public metadata fixture. No inference prompts or paid requests were sent.

## Reproduce

Use the repository's isolated canonical GOPATH setup (populated by `make check`)
so vendored test identities and Go result caching remain valid:

```sh
OMPIG_CHECK_CACHE=/tmp/omp-model-check make check
compiler="$(go env GOROOT)/bin/go"
cd /tmp/omp-model-check/gopath/src
GO111MODULE=off GOPATH=/tmp/omp-model-check/gopath GOTOOLCHAIN=local GOMAXPROCS=4 \
  "$compiler" test -run '^$' \
  -bench 'BenchmarkOMPCatalog|BenchmarkModelCatalog|BenchmarkModelBrowser' \
  -benchmem -benchtime=500ms -count=5 \
  github.com/MichaelKinsy/PiG/ai \
  github.com/MichaelKinsy/PiG/internal/codingagent \
  github.com/MichaelKinsy/PiG/tui
```

For attribution, select a single case with `-benchtime=3s` and pass
`-cpuprofile=/tmp/catalog.cpu -memprofile=/tmp/catalog.mem -o /tmp/catalog.test`;
inspect it with `go tool pprof -top` and `go tool pprof -alloc_space -top`.
The before executable/test binaries were built from `342fa69` with only the
benchmark test files added; these files do not alter production behavior.
