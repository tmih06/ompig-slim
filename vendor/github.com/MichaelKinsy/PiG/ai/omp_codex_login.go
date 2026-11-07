package ai

// ompig-slim addition: token profile semantics from
// packages/ai/src/registry/oauth/openai-codex.ts in OMP. Workspace-less free
// accounts identify by email; ID tokens are read for claims and never persisted.

import (
	"encoding/json"
	"strings"
)

// codexRequestAuth carries an explicit workspace when available, including the
// ID-token fallback. An email-only free account sends only its bearer token.
func codexRequestAuth(c Credential) ModelAuth {
	auth := ModelAuth{APIKey: c.Access}
	if c.AccountID != "" {
		auth.Headers = ProviderHeaders{"ChatGPT-Account-Id": new(c.AccountID)}
	}
	return auth
}

// codexJWTProfile decodes metadata without verifying token signatures. Both
// standard and URL-safe base64 payloads are accepted like OMP's Buffer decoder.
// Invalid JSON or JWT shape yields no claims, never a fabricated identity.
func codexJWTProfile(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := decodeJWTPayload(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return nil
	}
	return claims
}

// codexClaimString reads one typed string from a nested claims object. Missing,
// null and mistyped members are absent rather than printed or coerced.
func codexClaimString(claims map[string]any, namespace, key string) string {
	nested, _ := claims[namespace].(map[string]any)
	value, _ := nested[key].(string)
	return value
}

// enrichCodexProfile extracts account/email/plan from access then ID-token
// claims. A login records the workspace and plan for OMP-compatible identity.
// It does not validate identity: refresh responses may legitimately omit claims.
func enrichCodexProfile(c OAuthCredentials, idToken string, login bool) OAuthCredentials {
	access, id := codexJWTProfile(c.Access), codexJWTProfile(idToken)
	account := firstNonEmptyClaim(codexClaimString(access, codexJWTClaimPath, "chatgpt_account_id"), codexClaimString(id, codexJWTClaimPath, "chatgpt_account_id"))
	email := strings.ToLower(strings.TrimSpace(firstNonEmptyClaim(codexClaimString(access, codexJWTProfileClaim, "email"), codexClaimString(id, codexJWTProfileClaim, "email"))))
	plan := strings.ToLower(strings.TrimSpace(firstNonEmptyClaim(codexClaimString(access, codexJWTClaimPath, "chatgpt_plan_type"), codexClaimString(id, codexJWTClaimPath, "chatgpt_plan_type"))))
	if account != "" {
		c.AccountID = account
	}
	if c.Extra == nil {
		c.Extra = map[string]json.RawMessage{}
	}
	if email != "" {
		c.Extra["email"], _ = json.Marshal(email)
	}
	if login && account != "" {
		c.Extra["orgId"], _ = json.Marshal(account)
	}
	if login && plan != "" {
		c.Extra["orgName"], _ = json.Marshal(plan)
	}
	return c
}

// codexProfileString reads stored non-secret metadata; malformed values are absent.
func codexProfileString(c OAuthCredentials, key string) string {
	var value string
	_ = json.Unmarshal(c.Extra[key], &value)
	return value
}

// preserveCodexProfile keeps the selected workspace/organization stable across
// refreshes and fills an omitted email/account from stored metadata. Clone the
// extras so the refreshed credential never mutates its caller's owned maps.
func preserveCodexProfile(refreshed, stored OAuthCredentials) OAuthCredentials {
	refreshed.Extra = cloneCredentialExtra(refreshed.Extra)
	if refreshed.Extra == nil {
		refreshed.Extra = map[string]json.RawMessage{}
	}
	if refreshed.AccountID == "" {
		refreshed.AccountID = stored.AccountID
	}
	for _, key := range []string{"email", "orgId", "orgName"} {
		old := codexProfileString(stored, key)
		if old != "" && (key != "email" || codexProfileString(refreshed, key) == "") {
			refreshed.Extra[key], _ = json.Marshal(old)
		}
	}
	return refreshed
}
