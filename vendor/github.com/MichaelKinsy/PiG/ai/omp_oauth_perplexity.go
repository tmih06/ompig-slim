package ai

// Native port of OMP's Perplexity sign-in (pi-ai registry/oauth/perplexity.ts and
// the auth "perplexity" policy: custom login, `expiry "jwt-or-never"`,
// `refresh "none"`, PERPLEXITY_API_KEY for ambient keys).
//
// Three paths, in the order OMP tries them:
//
//  1. Borrow the legacy macOS desktop app's session token. Suppressed by
//     PI_AUTH_NO_BORROW.
//  2. Browser SSO for an organization account when a host-managed browser
//     callback is available. Its captured cookie is validated against
//     /api/auth/session.
//  3. Email + OTP (with an authenticator challenge when the account has one) over
//     the same endpoints the web sign-in uses.
//
// The stored access token is the Perplexity session JWT; its `exp` claim, when
// present, decides expiry, and a token without one is treated as non-expiring.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const (
	perplexityAPIVersion   = "2.18"
	perplexityNativeBundle = "ai.perplexity.mac"
	perplexityUserAgent    = "Perplexity/641 CFNetwork/1568 Darwin/25.2.0"
	perplexityBaseURL      = "https://www.perplexity.ai"
	perplexityOAuthName    = "Perplexity (Pro/Max)"
	// perplexitySessionCookie is the session cookie the web sign-in issues. The
	// unprefixed name is the older spelling some deployments still use.
	perplexitySessionCookie       = "__Secure-next-auth.session-token"
	perplexityLegacySessionCookie = "next-auth.session-token"
)

// perplexityCREDENTIAL is one Perplexity credential: the session JWT and, when
// known, the account email.
func perplexityCredential(jwt, email string) OAuthCredentials {
	credentials := OAuthCredentials{
		Access:  jwt,
		Refresh: jwt,
		Expires: ompOAuthJWTExpiryMs(jwt),
	}
	ompOAuthSetEmail(&credentials, email)
	return credentials
}

// perplexityBorrowDisabled reports PI_AUTH_NO_BORROW, which opts out of reading
// another app's stored session.
func perplexityBorrowDisabled() bool {
	return os.Getenv("PI_AUTH_NO_BORROW") != ""
}

// extractPerplexityNativeToken reads the legacy ai.perplexity.mac app's session.
// Newer Mac apps keep it in a restricted Keychain, so this is best-effort by
// design: any failure means "nothing to borrow".
func extractPerplexityNativeToken(ctx context.Context) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	command := exec.CommandContext(ctx, "defaults", "read", perplexityNativeBundle, "authToken")
	output, err := command.Output()
	if err != nil {
		return ""
	}
	token := strings.TrimSpace(string(output))
	if token == "" || token == "(null)" {
		return ""
	}
	return token
}

// perplexitySession is a per-login HTTP session with a cookie jar, which the web
// sign-in requires: the CSRF token is bound to the cookies set with it.
type perplexitySession struct {
	client *http.Client
}

func newPerplexitySession() (*perplexitySession, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &perplexitySession{client: &http.Client{Jar: jar, Timeout: ompOAuthRequestTimeout}}, nil
}

// request sends one signed-in request. A non-2xx status is returned to the caller
// with the body, because each step reports its own failure detail.
func (s *perplexitySession) request(ctx context.Context, method, endpoint string, headers map[string]string, body []byte) (int, []byte, error) {
	if ctx.Err() != nil {
		return 0, nil, errors.New("Login cancelled")
	}
	var reader *strings.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	var request *http.Request
	var err error
	if reader != nil {
		request, err = http.NewRequestWithContext(ctx, method, endpoint, reader)
	} else {
		request, err = http.NewRequestWithContext(ctx, method, endpoint, nil)
	}
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("User-Agent", perplexityUserAgent)
	request.Header.Set("X-App-ApiVersion", perplexityAPIVersion)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := s.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, errors.New("Login cancelled")
		}
		return 0, nil, err
	}
	defer func() { _ = response.Body.Close() }()
	payload, readErr := io.ReadAll(response.Body)
	if ctx.Err() != nil {
		return 0, nil, errors.New("Login cancelled")
	}
	return response.StatusCode, payload, readErr
}

// httpEmailLogin sends an email OTP and exchanges it for a session JWT, handling
// the authenticator challenge some accounts require.
func httpEmailLogin(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	email, err := ompOAuthPrompt(ctx, callbacks, OAuthPrompt{
		Message:     "Enter your Perplexity email address",
		Placeholder: "user@example.com",
	})
	if err != nil {
		return OAuthCredentials{}, err
	}
	trimmedEmail := strings.TrimSpace(email)
	if trimmedEmail == "" {
		return OAuthCredentials{}, errors.New("Email is required for Perplexity login")
	}
	if ctx.Err() != nil {
		return OAuthCredentials{}, errors.New("Login cancelled")
	}

	session, err := newPerplexitySession()
	if err != nil {
		return OAuthCredentials{}, err
	}

	ompOAuthProgress(callbacks, "Fetching Perplexity CSRF token...")
	status, payload, err := session.request(ctx, "GET", perplexityBaseURL+"/api/auth/csrf", nil, nil)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if status < 200 || status >= 300 {
		return OAuthCredentials{}, fmt.Errorf("Perplexity CSRF request failed: %d", status)
	}
	csrf := strings.TrimSpace(ompOAuthString(ompOAuthObject(payload)["csrfToken"]))
	if csrf == "" {
		return OAuthCredentials{}, errors.New("Perplexity CSRF response missing csrfToken")
	}

	ompOAuthProgress(callbacks, "Sending login code to your email...")
	sendBody, err := ompOAuthJSONBody(map[string]any{"email": trimmedEmail, "csrfToken": csrf})
	if err != nil {
		return OAuthCredentials{}, err
	}
	status, payload, err = session.request(ctx, "POST", perplexityBaseURL+"/api/auth/signin-email",
		map[string]string{"Content-Type": "application/json"}, sendBody)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if status < 200 || status >= 300 {
		return OAuthCredentials{}, fmt.Errorf("Perplexity send login code failed (%d): %s", status, fetchBodyText(payload))
	}

	otp, err := ompOAuthPrompt(ctx, callbacks, OAuthPrompt{
		Message:     "Enter the code sent to your email",
		Placeholder: "123456",
	})
	if err != nil {
		return OAuthCredentials{}, err
	}
	trimmedOTP := strings.TrimSpace(otp)
	if trimmedOTP == "" {
		return OAuthCredentials{}, errors.New("OTP code is required")
	}

	ompOAuthProgress(callbacks, "Verifying login code...")
	verifyBody, err := ompOAuthJSONBody(map[string]any{
		"email":     trimmedEmail,
		"otp":       trimmedOTP,
		"csrfToken": csrf,
	})
	if err != nil {
		return OAuthCredentials{}, err
	}
	status, payload, err = session.request(ctx, "POST", perplexityBaseURL+"/api/auth/signin-otp",
		map[string]string{"Content-Type": "application/json"}, verifyBody)
	if err != nil {
		return OAuthCredentials{}, err
	}
	verified := ompOAuthObject(payload)
	if status < 200 || status >= 300 {
		return OAuthCredentials{}, fmt.Errorf("Perplexity OTP verification failed: %s", perplexityReason(verified))
	}

	if ompOAuthString(verified["status"]) == "totp_challenge_required" && ompOAuthString(verified["challenge_token"]) != "" {
		totp, err := ompOAuthPrompt(ctx, callbacks, OAuthPrompt{
			Message:     "Enter the code from your authenticator app",
			Placeholder: "123456",
		})
		if err != nil {
			return OAuthCredentials{}, err
		}
		trimmedTOTP := strings.TrimSpace(totp)
		if trimmedTOTP == "" {
			return OAuthCredentials{}, errors.New("Authenticator code is required")
		}
		ompOAuthProgress(callbacks, "Verifying authenticator code...")
		totpBody, err := ompOAuthJSONBody(map[string]any{
			"token": ompOAuthString(verified["challenge_token"]),
			"code":  trimmedTOTP,
		})
		if err != nil {
			return OAuthCredentials{}, err
		}
		totpStatus, totpPayload, err := session.request(ctx, "POST", perplexityBaseURL+"/api/auth/totp/challenge-verify",
			map[string]string{"Content-Type": "application/json"}, totpBody)
		if err != nil {
			return OAuthCredentials{}, err
		}
		next := ompOAuthObject(totpPayload)
		if totpStatus < 200 || totpStatus >= 300 {
			return OAuthCredentials{}, fmt.Errorf("Perplexity authenticator verification failed: %s", perplexityReason(next))
		}
		verified = next
	}

	// The web flow finishes by setting the session cookie instead of returning a token,
	// so the cookie is the second source. A response that names neither is rejected.
	token := strings.TrimSpace(ompOAuthString(verified["token"]))
	if token == "" {
		token = strings.TrimSpace(ompOAuthString(verified["challenge_token"]))
	}
	if token == "" {
		token = session.cookieValue(perplexityBaseURL)
	}
	if status := strings.TrimSpace(ompOAuthString(verified["status"])); status != "" && status != "success" && token == "" {
		return OAuthCredentials{}, fmt.Errorf("Perplexity OTP verification response rejected: %s", perplexityReason(verified))
	}
	if token == "" || strings.TrimSpace(ompOAuthString(verified["error_code"])) != "" {
		return OAuthCredentials{}, fmt.Errorf("Perplexity OTP verification response rejected: %s", perplexityReason(verified))
	}
	return perplexityCredential(token, trimmedEmail), nil
}

// cookieValue reads the session cookie the jar holds for the base URL.
func (s *perplexitySession) cookieValue(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	for _, cookie := range s.client.Jar.Cookies(parsed) {
		if cookie.Name == perplexitySessionCookie || cookie.Name == perplexityLegacySessionCookie {
			return strings.TrimSpace(cookie.Value)
		}
	}
	return ""
}

// perplexityReason is the human detail a rejected auth response carries.
func perplexityReason(body map[string]any) string {
	for _, name := range []string{"text", "error_code", "status", "error"} {
		if value := strings.TrimSpace(ompOAuthString(body[name])); value != "" {
			return value
		}
	}
	return "missing token"
}

// perplexitySessionToken obtains the session cookie from a host-managed browser.
func perplexitySessionToken(ctx context.Context, callbacks OAuthLoginCallbacks) (string, error) {
	if callbacks.OnBrowserSession == nil {
		return "", errors.New("Browser SSO is unavailable in this client")
	}
	ompOAuthProgress(callbacks, "Complete Perplexity sign-in in the browser window. Choose SSO for your organization.")
	token, err := callbacks.OnBrowserSession(ctx, OAuthBrowserSessionRequest{
		URL:         perplexityBaseURL + "/auth/signin",
		CookieNames: []string{perplexitySessionCookie, perplexityLegacySessionCookie},
	})
	if err != nil {
		return "", err
	}
	if ctx.Err() != nil {
		return "", errors.New("Login cancelled")
	}
	return strings.TrimSpace(token), nil
}

// browserSsoLogin validates a session cookie and reads the account email from the
// session. The cookie must belong to a signed-in account, which the session lookup
// proves; an invalid or expired cookie is rejected.
func browserSsoLogin(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	token, err := perplexitySessionToken(ctx, callbacks)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if ctx.Err() != nil {
		return OAuthCredentials{}, errors.New("Login cancelled")
	}
	if token == "" || strings.ContainsAny(token, " \t\r\n;") {
		return OAuthCredentials{}, errors.New("Perplexity SSO captured an invalid session cookie")
	}

	ompOAuthProgress(callbacks, "Validating Perplexity session...")
	status, payload, _, err := ompOAuthRequest(ctx, "GET", perplexityBaseURL+"/api/auth/session", map[string]string{
		"Cookie":           perplexitySessionCookie + "=" + token,
		"User-Agent":       perplexityUserAgent,
		"X-App-ApiVersion": perplexityAPIVersion,
		"Accept":           "application/json",
	}, nil)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if status < 200 || status >= 300 {
		return OAuthCredentials{}, fmt.Errorf("Perplexity session validation failed (%d)", status)
	}
	session := ompOAuthObject(payload)
	if session == nil {
		return OAuthCredentials{}, errors.New("Perplexity returned an invalid session response")
	}
	user, _ := session["user"].(map[string]any)
	email := strings.TrimSpace(ompOAuthString(user["email"]))
	if email == "" {
		return OAuthCredentials{}, errors.New("Perplexity session is invalid or expired. Sign in again.")
	}
	return perplexityCredential(token, email), nil
}

// LoginPerplexity prefers the borrowed desktop session, then offers browser SSO or
// the email OTP flow.
func LoginPerplexity(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	if callbacks.OnPrompt == nil && callbacks.OnPromptContext == nil {
		return OAuthCredentials{}, errors.New("onPrompt callback is required for Perplexity login")
	}
	if ctx.Err() != nil {
		return OAuthCredentials{}, errors.New("Login cancelled")
	}

	if !perplexityBorrowDisabled() {
		ompOAuthProgress(callbacks, "Checking for Perplexity desktop app...")
		if token := extractPerplexityNativeToken(ctx); token != "" {
			ompOAuthProgress(callbacks, "Found Perplexity JWT from native app")
			return perplexityCredential(token, ""), nil
		}
		if ctx.Err() != nil {
			return OAuthCredentials{}, errors.New("Login cancelled")
		}
	}

	// Hosts without managed-browser support use email directly, as in the source.
	if callbacks.OnBrowserSession == nil {
		return httpEmailLogin(ctx, callbacks)
	}
	method, err := ompOAuthPrompt(ctx, callbacks, OAuthPrompt{
		Message:     "Login method: sso (browser) or email; blank for sso",
		Placeholder: "sso / email",
		AllowEmpty:  true,
	})
	if err != nil {
		return OAuthCredentials{}, err
	}
	if ctx.Err() != nil {
		return OAuthCredentials{}, errors.New("Login cancelled")
	}
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "", "sso":
		return browserSsoLogin(ctx, callbacks)
	case "email":
		return httpEmailLogin(ctx, callbacks)
	default:
		return OAuthCredentials{}, errors.New("Choose sso or email for Perplexity login")
	}
}

// perplexityOAuthProvider is the registered Perplexity sign-in flow.
type perplexityOAuthProvider struct{}

func (perplexityOAuthProvider) ID() string                              { return "perplexity" }
func (perplexityOAuthProvider) Name() string                            { return perplexityOAuthName }
func (perplexityOAuthProvider) IsSubscription() bool                    { return true }
func (perplexityOAuthProvider) UsesCallbackServer() bool                { return false }
func (perplexityOAuthProvider) GetAPIKey(creds OAuthCredentials) string { return creds.Access }

func (p perplexityOAuthProvider) Login(callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return p.LoginContext(context.Background(), callbacks)
}

// LoginContext runs the Perplexity sign-in with the caller's cancellation.
func (p perplexityOAuthProvider) LoginContext(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return LoginPerplexity(ctx, callbacks)
}

// RefreshToken returns the credential unchanged: the policy is `refresh "none"`
// and Perplexity sessions do not rotate client-side.
func (perplexityOAuthProvider) RefreshToken(creds OAuthCredentials) (OAuthCredentials, error) {
	return creds, nil
}

// RefreshTokenContext is RefreshToken with the caller's cancellation.
func (p perplexityOAuthProvider) RefreshTokenContext(ctx context.Context, creds OAuthCredentials) (OAuthCredentials, error) {
	return p.RefreshToken(creds)
}
