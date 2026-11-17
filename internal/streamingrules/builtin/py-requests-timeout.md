---
description: "A Requests call has no explicit timeout. Verify a bounded network wait."
condition: "\\brequests\\.(?:get|post|put|patch|delete|head|options)\\s*\\((?![^()\\r\\n]{0,600}(?:\\btimeout[ \\t]*=|\\*\\*))[^()\\r\\n]{0,600}\\)"
scope: "tool:edit(*.py), tool:write(*.py)"
interruptMode: never
---

Requests calls do not time out by default. Supply a suitable timeout or confirm an existing wrapper enforces the intended bound. The Requests timeout is a socket inactivity limit, not a total download deadline. Long-lived streams can have different requirements. Nested call expressions are not analyzed by this check.

Source: https://requests.readthedocs.io/en/latest/user/quickstart/#timeouts
