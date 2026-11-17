---
description: "An empty catch hides a failure. Handle it or document intentional suppression."
condition: "\\bcatch(?:\\s*\\([^\\r\\n)]{0,100}\\))?\\s*\\{\\s*\\}"
scope: "tool:edit(*.js), tool:write(*.js), tool:edit(*.jsx), tool:write(*.jsx), tool:edit(*.mjs), tool:write(*.mjs), tool:edit(*.cjs), tool:write(*.cjs), tool:edit(*.ts), tool:write(*.ts), tool:edit(*.tsx), tool:write(*.tsx), tool:edit(*.mts), tool:write(*.mts), tool:edit(*.cts), tool:write(*.cts)"
interruptMode: never
---

Inspect the failure being suppressed. Handle or propagate unexpected errors and preserve useful context. If an expected failure is intentionally ignored, add a concrete explanation inside the catch. Do not add a fake fallback or log and continue when the operation must succeed.

Source: https://raw.githubusercontent.com/eslint/eslint/main/docs/src/rules/no-empty.md
