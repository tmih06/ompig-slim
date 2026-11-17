---
description: "Array.forEach does not await async callbacks. Make completion and failure handling explicit."
condition: "\\.forEach\\s*\\(\\s*async\\b"
scope: "tool:edit(*.js), tool:write(*.js), tool:edit(*.jsx), tool:write(*.jsx), tool:edit(*.mjs), tool:write(*.mjs), tool:edit(*.cjs), tool:write(*.cjs), tool:edit(*.ts), tool:write(*.ts), tool:edit(*.tsx), tool:write(*.tsx), tool:edit(*.mts), tool:write(*.mts), tool:edit(*.cts), tool:write(*.cts)"
interruptMode: never
---

Array.forEach ignores returned Promises. Use for...of with await for sequential work, or explicit concurrency with completion and rejection handling. Deliberate detached work needs its own error and lifetime policy. Check whether a different API named forEach has different semantics.

Source: https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Array/forEach
