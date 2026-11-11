<p align="center">
  <img src="assets/ompig-banner.svg" alt="OMPi in GO-slim startup logo" width="800">
</p>

<p align="center">
  <a href="https://github.com/tmih06/ompig-slim/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/tmih06/ompig-slim/actions/workflows/ci.yml/badge.svg?branch=main"></a>
  <a href="https://github.com/tmih06/ompig-slim/releases"><img alt="Version" src="https://img.shields.io/badge/version-0.1.2-blue?style=flat-square"></a>
  <a href="go.mod"><img alt="Go 1.27" src="https://img.shields.io/github/go-mod/go-version/tmih06/ompig-slim?style=flat-square&logo=go&logoColor=white"></a>
  <img alt="Platforms" src="https://img.shields.io/badge/platforms-linux%20%7C%20macOS-2ea44f?style=flat-square">
  <a href="https://github.com/MichaelKinsy/PiG"><img alt="PiG pin" src="https://img.shields.io/badge/PiG-v0.7.0-8A2BE2?style=flat-square"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/tmih06/ompig-slim?style=flat-square"></a>
  <a href="CONTRIBUTING.md"><img alt="Contributions" src="https://img.shields.io/badge/PRs-discuss%20first-orange?style=flat-square"></a>
</p>

<p align="center">
  A native Go coding agent: <a href="https://github.com/MichaelKinsy/PiG">PiG</a>'s runtime plus a curated set of
  <a href="https://github.com/can1357/oh-my-pi">omp</a>'s features, in one self-contained binary.
</p>

## How it compares

| | **ompig-slim** | [omp](https://github.com/can1357/oh-my-pi) | [pi](https://github.com/earendil-works/pi) |
|---|:--|:--|:--|
| Runtime | one Go binary | Bun + JS + native addon | Node + JS |
| Startup, median | **121 ms** | 2,176 ms | 1,129 ms |
| Turn, median | **21 ms** | 44 ms | 36 ms |
| Peak RSS | **179 MiB** | 499 MiB | 446 MiB |
| Installed | **73 MB** | 419 MB | 271 MB |
| Config | `~/.pi/agent`, `.pi` | `~/.omp/agent`, `.omp` | `~/.pi/agent`, `.pi` |
| Extensions | TS, Go, Rust, Python, Lisp | TypeScript | TypeScript |
| `todo`, `web_search` | built in | built in | — |
| Subagents, Agent Hub | opt-in | built in | — |
| LSP, debugger | — | built in | — |

<sub>Five 120-turn sessions plus five 12-turn resumes, Linux arm64, 4 vCPUs, synthetic loopback model.
Startup/turn/RSS are long-session medians; "installed" adds entrypoint, dependencies, and interpreter.
Baselines: omp 18.18.0, pi 0.73.1. Measures harness overhead, not model quality; feature rows describe
shipped code, not defaults. Details: [benchmarks/COMPARISON.md](benchmarks/COMPARISON.md).</sub>

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/tmih06/ompig-slim/main/scripts/install.sh | bash
```

Installs to `~/.local/bin/ompig-slim` (Linux and macOS, x64 or arm64), verifies the published SHA-256, and
needs no Go or `sudo`. Pass `--version v0.1.2` to pin a release or `--dir` to install elsewhere. Then:

```sh
ompig-slim update           # replace the installed binary
ompig-slim update --all     # packages first, then the binary
ompig-slim update --models  # refresh model catalogs
```

From source with Go 1.27+ (dependencies are vendored): `make build && ./bin/ompig-slim`, or `make install`.

## Usage

```sh
cd ~/code/thing && ompig-slim    # then /login, or export OPENAI_API_KEY
```

| Command | What it does |
|---|---|
| `ompig-slim "list the failing tests"` | start interactive with an opening prompt |
| `ompig-slim -p "review this diff"` | print one answer and exit |
| `ompig-slim --mode json` / `--mode rpc` | JSONL event stream, or an RPC loop over stdio |
| `ompig-slim -c` / `-r` / `--session <id>` | continue the last session, pick one, open by id |
| `ompig-slim --model openai/gpt-4o:high` | pick `provider/id` with a thinking level |
| `ompig-slim --model @review` | run on a saved [model role](docs/model-roles.md) |
| `ompig-slim @src/a.ts "explain"` | attach files or images to the prompt |
| `ompig-slim --tools read,grep,find` | restrict the tool set for one run |

| Key | Action |
|---|---|
| `Ctrl+O` | expand or collapse tool output |
| `Ctrl+P` | cycle to the next model in the role cycle |
| `Alt+P` | model picker for this session |
| `Shift+Tab` | cycle thinking level |
| `Ctrl+Q` | queue a follow-up behind the current task |
| `Alt+W` | fold the task/quota widget to its border control |
| `Alt+A` | Agent Hub (live subagents) |
| `Esc` | interrupt |

Slash commands: `/login` `/model` `/settings` `/usage` `/subagents` `/compact` `/tree` `/fork` `/resume`
`/export` `/hotkeys`. `ompig-slim --help` lists every flag.

### For other tools

Every session publishes what the agent is doing, so a multiplexer, status bar, or notifier can watch
a run without knowing anything about this program:

```sh
# one document per running agent, rewritten on each change
ls  <PIG_HOME|~/.pi>/state/agent/*/status.json
cat <PIG_HOME|~/.pi>/state/agent/$$/status.json
# {"state": "working", "agent": "ompig-slim", "mode": "tui", "session_id": "...",
#  "session_file": "...", "cwd": "...", "pid": 1234, "updated_at": "..."}
```

`state` is `idle`, `working`, `blocked` (with the dialog in `detail`), or `exited`. Check
`mode == "tui"` if you only care about interactive agents. The files are written atomically; treat a
missing one, or one whose `pid` is no longer running, as "no information" rather than an error.

The [Herdr](https://herdr.dev) adapter is an installable extension built on that file, not part of the
binary:

```sh
ompig-slim install ./extensions/herdr
```

It gives Herdr a sidebar row, tab and workspace rollups, `herdr agent wait`, and session restore after
a Herdr server restart.

## What's inside

| Area | Detail |
|---|---|
| Tools | `read` (files, dirs, URLs), `bash`, `edit`, `write`, `grep`, `find`, `web_search`, `codemode`, `todo` |
| Prompt | `/skill:` badges compose several skills in one prompt; `@path` and `@url` mentions are read up front |
| Loop guard | Steers the primary agent away from identical tool calls after five consecutive turns |
| Models | ~5,900 IDs across 90 catalog providers, OAuth, API keys, custom endpoints |
| Routing | Roles, fallback chains, auto thinking, `/model` manager, `Alt+P` picker |
| Accounts | Multiple accounts per provider, quota-aware routing, imported credentials, rate-limit resets |
| Subagents | `Agent`, `get_subagent_result`, `steer_subagent`, workflows, Agent Hub |
| TUI | omp-style tool cards, prompt-anchored task/quota widget, stop-time todo reminder |
| Integrations | Publishes agent state for other tools; Herdr adapter ships as an installable extension |
| Themes | 100 palettes plus `system`/`dark`/`light`, colorful Markdown, palette-aware body color |
| State | Pi layout: `~/.pi/agent` global, `.pi` per project, so existing sessions carry over |

Codemode and other tools run with your user permissions; codemode is not an OS sandbox. Credentials live
in local SQLite, or in the configured broker’s SQLite vault, unencrypted at rest.

## Remote broker

One hosted service keeps sign-in, custom providers, settings, model roles and keybindings identical across
your machines. Its SQLite vault owns credentials and **refreshes OAuth tokens on the broker**; clients
receive access credentials, never refresh tokens. Inference and tools stay on each client—no traffic proxy.

```sh
docker run -d --name ompig-broker -p 127.0.0.1:8765:8765 \
  -e OMPIG_BROKER_TOKEN -v ompig-profile:/data \
  ghcr.io/tmih06/ompig-slim-broker:<version>
```

On each client, open `/login`, choose **Remote ompig-slim broker**, and enter the URL and secret. Unattended
machines can skip the dialog: `ompig-slim broker setup --url <origin> --token-file <path>` writes the same
connection. Import existing local accounts explicitly with `ompig-slim broker migrate --from-local`.
Preferences synchronize with revision checks; account writes persist directly on the broker. Use HTTPS or
a private encrypted network. See [broker setup and containers](docs/remote-broker.md) for migration,
deployment and limits. Broker and standalone mode use the same `agent.db`, so a closed database can
[transfer directly between them](docs/remote-broker.md#back-up-or-switch-to-standalone-mode).

## Development

```sh
make check             # tests and vet, patched PiG library included
make race              # the same with the race detector
make smoke             # model menu, widget, codemode, subagent, web-tool PTY scenarios
make test-install      # curl installer against a loopback release server
make test-self-update  # update/upgrade against a loopback release server
make test-broker-container  # the broker image and two clients, needs Docker
make test-broker-cluster    # one broker host plus two client containers, needs Docker
make release           # build the four platform archives
```

Read [CONTRIBUTING.md](CONTRIBUTING.md) first: the vendored PiG tree carries project-specific patches that
`go mod vendor` would drop. Architecture and maintenance rulings live in [CONTEXT.md](CONTEXT.md).

Independent personal project, not an official PiG, Pi, or omp release. MIT — see [LICENSE](LICENSE) and
[NOTICE](NOTICE) for attribution.
