package ai

// Ports Mistral Vibe's browser sign-in (node_modules/@oh-my-pi/pi-ai/src/registry/oauth/mistral-vibe.ts),
// which mirrors the Vibe Code CLI (mistralai/mistral-vibe, vibe/setup/auth/):
//
//  1. POST /api/vibe/sign-in { code_challenge, code_challenge_method: "S256" }
//     -> { process_id, sign_in_url, poll_url, expires_at }
//  2. The user approves at sign_in_url; poll_url answers { status, exchange_token? }
//     (HTTP 410 once expired, 429 when polled too often).
//  3. POST /api/vibe/sign-in/{process_id}/exchange { exchange_token, code_verifier } -> { api_key }
//
// The minted key is a regular Mistral API key billed against the plan's Vibe Code quota, so the
// login stores it as a plain API-key credential.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	mistralConsoleOrigin  = "https://console.mistral.ai"
	mistralAPIPath        = "/api"
	mistralRequestTimeout = 30 * time.Second
	// mistralPollIntervalSeconds is the Vibe CLI's poll cadence.
	mistralPollIntervalSeconds = 3
	// mistralMaxConsecutivePollFailures is the Vibe CLI's failure budget.
	mistralMaxConsecutivePollFailures = 3
	// mistralMaxRetryAfter caps one honoured Retry-After, so a large value cannot hide an approval
	// until the sign-in expires.
	mistralMaxRetryAfter = 30 * time.Second
	// mistralWorkspaceDeniedErrorCode is the console error_code for a workspace that may not mint keys.
	mistralWorkspaceDeniedErrorCode = 9004
	mistralExpiredMessage           = "Mistral sign-in expired; run /login again"
)

// mistralVibeOAuthProvider implements Mistral's browser PKCE sign-in. The console origin and client
// are injectable so a test can drive the protocol.
type mistralVibeOAuthProvider struct {
	client *http.Client
	origin string
}

func newMistralVibeOAuthProvider() mistralVibeOAuthProvider {
	return mistralVibeOAuthProvider{client: &http.Client{}, origin: mistralConsoleOrigin}
}

func (mistralVibeOAuthProvider) ID() string           { return "mistral" }
func (mistralVibeOAuthProvider) Name() string         { return "Mistral" }
func (mistralVibeOAuthProvider) IsSubscription() bool { return true }
func (mistralVibeOAuthProvider) UsesCallbackServer() bool {
	return false
}
func (mistralVibeOAuthProvider) GetAPIKey(c OAuthCredentials) string { return c.Access }

func (p mistralVibeOAuthProvider) RefreshToken(creds OAuthCredentials) (OAuthCredentials, error) {
	return ompUnchangedRefresh(creds)
}

// Login runs the flow without cancellation. Prefer LoginContext.
func (p mistralVibeOAuthProvider) Login(callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return p.LoginContext(context.Background(), callbacks)
}

// mistralSignIn is one started browser sign-in.
type mistralSignIn struct {
	processID   string
	signInURL   string
	pollURL     string
	expiresAtMs float64
}

// LoginContext mints a Vibe API key with the owning operation's cancellation.
func (p mistralVibeOAuthProvider) LoginContext(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	// Mistral's verifier is 96 random bytes base64url encoded (registry/oauth/pkce.ts), longer than
	// the shared 32-byte GeneratePKCE.
	verifier, challenge, err := mistralPKCE()
	if err != nil {
		return OAuthCredentials{}, err
	}
	signIn, err := p.startSignIn(ctx, challenge)
	if err != nil {
		return OAuthCredentials{}, err
	}
	ompNotifyAuth(callbacks, signIn.signInURL,
		"Sign in with your Mistral account and approve Mistral Vibe access, then return here.")
	ompNotifyProgress(callbacks, "Waiting for Mistral sign-in to complete...")

	expiresInSeconds := math.Max(0, signIn.expiresAtMs-deviceCodeNow()) / 1000
	intervalSeconds := float64(mistralPollIntervalSeconds)
	exchangeToken, err := PollOAuthDeviceCodeFlow(ctx, DeviceCodePollOptions[string]{
		IntervalSeconds:  &intervalSeconds,
		ExpiresInSeconds: &expiresInSeconds,
		Poll:             p.signInPoller(ctx, signIn.pollURL),
	})
	if err != nil {
		return OAuthCredentials{}, err
	}
	ompNotifyProgress(callbacks, "Exchanging sign-in for a Mistral API key...")
	key, err := p.exchangeForAPIKey(ctx, signIn.processID, exchangeToken, verifier)
	if err != nil {
		return OAuthCredentials{}, err
	}
	return ompKeyCredential(key), nil
}

// mistralPKCE is generatePKCE of registry/oauth/pkce.ts: a 96-byte verifier, base64url encoded,
// with its S256 challenge.
func mistralPKCE() (verifier string, challenge string, err error) {
	verifierBytes := make([]byte, 96)
	if _, err := rand.Read(verifierBytes); err != nil {
		return "", "", err
	}
	verifier = base64URLEncode(verifierBytes)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64URLEncode(sum[:]), nil
}

// mistralOAuthError is the console failure of one step, carrying the status that explains it.
type mistralOAuthError struct {
	message string
	status  int
}

func (e *mistralOAuthError) Error() string { return e.message }

func mistralErrorf(format string, args ...any) error {
	return &mistralOAuthError{message: fmt.Sprintf(format, args...)}
}

// post sends one console API request. Cancellation surfaces as the cancel message, and an
// unparseable body as absent members.
func (p mistralVibeOAuthProvider) post(ctx context.Context, path string, body map[string]string) (int, map[string]any, error) {
	if err := ompCancelled(ctx); err != nil {
		return 0, nil, err
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	resp, err := ompHTTPPost(ctx, p.client, p.origin+mistralAPIPath+path, encoded, mistralRequestTimeout, nil)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	members, _ := ompReadJSONBody(resp)
	return resp.StatusCode, members, nil
}

// consoleURL accepts only console URLs at or below basePath, segment-aware: `/api` rejects `/api-x`.
func consoleURL(value, origin, basePath, message string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme+"://"+parsed.Host != origin {
		return "", mistralErrorf("%s: unexpected URL %s", message, value)
	}
	if parsed.Path != basePath && !strings.HasPrefix(parsed.Path, basePath+"/") {
		return "", mistralErrorf("%s: unexpected URL %s", message, value)
	}
	return parsed.String(), nil
}

// startSignIn creates the browser sign-in process and validates its URLs.
func (p mistralVibeOAuthProvider) startSignIn(ctx context.Context, challenge string) (mistralSignIn, error) {
	const message = "Failed to start Mistral browser sign-in"
	status, payload, err := p.post(ctx, "/vibe/sign-in", map[string]string{
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
	})
	if err != nil {
		return mistralSignIn{}, err
	}
	if status < 200 || status >= 300 {
		return mistralSignIn{}, mistralErrorf("%s: HTTP %d", message, status)
	}
	processID, okProcess := ompJSONString(payload, "process_id")
	signInURL, okSignIn := ompJSONString(payload, "sign_in_url")
	pollURL, okPoll := ompJSONString(payload, "poll_url")
	expiresAt, okExpires := ompJSONString(payload, "expires_at")
	if !okProcess || processID == "" || !okSignIn || !okPoll || !okExpires {
		return mistralSignIn{}, mistralErrorf("%s: response missing required fields", message)
	}
	expiresAtMs := jsDateParse(expiresAt)
	if math.IsNaN(expiresAtMs) {
		return mistralSignIn{}, mistralErrorf("%s: invalid expires_at", message)
	}
	trustedSignIn, err := consoleURL(signInURL, p.origin, "", message)
	if err != nil {
		return mistralSignIn{}, err
	}
	trustedPoll, err := consoleURL(pollURL, p.origin, mistralAPIPath, message)
	if err != nil {
		return mistralSignIn{}, err
	}
	return mistralSignIn{processID: processID, signInURL: trustedSignIn, pollURL: trustedPoll, expiresAtMs: expiresAtMs}, nil
}

// signInPoller returns the classifier one poll of the console's poll URL answers with. A 410 expires
// the sign-in, a 429 slows the cadence with the console's Retry-After, and network errors, non-2xx
// answers and unreadable bodies count as pending until the consecutive-failure budget runs out.
func (p mistralVibeOAuthProvider) signInPoller(ctx context.Context, pollURL string) func() (DeviceCodePollResult[string], error) {
	failures := 0
	return func() (DeviceCodePollResult[string], error) {
		status, members, headers := p.pollOnce(ctx, pollURL)
		if status == -1 {
			return DeviceCodePollResult[string]{}, errors.New(ompCancelMessage)
		}
		if status == http.StatusGone {
			return DeviceCodePollResult[string]{}, &mistralOAuthError{message: mistralExpiredMessage, status: status}
		}
		if status == http.StatusTooManyRequests {
			failures = 0
			return DeviceCodePollResult[string]{Status: DevicePollSlowDown, IntervalSeconds: retryAfterSeconds(headers)}, nil
		}
		answer, okStatus := ompJSONString(members, "status")
		if !okStatus || status < 200 || status >= 300 {
			failures++
			if failures < mistralMaxConsecutivePollFailures {
				return DeviceCodePollResult[string]{Status: DevicePollPending}, nil
			}
			reason := "request failed"
			if status != 0 {
				reason = fmt.Sprintf("HTTP %d", status)
			}
			return DeviceCodePollResult[string]{}, &mistralOAuthError{message: "Mistral sign-in status unavailable: " + reason, status: status}
		}
		failures = 0
		switch answer {
		case "pending":
			return DeviceCodePollResult[string]{Status: DevicePollPending}, nil
		case "completed":
			if token, ok := ompJSONString(members, "exchange_token"); ok && token != "" {
				return DeviceCodePollResult[string]{Status: DevicePollComplete, Value: token}, nil
			}
			return DeviceCodePollResult[string]{}, mistralErrorf("Mistral sign-in completed without an exchange token")
		case "expired":
			return DeviceCodePollResult[string]{}, &mistralOAuthError{message: mistralExpiredMessage}
		case "denied":
			return DeviceCodePollResult[string]{}, mistralErrorf("Mistral sign-in was denied")
		case "error":
			message, _ := ompJSONString(members, "message")
			if message == "" {
				message = "Mistral sign-in failed"
			}
			return DeviceCodePollResult[string]{}, mistralErrorf("%s", message)
		default:
			return DeviceCodePollResult[string]{}, mistralErrorf("Mistral sign-in returned unknown state %q", answer)
		}
	}
}

// pollOnce reads one poll response: its status and headers, plus its members when the body is a
// JSON object. A failed request reports status 0; a cancelled caller reports -1.
func (p mistralVibeOAuthProvider) pollOnce(ctx context.Context, pollURL string) (int, map[string]any, http.Header) {
	requestCtx, cancel := context.WithTimeout(ctx, mistralRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, pollURL, nil)
	if err != nil {
		return 0, nil, nil
	}
	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return -1, nil, nil
		}
		return 0, nil, nil
	}
	defer func() { _ = resp.Body.Close() }()
	members, _ := ompReadJSONBody(resp)
	return resp.StatusCode, members, resp.Header
}

// retryAfterSeconds is the console's Retry-After as a capped interval in seconds, or nil when the
// response names no delay (the RFC 8628 default then applies).
func retryAfterSeconds(headers http.Header) *float64 {
	delay := retryAfterFromHeaders(headers)
	if delay <= 0 {
		return nil
	}
	capped := math.Min(float64(delay.Milliseconds()), float64(mistralMaxRetryAfter.Milliseconds())) / 1000
	return &capped
}

// exchangeForAPIKey trades the exchange token and the PKCE verifier for a Mistral API key.
func (p mistralVibeOAuthProvider) exchangeForAPIKey(ctx context.Context, processID, exchangeToken, verifier string) (string, error) {
	const message = "Failed to exchange Mistral sign-in for an API key"
	status, payload, err := p.post(ctx, "/vibe/sign-in/"+ompEncodeURIComponent(processID)+"/exchange", map[string]string{
		"exchange_token": exchangeToken,
		"code_verifier":  verifier,
	})
	if err != nil {
		return "", err
	}
	if status == http.StatusForbidden && ompJSONIsWorkspaceDenied(payload) {
		return "", &mistralOAuthError{status: status, message: "Your Mistral workspace can't be used for Mistral Vibe; run /login again and pick another workspace in the browser, or ask your administrator"}
	}
	if status < 200 || status >= 300 {
		return "", mistralErrorf("%s: HTTP %d", message, status)
	}
	apiKey, ok := ompJSONString(payload, "api_key")
	if !ok || apiKey == "" {
		return "", mistralErrorf("%s: response missing api_key", message)
	}
	return apiKey, nil
}

// ompJSONIsWorkspaceDenied reports the console's error_code for a workspace that may not mint keys.
func ompJSONIsWorkspaceDenied(payload map[string]any) bool {
	code, ok := ompJSONNumber(payload, "error_code")
	return ok && code == mistralWorkspaceDeniedErrorCode
}

// retryAfterFromHeaders is getRetryAfterMsFromHeaders for the headers a poll answers with: the
// largest of retry-after-ms, retry-after (seconds or a date) and the x-ratelimit-reset variants.
func retryAfterFromHeaders(headers http.Header) time.Duration {
	now := time.Now().UnixMilli()
	var best int64
	consider := func(ms int64) {
		if ms > best {
			best = ms
		}
	}
	if value := strings.TrimSpace(headers.Get("retry-after-ms")); value != "" {
		if ms, err := strconv.ParseFloat(value, 64); err == nil && !math.IsNaN(ms) && !math.IsInf(ms, 0) && ms > 0 {
			consider(int64(math.Ceil(ms)))
		}
	}
	if value := strings.TrimSpace(headers.Get("retry-after")); value != "" {
		if seconds, err := strconv.ParseFloat(value, 64); err == nil && !math.IsNaN(seconds) && !math.IsInf(seconds, 0) {
			if seconds > 0 {
				consider(int64(math.Ceil(seconds * 1000)))
			}
		} else if dateMs := DateParseHeader(value); !math.IsNaN(dateMs) {
			if delay := int64(math.Ceil(dateMs - float64(now))); delay > 0 {
				consider(delay)
			}
		}
	}
	if ms, ok := resetHeaderDelay(headers.Get("x-ratelimit-reset-ms"), true, now); ok {
		consider(ms)
	}
	if ms, ok := resetHeaderDelay(headers.Get("x-ratelimit-reset"), false, now); ok {
		consider(ms)
	}
	return time.Duration(best) * time.Millisecond
}

// resetHeaderDelay is parseResetHeader: an absolute or relative reset instant, in ms or seconds.
func resetHeaderDelay(value string, millis bool, nowMs int64) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	numeric, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(numeric) || math.IsInf(numeric, 0) || numeric <= 0 {
		return 0, false
	}
	var targetMs int64
	switch {
	case numeric > 1e12:
		targetMs = int64(numeric)
	case numeric > 1e9:
		targetMs = int64(numeric * 1000)
	case millis:
		return int64(math.Ceil(numeric)), true
	default:
		return int64(math.Ceil(numeric * 1000)), true
	}
	if targetMs <= nowMs {
		return 0, false
	}
	return int64(math.Ceil(float64(targetMs - nowMs))), true
}
