---
description: "Box::leak keeps an allocation forever. Prefer owned values or shared ownership."
condition: 'Box::leak'
scope: "tool:edit(*.rs), tool:write(*.rs)"
interruptMode: never
---

Do not use Box::leak to bypass ownership or lifetime constraints. Prefer owned values, Arc, LazyLock, or OnceLock. An intentional process-lifetime allocation needs a concrete reason.
