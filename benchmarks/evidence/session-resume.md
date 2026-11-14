# Old-session task-widget restoration

Workload: `session_load_benchmark_test.go` creates 10,000 chained `read`
tool results, each containing 4,096 bytes of text (about 42 MiB JSONL).
Linux arm64, Go 1.27.4, four benchmark CPUs. No provider, credentials or
network are involved. Fixture creation is excluded from timings.

The initial regression test failed with:

```text
TestSessionRestoreTodoSkipsUnrelatedHistory:
    todo restore decoded 9744 unrelated tool results
```

The session retains 256 tail entries; restoring the task widget previously
read and decoded all remaining tool results even though none were `todo`.
The widget runs before transcript painting, including for compacted sessions
whose old file/shell results would otherwise remain cold.

| Operation | Before, two runs | After, two runs |
|---|---:|---:|
| Widget restore | 949.58 / 687.75 ms | 0.171 / 0.199 ms |
| Restore allocated bytes/op | 236,913,168 / 236,912,874 | 16 / 16 |
| Restore allocations/op | 292,467 / 292,465 | 1 / 1 |
| Unrelated cold entries decoded | 9,744 | 0 |
| File indexing + tail restore | 124.51 / 111.81 ms | 115.86 / 144.01 ms |
| Indexing allocated bytes/op | 21,568,186 / 21,567,253 | 22,405,341 / 22,405,442 |

Mechanism: extend the existing interned message-kind index with tool names,
then filter the branch before decoding task-widget records. Slot size is
unchanged; the intern table gains a kind per distinct tool/result combination.
Index construction spends about 0.8 MB more transient allocations in this
fixture to avoid about 237 MB during widget restoration. No session format,
ordering, compaction boundaries or widget authority rules change.

These are component timings, **not** measured end-to-end terminal startup.
Before timings used three iterations and CPU profiling; after timings used
three iterations without profiling. The orders-of-magnitude reduction is also
locked down by a deterministic zero-unrelated-reads assertion, independent of
wall-clock variation. File indexing remains roughly the same order of cost;
no indexing speedup is claimed. Picker listing and transcript painting were
not changed or independently benchmarked in this pass.

Reproduce using the repository's canonical GOPATH test layout, prepared by
`scripts/check-go.sh`:

```sh
compiler="$(go env GOROOT)/bin/go"
GO111MODULE=off GOPATH="$HOME/.cache/ompig-check/gopath" GOTOOLCHAIN=local \
  "$compiler" test github.com/MichaelKinsy/PiG/internal/codingagent \
  -run 'TestSessionRestoreTodo|TestTodoRestoreIndexedBranch' \
  -bench '^BenchmarkSession(Load|RestoreTodo)$' -benchtime=3x -count=2
```

Functional coverage includes pre-compaction plans, fork isolation, read-only
views, failed mutations, appended clearing mutations, and escaped,
case-insensitive and duplicate tool-name JSON members. Verification also ran
the complete coding-agent package, targeted race tests, root tests, vet and
`scripts/smoke-test-info-widgets.py` against a newly built binary in regular
and fullscreen terminals.
