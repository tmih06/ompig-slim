# Knowledge graph

The entities Pig is built from, how they relate, where each lives, and the command that inspects it. Use it to locate a concept before reading its page; each row links the page to read next.

| Entity | What it is | Where it lives | Inspect with | Read |
|---|---|---|---|---|
| Pi | Upstream TypeScript coding agent; the behavior oracle. | `https://github.com/earendil-works/pi` | `ompig-slim version` | [divergences.md](divergences.md) |
| PiG | The Go implementation; executable pig. | `the pig executable` | `ompig-slim verify` | [README.md](README.md) |
| Stock PiG | Product-neutral pig binary with no Piglet selected. | `the pig executable` | `ompig-slim version` | [built-in-extensions.md](built-in-extensions.md) |
| Session | Append-only JSONL tree of entries for one conversation. | `~/.pi/agent/sessions` | `/tree, /session` | [sessions.md](sessions.md) |
| Model Runtime | Session-owned model lookup, auth, completion, and streaming used by every mode. | `inside the pig executable` | `/model, pig --list-models` | [models.md](models.md) |
| Provider | Turns a Transcript into one Event Stream for one API. | `inside the pig executable` | `ompig-slim --list-models` | [providers.md](providers.md) |
| Mode | Interactive TUI, print, JSON event stream, or RPC over stdin/stdout. | `ompig-slim flags: -p, --mode json, --mode rpc` | `ompig-slim --mode rpc` | [commands.md](commands.md) |
| Resource | One capability: extension, skill, prompt, hook, MCP definition, theme, or environment. | `~/.pi/agent, .pi/ in a project` | `ompig-slim status --json` | [concepts.md](concepts.md) |
| Extension | Code Resource in Go, Rust, Python, or Node/TypeScript that registers tools, commands, events, and UI. | `~/.pi/agent/extensions, .pi/ in a project` | `ompig-slim install --validate-only <dir>` | [extensions.md](extensions.md) |
| Extension SDK | One extension contract per language: Go, Rust, Python, TypeScript declarations. | `scaffolded per language by pig extension init` | `ompig-slim extension init` | [extension-api.md](extension-api.md) |
| Extension realization | How an extension runs: isolated process, packed runtime cell, or fused into a Piglet Binary (Go only). | `inside the pig executable` | `ompig-slim status --json` | [runtime-cells.md](runtime-cells.md) |
| Extension Host | Loads, isolates, reloads, and shuts down extension realizations over one wire contract. | `inside the pig executable` | `/reload` | [runtime-cells.md](runtime-cells.md) |
| Package | Versioned collection of Resources from npm, git, or a local path; never owns a Piglet. | `~/.pi/agent/npm, ~/.pi/agent/git` | `ompig-slim list` | [packages.md](packages.md) |
| Piglet | One named agent: selects and scopes Resources and sets defaults. | `*.yaml Piglet file` | `ompig-slim piglet validate <file>` | [piglets.md](piglets.md) |
| PiG Standard | The optional curated Piglet: login identity, PiG Runner, Angry Pigs. | `an installed Piglet` | `ompig-slim piglet list` | [built-in-extensions.md](built-in-extensions.md) |
| Piglet release | Pins the exact Resource closure and component plan of a Piglet. | `release.version and managed records` | `ompig-slim piglet show --record <path>` | [piglets.md](piglets.md) |
| Piglet Binary | Target-native pig executable for one Piglet release; verifies its closure at startup and compiles out the built-ins its strip lists name. | `ompig-slim piglet build --format binary` | `ompig-slim verify` | [piglets.md](piglets.md) |
| Builder | Produces Piglet Binaries: native (needs Go and PiG source) or container (needs Docker or Podman). | `ompig-slim piglet build` | `ompig-slim setup` | [piglets.md](piglets.md) |
| Slot | A part of PiG that a Piglet can change: keep PiG's own part, replace it with a fused member (today the frontend), or strip it. | `slots: and strip: in the Piglet file` | `ompig-slim piglet show <name>` | [piglet-strip.md](piglet-strip.md) |
| Strip list | Disables built-in tools, commands, extensions, APIs, or features by stable ID; tools: is a ceiling, strip removes. | `strip: in the Piglet file` | `ompig-slim piglet schema, pig piglet show <name>` | [piglet-strip.md](piglet-strip.md) |
| Overlay Piglet | A Piglet chosen at launch on a Piglet Binary that is not built with build.closed; it can undo runtime strips but not compiled-out ones. | `--piglet, PIG_PIGLET_PATH, PIG_PIGLET_NAME` | `/piglet` | [piglets.md](piglets.md) |
| Divergence | A numbered, approved, user-visible difference from pinned Pi. | `divergences.md in this bundle` | `ompig-slim docs show divergences` | [divergences.md](divergences.md) |

| From | Relation | To | Note |
|---|---|---|---|
| PiG | implements | Pi | exact pinned release |
| Stock PiG | builds | PiG | one binary per target |
| Session | owns | Model Runtime | one per Session |
| Model Runtime | streams through | Provider | one Provider per request |
| Mode | drives | Session | TUI, print, JSON, RPC share one Session path |
| Package | distributes | Resource | one to many |
| Extension | is a | Resource |  |
| Extension | written with | Extension SDK | one language per extension |
| Extension | realized as | Extension realization | isolated, packed, or fused |
| Extension Host | hosts | Extension realization | same contract in every realization |
| Piglet | selects | Resource | and scopes discovery |
| PiG Standard | is a | Piglet | explicit, never active in Stock |
| Piglet release | pins | Piglet | exact closure |
| Piglet Binary | realizes | Piglet release | per target |
| Builder | produces | Piglet Binary | no silent fallback between builders |
| Piglet | declares | Strip list | tools is a ceiling; strip removes built-ins |
| Strip list | disables | Slot | by stable ID, in runtime or binary |
| Piglet | replaces | Slot | slots.frontend |
| Overlay Piglet | is a | Piglet | refused only by a Binary built with build.closed |
| Overlay Piglet | applies to | Piglet Binary | at launch |
| Divergence | documents difference from | Pi | numbered D<N> |

The machine-readable form is `pig-knowledge-graph.jsonld` on the documentation site.
