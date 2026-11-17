---
description: "Register WaitGroup work before launching its goroutine, not inside it."
condition: "\\bgo\\s+func\\s*\\([^)]{0,160}\\)\\s*\\{\\s*[A-Za-z_]\\w*\\.Add\\s*\\("
scope: "tool:edit(*.go), tool:write(*.go)"
interruptMode: never
---

If this receiver is a sync.WaitGroup, call Add before starting the goroutine so Wait cannot run before registration. On supported Go versions, WaitGroup.Go can own that lifecycle. A different type with an Add method can be valid; verify the receiver rather than rewriting blindly.

Source: https://raw.githubusercontent.com/dominikh/go-tools/master/staticcheck/sa2000/sa2000.go
