# Environment variables

ompig-slim reads these variables at startup. Each one has a setting or a default that
covers the normal case, so set a variable only when you need to change ompig-slim from
outside its configuration: in a container, in CI, or while debugging.

## Location

| Variable | Effect |
|---|---|
| `OMPIG_SLIM_CODING_AGENT_DIR` | Agent directory. Default `~/.pi/agent`. The only variable that moves it |
| `OMPIG_SLIM_CODING_AGENT_SESSION_DIR` | Session storage. `--session-dir` wins over it; the `sessionDir` setting is the fallback |
| `PIG_HOME` | Configuration root for product state: docs, SDK caches, Piglets, Wazero cache. Default `~/.pi`. An empty value is ignored |
| `XDG_CONFIG_HOME` | Not consulted: the configuration root is `PIG_HOME` or `~/.pi` |
| `PIG_USE_PI_DIRS` | Accepted for compatibility and changes nothing: this build always uses Pi's `~/.pi`/`.pi` (D2) |

`PIG_CODING_AGENT_DIR`, `PIG_CODING_AGENT_SESSION_DIR` and the `PI_*` spellings of both are **not read** by this build; they are upstream PiG's and are listed in `--help` only because that text is Pi's. Set `OMPIG_SLIM_CODING_AGENT_DIR` and `OMPIG_SLIM_CODING_AGENT_SESSION_DIR`.

`PIG_HOME` moves product state only. The agent directory stays `~/.pi/agent` unless `OMPIG_SLIM_CODING_AGENT_DIR` names another one, so settings, credentials, sessions and trust decisions do not follow `PIG_HOME`. See [Configuration](config.md).

## Network

| Variable | Effect |
|---|---|
| `PIG_OFFLINE`, `PI_OFFLINE` | Do not check for package updates or refresh dynamic model catalogs such as Radius. Accepts `1`, `true` or `yes`; any `PI_OFFLINE` value stops the catalog refresh |
| `PIG_UPDATE_URL` | Release manifest used by `ompig-slim update` |
| `PIG_SELF_UPDATE_COMMAND` / `PI_SELF_UPDATE_COMMAND` | Command `ompig-slim update` runs to update ompig-slim itself, split on whitespace (no quoting), with `{packageName}`, `{version}` and `{installSpec}` placeholders. `PIG_SELF_UPDATE_COMMAND` is read first and either takes precedence over the `selfUpdateCommand` setting. |
| `PIG_UPDATE_TRUST_ROOT` | Public key that signs the release manifest |
| `PI_SKIP_VERSION_CHECK` | Do not check for a newer ompig-slim release at startup. Any non-empty value disables the check, as in Pi. `ompig-slim update` still checks |
| `PI_OAUTH_CALLBACK_HOST` | Host the OAuth callback listens on |
| `PI_SHARE_GATEWAY_URL` | Artifact upload endpoint used only when you run `/share`. Default `https://pi-in-go.dev/v1/artifacts?visibility=unlisted&title=ompig-slim+session` |
| `PI_CODEMODE_MAX_WORKERS` | Maximum codemode VMs alive at once across all sessions in the process; default `2`. Each VM's script may use up to 256 MB. Raise it when one process runs several sessions, such as in-process subagents. A value that is not a positive integer is ignored; the variable is read on every script. Further scripts wait for a free VM. See [Codemode](codemode.md) |
| `PI_TELEMETRY` | Override the `enableInstallTelemetry` setting: `1`, `true` or `yes` enables it, anything else disables it. Gates the install/update ping to `https://pi-in-go.dev/api/report-install?version=<version>` (sent only after a fresh install or an update with new changelog entries) and the OpenRouter/NVIDIA/Cloudflare attribution headers. `PI_OFFLINE` also stops the ping regardless of this setting. |
| `PIG_INSTALL_TELEMETRY_URL` | Overrides the install/update ping's destination URL. No upstream equivalent; tests use it to point at a local server instead of the network. |

Set `PIG_OFFLINE` in CI. Without it, every run checks for updates and fails
slowly when the network is blocked.

Offline mode, from `--offline`, `PIG_OFFLINE` or `PI_OFFLINE`, sets
`PI_OFFLINE=1` and `PI_SKIP_VERSION_CHECK=1` in the environment extensions
inherit, as Pi does. Pi extensions that check `PI_OFFLINE` see ompig-slim's offline
mode.

## Diagnostics

| Variable | Effect |
|---|---|
| `PIG_DEBUG` | Write a debug log |
| `PIG_DEBUG_KEYS` | Log every key as ompig-slim decodes it |
| `PIG_DEBUG_TOOLS` | Print the number of tools sent with each OpenAI Chat Completions request to stderr. Needs the exact value `1` |
| `PIG_RENDER_DEBUG` | Write render diagnostics |
| `PI_TUI_DEBUG_REDRAW` | Log why the screen was fully repainted. Needs the exact value `1` |
| `PIG_STARTUP_TRACE` | Time each startup step |
| `PIG_PROFILE` | Write Go runtime profiles when ompig-slim exits. Takes a comma-separated list of `cpu`, `heap`, `allocs`, `block`, `mutex`, `goroutine`, `trace`, `metrics` and `flight`. `metrics` writes `pig-<pid>-metrics.json`: allocation and GC counts for the whole run, live heap, peak RSS, and the goroutines, open files and child processes left at exit. `flight` keeps the last 10 seconds of the execution trace in memory and writes it as `<agent dir>/crash-output/<pid>.flight.trace` (mode 0600) when the session ends without a user request, beside the crash output file; it writes nothing at a normal exit |
| `PIG_PROFILE_DIR` | Directory for `PIG_PROFILE` output. Default: the working directory |

Use `PIG_DEBUG_KEYS` when a keybinding does not respond: it shows whether the key
reached ompig-slim at all. See [terminal setup](terminal-setup.md).

Use `PI_TUI_DEBUG_REDRAW=1` when the view jumps while output arrives. The log records each full repaint's cause in `pi-tui-debug.log` in the configured TUI log directory, normally the agent directory. Standalone TUI instances without a configured log directory do not write redraw logs. Pi reads the same variable. ompig-slim ignores the names `PIG_DEBUG_REDRAW` and `PI_DEBUG_REDRAW`.

When a session ends without you asking it to, look in the agent directory (`~/.pi/agent`, or the directory `OMPIG_SLIM_CODING_AGENT_DIR` names). `exit.log` has one line for each signal (`SIGHUP`, `SIGTERM`, `SIGINT`) and each closed terminal, with the time, the pid and the parent pid. If only the input closes while the terminal stays open, ompig-slim keeps running, as Pi does. `crashes.json` has the panics and fatal errors, and the next start announces the newest one. A session that the operating system killed leaves no line of its own, so the next start adds one to `exit.log`. `ompig-slim diagnose` prints these paths and the newest `exit.log` lines. The `PIG_DEBUG` log is `ompig-slim-debug.log` in the temporary directory (`$TMPDIR` on macOS, a per-user folder).

`PIG_DEBUG_TOOLS` and `PI_TUI_DEBUG_REDRAW` need the exact value `1`. The other
diagnostic variables accept any non-empty value.

## Sessions

| Variable | Effect |
|---|---|
| `OMPIG_SLIM_CODING_AGENT_SESSION_DIR` | Session storage and lookup. `--session-dir` overrides it |

`PIG_CODING_AGENT_SESSION_DIR` and `PI_CODING_AGENT_SESSION_DIR` are not read by this build.

## Experimental features

| Variable | Effect |
|---|---|
| `PI_EXPERIMENTAL` | Turn on experimental features when set to `1`. The footer then shows an `xp` badge |
| `PIG_EXPERIMENTAL` | Same as `PI_EXPERIMENTAL`. Accepts `1`, `true` or `yes` |

## Updates and deployments

These variables describe how ompig-slim was installed, so `ompig-slim update` can give the right
instruction. A product deployment sets them. You do not need them for a normal
install.

| Variable | Effect |
|---|---|
| `PIG_INSTALL_TIER` | Declare the installation type when ompig-slim cannot detect it: `container`, `image`, `immutable-binary` or `piglet-binary`. Any other value is an error |
| `PIG_IMAGE_REF` | Image reference that `ompig-slim update` names for a container or image install |
| `PIG_IMAGE_PULL_CMD` | Pull command that `ompig-slim update` prints for a container or image install |
| `PIG_REDEPLOY_INSTRUCTION` | Exact redeploy instruction that `ompig-slim update` prints for a container or image install |
| `PIG_UPDATE_ALLOW_LOOPBACK_HTTP` | Allow an `http://localhost` or loopback `PIG_UPDATE_URL`. Accepts `1`, `true` or `yes`. For testing a release server |

## Display

| Variable | Effect |
|---|---|
| `PI_HARDWARE_CURSOR` | Show the terminal's own cursor |
| `PI_CLEAR_ON_SHRINK` | Repaint when content shrinks |
| `PI_CLIPBOARD_OSC52` | Set to `1` to also send copied text to the terminal with OSC 52 after the local clipboard write, as ompig-slim does over SSH or Mosh. Use it when a remote client reaches ompig-slim without SSH variables, such as a multiplexer attached from another machine or a remote IDE daemon |
| `PI_HYPERLINKS` | Override OSC 8 hyperlink detection with `1`, `0`, or `auto` |
| `PI_IMAGE_PROTOCOL` | Override inline image detection with `kitty`, `iterm2`, `none`, or `auto` |
| `PI_TRUE_COLOR` | Override true-color detection with `1`, `0`, or `auto`. With true color off, themes use the 256-color palette |

Each has a setting that does the same thing. A `terminal.hyperlinks`,
`terminal.images` or `terminal.trueColor` setting wins over its variable. See [settings](settings.md).

## Extensions and piglets

| Variable | Effect |
|---|---|
| `PIG_PIGLET_NAME` | Piglet to run |
| `PIG_PIGLET_PATH` | Directories searched for piglets |
| `PIG_ACTIVE_PIGLET_SOURCE` | A session sets it for its tools: the path of the Piglet it runs, or the name of a Piglet Binary. `ompig-slim status`, `ompig-slim diagnose` and `ompig-slim piglet show` read it. Setting it does not select a Piglet. |
| `PIG_SDK_GO_ROOT` | Go SDK used when building an extension |
| `PIG_SDK_PY_ROOT` | Python SDK used when building an extension |
| `PIG_SDK_RS_ROOT` | Rust SDK used when building an extension |
| `PIG_CELL_BUILD_TIMEOUT` | Time an extension build may take |
| `PIG_TS_RUNTIME` | Runtime that runs TypeScript and JavaScript extensions for this process: `node` or an installed runtime's name. Overrides the `extensionsTypescriptRuntime` setting (D19). |
| `PIG_TS_RUNTIMES_DIR` | Runtime slot directory. Default `<agent dir>/runtimes/typescript` |
| `PIG_BUILDERS_FILE` | Container builder configuration for `ompig-slim piglet build`. Default `$PIG_HOME/state/pigletbuild/builders.json` |
| `PIG_CONTAINER_ENGINE` | Engine of the built-in `container` builder for `ompig-slim piglet build`: `docker` or `podman`. Default: Podman when on `PATH`, else Docker |
| `PIG_PIGLET_PULL_ALLOW_LOOPBACK_HTTP` | Allow `ompig-slim piglet pull`, `ompig-slim piglet update` and `ompig-slim piglet publish --to npm --binaries` to use an `http://localhost` or loopback release URL. Accepts `1`, `true` or `yes`. For testing a release server |
| `PIG_PIGLET_GITHUB_URL` | Loopback origin, such as `http://127.0.0.1:8080`, that replaces `github.com` and `api.github.com` for `github:` release refs (`ompig-slim piglet pull`, `ompig-slim piglet publish --to npm --binaries`) and `ompig-slim piglet update` discovery. Requires `PIG_PIGLET_PULL_ALLOW_LOOPBACK_HTTP`; any other host is refused. For testing a release server |
| `PIG_LOGO_GLYPHFREE` | `1` renders the logo without special glyphs in `ompig-slim extension preview-login`, and `0` keeps the glyphs. Unset, ompig-slim drops the glyphs only in Apple Terminal |

The `PIG_SDK_*` variables point a build at an SDK checkout instead of the staged
copy. Use them while developing the SDK itself. See
[extensions](extensions.md).

## Process markers

ompig-slim sets two markers that every child process inherits, as Pi does:

| Variable | Effect |
|---|---|
| `AI_AGENT=pi` | Generic marker that lets tooling identify the launching agent |
| `PI_CODING_AGENT=true` | Lets a child process detect that it runs inside the coding agent |

## Related

- [Providers](providers.md) lists the credential variables for each provider.
- [Settings](settings.md) covers the same behavior in configuration.
- [Config](config.md) lists the files ompig-slim reads.
- [Terminal setup](terminal-setup.md) covers display and key problems.
