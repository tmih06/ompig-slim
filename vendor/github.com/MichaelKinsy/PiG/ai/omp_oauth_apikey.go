package ai

// Ports the omp-slim sign-in flows whose end state is an API key for a provider this build already
// serves, as pi-ai does with createOmpFlow (packages/ai/src/auth/oauth/omp-flow.ts) over the login
// functions of node_modules/@oh-my-pi/pi-ai/src/registry/oauth.
//
// Kilo, Xiaomi MiMo, Alibaba Coding Plan, QwenCloud Token Plan, Cloudflare Workers AI, Cloudflare AI
// Gateway and Mistral Vibe are exposed as OAuth providers so /login offers them as subscription
// sign-in. Every one ends in a credential the provider's own toAuth reads: a bearer key, a composite
// key the request layer decodes, or Alibaba's enterprise endpoint.
//
// This file holds the pieces the flows share: the never-expiring key credential, prompt plumbing,
// the API-key validation probes (pi-ai registry/api-key-validation.ts) and the two flows that only
// paste a key (Kilo's device authorization and Xiaomi's regional probe).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// OMP sign-in surface shared by these flows.

const (
	// ompNeverExpiresMs is Number.MAX_SAFE_INTEGER, the expiry createOmpFlow stamps on a minted key:
	// a key has nothing to refresh and never expires.
	ompNeverExpiresMs = 9007199254740991

	// ompValidationTimeout is the 15s budget each validation probe gets from api-key-validation.ts.
	ompValidationTimeout = 15 * time.Second

	// ompCancelMessage is the message Pi reserves for a user-cancelled login; the interactive
	// dialog returns to its previous menu on it instead of reporting a failure.
	ompCancelMessage = "Login cancelled"
)

// ompCredentialForValidationError builds the probe failure message of api-key-validation.ts
// createApiKeyValidationError, including the response body when it has one.
func ompCredentialForValidationError(provider string, status int, details string) error {
	if details != "" {
		return fmt.Errorf("%s API key validation failed (%d): %s", provider, status, details)
	}
	return fmt.Errorf("%s API key validation failed (%d)", provider, status)
}

// ompEncodeURIComponent is JavaScript's encodeURIComponent: every byte of the UTF-8 encoding is
// escaped except A-Z a-z 0-9 - _ . ! ~ * ' ( ).
func ompEncodeURIComponent(value string) string {
	const unescaped = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.!~*'()"
	var out strings.Builder
	out.Grow(len(value))
	for i := range len(value) {
		c := value[i]
		if strings.IndexByte(unescaped, c) >= 0 {
			out.WriteByte(c)
			continue
		}
		out.WriteByte('%')
		const hex = "0123456789ABCDEF"
		out.WriteByte(hex[c>>4])
		out.WriteByte(hex[c&0x0f])
	}
	return out.String()
}

// ompHTTPPost sends a JSON body with a Content-Type header and returns the response. The caller
// closes the body. A cancelled ctx returns the cancel message instead of a transport error.
func ompHTTPPost(ctx context.Context, client *http.Client, endpoint string, body []byte, timeout time.Duration, headers map[string]string) (*http.Response, error) {
	requestCtx, cancel := context.WithCancel(ctx)
	if timeout > 0 {
		cancel()
		requestCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	var payload io.Reader
	if body != nil {
		payload = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, payload)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		cancel()
		if ctx.Err() != nil {
			return nil, errors.New(ompCancelMessage)
		}
		return nil, err
	}
	// The response body outlives the request context, so the deadline is released only when the body
	// closes; otherwise a slow body read would be cut off by the request timeout.
	resp.Body = &ompCancelOnCloseBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// ompCancelOnCloseBody releases a request's context when its body closes.
type ompCancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *ompCancelOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	if b.cancel != nil {
		cancel := b.cancel
		b.cancel = nil
		cancel()
	}
	return err
}

// ompReadJSONBody decodes a JSON object body, keeping numbers as their JavaScript values. ok is
// false for a body that is not a JSON object (upstream's `response.json()` rejection or a non-object).
func ompReadJSONBody(resp *http.Response) (map[string]any, bool) {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false
	}
	value, _, ok := oauthJSONValue(raw)
	if !ok {
		return nil, false
	}
	members, isObject := value.(map[string]any)
	if !isObject {
		return nil, false
	}
	return members, true
}

// ompBodyText reads a response body as text, or "" when it cannot be read.
func ompBodyText(resp *http.Response) string {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return fetchBodyText(raw)
}

// ompJSONString returns a JSON member when it is a string.
func ompJSONString(members map[string]any, key string) (string, bool) {
	value, ok := members[key].(string)
	return value, ok
}

// ompJSONNumber returns a JSON member when it is a number.
func ompJSONNumber(members map[string]any, key string) (float64, bool) {
	value, ok := members[key].(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := value.Float64()
	if err != nil {
		return 0, false
	}
	return parsed, true
}

// ompRequirePrompt reports the flow's onPrompt-required failure before it starts work, as each of
// these logins does before its first prompt or auth notice (Pi's OnPromptRequiredError).
func ompRequirePrompt(callbacks OAuthLoginCallbacks, providerLabel string) error {
	if callbacks.OnPromptContext == nil && callbacks.OnPrompt == nil {
		return fmt.Errorf("%s login requires onPrompt callback", providerLabel)
	}
	return nil
}

// ompPrompt asks a question through the flow's text prompt callback. It reports the flow's
// onPrompt-required failure when the host supplies no prompt callback, which is Pi's
// OnPromptRequiredError.
func ompPrompt(ctx context.Context, callbacks OAuthLoginCallbacks, providerLabel, message, placeholder string, allowEmpty bool) (string, error) {
	if callbacks.OnPromptContext == nil && callbacks.OnPrompt == nil {
		return "", fmt.Errorf("%s login requires onPrompt callback", providerLabel)
	}
	prompt := OAuthPrompt{Message: message, Placeholder: placeholder, AllowEmpty: allowEmpty}
	if callbacks.OnPromptContext != nil {
		return callbacks.OnPromptContext(ctx, prompt)
	}
	return callbacks.OnPrompt(prompt)
}

// ompNotifyAuth shows the login URL and its instructions, when the host reads events.
func ompNotifyAuth(callbacks OAuthLoginCallbacks, url, instructions string) {
	if callbacks.OnAuth != nil {
		callbacks.OnAuth(OAuthAuthInfo{URL: url, Instructions: instructions})
	}
}

// ompNotifyProgress reports a step of the flow.
func ompNotifyProgress(callbacks OAuthLoginCallbacks, message string) {
	if callbacks.OnProgress != nil {
		callbacks.OnProgress(message)
	}
}

// ompCancelled reports whether the flow's ctx was cancelled: a cancelled login answers with the
// shared cancel message, which the host renders as a cancelled dialog rather than a failure.
func ompCancelled(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.New(ompCancelMessage)
	}
	return nil
}

// ompKeyCredential is createOmpFlow's credential for a flow that returned a bare key: the key is
// both access and refresh, and it never expires.
func ompKeyCredential(key string) OAuthCredentials {
	credentials := OAuthCredentials{Refresh: key, Access: key, order: credentialKeyOrder{keys: refreshFirstCredentialMembers}}
	credentials.SetExpiresMillis(ompNeverExpiresMs)
	return credentials
}

// ompKeyCredentialWithExtra is ompKeyCredential carrying the provider-owned fields a flow returns
// alongside its key (an enterprise endpoint, for Alibaba Coding Plan).
func ompKeyCredentialWithExtra(key string, extra map[string]any) (OAuthCredentials, error) {
	credentials := ompKeyCredential(key)
	if len(extra) == 0 {
		return credentials, nil
	}
	encoded := make(map[string]json.RawMessage, len(extra))
	for name, value := range extra {
		data, err := json.Marshal(value)
		if err != nil {
			return OAuthCredentials{}, err
		}
		encoded[name] = data
	}
	credentials.Extra = encoded
	return credentials, nil
}

// ompUnchangedRefresh is the refresh of a minted key: nothing rotates, so the stored credential is
// returned as it is (createOmpFlow's refresh).
func ompUnchangedRefresh(creds OAuthCredentials) (OAuthCredentials, error) { return creds, nil }

// validateOpenAICompatibleAPIKey probes an OpenAI-compatible chat endpoint with a one-token request
// and reports the provider's own failure text (api-key-validation.ts
// validateOpenAICompatibleApiKey).
func validateOpenAICompatibleAPIKey(ctx context.Context, client *http.Client, provider, apiKey, baseURL, model, maxTokensField string, maxTokens int) error {
	if maxTokensField == "" {
		maxTokensField = "max_tokens"
	}
	payload := map[string]any{
		"model":        model,
		"messages":     []map[string]string{{"role": "user", "content": "ping"}},
		maxTokensField: maxTokens,
		"temperature":  0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := ompHTTPPost(ctx, client, strings.TrimRight(baseURL, "/")+"/chat/completions", body, ompValidationTimeout, map[string]string{"Authorization": "Bearer " + apiKey})
	if err != nil {
		return err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_ = resp.Body.Close()
		return nil
	}
	details := strings.TrimSpace(ompBodyText(resp))
	_ = resp.Body.Close()
	return ompCredentialForValidationError(provider, resp.StatusCode, details)
}

// validateAPIKeyAgainstModelsEndpoint probes a provider's model list, which validates a key whose
// plan may not reach a particular model (api-key-validation.ts validateApiKeyAgainstModelsEndpoint).
func validateAPIKeyAgainstModelsEndpoint(ctx context.Context, client *http.Client, provider, apiKey, modelsURL string) error {
	requestCtx, cancel := context.WithTimeout(ctx, ompValidationTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return errors.New(ompCancelMessage)
		}
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return ompCredentialForValidationError(provider, resp.StatusCode, strings.TrimSpace(ompBodyText(resp)))
}

// isXiaomiTokenPlanKey identifies a Xiaomi Token Plan key, which authenticates against the regional
// clusters rather than the pay-as-you-go host (pi-catalog wire/xiaomi-token-plan.ts).
func isXiaomiTokenPlanKey(apiKey string) bool {
	return strings.HasPrefix(apiKey, "tp-") || strings.HasPrefix(apiKey, "ttp-")
}

// ─── Kilo Gateway ─────────────────────────────────────────────────────────────

const (
	kiloDeviceAuthBaseURL = "https://api.kilo.ai/api/device-auth"
	kiloPollInterval      = 5 * time.Second
	kiloKeyLifetimeMs     = int64(365 * 24 * 60 * 60 * 1000)
)

// kiloOAuthProvider is Kilo Gateway's device-authorization login: the browser shows a short code,
// the flow polls until the user approves, and the approved token becomes the gateway key for a year
// (registry/oauth/kilo.ts loginKilo).
type kiloOAuthProvider struct {
	client    *http.Client
	baseURL   string
	pollDelay func(context.Context, time.Duration) error
}

func newKiloOAuthProvider() kiloOAuthProvider {
	return kiloOAuthProvider{client: &http.Client{}, baseURL: kiloDeviceAuthBaseURL}
}

func (kiloOAuthProvider) ID() string                          { return "kilo" }
func (kiloOAuthProvider) Name() string                        { return "Kilo Gateway" }
func (kiloOAuthProvider) IsSubscription() bool                { return true }
func (kiloOAuthProvider) UsesCallbackServer() bool            { return false }
func (kiloOAuthProvider) GetAPIKey(c OAuthCredentials) string { return c.Access }

func (p kiloOAuthProvider) RefreshToken(creds OAuthCredentials) (OAuthCredentials, error) {
	return ompUnchangedRefresh(creds)
}

// Login runs the flow without cancellation. Prefer LoginContext.
func (p kiloOAuthProvider) Login(callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return p.LoginContext(context.Background(), callbacks)
}

// LoginContext runs Kilo's device authorization with the owning operation's cancellation.
func (p kiloOAuthProvider) LoginContext(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	if err := ompCancelled(ctx); err != nil {
		return OAuthCredentials{}, err
	}
	initiate, err := ompHTTPPost(ctx, p.client, p.baseURL+"/codes", []byte("{}"), 0, nil)
	if err != nil {
		return OAuthCredentials{}, err
	}
	switch {
	case initiate.StatusCode == http.StatusTooManyRequests:
		_ = initiate.Body.Close()
		return OAuthCredentials{}, errors.New("Too many pending authorization requests. Please try again later.")
	case initiate.StatusCode < 200 || initiate.StatusCode >= 300:
		status := initiate.StatusCode
		_ = initiate.Body.Close()
		return OAuthCredentials{}, fmt.Errorf("Failed to initiate device authorization: %d", status)
	}
	// A body that does not parse has no fields, which the required-field check reports.
	members, _ := ompReadJSONBody(initiate)
	_ = initiate.Body.Close()
	userCode, hasCode := ompJSONString(members, "code")
	verificationURL, hasURL := ompJSONString(members, "verificationUrl")
	expiresIn, hasExpiry := ompJSONNumber(members, "expiresIn")
	if !hasCode || !hasURL || !hasExpiry || math.IsNaN(expiresIn) || expiresIn <= 0 {
		return OAuthCredentials{}, errors.New("Kilo device authorization response missing required fields")
	}
	ompNotifyAuth(callbacks, verificationURL, "Enter code: "+userCode)

	sleep := p.pollDelay
	if sleep == nil {
		sleep = func(sleepCtx context.Context, d time.Duration) error {
			return Sleep(sleepCtx, float64(d)/float64(time.Millisecond))
		}
	}
	deadline := nowMillis() + int64(expiresIn*1000)
	for nowMillis() < deadline {
		if err := ompCancelled(ctx); err != nil {
			return OAuthCredentials{}, err
		}
		requestCtx, cancel := context.WithCancel(ctx)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, p.baseURL+"/codes/"+ompEncodeURIComponent(userCode), nil)
		if err != nil {
			cancel()
			return OAuthCredentials{}, err
		}
		resp, err := p.client.Do(req)
		if err != nil {
			cancel()
			if ctx.Err() != nil {
				return OAuthCredentials{}, errors.New(ompCancelMessage)
			}
			return OAuthCredentials{}, err
		}
		if resp.StatusCode == http.StatusAccepted {
			_ = resp.Body.Close()
			cancel()
			if err := sleep(ctx, kiloPollInterval); err != nil {
				return OAuthCredentials{}, errors.New(ompCancelMessage)
			}
			continue
		}
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusGone || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			status := resp.StatusCode
			_ = resp.Body.Close()
			cancel()
			switch status {
			case http.StatusForbidden:
				return OAuthCredentials{}, errors.New("Authorization was denied")
			case http.StatusGone:
				return OAuthCredentials{}, errors.New("Authorization code expired. Please try again.")
			}
			return OAuthCredentials{}, fmt.Errorf("Failed to poll device authorization: %d", status)
		}
		answer, _ := ompReadJSONBody(resp)
		_ = resp.Body.Close()
		cancel()
		status, _ := ompJSONString(answer, "status")
		token, hasToken := ompJSONString(answer, "token")
		switch status {
		case "approved":
			if hasToken && token != "" {
				credentials := OAuthCredentials{Refresh: "", Access: token, order: credentialKeyOrder{keys: refreshFirstCredentialMembers}}
				credentials.SetExpiresMillis(float64(nowMillis() + kiloKeyLifetimeMs))
				return credentials, nil
			}
		case "denied":
			return OAuthCredentials{}, errors.New("Authorization was denied")
		case "expired":
			return OAuthCredentials{}, errors.New("Authorization code expired. Please try again.")
		}
		if err := sleep(ctx, kiloPollInterval); err != nil {
			return OAuthCredentials{}, errors.New(ompCancelMessage)
		}
	}
	if err := ompCancelled(ctx); err != nil {
		return OAuthCredentials{}, err
	}
	return OAuthCredentials{}, errors.New("Authentication timed out. Please try again.")
}

// ─── Xiaomi MiMo ─────────────────────────────────────────────────────────────

const (
	xiaomiStandardAuthURL = "https://platform.xiaomimimo.com/#/console/api-keys"
	// xiaomiStandardBaseURL is the pay-as-you-go host; Token Plan keys use their regional cluster.
	xiaomiStandardBaseURL = "https://api.xiaomimimo.com/v1"
	// xiaomiValidationModel is the catalog default model the probe asks for.
	xiaomiValidationModel = "mimo-v2.6-flash"
)

// xiaomiTokenPlanEndpoints are the regional Token Plan clusters in the fallback order login probes:
// Singapore, then Europe, then China (registry/oauth/xiaomi.ts).
var xiaomiTokenPlanEndpoints = []struct {
	baseURL string
	model   string
}{
	{"https://token-plan-sgp.xiaomimimo.com/v1", xiaomiValidationModel},
	{"https://token-plan-ams.xiaomimimo.com/v1", xiaomiValidationModel},
	{"https://token-plan-cn.xiaomimimo.com/v1", xiaomiValidationModel},
}

// xiaomiOAuthProvider is Xiaomi MiMo's API-key login, which accepts both pay-as-you-go keys and
// regional Token Plan keys and proves the key against the right hosts before storing it.
type xiaomiOAuthProvider struct {
	client *http.Client
}

func newXiaomiOAuthProvider() xiaomiOAuthProvider { return xiaomiOAuthProvider{client: &http.Client{}} }

func (xiaomiOAuthProvider) ID() string                          { return "xiaomi" }
func (xiaomiOAuthProvider) Name() string                        { return "Xiaomi MiMo" }
func (xiaomiOAuthProvider) IsSubscription() bool                { return true }
func (xiaomiOAuthProvider) UsesCallbackServer() bool            { return false }
func (xiaomiOAuthProvider) GetAPIKey(c OAuthCredentials) string { return c.Access }

func (p xiaomiOAuthProvider) RefreshToken(creds OAuthCredentials) (OAuthCredentials, error) {
	return ompUnchangedRefresh(creds)
}

// Login runs the flow without cancellation. Prefer LoginContext.
func (p xiaomiOAuthProvider) Login(callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return p.LoginContext(context.Background(), callbacks)
}

// LoginContext prompts for a MiMo key and validates it with the owning operation's cancellation.
func (p xiaomiOAuthProvider) LoginContext(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	if err := ompRequirePrompt(callbacks, "Xiaomi MiMo"); err != nil {
		return OAuthCredentials{}, err
	}
	ompNotifyAuth(callbacks, xiaomiStandardAuthURL, "Copy your API key from the Xiaomi MiMo console")
	answer, err := ompPrompt(ctx, callbacks, "Xiaomi MiMo",
		"Paste your Xiaomi API key (sk-... or Token Plan tp-... / ttp-...)", "sk-..., tp-..., or ttp-...", false)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if err := ompCancelled(ctx); err != nil {
		return OAuthCredentials{}, err
	}
	apiKey := trimJSWhitespace(answer)
	if apiKey == "" {
		return OAuthCredentials{}, errors.New("API key is required")
	}
	ompNotifyProgress(callbacks, "Validating xiaomi API key...")
	if err := p.validate(ctx, apiKey); err != nil {
		return OAuthCredentials{}, err
	}
	return ompKeyCredential(apiKey), nil
}

// validate proves the key against the hosts its format selects. A Token Plan key is tried against the
// regional clusters in order: a 401 means that cluster did not issue it, so the next is tried; any
// other status means the cluster accepted the key but the probe failed, which is reported at once
// instead of being replaced by a later region's 401. Each endpoint gets its own timeout, so a
// regional outage falls through to the next cluster.
func (p xiaomiOAuthProvider) validate(ctx context.Context, apiKey string) error {
	endpoints := xiaomiTokenPlanEndpoints
	if !isXiaomiTokenPlanKey(apiKey) {
		endpoints = []struct {
			baseURL string
			model   string
		}{{xiaomiStandardBaseURL, xiaomiValidationModel}}
	}
	var lastErr error
	for _, endpoint := range endpoints {
		status, details, err := p.probe(ctx, apiKey, endpoint.baseURL, endpoint.model)
		if err != nil {
			if ctx.Err() != nil {
				return err
			}
			lastErr = err
			continue
		}
		if status >= 200 && status < 300 {
			return nil
		}
		failure := ompCredentialForValidationError("Xiaomi MiMo", status, details)
		if status != http.StatusUnauthorized {
			return failure
		}
		lastErr = failure
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("Xiaomi MiMo API key validation failed")
}

// probe posts the one-token chat request and returns the status and trimmed body.
func (p xiaomiOAuthProvider) probe(ctx context.Context, apiKey, baseURL, model string) (int, string, error) {
	body, err := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": 1,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
	})
	if err != nil {
		return 0, "", err
	}
	resp, err := ompHTTPPost(ctx, p.client, strings.TrimRight(baseURL, "/")+"/chat/completions", body, ompValidationTimeout, map[string]string{"Authorization": "Bearer " + apiKey})
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, strings.TrimSpace(ompBodyText(resp)), nil
}
