package ai

// Native port of OMP's Snowflake Cortex sign-in (pi-ai registry/oauth/snowflake.ts
// and the snowflake transport in registry/snowflake.ts plus the auth "snowflake"
// policy: custom login, custom refresh, structured API-key format, callback port
// 54551).
//
// Snowflake runs OAuth for local applications: the built-in
// SNOWFLAKE$LOCAL_APPLICATION client, PKCE, and a loopback callback. The account
// host is prompted first, because it is part of every endpoint. The flow's
// end state is a bearer token scoped to one account, so the account URL travels
// with the credential and is restored into the request base URL.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	snowflakeOAuthClientID      = "LOCAL_APPLICATION"
	snowflakeCallbackPort       = 54551
	snowflakeOAuthName          = "Snowflake Cortex"
	snowflakeAccountPlaceholder = "https://snowflake-account.invalid"
)

// snowflakeInvalidAccountMessage is the prompt shown for any account value that
// does not name a Snowflake host.
const snowflakeInvalidAccountMessage = "Paste your Snowflake account identifier (orgname-accountname) or account URL"

// snowflakeDNSLabel matches one DNS label: the host must be a sequence of them.
var snowflakeDNSLabel = func(label string) bool {
	if label == "" || len(label) > 63 {
		return false
	}
	for index, c := range label {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-':
			if index == 0 || index == len(label)-1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// snowflakeLegacyRegion matches an AWS region id such as us-east-1 or
// us-gov-west-1. Legacy Snowsight links put it where the organization goes, and
// its host mapping is region-specific, so those links are refused rather than
// guessed.
var snowflakeLegacyRegion = func(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) < 3 {
		return false
	}
	if len(parts[0]) != 2 || !allLowerAlpha(parts[0]) {
		return false
	}
	index := 1
	if parts[1] == "gov" {
		index = 2
	}
	if len(parts) != index+2 {
		return false
	}
	if !allLowerAlpha(parts[index]) {
		return false
	}
	digits := parts[index+1]
	if digits == "" {
		return false
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func allLowerAlpha(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

// NormalizeSnowflakeAccountURL turns an account identifier, a host, or a
// Snowsight link into the account's https origin, rejecting anything that could
// receive a credential at an unintended host.
//
// Accepted forms: `myorg-myaccount`, `myorg-myaccount.snowflakecomputing.com`,
// `myorg-myaccount:443`, `https://myorg-myaccount.snowflakecomputing.com`,
// `https://myorg-myaccount.snowflakecomputing.com:443/…`, and Snowsight account
// links `https://app.snowflake.com/<org>/<account>/…`.
func NormalizeSnowflakeAccountURL(input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", errors.New("Snowflake account is required: run /login snowflake or set SNOWFLAKE_ACCOUNT")
	}
	// Only `<scheme>://` is a scheme; `host:443` is a schemeless authority.
	lowerPrefix := strings.ToLower(value)
	schemeEnd := strings.Index(lowerPrefix, "://")
	hasScheme := schemeEnd > 0
	if hasScheme {
		scheme := lowerPrefix[:schemeEnd]
		if scheme != "https" {
			return "", errors.New("Snowflake account URL must use https")
		}
	}
	if strings.ContainsRune(value, '\\') {
		return "", errors.New(snowflakeInvalidAccountMessage)
	}

	var host string
	if hasScheme {
		rest := value[schemeEnd+3:]
		end := strings.IndexAny(rest, "/?#")
		authority, path := rest, ""
		if end >= 0 {
			authority, path = rest[:end], rest[end:]
		}
		if authority == "" {
			return "", errors.New(snowflakeInvalidAccountMessage)
		}
		// Validate the raw authority: URL parsing otherwise repairs some malformed inputs.
		host = strings.ToLower(strings.ReplaceAll(authority, "_", "-"))
		if strings.HasSuffix(host, ":443") {
			host = host[:len(host)-len(":443")]
		}
		if host == "app.snowflake.com" {
			segments := strings.Split(strings.SplitN(strings.TrimPrefix(path, "/"), "?", 2)[0], "/")
			org, account := "", ""
			if len(segments) > 0 {
				org = strings.ToLower(strings.ReplaceAll(segments[0], "_", "-"))
			}
			if len(segments) > 1 {
				account = strings.ToLower(strings.ReplaceAll(segments[1], "_", "-"))
			}
			if !snowflakeDNSLabel(org) || !snowflakeDNSLabel(account) {
				return "", errors.New(snowflakeInvalidAccountMessage)
			}
			if snowflakeLegacyRegion(org) {
				return "", errors.New("Legacy Snowsight links don't identify the account host; paste your account identifier (orgname-accountname) or account URL")
			}
			host = org + "-" + account + ".snowflakecomputing.com"
		}
	} else {
		if strings.ContainsAny(value, "/?#") {
			return "", errors.New(snowflakeInvalidAccountMessage)
		}
		host = strings.ToLower(strings.ReplaceAll(value, "_", "-"))
		if strings.HasSuffix(host, ":443") {
			host = host[:len(host)-len(":443")]
		}
		if !strings.HasSuffix(host, ".snowflakecomputing.com") && !strings.HasSuffix(host, ".snowflakecomputing.cn") {
			host += ".snowflakecomputing.com"
		}
	}

	if strings.HasSuffix(host, ".snowflakecomputing.cn") {
		return "", errors.New("Snowflake Cortex REST API is not available in China-region accounts (.snowflakecomputing.cn)")
	}
	const suffix = ".snowflakecomputing.com"
	if !strings.HasSuffix(host, suffix) || host == suffix {
		return "", errors.New(snowflakeInvalidAccountMessage)
	}
	for _, label := range strings.Split(host, ".") {
		if !snowflakeDNSLabel(label) {
			return "", errors.New(snowflakeInvalidAccountMessage)
		}
	}
	return "https://" + host, nil
}

// snowflakeTokens is one token response, validated.
type snowflakeTokens struct {
	Access  string
	Refresh string
	Expires int64
}

// parseSnowflakeTokenResponse validates a token-exchange or refresh body.
//
// Snowflake returns `expires_in` in seconds and may omit `refresh_token`; a
// response missing a token or a positive lifetime is rejected rather than stored
// as a credential no request can use.
func parseSnowflakeTokenResponse(payload []byte) (snowflakeTokens, error) {
	body := ompOAuthObject(payload)
	if body == nil {
		return snowflakeTokens{}, errors.New("Snowflake returned an invalid token response")
	}
	access := strings.TrimSpace(ompOAuthString(body["access_token"]))
	expiresIn, hasExpires := ompOAuthNumber(body["expires_in"])
	if access == "" || !hasExpires || expiresIn <= 0 {
		return snowflakeTokens{}, errors.New("Snowflake returned an invalid token response")
	}
	refresh := ""
	if raw, present := body["refresh_token"]; present {
		value := strings.TrimSpace(ompOAuthString(raw))
		if value == "" {
			return snowflakeTokens{}, errors.New("Snowflake returned an invalid token response")
		}
		refresh = value
	}
	lifetimeMs := expiresIn * 1000
	if lifetimeMs <= 0 {
		return snowflakeTokens{}, errors.New("Snowflake returned an invalid token lifetime")
	}
	return snowflakeTokens{Access: access, Refresh: refresh, Expires: nowMillis() + int64(lifetimeMs)}, nil
}

// exchangeSnowflakeCode trades an authorization code for tokens at the account's
// token endpoint.
func exchangeSnowflakeCode(ctx context.Context, accountURL, code, verifier, redirectURI string) (OAuthCredentials, error) {
	fields := formFields{
		{"grant_type", "authorization_code"},
		{"code", code},
		{"redirect_uri", redirectURI},
		{"code_verifier", verifier},
		{"client_id", snowflakeOAuthClientID},
	}
	status, payload, _, err := ompOAuthRequest(ctx, "POST", accountURL+"/oauth/token-request", map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	}, []byte(fields.Encode()))
	if err != nil {
		return OAuthCredentials{}, err
	}
	if status < 200 || status >= 300 {
		return OAuthCredentials{}, fmt.Errorf("Snowflake token exchange failed (%d)", status)
	}
	tokens, err := parseSnowflakeTokenResponse(payload)
	if err != nil {
		return OAuthCredentials{}, err
	}
	return OAuthCredentials{
		Access:  tokens.Access,
		Refresh: tokens.Refresh,
		Expires: tokens.Expires,
		// The account URL rides in the credential so refresh and request routing know the host.
		Extra: map[string]json.RawMessage{"enterpriseUrl": mustJSON(accountURL)},
	}, nil
}

// mustJSON encodes a string that cannot fail to encode.
func mustJSON(value string) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return encoded
}

// LoginSnowflake runs the Snowflake loopback OAuth flow.
func LoginSnowflake(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	account, err := ompOAuthPrompt(ctx, callbacks, OAuthPrompt{
		Message:     "Enter your Snowflake account identifier or URL",
		Placeholder: "myorg-myaccount",
	})
	if err != nil {
		return OAuthCredentials{}, err
	}
	accountURL, err := NormalizeSnowflakeAccountURL(account)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if ctx.Err() != nil {
		return OAuthCredentials{}, errors.New("Login cancelled")
	}

	pkce, err := GeneratePKCE()
	if err != nil {
		return OAuthCredentials{}, err
	}
	state, err := ompOAuthRandomHex(16)
	if err != nil {
		return OAuthCredentials{}, err
	}

	// Bind the callback first so the advertised redirect URI carries the real port. The
	// token exchange runs inside the callback handler, so a failed exchange shows on the
	// browser page instead of being reported only after it closes.
	var redirectURI string
	capture := func(callbackCtx context.Context, code string) (OAuthCredentials, error) {
		return exchangeSnowflakeCode(callbackCtx, accountURL, code, pkce.Verifier, redirectURI)
	}
	callback, err := StartOAuthCallbackServer(ctx, OAuthCallbackServerOptions[OAuthCredentials]{
		ProviderName: "Snowflake",
		Host:         oauthCallbackHost(),
		Port:         snowflakeCallbackPort,
		// Some local-app integrations reject callback subpaths despite the documented allowance.
		Path: "/",
		// The redirect host is 127.0.0.1: the local-application client registers it exactly.
		RedirectHost: "127.0.0.1",
		State:        &state,
		Complete:     capture,
	})
	if err != nil {
		return OAuthCredentials{}, err
	}
	defer callback.Close()
	redirectURI = callback.RedirectURI

	query := orderedQuery(
		"client_id", snowflakeOAuthClientID,
		"response_type", "code",
		"redirect_uri", redirectURI,
		"scope", "refresh_token",
		"state", state,
		"code_challenge", pkce.Challenge,
		"code_challenge_method", "S256",
	)
	if callbacks.OnAuth != nil {
		callbacks.OnAuth(OAuthAuthInfo{
			URL:          accountURL + "/oauth/authorize?" + query,
			Instructions: "Sign in to Snowflake in your browser and approve access for omp.",
		})
	}
	ompOAuthProgress(callbacks, "Waiting for browser authentication...")

	result, err := WaitForCallbackOrManualInput(ctx, manualCodeInteraction(callbacks), callback, AuthManualCodePrompt{
		Message:     "Complete login in your browser, or paste the authorization code / redirect URL here:",
		Placeholder: redirectURI,
	})
	if err != nil {
		return OAuthCredentials{}, err
	}
	if result.Callback {
		return result.Value, nil
	}
	code, pastedState := parseSnowflakeCallbackInput(result.Input)
	if code == "" {
		return OAuthCredentials{}, errors.New("Missing authorization code")
	}
	if pastedState != "" && pastedState != state {
		return OAuthCredentials{}, errors.New("OAuth state mismatch")
	}
	return exchangeSnowflakeCode(ctx, accountURL, code, pkce.Verifier, redirectURI)
}

// parseSnowflakeCallbackInput reads a pasted redirect URL or a bare code.
func parseSnowflakeCallbackInput(input string) (code, state string) {
	value := trimJSWhitespace(input)
	if value == "" {
		return "", ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" {
		query := parsed.Query()
		return query.Get("code"), query.Get("state")
	}
	if strings.Contains(value, "code=") {
		fields := authorizationQueryParams(value)
		return fields.Get("code"), fields.Get("state")
	}
	code, state, _ = strings.Cut(value, "#")
	return strings.TrimSpace(code), strings.TrimSpace(state)
}

// RefreshSnowflakeToken renews a Snowflake bearer token at the credential's own
// account host.
func RefreshSnowflakeToken(ctx context.Context, creds OAuthCredentials) (OAuthCredentials, error) {
	refresh := strings.TrimSpace(creds.Refresh)
	if refresh == "" {
		// Without refresh issuance the shared refresh skew would discard a still-valid token.
		if float64(nowMillis()) < creds.ExpiresMillis() {
			return creds, nil
		}
		return OAuthCredentials{}, errors.New("Snowflake did not issue a refresh token; run /login snowflake again or use SNOWFLAKE_ACCOUNT and SNOWFLAKE_PAT")
	}
	accountURL, err := NormalizeSnowflakeAccountURL(ompOAuthExtraString(creds, "enterpriseUrl"))
	if err != nil {
		return OAuthCredentials{}, errors.New("Invalid Snowflake account; run /login snowflake again")
	}
	fields := formFields{
		{"grant_type", "refresh_token"},
		{"refresh_token", refresh},
		{"client_id", snowflakeOAuthClientID},
	}
	status, payload, _, err := ompOAuthRequest(ctx, "POST", accountURL+"/oauth/token-request", map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	}, []byte(fields.Encode()))
	if err != nil {
		return OAuthCredentials{}, err
	}
	if status < 200 || status >= 300 {
		return OAuthCredentials{}, fmt.Errorf("Snowflake token refresh failed (%d)", status)
	}
	tokens, err := parseSnowflakeTokenResponse(payload)
	if err != nil {
		return OAuthCredentials{}, err
	}
	refreshed := creds
	refreshed.Access = tokens.Access
	refreshed.Expires = tokens.Expires
	if tokens.Refresh != "" {
		refreshed.Refresh = tokens.Refresh
	}
	if refreshed.Extra == nil {
		refreshed.Extra = map[string]json.RawMessage{}
	}
	refreshed.Extra["enterpriseUrl"] = mustJSON(accountURL)
	return refreshed, nil
}

// snowflakeOAuthProvider is the registered Snowflake sign-in flow.
type snowflakeOAuthProvider struct{}

func (snowflakeOAuthProvider) ID() string               { return "snowflake" }
func (snowflakeOAuthProvider) Name() string             { return snowflakeOAuthName }
func (snowflakeOAuthProvider) IsSubscription() bool     { return true }
func (snowflakeOAuthProvider) UsesCallbackServer() bool { return true }

// GetAPIKey is the structured credential the Snowflake transport reads: the
// request bearer plus the account host that every endpoint hangs off.
func (snowflakeOAuthProvider) GetAPIKey(creds OAuthCredentials) string {
	accountURL := ompOAuthExtraString(creds, "enterpriseUrl")
	structured := struct {
		Token         string `json:"token"`
		EnterpriseURL string `json:"enterpriseUrl"`
	}{Token: creds.Access, EnterpriseURL: accountURL}
	encoded, err := json.Marshal(structured)
	if err != nil {
		return creds.Access
	}
	return string(encoded)
}

func (p snowflakeOAuthProvider) Login(callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return p.LoginContext(context.Background(), callbacks)
}

// LoginContext runs the Snowflake flow with the caller's cancellation.
func (p snowflakeOAuthProvider) LoginContext(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return LoginSnowflake(ctx, callbacks)
}

// RefreshToken refreshes without cancellation. Prefer RefreshTokenContext.
func (p snowflakeOAuthProvider) RefreshToken(creds OAuthCredentials) (OAuthCredentials, error) {
	return p.RefreshTokenContext(context.Background(), creds)
}

// RefreshTokenContext renews a Snowflake grant with the caller's cancellation.
func (p snowflakeOAuthProvider) RefreshTokenContext(ctx context.Context, creds OAuthCredentials) (OAuthCredentials, error) {
	return RefreshSnowflakeToken(ctx, creds)
}
