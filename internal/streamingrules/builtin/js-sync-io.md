---
description: "Synchronous I/O or heavy work blocks the Node event loop. Check the execution context."
condition: "\\b(?:readFileSync|writeFileSync|appendFileSync|readdirSync|statSync|execSync|execFileSync|spawnSync|pbkdf2Sync|scryptSync|gzipSync|gunzipSync)\\s*\\("
scope: "tool:edit(*.js), tool:write(*.js), tool:edit(*.jsx), tool:write(*.jsx), tool:edit(*.mjs), tool:write(*.mjs), tool:edit(*.cjs), tool:write(*.cjs), tool:edit(*.ts), tool:write(*.ts), tool:edit(*.tsx), tool:write(*.tsx), tool:edit(*.mts), tool:write(*.mts), tool:edit(*.cts), tool:write(*.cts)"
interruptMode: never
---

Avoid these synchronous filesystem, subprocess, compression, or crypto APIs in Node server request paths. Prefer asynchronous APIs or suitable worker execution. Startup, build scripts, short CLI programs, and isolated workers can intentionally use synchronous calls. Check actual workload before claiming a performance improvement.

Source: https://nodejs.org/en/learn/asynchronous-work/dont-block-the-event-loop
