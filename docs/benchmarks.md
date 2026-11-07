# CLI performance benchmarks

## CI build chain

`.github/workflows/ci.yml` builds and verifies the four release archives once in `release`.
Linux and macOS `check` jobs download the appropriate archive and call `make smoke-built` instead of rebuilding.
The Linux `benchmark` job waits for `release`, `check`, and `hygiene`, downloads the same artifact, verifies its checksum, and extracts it.
The benchmark job does not install Go or invoke a build command.
Compiling test binaries in `check` remains necessary; those are not additional release executable builds.

The runner installs original upstream Pi (`@mariozechner/pi-coding-agent` 0.73.1), OMP (`@oh-my-pi/pi-coding-agent` 18.18.0), Node 24.18.0, and Bun 1.4.4 through `actions/setup-node` and `oven-sh/setup-bun`.
Pi here means the original Node CLI, not the Go PiG port.
OMP and Pi have separate npm manifests and integrity-checked lockfiles under `benchmarks/`.
Separate dependency trees make installed-footprint comparisons meaningful and avoid sharing one baseline's dependencies with another.
No globally installed CLI is used.

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
| ompig-slim | 72,614,151 | 32,292,361 | 0 | 0 | 0 | 0 | 72,614,151 | 32,292,361 |
| OMP 18.18.0 | 18,601,469 | 6,404,768 | 293,001,827 | 84,622,789 | 107,339,768 | 51,651,341 | 418,943,064 | 142,671,840 |
| Pi 0.73.1 | 870 | 561 | 149,604,645 | 31,644,723 | 121,723,800 | 43,894,882 | 271,329,315 | 75,490,930 |

Reading the table:

- `ompig-slim` is one self-contained native executable: no interpreter, no dependency tree, nothing to resolve at startup. Nothing is downloaded at run time.
- OMP's disk cost is dominated by its platform native addon (`pi_natives.linux-arm64.node`, 113 MiB) and Bun itself. Pi's is dominated by its JavaScript dependency tree and Node.
- Compared as total installed bytes, `ompig-slim` is 17.3% of OMP's footprint and 26.8% of Pi's.
- Compared as compressed download bytes, it is 22.6% of OMP's and 42.8% of Pi's.
- The 72.6 MB native binary is larger than OMP's 18.6 MB JavaScript bundle and Pi's 870-byte launcher. A fair single-file comparison would only be against another self-contained binary; neither baseline ships one.
- The interpreter numbers are the exact Node and Bun executables on this host and are not charged to a CLI a second time elsewhere.
- Two other views are worth stating explicitly: total installed bytes is 4.8x smaller than OMP and 2.7x smaller than Pi; compressed download bytes is 3.4x smaller than OMP and 1.3x smaller than Pi.
- The compressed gap against Pi is narrow because a stripped Go binary, dominated by machine code, type metadata, and pclntab, compresses less than JavaScript text does. Raw disk footprint is where the native build wins clearly.

Locally cross-compiled `linux/amd64` release artifact: `ompig-slim-linux-x64.tar.gz` is 34,725,899 bytes, containing a 77,697,184-byte binary plus licenses.
The arm64 development build above is 72,614,151 bytes; architecture and linker inputs differ, so compare release archives to release archives.

Contents of the stripped arm64 executable:

| Section | Bytes |
|---|---:|
| `.text` | 25,411,860 |
| `.gopclntab` | 20,054,054 |
| `.rodata` | 16,279,225 |
| `.go.type` | 5,517,000 |
| `.noptrdata` | 3,924,961 |
| `.data` | 1,270,338 |

The 33.5 MB FIPS DRBG scratch buffer is `.noptrbss` and is virtual address space, not file bytes.
Unstripped builds are 97,487,846 bytes; the existing `-s -w` flags already remove the symbol table and DWARF data.
A further `strip -s` recovered only 42,327 bytes, so external stripping would add a toolchain dependency for no meaningful gain.
xz reaches 24,759,828 bytes and zstd-19 reaches 27,356,167 bytes on this binary; release archives stay on gzip because it is universally available and already verified.
Largest embedded payloads are the codemode/QuickJS runtime, syntax-highlighting data, and the Lisp extension builder.

`benchmarks/evidence/footprint.json` records these numbers with the command that produced them.
The footprint figures come from a single short session whose purpose is only to emit the distribution columns; the 120-turn timing numbers above come from separate full runs.
