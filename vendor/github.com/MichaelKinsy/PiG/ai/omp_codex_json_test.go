package ai

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/extensions/sdk/json"
)

// codexJSONTestJWT creates synthetic, unsigned metadata; no real credentials are used.
func codexJSONTestJWT(account string, expiry time.Time) string {
	payload := fmt.Sprintf(`{"exp":%d,"https://api.openai.com/auth":{"chatgpt_account_id":%q,"chatgpt_plan_type":"plus"}}`, expiry.Unix(), account)
	return "e30." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".fixture"
}

// TestCodexJSONImport verifies the external input boundary and expiry precedence.
func TestCodexJSONImport(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	token := codexJSONTestJWT("acct", now.Add(2*time.Hour))
	input := fmt.Sprintf(`{
  "accessToken":%q,"refreshToken":"fixture-refresh",
  "expiresAt":%q,"expiresIn":864000,"idToken":"discard-me",
  "providerSpecificData":{"chatgptAccountId":"acct","chatgptPlanType":"plus"},
  "email":"discard@example.invalid","name":"discard-me"
 }`, token, now.Add(24*time.Hour).Format(time.RFC3339))
	credential, err := parseCodexJSON(input, now)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Access != token || credential.Refresh != "fixture-refresh" || credential.AccountID != "acct" || credential.Expires != now.Add(2*time.Hour).UnixMilli() || len(credential.Extra) != 0 {
		t.Fatalf("incorrect minimal credential: %+v", credential)
	}
	stored, err := credentialFromOAuth(credential)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(stored)
	if err != nil || strings.Contains(string(encoded), "discard") || strings.Contains(string(encoded), "idToken") {
		t.Fatalf("unneeded fields persisted: %s (%v)", encoded, err)
	}
	for _, bad := range []string{
		`null`, `[]`, `{`, `{"accessToken":3}`,
		`{"accessToken":"fixture","expiresIn":-1}`,
		`{"accessToken":"fixture","providerSpecificData":{"chatgptAccountId":"acct"},"expiresIn":1e30}`,
		fmt.Sprintf(`{"accessToken":%q,"providerSpecificData":{"chatgptAccountId":"other"}}`, token),
		fmt.Sprintf(`{"accessToken":%q,"expiresAt":"not-a-date"}`, token),
		fmt.Sprintf(`{"accessToken":%q,"expiresAt":%q}`, token, now.Add(-time.Hour).Format(time.RFC3339)),
	} {
		if _, err := parseCodexJSON(bad, now); err == nil {
			t.Fatalf("accepted invalid import: %s", bad)
		}
	}
	for _, input := range []string{
		fmt.Sprintf(`{"accessToken":%q}`, token),
		`{"accessToken":"opaque-fixture","expiresIn":7200,"providerSpecificData":{"chatgptAccountId":"acct"}}`,
	} {
		if _, err := parseCodexJSON(input, now); err != nil {
			t.Fatalf("valid minimal import failed: %v", err)
		}
	}
}

// TestCodexJSONRegistrationAndSecretPrompt exercises the actual built-in auth
// adapter, model catalog, entitlement rules and usage-header registration.
func TestCodexJSONRegistrationAndSecretPrompt(t *testing.T) {
	provider, ok := GetOAuthProvider(codexJSONProviderID)
	if !ok || provider.Name() != codexJSONName {
		t.Fatal("import provider not registered")
	}
	auth, err := BuiltinProviderAuth(codexJSONProviderID)
	if err != nil || auth.OAuth == nil || auth.APIKey != nil {
		t.Fatalf("auth not wired: %+v %v", auth, err)
	}
	input := fmt.Sprintf(`{"accessToken":%q}`, codexJSONTestJWT("acct", time.Now().Add(time.Hour)))
	credential, err := auth.OAuth.Login(t.Context(), AuthInteraction{Prompt: func(ctx context.Context, prompt AuthPrompt) (string, error) {
		if _, secret := prompt.(AuthSecretPrompt); !secret {
			t.Fatalf("credential prompt is not secret: %T", prompt)
		}
		return input, nil
	}}, LoginOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := auth.OAuth.ToAuth(credential)
	if err != nil || result.APIKey != credential.Access || *result.Headers["ChatGPT-Account-Id"] != "acct" {
		t.Fatalf("incorrect request auth: %+v %v", result, err)
	}
	models := GetBuiltinModels(codexJSONProviderID)
	originals := GetBuiltinModels("openai-codex")
	if len(models) == 0 || len(models) != len(originals) {
		t.Fatal("Codex models not mirrored")
	}
	for i, model := range models {
		if model.ProviderID() != codexJSONProviderID || model.ID != originals[i].ID || model.ProviderMeta.API != originals[i].ProviderMeta.API {
			t.Fatalf("incorrect catalog model: %+v", model)
		}
		encoded, ok, err := CatalogModelJSON(model)
		if err != nil || !ok || !strings.Contains(string(encoded), `"provider":"codex-json"`) {
			t.Fatalf("serialized model has wrong provider: %s %v", encoded, err)
		}
	}
	assembled := builtinProvider(codexJSONProviderID)
	if assembled.Name != codexJSONName || assembled.Auth.OAuth == nil {
		t.Fatal("provider assembly failed")
	}
	for _, model := range []string{"gpt-5.6-sol", "gpt-5.6-spark"} {
		if !reflect.DeepEqual(AccountMeetsModelPlan(codexJSONProviderID, model, credential), AccountMeetsModelPlan("openai-codex", model, credential)) || !reflect.DeepEqual(AccountRequestBlockScopes(codexJSONProviderID, model), AccountRequestBlockScopes("openai-codex", model)) {
			t.Fatal("Codex account policy drift")
		}
	}
	usage := codexJSONUsageProvider()
	report := usage.ParseRateLimitHeaders(map[string]string{"x-codex-primary-used-percent": "100"}, time.Now().UnixMilli(), new(429))
	if report == nil || report.Provider != codexJSONProviderID || len(report.Limits) == 0 || report.Limits[0].Scope.Provider != codexJSONProviderID {
		t.Fatalf("incorrect usage ownership: %+v", report)
	}
}

// codexJSONTestTransport redirects only the fixed token endpoint to a local server.
type codexJSONTestTransport struct {
	target *url.URL
	base   http.RoundTripper
}

// RoundTrip refuses unexpected hosts so lifecycle tests cannot contact live services.
func (tr codexJSONTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.String() != codexTokenURL {
		return nil, errors.New("unexpected live endpoint")
	}
	clone := req.Clone(req.Context())
	u := *req.URL
	u.Scheme = tr.target.Scheme
	u.Host = tr.target.Host
	clone.URL = &u
	return tr.base.RoundTrip(clone)
}

// withCodexJSONTokenServer installs an isolated HTTP token fixture for a serial test.
func withCodexJSONTokenServer(t *testing.T, status int, body string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil || req.Form.Get("grant_type") != "refresh_token" {
			t.Error("incorrect token request")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	target, _ := url.Parse(server.URL)
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: codexJSONTestTransport{target: target, base: server.Client().Transport}}
	t.Cleanup(func() { oauthRefreshWork.Wait(); http.DefaultClient = previous; server.Close() })
}

// TestCodexJSONRefreshLifecycle distinguishes permanent rejection from transient
// errors, verifies secret scrubbing, and proves ordinary Codex accounts are untouched.
func TestCodexJSONRefreshLifecycle(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		retire bool
	}{
		{"invalid grant", 400, `{"error":"invalid_grant"}`, true},
		{"revoked refresh", 401, `{"error":{"code":"refresh_token_invalidated"}}`, true},
		{"reused refresh", 400, `{"error":"refresh_token_reused"}`, true},
		{"unclassified unauthorized", 401, `{"error":"unknown"}`, false},
		{"server failure", 503, `{"error":"invalid_grant"}`, false},
		{"rate limited", 429, `{"error":"invalid_grant"}`, false},
		{"unknown client error", 400, `{"error":"invalid_request"}`, false},
		{"invalid response", 200, `not json`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withCodexJSONTokenServer(t, tc.status, tc.body)
			store, _ := openTestStore(t)
			ctx := t.Context()
			expired := oauthCredential("dead", "", time.Now().Add(-time.Hour))
			key := deriveAccountKey(expired)
			if _, err := store.SaveAccount(ctx, codexJSONProviderID, key, expired); err != nil {
				t.Fatal(err)
			}
			if err := store.BlockAccount(ctx, codexJSONProviderID, key, "chat", time.Now().Add(time.Hour).UnixMilli()); err != nil {
				t.Fatal(err)
			}
			auth, _ := BuiltinProviderAuth(codexJSONProviderID)
			_, err := ResolveProviderAuth(ctx, codexJSONProviderID, auth, store, AuthContext{}, WithAccountKey(AuthResolutionOverrides{}, key))
			if err == nil {
				t.Fatal("failed refresh was accepted")
			}
			saved, err := store.ReadAccount(ctx, codexJSONProviderID, key)
			if err != nil {
				t.Fatal(err)
			}
			if tc.retire != (saved == nil) {
				t.Fatalf("retire=%v, credential=%+v", tc.retire, saved)
			}
			var data string
			var disabled *string
			if err := store.db.QueryRow("SELECT data, disabled_cause FROM auth_credentials WHERE provider = ?", codexJSONProviderID).Scan(&data, &disabled); err != nil {
				t.Fatal(err)
			}
			if tc.retire && (data != "{}" || disabled == nil) {
				t.Fatalf("secrets not scrubbed: %s", data)
			}
			if !tc.retire && (saved.Refresh != expired.Refresh || disabled != nil) {
				t.Fatal("transient failure changed credential")
			}
			if tc.retire {
				var count int
				if err := store.db.QueryRow("SELECT COUNT(*) FROM auth_credential_blocks").Scan(&count); err != nil || count != 0 {
					t.Fatalf("stale account blocks: %d %v", count, err)
				}
			}
			// The new lifecycle must never opt regular browser Codex accounts into cleanup.
			if _, err := store.SaveAccount(ctx, "openai-codex", key, expired); err != nil {
				t.Fatal(err)
			}
			ordinary, _ := BuiltinProviderAuth("openai-codex")
			_, _ = ResolveProviderAuth(ctx, "openai-codex", ordinary, store, AuthContext{}, WithAccountKey(AuthResolutionOverrides{}, key))
			saved, err = store.ReadAccount(ctx, "openai-codex", key)
			if err != nil || saved == nil {
				t.Fatal("ordinary Codex credential removed")
			}
		})
	}
}

// TestCodexJSONRefreshRotation verifies successful refresh persistence and stable identity.
func TestCodexJSONRefreshRotation(t *testing.T) {
	token := codexJSONTestJWT("acct", time.Now().Add(time.Hour))
	withCodexJSONTokenServer(t, 200, fmt.Sprintf(`{"access_token":%q,"refresh_token":"rotated-fixture","expires_in":3600}`, token))
	store, _ := openTestStore(t)
	old := oauthCredential("acct", "", time.Now().Add(-time.Hour))
	key := deriveAccountKey(old)
	if _, err := store.SaveAccount(t.Context(), codexJSONProviderID, key, old); err != nil {
		t.Fatal(err)
	}
	auth, _ := BuiltinProviderAuth(codexJSONProviderID)
	result, err := ResolveProviderAuth(t.Context(), codexJSONProviderID, auth, store, AuthContext{}, WithAccountKey(AuthResolutionOverrides{}, key))
	if err != nil || result == nil || result.Auth.APIKey != token {
		t.Fatalf("rotation failed: %+v %v", result, err)
	}
	saved, err := store.ReadAccount(t.Context(), codexJSONProviderID, key)
	if err != nil || saved.Refresh != "rotated-fixture" || saved.AccountID != "acct" {
		t.Fatalf("rotation not persisted: %+v %v", saved, err)
	}
}

// TestCodexJSONAccessOnlyExpiry preserves the final valid minutes of an
// access-only import, then scrubs it when forced refresh establishes rejection.
func TestCodexJSONAccessOnlyExpiry(t *testing.T) {
	store, _ := openTestStore(t)
	c := oauthCredential("acct", "", time.Now().Add(time.Minute))
	c.Refresh = ""
	key := deriveAccountKey(c)
	if _, err := store.SaveAccount(t.Context(), codexJSONProviderID, key, c); err != nil {
		t.Fatal(err)
	}
	auth, _ := BuiltinProviderAuth(codexJSONProviderID)
	overrides := WithAccountKey(AuthResolutionOverrides{}, key)
	if result, err := ResolveProviderAuth(t.Context(), codexJSONProviderID, auth, store, AuthContext{}, overrides); err != nil || result == nil {
		t.Fatalf("live access-only token was retired early: %v", err)
	}
	overrides.MinOAuthValidityMs = new(float64(10 * 60 * 1000))
	if _, err := ResolveProviderAuth(t.Context(), codexJSONProviderID, auth, store, AuthContext{}, overrides); err == nil {
		t.Fatal("access-only export ignored minimum validity")
	}
	if saved, err := store.ReadAccount(t.Context(), codexJSONProviderID, key); err != nil || saved == nil {
		t.Fatal("minimum-validity rejection retired a still-live credential")
	}
	overrides.MinOAuthValidityMs = nil
	overrides.ForceOAuthRefresh = true
	if _, err := ResolveProviderAuth(t.Context(), codexJSONProviderID, auth, store, AuthContext{}, overrides); !errors.Is(err, errCodexJSONExpired) {
		t.Fatalf("expected permanent rejection, got %v", err)
	}
	if saved, err := store.ReadAccount(t.Context(), codexJSONProviderID, key); err != nil || saved != nil {
		t.Fatal("rejected access-only account retained")
	}
}
