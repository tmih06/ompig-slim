package ai

// Native ports of OMP's browser sign-in flows whose endpoint state is a durable API key or a
// rotating bearer token: Z.ai, Cursor, Snowflake and Perplexity.
//
// This file holds the pieces they share: interactive prompt plumbing that works through either
// login callback form, small HTTP/JSON helpers with a per-request deadline, and JWT claim reading
// without signature verification (upstream never verifies these tokens; it only reads `exp` and
// `sub` to decide expiry and account identity).

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// ompOAuthMaxSafeInteger is JavaScript's Number.MAX_SAFE_INTEGER, the expiry Pi stores for a
	// credential that never needs refreshing (a minted API key or a server-side session).
	ompOAuthMaxSafeInteger int64 = 1<<53 - 1
	// ompOAuthNeverExpires is JavaScript's maximum Date value (8.64e15 ms), the expiry OMP keeps
	// for a JWT that carries no `exp` claim.
	ompOAuthNeverExpires int64 = 8640000000000000
	// ompOAuthRequestTimeout bounds one provider HTTP request, matching the 30s AbortSignal the
	// upstream flows pass to fetch.
	ompOAuthRequestTimeout = 30 * time.Second
)

// ompOAuthPrompt asks the user for one value, using whichever callback form the host supplies.
func ompOAuthPrompt(ctx context.Context, callbacks OAuthLoginCallbacks, prompt OAuthPrompt) (string, error) {
	if callbacks.OnPromptContext != nil {
		return callbacks.OnPromptContext(ctx, prompt)
	}
	if callbacks.OnPrompt != nil {
		return callbacks.OnPrompt(prompt)
	}
	return "", errors.New("login requires interactive input")
}

// ompOAuthManualCode asks for a pasted code or redirect URL. It runs the flow's own manual_code
// prompt so the host shows the same label OMP's controller supplies, and keeps the prompt's
// cancellation behaviour.
func ompOAuthManualCode(ctx context.Context, callbacks OAuthLoginCallbacks, prompt AuthManualCodePrompt) (string, error) {
	return manualCodeInteraction(callbacks).Prompt(ctx, prompt)
}

// ompOAuthProgress reports progress when the host wants it.
func ompOAuthProgress(callbacks OAuthLoginCallbacks, message string) {
	if callbacks.OnProgress != nil {
		callbacks.OnProgress(message)
	}
}

// ompOAuthHTTPClient is the client every flow here uses. It is the default transport so the
// process-wide proxy environment applies exactly as it does for fetch.
var ompOAuthHTTPClient = &http.Client{}

// ompOAuthRequest sends one request with the flow's deadline and returns the body. A cancelled
// context reports "Login cancelled", as every upstream flow's fetch rejection does.
func ompOAuthRequest(ctx context.Context, method, endpoint string, headers map[string]string, body []byte) (int, []byte, http.Header, error) {
	if ctx.Err() != nil {
		return 0, nil, nil, errors.New("Login cancelled")
	}
	requestCtx, cancel := context.WithTimeout(ctx, ompOAuthRequestTimeout)
	defer cancel()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(requestCtx, method, endpoint, reader)
	if err != nil {
		return 0, nil, nil, err
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := ompOAuthHTTPClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, nil, errors.New("Login cancelled")
		}
		return 0, nil, nil, err
	}
	defer func() { _ = response.Body.Close() }()
	payload, readErr := io.ReadAll(response.Body)
	if ctx.Err() != nil {
		return 0, nil, nil, errors.New("Login cancelled")
	}
	return response.StatusCode, payload, response.Header, readErr
}

// ompOAuthJSONValue parses a response body as JSON. ok is false for an empty or unreadable body.
func ompOAuthJSONValue(raw []byte) (any, bool) {
	raw = []byte(fetchBodyText(raw))
	if len(raw) == 0 || !json.Valid(raw) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	return value, true
}

// ompOAuthObject reads a response body as a JSON object. A missing or non-object body reads as an
// empty object, which every caller's field checks then reject.
func ompOAuthObject(raw []byte) map[string]any {
	value, ok := ompOAuthJSONValue(raw)
	if !ok {
		return nil
	}
	members, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return members
}

// ompOAuthString reads a string member.
func ompOAuthString(value any) string {
	text, _ := value.(string)
	return text
}

// ompOAuthNumber reads a JSON number member as its JavaScript value.
func ompOAuthNumber(value any) (float64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := number.Float64()
	if err != nil {
		return 0, false
	}
	return parsed, true
}

// ompOAuthJSONBody encodes a request payload, reporting an encoding failure rather than sending an
// empty body.
func ompOAuthJSONBody(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode request body: %w", err)
	}
	return body, nil
}

// ompOAuthJWTClaims reads a JWT payload without verifying its signature. ok is false when the
// token is not three dot-separated parts, its payload is not base64url JSON, or that JSON is not
// an object.
func ompOAuthJWTClaims(token string) (map[string]any, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil, false
	}
	value, ok := ompOAuthJSONValue(raw)
	if !ok {
		return nil, false
	}
	members, ok := value.(map[string]any)
	return members, ok
}

// ompOAuthJWTExpiryMs is the expiry a stored credential carries for a JWT: `exp` minus a five
// minute safety margin, or never when the token has no numeric `exp` (OMP treats those sessions as
// server-side and non-expiring from the client's point of view).
func ompOAuthJWTExpiryMs(token string) int64 {
	claims, ok := ompOAuthJWTClaims(token)
	if !ok {
		return ompOAuthNeverExpires
	}
	exp, ok := ompOAuthNumber(claims["exp"])
	if !ok {
		return ompOAuthNeverExpires
	}
	return int64(exp*1000) - 5*60*1000
}

// ompOAuthSetEmail records the account email a flow learned, keeping the field order the
// provider's credential literal writes.
func ompOAuthSetEmail(credentials *OAuthCredentials, email string) {
	email = strings.TrimSpace(email)
	if email == "" {
		return
	}
	if credentials.Extra == nil {
		credentials.Extra = map[string]json.RawMessage{}
	}
	encoded, err := json.Marshal(email)
	if err != nil {
		return
	}
	credentials.Extra["email"] = encoded
}

// ompOAuthExtraString reads a string claim this package stored in a flow
// credential's Extra fields.
func ompOAuthExtraString(credentials OAuthCredentials, field string) string {
	raw, ok := credentials.Extra[field]
	if !ok {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

// ompOAuthRandomHex is the CSRF state some flows send: n random bytes written as
// lowercase hex, matching upstream's generateState.
func ompOAuthRandomHex(n int) (string, error) {
	buffer := make([]byte, n)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
