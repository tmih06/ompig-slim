package ai

// Ports the two Cloudflare composite-key logins (node_modules/@oh-my-pi/pi-ai/src/registry/oauth/
// cloudflare-workers-ai.ts and cloudflare-ai-gateway.ts) over the wire form of
// @oh-my-pi/pi-catalog wire/cloudflare.ts.
//
// Both collect a bearer token plus the routing ids the endpoint needs, then store them as one JSON
// api key `{token, accountId?, gatewayId?}`. The request layer decodes that string: Workers AI uses
// the account id to fill `{CLOUDFLARE_ACCOUNT_ID}` and the AI Gateway uses both ids for its
// `->/v1/<account>/<gateway>` prefix, sending the token as `cf-aig-authorization`.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

const (
	cloudflareTokensAuthURL  = "https://dash.cloudflare.com/profile/api-tokens"
	cloudflareGatewayAuthURL = "https://developers.cloudflare.com/ai-gateway/configuration/authentication/"
)

// cloudflareCredential is the composite a Cloudflare login produces.
type cloudflareCredential struct {
	Token     string `json:"token"`
	AccountID string `json:"accountId,omitempty"`
	GatewayID string `json:"gatewayId,omitempty"`
}

// parseCloudflareCredential reads a stored Cloudflare key. A legacy plain token is a credential with
// no routing ids; a JSON body needs a non-blank token and string ids. ok is false for an empty or
// malformed value (parseCloudflareCredential, wire/cloudflare.ts).
func parseCloudflareCredential(value string) (cloudflareCredential, bool) {
	trimmed := trimJSWhitespace(value)
	if trimmed == "" {
		return cloudflareCredential{}, false
	}
	if !strings.HasPrefix(trimmed, "{") {
		return cloudflareCredential{Token: trimmed}, true
	}
	var stored struct {
		Token     *string `json:"token"`
		AccountID *string `json:"accountId"`
		GatewayID *string `json:"gatewayId"`
	}
	if err := json.Unmarshal([]byte(trimmed), &stored); err != nil {
		return cloudflareCredential{}, false
	}
	if stored.Token == nil || trimJSWhitespace(*stored.Token) == "" {
		return cloudflareCredential{}, false
	}
	credential := cloudflareCredential{Token: trimJSWhitespace(*stored.Token)}
	if stored.AccountID != nil {
		credential.AccountID = trimJSWhitespace(*stored.AccountID)
	}
	if stored.GatewayID != nil {
		credential.GatewayID = trimJSWhitespace(*stored.GatewayID)
	}
	return credential, true
}

// serializeCloudflareCredential is the stored form of a Cloudflare key: the token plus the ids the
// login collected, with blank optional ids omitted (serializeCloudflareCredential).
func serializeCloudflareCredential(credential cloudflareCredential) string {
	stored := cloudflareCredential{Token: trimJSWhitespace(credential.Token)}
	if accountID := trimJSWhitespace(credential.AccountID); accountID != "" {
		stored.AccountID = accountID
	}
	if gatewayID := trimJSWhitespace(credential.GatewayID); gatewayID != "" {
		stored.GatewayID = gatewayID
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return stored.Token
	}
	return string(encoded)
}

// cloudflareWorkersAIOAuthProvider collects the Workers AI token and the account id its
// account-scoped endpoint needs.
type cloudflareWorkersAIOAuthProvider struct {
	client *http.Client
}

func newCloudflareWorkersAIOAuthProvider() cloudflareWorkersAIOAuthProvider {
	return cloudflareWorkersAIOAuthProvider{client: &http.Client{}}
}

func (cloudflareWorkersAIOAuthProvider) ID() string   { return "cloudflare-workers-ai" }
func (cloudflareWorkersAIOAuthProvider) Name() string { return "Cloudflare Workers AI" }
func (cloudflareWorkersAIOAuthProvider) IsSubscription() bool {
	return true
}
func (cloudflareWorkersAIOAuthProvider) UsesCallbackServer() bool { return false }
func (cloudflareWorkersAIOAuthProvider) GetAPIKey(c OAuthCredentials) string {
	return c.Access
}

func (p cloudflareWorkersAIOAuthProvider) RefreshToken(creds OAuthCredentials) (OAuthCredentials, error) {
	return ompUnchangedRefresh(creds)
}

// Login runs the flow without cancellation. Prefer LoginContext.
func (p cloudflareWorkersAIOAuthProvider) Login(callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return p.LoginContext(context.Background(), callbacks)
}

// LoginContext collects the token and the account id.
func (p cloudflareWorkersAIOAuthProvider) LoginContext(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	if err := ompRequirePrompt(callbacks, "Cloudflare Workers AI"); err != nil {
		return OAuthCredentials{}, err
	}
	ompNotifyAuth(callbacks, cloudflareTokensAuthURL, "Create an API token with Workers AI Read and Edit permissions, then copy it here.")

	token, err := ompPrompt(ctx, callbacks, "Cloudflare Workers AI", "Paste your Cloudflare API token", "Workers AI API token", false)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if err := ompCancelled(ctx); err != nil {
		return OAuthCredentials{}, err
	}
	if trimJSWhitespace(token) == "" {
		return OAuthCredentials{}, errors.New("API key is required")
	}

	accountID, err := ompPrompt(ctx, callbacks, "Cloudflare Workers AI", "Enter your Cloudflare account ID", "32-character account ID", false)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if err := ompCancelled(ctx); err != nil {
		return OAuthCredentials{}, err
	}
	if trimJSWhitespace(accountID) == "" {
		return OAuthCredentials{}, errors.New("Cloudflare account ID is required")
	}

	key := serializeCloudflareCredential(cloudflareCredential{Token: token, AccountID: accountID})
	return ompKeyCredential(key), nil
}

// cloudflareAIGatewayOAuthProvider collects the gateway token plus the account and gateway ids its
// `->/v1/<account>/<gateway>` endpoint needs.
type cloudflareAIGatewayOAuthProvider struct {
	client *http.Client
}

func newCloudflareAIGatewayOAuthProvider() cloudflareAIGatewayOAuthProvider {
	return cloudflareAIGatewayOAuthProvider{client: &http.Client{}}
}

func (cloudflareAIGatewayOAuthProvider) ID() string   { return "cloudflare-ai-gateway" }
func (cloudflareAIGatewayOAuthProvider) Name() string { return "Cloudflare AI Gateway" }
func (cloudflareAIGatewayOAuthProvider) IsSubscription() bool {
	return true
}
func (cloudflareAIGatewayOAuthProvider) UsesCallbackServer() bool { return false }
func (cloudflareAIGatewayOAuthProvider) GetAPIKey(c OAuthCredentials) string {
	return c.Access
}

func (p cloudflareAIGatewayOAuthProvider) RefreshToken(creds OAuthCredentials) (OAuthCredentials, error) {
	return ompUnchangedRefresh(creds)
}

// Login runs the flow without cancellation. Prefer LoginContext.
func (p cloudflareAIGatewayOAuthProvider) Login(callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return p.LoginContext(context.Background(), callbacks)
}

// LoginContext collects the token, the account id and the gateway id.
func (p cloudflareAIGatewayOAuthProvider) LoginContext(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	if err := ompRequirePrompt(callbacks, "Cloudflare AI Gateway"); err != nil {
		return OAuthCredentials{}, err
	}
	ompNotifyAuth(callbacks, cloudflareGatewayAuthURL, "Create an AI Gateway token with Run permission, then copy it here.")

	apiKey, err := ompPrompt(ctx, callbacks, "Cloudflare AI Gateway", "Paste your Cloudflare AI Gateway token/API key", "cfut_...", false)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if err := ompCancelled(ctx); err != nil {
		return OAuthCredentials{}, err
	}
	if trimJSWhitespace(apiKey) == "" {
		return OAuthCredentials{}, errors.New("API key is required")
	}

	accountID, err := ompPrompt(ctx, callbacks, "Cloudflare AI Gateway", "Enter your Cloudflare account ID", "32-character account ID", false)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if err := ompCancelled(ctx); err != nil {
		return OAuthCredentials{}, err
	}
	if trimJSWhitespace(accountID) == "" {
		return OAuthCredentials{}, errors.New("Cloudflare account ID is required")
	}

	gatewayID, err := ompPrompt(ctx, callbacks, "Cloudflare AI Gateway", "Enter your Cloudflare AI Gateway ID", "default", false)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if err := ompCancelled(ctx); err != nil {
		return OAuthCredentials{}, err
	}
	if trimJSWhitespace(gatewayID) == "" {
		return OAuthCredentials{}, errors.New("Cloudflare AI Gateway ID is required")
	}

	key := serializeCloudflareCredential(cloudflareCredential{Token: apiKey, AccountID: accountID, GatewayID: gatewayID})
	return ompKeyCredential(key), nil
}
