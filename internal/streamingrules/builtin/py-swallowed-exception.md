---
description: "A broad exception handler discards failures. Preserve or handle the actual error."
condition: "(?m)^[ \\t]*except[ \\t]+(?:Exception|BaseException)(?:[ \\t]+as[ \\t]+\\w+)?[ \\t]*:\\s*pass\\b"
scope: "tool:edit(*.py), tool:write(*.py)"
interruptMode: never
---

Catching Exception or BaseException and immediately passing can hide corrupt input, partial writes, and programming errors. Handle a specific expected exception, propagate unexpected failures, or record actionable context when continuing is safe. Do not replace this with a fake successful fallback.

Source: https://docs.astral.sh/ruff/rules/try-except-pass/
