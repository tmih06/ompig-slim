# Go 1.27.3 versus Go 1.27.4 rebenchmark

Measured on Linux arm64 with four reported CPUs using the repository's synthetic
loopback model workload. Both binaries were built from the same checkout with
identical flags and benchmark settings; only the Go toolchain changed.

Commands:

```sh
GOTOOLCHAIN=go1.27.3 go build -mod=vendor -trimpath -ldflags='-s -w' \
  -o /tmp/ompig-go-bench/ompig-go1.27.3 .
GOTOOLCHAIN=go1.27.4 go build -mod=vendor -trimpath -ldflags='-s -w' \
  -o /tmp/ompig-go-bench/ompig-go1.27.4 .
python3 scripts/benchmark.py --slim /tmp/ompig-go-bench/ompig-go1.27.3 \
  --only ompig-slim --runs 5 --warmups 1 --turns 120 --resume-turns 12 \
  --output benchmark-results/go1.27.3-paired
python3 scripts/benchmark.py --slim /tmp/ompig-go-bench/ompig-go1.27.4 \
  --only ompig-slim --runs 5 --warmups 1 --turns 120 --resume-turns 12 \
  --output benchmark-results/go1.27.4-paired
```

## CLI workload

| Metric | Go 1.27.3 | Go 1.27.4 | Change |
|---|---:|---:|---:|
| Ready (ms) | 121.9 | 121.1 | -0.6% |
| Long turn (ms) | 21.3 | 21.3 | -0.0% |
| Long p95 (ms) | 31.2 | 31.5 | +1.1% |
| Long peak RSS (MiB) | 176.9 | 173.8 | -1.8% |
| Long CPU (s) | 1.51 | 1.52 | +0.7% |
| Resume ready (ms) | 200.6 | 189.4 | -5.6% |
| Resume turn (ms) | 32.3 | 32.7 | +1.4% |
| Resume p95 (ms) | 37.1 | 36.4 | -1.9% |

## Canonical JSON benchmark

Median of five benchmark samples:

| Payload | Go 1.27.3 (ns/op) | Go 1.27.4 (ns/op) | Change |
|---|---:|---:|---:|
| Short frame | 223.1 | 223.9 | +0.4% |
| Source file | 4,866 | 4,858 | -0.2% |
| Large history | 674,186 | 670,097 | -0.6% |

The paired run shows no meaningful broad performance change from Go 1.27.3 to
Go 1.27.4 for this workload. The existing large JSON gains are from the
repository's canonical scanner optimization, not this patch-level update.
Raw CLI results are retained in the ignored `benchmark-results/go1.27.3-paired`
and `benchmark-results/go1.27.4-paired` directories.
