---
description: "Deprecated io/ioutil import. Use io and os."
condition: '"io/ioutil"'
scope: "tool:edit(*.go), tool:write(*.go)"
interruptMode: never
---

Use io and os instead of the deprecated io/ioutil package. Replace ioutil.ReadFile and WriteFile with os.ReadFile and os.WriteFile. Use io.ReadAll and io.NopCloser. os.ReadDir returns directory entries, not FileInfo values.
