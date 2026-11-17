---
description: "YAML object construction may be unsafe. Use a safe loader for external data."
condition: "\\byaml\\.load\\s*\\((?![^()]{0,600}\\b(?:SafeLoader|CSafeLoader)\\b)[^()]{0,600}\\)"
scope: "tool:edit(*.py), tool:write(*.py)"
interruptMode: never
---

For untrusted YAML, use yaml.safe_load or an explicit SafeLoader or CSafeLoader and validate the resulting shape. Generic or unsafe loaders may construct Python objects. A deliberately restricted custom loader or trusted object-serialization format needs its own review.

Source: https://pyyaml.org/wiki/PyYAMLDocumentation
