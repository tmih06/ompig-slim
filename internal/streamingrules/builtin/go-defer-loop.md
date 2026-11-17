---
description: "Deferred cleanup inside a loop waits for the function to return. Check resource lifetime."
condition: "\\bfor\\b[^{\\r\\n]{0,160}\\{[^{}]{0,800}\\bdefer\\s+"
scope: "tool:edit(*.go), tool:write(*.go)"
interruptMode: never
---

A defer runs at function return, not at the end of the current iteration. For long-running loops, close each resource explicitly or put per-item work in a function that owns its defer. Keep delayed cleanup when it is intentional and bounded. Check nested functions before changing this match.

Source: https://raw.githubusercontent.com/dominikh/go-tools/master/staticcheck/sa9001/sa9001.go
