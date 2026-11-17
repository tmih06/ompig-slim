---
description: "Bare except catches interruption and exit signals as well as ordinary errors."
condition: "(?m)^[ \\t]*except[ \\t]*:"
scope: "tool:edit(*.py), tool:write(*.py)"
interruptMode: never
---

Catch the specific exception the operation is expected to raise. A bare except also intercepts KeyboardInterrupt and SystemExit. Cleanup that re-raises may be deliberate, but do not silently turn interruption or unexpected failure into success.

Source: https://docs.astral.sh/ruff/rules/bare-except/
