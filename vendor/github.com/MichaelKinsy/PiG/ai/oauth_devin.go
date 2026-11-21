package ai

// Devin OAuth: browser PKCE sign-in against app.devin.ai plus adoption of the
// official Devin CLI's stored credentials. Ports
// packages/ai/src/registry/oauth/devin.ts and devin-cli.ts.
//
// Ports packages/ai/src/registry/oauth/devin.ts, packages/ai/src/registry/oauth/devin-cli.ts.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	devinLoginBrowserMethod = "browser"
	devinLoginCLIMethod     = "cli"

	devinAuthorizeURL = "https://app.devin.ai/auth/cli/continue"
	devinTokenURL     = "https://api.devin.ai/auth/cli/token"
	devinCallbackPort = 59653
	devinCallbackPath = "/callback"

	devinTokenExpirySkewMillis = 5 * 60 * 1000
	devinFallbackExpiryMillis  = 365 * 24 * 60 * 60 * 1000
	devinIdentityTimeout       = 30 * time.Second
)

// devinAcceptedAPIHosts are the api_server_url hosts the Devin CLI may name.
var devinAcceptedAPIHosts = map[string]bool{
	"server.codeium.com":             true,
	"server.enterprise.windsurf.com": true,
}

// devinOAuthProvider is the registry entry for provider id "devin".
type devinOAuthProvider struct {
	client       *http.Client
	authorizeURL string
	tokenURL     string
	host         string
	port         int
	path         string
	// cliPaths overrides the credential-file probe order (tests).
	cliPaths func() []string
	// identityBaseURL overrides the GetUserStatus endpoint host (tests).
	identityBaseURL string
	// acceptedHosts overrides the api_server_url allowlist (tests).
	acceptedHosts map[string]bool
	now           func() time.Time
}

func newDevinOAuthProvider() *devinOAuthProvider {
	return &devinOAuthProvider{
		client:       http.DefaultClient,
		authorizeURL: devinAuthorizeURL,
		tokenURL:     devinTokenURL,
		host:         "127.0.0.1",
		port:         devinCallbackPort,
		path:         devinCallbackPath,
	}
}

func (p *devinOAuthProvider) ID() string               { return devinProviderName }
func (p *devinOAuthProvider) Name() string             { return "Devin" }
func (p *devinOAuthProvider) UsesCallbackServer() bool { return true }
func (p *devinOAuthProvider) IsSubscription() bool     { return true }

func (p *devinOAuthProvider) httpClient() *http.Client {
	if p.client != nil {
		return p.client
	}
	return http.DefaultClient
}

func (p *devinOAuthProvider) Login(callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return p.LoginContext(context.Background(), callbacks)
}

func (p *devinOAuthProvider) LoginContext(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	method := devinLoginBrowserMethod
	if callbacks.OnSelectContext != nil || callbacks.OnSelect != nil {
		prompt := OAuthSelectPrompt{
			Message: "Sign in to Devin",
			Options: []OAuthSelectOption{
				{ID: devinLoginBrowserMethod, Label: "Browser sign-in"},
				{ID: devinLoginCLIMethod, Label: "Use Devin CLI credentials"},
			},
		}
		var selected string
		var err error
		if callbacks.OnSelectContext != nil {
			selected, err = callbacks.OnSelectContext(ctx, prompt)
		} else {
			selected, err = callbacks.OnSelect(prompt)
		}
		if err != nil {
			return OAuthCredentials{}, err
		}
		method = selected
	}
	var credentials OAuthCredentials
	var err error
	switch method {
	case devinLoginCLIMethod:
		credentials, err = p.loginCLI(ctx)
	case devinLoginBrowserMethod:
		credentials, err = p.loginBrowser(ctx, callbacks)
	default:
		return OAuthCredentials{}, fmt.Errorf("unknown Devin login method: %s", method)
	}
	if err != nil {
		return OAuthCredentials{}, err
	}
	p.enrichIdentity(ctx, &credentials)
	return credentials, nil
}

// devinJWTExpiry reads the exp claim of a JWT-shaped session token.
func devinJWTExpiry(token string) (int64, bool) {
	exp, _ := devinJWTClaims(token)["exp"].(float64)
	if exp <= 0 {
		return 0, false
	}
	return int64(exp*1000) - devinTokenExpirySkewMillis, true
}

// loginBrowser runs the PKCE authorization-code flow.
func (p *devinOAuthProvider) loginBrowser(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	pkce, err := GeneratePKCE()
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("Devin sign-in failed: generate PKCE: %w", err)
	}
	state := randomUUIDString()

	startCallback := func(port int) (*OAuthCallbackServer[string], error) {
		return StartOAuthCallbackServer(ctx, OAuthCallbackServerOptions[string]{
			ProviderName: "Devin",
			Host:         firstNonEmptyString(p.host, "127.0.0.1"),
			Port:         port,
			Path:         firstNonEmptyString(p.path, devinCallbackPath),
			State:        &state,
			Complete:     func(_ context.Context, code string) (string, error) { return code, nil },
		})
	}
	port := p.port
	if port == 0 {
		port = devinCallbackPort
	}
	callback, err := startCallback(port)
	if err != nil {
		callback, err = startCallback(0)
		if err != nil {
			callback = nil
		}
	}
	if callback != nil {
		defer callback.Close()
	}
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", port, firstNonEmptyString(p.path, devinCallbackPath))
	if callback != nil {
		redirectURI = callback.RedirectURI
	}

	authorize, err := url.Parse(firstNonEmptyString(p.authorizeURL, devinAuthorizeURL))
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("Devin sign-in failed: %w", err)
	}
	query := url.Values{}
	query.Set("response_type", "code")
	query.Set("redirect_uri", redirectURI)
	query.Set("code_challenge", pkce.Challenge)
	query.Set("code_challenge_method", "S256")
	query.Set("state", state)
	query.Set("prompt", "select_account")
	authorize.RawQuery = query.Encode()
	if callbacks.OnAuth != nil {
		callbacks.OnAuth(OAuthAuthInfo{
			URL:          authorize.String(),
			Instructions: "Complete the sign-in in your browser. If it runs on another machine, paste the final redirect URL here.",
		})
	}

	interaction := manualCodeInteraction(callbacks)
	result, err := WaitForCallbackOrManualInput(ctx, interaction, callback, AuthManualCodePrompt{
		Message:     "Complete sign-in in your browser, or paste the redirect URL here:",
		Placeholder: redirectURI,
	})
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("Devin sign-in failed: %w", err)
	}
	code := result.Input
	if result.Callback {
		code = result.Value
	} else {
		var manualState string
		code, manualState, err = devinParseAuthorizationInput(result.Input)
		if err != nil {
			return OAuthCredentials{}, fmt.Errorf("Devin sign-in failed: %w", err)
		}
		if manualState != "" && manualState != state {
			return OAuthCredentials{}, fmt.Errorf("Devin sign-in failed: state mismatch")
		}
	}
	if strings.TrimSpace(code) == "" {
		return OAuthCredentials{}, fmt.Errorf("Devin sign-in failed: missing authorization code")
	}

	token, err := p.exchangeCode(ctx, code, pkce.Verifier)
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("Devin sign-in failed: %w", err)
	}
	return p.credentialFromToken(token), nil
}

// devinParseAuthorizationInput accepts a redirect URL, a query string, a bare
// code, or the code#state fragment form. The fragment state overrides a state
// found in the query.
func devinParseAuthorizationInput(input string) (code, state string, err error) {
	trimmed := trimJSWhitespace(input)
	if trimmed == "" {
		return "", "", errors.New("empty authorization input")
	}
	var fragment string
	if strings.Contains(trimmed, "://") {
		parsed, parseErr := url.Parse(trimmed)
		if parseErr != nil {
			return "", "", fmt.Errorf("invalid redirect URL: %w", parseErr)
		}
		query := parsed.Query()
		code, state = query.Get("code"), query.Get("state")
		fragment = parsed.Fragment
	} else if strings.Contains(trimmed, "#") {
		parts := strings.SplitN(trimmed, "#", 2)
		code, fragment = parts[0], parts[1]
		if strings.Contains(code, "=") {
			values, _ := url.ParseQuery(code)
			code, state = values.Get("code"), values.Get("state")
		}
	} else if strings.Contains(trimmed, "=") {
		values, parseErr := url.ParseQuery(trimmed)
		if parseErr != nil {
			return "", "", fmt.Errorf("invalid authorization query: %w", parseErr)
		}
		code, state = values.Get("code"), values.Get("state")
	} else {
		code = trimmed
	}
	// A fragment carries code#state or code=..&state=.. and overrides the query state.
	if fragment != "" {
		if strings.Contains(fragment, "=") {
			values, _ := url.ParseQuery(fragment)
			if values.Get("code") != "" {
				code = values.Get("code")
			}
			if values.Get("state") != "" {
				state = values.Get("state")
			}
		} else if strings.Contains(fragment, "#") {
			parts := strings.SplitN(fragment, "#", 2)
			if parts[0] != "" {
				code = parts[0]
			}
			if parts[1] != "" {
				state = parts[1]
			}
		} else if code == "" {
			code = fragment
		} else {
			state = fragment
		}
	}
	return code, state, nil
}

// exchangeCode posts the authorization code and verifier to the token endpoint.
func (p *devinOAuthProvider) exchangeCode(ctx context.Context, code, verifier string) (string, error) {
	body, err := json.Marshal(map[string]string{"code": code, "code_verifier": verifier})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, firstNonEmptyString(p.tokenURL, devinTokenURL), strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}
	var decoded struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return "", fmt.Errorf("token exchange returned an unreadable response")
	}
	if strings.TrimSpace(decoded.Token) == "" {
		return "", fmt.Errorf("token exchange returned no token")
	}
	return decoded.Token, nil
}

// credentialFromToken builds the stored credential: access and refresh are the
// session token; expiry comes from the JWT exp claim.
func (p *devinOAuthProvider) credentialFromToken(token string) OAuthCredentials {
	credentials := ompKeyCredential(token)
	credentials.order = credentialKeyOrder{keys: accessFirstCredentialMembers}
	if expires, ok := devinJWTExpiry(token); ok {
		credentials.SetExpiresMillis(float64(expires))
	} else {
		now := time.Now()
		if p.now != nil {
			now = p.now()
		}
		credentials.SetExpiresMillis(float64(now.Add(devinFallbackExpiryMillis * time.Millisecond).UnixMilli()))
	}
	return credentials
}

// loginCLI adopts the official Devin CLI's credentials.toml.
func (p *devinOAuthProvider) loginCLI(ctx context.Context) (OAuthCredentials, error) {
	path, err := p.cliCredentialPath()
	if err != nil {
		return OAuthCredentials{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return OAuthCredentials{}, fmt.Errorf("Devin CLI credentials unreadable: %w", err)
	}
	var parsed struct {
		WindsurfAPIKey string `toml:"windsurf_api_key"`
		APIServerURL   string `toml:"api_server_url"`
	}
	if _, err := toml.Decode(string(data), &parsed); err != nil {
		return OAuthCredentials{}, fmt.Errorf("Devin CLI credentials are not valid TOML: %w", err)
	}
	key := strings.TrimSpace(parsed.WindsurfAPIKey)
	if key == "" {
		return OAuthCredentials{}, fmt.Errorf("Devin CLI credentials at %s have no windsurf_api_key", path)
	}
	serverURL := strings.TrimSpace(parsed.APIServerURL)
	if serverURL != "" {
		host := serverURL
		if parsed, parseErr := url.Parse(serverURL); parseErr == nil && parsed.Host != "" {
			host = parsed.Hostname()
		} else if idx := strings.Index(host, "://"); idx >= 0 {
			host = host[idx+3:]
		}
		if idx := strings.Index(host, ":"); idx >= 0 {
			host = host[:idx]
		}
		if !p.apiHostAllowed(host) {
			return OAuthCredentials{}, fmt.Errorf("Devin CLI api_server_url host %q is not a supported Devin server", host)
		}
	}
	// An adopted windsurf_api_key is a non-expiring seat key, not a session
	// JWT: it keeps ompKeyCredential's never-expires expiry.
	credentials := ompKeyCredential(key)
	credentials.order = credentialKeyOrder{keys: accessFirstCredentialMembers}
	if serverURL != "" {
		encoded, _ := json.Marshal(serverURL)
		credentials.Extra = map[string]json.RawMessage{"apiServerUrl": encoded}
	}
	return credentials, nil
}

func (p *devinOAuthProvider) apiHostAllowed(host string) bool {
	hosts := devinAcceptedAPIHosts
	if p.acceptedHosts != nil {
		hosts = p.acceptedHosts
	}
	return hosts[strings.ToLower(host)]
}

// cliCredentialPath probes the Devin CLI credential locations in omp's order
// and returns the first file that exists.
func (p *devinOAuthProvider) cliCredentialPath() (string, error) {
	paths := devinCLICredentialPaths
	if p.cliPaths != nil {
		paths = p.cliPaths
	}
	for _, candidate := range paths() {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no Devin CLI credentials file found")
}

// devinCLICredentialPaths is the probe order of devin-cli.ts, deduped:
// $APPDATA (windows), $XDG_DATA_HOME, ~/.local/share, ~/Library/Application
// Support (non-windows).
func devinCLICredentialPaths() []string {
	var paths []string
	seen := map[string]bool{}
	add := func(path string) {
		if path != "" && !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	if runtime.GOOS == "windows" {
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			add(filepath.Join(appdata, "devin", "credentials.toml"))
		}
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		add(filepath.Join(xdg, "devin", "credentials.toml"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		add(filepath.Join(home, ".local", "share", "devin", "credentials.toml"))
		if runtime.GOOS != "windows" {
			add(filepath.Join(home, "Library", "Application Support", "devin", "credentials.toml"))
		}
	}
	return paths
}

// enrichIdentity fills account identity from the Devin usage/status endpoint;
// a failed lookup keeps the credential.
func (p *devinOAuthProvider) enrichIdentity(ctx context.Context, credentials *OAuthCredentials) {
	token := devinCredentialToken(UsageCredential{Type: CredentialOAuth, AccessToken: credentials.Access})
	if token == "" {
		return
	}
	baseURL := devinDefaultBaseURL
	if p.identityBaseURL != "" {
		baseURL = p.identityBaseURL
	} else if raw, ok := credentials.Extra["apiServerUrl"]; ok {
		var serverURL string
		if json.Unmarshal(raw, &serverURL) == nil && serverURL != "" {
			baseURL = serverURL
		}
	}
	lookupCtx, cancel := context.WithTimeout(ctx, devinIdentityTimeout)
	defer cancel()
	report, err := fetchDevinUsage(lookupCtx, UsageFetchParams{
		Provider:   devinProviderName,
		Credential: UsageCredential{Type: CredentialOAuth, AccessToken: credentials.Access},
		BaseURL:    baseURL,
	}, UsageFetchContext{HTTPClient: p.httpClient()})
	if err != nil || report == nil || report.Metadata == nil {
		return
	}
	metadata := report.Metadata
	stringField := func(key string) string {
		value, _ := metadata[key].(string)
		return strings.TrimSpace(value)
	}
	if accountID := stringField("accountId"); accountID != "" {
		// One user can sign in to several orgs; the org scopes the account so
		// distinct org logins do not collapse (the mistral |workspace: pattern).
		if orgID := stringField("orgId"); orgID != "" {
			credentials.AccountID = accountID + "|org:" + orgID
		} else {
			credentials.AccountID = accountID
		}
	}
	if credentials.Extra == nil {
		credentials.Extra = map[string]json.RawMessage{}
	}
	for _, key := range []string{"email", "orgId", "orgName"} {
		if value := stringField(key); value != "" {
			encoded, _ := json.Marshal(value)
			credentials.Extra[key] = encoded
		}
	}
}

// RefreshToken returns the credential unchanged: a Devin session token has no
// refresh grant. An already-expired token fails with a re-login error instead
// of letting every request fail opaquely; the never-expires CLI seat key is
// unaffected.
func (p *devinOAuthProvider) RefreshToken(creds OAuthCredentials) (OAuthCredentials, error) {
	expires := creds.ExpiresMillis()
	if !math.IsNaN(expires) && expires < ompNeverExpiresMs && float64(nowMillis()) >= expires {
		return OAuthCredentials{}, errors.New("Devin session expired; run /login again")
	}
	return creds, nil
}

func (p *devinOAuthProvider) GetAPIKey(creds OAuthCredentials) string {
	return creds.Access
}
