# Install and use ompig-slim

Use this page to get from a fresh binary to a working session. Return to the
[docs router](README.md) for focused tasks.

## 1. Build or install the binary

Install the latest release into `~/.local/bin` (Linux and macOS, x64 or arm64).
The installer verifies the published SHA-256 and needs no Go and no `sudo`:

```bash
curl -fsSL https://raw.githubusercontent.com/tmih06/ompig-slim/main/scripts/install.sh | bash
```

Pass `--version v0.1.0` to pin a release, or `--dir` to install elsewhere.
Archives are also on [GitHub Releases](https://github.com/tmih06/ompig-slim/releases).

From a source checkout with Go 1.27 or newer, the dependencies are vendored:

```bash
make build          # writes ./bin/ompig-slim
./bin/ompig-slim --version
make install        # or install into ~/.local/bin
```

`ompig-slim update` replaces a binary the installer placed. A build from source
and a `go install` are not installer receipts, so update those by rebuilding.
`go install` is not supported for this module: the entry point is the repository
root, so build it from a checkout instead.

ompig-slim stores user state under `~/.pi`, and the agent directory is
`~/.pi/agent`. Set `OMPIG_SLIM_CODING_AGENT_DIR` to move the agent directory, or
`PIG_HOME` for an isolated configuration root in tests and CI. Only the first
moves settings, credentials and sessions.

### Change what the install script does

`install.sh` reads these environment variables. Set them on the `sh` side of the
pipe, for example:

```bash
curl -fsSL https://raw.githubusercontent.com/tmih06/ompig-slim/main/scripts/install.sh \
  | OMPIG_SLIM_VERSION=v0.1.0 bash
```

| Variable | Meaning |
|---|---|
| `OMPIG_SLIM_VERSION` | Install this release tag instead of the latest stable. |
| `OMPIG_SLIM_INSTALL_DIR` | Directory that receives `ompig-slim`. The default is `$HOME/.local/bin`. |
| `OMPIG_SLIM_DOWNLOAD_BASE` | Release download root, for a mirror or a test server. The default is this repository's GitHub Releases URL. |

The script verifies the archive's SHA-256 before it writes anything to the
install directory, whichever mirror these variables select.

### macOS Gatekeeper after a manual download

A browser download can add the local `com.apple.quarantine` attribute to an
archive. Verify the published SHA-256 first, then clear it on that one file:

```bash
xattr -d com.apple.quarantine /path/to/ompig-slim
/path/to/ompig-slim --version
```

Do not recursively remove quarantine attributes from a broad directory.

## 2. Authenticate a model provider

```bash
ompig-slim login github-copilot
```

API-key providers may use their documented environment variable:

```bash
export OPENAI_API_KEY=...
ompig-slim --model openai/gpt-4.1
```

See [Providers](providers.md). Product extensions may contribute additional
login targets, such as a platform host, through the same `ompig-slim login` surface.
Use `ompig-slim login --help`/`ompig-slim --help` for the targets available in your binary.

## 3. Install packages

Raw ompig-slim accepts local, git, npm, and HTTP package sources:

```bash
ompig-slim install ./my-package
ompig-slim install git:https://github.com/example/my-package.git
ompig-slim install npm:@example/package
```

An installed product extension may add another resolver scheme, such as a
private marketplace. Its authentication and catalog commands are product
features, not assumptions in raw ompig-slim.

## 4. Run ompig-slim

```bash
ompig-slim                              # interactive TUI
ompig-slim -p "summarize this repo"     # one-shot prompt
ompig-slim --model github-copilot/gpt-5-mini
```

## 5. Install and run a piglet

Install and validate a registerable Piglet, then invoke it by name:

```bash
ompig-slim piglet add ./piglets/my-agent.yaml
ompig-slim piglet validate my-agent
ompig-slim --piglet my-agent
```

A Piglet with relative `local:` extension or skill origins is source-bound.
Run it directly instead of registering it:

```bash
ompig-slim piglet validate ./piglets/my-agent.yaml
ompig-slim --piglet ./piglets/my-agent.yaml
```

See [Piglets](piglets.md) for schema and resolution.

## 6. Build an explicit Piglet entry point or Binary

A source script is a thin Host Piglet entry point. It is not managed by ompig-slim and
creates no ompig-slim state or record:

```bash
ompig-slim piglet build my-agent --format script --out ~/.local/bin/my-agent
my-agent
```

On Windows the script is a cmd.exe batch file, so give it a `.cmd` name, for
example `--out my-agent.cmd` (D69).

A Piglet Binary is a target-native executable for one immutable Piglet
composition:

```bash
ompig-slim piglet build my-agent --format binary --out ./pig-my-agent
./pig-my-agent
```

The Piglet artifact lifecycle lives under `ompig-slim piglet`.

A Piglet Binary may fuse compatible Go components and host exact subprocess
components through ompig-slim's ordinary subprocess extension host. Its registered
Piglet release records where each component comes from: binary, release
closure, required `agentEnv`, or external mapping. Subprocess use does not create
a different binary kind.

Piglet Image is the planned OCI carrier for a Piglet that needs an environment-complete
runtime. It is not in this release; no command produces one. Artifact startup never silently installs, pulls, logs in, or substitutes
an origin.

## 7. Set up an extension toolchain only when authoring/building

You need a language toolchain to author a source extension or build its release
closure. A consumer needs no compiler for a prebuilt native component; interpreted
subprocess components still need the exact runtime recorded by the Piglet
release or supplied by `agentEnv`.

Check before installing anything:

```bash
go version
cargo --version
python3 --version
node --version
```

Prefer a version manager already present on the machine. Otherwise use the OS
package manager or the language's official per-user installer, with the user's
consent. Do not remove or replace an existing system toolchain without asking.

For a Go extension, the required version is the `go` directive in the staged SDK
module. `ompig-slim extension init` stages an offline-resolvable project and
`ompig-slim install <path> --validate-only --json` validates it.

## Troubleshooting

| Symptom | Likely cause | Action |
|---|---|---|
| `ompig-slim` not found | binary directory absent from `PATH` | add the install directory |
| no selectable models | no provider credential | run `ompig-slim login <provider>` or set its API key env var |
| contributed source scheme is unknown | product resolver is not installed/loaded | install or run the product distribution that owns it |
| piglet origin does not resolve | missing local source/package/product resolver | run `ompig-slim piglet validate <name>` and fix the reported origin |
| source extension fails on first use | missing language toolchain | install it with consent or use a prebuilt Piglet release/Image |
| Piglet artifact cannot start a component | registered release/environment/external requirement missing | rebuild with a supported format or follow the reported mapping remediation; do not substitute PATH content |

## Offline docs

```bash
ompig-slim docs list
ompig-slim docs show install
ompig-slim docs path
```
