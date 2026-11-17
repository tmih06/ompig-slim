---
description: "mem::uninitialized is deprecated and invalid for nearly all types. Use a sound initialization design."
condition: "\\b(?:std::)?mem::uninitialized\\s*(?:::\\s*<[^>\\r\\n]{1,80}>)?\\s*\\("
scope: "tool:edit(*.rs), tool:write(*.rs)"
interruptMode: never
---

Do not create a typed value with mem::uninitialized. It causes immediate undefined behavior for nearly all types, even when unused. Prefer ordinary initialization. When delayed initialization is required, use MaybeUninit and prove every required field is initialized before assume_init.

Source: https://doc.rust-lang.org/std/mem/fn.uninitialized.html
