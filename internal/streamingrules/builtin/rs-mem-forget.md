---
description: "mem::forget skips destruction and can leak resources. Check ownership transfer."
condition: "\\b(?:std::)?mem::forget\\s*(?:::\\s*<[^>\\r\\n]{1,80}>)?\\s*\\("
scope: "tool:edit(*.rs), tool:write(*.rs)"
interruptMode: never
---

Prefer ordinary ownership and Drop for resource cleanup. mem::forget is safe Rust but suppresses destruction and can leak memory or handles. FFI ownership transfer can require this behavior; preserve the external owner contract and consider ManuallyDrop where appropriate rather than freeing externally owned data.

Source: https://doc.rust-lang.org/std/mem/fn.forget.html
