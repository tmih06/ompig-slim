---
description: "SQL is built with formatting at execution. Bind data as query parameters."
condition: "\\.\\s*(?:Query(?:Row)?|Exec)(?:Context)?\\s*\\(\\s*(?:[A-Za-z_]\\w*\\s*,\\s*)?fmt\\.Sprintf\\s*\\("
scope: "tool:edit(*.go), tool:write(*.go)"
interruptMode: never
---

If this method executes SQL, pass data separately using the driver's parameter placeholders instead of formatting it into the SQL text. Identifiers cannot generally be bound as values; validate an allowlist or use the database's identifier-quoting API when dynamic identifiers are required. No input trust or receiver type is proven by this match.

Source: https://go.dev/doc/database/sql-injection
