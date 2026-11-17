---
description: "TLS certificate verification is disabled. Restore verification or prove the custom verifier."
condition: "\\bInsecureSkipVerify\\s*(?::|=)\\s*true\\b"
scope: "tool:edit(*.go), tool:write(*.go)"
interruptMode: never
---

InsecureSkipVerify disables normal certificate-chain and hostname checks. Use valid trust roots and hostnames, or a complete custom verifier. Tests and sound VerifyConnection or VerifyPeerCertificate implementations can justify this setting; inspect that contract before changing it.

Source: https://raw.githubusercontent.com/golang/go/master/src/crypto/tls/common.go
