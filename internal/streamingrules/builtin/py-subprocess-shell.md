---
description: "shell=True introduces shell interpretation. Check quoting and input trust."
condition: "\\bsubprocess\\.(?:run|Popen|call|check_call|check_output)\\s*\\([^)]{0,600}\\bshell\\s*=\\s*True\\b"
scope: "tool:edit(*.py), tool:write(*.py)"
interruptMode: never
---

Prefer an argument list without shell=True when no shell syntax is required. If a shell is necessary, do not interpolate untrusted values into the command. Verify platform-specific quoting, especially Windows batch files, before changing execution semantics. Controlled shell programs can be legitimate.

Source: https://docs.python.org/3/library/subprocess.html#security-considerations
