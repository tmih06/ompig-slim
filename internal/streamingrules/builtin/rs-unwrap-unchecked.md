---
description: "Unchecked unwrap requires a proven valid variant. Prefer checked control flow at boundaries."
condition: "\\.unwrap_unchecked\\s*\\("
scope: "tool:edit(*.rs), tool:write(*.rs)"
interruptMode: never
---

An unchecked unwrap removes the variant check. Calling it on an invalid variant can cause undefined behavior. Prefer match, if let, or error propagation when validity is not independently established. An unsafe internal invariant needs concrete proof, not a performance guess.

Source: https://doc.rust-lang.org/std/option/enum.Option.html#method.unwrap_unchecked
