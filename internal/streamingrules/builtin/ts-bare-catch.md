---
description: "Underscore-prefixed catch binding. Use bare catch when the error is unused."
condition: 'catch \(_'
scope: "tool:edit(*.ts), tool:edit(*.tsx), tool:write(*.ts), tool:write(*.tsx)"
interruptMode: never
---

Use catch {} when the caught error is unused. Keep a named binding when the handler uses the error.
