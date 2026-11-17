---
description: "Legacy math/rand import. Prefer math/rand/v2 for new code."
condition: '"math/rand"'
scope: "tool:edit(*.go), tool:write(*.go)"
interruptMode: never
---

Prefer math/rand/v2 for new code targeting Go 1.22 or later. Keep math/rand when compatibility requires its reproducible seeded sequence. Use crypto/rand for security-sensitive values.
