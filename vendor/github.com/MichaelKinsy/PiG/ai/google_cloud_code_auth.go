package ai

// Ports packages/ai/src/auth/oauth/google-cloud-code.ts (the product's Google sign-in for both providers)
// and the Cloud Code Assist provisioning of @oh-my-pi/pi-ai src/registry/oauth/google-gemini-cli.ts and
// src/registry/oauth/google-antigravity.ts: PKCE authorization, token exchange, refresh, and the
// loadCodeAssist/onboardUser project binding that produces the credential blob the transport sends.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// googleCCAClientID is the Gemini CLI's published OAuth client (base64 in OMP's auth rules).
	googleCCAClientID     = "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"
	googleCCAClientSecret = "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"
	googleCCAAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	googleCCATokenURL     = "https://oauth2.googleapis.com/token"
	googleCCAUserInfoURL  = "https://www.googleapis.com/oauth2/v1/userinfo?alt=json"
	googleCCACallbackPort = 8085
	googleCCACallbackPath = "/oauth2callback"
)

// googleCCAScopes are the scopes the Cloud Code Assist sign-in requests.
var googleCCAScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
}

const (
	googleCCALoginTimeoutMs   = 5 * 60 * 1000
	googleCCARequestTimeoutMs = 30 * 1000
	// googleCCAPollIntervalMs is the LRO poll cadence of the Gemini CLI project binding.
	googleCCAPollIntervalMs = 5000
	// googleCCAPollMaxAttempts bounds a stuck operation instead of polling forever.
	googleCCAPollMaxAttempts = 24
	// googleCCAOnboardTimeoutMs bounds the Antigravity free-tier onboarding.
	googleCCAOnboardTimeoutMs = 30 * 1000
	// googleCCAOnboardPollIntervalMs is the Antigravity onboarding poll cadence.
	googleCCAOnboardPollIntervalMs = 1000
)

const (
	googleCCATierFree     = "free-tier"
	googleCCATierLegacy   = "legacy-tier"
	googleCCATierStandard = "standard-tier"
)

// googleOAuthFlow is one provider's Google sign-in: both share the authorization-code shape and differ in
// the provider id, display name and project binding.
type googleOAuthFlow struct {
	providerID   string
	displayName  string
	clientID     string
	clientSecret string
}

var (
	googleGeminiCliFlow = googleOAuthFlow{
		providerID:   GoogleGeminiCliProviderID,
		displayName:  "Google Cloud Code Assist (Gemini CLI)",
		clientID:     googleCCAClientID,
		clientSecret: googleCCAClientSecret,
	}
	googleAntigravityFlow = googleOAuthFlow{
		providerID:   GoogleAntigravityProviderID,
		displayName:  "Google Antigravity",
		clientID:     googleCCAClientID,
		clientSecret: googleCCAClientSecret,
	}
)

// googleTokenResponse is Google's token endpoint reply.
type googleTokenResponse struct {
	AccessToken  string          `json:"access_token"`
	RefreshToken string          `json:"refresh_token"`
	ExpiresIn    json.RawMessage `json:"expires_in"`
}

// googleUserInfo is the profile lookup reply.
type googleUserInfo struct {
	Email string `json:"email"`
}

// googleOAuthFetch performs one provisioning round-trip: the controller signal and a per-request timeout
// compose, cancellation surfaces as a cancelled login, and a stall surfaces as a timeout.
func googleOAuthFetch(ctx context.Context, client *http.Client, method, requestURL string, body io.Reader, headers map[string]string, timeoutMs int) ([]byte, int, http.Header, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, nil, errors.New("OAuth login cancelled")
	}
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, method, requestURL, body)
	if err != nil {
		return nil, 0, nil, err
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, nil, errors.New("OAuth login cancelled")
		}
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			return nil, 0, nil, fmt.Errorf("Timed out after %dms waiting for %s", timeoutMs, requestURL)
		}
		return nil, 0, nil, err
	}
	defer func() { _ = response.Body.Close() }()
	data, readErr := io.ReadAll(response.Body)
	return data, response.StatusCode, response.Header, readErr
}

// googleExpiresInSeconds reads expires_in, defaulting to an hour.
func googleExpiresInSeconds(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 3600
	}
	var number float64
	if json.Unmarshal(raw, &number) == nil && number > 0 {
		return number
	}
	return 3600
}

// googleExchangeCode trades an authorization code for tokens and reads the account's email.
func googleExchangeCode(ctx context.Context, client *http.Client, flow googleOAuthFlow, code, verifier, redirectURI string) (OAuthCredentials, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", flow.clientID)
	form.Set("client_secret", flow.clientSecret)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", verifier)
	data, status, _, err := googleOAuthFetch(ctx, client, http.MethodPost, googleCCATokenURL, strings.NewReader(form.Encode()), map[string]string{
		"content-type": "application/x-www-form-urlencoded",
	}, googleCCARequestTimeoutMs)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if status < 200 || status >= 300 {
		return OAuthCredentials{}, fmt.Errorf("Google token exchange failed (%d): %s", status, firstCharacters(fetchBodyText(data), 300))
	}
	var token googleTokenResponse
	if err := json.Unmarshal(data, &token); err != nil || token.AccessToken == "" {
		return OAuthCredentials{}, errors.New("Google token exchange returned no access token")
	}
	email := googleLookupEmail(ctx, client, token.AccessToken)
	credentials := OAuthCredentials{
		Access:  token.AccessToken,
		Refresh: token.RefreshToken,
		order:   credentialKeyOrder{keys: accessFirstCredentialMembers},
	}
	// Refreshed a few minutes early, so a token never expires mid-request.
	credentials.SetExpiresMillis(float64(nowMillis()) + googleExpiresInSeconds(token.ExpiresIn)*1000 - 5*60*1000)
	credentials.Extra = map[string]json.RawMessage{}
	if email != "" {
		credentials.Extra["email"] = ccaMarshalJSON(email)
	}
	return credentials, nil
}

// googleLookupEmail reads the signed-in account's email address, best effort.
func googleLookupEmail(ctx context.Context, client *http.Client, accessToken string) string {
	data, status, _, err := googleOAuthFetch(ctx, client, http.MethodGet, googleCCAUserInfoURL, nil, map[string]string{
		"authorization": "Bearer " + accessToken,
	}, googleCCARequestTimeoutMs)
	if err != nil || status < 200 || status >= 300 {
		return ""
	}
	var info googleUserInfo
	if json.Unmarshal(data, &info) != nil {
		return ""
	}
	return info.Email
}

func firstCharacters(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}

// googleLoginWithPKCE runs the browser authorization and binds the grant to a Cloud Code project.
func googleLoginWithPKCE(ctx context.Context, callbacks OAuthLoginCallbacks, client *http.Client, flow googleOAuthFlow) (OAuthCredentials, error) {
	pkce, err := GeneratePKCE()
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("%s sign-in failed: generate PKCE: %w", flow.displayName, err)
	}
	startCallback := func(port int) (*OAuthCallbackServer[string], error) {
		return StartOAuthCallbackServer(ctx, OAuthCallbackServerOptions[string]{
			ProviderName: flow.displayName,
			Host:         "127.0.0.1",
			Port:         port,
			Path:         googleCCACallbackPath,
			Complete:     func(_ context.Context, code string) (string, error) { return code, nil },
			Timeout:      googleCCALoginTimeoutMs * time.Millisecond,
		})
	}
	callback, err := startCallback(googleCCACallbackPort)
	if err != nil {
		// The preferred port can be forwarded into a container or over SSH; any loopback port works.
		callback, err = startCallback(0)
		if err != nil {
			callback = nil
		}
	}
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", googleCCACallbackPort, googleCCACallbackPath)
	if callback != nil {
		defer callback.Close()
		redirectURI = callback.RedirectURI
	}

	authorize, err := url.Parse(googleCCAAuthorizeURL)
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("%s sign-in failed: %w", flow.displayName, err)
	}
	query := url.Values{}
	query.Set("client_id", flow.clientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(googleCCAScopes, " "))
	query.Set("code_challenge", pkce.Challenge)
	query.Set("code_challenge_method", "S256")
	query.Set("access_type", "offline")
	query.Set("prompt", "consent")
	authorize.RawQuery = query.Encode()
	if callbacks.OnAuth != nil {
		callbacks.OnAuth(OAuthAuthInfo{
			URL:          authorize.String(),
			Instructions: "Complete the sign-in in your browser. If it runs on another machine, paste the final redirect URL here.",
		})
	}
	interaction := manualCodeInteraction(callbacks)
	result, err := WaitForCallbackOrManualInput(ctx, interaction, callback, AuthManualCodePrompt{
		Message:     "Complete sign-in in your browser, or paste the redirect URL here:",
		Placeholder: redirectURI,
	})
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("%s sign-in failed: %w", flow.displayName, err)
	}
	code := result.Input
	if result.Callback {
		code = result.Value
	} else {
		code = googleAuthorizationCode(result.Input)
	}
	if code == "" {
		return OAuthCredentials{}, fmt.Errorf("%s sign-in failed: missing authorization code", flow.displayName)
	}
	credentials, err := googleExchangeCode(ctx, client, flow, code, pkce.Verifier, redirectURI)
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("%s sign-in failed: %w", flow.displayName, err)
	}
	if credentials.Refresh == "" {
		return OAuthCredentials{}, fmt.Errorf("%s sign-in failed: No refresh token received. Please try again.", flow.displayName)
	}
	if callbacks.OnProgress != nil {
		callbacks.OnProgress("Binding the sign-in to a Cloud Code project...")
	}
	projectID, err := googleDiscoverProject(ctx, client, flow, credentials.Access, callbacks.OnProgress)
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("%s sign-in failed: %w", flow.displayName, err)
	}
	credentials.ProjectID = projectID
	return credentials, nil
}

// googleAuthorizationCode extracts the code from a pasted redirect URL, or takes a bare code.
func googleAuthorizationCode(input string) string {
	trimmed := trimJSWhitespace(input)
	if trimmed == "" {
		return ""
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" {
		return parsed.Query().Get("code")
	}
	if query, ok := strings.CutPrefix(trimmed, "?"); ok {
		if values, err := url.ParseQuery(query); err == nil {
			return values.Get("code")
		}
	}
	if strings.Contains(trimmed, "code=") {
		values, err := url.ParseQuery(trimmed)
		if err == nil && values.Get("code") != "" {
			return values.Get("code")
		}
	}
	return trimmed
}

// googleRefreshCredential refreshes only the access token: the provisioning stays bound to the project.
func googleRefreshCredential(ctx context.Context, client *http.Client, flow googleOAuthFlow, credential Credential) (Credential, error) {
	if credential.Refresh == "" {
		return credential, nil
	}
	form := url.Values{}
	form.Set("client_id", flow.clientID)
	form.Set("client_secret", flow.clientSecret)
	form.Set("refresh_token", credential.Refresh)
	form.Set("grant_type", "refresh_token")
	data, status, _, err := googleOAuthFetch(ctx, client, http.MethodPost, googleCCATokenURL, strings.NewReader(form.Encode()), map[string]string{
		"content-type": "application/x-www-form-urlencoded",
	}, googleCCARequestTimeoutMs)
	if err != nil {
		return Credential{}, err
	}
	if status < 200 || status >= 300 {
		return Credential{}, fmt.Errorf("Google token refresh failed (%d): %s", status, firstCharacters(fetchBodyText(data), 300))
	}
	var token googleTokenResponse
	if err := json.Unmarshal(data, &token); err != nil || token.AccessToken == "" {
		return Credential{}, errors.New("Google token refresh returned no access token")
	}
	refreshed := credential
	refreshed.Access = token.AccessToken
	refreshed.SetExpiresMillis(float64(nowMillis()) + googleExpiresInSeconds(token.ExpiresIn)*1000 - 5*60*1000)
	return refreshed, nil
}

// ─── Project binding ─────────────────────────────────────────────────────────

// googleLoadCodeAssistPayload is the Gemini CLI project lookup's reply.
type googleLoadCodeAssistPayload struct {
	CloudaicompanionProject string                  `json:"cloudaicompanionProject"`
	CurrentTier             *googleAntigravityTier  `json:"currentTier"`
	AllowedTiers            []googleAntigravityTier `json:"allowedTiers"`
}

// googleLongRunningOperation is the provisioning operation's reply.
type googleLongRunningOperation struct {
	Name     string `json:"name"`
	Done     bool   `json:"done"`
	Response *struct {
		CloudaicompanionProject *struct {
			ID string `json:"id"`
		} `json:"cloudaicompanionProject"`
	} `json:"response"`
}

// googleRPCErrorBody is a Google RPC error, whose details decide a VPC-SC affected account.
type googleRPCErrorBody struct {
	Error struct {
		Details []struct {
			Reason string `json:"reason"`
		} `json:"details"`
	} `json:"error"`
}

// googleDiscoverProject resolves the project the sign-in is bound to: the Gemini CLI asks loadCodeAssist
// and onboards the account's tier; Antigravity provisions its free tier.
func googleDiscoverProject(ctx context.Context, client *http.Client, flow googleOAuthFlow, accessToken string, onProgress func(string)) (string, error) {
	if flow.providerID == GoogleAntigravityProviderID {
		return googleAntigravityProject(ctx, client, accessToken, onProgress)
	}
	return googleGeminiCliProject(ctx, client, accessToken, onProgress)
}

// googleGeminiCliProject ports googleGeminiCliProjectHook's provisioning: loadCodeAssist, then onboardUser
// with the account's default tier, then the long-running operation that names the project.
func googleGeminiCliProject(ctx context.Context, client *http.Client, accessToken string, onProgress func(string)) (string, error) {
	endpoint := strings.TrimRight(googleCloudCodeAssistEndpoint, "/")
	envProjectID := googleEnvProjectValue()
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
		"Content-Type":  "application/json",
	}
	for name, value := range ccaGeminiCliHeaders("") {
		headers[name] = value
	}
	if onProgress != nil {
		onProgress("Checking for existing Cloud Code Assist project...")
	}
	body := map[string]any{
		"cloudaicompanionProject": envProjectID,
		"metadata": map[string]any{
			"ideType":     "IDE_UNSPECIFIED",
			"platform":    "PLATFORM_UNSPECIFIED",
			"pluginType":  "GEMINI",
			"duetProject": envProjectID,
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	data, status, _, err := googleOAuthFetch(ctx, client, http.MethodPost, endpoint+"/v1internal:loadCodeAssist", strings.NewReader(string(encoded)), headers, googleCCARequestTimeoutMs)
	if err != nil {
		return "", err
	}
	var payload googleLoadCodeAssistPayload
	if status < 200 || status >= 300 {
		var rpcError googleRPCErrorBody
		if json.Unmarshal(data, &rpcError) == nil && googleIsVPCSCAffected(rpcError) {
			payload = googleLoadCodeAssistPayload{CurrentTier: &googleAntigravityTier{ID: googleCCATierStandard}}
		} else {
			return "", fmt.Errorf("loadCodeAssist failed: %d %s: %s", status, googleStatusText(status), fetchBodyText(data))
		}
	} else if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("loadCodeAssist failed: %w", err)
	}
	if payload.CurrentTier != nil {
		if payload.CloudaicompanionProject != "" {
			return payload.CloudaicompanionProject, nil
		}
		if envProjectID != "" {
			return envProjectID, nil
		}
		return "", errors.New(googleCCACloudProjectRequirement())
	}
	tierID := googleDefaultTier(payload.AllowedTiers)
	if tierID != googleCCATierFree && envProjectID == "" {
		return "", errors.New(googleCCACloudProjectRequirement())
	}
	if onProgress != nil {
		onProgress("Provisioning Cloud Code Assist project (this may take a moment)...")
	}
	onboardBody := map[string]any{
		"tierId": tierID,
		"metadata": map[string]any{
			"ideType":    "IDE_UNSPECIFIED",
			"platform":   "PLATFORM_UNSPECIFIED",
			"pluginType": "GEMINI",
		},
	}
	if tierID != googleCCATierFree && envProjectID != "" {
		onboardBody["cloudaicompanionProject"] = envProjectID
		onboardBody["metadata"].(map[string]any)["duetProject"] = envProjectID
	}
	encoded, err = json.Marshal(onboardBody)
	if err != nil {
		return "", err
	}
	data, status, _, err = googleOAuthFetch(ctx, client, http.MethodPost, endpoint+"/v1internal:onboardUser", strings.NewReader(string(encoded)), headers, googleCCARequestTimeoutMs)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("onboardUser failed: %d %s: %s", status, googleStatusText(status), fetchBodyText(data))
	}
	var operation googleLongRunningOperation
	if err := json.Unmarshal(data, &operation); err != nil {
		return "", fmt.Errorf("onboardUser failed: %w", err)
	}
	if !operation.Done && operation.Name != "" {
		operation, err = googlePollOperation(ctx, client, endpoint, operation.Name, headers, onProgress)
		if err != nil {
			return "", err
		}
	}
	if operation.Response != nil && operation.Response.CloudaicompanionProject != nil && operation.Response.CloudaicompanionProject.ID != "" {
		return operation.Response.CloudaicompanionProject.ID, nil
	}
	if envProjectID != "" {
		return envProjectID, nil
	}
	return "", errors.New("Could not discover or provision a Google Cloud project. Try setting the GOOGLE_CLOUD_PROJECT or GOOGLE_CLOUD_PROJECT_ID environment variable. See https://goo.gle/gemini-cli-auth-docs#workspace-gca")
}

// googlePollOperation polls a provisioning operation to completion.
func googlePollOperation(ctx context.Context, client *http.Client, endpoint, name string, headers map[string]string, onProgress func(string)) (googleLongRunningOperation, error) {
	for attempt := 0; attempt < googleCCAPollMaxAttempts; attempt++ {
		if attempt > 0 {
			if onProgress != nil {
				onProgress(fmt.Sprintf("Waiting for project provisioning (attempt %d/%d)...", attempt+1, googleCCAPollMaxAttempts))
			}
			if err := googleSleep(ctx, googleCCAPollIntervalMs); err != nil {
				return googleLongRunningOperation{}, err
			}
		}
		if err := ctx.Err(); err != nil {
			return googleLongRunningOperation{}, errors.New("OAuth login cancelled")
		}
		data, status, _, err := googleOAuthFetch(ctx, client, http.MethodGet, endpoint+"/v1internal/"+name, nil, headers, googleCCARequestTimeoutMs)
		if err != nil {
			return googleLongRunningOperation{}, err
		}
		if status < 200 || status >= 300 {
			return googleLongRunningOperation{}, fmt.Errorf("Failed to poll operation: %d %s", status, googleStatusText(status))
		}
		var operation googleLongRunningOperation
		if err := json.Unmarshal(data, &operation); err != nil {
			return googleLongRunningOperation{}, fmt.Errorf("Failed to poll operation: %w", err)
		}
		if operation.Done {
			return operation, nil
		}
	}
	return googleLongRunningOperation{}, fmt.Errorf("Project provisioning did not complete after %d attempts", googleCCAPollMaxAttempts)
}

// googleAntigravityProject ports googleAntigravityProjectHook's provisioning: loadCodeAssist, the free-tier
// eligibility check, the onboarding operation, then a refreshed lookup that names the project.
func googleAntigravityProject(ctx context.Context, client *http.Client, accessToken string, onProgress func(string)) (string, error) {
	endpoint := strings.TrimRight(googleAntigravityPrimaryEndpoint, "/")
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
		"Content-Type":  "application/json",
	}
	for name, value := range ccaAntigravityHeaders() {
		headers[name] = value
	}
	metadata := map[string]any{"ideType": "ANTIGRAVITY"}
	request := func(label, method, requestURL string, body any, timeoutMs int) ([]byte, error) {
		var reader io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}
			reader = strings.NewReader(string(encoded))
		}
		data, status, _, err := googleOAuthFetch(ctx, client, method, requestURL, reader, headers, timeoutMs)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("%s failed: %d %s: %s", label, status, googleStatusText(status), fetchBodyText(data))
		}
		return data, nil
	}
	load := func(body map[string]any) (googleAntigravityLoadCodeAssist, error) {
		data, err := request("loadCodeAssist", http.MethodPost, endpoint+"/v1internal:loadCodeAssist", body, googleCCARequestTimeoutMs)
		if err != nil {
			return googleAntigravityLoadCodeAssist{}, err
		}
		var payload googleAntigravityLoadCodeAssist
		if err := json.Unmarshal(data, &payload); err != nil {
			return googleAntigravityLoadCodeAssist{}, fmt.Errorf("failed to unmarshal LoadCodeAssistResponse: %w", err)
		}
		return payload, nil
	}
	loadCodeAssist := func() (googleAntigravityLoadCodeAssist, error) {
		payload, err := load(map[string]any{"metadata": metadata})
		if err != nil {
			return googleAntigravityLoadCodeAssist{}, err
		}
		if payload.PaidTier == nil && payload.CloudaicompanionProject != "" {
			payload, err = load(map[string]any{"cloudaicompanionProject": payload.CloudaicompanionProject, "metadata": metadata})
			if err != nil {
				return googleAntigravityLoadCodeAssist{}, err
			}
		}
		return payload, nil
	}

	if onProgress != nil {
		onProgress("Checking Cloud Code Assist account status...")
	}
	initial, err := loadCodeAssist()
	if err != nil {
		return "", err
	}
	if err := googleAssertFreeTierEligible(initial); err != nil {
		return "", err
	}
	if initial.CurrentTier == nil {
		if onProgress != nil {
			onProgress("Provisioning the Antigravity free tier...")
		}
		if err := googleAntigravityOnboard(ctx, request); err != nil {
			return "", err
		}
	}
	if onProgress != nil {
		onProgress("Refreshing Cloud Code Assist project...")
	}
	refreshed, err := loadCodeAssist()
	if err != nil {
		return "", err
	}
	if refreshed.CloudaicompanionProject != "" {
		return refreshed.CloudaicompanionProject, nil
	}
	return "", errors.New("loadCodeAssist did not return a cloudaicompanionProject")
}

// googleAntigravityLoadCodeAssist is Antigravity's project lookup reply.
type googleAntigravityLoadCodeAssist struct {
	CurrentTier             *googleAntigravityTier  `json:"currentTier"`
	PaidTier                *googleAntigravityTier  `json:"paidTier"`
	CloudaicompanionProject string                  `json:"cloudaicompanionProject"`
	AllowedTiers            []googleAntigravityTier `json:"allowedTiers"`
	IneligibleTiers         []struct {
		TierID        string `json:"tierId"`
		ReasonMessage string `json:"reasonMessage"`
		ValidationURL string `json:"validationUrl"`
	} `json:"ineligibleTiers"`
}

// googleAntigravityTier is one tier record.
type googleAntigravityTier struct {
	ID        string `json:"id"`
	IsDefault bool   `json:"isDefault"`
}

// googleAssertFreeTierEligible reports the account's free-tier ineligibility, with its validation URL.
func googleAssertFreeTierEligible(payload googleAntigravityLoadCodeAssist) error {
	for _, tier := range payload.AllowedTiers {
		if tier.ID == googleCCATierFree {
			return nil
		}
	}
	for _, tier := range payload.IneligibleTiers {
		if tier.TierID != googleCCATierFree || tier.ReasonMessage == "" {
			continue
		}
		message := tier.ReasonMessage
		if tier.ValidationURL != "" {
			message += "\n" + tier.ValidationURL
		}
		return errors.New(message)
	}
	return nil
}

// googleAntigravityOnboard provisions the Antigravity free tier and waits for its operation.
func googleAntigravityOnboard(ctx context.Context, request func(label, method, requestURL string, body any, timeoutMs int) ([]byte, error)) error {
	endpoint := strings.TrimRight(googleAntigravityPrimaryEndpoint, "/")
	deadline := time.Now().Add(googleCCAOnboardTimeoutMs * time.Millisecond)
	remaining := func() (int, error) {
		left := time.Until(deadline)
		if left <= 0 {
			return 0, fmt.Errorf("onboardUser timed out after %dms", googleCCAOnboardTimeoutMs)
		}
		return int(left / time.Millisecond), nil
	}
	timeoutMs, err := remaining()
	if err != nil {
		return err
	}
	data, err := request("onboardUser", http.MethodPost, endpoint+"/v1internal:onboardUser", map[string]any{
		"tierId":   googleCCATierFree,
		"metadata": map[string]any{"ideType": "ANTIGRAVITY"},
	}, timeoutMs)
	if err != nil {
		return err
	}
	operation, err := googleParseOnboardOperation(data)
	if err != nil {
		return err
	}
	for {
		if operation.Done {
			if operation.Error != nil {
				return fmt.Errorf("OnboardUser operation failed: %s", googleDescribeOperationError(operation.Error))
			}
			if operation.Response == nil {
				return errors.New("failed to unmarshal OnboardUserResponse")
			}
			return nil
		}
		wait, err := remaining()
		if err != nil {
			return err
		}
		if err := googleSleep(ctx, min(googleCCAOnboardPollIntervalMs, wait)); err != nil {
			return err
		}
		if operation.Name == "" {
			return errors.New("onboardUser returned an operation without a name")
		}
		timeoutMs, err := remaining()
		if err != nil {
			return err
		}
		data, err := request("onboardUser operation", http.MethodGet, endpoint+"/v1internal/"+operation.Name, nil, timeoutMs)
		if err != nil {
			return err
		}
		operation, err = googleParseOnboardOperation(data)
		if err != nil {
			return err
		}
	}
}

// googleOnboardOperation is the onboarding operation's reply.
type googleOnboardOperation struct {
	Name     string                       `json:"name"`
	Done     bool                         `json:"done"`
	Error    *googleOnboardOperationError `json:"error"`
	Response *googleOnboardOperationReply `json:"response"`
}

// googleOnboardOperationError is an operation's failure record.
type googleOnboardOperationError struct {
	Code    *int   `json:"code"`
	Message string `json:"message"`
}

// googleOnboardOperationReply is an operation's result.
type googleOnboardOperationReply struct {
	Type                    string `json:"@type"`
	CloudaicompanionProject string `json:"cloudaicompanionProject"`
}

func googleParseOnboardOperation(data []byte) (googleOnboardOperation, error) {
	var operation googleOnboardOperation
	if err := json.Unmarshal(data, &operation); err != nil {
		return googleOnboardOperation{}, fmt.Errorf("failed to unmarshal OnboardUser operation: %w", err)
	}
	return operation, nil
}

func googleDescribeOperationError(operation *googleOnboardOperationError) string {
	if operation.Message != "" {
		if operation.Code != nil {
			return strconv.Itoa(*operation.Code) + ": " + operation.Message
		}
		return operation.Message
	}
	encoded, err := json.Marshal(operation)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// googleSleep waits for duration, ending early when ctx is cancelled.
func googleSleep(ctx context.Context, durationMs int) error {
	timer := time.NewTimer(time.Duration(durationMs) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errors.New("OAuth login cancelled")
	case <-timer.C:
		return nil
	}
}

// googleDefaultTier is getDefaultTier: the marked default, else the legacy tier, else free.
func googleDefaultTier(tiers []googleAntigravityTier) string {
	if len(tiers) == 0 {
		return googleCCATierLegacy
	}
	for _, tier := range tiers {
		if tier.IsDefault {
			return tier.ID
		}
	}
	return googleCCATierLegacy
}

// googleIsVPCSCAffected reports the SECURITY_POLICY_VIOLATED detail that marks a VPC-SC affected account,
// which the Gemini CLI treats as already on the standard tier.
func googleIsVPCSCAffected(payload googleRPCErrorBody) bool {
	for _, detail := range payload.Error.Details {
		if detail.Reason == "SECURITY_POLICY_VIOLATED" {
			return true
		}
	}
	return false
}

// googleCCACloudProjectRequirement is the workspace-account instruction.
func googleCCACloudProjectRequirement() string {
	return "This account requires setting the GOOGLE_CLOUD_PROJECT or GOOGLE_CLOUD_PROJECT_ID environment variable. See https://goo.gle/gemini-cli-auth-docs#workspace-gca"
}

// googleEnvProjectValue reads the configured project, which wins over provisioning.
func googleEnvProjectValue() string {
	for _, name := range []string{"GOOGLE_CLOUD_PROJECT", "GOOGLE_CLOUD_PROJECT_ID"} {
		if value := trimSpaceJS(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

// googleStatusText is a status code's reason phrase: Go knows the standard ones.
func googleStatusText(status int) string {
	if text := http.StatusText(status); text != "" {
		return text
	}
	return "Unknown Status"
}

// ─── OAuth provider registration ─────────────────────────────────────────────

// googleCloudCodeOAuthProvider is the registered OAuth flow of one Google provider. Its client is the
// package HTTP client seam: a test drives provisioning against a fixture without a browser.
type googleCloudCodeOAuthProvider struct {
	flow   googleOAuthFlow
	client *http.Client
}

// oauthClient is the flow's HTTP client, defaulting to the standard one.
func (provider googleCloudCodeOAuthProvider) oauthClient() *http.Client {
	if provider.client != nil {
		return provider.client
	}
	return http.DefaultClient
}

func (provider googleCloudCodeOAuthProvider) ID() string               { return provider.flow.providerID }
func (provider googleCloudCodeOAuthProvider) Name() string             { return provider.flow.displayName }
func (provider googleCloudCodeOAuthProvider) UsesCallbackServer() bool { return true }
func (provider googleCloudCodeOAuthProvider) IsSubscription() bool     { return true }

func (provider googleCloudCodeOAuthProvider) Login(callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return provider.LoginContext(context.Background(), callbacks)
}

func (provider googleCloudCodeOAuthProvider) LoginContext(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return googleLoginWithPKCE(ctx, callbacks, provider.oauthClient(), provider.flow)
}

func (provider googleCloudCodeOAuthProvider) RefreshToken(credentials OAuthCredentials) (OAuthCredentials, error) {
	return provider.RefreshTokenContext(context.Background(), credentials)
}

func (provider googleCloudCodeOAuthProvider) RefreshTokenContext(ctx context.Context, credentials OAuthCredentials) (OAuthCredentials, error) {
	credential, err := credentialFromOAuth(credentials)
	if err != nil {
		return OAuthCredentials{}, err
	}
	refreshed, err := googleRefreshCredential(ctx, provider.oauthClient(), provider.flow, credential)
	if err != nil {
		return OAuthCredentials{}, err
	}
	return credentialToOAuth(refreshed), nil
}

// GetAPIKey is the transport credential blob of the grant.
func (provider googleCloudCodeOAuthProvider) GetAPIKey(credentials OAuthCredentials) string {
	credential, err := credentialFromOAuth(credentials)
	if err != nil {
		return ""
	}
	return googleCloudCodeCredentialAPIKey(credential)
}

// googleCloudCodeCredentialAPIKey builds the blob the transport reads from a stored credential.
func googleCloudCodeCredentialAPIKey(credential Credential) string {
	var expiresAt *float64
	if credential.HasExpires() {
		exact := credential.ExpiresMillis()
		if exact > 0 && exact == exact {
			expiresAt = &exact
		}
	}
	email := ""
	if raw, ok := credential.Extra["email"]; ok {
		var decoded string
		if json.Unmarshal(raw, &decoded) == nil {
			email = decoded
		}
	}
	return ccaCredentialAPIKeyJSON(credential.Access, credential.Refresh, expiresAt, credential.ProjectID, email)
}
