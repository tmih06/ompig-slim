---
description: "A JSON round trip is not a general deep clone. Check data loss and clone requirements."
condition: "\\bJSON\\.parse\\s*\\(\\s*JSON\\.stringify\\s*\\("
scope: "tool:edit(*.js), tool:write(*.js), tool:edit(*.jsx), tool:write(*.jsx), tool:edit(*.mjs), tool:write(*.mjs), tool:edit(*.cjs), tool:write(*.cjs), tool:edit(*.ts), tool:write(*.ts), tool:edit(*.tsx), tool:write(*.tsx), tool:edit(*.mts), tool:write(*.mts), tool:edit(*.cts), tool:write(*.cts)"
interruptMode: never
---

JSON round trips drop unsupported values, change Date values, and reject cycles. If a true deep copy is required, consider structuredClone when the runtime and value types support it, or use an explicit domain copy. Keep JSON round trips when JSON normalization is intentional. Neither alternative preserves every class or function semantics.

Source: https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/JSON/stringify
