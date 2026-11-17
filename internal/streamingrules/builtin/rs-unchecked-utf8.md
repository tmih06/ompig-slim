---
description: "Unchecked UTF-8 conversion requires valid bytes. Validate external data before creating strings."
condition: "\\b(?:str|String)::from_utf8_unchecked(?:_mut)?\\s*\\("
scope: "tool:edit(*.rs), tool:write(*.rs)"
interruptMode: never
---

Prefer str::from_utf8 or String::from_utf8 and handle invalid input. Unchecked conversion requires an independently established UTF-8 invariant. Validated internal buffers can justify skipping a repeated check, but do not assume external bytes satisfy the invariant.

Source: https://doc.rust-lang.org/std/str/fn.from_utf8_unchecked.html
