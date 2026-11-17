---
description: "A TypeScript suppression hides compiler diagnostics. Fix the type boundary or justify a narrow exception."
condition: "(?m)^[ \\t]*(?://|/\\*+|\\*)[ \\t]*@ts-(?:ignore|nocheck)\\b"
scope: "tool:edit(*.ts), tool:write(*.ts), tool:edit(*.tsx), tool:write(*.tsx), tool:edit(*.mts), tool:write(*.mts), tool:edit(*.cts), tool:write(*.cts)"
interruptMode: never
---

Do not suppress real correctness errors with ts-ignore or ts-nocheck. Fix the model, narrow unknown values, or validate external input. A proven compiler or library typing defect can justify a narrow suppression with a concrete reason. Where supported, descriptive ts-expect-error makes stale exceptions visible. Generated files and version-compatibility cases need separate judgment.

Source: https://typescript-eslint.io/rules/ban-ts-comment/
