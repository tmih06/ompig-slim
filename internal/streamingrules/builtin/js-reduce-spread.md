---
description: "Spreading a growing reduce accumulator repeatedly can cause quadratic copying."
condition: "\\.reduce\\s*\\(\\s*\\(\\s*([A-Za-z_$][\\w$]*)\\s*,[^)]{0,120}\\)\\s*=>\\s*\\(?\\s*[\\[{]\\s*\\.\\.\\.\\s*\\1\\b"
scope: "tool:edit(*.js), tool:write(*.js), tool:edit(*.jsx), tool:write(*.jsx), tool:edit(*.mjs), tool:write(*.mjs), tool:edit(*.cjs), tool:write(*.cjs), tool:edit(*.ts), tool:write(*.ts), tool:edit(*.tsx), tool:write(*.tsx), tool:edit(*.mts), tool:write(*.mts), tool:edit(*.cts), tool:write(*.cts)"
interruptMode: never
---

This reducer copies its accumulated object or array for each item. For large or hot inputs, build an operation-owned accumulator without copying it each time, or use an appropriate grouping or flattening API. Do not mutate shared input. Small bounded collections can be fine; verify workload and semantics.

Source: https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Array/reduce#when_to_not_use_reduce
