package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/internal/coding/pigidentity"
)

// codexLoginJWT creates synthetic JWTs; these tests never contact OpenAI.
func codexLoginJWT(payload string) string {
	return "e30." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".fixture"
}

// codexLoginRoundTrip implements a deterministic token/device endpoint fixture.
type codexLoginRoundTrip func(*http.Request) (*http.Response, error)

// RoundTrip delegates to the fixture without using a real network transport.
func (f codexLoginRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// codexLoginMock installs a serial HTTP fixture and restores it after owned
// refresh work drains. Unexpected URLs fail rather than reaching live services.
func codexLoginMock(t *testing.T, handler codexLoginRoundTrip) {
	t.Helper()
	old := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: handler}
	t.Cleanup(func() { oauthRefreshWork.Wait(); http.DefaultClient = old })
}

// codexLoginResponse builds an owned JSON response body for http.Client callers.
func codexLoginResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

// codexLoginVirtualClock eliminates device polling delays while recording the
// intervals, keeping tests deterministic and independent of wall-clock speed.
func codexLoginVirtualClock(t *testing.T) *[]float64 {
	t.Helper()
	oldNow, oldSleep := deviceCodeNow, deviceCodeSleep
	now := float64(0)
	waits := []float64{}
	deviceCodeNow = func() float64 { return now }
	deviceCodeSleep = func(ctx context.Context, ms float64) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		waits = append(waits, ms)
		now += ms
		return nil
	}
	t.Cleanup(func() { deviceCodeNow, deviceCodeSleep = oldNow, oldSleep })
	return &waits
}

// TestOMPCodexLoginFreeTier exercises both complete login paths, including
// browser PKCE/scopes/originator and device callback/exchange redirect, with an
// email-only free token that the inherited Pi account-ID gate rejected.
func TestOMPCodexLoginFreeTier(t *testing.T) {
	for _, method := range []string{OpenAICodexBrowserLoginMethod, OpenAICodexDeviceCodeLoginMethod} {
		t.Run(method, func(t *testing.T) {
			waits := codexLoginVirtualClock(t)
			token := codexLoginJWT(`{"sub":"free-fixture","https://api.openai.com/auth":{"chatgpt_plan_type":"free"},"https://api.openai.com/profile":{"email":" Free@Example.invalid "}}`)
			exchanged := false
			codexLoginMock(t, func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method: %s", r.Method)
				}
				switch r.URL.String() {
				case codexDeviceUserCodeURL:
					var body map[string]string
					if json.NewDecoder(r.Body).Decode(&body) != nil || body["client_id"] != codexClientID {
						t.Error("invalid device client ID")
					}
					// OMP accepts a missing polling interval; use its five-second default.
					return codexLoginResponse(`{"device_auth_id":"fixture-device","user_code":"FIXTURE-CODE"}`), nil
				case codexDeviceTokenURL:
					var body map[string]string
					if json.NewDecoder(r.Body).Decode(&body) != nil || body["device_auth_id"] != "fixture-device" || body["user_code"] != "FIXTURE-CODE" {
						t.Error("invalid device polling body")
					}
					return codexLoginResponse(`{"authorization_code":"fixture-code","code_verifier":"fixture-verifier"}`), nil
				case codexTokenURL:
					exchanged = true
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					redirect := codexRedirectURI
					if method == OpenAICodexDeviceCodeLoginMethod {
						redirect = codexDeviceRedirectURI
					}
					if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("client_id") != codexClientID || r.Form.Get("redirect_uri") != redirect || r.Form.Get("code") != "fixture-code" || r.Form.Get("code_verifier") == "" {
						t.Errorf("invalid code exchange: %v", r.Form)
					}
					return codexLoginResponse(fmt.Sprintf(`{"access_token":%q,"refresh_token":"fixture-refresh","expires_in":3600}`, token)), nil
				default:
					return nil, fmt.Errorf("unexpected URL: %s", r.URL)
				}
			})
			var state string
			promptedDevice := false
			hostName := "pi"
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			creds, err := (CodexOAuthProvider{}).LoginContext(ctx, OAuthLoginCallbacks{
				AgentName: &hostName,
				OnSelect:  func(p OAuthSelectPrompt) (string, error) { return method, nil },
				OnAuth: func(info OAuthAuthInfo) {
					parsed, err := url.Parse(info.URL)
					if err != nil {
						t.Fatal(err)
					}
					q := parsed.Query()
					state = q.Get("state")
					if q.Get("originator") != "codex_cli_rs" || q.Get("client_id") != codexClientID || q.Get("redirect_uri") != codexRedirectURI || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || state == "" {
						t.Errorf("invalid browser auth URL: %s", info.URL)
					}
					for _, scope := range []string{"openid", "profile", "email", "offline_access", "api.connectors.read", "api.connectors.invoke"} {
						if !strings.Contains(" "+q.Get("scope")+" ", " "+scope+" ") {
							t.Errorf("missing scope %s", scope)
						}
					}
				},
				OnManualCodeInputContext: func(context.Context) (string, error) { return "fixture-code#" + state, nil },
				OnDeviceCode: func(info OAuthDeviceCodeInfo) {
					promptedDevice = true
					if info.UserCode != "FIXTURE-CODE" || info.VerificationURI != codexDeviceVerificationURI {
						t.Errorf("invalid device prompt: %+v", info)
					}
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !exchanged || creds.AccountID != "" || codexProfileString(creds, "email") != "free@example.invalid" || codexProfileString(creds, "orgName") != "free" {
				t.Fatalf("free token not accepted: %+v", creds)
			}
			if method == OpenAICodexDeviceCodeLoginMethod && (!promptedDevice || !reflect.DeepEqual(*waits, []float64{5000})) {
				t.Fatalf("device prompt/timing: %v %v", promptedDevice, *waits)
			}
			credential, err := credentialFromOAuth(creds)
			if err != nil {
				t.Fatal(err)
			}
			key := deriveAccountKey(credential)
			if key == nil || *key != "free@example.invalid" {
				t.Fatalf("free account identity: %v", key)
			}
			store, _ := openTestStore(t)
			if _, err := store.SaveAccount(ctx, "openai-codex", key, credential); err != nil {
				t.Fatal(err)
			}
			saved, err := store.ReadAccount(ctx, "openai-codex", key)
			if err != nil || saved == nil || credentialClaim(*saved, "email") != "free@example.invalid" {
				t.Fatalf("free account was not persisted: %+v %v", saved, err)
			}
		})
	}
}

// TestOMPCodexTokenProfileFallback covers access/id-token precedence, UTF-8 and
// base64url decoding, no-identity rejection and the rule against storing ID tokens.
func TestOMPCodexTokenProfileFallback(t *testing.T) {
	access := codexLoginJWT(`{"https://api.openai.com/profile":{"email":"Access@Example.invalid"}}`)
	id := codexLoginJWT(`{"https://api.openai.com/auth":{"chatgpt_account_id":"workspace-fixture","chatgpt_plan_type":"free"},"https://api.openai.com/profile":{"email":"ID@Example.invalid"}}`)
	codexLoginMock(t, func(r *http.Request) (*http.Response, error) {
		return codexLoginResponse(fmt.Sprintf(`{"access_token":%q,"refresh_token":"fixture-refresh","expires_in":3600,"id_token":%q}`, access, id)), nil
	})
	c, err := codexCredentialsFromToken(exchangeCodexAuthorizationCode(t.Context(), "code", "verifier", codexRedirectURI))
	if err != nil || c.AccountID != "workspace-fixture" || codexProfileString(c, "orgId") != "workspace-fixture" || codexProfileString(c, "orgName") != "free" || codexProfileString(c, "email") != "access@example.invalid" {
		t.Fatalf("incorrect profile fallback: %+v %v", c, err)
	}
	for key := range c.Extra {
		if strings.Contains(strings.ToLower(key), "token") {
			t.Fatalf("raw token persisted in %s", key)
		}
	}
	credential, err := credentialFromOAuth(c)
	if err != nil {
		t.Fatal(err)
	}
	auth := codexRequestAuth(credential)
	if auth.Headers == nil || *auth.Headers["ChatGPT-Account-Id"] != "workspace-fixture" {
		t.Fatal("ID-token workspace not forwarded")
	}
	unicodeToken := codexLoginJWT(`{"https://api.openai.com/auth":{"chatgpt_account_id":"workspace-💡"}}`)
	if CodexAccountID(unicodeToken) != "workspace-💡" {
		t.Fatal("JWT not decoded as UTF-8/base64url")
	}
	for _, bad := range []string{"not-jwt", codexLoginJWT(`{"sub":"fixture"}`), codexLoginJWT(`{"https://api.openai.com/profile":{"email":42}}`)} {
		c := enrichCodexProfile(OAuthCredentials{Access: bad}, "", true)
		if _, err := codexCredentialsFromToken(c, nil); err == nil {
			t.Fatalf("accepted token with no account/email: %s", bad)
		}
	}
}

// TestOMPCodexRefreshKeepsIdentity verifies refreshes lacking account/email
// claims preserve the selected identity and do not create an anonymous account.
func TestOMPCodexRefreshKeepsIdentity(t *testing.T) {
	rotated := codexLoginJWT(`{"sub":"fixture"}`)
	codexLoginMock(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != codexTokenURL {
			return nil, errors.New("unexpected endpoint")
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh" {
			t.Error("invalid refresh form")
		}
		return codexLoginResponse(fmt.Sprintf(`{"access_token":%q,"refresh_token":"new-refresh","expires_in":3600}`, rotated)), nil
	})
	for _, workspace := range []string{"", "workspace"} {
		stored := enrichCodexProfile(OAuthCredentials{Access: codexLoginJWT(`{"https://api.openai.com/profile":{"email":"free@example.invalid"}}`), Refresh: "old-refresh"}, "", true)
		stored.AccountID = workspace
		if workspace != "" {
			stored.Extra["orgId"] = json.RawMessage(`"workspace"`)
		}
		c, err := (CodexOAuthProvider{}).RefreshTokenContext(t.Context(), stored)
		if err != nil || c.Access != rotated || c.Refresh != "new-refresh" || c.AccountID != workspace || codexProfileString(c, "email") != "free@example.invalid" || codexProfileString(c, "orgId") != workspace {
			t.Fatalf("refresh lost identity: %+v %v", c, err)
		}
	}
}

// TestOMPCodexDevicePolling waits before polling and keeps OMP's safety margin;
// pending responses cannot cause a tight loop or continue beyond the attempt cap.
func TestOMPCodexDevicePolling(t *testing.T) {
	for _, finish := range []bool{true, false} {
		t.Run(fmt.Sprint(finish), func(t *testing.T) {
			waits := codexLoginVirtualClock(t)
			polls := 0
			codexLoginMock(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != codexDeviceTokenURL {
					return nil, errors.New("unexpected endpoint")
				}
				polls++
				if finish && polls == 2 {
					return codexLoginResponse(`{"authorization_code":"code","code_verifier":"verifier"}`), nil
				}
				response := codexLoginResponse(`{}`)
				response.StatusCode = 403
				return response, nil
			})
			_, err := pollCodexDeviceAuth(t.Context(), codexDeviceAuthInfo{deviceAuthID: "device", userCode: "code", intervalSeconds: 1})
			if finish && (err != nil || polls != 2 || !reflect.DeepEqual(*waits, []float64{4000, 4000})) {
				t.Fatalf("incorrect device timing: %d %v %v", polls, *waits, err)
			}
			if !finish && (err == nil || polls != codexDeviceMaxPolls) {
				t.Fatalf("unbounded device polling: %d %v", polls, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := pollCodexDeviceAuth(ctx, codexDeviceAuthInfo{intervalSeconds: 5}); err == nil {
		t.Fatal("cancelled polling continued")
	}
}

// TestOMPCodexNativeOriginator pins shared browser/HTTP/WebSocket identity and
// omits the workspace header entirely for an email-only account.
func TestOMPCodexNativeOriginator(t *testing.T) {
	if pigidentity.CodexOriginator != "codex_cli_rs" {
		t.Fatal("Codex still uses host branding")
	}
	headers := codexWebSocketHeaders(ClientIdentityTarget{}, nil, nil, nil, "fixture", "", "request")
	if headers.Get("originator") != "codex_cli_rs" {
		t.Fatal("WebSocket originator mismatch")
	}
	if _, present := headers["Chatgpt-Account-Id"]; present {
		t.Fatal("empty workspace header sent")
	}
}
