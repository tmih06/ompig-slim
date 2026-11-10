# CI performance and coverage

## Scheduling and correctness

- Go checks and races start independently of release builds on both Ubuntu and
  macOS. Explicit whole-project vet runs on Ubuntu for **both Linux and Darwin
  arm64 build contexts**, preserving checks of platform-specific files. macOS
  still runs both normal and race-enabled tests and their default vet checks.
- Only executable consumers wait for `release`. Benchmarks do not wait for tests
  or hygiene: they consume the same verified archive and run the same five serial
  samples. Publication waits for **all** validation jobs, including benchmarks.
- Native smoke scenarios are distributed across two machines per OS. The shard
  runner reads `make --dry-run smoke-built`, so the Makefile remains the sole
  scenario list. Scenarios must be independent Python commands with private
  profiles/servers; changing that contract requires changing the shard runner.
- Installer/update tests and platform Bash compatibility run on both OSes. The
  benchmark fixture and shard unit tests run once on Ubuntu. Native jobs neither
  install Go nor restore its cache. No frontend/Rust cache or tool-build cache is
  stored; this is a vendored Go repository with Node-based benchmark baselines.
- There are five macOS jobs per run (two Go, two smoke, one update), not a wide
  per-scenario matrix. The account-wide limit still applies across workflows;
  overlapping main commits can queue, but must never be discarded.
- Only pull requests share a concurrency group and cancel superseded work. Main,
  tag and manually dispatched runs each have a unique group. Merely setting
  `cancel-in-progress: false` is insufficient: GitHub still replaces an older
  *pending* run in a shared group.

## Docs filtering and required checks

Only root `README.md`, `CONTEXT.md` and `docs/**` are excluded from branch/PR
triggers. Embedded `prompts/*.md`, vendored files, scripts, assets, dependency
locks and workflow changes still trigger CI. Tags always run: GitHub does not
apply path filters to tag pushes. Manual dispatch remains available.

The active repository ruleset was inspected with `gh api` before enabling these
filters. It requires CodeQL/code-quality checks, not Actions status contexts;
CodeQL is separately managed by GitHub and was not disabled or weakened. There
is therefore no required CI status stranded by a docs-only path skip. **Before
adding a required Actions check**, replace workflow-level path filtering with
an always-triggered change-detection workflow and an always-reporting gate that
requires all applicable jobs to succeed. Do not require a path-filtered workflow.

## Cache and tools

Go deliberately retains `actions/cache` instead of setup-go's built-in cache:
inspection of the pinned setup-go implementation showed that it restores exact
keys only, without a fallback prefix. A source-keyed cache would build every new
revision cold; a lock-only immutable cache would never save updated results for
our patched vendored library. Role-specific fallback prefixes retain incremental
results across source revisions without pulling race or cross-target build data
into jobs that cannot reuse it.

Only `GOCACHE` is stored, never unused module downloads. A small file inside the
checkout fingerprints Git's top-level blob/tree IDs, excluding the three docs
paths; subtree IDs cover every descendant without scanning vendor through an
Actions glob. OS, architecture, toolchain, role and source fingerprint form the
cache key. Go still checks sources, flags and observed inputs; a cache hit never
skips a command.

Caches are evictable acceleration, not artifacts or correctness inputs. Keep
only Go results and npm's downloaded packages; setup-node hashes both benchmark
lockfiles. Do not cache `node_modules`, binaries already uploaded as artifacts,
or compiled CI toolchains. Lint/secret-scan tools are pinned upstream binaries,
verified against release SHA-256 manifests. The full-history scan still includes
nested archives (`--log-opts=--all --max-archive-depth=10`).

## Measurement method and baseline

The baseline is the last 50 repository runs returned by `gh run list --limit 50`
on 2026-11-10, IDs 42628252445 through 42818246944, including GitHub-managed
CodeQL, dependency and security workflows. Job and step timestamps came from
`gh api repos/tmih06/ompig-slim/actions/runs/<id>/jobs?per_page=100` for all 50.

- **Queue**: job `created_at` to `started_at`, not workflow creation to job start.
  The latter incorrectly calls dependency waiting runner queue time.
- **Setup**: checkout, tool/dependency installation, cache restore/save, artifact
  download/extraction, runner setup and teardown.
- **Checks**: tests, builds, verification, scans, benchmarks and result upload.
- Dependency wait (workflow creation to job creation) is reported separately.
  Step totals can omit runner gaps; component medians need not sum to total.
  Incomplete/skipped jobs are excluded; completed failures remain in job medians.

Baseline job medians in seconds (unchanged GitHub-managed workflows omitted):

| Job | Samples | Dependency wait | Queue | Setup | Checks | Job total |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Four-archive release | 28 | 1 | 3 | 52 | 141 | 217 |
| Linux tests/vet/race/smoke | 26 | 236 | 3 | 54 | 301 | 356 |
| macOS tests/vet/race/smoke | 26 | 236 | 4 | 120 | 548 | 690 |
| Serial CLI benchmark | 22 | 958 | 3 | 31 | 155 | 187 |
| Hygiene/full-history secrets | 28 | 1 | 3 | 15 | 159 | 176 |
| Broker contract (other branches) | 2 | 1 | 3 | 26 | 147 | 175 |

The 22 successful completed CI runs had a **19m15s** median wall time (range
13m43s–34m43s). Most runner queues were only 3–4 seconds; unnecessary dependency
waits, large cache transfers and serialized macOS scenarios were the bottlenecks.
The broker contract exists only on other feature branches and was not modified.

Two consecutive local `make build` invocations took **0.533s and 0.500s**, both
successful, with the existing Go cache. Builds are already incremental; no build
script or Docker change was justified. The uncommitted user changes present in
the working tree were not included in the pushed CI commits.

## Remote verification

The initial optimized run [42819604972](https://github.com/tmih06/ompig-slim/actions/runs/42819604972)
passed in **9m16s** with cold Go caches. This exposed an invalid setup-go cache
input outside the workspace; the second pass moved it inside the checkout.
It also split the remaining 3m31s macOS smoke job into two machines without
duplicating the Makefile's scenario list. The cold second pass
[42821772377](https://github.com/tmih06/ompig-slim/actions/runs/42821772377) passed
in **7m56s**, then the same-SHA warm repeat
[42823071480](https://github.com/tmih06/ompig-slim/actions/runs/42823071480) passed
in **4m31s**. The longest macOS smoke job was **1m57s** in that repeat.

The final cache review found setup-go's exact-key limitation, so the final pass
restores custom role-specific Go caches across revisions instead. A validation
run caught unsupported Git exclusion pathspecs in `ls-tree`; filtering top-level
records instead was executed and checked locally before another remote run.

The prebuilt gitleaks scanned 92 commits / 212.27 MB in **10.6s**, versus the
previous compiled tool's 89 commits / 212.23 MB in **159s** on run 42814272640.
The command, pinned version and nested-archive depth are unchanged; this is an
observed runner comparison, not a claim about the Go compiler's implementation.
