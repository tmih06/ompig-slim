# Get started with ompig-slim

ompig-slim includes the reference documentation required to configure models,
author extensions, use Packages, and compose Piglets. It registers no onboarding
extension.

On a first interactive start, ompig-slim shows a first-time setup before the session opens (D88). It appears when `OMPIG_SLIM_CODING_AGENT_DIR` is not set and the agent directory has no `settings.json` yet. It has three steps: a theme (System, Dark or Light, with a live preview), a sprite (the mascot `/sprite` previews) and a choice to share anonymous usage data. Finishing saves the answers to `settings.json` and to the sprite state, so the setup does not return. `Esc` skips it and saves no answer. The setup still does not return: the same start writes `settings.json` (with `lastChangelogVersion`), and the setup appears only while that file is missing. Print, JSON and RPC modes never show it. Pi 1.5 shows its own two-step version (theme and analytics) only for its official distribution with `PI_EXPERIMENTAL=1` (`startup-ui.ts` `shouldRunFirstTimeSetup`), so ompig-slim shows it more often than Pi does. Change the theme later with `/settings`, the sprite with `/sprite`, and the usage-data choice by setting `enableAnalytics` in `settings.json`.

Ask the running agent a direct setup question when a model is available. Tell it
to read the matching local documentation before it changes files.

## Set up a project Piglet

Use this prompt when you want ompig-slim to create a project-specific composition:

```text
Read the local ompig-slim documentation. Set up a Piglet for this repository. Explain
each selected model preference, system prompt, built-in tool, skill, and
extension. Write the Piglet source, validate it with pig piglet validate, and
show the exact pig --piglet command.
```

The expected result is a Piglet source under `~/.pi/piglets/`, a project-local
`.pi/piglets/` directory, or another explicit path.

## Scaffold an extension

```bash
ompig-slim extension init ./my-extension --lang go
ompig-slim install ./my-extension --validate-only --json
ompig-slim -e ./my-extension
```

Edit the extension and use `/reload` in the session. Read `extensions.md` for
the source forms. Read `extension-api.md` for the host API.

## Compose a Piglet

A Piglet selects extensions, skills, built-ins and a model for one named agent.
Write one with the prompt above, then run it by name or path:

```bash
ompig-slim --piglet ./my-piglet.yaml
```

Build a directly executable composition when you do not want the target host to
compile compatible Go or Rust extension sources:

```bash
ompig-slim piglet build ./my-piglet.yaml --format binary --out ./my-agent
```

## Related documents

- [Concepts](concepts.md)
- [Extensions](extensions.md)
- [Extension API](extension-api.md)
- [Packages](packages.md)
- [Piglets](piglets.md)
- [Built-in extensions](built-in-extensions.md)
