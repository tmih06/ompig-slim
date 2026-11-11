# Configuration

PiG reads configuration from two places: the agent directory, which applies to every project you open, and the `.pi` directory inside a project. PiG also loads instruction files such as `AGENTS.md` from the directories around your working directory.

ompig-slim keeps its state under the configuration root, `~/.pi` by default, the same tree Pi uses. This build shares Pi and OMP's configuration through `PIG_HOME`, `PIG_CODING_AGENT_DIR` and the `~/.pi`/`.pi` defaults (`AppName` and `UsePiDirs` in `internal/codingagent/paths.go`); upstream PiG's separate `~/.pig` tree and its `PIG_USE_PI_DIRS` opt-in are divergence D2 and do not apply here.

Use `/settings` in an interactive session to change common preferences. When you edit a configuration file by hand, run `/reload` so the running session picks up the change. `/reload` rereads settings, keybindings, extensions, skills, prompt templates, themes and context files.

## Configuration root

PiG chooses one configuration root at startup:

| Order | Source | Root |
|---|---|---|
| 1 | `PIG_HOME` is set and not empty | `$PIG_HOME` |
| 2 | default | `~/.pi` |

`XDG_CONFIG_HOME` is not consulted by this build. Pi reads neither `PIG_HOME` nor `XDG_CONFIG_HOME`. Its default agent directory is `~/.pi/agent`.

The root holds these directories:

| Path | Contents |
|---|---|
| `<root>/agent/` | The agent directory. See below. |
| `<root>/piglets/` | Your [Piglet](piglets.md) source files. |
| `<root>/artifacts/piglets/` | Piglet Binaries that PiG manages. |
| `<root>/receipts/piglets/` | Piglet resolution and build records. |
| `<root>/state/<namespace>/` | State that one PiG capability owns. |
| `<root>/docs/` | The offline documentation that `ompig-slim docs` writes. |

## Agent directory

The agent directory is `<root>/agent`, which is `~/.pi/agent` by default. Set `PIG_CODING_AGENT_DIR` to move only this directory. Pi reads `PI_CODING_AGENT_DIR` for the same purpose. PiG reads that variable only in shared mode.

This page writes the agent directory as `<agent-dir>`.

| Path | Purpose |
|---|---|
| `<agent-dir>/settings.json` | User [settings](settings.md), including Package and resource declarations. |
| `<agent-dir>/keybindings.json` | Custom [keybindings](keybindings.md). |
| `<agent-dir>/models.json` | Custom endpoints and models. See [custom providers](custom-provider.md). |
| `<agent-dir>/auth.json` | Stored API keys and OAuth tokens. See [providers](providers.md). |
| `<agent-dir>/trust.json` | Saved project trust decisions. See [security](trust.md). |
| `<agent-dir>/AGENTS.md` | Your instructions for every project. See [context files](#context-files). |
| `<agent-dir>/SYSTEM.md` | Replaces PiG's default system prompt. |
| `<agent-dir>/APPEND_SYSTEM.md` | Adds text to the end of the system prompt. |
| `<agent-dir>/extensions/` | User [extensions](extensions.md). |
| `<agent-dir>/skills/` | User [skills](skills.md). |
| `<agent-dir>/prompts/` | User [prompt templates](prompt-templates.md). |
| `<agent-dir>/themes/` | User [themes](themes.md). |
| `<agent-dir>/sessions/` | Saved [sessions](sessions.md), one directory per project. |
| `<agent-dir>/bin/` | Helper binaries that PiG downloads for its tools, such as `fd` and `rg`. |
| `<agent-dir>/npm/`, `<agent-dir>/git/` | Packages installed at user scope. See [packages](packages.md). |

PiG reuses helpers already installed in `<agent-dir>/bin/`. Concurrent requests for the same missing helper share one download within a tools manager, including when installation finishes before a waiting request claims the download. Pi starts separate downloads for overlapping same-tool requests; PiG's coalescing is an additive robustness improvement (D81). Separate managers and processes do not share this coordination.

PiG also finds skills in `~/.agents/skills/`. See [skills](skills.md).

## Using Pi's directories

This build always uses Pi and OMP's configuration: `internal/codingagent.UsePiDirs` returns true, so nothing has to be set. The agent directory is `~/.pi/agent` unless `PIG_CODING_AGENT_DIR` names another one, and the project directory is `<cwd>/.pi`.

`PIG_USE_PI_DIRS=1` is upstream PiG's opt-in; it is accepted for compatibility and changes nothing here, because the shared tree is already selected.

`PI_CODING_AGENT_DIR` and `PI_CODING_AGENT_SESSION_DIR` are read as fallbacks beside `PIG_CODING_AGENT_DIR` and `PIG_CODING_AGENT_SESSION_DIR`. `--session-dir` still wins over either, followed by the `sessionDir` setting. `PIG_HOME` controls the product's own SDK caches, documentation, Piglets and other state, not the agent directory.

Sharing includes settings, credentials, custom models, keybindings, trust decisions, sessions and discovered resources. Pi and Pi 1.5.0 read the same current JSON and JSONL shapes. Settings writes retain unknown keys. This build does not write `models.json`. Use Pi-compatible Package declarations and resources; PiG-only extensions and Piglets do not become Pi-compatible by changing their directory.

The product coordinates auth, settings, trust and dynamic model-catalog cache writes with Pi's directory-lock protocol. Independent sessions can use the same agent directory. Do not edit one session concurrently from two processes. Stop older processes before sharing their files. Empty regular lock files from v0.2.0 on are reclaimed after checking that no older writer holds them. If an older process holds a lock, acquisition uses the normal timeout and asks you to stop that process. Nonempty files, symlinks and active lock directories are not reclaimed.

Packages that hard-code Pi paths, such as Powerline and `pi-acp`, need the default `~/.pi/agent` location:

```bash
PI_ACP_PI_COMMAND="$(command -v ompig-slim)" pi-acp
```

The adapter keeps its own `~/.pi/pi-acp/session-map.json`; this build does not copy or rewrite it. Use one adapter process per home to avoid competing map writes. A custom `PI_CODING_AGENT_DIR` does not relocate paths a package hard-codes, including pi-acp's prompt-template directories.

## Project directory

A project keeps its configuration in `.pi` inside the working directory. PiG reads `.pi` only from the working directory itself. It does not look for `.pi` in parent directories.

| Path | Purpose |
|---|---|
| `.pi/settings.json` | Project settings. Keys here override the same keys in `<agent-dir>/settings.json`. |
| `.pi/SYSTEM.md` | Replaces the system prompt for this project. |
| `.pi/APPEND_SYSTEM.md` | Adds project text to the end of the system prompt. |
| `.pi/extensions/` | Project extensions. |
| `.pi/skills/` | Project skills. |
| `.pi/prompts/` | Project prompt templates. |
| `.pi/themes/` | Project themes. |
| `.pi/npm/`, `.pi/git/` | Packages installed with `ompig-slim install --local`. |
| `.pi/piglets/` | Project Piglets. |

### Project trust

A project directory can contain code that runs on your machine. PiG therefore asks whether you trust a project before it reads `.pi/settings.json`, `.pi/mcp.json`, `SYSTEM.md`, `APPEND_SYSTEM.md`, extensions, skills, prompt templates or themes from it. If you do not trust the project, PiG ignores all of them. A Piglet that `--piglet <name>` finds in `.pi/piglets/` is used only when the project has a saved trust decision, `defaultProjectTrust` is `"always"`, or the run passes `--approve`. PiG selects a Piglet before it can ask, so it uses your own Piglet of that name for the run, and the trust prompt, when one appears, names the project's Piglet.

One setting is an exception. PiG reads `sessionDir` from the project settings before it asks, because it must locate the project's sessions first.

Use `--approve` (`-a`) to trust the project for one run, or `--no-approve` (`-na`) to ignore project files for one run. Use `/trust` to save a decision. See [security](trust.md).

### System prompt files

PiG looks for `SYSTEM.md` and for `APPEND_SYSTEM.md` separately. For each name, a file in a trusted project wins over the file in the agent directory. PiG uses one file per name and does not combine the two.

The command line overrides both files. `--system-prompt` replaces the discovered `SYSTEM.md`. `--append-system-prompt` replaces the discovered `APPEND_SYSTEM.md`, and you can give it more than once. Each value can be text or a path to a file.

## Context files

Context files hold instructions for the model, such as build commands, code conventions and project rules. PiG adds their content to the system prompt.

PiG looks for context files in the agent directory, in the working directory, and in every parent directory up to the file system root. In each directory it takes the first file that exists from this list:

1. `AGENTS.override.md`
2. `AGENTS.md`
3. `AGENTS.MD`
4. `CLAUDE.md`
5. `CLAUDE.MD`

A directory contributes at most one file. `AGENTS.override.md` therefore replaces `AGENTS.md` or `CLAUDE.md` in its own directory only. It does not hide files in the agent directory or in other directories.

PiG loads the files in this order:

1. the file in the agent directory;
2. the files from the file system root down to the working directory.

The file closest to your working directory comes last, so its instructions appear after the general ones.

In a Git worktree that sits inside its main checkout, PiG skips the main checkout's context file when the worktree root has its own file with the same name. This prevents the same repository instructions from loading twice.

Context files do not need project trust. PiG loads them even when you decline to trust the project. Pi behaves the same way.

The interactive startup banner lists the loaded files on its `[Context]` line. Use `--no-context-files` (`-nc`) to load none of them for one run.

After an in-process switch to a session from another project, PiG keeps the context files of the project it started in. Pi rebuilds them for the destination project. This difference is D61.

## Startup migrations

On startup, PiG runs the one-time migrations that Pi runs:

1. When `<agent-dir>/auth.json` does not exist, PiG moves credentials from a legacy `<agent-dir>/oauth.json` and from `apiKeys` in `<agent-dir>/settings.json` into `auth.json`.
2. PiG moves session files from the top of the agent directory into `<agent-dir>/sessions/`.
3. PiG moves the `fd` and `rg` binaries from `<agent-dir>/tools/` into `<agent-dir>/bin/`.
4. PiG renames a `commands/` directory to `prompts/` and warns about deprecated extension directories.

PiG applies these migrations only to the selected directories. It does not import files from the unselected tree.

## Diagnostics

`ompig-slim diagnose` prints the PiG and pinned Pi versions, the build, the Go runtime, the platform, the binary path, the staged extension SDKs, the resolved configuration paths, the project trust decision, the active Piglet, the providers with stored credentials, the built-in tools, and the default model that a session starts with. It names the settings file of each default model setting, and it does not count the `defaultModel` of an untrusted project. Pi reads `defaultModel` as a model id, so `"bench/bench-2"` is not split into a provider and a model, and diagnose shows the model that startup falls back to.

Set `PIG_STARTUP_TRACE=1` to print the time of each startup step. See [environment variables](environment-variables.md) for the other diagnostic variables.

## Extension runtime variables

PiG sets these variables for the extension processes it starts. Do not set them yourself.

| Variable | Effect |
|---|---|
| `PIG_EXT_SOCKET` | Socket the extension connects to. |
| `PIG_EXT_SOCKET_<NAME>` | Socket for one extension inside a packed runtime cell. |
| `PIG_EXT_PACKED_CELL` | Key of the packed runtime cell. |
| `PIG_EXT_PACKED_CELL_HASH` | Cache key of the packed runtime cell. |

`PIG_PARITY_HARNESS=1` turns on the hidden `/probe-*` slash commands that the parity harness uses.

## Related

- [Settings](settings.md) lists every settings key.
- [Environment variables](environment-variables.md) lists the variables PiG reads.
- [Project trust](trust.md) explains trust decisions.
