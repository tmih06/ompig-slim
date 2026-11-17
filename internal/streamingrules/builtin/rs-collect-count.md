---
description: "Collecting a Vec only to count it allocates unnecessarily. Check whether Iterator.count preserves semantics."
condition: "\\.collect\\s*::\\s*<\\s*Vec\\s*<[^>\\r\\n]{1,80}>\\s*>\\s*\\(\\s*\\)\\s*\\.len\\s*\\(\\s*\\)"
scope: "tool:edit(*.rs), tool:write(*.rs)"
interruptMode: never
---

If only the count is needed, use Iterator.count rather than materializing a Vec and taking len. Preserve iterator side effects, destructor timing, ownership, and type inference. This regex cannot reproduce Clippy's type and mutation checks, so verify those conditions before replacing the collection.

Source: https://raw.githubusercontent.com/rust-lang/rust-clippy/master/clippy_lints/src/methods/needless_collect.rs
