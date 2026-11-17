---
description: "Regex work inside a loop may recompile a stable pattern. Compile once and reuse it."
condition: "\\bfor\\b[^{\\r\\n]{0,160}\\{[^{}]{0,800}\\bregexp\\.(?:MustCompile|Compile|Match(?:String|Reader)?)\\s*\\("
scope: "tool:edit(*.go), tool:write(*.go)"
interruptMode: never
---

Check whether this regex pattern is stable across iterations. Package-level regexp.Match functions compile on every call. Move stable compilation outside a hot loop and reuse the Regexp. Keep per-iteration compilation when the pattern genuinely changes. This regex check does not resolve loop or function boundaries.

Source: https://raw.githubusercontent.com/dominikh/go-tools/master/staticcheck/sa6000/sa6000.go
