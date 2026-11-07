package ai

// Protocol regressions for the OAuth browser flows this harness adds: the parsing and
// normalization rules each flow depends on, and the pieces whose behaviour is not obvious
// from the request shape (Z.ai's envelope handling and key-mint chain, Snowflake account
// normalization, Cursor's JWT subject/profile matching, Perplexity's JWT expiry rule).

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func ompTestJWT(t *testing.T, payload map[string]any) string {
	t.Helper()
	encode := func(value any) string {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("encode JWT segment: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return encode(map[string]any{"alg": "none"}) + "." + encode(payload) + ".sig"
}

func TestParseZaiAuthorizationInput(t *testing.T) {
	cases := map[string]string{
		"zcode://zai-auth/callback?code=abc&state=x": "abc",
		"https://host/cb?authCode=zzz":               "zzz",
		"abc":                                        "abc",
		// A shell may leave the rest of the query attached to a bare paste.
		"abc&state=x": "abc",
		"abc#state":   "abc",
		"   ":         "",
		// A URL that carries no code yields nothing rather than the URL text.
		"https://host/cb?state=x": "",
	}
	for input, want := range cases {
		if got := ParseZaiAuthorizationInput(input); got != want {
			t.Errorf("ParseZaiAuthorizationInput(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestZaiKeyMintChain(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/auth/z/login":
			_, _ = w.Write([]byte(`{"code":200,"data":{"access_token":"biz"}}`))
		case r.URL.Path == "/api/biz/customer/getCustomerInfo":
			_, _ = w.Write([]byte(`{"code":200,"data":{"organizations":[` +
				`{"organizationId":"other","projects":[{"projectId":"p0"}]},` +
				`{"organizationId":"o1","isDefault":true,"projects":[{"projectId":"p1","isDefault":true}]}]}}`))
		case strings.HasSuffix(r.URL.Path, "/api_keys"):
			// An existing build key is reused, and its list entry has the secret masked.
			_, _ = w.Write([]byte(`{"code":200,"data":[{"name":"oh-my-pi","apiKey":"KEY","secretKey":"****"}]}`))
		case strings.Contains(r.URL.Path, "/api_keys/copy/"):
			_, _ = w.Write([]byte(`{"code":200,"data":{"secretKey":"SECRET"}}`))
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	key, err := zaiOAuthMintAPIKey(t.Context(), "oauth-access", server.URL, server.URL+"/api/auth/z/login")
	if err != nil {
		t.Fatalf("zaiOAuthMintAPIKey: %v", err)
	}
	if key != "KEY.SECRET" {
		t.Fatalf("minted key = %q, want KEY.SECRET", key)
	}
	// The default organization and project are chosen, and the secret comes from copy.
	want := []string{
		"/api/auth/z/login",
		"/api/biz/customer/getCustomerInfo",
		"/api/biz/v1/organization/o1/projects/p1/api_keys",
		"/api/biz/v1/organization/o1/projects/p1/api_keys/copy/KEY",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for index := range want {
		if calls[index] != want[index] {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
}

func TestZaiKeyMintRejectsFailedEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":401,"msg":"token is invalid"}`))
	}))
	defer server.Close()

	_, err := zaiOAuthMintAPIKey(t.Context(), "oauth-access", server.URL, server.URL+"/api/auth/z/login")
	if err == nil || !strings.Contains(err.Error(), "token is invalid") {
		t.Fatalf("error = %v, want the envelope message", err)
	}
}

func TestNormalizeSnowflakeAccountURL(t *testing.T) {
	valid := map[string]string{
		"myorg-myaccount":                                         "https://myorg-myaccount.snowflakecomputing.com",
		"MYORG_MYACCOUNT":                                         "https://myorg-myaccount.snowflakecomputing.com",
		"myorg-myaccount.snowflakecomputing.com":                  "https://myorg-myaccount.snowflakecomputing.com",
		"https://myorg-myaccount.snowflakecomputing.com:443":      "https://myorg-myaccount.snowflakecomputing.com",
		"https://myorg-myaccount.snowflakecomputing.com/path?q=1": "https://myorg-myaccount.snowflakecomputing.com",
		"https://app.snowflake.com/myorg/myaccount/":              "https://myorg-myaccount.snowflakecomputing.com",
	}
	for input, want := range valid {
		got, err := NormalizeSnowflakeAccountURL(input)
		if err != nil {
			t.Errorf("NormalizeSnowflakeAccountURL(%q): %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeSnowflakeAccountURL(%q) = %q, want %q", input, got, want)
		}
	}

	invalid := []string{
		"",
		"http://myorg-myaccount.snowflakecomputing.com",
		"https://app.snowflake.com/us-east-1/abc",
		"myorg-myaccount.snowflakecomputing.cn",
		"https://evil.example.com",
		"myorg/myaccount",
		"-bad-.snowflakecomputing.com",
	}
	for _, input := range invalid {
		if got, err := NormalizeSnowflakeAccountURL(input); err == nil {
			t.Errorf("NormalizeSnowflakeAccountURL(%q) = %q, want an error", input, got)
		}
	}
}

func TestParseSnowflakeTokenResponse(t *testing.T) {
	// A missing refresh token is valid; a missing token or lifetime is not.
	tokens, err := parseSnowflakeTokenResponse([]byte(`{"access_token":"a","expires_in":600}`))
	if err != nil {
		t.Fatalf("parse response without refresh token: %v", err)
	}
	if tokens.Access != "a" || tokens.Refresh != "" || tokens.Expires <= nowMillis() {
		t.Fatalf("tokens = %+v", tokens)
	}
	for _, body := range []string{
		`{}`,
		`{"access_token":"a"}`,
		`{"access_token":"","expires_in":600}`,
		`{"access_token":"a","expires_in":0}`,
		`{"access_token":"a","expires_in":600,"refresh_token":""}`,
		`not json`,
	} {
		if _, err := parseSnowflakeTokenResponse([]byte(body)); err == nil {
			t.Errorf("parseSnowflakeTokenResponse(%s) accepted an invalid body", body)
		}
	}
}

func TestCursorAccessTokenSubject(t *testing.T) {
	token := ompTestJWT(t, map[string]any{"sub": "github|1245379", "exp": 4_000_000_000})
	if got := cursorAccessTokenSubject(token); got != "github|1245379" {
		t.Fatalf("subject = %q", got)
	}
	if got := ExtractCursorAccessTokenUserID(token); got != "1245379" {
		t.Fatalf("user id = %q", got)
	}
	// A token with no `|` names the account with its whole sub claim.
	plain := ompTestJWT(t, map[string]any{"sub": "auth0|user_9"})
	if got := ExtractCursorAccessTokenUserID(plain); got != "user_9" {
		t.Fatalf("user id = %q", got)
	}
	if got := ExtractCursorAccessTokenUserID("not-a-jwt"); got != "" {
		t.Fatalf("user id of a malformed token = %q", got)
	}
	// The expiry carries the five minute margin.
	expiring := ompTestJWT(t, map[string]any{"exp": 1_000_000})
	if got, want := cursorTokenExpiry(expiring), int64(1_000_000)*1000-5*60*1000; got != want {
		t.Fatalf("expiry = %d, want %d", got, want)
	}
	if !IsCursorTokenExpiringSoon(expiring, 300) {
		t.Fatal("an expired token must report expiring soon")
	}
}

func TestFetchCursorAccountEmailRequiresMatchingSubject(t *testing.T) {
	token := ompTestJWT(t, map[string]any{"sub": "github|42"})
	subject := "github|42"
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Header.Get("Connect-Protocol-Version") != "1" {
			t.Errorf("Connect-Protocol-Version = %q", r.Header.Get("Connect-Protocol-Version"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authId":"` + subject + `","email":"a@b.c"}`))
	}))
	defer server.Close()

	previous := cursorProfileEndpoint
	cursorProfileEndpoint = server.URL
	defer func() { cursorProfileEndpoint = previous }()

	email, err := FetchCursorAccountEmail(t.Context(), token)
	if err != nil {
		t.Fatalf("FetchCursorAccountEmail: %v", err)
	}
	if email != "a@b.c" {
		t.Fatalf("email = %q", email)
	}
	if gotAuth != "Bearer "+token {
		t.Fatalf("Authorization = %q", gotAuth)
	}
}

func TestPerplexityJWTExpiry(t *testing.T) {
	// A session JWT without `exp` never expires client-side.
	if got := ompOAuthJWTExpiryMs("opaque-session-token"); got != ompOAuthNeverExpires {
		t.Fatalf("expiry of an opaque token = %d", got)
	}
	token := ompTestJWT(t, map[string]any{"sub": "user"})
	if got := ompOAuthJWTExpiryMs(token); got != ompOAuthNeverExpires {
		t.Fatalf("expiry without exp claim = %d", got)
	}
	withExp := ompTestJWT(t, map[string]any{"exp": 1_700_000_000})
	if got, want := ompOAuthJWTExpiryMs(withExp), int64(1_700_000_000)*1000-5*60*1000; got != want {
		t.Fatalf("expiry = %d, want %d", got, want)
	}
}

func TestSnowflakeOAuthProviderStructuredKey(t *testing.T) {
	credentials := OAuthCredentials{Access: "bearer"}
	credentials.Extra = map[string]json.RawMessage{"enterpriseUrl": mustJSON("https://org-acct.snowflakecomputing.com")}
	key := snowflakeOAuthProvider{}.GetAPIKey(credentials)
	result, err := snowflakeModelAuth(key)
	if err != nil {
		t.Fatalf("snowflakeModelAuth: %v", err)
	}
	if result.Auth.APIKey != "bearer" || result.Auth.BaseURL != "https://org-acct.snowflakecomputing.com" {
		t.Fatalf("auth = %+v", result.Auth)
	}
	// A structured key without its account must not degrade into a bearer token.
	if _, err := snowflakeModelAuth(`{"token":"bearer"}`); err == nil {
		t.Fatal("a structured key without enterpriseUrl was accepted")
	}
}
