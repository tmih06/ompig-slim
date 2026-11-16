# Quit summary

## Behavior

Enter `/quit` in the interactive editor. After the terminal UI closes, the
terminal shows the startup logo and a session summary. The summary includes
message and tool-call counts, input tokens including cached tokens, cache writes,
and output tokens. Recorded cost appears when positive. Saved sessions include
a resume command. In-memory sessions still show usage without a resume command.

The summary uses the same session accounting as `/session`, including usage
already recorded in a resumed session. It does not estimate Copilot credits or
repository change totals. Redirected stdout does not receive the summary.

## Verification

```sh
make build
python3 scripts/smoke-test-quit.py ./bin/ompig-slim
```

The smoke test uses isolated temporary profiles and a loopback model. It drives
`/quit` in regular and fullscreen modes, checks known token and cost totals, and
asserts that the summary follows alternate-screen teardown. It also checks empty
and populated in-memory sessions and quoting of custom session directories.
The command runs as part of `make smoke`.
