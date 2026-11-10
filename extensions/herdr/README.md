# Herdr adapter

Reports ompig-slim's agent state to [Herdr](https://herdr.dev), so a pane running
this harness gets a sidebar row, tab and workspace rollups, `herdr agent wait`, and
session restore after a Herdr server restart.

```sh
ompig-slim install ./extensions/herdr
```

Outside a Herdr pane the extension does nothing: it checks `HERDR_ENV`,
`HERDR_PANE_ID`, and `HERDR_BIN_PATH` before starting its poller.

## What it does

Herdr identifies an agent from the **process name** in the pane's foreground job,
against a closed list (`pi`, `omp`, `claude`, …). `ompig-slim` is not in that list,
so the pane looks like a plain shell. Herdr's documented integration point for
anything else is `pane report-agent`, which this extension calls.

It derives no state of its own. It polls the document the harness publishes —

```text
<PIG_HOME|~/.pi>/state/agent/status.json
```

— and forwards each change, so it cannot disagree with what the agent actually
reported. Any other consumer of that file needs no change here.

## Requirements

- `ompig-slim` on `PATH`, because Herdr types the resume command into a fresh
  shell and rejects a path. The curl installer satisfies this.
- Herdr 0.9.2 or later for session restore. Older versions ignore the resume
  command; state reporting still works.

## Notes

The `require github.com/MichaelKinsy/PiG/extensions/sdk v0.0.0` in `go.mod` is a
placeholder: PiG rewrites it to the SDK staged for the running version when it
builds the extension. `make test-extensions` runs this module's tests by adding a
`replace` to that staged copy and removing it afterwards.
