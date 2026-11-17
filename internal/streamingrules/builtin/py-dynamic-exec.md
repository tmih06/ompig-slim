---
description: "eval or exec runs Python code. Do not use it as a parser for untrusted data."
condition: "(?m)^[ \\t]*(?:[A-Za-z_]\\w*[ \\t]*=[ \\t]*)?(?:eval|exec)[ \\t]*\\("
scope: "tool:edit(*.py), tool:write(*.py)"
interruptMode: never
---

Use a data parser with explicit validation instead of evaluating external input. eval and exec are legitimate in trusted code-generation tools, but restricting a globals dictionary is not a security sandbox. If literal_eval fits a trusted bounded input format, remember that it also needs resource limits.

Source: https://docs.python.org/3/library/functions.html#exec
