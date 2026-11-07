# ompig-slim

[![CI](https://github.com/tmih06/ompig-slim/actions/workflows/ci.yml/badge.svg)](https://github.com/tmih06/ompig-slim/actions/workflows/ci.yml)

Public repository: [tmih06/ompig-slim](https://github.com/tmih06/ompig-slim). Licensed under [MIT](LICENSE), with upstream notices preserved.

A native Go coding harness using [PiG](https://github.com/MichaelKinsy/PiG) **as a library**, with tmih06's personal `omp-slim` behavior ported into the library runtime. The application is `ompig-slim` **0.1.0**; the pinned PiG library remains **0.7.0** (Pi 1.5.0).

The entry point calls `coding/cli.Main`; it does not launch an installed `pig`, wrap another agent process, or replace PiG's agent loop. The vendored library contains the personal patches because account routing, provider transports, and TUI behavior are not all exposed by PiG's public extension hooks.

## Build and run

```sh
git clone https://github.com/tmih06/ompig-slim.git
cd ompig-slim
make build
./bin/ompig-slim --version
./bin/ompig-slim version
./bin/ompig-slim

make run ARGS='--help'
make install                    # ~/.local/bin/ompig-slim
make install PREFIX=/some/path  # /some/path/bin/ompig-slim
```

Go 1.27 or newer is required. `go.mod` selects Go 1.27.3; install that toolchain or permit Go's automatic toolchain download. Dependencies are pinned in `go.mod`/`go.sum` and included in `vendor/`. Normal builds use `-mod=vendor` and need neither Bun nor Node for the default application or codemode. Optional integrations can require their own executables.

Build from the checkout with the vendored dependencies. `go install …@latest` does not use this repository's patched vendor tree and is not a supported installation method.

Use `/login` interactively, `ompig-slim login`, or a provider's API-key environment variable. `--help` documents the complete CLI. Stock PiG tools, interactive/print/JSON/RPC modes, sessions and branching, extensions, skills, themes, MCP, Piglets, and the Lisp extension builder remain linked.

## Shared configuration

The default agent directory is **`~/.pi/agent`** and project configuration uses **`.pi`**, rather than creating a separate `.pig` profile.

| Setting or file | Purpose |
| --- | --- |
| `OMPIG_SLIM_CODING_AGENT_DIR` | Override the agent directory |
| `OMPIG_SLIM_CODING_AGENT_SESSION_DIR` | Override session storage |
| `~/.pi/agent/agent.db` | OMP-compatible SQLite credentials, account state, and caches |
| `~/.pi/agent/settings.json` | Global settings |
| `~/.pi/agent/models.yml` | Preferred custom model configuration |
| `~/.pi/agent/models.yaml` | Used when `models.yml` is absent |
| `~/.pi/agent/models.json` | Used when both YAML files are absent |
| `~/.pi/agent/models-metadata.json` | Persisted model-metadata snapshot |
| `PIG_HOME` | Explicit upstream PiG library configuration-root override |

YAML uses the same schema as JSON. For example, a local OpenAI-compatible server can be configured without an extension:

```yaml
providers:
  local:
    api: openai-completions
    baseUrl: http://127.0.0.1:11434/v1
    apiKey: local
    models:
      - id: qwen3:8b
        name: Local Qwen3
        contextWindow: 32768
        maxTokens: 8192
```

### Accounts and migration

On first use of an empty credential database, valid legacy `auth.json` entries are imported and the original is retained as **`auth.json.migrated`**. Existing database rows and logout tombstones prevent stale legacy credentials from being silently restored. Back up the agent directory before sharing it between applications. Credentials are stored locally, not encrypted at rest; filesystem permissions remain part of the trust boundary.

```sh
ompig-slim auth login anthropic
ompig-slim auth login anthropic --no-browser
ompig-slim auth login aiand --api-key
ompig-slim auth accounts
ompig-slim auth accounts --provider openai --json
ompig-slim auth use openai ACCOUNT_KEY
ompig-slim auth logout openai --account ACCOUNT_KEY
```

`auth login` requires an interactive terminal and masks secret input. Account sign-in is preferred when available; `--api-key` explicitly chooses API-key setup. `--no-browser` selects an alternative sign-in method when offered and never launches a browser. It cannot be combined with `--api-key`. Successful Radius sign-in also configures its global MCP server without a second prompt.

Perplexity uses email OTP in CLI hosts without a managed-browser callback; SDK hosts can supply `OnBrowserSession` to offer SSO. Cursor and Perplexity sign-in availability does not imply a new model request transport beyond those in the source harness.

The SQLite schema is interoperable with OMP. The personal preferred-account cache key remains `omp-slim:auth-active:<provider>` for compatibility with the source fork; this preference is not a promise that OMP itself follows the same selection. Explicit request API keys bypass account-pool fallback.

## Personal feature port

| Area | Behavior |
| --- | --- |
| Identity and startup | `ompig-slim` name, version 0.1.0, personal half-block startup mark, shared Pi paths, and cache-miss notices enabled by default |
| Account storage | Multiple accounts per provider, account-specific logout, migration, preferred account, refresh leases, cross-process blocks, and preserved OMP SQLite identities |
| Account routing | Eligible Codex free accounts before paid accounts, per-session/provider/model affinity, plan/model eligibility, quota/auth/policy failure scopes, same-account refresh before rotation, and no replay after streamed output |
| Providers | Added OpenAI-compatible catalogs, provider-specific environments/headers/options, and DeepSeek cache-read accounting that uses the first strictly positive cache counter |
| Sign-in | Subscription/API-key setup flows plus native browser/device OAuth and credential refresh |
| Google | Gemini CLI and Antigravity sign-in, project/model discovery, and native Cloud Code Assist streaming, thinking, tools, usage, and error handling |
| Codemode | Default built-in code execution with persistent per-run state and nested tools; survives `--no-extensions` and respects tool-selection flags |
| Web tools | Default-active, credential-free `web_search` with sequential provider fallback and local `web_fetch` rendering; both survive `--no-extensions` |
| Usage | `/usage` plan-grouped quota dashboard, detail view, refresh/caching, reset/daybreak/credit displays, and local 371-day activity heatmap with fork de-duplication |
| Model metadata | Persisted catalog, two-hour freshness, ETag/304 handling, failed-fetch backoff, safe model-family matching, explicit-field protection, and generation-aware enrichment caches |
| Model selectors | Intelligence/TPS/context/cost columns and selected-model facts in both `/model` and `/scoped-models` |
| Output and themes | Per-run throughput including tool/wait time, contrasting dark/light palettes, semantic Agent Hub surfaces, and a fitting-line ANSI truncation fast path |
| Distribution | Linux/macOS x64/arm64 archives, checksums, bundled license notices, native Go validation, and personal-fork release notifications |

Added provider IDs: `aiand`, `aimlapi`, `alibaba-coding-plan`, `alibaba-token-plan`, `cline-pass`, `commandcode`, `coralbricks`, `deepinfra`, `inception`, `kilo`, `modular`, `ollama-cloud`, `sambanova`, `scaleway`, `synthetic`, and `venice`.

Additional sign-in/setup paths cover Z.ai, Kilo, Xiaomi, Alibaba Coding Plan, Alibaba Token Plan/QwenCloud, Cloudflare Workers AI, Cloudflare AI Gateway, Mistral Vibe, Cursor, Snowflake, Perplexity, Google Antigravity, and Gemini CLI. These are native protocol implementations, not fabricated credentials or a TypeScript compatibility subprocess.

Codemode is available with extensions disabled. Use `--tools read,codemode`, `--exclude-tools codemode`, `--no-tools`, or `--no-builtin-tools` to select the tools exposed to a run. It is not an operating-system security sandbox: the agent's tools run with your user permissions.

### Built-in web tools

`web_search` uses **DuckDuckGo → Startpage → Parallel → Exa → Firecrawl**,
stopping at the first usable response. No API keys, paid accounts, browser
downloads, or hosted reader are required. Sources include URLs and short
snippets; bot walls, rate limits, and failed providers are reported. Search
operators include `site:`, `-site:`, `inurl:`, `intitle:`, `filetype:`,
`after:`, `before:`, quoted phrases, exclusions, and `OR`. Filters are
best-effort: an eliminating constraint is relaxed with a note rather than
silently returning unrelated results.

`web_fetch` accepts a `url`, optional `offset`/`limit`, and `timeoutSeconds`.
HTML becomes Markdown locally; text, JSON, feeds, notebooks, images,
Office/EPUB documents, and archive/SQLite listings have native handlers.
PDF extraction is best-effort text extraction, not OCR or a full PDF renderer.
Append `:raw` to bypass text shaping or `:50-80` to select rendered lines.
The body cap is 50 MiB; model-visible output is capped at 300 lines / 50 KiB,
with full oversized text saved to a file that `read` can paginate.

Optional settings in `~/.pi/agent/settings.json`:

```json
{
  "webSearch": {
    "chain": ["duckduckgo", "startpage", "parallel", "exa", "firecrawl"],
    "timeoutSeconds": 60,
    "results": 10
  },
  "webFetch": {
    "reader": "auto",
    "maxBytes": 52428800,
    "timeoutSeconds": 30
  }
}
```

A configured search chain replaces the default; only the five listed engines
are supported. Search also accepts `limit`, `num_search_results`, `recency`
(`day`, `week`, `month`, `year`), and `timeout` in seconds. Time filters cannot
be verified for undated sources; responses explain that limitation.
`webFetch.reader` accepts `auto` or `native`; both use the local reader.
Search providers can be redirected through `webSearch.endpoints`, a map from
provider ID to transport URL, for local instances or fixtures.

Both tools are active by default and respect the normal tool-selection flags.
Use `--tools web_search,web_fetch` to select them or
`--exclude-tools web_search,web_fetch` to remove them. There is no network
request until a tool is called. Free endpoints are shared and may change or
rate-limit requests; JavaScript-only pages and scanned/font-encoded PDFs may
not yield usable text.

### Built-in subagents

`/subagents` toggles native subagents and saves the choice in `.pi/subagents.json`.
Use `/subagents on`, `/subagents off`, or `/subagents status` explicitly.
The feature starts disabled; it works with `--no-extensions`. Tool allowlists,
`--exclude-tools`, `--no-tools`, and `--no-builtin-tools` still apply.
Turning it off cancels and drains its agents and workflows.

`/agents` opens agent types, running/completed conversations, workflows,
scheduled jobs, and settings. Enabled sessions expose the `Agent`,
`get_subagent_result`, `steer_subagent`, and `SubagentWorkflow` tools.
Agents run in the background by default, with ten background slots and a
separate foreground pool. Completion notifications reach the parent at a
turn boundary; callers can also retrieve the full result or resume an agent.

The above-editor tree shows live activity, turns (`0/2 turns`), tool uses,
tokens, context usage, and compactions. Agent creation cards stay on one line:
`Agent  Run sleep loop one`.

Open the Agent Hub with **Alt+A**, **Ctrl+S**, `/agent-hub`, or double **Left**
at an empty prompt. It provides Agents and Activity tabs, search, flat or
parent grouping (**Tab**), and a detail inspector. Use **Up/Down** or **j/k** to
select, **Enter** to open an agent conversation, **r** to revive a parked
session without a model turn, and **x** to kill the selected agent. On narrow
terminals, **Shift+Tab** opens its details. In Activity, **f** changes event filters,
**s** changes scope, and **Space** toggles following.

The Hub uses a full-row selection highlight, separate roster and inspector
surfaces, colored status labels, and a context meter that changes color as it
fills. The agent tree and tool cards share the same palette. The `system`
theme follows terminal colors and uses the built-in dark/light palette when
the terminal cannot report its colors; custom themes keep their own tokens.

In a focused agent conversation, type to steer a running agent or resume a
settled session. Empty **Esc** or double **Left** returns to the main prompt;
**Alt+A** returns to the Hub. **Ctrl+O** expands conversation tool output.
**Ctrl+B** backgrounds running foreground calls. `@handle message` addresses
an agent; `@main message` addresses the parent.

Define custom agents in `.pi/agents/*.md`, `.agents/agents/*.md`, or
`~/.pi/agent/agents/*.md` with YAML frontmatter. Project definitions override
global and installed-package agents. Embedded types are `general-purpose`,
`Explore`, and `Plan`. Frontmatter can select tools, exclude tools/extensions,
pin a model and thinking level, set turn limits, enable persistent memory,
choose worktree isolation, and explicitly allow nested subagents. Nested
delegation is off unless `allowed_subagents` is present and is depth capped.
Global settings in `~/.pi/agent/subagents.json` are overridden by the project's
`.pi/subagents.json`; `/agents → Settings` edits the project layer.

`SubagentWorkflow` accepts inline JavaScript, a `scriptPath`, or a saved name
from `.pi/workflows`, `.agents/workflows`, or the global agent directory.
Scripts begin with `export const meta = {name, description}` and can use
`agent`, `parallel`, `pipeline`, `phase`, `log`, `args`, `budget`, and one level
of nested `workflow` calls. The native QuickJS runtime requires no Node.
Structured-output schemas, command gates, labeled child resumes, and replay
of successful leading calls are supported. The workflow inspector supports
pause/resume, skip, and retry while a child is running. Package agent types
are read-only, with project Eject and Disable actions. Worktree changes are preserved on
a branch; failures retain the checkout instead of deleting uncommitted work.

The source reference is [tintinweb/pi-subagents](https://github.com/tintinweb/pi-subagents)
at `50460b5df6934c4237983a504808a04ee0f8a0a0`.

Release notices follow the actual source fork, **`tmih06/omp-slim`**; renaming this local application does not create a new remote repository. That unsigned GitHub feed cannot replace the executable. An explicitly configured upstream signed-update manifest retains its signature verification; do not point it at a stock release if you want to preserve personal patches.

The prompt’s top-right border follows the router’s accepted live account or last confirmed route for the selected model, falling back to the store’s active account before a route is known, just like `/usage`. It totals accounts within that account’s subscription tier, choosing the quota window with the shortest known duration (for example, 5 hours before 7 days) and showing its next account reset countdown. The quota display does not imply that the selected model has access; the bar does not infer access from model names or tier rules. Provider rejections drive the router’s fallback and the bar follows the resulting account. Window durations and reset times come from the provider: free-tier 30-day quotas stay separate from Plus 7-day quotas when totaling accounts. Quotas refresh every minute in the background; unavailable accounts mark the total as partial. The label shows only the window, the usage board’s detailed bar, remaining percentage, and next reset. The bar fits 12–24 cells and hides when the border has insufficient space or quota data cannot be fetched. Initial loading also leaves the border empty.

`/usage` displays completed account reports as they arrive. Independent accounts fetch concurrently, so a slow provider does not delay healthy providers. Opening the dashboard uses the five-minute quota cache; press `r` to force a refresh. Closing the board cancels its requests, and reopening starts a fresh request if the previous one was canceled.

## Checks and native packages

```sh
make check                      # root + vendored PiG tests and vet (native Go caches)
make race                       # the same packages under the race detector
make smoke                      # compiled codemode, subagent, and web-tool scenarios
make release                    # all four supported native targets
make release ARGS='--platform linux-arm64 --out bin/releases'
```

CI runs tests, vet, race checks, compiled codemode smoke tests, and editable PTY startup checks on Linux and macOS. Separate jobs validate workflows/scripts, scan the complete Git history including nested archives for secrets, and build all four release targets with checksum, executable-header, and license-content verification. Go build/test/vet caches and pinned CI tools are cached with input-based invalidation; cache hits still run the check commands. The compiled codemode scenario uses a local HTTP provider fixture. The fixture requires no account credentials or paid provider calls. Unit/protocol fixtures and local executable scenarios do not establish that every remote provider's current login service has been live-tested.

The native executable was also exercised with isolated profiles: streamed file/shell/nested-code tools, session continuation, JSON/RPC, HTML export, SQLite migration and account logout, quota rotation, all 802 added catalog entries, and local requests through all 16 added provider IDs. A real HTTP fixture additionally verified forced OAuth refresh after a 401 and persisted credential rotation through the library's request path. Real terminal sessions covered masked API-key entry/cancellation, startup, throughput, `/usage` navigation, and both model selectors. Race checks passed for AI/account routing, the coding runtime, metadata, TUI, and CLI packages. Other platform archives are cross-built; only the Linux arm64 executable was run on this host.

Release archives are named `ompig-slim-{linux,darwin}-{x64,arm64}.tar.gz` and contain the executable, documentation, license notices, and dependency licenses. Windows, Android, p2, and Nix distributions are not personal release targets.

Measure editable startup against your normal profile with `python3 scripts/measure-startup.py ./bin/ompig-slim --runs 5 --max-ms 500`. The probe starts a real PTY, types as soon as raw mode starts, and measures when that edit is painted. It includes process launch time. The editor accepts input while terminal color discovery and the rest of initialization finish.

For startup diagnostics, run `PIG_STARTUP_TRACE=1 ompig-slim`; `PI_TIMING=1` adds phase durations. Cached metadata is restored before the first model listing, without a network request. Canonical metadata snapshots use typed decoding, stored catalog shapes are built lazily when saved, the matching index reuses parsed identities and ranked candidates, and unchanged/offline refreshes retain enrichment instead of rebuilding it. A downloaded catalog still invalidates enrichment, and an ETag/304 revalidation updates provenance without losing model metrics.

Selector-only metric lookups do not scan bundled reasoning/cache-policy references: those references cannot contribute selector metrics. Full custom-model enrichment still resolves them for cache policy and effort inheritance. `BenchmarkMetadataSelectorMetrics` exercises the selector path against the actual bundled reference catalog.

## Maintaining the library port

Source provenance: `tmih06/omp-slim`, HEAD `ba23ed9c3` over baseline `5a31a6d5f`, including the existing working-tree auth, account-selection, usage, and metadata fixes. No source profile credentials belong in this repository.

**Do not run `go mod vendor` over this tree without preserving and reapplying the personal patches.** It replaces the modified PiG runtime. Stage dependency updates separately, compare the library changes, retain licenses, and run both `make check` and the compiled smoke scenario. See [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md).

PiG is maintained by Michael Kinsy and contributors under the MIT license, following [Pi](https://github.com/earendil-works/pi) by Mario Zechner and contributors. Personal behavior derives from tmih06's `omp-slim` fork. This project is independent of PiG, Pi, and oh-my-pi; attribution and third-party terms are retained in [LICENSE](LICENSE), [NOTICE](NOTICE), and the vendored license files.
