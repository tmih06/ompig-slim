---
description: "TypeScript any annotation or assertion. Use a concrete type or validate unknown input."
condition: '(?m)^(?![ \t]*(?://|/\*|\*(?:\s|/)))(?:(?!//)[^\n])*?(?:: |\bas )any(?![\w$.]|[ \t]*\?\.|[ \t]+(?!(?:as|satisfies|in|instanceof|extends)\b)[A-Za-z_$])'
scope: "tool:edit(*.ts), tool:edit(*.tsx), tool:write(*.ts), tool:write(*.tsx)"
interruptMode: never
---

Avoid : any and as any. Use unknown for unvalidated input, validate external data at the boundary, and use domain types or generics when the shape is known. Check whether this is code rather than a quoted example before changing it.
