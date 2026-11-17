---
description: "Unpickling can execute code. Only deserialize trusted, integrity-protected pickle data."
condition: "\\bpickle\\.loads?\\s*\\("
scope: "tool:edit(*.py), tool:write(*.py)"
interruptMode: never
---

Never unpickle attacker-controlled or tampered data. Use a data-only format and validate its structure for external inputs. Trusted local persistence can legitimately use pickle, so inspect the trust boundary rather than assuming every load is exploitable.

Source: https://docs.python.org/3/library/pickle.html
