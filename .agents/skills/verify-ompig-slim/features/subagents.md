# Subagent identity and live thinking

Run `make build`, then run:

```sh
python3 scripts/smoke-test-subagent-ui.py ./bin/ompig-slim
```

The helper uses an isolated profile, a PTY, and a loopback SSE model. It keeps
the child running while it checks the following user-visible behavior:

- The footer counts the child (`N subagent · M working ↓`) while the tree
  stays hidden until Down on an empty prompt. The count returns to zero
  working when the child completes without reopening the tree.
- Down draws the tree below the footer; the row shows the role, child name,
  task, and usage in that order, and Enter on the highlighted row opens the
  child's focused transcript.
- The Agent Hub role color matches the configured role color in the main view.
- Thinking text updates beneath the child row, in the hub inspector, and in
  the opened child transcript before the child completes.
- History Up/Down restores the draft before the extra Down reopens the tree,
  and a nonempty prompt never opens it.
- Thirty seconds after the child finishes, the tree and footer count drop it.

The helper runs in fullscreen and regular modes. It releases the fixture
stream and terminates only its own process during cleanup. The check is part
of `make smoke`.
