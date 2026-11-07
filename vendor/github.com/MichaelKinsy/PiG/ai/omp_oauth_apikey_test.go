package ai

// Protocol tests for the omp-slim API-key sign-in flows: the wire shapes their logins send and accept,
// the credential forms they store, and the probe fallback that decides whether a key is accepted.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ompTestCallbacks records the events and answers a fixed prompt sequence.
type ompTestCallbacks struct {
	prompts  []string
	index    int
	answers  []string
	auth     []OAuthAuthInfo
	progress []string
}

func (c *ompTestCallbacks) callbacks() OAuthLoginCallbacks {
	return OAuthLoginCallbacks{
		OnAuth:     func(info OAuthAuthInfo) { c.auth = append(c.auth, info) },
		OnProgress: func(message string) { c.progress = append(c.progress, message) },
		OnPromptContext: func(_ context.Context, prompt OAuthPrompt) (string, error) {
			c.prompts = append(c.prompts, prompt.Message)
			if c.index >= len(c.answers) {
				return "", fmt.Errorf("unexpected prompt %q", prompt.Message)
			}
			answer := c.answers[c.index]
			c.index++
			return answer, nil
		},
	}
}

func TestOMPKiloDeviceAuthorizationPollsUntilApproved(t *testing.T) {
	var polls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/codes":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":"AZ12","verificationUrl":"https://kilo.example/device","expiresIn":600}`))
		case r.Method == http.MethodGet && r.URL.Path == "/codes/AZ12":
			polls++
			if polls < 3 {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"approved","token":"kilo-token"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := kiloOAuthProvider{
		client:    server.Client(),
		baseURL:   server.URL,
		pollDelay: func(context.Context, time.Duration) error { return nil },
	}
	callbacks := &ompTestCallbacks{}
	credentials, err := provider.LoginContext(context.Background(), callbacks.callbacks())
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if credentials.Access != "kilo-token" || credentials.Refresh != "" {
		t.Fatalf("credential = %+v, want the approved token as access only", credentials)
	}
	if remaining := credentials.ExpiresMillis() - float64(nowMillis()); remaining < float64(kiloKeyLifetimeMs)-5000 {
		t.Fatalf("expiry is %v ms away, want about a year", remaining)
	}
	if polls != 3 {
		t.Fatalf("polls = %d, want 3", polls)
	}
	if len(callbacks.auth) != 1 || callbacks.auth[0].Instructions != "Enter code: AZ12" {
		t.Fatalf("auth events = %+v, want one with the user code", callbacks.auth)
	}
}

func TestOMPKiloDeniedStopsPolling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"code":"AZ12","verificationUrl":"https://kilo.example/device","expiresIn":600}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	provider := kiloOAuthProvider{client: server.Client(), baseURL: server.URL,
		pollDelay: func(context.Context, time.Duration) error { return nil }}
	_, err := provider.LoginContext(context.Background(), (&ompTestCallbacks{}).callbacks())
	if err == nil || err.Error() != "Authorization was denied" {
		t.Fatalf("err = %v, want the denial", err)
	}
}

func TestOMPKiloMissingFieldsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"AZ12"}`))
	}))
	defer server.Close()

	provider := kiloOAuthProvider{client: server.Client(), baseURL: server.URL}
	_, err := provider.LoginContext(context.Background(), (&ompTestCallbacks{}).callbacks())
	if err == nil || !strings.Contains(err.Error(), "missing required fields") {
		t.Fatalf("err = %v, want the missing-field failure", err)
	}
}

func TestOMPKiloCancellationReportsCancelMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"AZ12","verificationUrl":"https://kilo.example/device","expiresIn":600}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	provider := kiloOAuthProvider{client: server.Client(), baseURL: server.URL,
		pollDelay: func(context.Context, time.Duration) error {
			cancel()
			return nil
		}}
	_, err := provider.LoginContext(ctx, (&ompTestCallbacks{}).callbacks())
	if err == nil || err.Error() != ompCancelMessage {
		t.Fatalf("err = %v, want %q", err, ompCancelMessage)
	}
}

func TestOMPXiaomiTokenPlanRegionalFallback(t *testing.T) {
	original := xiaomiTokenPlanEndpoints
	defer func() { xiaomiTokenPlanEndpoints = original }()

	var hosts []string
	// Two clusters: the first rejects the key with 401, the second accepts it.
	sgp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("probe path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tp-regional" {
			t.Errorf("authorization = %q", got)
		}
		hosts = append(hosts, "sgp")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer sgp.Close()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts = append(hosts, "ams")
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()

	xiaomiTokenPlanEndpoints = []struct {
		baseURL string
		model   string
	}{
		{sgp.URL + "/v1", xiaomiValidationModel},
		{ok.URL + "/v1", xiaomiValidationModel},
	}

	provider := xiaomiOAuthProvider{client: sgp.Client()}
	callbacks := &ompTestCallbacks{answers: []string{" tp-regional "}}
	credentials, err := provider.LoginContext(context.Background(), callbacks.callbacks())
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if credentials.Access != "tp-regional" {
		t.Fatalf("access = %q, want the trimmed key", credentials.Access)
	}
	if len(hosts) != 2 || hosts[0] != "sgp" || hosts[1] != "ams" {
		t.Fatalf("probe order = %v, want sgp then ams", hosts)
	}
}

func TestOMPXiaomiNon401StopsRegionalFallback(t *testing.T) {
	original := xiaomiTokenPlanEndpoints
	defer func() { xiaomiTokenPlanEndpoints = original }()

	var probes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probes++
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":"quota"}`))
	}))
	defer server.Close()
	xiaomiTokenPlanEndpoints = []struct {
		baseURL string
		model   string
	}{{server.URL + "/v1", xiaomiValidationModel}, {server.URL + "/v1", xiaomiValidationModel}}

	provider := xiaomiOAuthProvider{client: server.Client()}
	_, err := provider.LoginContext(context.Background(), (&ompTestCallbacks{answers: []string{"tp-x"}}).callbacks())
	if err == nil || !strings.Contains(err.Error(), "402") {
		t.Fatalf("err = %v, want the 402 validation failure", err)
	}
	if probes != 1 {
		t.Fatalf("probes = %d, want the fallback to stop after a non-401", probes)
	}
}

func TestOMPXiaomiEmptyKeyRejected(t *testing.T) {
	provider := xiaomiOAuthProvider{client: &http.Client{}}
	_, err := provider.LoginContext(context.Background(), (&ompTestCallbacks{answers: []string{"   "}}).callbacks())
	if err == nil || err.Error() != "API key is required" {
		t.Fatalf("err = %v, want the required-key failure", err)
	}
}

func TestOMPCloudflareCredentialRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		input cloudflareCredential
		want  string
	}{
		{"gateway", cloudflareCredential{Token: " cfut_x ", AccountID: " acc ", GatewayID: " gw "},
			`{"token":"cfut_x","accountId":"acc","gatewayId":"gw"}`},
		{"workers", cloudflareCredential{Token: "cfut_x", AccountID: "acc"},
			`{"token":"cfut_x","accountId":"acc"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			serialized := serializeCloudflareCredential(tc.input)
			if serialized != tc.want {
				t.Fatalf("serialized = %s, want %s", serialized, tc.want)
			}
			parsed, ok := parseCloudflareCredential(serialized)
			if !ok {
				t.Fatalf("parse(%s) failed", serialized)
			}
			if parsed.Token != "cfut_x" || parsed.AccountID != "acc" {
				t.Fatalf("parsed = %+v", parsed)
			}
		})
	}
}

func TestOMPCloudflareCredentialParsing(t *testing.T) {
	legacy, ok := parseCloudflareCredential("  plain-token  ")
	if !ok || legacy.Token != "plain-token" || legacy.AccountID != "" {
		t.Fatalf("legacy = %+v ok=%v", legacy, ok)
	}
	for _, invalid := range []string{"", "   ", `{"accountId":"acc"}`, `{"token":"  "}`, `{"token":1}`, "{not json"} {
		if parsed, ok := parseCloudflareCredential(invalid); ok {
			t.Fatalf("parse(%q) = %+v, want rejection", invalid, parsed)
		}
	}
}

func TestOMPCloudflareWorkersLoginStoresComposite(t *testing.T) {
	callbacks := &ompTestCallbacks{answers: []string{"cfut_token", "acc123"}}
	credentials, err := newCloudflareWorkersAIOAuthProvider().LoginContext(context.Background(), callbacks.callbacks())
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if len(callbacks.auth) != 1 || callbacks.auth[0].URL != cloudflareTokensAuthURL {
		t.Fatalf("auth events = %+v", callbacks.auth)
	}
	env := ompCompositeKeyEnv(credentials.Access)
	if env[cloudflareAccountID] != "acc123" || env[cloudflareGatewayID] != "" {
		t.Fatalf("env = %+v", env)
	}
	auth, handled, err := ompOAuthToAuth("cloudflare-workers-ai", Credential{Type: CredentialOAuth, Access: credentials.Access})
	if err != nil {
		t.Fatalf("request auth: %v", err)
	}
	if !handled || auth.APIKey != "cfut_token" || auth.Headers != nil {
		t.Fatalf("request auth = %+v handled=%v", auth, handled)
	}
}

func TestOMPCloudflareGatewayRequestUsesGatewayHeader(t *testing.T) {
	key := serializeCloudflareCredential(cloudflareCredential{Token: "cfut_gw", AccountID: "acc", GatewayID: "gw"})
	auth, handled, err := ompOAuthToAuth("cloudflare-ai-gateway", Credential{Type: CredentialOAuth, Access: key})
	if err != nil {
		t.Fatalf("request auth: %v", err)
	}
	if !handled {
		t.Fatal("gateway credential was not handled")
	}
	if auth.APIKey != "" {
		t.Fatalf("api key = %q, want none", auth.APIKey)
	}
	if value := auth.Headers["cf-aig-authorization"]; value == nil || *value != "Bearer cfut_gw" {
		t.Fatalf("cf-aig-authorization = %v", value)
	}
	if auth.Headers["Authorization"] != nil || auth.Headers["x-api-key"] != nil {
		t.Fatalf("caller auth headers must be cleared: %+v", auth.Headers)
	}
}

func TestOMPCloudflareGatewayLoginRequiresBothIDs(t *testing.T) {
	provider := newCloudflareAIGatewayOAuthProvider()
	_, err := provider.LoginContext(context.Background(), (&ompTestCallbacks{answers: []string{"cfut", "acc", "  "}}).callbacks())
	if err == nil || err.Error() != "Cloudflare AI Gateway ID is required" {
		t.Fatalf("err = %v, want the gateway-id failure", err)
	}
}

func TestOMPAlibabaCodingPlanCustomURLStoresEnterpriseURL(t *testing.T) {
	var probed string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probed = r.URL.Path
		if got := r.Header.Get("Authorization"); got != "Bearer sk-plan" {
			t.Errorf("authorization = %q", got)
		}
		if r.Method != http.MethodGet {
			t.Errorf("custom endpoint validation method = %q, want GET", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := alibabaCodingPlanProvider{client: server.Client()}
	// The China endpoint is a constant, so the probe is observed by pointing the custom URL at the
	// test server and checking the credential serialization separately.
	callbacks := &ompTestCallbacks{answers: []string{"3", " " + server.URL + "/v1/ ", "sk-plan"}}
	credentials, err := provider.LoginContext(context.Background(), callbacks.callbacks())
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if probed != "/v1/models" {
		t.Fatalf("probe path = %q, want the custom endpoint's model list", probed)
	}
	parsed := ompTestCredentialFromOAuth(t, credentials)
	if parsed.EnterpriseDomain != server.URL+"/v1" {
		t.Fatalf("enterpriseUrl = %q, want the trimmed custom URL", parsed.EnterpriseDomain)
	}
	if parsed.Access != "sk-plan" || parsed.Refresh != "sk-plan" {
		t.Fatalf("credential = access %q refresh %q", parsed.Access, parsed.Refresh)
	}
	auth, handled, err := ompOAuthToAuth("alibaba-coding-plan", parsed)
	if err != nil {
		t.Fatalf("request auth: %v", err)
	}
	if !handled || auth.BaseURL != server.URL+"/v1" || auth.APIKey != "sk-plan" {
		t.Fatalf("request auth = %+v", auth)
	}
}

func TestOMPAlibabaCodingPlanCustomURLRequired(t *testing.T) {
	provider := alibabaCodingPlanProvider{client: &http.Client{}}
	_, err := provider.LoginContext(context.Background(), (&ompTestCallbacks{answers: []string{"3", "   "}}).callbacks())
	if err == nil || !strings.Contains(err.Error(), "Custom URL is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestOMPAlibabaTokenPlanCookieValidation(t *testing.T) {
	cases := []struct {
		cookie string
		valid  bool
	}{
		{"session=abc", true},
		{"a=1; b=2", true},
		{"  ;  ", false},
		{"noequals", false},
		{"=value", false},
		{"name=   ", false},
	}
	for _, tc := range cases {
		if got := validCookieHeader(tc.cookie); got != tc.valid {
			t.Fatalf("validCookieHeader(%q) = %v, want %v", tc.cookie, got, tc.valid)
		}
	}
}

func TestOMPAlibabaTokenPlanSerialization(t *testing.T) {
	if got := serializeAlibabaTokenPlanCredential("sk-sp-x", "", ""); got != "sk-sp-x" {
		t.Fatalf("bare = %q", got)
	}
	stored := serializeAlibabaTokenPlanCredential("sk-sp-x", "session=abc", "")
	parsed, ok := parseAlibabaTokenPlanCredential(stored)
	if !ok || parsed.Token != "sk-sp-x" || parsed.Cookie != "session=abc" || parsed.BaseURL != "" {
		t.Fatalf("parsed = %+v ok=%v", parsed, ok)
	}
	regional := serializeAlibabaTokenPlanCredential("sk-sp-x", "", alibabaTokenPlanCNBaseURL)
	parsed, ok = parseAlibabaTokenPlanCredential(regional)
	if !ok || parsed.BaseURL != alibabaTokenPlanCNBaseURL {
		t.Fatalf("regional = %+v ok=%v", parsed, ok)
	}
	auth, handled, err := ompOAuthToAuth("alibaba-token-plan", Credential{Type: CredentialOAuth, Access: regional})
	if err != nil {
		t.Fatalf("request auth: %v", err)
	}
	if !handled || auth.APIKey != "sk-sp-x" || auth.BaseURL != alibabaTokenPlanCNBaseURL {
		t.Fatalf("request auth = %+v", auth)
	}
}

func TestOMPMistralPKCEChallengeIsS256OfVerifier(t *testing.T) {
	verifier, challenge, err := mistralPKCE()
	if err != nil {
		t.Fatalf("pkce: %v", err)
	}
	if len(verifier) != 128 {
		t.Fatalf("verifier length = %d, want 96 bytes base64url (128 chars)", len(verifier))
	}
	if challenge != base64URLEncode(sha256SumOf(verifier)) {
		t.Fatal("challenge is not the S256 of the verifier")
	}
	if second, secondChallenge, _ := mistralPKCE(); second == verifier || secondChallenge == challenge {
		t.Fatal("two sign-ins share a verifier")
	}
}

func TestOMPMistralSignInMintsKey(t *testing.T) {
	var exchangeBody map[string]string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The process id is percent-encoded on the wire, so compare the raw path.
		switch r.URL.EscapedPath() {
		case "/api/vibe/sign-in":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["code_challenge_method"] != "S256" || body["code_challenge"] == "" {
				t.Errorf("sign-in body = %+v", body)
			}
			expiry := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
			_, _ = fmt.Fprintf(w, `{"process_id":"proc 1","sign_in_url":%q,"poll_url":%q,"expires_at":%q}`,
				server.URL+"/console", server.URL+"/api/vibe/sign-in/proc%201/poll", expiry)
		case "/api/vibe/sign-in/proc%201/poll":
			_, _ = w.Write([]byte(`{"status":"completed","exchange_token":"ex-1"}`))
		case "/api/vibe/sign-in/proc%201/exchange":
			_ = json.NewDecoder(r.Body).Decode(&exchangeBody)
			_, _ = w.Write([]byte(`{"api_key":"mistral-key"}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := mistralVibeOAuthProvider{client: server.Client(), origin: server.URL}
	callbacks := &ompTestCallbacks{}
	credentials, err := provider.LoginContext(context.Background(), callbacks.callbacks())
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if credentials.Access != "mistral-key" {
		t.Fatalf("access = %q", credentials.Access)
	}
	if exchangeBody["exchange_token"] != "ex-1" || exchangeBody["code_verifier"] == "" {
		t.Fatalf("exchange body = %+v", exchangeBody)
	}
	if len(callbacks.auth) != 1 || callbacks.auth[0].URL != server.URL+"/console" {
		t.Fatalf("auth events = %+v", callbacks.auth)
	}
	if len(callbacks.progress) != 2 {
		t.Fatalf("progress = %v", callbacks.progress)
	}
}

func TestOMPMistralExpiredPollFails(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The process id is percent-encoded on the wire, so compare the raw path.
		switch r.URL.EscapedPath() {
		case "/api/vibe/sign-in":
			expiry := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
			_, _ = fmt.Fprintf(w, `{"process_id":"p","sign_in_url":%q,"poll_url":%q,"expires_at":%q}`,
				server.URL+"/console", server.URL+"/api/poll", expiry)
		default:
			w.WriteHeader(http.StatusGone)
		}
	}))
	defer server.Close()

	provider := mistralVibeOAuthProvider{client: server.Client(), origin: server.URL}
	_, err := provider.LoginContext(context.Background(), (&ompTestCallbacks{}).callbacks())
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("err = %v, want the expiry failure", err)
	}
}

func TestOMPMistralRejectsForeignPollURL(t *testing.T) {
	provider := mistralVibeOAuthProvider{client: &http.Client{}, origin: "https://console.mistral.ai"}
	if _, err := consoleURL("https://evil.example/api/vibe/sign-in/1/poll", provider.origin, mistralAPIPath, "start"); err == nil {
		t.Fatal("foreign origin accepted")
	}
	if _, err := consoleURL("https://console.mistral.ai/api-x", provider.origin, mistralAPIPath, "start"); err == nil {
		t.Fatal("segment-adjacent path accepted")
	}
	trusted, err := consoleURL("https://console.mistral.ai/api/vibe/sign-in/1/poll", provider.origin, mistralAPIPath, "start")
	if err != nil || trusted == "" {
		t.Fatalf("trusted URL rejected: %v", err)
	}
}

func TestOMPMistralWorkspaceDeniedMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == "/api/vibe/sign-in/p/exchange" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error_code":9004}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := mistralVibeOAuthProvider{client: server.Client(), origin: server.URL}
	_, err := provider.exchangeForAPIKey(context.Background(), "p", "ex", "verifier")
	if err == nil || !strings.Contains(err.Error(), "workspace can't be used") {
		t.Fatalf("err = %v, want the workspace denial", err)
	}
}

func TestOMPRetryAfterHeaderParsing(t *testing.T) {
	headers := http.Header{}
	headers.Set("retry-after", "2")
	if got := retryAfterFromHeaders(headers); got != 2*time.Second {
		t.Fatalf("retry-after = %v, want 2s", got)
	}
	headers.Set("retry-after-ms", "2500")
	if got := retryAfterFromHeaders(headers); got != 2500*time.Millisecond {
		t.Fatalf("largest delay = %v, want 2.5s", got)
	}
	capped := retryAfterSeconds(func() http.Header {
		h := http.Header{}
		h.Set("retry-after", "600")
		return h
	}())
	if capped == nil || *capped != mistralMaxRetryAfter.Seconds() {
		t.Fatalf("capped = %v, want %v", capped, mistralMaxRetryAfter.Seconds())
	}
	if retryAfterSeconds(http.Header{}) != nil {
		t.Fatal("an absent Retry-After must leave the default cadence")
	}
}

func TestOMPEncodeURIComponentMatchesJavaScript(t *testing.T) {
	cases := map[string]string{
		"AZ 12":     "AZ%2012",
		"a/b?c=d":   "a%2Fb%3Fc%3Dd",
		"~-_.!*'()": "~-_.!*'()",
		"é":         "%C3%A9",
	}
	for input, want := range cases {
		if got := ompEncodeURIComponent(input); got != want {
			t.Fatalf("encodeURIComponent(%q) = %q, want %q", input, got, want)
		}
	}
}

// ompTestCredentialFromOAuth converts a flow result to the stored credential shape.
func ompTestCredentialFromOAuth(t *testing.T, credentials OAuthCredentials) Credential {
	t.Helper()
	credential, err := credentialFromOAuth(credentials)
	if err != nil {
		t.Fatalf("credential: %v", err)
	}
	return credential
}

// sha256SumOf is the raw SHA-256 digest of a string, for the PKCE challenge check.
func sha256SumOf(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}
