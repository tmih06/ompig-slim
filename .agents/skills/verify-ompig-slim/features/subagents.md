# Subagent identity and live thinking

Run `make build`, then run:

```sh
python3 scripts/smoke-test-subagent-ui.py ./bin/ompig-slim
```

The helper uses an isolated profile, a PTY, and a loopback SSE model. It keeps
the child running while it checks the following user-visible behavior:

- The main tree row shows the role, child name, task, and usage in that order.
- The Agent Hub role color matches the configured role color in the main view.
- Thinking text updates beneath the child row, in the hub inspector, and in
  the opened child transcript before the child completes.

The helper runs in fullscreen and regular modes. It releases the fixture
stream and terminates only its own process during cleanup. The check is part
of `make smoke`.
