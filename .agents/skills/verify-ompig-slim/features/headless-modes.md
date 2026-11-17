# Headless modes

## Sub-features

- One-shot text output with `-p`.
- JSONL events with `--mode json`.
- RPC request/response loops with `--mode rpc`.

## How to get to it (user POV)

Run `ompig-slim -p "..."` for a single answer, or select JSON/RPC when a
script needs machine-readable events.

## Driving it with the repository smoke harness

Use the repository's `scripts/smoke-test-*` scenarios and `make smoke-built`.
These launch the real binary with isolated profiles and faux/offline providers,
then inspect protocol output and exit status.

`scripts/smoke-test-streaming-rules.py` verifies code-pattern reminders and
pre-execution tool blocks in print mode with a loopback model. It checks the
next provider request, saved reminders, and the actual files written.
With `--tui`, it also checks distinct rule-poke cards and pattern summaries after
a forced redraw in both fullscreen and regular mode. The scenarios cover nested
codemode calls, display-only persistence, and rendering after session resume.
Catalog scenarios check new Go, JavaScript/TypeScript, Python, and Rust rules,
including batched pokes, repeated writes, and disabling all or selected built-ins.

## Gotchas

A provider/model fixture is required for deterministic no-network runs. Do not
replace the production mode with direct Go calls; protocol framing and process
lifecycle are part of this feature.
