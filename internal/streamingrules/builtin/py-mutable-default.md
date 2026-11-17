---
description: "A mutable default argument is shared between Python calls. Allocate per call when state should be fresh."
condition: "(?m)^[ \\t]*(?:async[ \\t]+)?def[ \\t]+\\w+[ \\t]*\\([^\\r\\n)]{0,300}=[ \\t]*(?:\\[[^\\]\\r\\n]{0,120}\\]|\\{[^}\\r\\n]{0,120}\\}|set\\([ \\t]*\\))"
scope: "tool:edit(*.py), tool:write(*.py)"
interruptMode: never
---

Default argument objects are created when the function is defined, not each time it runs. If callers need independent state, use an appropriate sentinel and allocate inside the function. Preserve API semantics. An intentional shared cache needs explicit ownership and can often use functools.lru_cache instead.

Source: https://docs.astral.sh/ruff/rules/mutable-argument-default/
