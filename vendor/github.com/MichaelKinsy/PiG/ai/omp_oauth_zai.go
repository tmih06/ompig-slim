package ai

// Native port of OMP's Z.ai (GLM Coding Plan) sign-in
// (packages/ai/src/auth/oauth/zai.ts plus its auth "zai-coding-plan" policy and
// registry/oauth/zai.ts:zaiMintKeyHook).
//
// The flow ends in a durable `id.secret` API key rather than a refreshable token
// pair: an authorization code (no PKCE, `zcode://` redirect because Z.ai rejects
// loopback redirect URIs for this client) is exchanged for a short-lived OAuth
// access token, which the business APIs then trade for such a key. Nothing here
// refreshes: the stored key does not expire.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

const (
	zaiOAuthDefaultClientID    = "client_P8X5CMWmlaRO9gyO-KSqtg" // gitleaks:allow -- public OAuth client ID
	zaiOAuthDefaultAuthorize   = "https://chat.z.ai/api/oauth/authorize"
	zaiOAuthDefaultToken       = "https://zcode.z.ai/api/v1/oauth/token"
	zaiOAuthDefaultRedirectURI = "zcode://zai-auth/callback"
	zaiOAuthDefaultBizBase     = "https://api.z.ai"
	// zaiOAuthKeyName is this build's key name on the Z.ai account. It is not ZCode's
	// `zcode-api-key`, so sign-in never mutates the desktop app's key.
	zaiOAuthKeyName = "oh-my-pi"
	zaiOAuthName    = "Z.AI (GLM Coding Plan · Sign in)"
	zaiOAuthLabel   = "Sign in with Z.AI"
)

// zaiOAuthEnv reads a ZAI_OAUTH_* override, treating an empty value as absent, as
// upstream's env() helper does.
func zaiOAuthEnv(name string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return ""
}

// ParseZaiAuthorizationInput turns a pasted redirect URL or a bare code into the
// authorization code.
//
// The callback uses a custom scheme no browser can load, so the user copies the
// redirect URL out of the address bar; a shell may also leave part of the query
// attached to a bare paste.
func ParseZaiAuthorizationInput(input string) string {
	text := trimJSWhitespace(input)
	if text == "" {
		return ""
	}
	if parsed, err := url.Parse(text); err == nil && parsed.Scheme != "" {
		// A URL carries the code in `code`, or `authCode` on some deployments; a URL with
		// neither is not a code, as upstream's undefined return says.
		query := parsed.Query()
		for _, name := range []string{"code", "authCode"} {
			if value := strings.TrimSpace(query.Get(name)); value != "" {
				return value
			}
		}
		return ""
	}
	// A bare code, possibly with `&state=…` or `#state` still attached.
	code, _, _ := strings.Cut(text, "&")
	code, _, _ = strings.Cut(code, "#")
	return strings.TrimSpace(code)
}

// zaiOAuthEnvelope reads Z.ai's `{ code, msg, data, success }` envelope. The OAuth
// token endpoint signals success with `code: 0`; the business APIs use `code: 200`
// or `success: true`. Bodies without a status wrapper pass through unchanged.
func zaiOAuthIsSuccessCode(value any) bool {
	switch code := value.(type) {
	case nil:
		return true
	case json.Number:
		return code.String() == "0" || code.String() == "200"
	case float64:
		return code == 0 || code == 200
	case string:
		return code == "0" || code == "200"
	}
	return false
}

// zaiOAuthUnwrap returns the envelope's `data` member, or the body itself when it
// has no status wrapper. A failed envelope returns its `msg`.
func zaiOAuthUnwrap(body map[string]any, operation string) (any, error) {
	if body == nil {
		return nil, nil
	}
	_, hasCode := body["code"]
	_, hasSuccess := body["success"]
	if !hasCode && !hasSuccess {
		return body, nil
	}
	failed := false
	if success, ok := body["success"].(bool); ok && !success {
		failed = true
	}
	if !zaiOAuthIsSuccessCode(body["code"]) {
		failed = true
	}
	if failed {
		detail := ompOAuthString(body["msg"])
		if detail == "" {
			detail = "code " + fmt.Sprint(body["code"])
		}
		return nil, fmt.Errorf("Z.ai %s failed: %s", operation, detail)
	}
	if data, ok := body["data"]; ok {
		return data, nil
	}
	return body, nil
}

// zaiOAuthGetJSON performs one authenticated GET and decodes a JSON object body.
func zaiOAuthGetJSON(ctx context.Context, endpoint string, headers map[string]string) (map[string]any, error) {
	status, payload, _, err := ompOAuthRequest(ctx, "GET", endpoint, headers, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("Z.ai request failed: status=%d url=%s body=%s", status, endpoint, fetchBodyText(payload))
	}
	return ompOAuthObject(payload), nil
}

// zaiOAuthPostJSON performs one authenticated JSON POST and decodes a JSON object
// body.
func zaiOAuthPostJSON(ctx context.Context, endpoint string, body map[string]any, headers map[string]string) (map[string]any, error) {
	encoded, err := ompOAuthJSONBody(body)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]string, len(headers)+1)
	for name, value := range headers {
		merged[name] = value
	}
	merged["Content-Type"] = "application/json"
	status, payload, _, err := ompOAuthRequest(ctx, "POST", endpoint, merged, encoded)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("Z.ai request failed: status=%d url=%s body=%s", status, endpoint, fetchBodyText(payload))
	}
	return ompOAuthObject(payload), nil
}

// zaiOAuthKeyArray coerces an api-keys list response (a bare array or one of the
// common wrapper shapes) to a slice of key records.
func zaiOAuthKeyArray(value any) []map[string]any {
	if list, ok := value.([]any); ok {
		out := make([]map[string]any, 0, len(list))
		for _, entry := range list {
			if record, ok := entry.(map[string]any); ok {
				out = append(out, record)
			}
		}
		return out
	}
	if record, ok := value.(map[string]any); ok {
		for _, field := range []string{"list", "keys", "apiKeys", "records"} {
			if list, ok := record[field].([]any); ok {
				return zaiOAuthKeyArray(list)
			}
		}
	}
	return nil
}

// zaiOAuthRecord reads a JSON object value, or nil.
func zaiOAuthRecord(value any) map[string]any {
	record, _ := value.(map[string]any)
	return record
}

// zaiOAuthTrimmed reads a non-blank string member.
func zaiOAuthTrimmed(value any) string {
	return strings.TrimSpace(ompOAuthString(value))
}

// zaiOAuthBusinessLogin exchanges the short-lived OAuth access token for a durable
// business token over ZCode's business-login endpoint. The business APIs reject
// the raw OAuth token.
func zaiOAuthBusinessLogin(ctx context.Context, oauthAccessToken, businessLoginURL string) (string, error) {
	body, err := zaiOAuthPostJSON(ctx, businessLoginURL, map[string]any{"token": oauthAccessToken}, nil)
	if err != nil {
		return "", err
	}
	value, err := zaiOAuthUnwrap(body, "business login")
	if err != nil {
		return "", err
	}
	data := zaiOAuthRecord(value)
	token := zaiOAuthTrimmed(data["access_token"])
	if token == "" {
		token = zaiOAuthTrimmed(data["accessToken"])
	}
	if token == "" {
		return "", errors.New("Z.ai business login returned no access token")
	}
	return token, nil
}

// zaiOAuthMintAPIKey provisions the durable Z.ai API key from a short-lived OAuth
// access token, mirroring ZCode: business login, resolve the default organization
// and project from getCustomerInfo, then find or create this build's key and read
// its secret from the copy endpoint.
//
// The secret is always read through copy: list entries mask it (`*****abcd`) and
// the create response's inline secret is not reliable across account states,
// whereas copy always returns the full value.
func zaiOAuthMintAPIKey(ctx context.Context, oauthAccessToken, bizBase, businessLoginURL string) (string, error) {
	bizToken, err := zaiOAuthBusinessLogin(ctx, oauthAccessToken, businessLoginURL)
	if err != nil {
		return "", err
	}
	auth := map[string]string{"Authorization": "Bearer " + bizToken}

	customerBody, err := zaiOAuthGetJSON(ctx, bizBase+"/api/biz/customer/getCustomerInfo", auth)
	if err != nil {
		return "", err
	}
	customerValue, err := zaiOAuthUnwrap(customerBody, "customer lookup")
	if err != nil {
		return "", err
	}
	customer := zaiOAuthRecord(customerValue)
	organizations, _ := customer["organizations"].([]any)
	var organization map[string]any
	for _, entry := range organizations {
		record := zaiOAuthRecord(entry)
		if record == nil {
			continue
		}
		if isDefault, _ := record["isDefault"].(bool); organization == nil || isDefault {
			organization = record
			if isDefault {
				break
			}
		}
	}
	if organization == nil {
		return "", errors.New("Z.ai key provisioning failed: no organization/project on account")
	}
	projects, _ := organization["projects"].([]any)
	var project map[string]any
	for _, entry := range projects {
		record := zaiOAuthRecord(entry)
		if record == nil {
			continue
		}
		if isDefault, _ := record["isDefault"].(bool); project == nil || isDefault {
			project = record
			if isDefault {
				break
			}
		}
	}
	organizationID := zaiOAuthTrimmed(organization["organizationId"])
	projectID := zaiOAuthTrimmed(project["projectId"])
	if organizationID == "" || projectID == "" {
		return "", errors.New("Z.ai key provisioning failed: no organization/project on account")
	}

	keysURL := fmt.Sprintf("%s/api/biz/v1/organization/%s/projects/%s/api_keys", bizBase, organizationID, projectID)
	listBody, err := zaiOAuthGetJSON(ctx, keysURL, auth)
	if err != nil {
		return "", err
	}
	listValue, err := zaiOAuthUnwrap(listBody, "api key list")
	if err != nil {
		return "", err
	}
	var keyRecord map[string]any
	for _, key := range zaiOAuthKeyArray(listValue) {
		if zaiOAuthTrimmed(key["name"]) == zaiOAuthKeyName {
			keyRecord = key
			break
		}
	}
	if keyRecord == nil {
		createBody, err := zaiOAuthPostJSON(ctx, keysURL, map[string]any{"name": zaiOAuthKeyName}, auth)
		if err != nil {
			return "", err
		}
		createValue, err := zaiOAuthUnwrap(createBody, "api key create")
		if err != nil {
			return "", err
		}
		keyRecord = zaiOAuthRecord(createValue)
	}
	apiKey := zaiOAuthTrimmed(keyRecord["apiKey"])
	if apiKey == "" {
		return "", errors.New("Z.ai key provisioning returned no apiKey")
	}

	copyBody, err := zaiOAuthGetJSON(ctx, keysURL+"/copy/"+url.PathEscape(apiKey), auth)
	if err != nil {
		return "", err
	}
	copyValue, err := zaiOAuthUnwrap(copyBody, "api key copy")
	if err != nil {
		return "", err
	}
	secretKey := zaiOAuthTrimmed(zaiOAuthRecord(copyValue)["secretKey"])
	if secretKey == "" {
		return "", errors.New("Z.ai key provisioning returned no secretKey")
	}
	return apiKey + "." + secretKey, nil
}

// zaiOAuthProvider is the registered Z.ai sign-in flow.
type zaiOAuthProvider struct{}

func (zaiOAuthProvider) ID() string               { return "zai" }
func (zaiOAuthProvider) Name() string             { return zaiOAuthName }
func (zaiOAuthProvider) IsSubscription() bool     { return true }
func (zaiOAuthProvider) UsesCallbackServer() bool { return false }
func (zaiOAuthProvider) LoginLabel() string       { return zaiOAuthLabel }

// GetAPIKey is the stored credential's access member: the minted durable key.
func (zaiOAuthProvider) GetAPIKey(creds OAuthCredentials) string { return creds.Access }

func (p zaiOAuthProvider) Login(callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return p.LoginContext(context.Background(), callbacks)
}

// LoginContext runs the Z.ai authorization-code flow and the key mint.
func (p zaiOAuthProvider) LoginContext(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	clientID := firstNonEmptyString(zaiOAuthEnv("ZAI_OAUTH_CLIENT_ID"), zaiOAuthDefaultClientID)
	authorizeURL := firstNonEmptyString(zaiOAuthEnv("ZAI_OAUTH_AUTHORIZE_URL"), zaiOAuthDefaultAuthorize)
	tokenURL := firstNonEmptyString(zaiOAuthEnv("ZAI_OAUTH_TOKEN_URL"), zaiOAuthDefaultToken)
	redirectURI := firstNonEmptyString(zaiOAuthEnv("ZAI_OAUTH_REDIRECT_URI"), zaiOAuthDefaultRedirectURI)
	bizBase := strings.TrimRight(firstNonEmptyString(os.Getenv("ZAI_BIZ_BASE"), zaiOAuthDefaultBizBase), "/")
	businessLoginURL := firstNonEmptyString(os.Getenv("ZAI_BUSINESS_LOGIN_URL"), bizBase+"/api/auth/z/login")

	state, err := ompOAuthRandomHex(16)
	if err != nil {
		return OAuthCredentials{}, err
	}
	authorize, err := url.Parse(authorizeURL)
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("Z.ai authorize URL: %w", err)
	}
	query := url.Values{}
	query.Set("client_id", clientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("state", state)
	authorize.RawQuery = query.Encode()
	if callbacks.OnAuth != nil {
		callbacks.OnAuth(OAuthAuthInfo{
			URL: authorize.String(),
			Instructions: "Complete the Z.ai sign-in in your browser, then paste the final redirect URL " +
				"(it starts with zcode://) or the authorization code.",
		})
	}

	pasted, err := ompOAuthManualCode(ctx, callbacks, AuthManualCodePrompt{
		Message:     "Paste the redirect URL or authorization code:",
		Placeholder: redirectURI,
	})
	if err != nil {
		return OAuthCredentials{}, err
	}
	code := ParseZaiAuthorizationInput(pasted)
	if code == "" {
		return OAuthCredentials{}, errors.New("Missing authorization code")
	}

	ompOAuthProgress(callbacks, "Exchanging the authorization code...")
	body, err := ompOAuthJSONBody(map[string]any{
		"provider":     "zai",
		"code":         code,
		"redirect_uri": redirectURI,
		"state":        state,
	})
	if err != nil {
		return OAuthCredentials{}, err
	}
	status, payload, _, err := ompOAuthRequest(ctx, "POST", tokenURL, map[string]string{"Content-Type": "application/json"}, body)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if status < 200 || status >= 300 {
		return OAuthCredentials{}, fmt.Errorf("Z.ai token exchange failed (%d): %s", status, truncateOAuthBody(payload, 300))
	}
	token := ompOAuthObject(payload)
	if token == nil {
		return OAuthCredentials{}, errors.New("Z.ai token exchange returned an unreadable response")
	}
	// Z.ai answers a failed exchange inside the same `{code, msg}` envelope the business APIs
	// use, and the token itself sits either at the top level (OMP's zai.ts reads
	// `body.zai.access_token`) or under the envelope's `data` member (its declarative rule reads
	// `data.zai.access_token`). Both shapes are accepted; a rejected envelope's message wins.
	if unwrapped, unwrapErr := zaiOAuthUnwrap(token, "token exchange"); unwrapErr != nil {
		return OAuthCredentials{}, unwrapErr
	} else if record := zaiOAuthRecord(unwrapped); record != nil {
		token = record
	}
	accessToken := zaiOAuthTrimmed(zaiOAuthRecord(token["zai"])["access_token"])
	if accessToken == "" {
		if data := zaiOAuthRecord(token["data"]); data != nil {
			accessToken = zaiOAuthTrimmed(zaiOAuthRecord(data["zai"])["access_token"])
		}
	}
	if accessToken == "" {
		return OAuthCredentials{}, errors.New("Z.ai token exchange returned no access token")
	}

	ompOAuthProgress(callbacks, "Provisioning an API key on your Z.ai account...")
	apiKey, err := zaiOAuthMintAPIKey(ctx, accessToken, bizBase, businessLoginURL)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if ctx.Err() != nil {
		return OAuthCredentials{}, errors.New("Login cancelled")
	}

	credentials := OAuthCredentials{
		// The minted key never expires and Z.ai issues no refresh token, so both fields carry it.
		Refresh: apiKey,
		Access:  apiKey,
		Expires: ompOAuthMaxSafeInteger,
	}
	if user := zaiOAuthRecord(token["user"]); user != nil {
		ompOAuthSetEmail(&credentials, ompOAuthString(user["email"]))
		if accountID := zaiOAuthTrimmed(user["id"]); accountID != "" {
			credentials.AccountID = accountID
		}
	}
	return credentials, nil
}

// RefreshToken is the identity function: a durable API key has nothing to refresh.
func (zaiOAuthProvider) RefreshToken(creds OAuthCredentials) (OAuthCredentials, error) {
	return creds, nil
}

// RefreshTokenContext is RefreshToken with the caller's cancellation.
func (p zaiOAuthProvider) RefreshTokenContext(ctx context.Context, creds OAuthCredentials) (OAuthCredentials, error) {
	return p.RefreshToken(creds)
}

// truncateOAuthBody shortens a provider error body for display, leaving it
// untouched when it is already short.
func truncateOAuthBody(raw []byte, limit int) string {
	text := fetchBodyText(raw)
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}
