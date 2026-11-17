# Codemode judge validation measurements

The judge bridge now reuses parsed JSON objects during validation. Previously,
`field(raw, key)` canonicalized and parsed the containing object on each lookup.
The replacement uses `orderedjson.Object.Get` after parsing each context,
question, criteria object, or image block once. It keeps the same validation and
error messages. It adds no cache or retained state.

## Method

Measurements ran on Linux arm64 with Go 1.27.4 and four logical CPUs. Five
before/after pairs alternated on the same machine. Each benchmark used a
300 ms sampling interval. Other processes shared the machine.

`BenchmarkJudgeContext` measures validation alone and the complete local
`models.judge` bridge with a fake judge. Each bridge operation creates its run,
validates and decodes the context, executes the fake judge, records the nested
call, serializes answers, and checks successful completion. It does not measure
QuickJS, role discovery, networking, or provider inference.

The small fixture has a 128-byte state and one question. The large fixture has
a 64 KiB state and 32 questions. All runs returned `PASS`. Boundary tests passed
against both versions, including malformed questions and image blocks.

Both versions used the same final tree based on `7e03f81`. The before binary
used a Go overlay restoring only the earlier `checkJudgeContext` implementation.
The overlay included the canonical GOPATH path because Go resolves the vendored
package through the isolated test workspace.

```sh
# After make deps, compile through the repository's isolated GOPATH test workspace.
export OMPIG_CHECK_CACHE=/tmp/ompig-codemode-judge-check
# From $OMPIG_CHECK_CACHE/gopath/src, using the compiler from go env GOROOT:
GO111MODULE=off GOPATH="$OMPIG_CHECK_CACHE/gopath" GOTOOLCHAIN=local \
  "$compiler" test -c -o /tmp/judge-final-after.test \
  github.com/MichaelKinsy/PiG/coding/extension/builtin/codemode
# Repeat for before with -overlay=/tmp/judge-baseline-overlay.json.
# Alternate these commands five times:
/tmp/judge-final-before.test -test.run='^$' -test.bench=BenchmarkJudgeContext -test.benchtime=300ms -test.count=1
/tmp/judge-final-after.test -test.run='^$' -test.bench=BenchmarkJudgeContext -test.benchtime=300ms -test.count=1
```

## Results

Times are median milliseconds per operation. Ranges show the minimum and maximum
of the five runs. Allocations are median bytes per operation.

| Fixture and operation | Before ms | After ms | Before range ms | After range ms | Before bytes | After bytes |
| --- | ---: | ---: | --- | --- | ---: | ---: |
| Small validation | 0.051 | 0.020 | 0.049 to 0.055 | 0.019 to 0.021 | 20,442 | 7,796 |
| Small local bridge | 0.072 | 0.040 | 0.072 to 0.075 | 0.039 to 0.043 | 24,490 | 11,828 |
| Large validation | 2.648 | 0.940 | 2.598 to 2.814 | 0.830 to 0.965 | 2,395,369 | 764,841 |
| Large local bridge | 3.384 | 1.663 | 3.364 to 3.638 | 1.579 to 1.709 | 2,571,523 | 938,774 |

An earlier, separate CPU-profile run attributed 58.7% of sampled CPU cumulatively
to `orderedjson.Object.UnmarshalJSON`. JSON scanning and garbage collection
accounted for much of the remaining work. The measured allocation reduction
matches the removed reparsing. Validation accounted for about 78% of the large
local bridge's before time. These results do not establish a reduction in
end-to-end provider latency.

The benchmark and boundary regression tests are checked in next to the codemode
bridge. Raw local samples are in `/tmp/judge-final-{before,after}-{1..5}.txt` for
this session. The profile is `/tmp/judge-validation-before.cpu`.

## Changes deliberately not made

- No candidate cache. Judge reassignment and credential changes remain live.
- No removal of the classifier availability recheck, route validation, or answer
  validation. Those changes could alter failure handling.
- No change to whether a route error stops fallback or how cancellation is
  prioritized when no candidate exists. Those are behavior decisions, not
  simplifications.
