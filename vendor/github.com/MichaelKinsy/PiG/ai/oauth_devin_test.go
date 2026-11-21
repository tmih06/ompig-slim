package ai

// Devin OAuth tests: CLI credential adoption, manual-paste and browser PKCE
// login against httptest fakes. Internal package for provider knobs.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func devinTestJWT(expSeconds int64) string {
	encode := func(value any) string {
		raw, _ := json.Marshal(value)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return encode(map[string]string{"alg": "none"}) + "." +
		encode(map[string]any{"exp": expSeconds, "sub": "user-1"}) + ".sig"
}

func devinTestProviderWith(t *testing.T, mutate func(*devinOAuthProvider)) *devinOAuthProvider {
	t.Helper()
	provider := newDevinOAuthProvider()
	provider.client = http.DefaultClient
	if mutate != nil {
		mutate(provider)
	}
	return provider
}

func TestDevinOAuthCLIAdoption(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.toml")
	content := "windsurf_api_key = \"cli-key-123\"\napi_server_url = \"https://server.codeium.com\"\n"
	if err := os.WriteFile(credPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := devinTestProviderWith(t, func(p *devinOAuthProvider) {
		p.cliPaths = func() []string { return []string{credPath, filepath.Join(dir, "missing.toml")} }
		p.identityBaseURL = "http://127.0.0.1:1"
	})
	creds, err := provider.Login(OAuthLoginCallbacks{
		OnSelectContext: func(ctx context.Context, prompt OAuthSelectPrompt) (string, error) {
			return devinLoginCLIMethod, nil
		},
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if creds.Access != "cli-key-123" || creds.Refresh != "cli-key-123" {
		t.Fatalf("credential %#v", creds)
	}
	var serverURL string
	if raw, ok := creds.Extra["apiServerUrl"]; ok {
		_ = json.Unmarshal(raw, &serverURL)
	}
	if serverURL != "https://server.codeium.com" {
		t.Fatalf("apiServerUrl %q", serverURL)
	}
	// An adopted windsurf_api_key is a non-expiring seat key.
	if got := creds.ExpiresMillis(); got != ompNeverExpiresMs {
		t.Fatalf("CLI seat key expiry %v, want never-expires %v", got, float64(ompNeverExpiresMs))
	}
}

func TestDevinOAuthCLIMissingKey(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.toml")
	if err := os.WriteFile(credPath, []byte("other = \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := devinTestProviderWith(t, func(p *devinOAuthProvider) {
		p.cliPaths = func() []string { return []string{credPath} }
	})
	_, err := provider.Login(OAuthLoginCallbacks{
		OnSelectContext: func(ctx context.Context, prompt OAuthSelectPrompt) (string, error) {
			return devinLoginCLIMethod, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "windsurf_api_key") {
		t.Fatalf("expected missing-key error, got %v", err)
	}
}

func TestDevinOAuthCLIUnsupportedHost(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.toml")
	content := "windsurf_api_key = \"k\"\napi_server_url = \"https://evil.example.com\"\n"
	if err := os.WriteFile(credPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := devinTestProviderWith(t, func(p *devinOAuthProvider) {
		p.cliPaths = func() []string { return []string{credPath} }
	})
	_, err := provider.Login(OAuthLoginCallbacks{
		OnSelectContext: func(ctx context.Context, prompt OAuthSelectPrompt) (string, error) {
			return devinLoginCLIMethod, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not a supported Devin server") {
		t.Fatalf("expected host error, got %v", err)
	}
}

func TestDevinOAuthCLINoFile(t *testing.T) {
	provider := devinTestProviderWith(t, func(p *devinOAuthProvider) {
		p.cliPaths = func() []string { return []string{filepath.Join(t.TempDir(), "none.toml")} }
	})
	_, err := provider.Login(OAuthLoginCallbacks{
		OnSelectContext: func(ctx context.Context, prompt OAuthSelectPrompt) (string, error) {
			return devinLoginCLIMethod, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "no Devin CLI credentials") {
		t.Fatalf("expected no-file error, got %v", err)
	}
}

func TestDevinOAuthManualPasteCodeState(t *testing.T) {
	var gotCode, gotVerifier string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotCode, gotVerifier = body["code"], body["code_verifier"]
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"token":%q}`, devinTestJWT(time.Now().Add(time.Hour).Unix()))
	}))
	defer tokenServer.Close()

	provider := devinTestProviderWith(t, func(p *devinOAuthProvider) {
		p.tokenURL = tokenServer.URL
		p.identityBaseURL = "http://127.0.0.1:1"
	})
	var authURL string
	creds, err := provider.Login(OAuthLoginCallbacks{
		OnAuth: func(info OAuthAuthInfo) { authURL = info.URL },
		OnSelectContext: func(ctx context.Context, prompt OAuthSelectPrompt) (string, error) {
			return devinLoginBrowserMethod, nil
		},
		OnManualCodeInput: func() (string, error) {
			// Extract state from the authorize URL and paste code#state.
			state := ""
			if parsed, err := url.ParseQuery(strings.SplitN(authURL, "?", 2)[1]); err == nil {
				state = parsed.Get("state")
			}
			return "auth-code-1#" + state, nil
		},
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if gotCode != "auth-code-1" {
		t.Fatalf("code %q", gotCode)
	}
	if gotVerifier == "" {
		t.Fatal("empty code_verifier")
	}
	if creds.Access == "" || !strings.Contains(creds.Access, ".") {
		t.Fatalf("credential %#v", creds)
	}
}

func TestDevinOAuthManualStateMismatch(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("token endpoint must not be hit on state mismatch")
	}))
	defer tokenServer.Close()
	provider := devinTestProviderWith(t, func(p *devinOAuthProvider) {
		p.tokenURL = tokenServer.URL
	})
	_, err := provider.Login(OAuthLoginCallbacks{
		OnSelectContext: func(ctx context.Context, prompt OAuthSelectPrompt) (string, error) {
			return devinLoginBrowserMethod, nil
		},
		OnManualCodeInput: func() (string, error) {
			return "code#wrong-state", nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("expected state mismatch, got %v", err)
	}
}

func TestDevinOAuthBrowserCallback(t *testing.T) {
	var gotCode, gotVerifier string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotCode, gotVerifier = body["code"], body["code_verifier"]
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"token":%q}`, devinTestJWT(time.Now().Add(time.Hour).Unix()))
	}))
	defer tokenServer.Close()

	provider := devinTestProviderWith(t, func(p *devinOAuthProvider) {
		p.tokenURL = tokenServer.URL
		p.port = 0 // free port for the test
		p.identityBaseURL = "http://127.0.0.1:1"
	})
	authURLCh := make(chan string, 1)
	done := make(chan struct {
		creds OAuthCredentials
		err   error
	}, 1)
	go func() {
		creds, err := provider.Login(OAuthLoginCallbacks{
			OnAuth: func(info OAuthAuthInfo) { authURLCh <- info.URL },
			OnSelectContext: func(ctx context.Context, prompt OAuthSelectPrompt) (string, error) {
				return devinLoginBrowserMethod, nil
			},
		})
		done <- struct {
			creds OAuthCredentials
			err   error
		}{creds, err}
	}()
	authURL := <-authURLCh
	// Extract redirect_uri + state, then hit the callback like a browser.
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("authorize url: %v", err)
	}
	q := parsed.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("response_type") != "code" || q.Get("prompt") != "select_account" {
		t.Fatalf("authorize query %v", q)
	}
	if !strings.Contains(q.Get("code_challenge"), "") || q.Get("code_challenge") == "" {
		t.Fatal("missing code_challenge")
	}
	redirect := q.Get("redirect_uri")
	state := q.Get("state")
	cb, err := url.Parse(redirect)
	if err != nil {
		t.Fatalf("redirect uri: %v", err)
	}
	cb.RawQuery = "code=browser-code&state=" + state
	resp, err := http.Get(cb.String())
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	_ = resp.Body.Close()
	out := <-done
	if out.err != nil {
		t.Fatalf("login: %v", out.err)
	}
	if gotCode != "browser-code" {
		t.Fatalf("code %q", gotCode)
	}
	if gotVerifier == "" {
		t.Fatal("empty verifier")
	}
	if out.creds.Access == "" {
		t.Fatal("empty access token")
	}
}

func TestDevinOAuthJWTExpiry(t *testing.T) {
	exp := time.Now().Add(2 * time.Hour).Unix()
	creds := devinTestProviderWith(t, nil).credentialFromToken(devinTestJWT(exp))
	want := exp*1000 - devinTokenExpirySkewMillis
	if creds.Expires != want {
		t.Fatalf("expires %d want %d", creds.Expires, want)
	}
	// Non-JWT token falls back to ~1 year.
	creds2 := devinTestProviderWith(t, nil).credentialFromToken("opaque-token")
	if creds2.Expires < time.Now().Add(300*24*time.Hour).Unix() {
		t.Fatalf("fallback expiry %d", creds2.Expires)
	}
}

func TestDevinOAuthIdentityEnrichment(t *testing.T) {
	// Fake GetUserStatus: response{1: userStatus{7:email, 36:userID, 5:teamID}}.
	userStatus := &devinPbWriter{}
	userStatus.appendString(7, "dev@example.com")
	userStatus.appendString(36, "user-42")
	userStatus.appendString(5, "team-9")
	response := &devinPbWriter{}
	response.appendMessage(1, userStatus.buffer)

	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/proto")
		_, _ = w.Write(response.buffer)
	}))
	defer statusServer.Close()

	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.toml")
	content := fmt.Sprintf("windsurf_api_key = \"id-key\"\napi_server_url = \"%s\"\n", statusServer.URL)
	if err := os.WriteFile(credPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := devinTestProviderWith(t, func(p *devinOAuthProvider) {
		p.cliPaths = func() []string { return []string{credPath} }
		p.client = statusServer.Client()
		p.acceptedHosts = map[string]bool{"127.0.0.1": true}
	})
	creds, err := provider.Login(OAuthLoginCallbacks{
		OnSelectContext: func(ctx context.Context, prompt OAuthSelectPrompt) (string, error) {
			return devinLoginCLIMethod, nil
		},
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	var email string
	if raw, ok := creds.Extra["email"]; ok {
		_ = json.Unmarshal(raw, &email)
	}
	if email != "dev@example.com" {
		t.Fatalf("email %q (extra %#v account %q)", email, creds.Extra, creds.AccountID)
	}
	// accountId is org-scoped so distinct org logins stay distinct accounts.
	if creds.AccountID != "user-42|org:team-9" {
		t.Fatalf("account id %q", creds.AccountID)
	}
}

func TestDevinOAuthIdentityFailureKeepsCredential(t *testing.T) {
	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer statusServer.Close()
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.toml")
	content := fmt.Sprintf("windsurf_api_key = \"id-key\"\napi_server_url = \"%s\"\n", statusServer.URL)
	if err := os.WriteFile(credPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := devinTestProviderWith(t, func(p *devinOAuthProvider) {
		p.cliPaths = func() []string { return []string{credPath} }
		p.client = statusServer.Client()
		p.acceptedHosts = map[string]bool{"127.0.0.1": true}
	})
	creds, err := provider.Login(OAuthLoginCallbacks{
		OnSelectContext: func(ctx context.Context, prompt OAuthSelectPrompt) (string, error) {
			return devinLoginCLIMethod, nil
		},
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if creds.Access != "id-key" {
		t.Fatalf("credential lost: %#v", creds)
	}
}

func TestDevinOAuthRefreshToken(t *testing.T) {
	provider := devinTestProviderWith(t, nil)

	// An expired browser session fails with an actionable re-login error.
	expired := OAuthCredentials{Access: "session", Refresh: "session"}
	expired.SetExpiresMillis(float64(nowMillis()) - 1000)
	if _, err := provider.RefreshToken(expired); err == nil || !strings.Contains(err.Error(), "Devin session expired; run /login again") {
		t.Fatalf("expired refresh err %v", err)
	}

	// The never-expires CLI seat key passes through unchanged.
	seat := ompKeyCredential("seat-key")
	out, err := provider.RefreshToken(seat)
	if err != nil || out.Access != "seat-key" {
		t.Fatalf("seat refresh %#v err %v", out, err)
	}

	// A still-valid session passes through unchanged.
	valid := OAuthCredentials{Access: "session"}
	valid.SetExpiresMillis(float64(nowMillis()) + float64(time.Hour.Milliseconds()))
	if out, err = provider.RefreshToken(valid); err != nil || out.Access != "session" {
		t.Fatalf("valid refresh %#v err %v", out, err)
	}
}
