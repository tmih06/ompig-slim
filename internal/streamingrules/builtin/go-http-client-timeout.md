---
description: "An empty HTTP client has no client timeout. Verify request deadlines and cancellation."
condition: "\\bhttp\\.Client\\s*\\{\\s*\\}"
scope: "tool:edit(*.go), tool:write(*.go)"
interruptMode: never
---

An empty http.Client has a zero Timeout, which means no client-wide timeout. Verify that request contexts or later configuration supply the intended deadline. Reuse configured clients. Long-lived streams can intentionally use context cancellation instead of a short overall timeout.

Source: https://raw.githubusercontent.com/golang/go/master/src/net/http/client.go
