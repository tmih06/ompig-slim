---
description: "A JSON decode error is discarded. Handle invalid input before using the result."
condition: "\\b_\\s*=\\s*json\\.Unmarshal\\s*\\("
scope: "tool:edit(*.go), tool:write(*.go)"
interruptMode: never
---

json.Unmarshal can report syntax or type errors and leave partially decoded data. Handle the error before consuming the result. Deliberate best-effort parsing of trusted data needs a clear reason; do not invent a successful default for malformed external input.

Source: https://raw.githubusercontent.com/golang/go/master/src/encoding/json/decode.go
