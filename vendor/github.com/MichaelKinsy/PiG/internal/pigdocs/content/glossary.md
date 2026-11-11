# Glossary

This page defines the terms ompig-slim documentation uses. Each term has its definition, where you meet it, and the page that explains it.

## Pi

Pi is the upstream TypeScript project at <https://github.com/earendil-works/pi>. Pi is the reference implementation for parity-bound behavior.

Where you meet it: The upstream project. Docs for Pi behavior that ompig-slim keeps cite it.

Read more: [Overview](https://pi-in-go.dev/docs/latest/)

## ompig-slim

ompig-slim is the Go implementation built from this repository. The executable is
`ompig-slim`. It follows the exact Pi build pinned in `.upstream/pin` (a release
tag, or a pinned commit). It contains Pi-compatible behavior, the generic
extension and Piglet infrastructure, and the reference documentation required to
use those APIs. It activates no optional extension and no implicit composition.

Where you meet it: The `ompig-slim` executable, the plain binary you install, and
every page of this documentation.

Read more: [How ompig-slim works](how-pig-works.md) and
[ompig-slim concepts](concepts.md)

## Resource

A Resource contributes one extension, skill, prompt, hook, MCP definition, theme, compatible agent declaration, context input, or agent environment.

Where you meet it: `ompig-slim piglet show` lists the Resources a Piglet selects. Packages carry them.

Read more: [ompig-slim concepts](concepts.md)

## Package

A Package distributes a versioned collection of Resources. Installing a Package makes its Resources available through normal discovery. A Package does not own or activate a Piglet.

Where you meet it: `ompig-slim install` and `ompig-slim list`.

Read more: [Packages](packages.md)

## Plugin

A Plugin packages capabilities for more than one compatible agent harness through an Agent Plugin or another supported vendor format.

Where you meet it: A package for other agent harnesses that ompig-slim can read.

Read more: [Packages](packages.md)

## Piglet

A Piglet defines one named agent. It selects and scopes Resources and declares defaults and environment requirements.

Where you meet it: `ompig-slim piglet` commands and the `--piglet` flag.

Read more: [Piglets](piglets.md)

## Piglet release

A Piglet release pins the exact Resource closure and executable component plan for a Piglet.

Where you meet it: The record a Piglet build writes. `ompig-slim piglet verify` checks it.

Read more: [Piglet lifecycle](piglet-lifecycle.md)

## Piglet Binary

A Piglet Binary is a target-native ompig-slim executable built for one immutable Piglet composition. It can contain fused Go extensions and prebuilt Go or Rust extension cells. External services, configuration, secrets, and interpreted runtimes remain external requirements unless the release states otherwise.

Where you meet it: `ompig-slim piglet build --format binary` writes one.

Read more: [Piglet Binaries](piglets.md#piglet-binary)

## Piglet Image

A Piglet Image is the planned OCI materialization of a Piglet and its runtime environment. It is not in this release. No ompig-slim command produces one, and `ompig-slim piglet build --format image` fails.

Where you meet it: Planned. `ompig-slim piglet build --format image` fails today.

Read more: [Piglet lifecycle](piglet-lifecycle.md)

## Composition

A Composition is a Piglet you select to add optional product extensions to a bare
ompig-slim. This repository ships none: every Composition is one you write or
install, and it stays explicit until you pass `--piglet`. A Package can
distribute a Composition's Resources but cannot own or activate it.

Where you meet it: `ompig-slim --piglet ./my-piglet.yaml`, or an installed
Piglet named with `--piglet <name>`.

Read more: [Piglets](piglets.md)

## Divergence

A divergence is an intentional user-visible or interoperability difference from the pinned Pi behavior. `docs/parity/DIVERGENCES.md` records each approved divergence. Language mechanics and implementation details that do not change observable behavior are not divergences.

Where you meet it: The "Differs from Pi" boxes and the numbered D entries.

Read more: [Divergences](https://github.com/MichaelKinsy/PiG/blob/main/docs/parity/DIVERGENCES.md)

## Evidence

Evidence is a reproducible artifact that supports a project claim. Scanner output is evidence input. It is not a validated inventory, license conclusion, vulnerability disposition, or compliance approval by itself.

Where you meet it: The quality and release records of the repository. A user does not need it.

Read more: [Repository quality](https://github.com/MichaelKinsy/PiG/blob/main/docs/project/repository-quality.md)

## Transcript

A Transcript is normalized, ordered model context. It carries typed system, user, assistant, and tool-result entries in their original order.

Where you meet it: The context ompig-slim sends to a model, in the Session file and in the Go SDK.

Read more: [Message types](message-types.md)

## Provider

A Provider consumes a normalized Transcript and produces one Event Stream.

Where you meet it: The model service behind a model name.

Read more: [Custom providers](custom-provider.md)

## Event Stream

An Event Stream yields ordered model events and one terminal assistant result. The caller context owns cancellation.

Where you meet it: The ordered events a Provider returns. This is not the JSON mode event stream, which is the CLI output.

Read more: [JSON event stream mode](json.md)

## Model Runtime

A Model Runtime owns mode-independent model lookup, authentication, completion, and streaming. TUI, print, JSON, and RPC use the same Model Runtime.

Where you meet it: The one model path that the TUI, print, JSON and RPC modes share.

Read more: [How ompig-slim works](how-pig-works.md)

## Main Screen

The Main Screen renders transcript and active input into the visible terminal region. Ordinary input changes update only the required visible rows and preserve terminal scrollback unless Pi requires a recovery redraw.

Where you meet it: The terminal area that shows the conversation. Scrollback stays intact.

Read more: [TUI components](tui.md)

## Extension Host

The Extension Host realizes extension Resources as isolated, packed, or fused components. External components retain their own declared protocol and lifecycle.

Where you meet it: The process model behind extensions: isolated, packed, or fused.

Read more: [Extensions](extensions.md)
