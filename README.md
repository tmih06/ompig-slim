<p align="center">
  <img src="assets/ompig-banner.svg" alt="OMPi in GO-slim startup logo" width="800">
</p>

<p align="center">
  <a href="https://github.com/tmih06/ompig-slim/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/tmih06/ompig-slim/actions/workflows/ci.yml/badge.svg?branch=main"></a>
  <a href="https://github.com/tmih06/ompig-slim/releases"><img alt="Version" src="https://img.shields.io/badge/version-0.2.5-blue?style=flat-square"></a>
  <a href="go.mod"><img alt="Go 1.27" src="https://img.shields.io/github/go-mod/go-version/tmih06/ompig-slim?style=flat-square&logo=go&logoColor=white"></a>
  <img alt="Platforms" src="https://img.shields.io/badge/platforms-linux%20%7C%20macOS%20%7C%20Windows-2ea44f?style=flat-square">
  <a href="https://github.com/MichaelKinsy/PiG"><img alt="PiG pin" src="https://img.shields.io/badge/PiG-v0.7.0-8A2BE2?style=flat-square"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/tmih06/ompig-slim?style=flat-square"></a>
  <a href=".github/CONTRIBUTING.md"><img alt="Contributions" src="https://img.shields.io/badge/PRs-discuss%20first-orange?style=flat-square"></a>
</p>

<p align="center">
  A native Go coding agent: <a href="https://github.com/MichaelKinsy/PiG">PiG</a>'s runtime plus a curated set of
  <a href="https://github.com/can1357/oh-my-pi">omp</a>'s features, in one self-contained binary.
</p>

## How it compares

| | **ompig-slim** | [omp](https://github.com/can1357/oh-my-pi) | [pi](https://github.com/earendil-works/pi) |
|---|:--|:--|:--|
| Runtime | one Go binary | Bun + JS + native addon | Node + JS |
| Startup, median | **121 ms** | 2,056 ms | 1,135 ms |
| Turn, median | **21 ms** | 41 ms | 36 ms |
| Peak RSS | **178 MiB** | 502 MiB | 439 MiB |
| Installed | **75 MB** | 422 MB | 238 MB |
| Config | `~/.pi/agent`, `.pi` | `~/.omp/agent`, `.omp` | `~/.pi/agent`, `.pi` |
| Extensions | TS, Go, Rust, Python, Lisp | TypeScript | TypeScript |
| `todo`, `web_search` | built in | built in | — |
| Subagents, Agent Hub | opt-in | built in | — |
| LSP, debugger | — | built in | — |

<sub>Five 120-turn sessions plus five 12-turn resumes, isolated local mode (no remote broker), Linux arm64, 4 vCPUs, synthetic loopback model.
Startup/turn/RSS are long-session medians; "installed" adds entrypoint, dependencies, and interpreter.
Baselines: omp 18.18.0, pi 1.5.2. Measures harness overhead, not model quality; feature rows describe
shipped code, not defaults. Details: [benchmarks/COMPARISON.md](benchmarks/COMPARISON.md).</sub>

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/tmih06/ompig-slim/main/scripts/install.sh | bash
```

Installs to `~/.local/bin/ompig-slim` (Linux and macOS, x64 or arm64), verifies the published SHA-256, and
needs no Go or `sudo`. Pass `--version v0.2.5` to pin a release or `--dir` to install elsewhere.

On native Windows x64 or ARM64, use PowerShell without WSL:

```powershell
Invoke-WebRequest -UseBasicParsing https://raw.githubusercontent.com/tmih06/ompig-slim/main/scripts/install.ps1 -OutFile install.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
```

See [native Windows setup](docs/windows.md) for prerequisites and limits. Update with
`ompig-slim update` (`--channel stable|beta`, `--all` for packages, `--models` for catalogs). Every merge to
`main` publishes a numbered beta for Linux, macOS, and Windows on x64 and ARM64.

From source with Go 1.27+: `make build && ./bin/ompig-slim`, or `make install`.

## Usage

```sh
cd ~/code/thing && ompig-slim    # then /login, or export OPENAI_API_KEY
ompig-slim -p "review this diff" # print one answer and exit
ompig-slim -c                    # continue the last session
ompig-slim --model @review       # run on a saved model role
ompig-slim --help                # every flag
```

Slash commands include `/login` `/model` `/settings` `/usage` `/goal` `/subagents` `/compact` `/tree`
`/fork` `/resume` and `/quit`. See the [usage reference](docs/usage.md) for all commands, shortcuts,
[`/goal`](docs/goal.md), [durable headless mode](docs/durable.md), and the agent-state file other tools can watch.

## Features

- **Tools:** `read`, `bash`, `edit`, `write`, `grep`, `find`, `web_search`, `codemode`, `todo`
- **Models:** ~5,900 IDs across 90 providers, OAuth, API keys, [custom endpoints](docs/custom-models.md), and
  [roles with fallback chains](docs/model-roles.md)
- **Accounts:** multiple accounts per provider with [quota-aware routing](docs/account-routing.md)
- **Subagents:** `Agent` with fixed roles, Agent Hub, an [on-demand tree](docs/subagents.md#live-inspection),
  and [explicit child outcomes](docs/subagents.md)
- **Guardrails:** [loop guard](docs/loop-guard.md) and [streaming rules](docs/streaming-rules.md)
- **Extensions:** TS, Go, Rust, Python, and Lisp; the [Herdr](https://herdr.dev) adapter is installable
- **Sessions:** [pre-compaction scrollback and cache-aware idle checkpoints](docs/compaction.md)
- **State:** Pi layout (`~/.pi/agent`, `.pi`), so existing sessions carry over
- **Themes:** `system`/`dark`/`light` built in, [100 optional palettes](themes/README.md)

Details: [features](docs/features.md). Codemode and other tools run with your user permissions and are not an
OS sandbox. Credentials live unencrypted in local SQLite or the configured broker's vault.

## Remote broker and workspace image

One hosted broker keeps sign-in, settings, model roles, and themes identical across machines and refreshes
OAuth tokens centrally. Inference and tools stay on each client.

```sh
docker run -d --name ompig-broker -p 127.0.0.1:8765:8765 \
  -e OMPIG_BROKER_TOKEN -v ompig-profile:/data \
  ghcr.io/tmih06/ompig-slim-broker:<version>
```

Setup, migration, and limits: [broker setup](docs/remote-broker.md). A small agent
[workspace image](docs/workspace-image.md) is published with each release.

## Development

```sh
make check   # tests and vet, patched PiG library included
make smoke   # PTY and codemode scenarios
make release # build the four platform archives
```

Read [CONTRIBUTING.md](.github/CONTRIBUTING.md) first: the tracked PiG tree carries project-specific patches.
Use `make deps` to restore other dependencies, not `go mod vendor`. Architecture and maintenance guidance
lives in [AGENTS.md](AGENTS.md).

Independent personal project, not an official PiG, Pi, or omp release. MIT. See [LICENSE](LICENSE) and
[NOTICE](NOTICE) for attribution.
