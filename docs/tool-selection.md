# Tool selection

Two different questions decide what a session can do, and they have different
answers:

- **Declared tools** — the tools listed in the request, which the model can call
  directly.
- **Callable tools** — the tools a `codemode` script can reach through
  `tools.<name>`. Only tools that are both registered and admitted by the same
  selection are callable, so a removed tool also disappears from scripts.

Selection is layered; each layer narrows the one below it.

| Layer | Lives in | Scope | Selects |
|---|---|---|---|
| Registry | linked packages, extensions, MCP servers | build and session start | every tool that can be registered: built-in, extended built-in, extension, MCP |
| `defaultTools` | `settings.json` | persistent | the built-in startup set, plus any inactive tool it names |
| CLI flags | the command line | one run | the final allowlist and denylist over all registered tools |
| Piglet `tools:` | `piglet.yaml` | one piglet | an upper bound on the built-in tools |

## Startup defaults

`read`, `bash`, `edit`, `write`, `grep`, `find`, `codemode`, `web_search`, `todo`
are active unless changed (`vendor/github.com/MichaelKinsy/PiG/internal/codingagent/settings_default_tools.go`).
`ls`, `powershell`, and `tool_search` are registered but start inactive.

`defaultTools` in `~/.pi/agent/settings.json` (global) or `<repo>/.pi/settings.json`
(project, trusted only) changes that set:

```json
{ "defaultTools": ["read", "bash", "grep", "codemode"] }
```

- **Plain names replace** the built-in defaults. `[]` is a list of nothing, not
  "unset" — the model gets no built-in tools.
- **`+name` activates, `-name` removes**, applied in list order. A list of only
  modifiers edits the defaults rather than replacing them, so `["-todo"]` keeps
  everything else.
- **A name that is not registered activates nothing.** There is no warning:
  `["ls"]` starts `ls` alone, and `["nope"]` leaves the defaults off and adds
  nothing. Spell names exactly.
- **Naming an inactive tool activates it**, which is how `tool_search` and `ls`
  become available at startup.
- **Project settings merge**, not replace: a project list of plain names replaces
  the global list outright, while a project list of only modifiers appends to it.
  Global `["read","bash"]` plus project `["-read"]` starts `bash`.

Extension tools register active and are not gated by `defaultTools`; this
binary's compiled-in `goal` is present even under `[]`.

## Per-run flags

| Flag | Effect |
|---|---|
| `--tools` / `-t <list>` | exclusive allowlist; everything not named is removed |
| `--exclude-tools` / `-xt <list>` | denylist, applied after the allowlist to every tool |
| `--no-tools` / `-nt` | allowlist of nothing |
| `--no-builtin-tools` / `-nbt` | built-ins off, extension and MCP tools stay |
| `--no-mcp` | no MCP servers connect, so no MCP tools |
| `--no-extensions` / `-ne` | extension tools gone; built-in extended tools (`todo`, `web_search`, `codemode`) and this binary's compiled-in `goal` survive |

Rules that are easy to get wrong:

- **`--tools` outranks every persistent setting.** Global `["todo"]` plus
  `--tools read` offers `read` only.
- **`--exclude-tools` outranks `--tools`.** Naming a tool in both offers neither.
- **`--tools` accepts `*` patterns** (`ba*`, `*`); `--exclude-tools` does too.
  `--tools '*'` offers every registered tool, including inactive ones such as
  `ls`, `powershell`, and `tool_search`, so the flag is the widest selection, not
  the default one.
- **Modifiers are a different mode.** A list of only `+name`/`-name` entries edits
  the current selection (`--tools +todo,-edit`); mixing plain names with modifiers
  is rejected: `tool names cannot be mixed with +name or -name entries`. Modifiers
  take exact names, never patterns.
- **An unknown name is silent.** `--tools nope` offers nothing; `--exclude-tools
  nope` changes nothing.
- **MCP tools survive a non-MCP allowlist** so `codemode` and `tool_search` can
  still reach them; add an `mcp__*` entry to the allowlist to filter them.
  `--no-tools` removes them.
- **`--tools` beats `--no-tools`.** `--no-tools --tools todo` offers `todo`.

## Resumed sessions

A session records the tool list it started with, and a resume replays that list
(`vendor/github.com/MichaelKinsy/PiG/coding/session_tools.go`). Editing
`defaultTools` afterwards affects new sessions only; the resumed session keeps its
recorded set. `--tools` modifiers and `--exclude-tools` do apply to a resume, so
they are the way to change tools mid-conversation:

```sh
ompig-slim -c --tools -read      # resume without read
ompig-slim -c --tools +tool_search
```

## Codemode and scripts

`codemode` is itself a tool, and it can only call tools the session admits. Its
description lists the callable set, and a script that reaches for a removed tool
fails with `tools.read does not exist. Available: bash, edit, write, grep, find,
web_search, goal.`

- `--tools codemode` offers `codemode` alone, so scripts get an empty tool set and
  every nested call fails. Select the tools the script needs beside it:
  `--tools codemode,read,write,bash`.
- `defaultTools: ["-read"]` has the same effect inside scripts, because the
  selection is shared.
- `todo` is deliberately excluded from scripts: its state is rebuilt from the
  session branch, which is not appended until the enclosing `codemode` call
  settles, so a chained call would read a stale list. Call `todo` directly.
- `tool_search` can load deferred tools into the session; it is only useful with
  extensions loaded, and calling it under `--no-extensions` fails with `Tool
  tool_search not found` because nothing registers it.

## Piglets and MCP servers

A Piglet's `tools:` field is an upper bound on the built-in tools its session may
use, not an activation list — it cannot turn a tool on, only keep it off
(`vendor/github.com/MichaelKinsy/PiG/coding/piglet/types.go`).

One MCP server is disabled without unloading it by setting `enabled: false` in its
`mcp.json` entry; disable the whole subsystem with `--no-mcp`
(`vendor/github.com/MichaelKinsy/PiG/coding/extension/mcp_servers.go`).

Package resources — extensions, skills, prompts, themes — are toggled by
`ompig-slim config`, which edits configured resources rather than individual
tools.

## Checking the result

These are traps for anyone verifying a change here:

- `ompig-slim diagnose` prints the unmodified built-in set from a stub `Settings`
  value, so it does not reflect `defaultTools` or any flag.
- `ompig-slim status` reports resource enablement (packages, skills, piglets), not
  the tool selection.
- This Go tree does not read `config.yml`; a `disabledExtensions:` entry there
  belongs to omp and has no effect here.

The reliable check is the system message's `tools` section, which lists exactly
the declared set:

```sh
ompig-slim --mode json --no-session --tools read,grep -p hi \
  | jq -r 'select(.type == "agent_end") | .messages[0].sections.tools'
```

```text
<tools>
- read: Read file contents
- grep: Search file contents for patterns (respects .gitignore)

Additional tools may be available.
</tools>
```

`scripts/smoke-test-codemode-binary.py` uses the stronger form of the same check
for every claim above: a loopback model records `body.tools` from the real
request, and the script then drives `codemode` to prove the callable set matches
the declared one.
