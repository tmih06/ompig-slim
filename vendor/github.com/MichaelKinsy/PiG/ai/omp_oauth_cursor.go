package ai

// Native port of OMP's Cursor OAuth sign-in (packages/ai/src/registry/oauth/cursor.ts
// plus its auth "cursor" policy: login and refresh both custom hooks).
//
// Cursor's desktop sign-in is a PKCE token poll rather than a redirect: the
// browser authorizes a uuid+challenge pair, and the CLI polls a poll endpoint
// until the account confirms. The resulting access token is a JWT whose `sub`
// names the account; the account profile RPC supplies the email used for
// account labels and policies.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	cursorLoginURL    = "https://cursor.com/loginDeepControl"
	cursorPollURL     = "https://api2.cursor.sh/auth/poll"
	cursorRefreshURL  = "https://api2.cursor.sh/oauth/token"
	cursorClientID    = "KbZUR41cY7W6zRSdpSUJ7I7mLYBKOCmB" // gitleaks:allow -- public OAuth client ID the Cursor IDE uses
	cursorOAuthName   = "Cursor (Claude, GPT, etc.)"
	cursorPollMax     = 150
	cursorPollBase    = 1000 * time.Millisecond
	cursorPollMaxWait = 10 * time.Second
	cursorPollBackoff = 1.2
	cursorProfileWait = 3 * time.Second
)

// cursorProfileEndpoint is the account-profile RPC; a variable so tests can point
// it at a stub server.
var cursorProfileEndpoint = "https://api2.cursor.sh/aiserver.v1.DashboardService/GetMe"

// cursorPollSleep is the poll delay, replaceable by tests so the backoff
// schedule can be observed without waiting.
var cursorPollSleep = func(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errors.New("Login cancelled")
	case <-timer.C:
		return nil
	}
}

// CursorAuthParams is one Cursor authorization attempt: the PKCE verifier the poll
// endpoint needs, the challenge and uuid the browser URL carries, and that URL.
type CursorAuthParams struct {
	Verifier  string
	Challenge string
	UUID      string
	LoginURL  string
}

// GenerateCursorAuthParams builds a Cursor authorization URL and its verifier.
func GenerateCursorAuthParams() (CursorAuthParams, error) {
	pkce, err := GeneratePKCE()
	if err != nil {
		return CursorAuthParams{}, err
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return CursorAuthParams{}, err
	}
	query := url.Values{}
	query.Set("challenge", pkce.Challenge)
	query.Set("uuid", id.String())
	query.Set("mode", "login")
	query.Set("redirectTarget", "cli")
	return CursorAuthParams{
		Verifier:  pkce.Verifier,
		Challenge: pkce.Challenge,
		UUID:      id.String(),
		LoginURL:  cursorLoginURL + "?" + query.Encode(),
	}, nil
}

// CursorPollOutcome is the token pair the poll endpoint yields.
type CursorPollOutcome struct {
	AccessToken  string
	RefreshToken string
}

// PollCursorAuth polls the Cursor authorization endpoint until the browser
// session confirms or the attempt budget runs out.
//
// A 404 means "not confirmed yet" and keeps polling with a growing delay; any
// other failure counts toward a three-strike limit, matching upstream so a
// flapping network does not burn the whole budget.
func PollCursorAuth(ctx context.Context, id, verifier string) (CursorPollOutcome, error) {
	delay := cursorPollBase
	consecutiveErrors := 0
	endpoint := cursorPollURL + "?uuid=" + url.QueryEscape(id) + "&verifier=" + url.QueryEscape(verifier)
	for range cursorPollMax {
		if err := cursorPollSleep(ctx, delay); err != nil {
			return CursorPollOutcome{}, err
		}
		status, payload, _, err := ompOAuthRequest(ctx, "GET", endpoint, map[string]string{"Accept": "application/json"}, nil)
		if err != nil {
			if ctx.Err() != nil {
				return CursorPollOutcome{}, errors.New("Login cancelled")
			}
			consecutiveErrors++
			if consecutiveErrors >= 3 {
				return CursorPollOutcome{}, errors.New("Too many consecutive errors during Cursor auth polling")
			}
			continue
		}
		if status == 404 {
			consecutiveErrors = 0
			delay = time.Duration(float64(delay) * cursorPollBackoff)
			if delay > cursorPollMaxWait {
				delay = cursorPollMaxWait
			}
			continue
		}
		if status >= 200 && status < 300 {
			body := ompOAuthObject(payload)
			access := strings.TrimSpace(ompOAuthString(body["accessToken"]))
			if access == "" {
				consecutiveErrors++
				if consecutiveErrors >= 3 {
					return CursorPollOutcome{}, errors.New("Too many consecutive errors during Cursor auth polling")
				}
				continue
			}
			return CursorPollOutcome{AccessToken: access, RefreshToken: ompOAuthString(body["refreshToken"])}, nil
		}
		return CursorPollOutcome{}, fmt.Errorf("Poll failed: %d", status)
	}
	return CursorPollOutcome{}, errors.New("Cursor authentication polling timeout")
}

// cursorTokenExpiry is a Cursor JWT's `exp` minus a five minute margin, or an hour
// from now when the token carries none (upstream getTokenExpiry).
func cursorTokenExpiry(accessToken string) int64 {
	if claims, ok := ompOAuthJWTClaims(accessToken); ok {
		if exp, ok := ompOAuthNumber(claims["exp"]); ok {
			return int64(exp*1000) - 5*60*1000
		}
	}
	return time.Now().Add(time.Hour).UnixMilli()
}

// cursorAccessTokenSubject is the full `sub` claim of a Cursor access token, such
// as `github|1245379` or `auth0|user_…`.
func cursorAccessTokenSubject(accessToken string) string {
	claims, ok := ompOAuthJWTClaims(accessToken)
	if !ok {
		return ""
	}
	return strings.TrimSpace(ompOAuthString(claims["sub"]))
}

// ExtractCursorAccessTokenUserID is the account identifier a Cursor access token
// names: the part after `|` in its `sub` claim, or the whole claim when it has
// none. It is what the web-session cookie pairs with the token.
func ExtractCursorAccessTokenUserID(accessToken string) string {
	subject := cursorAccessTokenSubject(accessToken)
	if subject == "" {
		return ""
	}
	if index := strings.IndexByte(subject, '|'); index >= 0 {
		return strings.TrimSpace(subject[index+1:])
	}
	return subject
}

// CursorSessionHeaders presents an access token to cursor.com as the user's web
// session, which is how the account APIs are read.
func CursorSessionHeaders(userID, accessToken string) map[string]string {
	return map[string]string{
		"Accept": "application/json",
		"Cookie": "WorkosCursorSessionToken=" + url.QueryEscape(userID+"::"+accessToken),
	}
}

// cursorConnectRequest is a unary Connect RPC call with an empty message,
// authenticated by the access token as a bearer token.
func cursorConnectRequest(accessToken string) map[string]string {
	return map[string]string{
		"Accept":                   "application/json",
		"Authorization":            "Bearer " + accessToken,
		"Connect-Protocol-Version": "1",
		"Content-Type":             "application/json",
	}
}

// FetchCursorAccountEmail reads the email of the Cursor account that owns
// accessToken. It is empty when the token names no subject or the profile names
// another subject.
func FetchCursorAccountEmail(ctx context.Context, accessToken string) (string, error) {
	subject := cursorAccessTokenSubject(accessToken)
	if subject == "" {
		return "", nil
	}
	profileCtx, cancel := context.WithTimeout(ctx, cursorProfileWait)
	defer cancel()
	status, payload, _, err := ompOAuthRequest(profileCtx, "POST", cursorProfileEndpoint, cursorConnectRequest(accessToken), []byte("{}"))
	if err != nil {
		if ctx.Err() != nil {
			return "", errors.New("Login cancelled")
		}
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("Cursor profile request failed: %d", status)
	}
	body := ompOAuthObject(payload)
	if body == nil || ompOAuthString(body["authId"]) != subject {
		return "", nil
	}
	return strings.TrimSpace(ompOAuthString(body["email"])), nil
}

// withCursorAccountEmail attaches the account email so account policies and
// pickers can name this login. A failed lookup leaves the credential as it was,
// because the tokens are already valid and the profile is only a label.
func withCursorAccountEmail(ctx context.Context, credentials OAuthCredentials) OAuthCredentials {
	email, err := FetchCursorAccountEmail(ctx, credentials.Access)
	if err != nil && ctx.Err() != nil {
		return credentials
	}
	ompOAuthSetEmail(&credentials, email)
	return credentials
}

// LoginCursor runs the interactive Cursor loop and returns the token pair.
func LoginCursor(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	params, err := GenerateCursorAuthParams()
	if err != nil {
		return OAuthCredentials{}, err
	}
	if callbacks.OnAuth != nil {
		callbacks.OnAuth(OAuthAuthInfo{URL: params.LoginURL})
	}
	ompOAuthProgress(callbacks, "Waiting for browser authentication...")
	outcome, err := PollCursorAuth(ctx, params.UUID, params.Verifier)
	if err != nil {
		return OAuthCredentials{}, err
	}
	return withCursorAccountEmail(ctx, OAuthCredentials{
		Access:  outcome.AccessToken,
		Refresh: outcome.RefreshToken,
		Expires: cursorTokenExpiry(outcome.AccessToken),
	}), nil
}

// RefreshCursorToken renews a Cursor session.
//
// Cursor answers a session it will not renew with 200, an empty token and
// `shouldLogout`, which is reported as invalid_grant. The renewal usually returns
// only an access token, so the stored refresh token is kept unless a new one is
// issued.
func RefreshCursorToken(ctx context.Context, refreshToken string) (OAuthCredentials, error) {
	body, err := ompOAuthJSONBody(map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     cursorClientID,
		"refresh_token": refreshToken,
	})
	if err != nil {
		return OAuthCredentials{}, err
	}
	status, payload, _, err := ompOAuthRequest(ctx, "POST", cursorRefreshURL, map[string]string{"Content-Type": "application/json"}, body)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if status < 200 || status >= 300 {
		return OAuthCredentials{}, fmt.Errorf("Cursor token refresh failed: %s", fetchBodyText(payload))
	}
	decoded := ompOAuthObject(payload)
	if logout, ok := decoded["shouldLogout"].(bool); ok && logout {
		return OAuthCredentials{}, errors.New("invalid_grant: Cursor ended this session; run /login cursor again")
	}
	access := strings.TrimSpace(ompOAuthString(decoded["access_token"]))
	if access == "" {
		return OAuthCredentials{}, errors.New("Cursor token refresh returned no access token")
	}
	refresh := strings.TrimSpace(ompOAuthString(decoded["refresh_token"]))
	if refresh == "" {
		refresh = refreshToken
	}
	return OAuthCredentials{Access: access, Refresh: refresh, Expires: cursorTokenExpiry(access)}, nil
}

// cursorOAuthProvider is the registered Cursor sign-in flow.
type cursorOAuthProvider struct{}

func (cursorOAuthProvider) ID() string                              { return "cursor" }
func (cursorOAuthProvider) Name() string                            { return cursorOAuthName }
func (cursorOAuthProvider) IsSubscription() bool                    { return false }
func (cursorOAuthProvider) UsesCallbackServer() bool                { return false }
func (cursorOAuthProvider) GetAPIKey(creds OAuthCredentials) string { return creds.Access }

func (p cursorOAuthProvider) Login(callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return p.LoginContext(context.Background(), callbacks)
}

// LoginContext runs the Cursor sign-in with the caller's cancellation.
func (p cursorOAuthProvider) LoginContext(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return LoginCursor(ctx, callbacks)
}

// RefreshToken refreshes without cancellation. Prefer RefreshTokenContext.
func (p cursorOAuthProvider) RefreshToken(creds OAuthCredentials) (OAuthCredentials, error) {
	return p.RefreshTokenContext(context.Background(), creds)
}

// RefreshTokenContext refreshes a Cursor session, re-reading the account email
// for credentials stored before it was captured.
func (p cursorOAuthProvider) RefreshTokenContext(ctx context.Context, creds OAuthCredentials) (OAuthCredentials, error) {
	refreshed, err := RefreshCursorToken(ctx, creds.Refresh)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if len(creds.Extra["email"]) > 0 {
		return refreshed, nil
	}
	return withCursorAccountEmail(ctx, refreshed), nil
}

// IsCursorTokenExpiringSoon reports whether a Cursor access token expires within
// thresholdSeconds, treating an unreadable token as expiring.
func IsCursorTokenExpiringSoon(accessToken string, thresholdSeconds int64) bool {
	claims, ok := ompOAuthJWTClaims(accessToken)
	if !ok {
		return true
	}
	exp, ok := ompOAuthNumber(claims["exp"])
	if !ok {
		return true
	}
	current := time.Now().Unix()
	return int64(exp)-current < thresholdSeconds
}
