# ompig-slim

<p align="center">
  <img src="assets/ompig-banner.svg" alt="OMPi in GO-slim startup logo" width="800">
</p>

[![CI](https://github.com/tmih06/ompig-slim/actions/workflows/ci.yml/badge.svg)](https://github.com/tmih06/ompig-slim/actions/workflows/ci.yml)

A native Go coding assistant built on [PiG](https://github.com/MichaelKinsy/PiG), with personal `omp-slim` features integrated into the library runtime. It provides PiG's interactive terminal and CLI with shared Pi/OMP configuration and added account routing, provider support, built-in web tools, codemode, and subagents.

This is an independent personal project, not an official PiG, Pi, or oh-my-pi release. See [LICENSE](LICENSE) and [NOTICE](NOTICE) for license and attribution details.

## Get started

Install the latest release into `~/.local/bin` with one command:

```sh
curl -fsSL https://raw.githubusercontent.com/tmih06/ompig-slim/main/scripts/install.sh | bash
```

The installer downloads the prebuilt binary for your OS and CPU, verifies its
published SHA-256 checksum, and replaces `~/.local/bin/ompig-slim` atomically. It
needs `bash`, `tar`, and `curl` or `wget`; it does not need Go or `sudo`. Use
`curl ... | bash -s -- --version v0.1.0` for a specific release, or `--dir` to
install elsewhere. Prebuilt binaries cover Linux and macOS on x64 and arm64.

On a terminal it shows the startup mark and a progress bar while it downloads.
Redirect its output and you get plain lines instead, with no escape sequences.

Run `ompig-slim --help` for CLI options. Sign in with `/login` in the interactive app or `ompig-slim login` in the terminal. Provider API-key environment variables are also supported.

Keep an installed copy current with `ompig-slim update`, or its synonym `ompig-slim upgrade`. Both show the same mark and progress bar, verify the checksum, and replace the executable in place. `update --all` also refreshes installed packages and `update --models` refreshes model catalogs.

### Build from source

Requires Go 1.27 or newer. The repository pins Go 1.27.3 and vendors its dependencies.

```sh
git clone https://github.com/tmih06/ompig-slim.git
cd ompig-slim
make build
./bin/ompig-slim
```

To install in `~/.local/bin`:

```sh
make install
```

## What it includes

- PiG's native Go runtime and full CLI, including interactive, print, JSON, and RPC modes.
- Shared Pi configuration under `~/.pi/agent` and project settings under `.pi`.
- Skill badges in the prompt box: select a skill with `/`, keep typing, and select
  more skills anywhere in the same prompt. Enter accepts a skill without sending.
- `@path` and `@url` mentions in a prompt are read before the model runs, with an
  indented `├─ Read path (N lines)` tree under the submitted prompt, so the agent
  does not spend a turn calling `read`. Long files are truncated; directories
  list; a URL is fetched and rendered as `read` would.
- Multiple provider accounts with account selection, quota-aware routing, and credential migration.
- Built-in `web_search` and persistent codemode tools. `read` fetches http(s)
  URLs as well as files, matching oh-my-pi, which has no separate fetch tool.
- Tool cards rendered like oh-my-pi's: stream tools draw a rounded output block,
  file and search tools a single inline status row (`✔ Read path:70-95`), each
  naming the tool, its target, and exit code or elapsed time, with a filled
  phase background on every card and grep/find results drawn as a file tree. Write and edit
  stream their content while the arguments arrive, with diffs, highlighting,
  and Ctrl+O expansion.
- A `todo` tool that records multi-step work, and a stop-time reminder that continues a run while the list still has open tasks.
- Optional native subagents, model metadata, usage views, and terminal model selectors.
- OMP-style model roles, per-role fallback chains, auto thinking, and a fullscreen `/model` manager, plus an Alt+P session picker. See [model roles](docs/model-roles.md) for controls and configuration.
- Native Linux and macOS releases for x64 and arm64.

Codemode and other agent tools run with your user permissions; codemode is not an OS security sandbox. Account credentials are stored locally in SQLite and are not encrypted at rest.

## Development

```sh
make check    # tests and vet, including the patched PiG library
make race     # race-enabled checks
make smoke    # real model-menu PTY, codemode, subagent, and web-tool scenarios
make test-install     # the curl installer against a loopback release server
make test-self-update # update/upgrade against a loopback release server
make release  # build supported platform archives
```

See [CONTRIBUTING.md](CONTRIBUTING.md) before changing the code or vendored runtime. In particular, the vendored PiG library contains project-specific patches; a routine `go mod vendor` would replace them.

For technical architecture, project conventions, and maintenance guidance, see [CONTEXT.md](CONTEXT.md).
