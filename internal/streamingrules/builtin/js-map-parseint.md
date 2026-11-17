---
description: "map passes its index as the parseInt radix. Use an explicit callback and radix."
condition: "\\.map\\s*\\(\\s*(?:Number\\.)?parseInt\\s*\\)"
scope: "tool:edit(*.js), tool:write(*.js), tool:edit(*.jsx), tool:write(*.jsx), tool:edit(*.mjs), tool:write(*.mjs), tool:edit(*.cjs), tool:write(*.cjs), tool:edit(*.ts), tool:write(*.ts), tool:edit(*.tsx), tool:write(*.tsx), tool:edit(*.mts), tool:write(*.mts), tool:edit(*.cts), tool:write(*.cts)"
interruptMode: never
---

Array.map passes value, index, and array to its callback. parseInt treats the index as a radix. Use value => parseInt(value, 10), or Number only when its full numeric-conversion semantics are intended.

Source: https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Array/map#using_parseint_with_map
