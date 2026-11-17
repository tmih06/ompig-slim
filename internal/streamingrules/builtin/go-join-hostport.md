---
description: "IPv6-unsafe address formatting. Use net.JoinHostPort."
condition: 'fmt\.Sprintf\("%s:%d"'
scope: "tool:edit(*.go), tool:write(*.go)"
interruptMode: never
---

Build network addresses with net.JoinHostPort(host, strconv.Itoa(port)). fmt.Sprintf("%s:%d", host, port) fails to bracket IPv6 literals.
