package codingagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// TestInteractiveCodexDeviceLoginUsesDeviceAuthorization exercises the actual
// interactive login entry point, not just the provider's separate device helper.
// OMP's device flow supplies the verifier and uses a fixed remote redirect;
// neither browser authorization nor pasted localhost callbacks may be requested.
func TestInteractiveCodexDeviceLoginUsesDeviceAuthorization(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	selected, displayed, exchanged := false, false, false
	token := "e30." + base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/auth":{"chatgpt_account_id":"fixture-account"}}`)) + ".fixture"
	creds, err := withMockDefaultClient(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Host != "auth.openai.com" {
			return nil, fmt.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["client_id"] != "app_EMoamEEZ73f0CkXaXp7hrann" {
				t.Error("invalid device initiation")
			}
			return jsonResponse(200, `{"device_auth_id":"fixture-device","user_code":"FIXTURE-CODE","interval":5}`), nil
		case "/api/accounts/deviceauth/token":
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["device_auth_id"] != "fixture-device" || body["user_code"] != "FIXTURE-CODE" {
				t.Error("invalid device poll")
			}
			return jsonResponse(200, `{"authorization_code":"fixture-code","code_verifier":"fixture-verifier"}`), nil
		case "/oauth/token":
			if err := r.ParseForm(); err != nil {
				return nil, err
			}
			if r.Form.Get("redirect_uri") != "https://auth.openai.com/deviceauth/callback" || r.Form.Get("code") != "fixture-code" || r.Form.Get("code_verifier") != "fixture-verifier" || r.Form.Get("grant_type") != "authorization_code" {
				t.Error("device exchange must use the remote redirect and server-issued verifier")
			}
			exchanged = true
			return jsonResponse(200, fmt.Sprintf(`{"access_token":%q,"refresh_token":"fixture-refresh","expires_in":3600}`, token)), nil
		default:
			return nil, fmt.Errorf("unexpected endpoint: %s", r.URL)
		}
	}, func() (ai.OAuthCredentials, error) {
		return loginOpenAICodex(ctx, ai.OAuthLoginCallbacks{
			OnSelect: func(ai.OAuthSelectPrompt) (string, error) {
				selected = true
				return ai.OpenAICodexDeviceCodeLoginMethod, nil
			},
			OnDeviceCode: func(info ai.OAuthDeviceCodeInfo) {
				displayed = true
				if info.UserCode != "FIXTURE-CODE" || info.VerificationURI != "https://auth.openai.com/codex/device" {
					t.Errorf("wrong device instructions: %+v", info)
				}
			},
			OnAuth: func(ai.OAuthAuthInfo) {
				t.Error("headless device login requested browser authorization")
				cancel()
			},
		})
	})
	if err != nil {
		t.Fatalf("device login failed: %v", err)
	}
	if !selected || !displayed || !exchanged || creds.AccountID != "fixture-account" {
		t.Fatalf("device login incomplete: selected=%v displayed=%v exchanged=%v account=%q", selected, displayed, exchanged, creds.AccountID)
	}
}
