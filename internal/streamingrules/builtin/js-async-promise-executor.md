---
description: "An async Promise executor can lose errors. Use an async function or a synchronous executor."
condition: "\\bnew\\s+Promise(?:\\s*<[^>\\r\\n]{1,160}>)?\\s*\\(\\s*async\\b"
scope: "tool:edit(*.js), tool:write(*.js), tool:edit(*.jsx), tool:write(*.jsx), tool:edit(*.mjs), tool:write(*.mjs), tool:edit(*.cjs), tool:write(*.cjs), tool:edit(*.ts), tool:write(*.ts), tool:edit(*.tsx), tool:write(*.tsx), tool:edit(*.mts), tool:write(*.mts), tool:edit(*.cts), tool:write(*.cts)"
interruptMode: never
---

Do not pass an async executor to new Promise. Its returned Promise is ignored, so thrown errors do not reject the outer Promise. Return an async function's result directly, or use a synchronous executor only when adapting a callback API.

Source: https://eslint.org/docs/latest/rules/no-async-promise-executor
