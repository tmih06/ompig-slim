package ai

// ompig-slim addition: imported, short-lived Codex accounts use the normal Codex
// API and account router, but only definitive refresh failures retire their secrets.

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

const codexJSONProviderID = "codex-json"
const codexJSONName = "Codex JSON"

// codexTokenRequestError preserves the existing Codex error text while exposing
// structured status/code to the imported-account lifecycle. Never classify by prose.
type codexTokenRequestError struct {
	status        int
	code, message string
}

// Error returns the original token-endpoint diagnostic for existing Codex callers.
func (e *codexTokenRequestError) Error() string { return e.message }

// codexTokenErrorCode reads both OAuth's string error and OpenAI's nested code;
// malformed bodies have no code and must not trigger destructive cleanup.
func codexTokenErrorCode(body []byte) string {
	var response struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &response) != nil {
		return ""
	}
	var code string
	if json.Unmarshal(response.Error, &code) == nil {
		return code
	}
	var nested struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(response.Error, &nested)
	return nested.Code
}

// errCodexJSONExpired is consumed by SQLite under the refresh lease. Its message
// also lets the existing account router rotate without replaying streamed output.
var errCodexJSONExpired = errors.New("Codex JSON token expired or revoked; import a new account")

// isCodexAccountProvider identifies providers sharing Codex entitlement and quota semantics.
func isCodexAccountProvider(id string) bool { return id == "openai-codex" || id == codexJSONProviderID }

// parseCodexJSON imports only request/refresh material and account identity. It
// never persists the input object, idToken, labels, or subscription metadata.
// Absolute expiry wins over expiresIn; JWT expiry caps either claimed lifetime.
// JWT claims are metadata, not signature verification or proof of entitlement.
func parseCodexJSON(input string, now time.Time) (OAuthCredentials, error) {
	var data struct {
		Access    string   `json:"accessToken"`
		Refresh   string   `json:"refreshToken"`
		ExpiresAt string   `json:"expiresAt"`
		ExpiresIn *float64 `json:"expiresIn"`
		Provider  struct {
			AccountID string `json:"chatgptAccountId"`
		} `json:"providerSpecificData"`
	}
	if len(input) > 1<<20 || json.Unmarshal([]byte(input), &data) != nil {
		return OAuthCredentials{}, errors.New("Codex JSON: expected a JSON object with accessToken and expiry")
	}
	data.Access = strings.TrimSpace(data.Access)
	data.Refresh = strings.TrimSpace(data.Refresh)
	if data.Access == "" {
		return OAuthCredentials{}, errors.New("Codex JSON: accessToken is required")
	}
	account := CodexAccountID(data.Access)
	supplied := strings.TrimSpace(data.Provider.AccountID)
	if account != "" && supplied != "" && account != supplied {
		return OAuthCredentials{}, errors.New("Codex JSON: account ID does not match accessToken")
	}
	if account == "" {
		account = supplied
	}
	if account == "" {
		return OAuthCredentials{}, errors.New("Codex JSON: chatgptAccountId is required when absent from accessToken")
	}
	var expiry time.Time
	if data.ExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, data.ExpiresAt)
		if err != nil {
			return OAuthCredentials{}, errors.New("Codex JSON: expiresAt must be an RFC3339 timestamp")
		}
		expiry = parsed
	} else if data.ExpiresIn != nil {
		// Bound before converting to duration to prevent overflow; imported accounts
		// need no lifetime beyond one year, but short lives are not hard-coded.
		if *data.ExpiresIn <= 0 || *data.ExpiresIn > 365*24*60*60 {
			return OAuthCredentials{}, errors.New("Codex JSON: expiresIn must be positive and at most one year (seconds)")
		}
		expiry = now.Add(time.Duration(*data.ExpiresIn * float64(time.Second)))
	}
	if claims, ok := usageJWTClaims(data.Access); ok {
		if value, present := claims["exp"]; present {
			seconds, ok := value.(float64)
			// JWT NumericDate is seconds since the epoch; reject invalid claims
			// rather than letting a caller-provided expiry resurrect a dead token.
			if !ok || seconds <= 0 || seconds >= 253402300800 {
				return OAuthCredentials{}, errors.New("Codex JSON: invalid accessToken expiry")
			}
			jwtExpiry := time.UnixMilli(int64(seconds * 1000))
			if expiry.IsZero() || jwtExpiry.Before(expiry) {
				expiry = jwtExpiry
			}
		}
	}
	if expiry.IsZero() || !expiry.After(now) {
		return OAuthCredentials{}, errors.New("Codex JSON: missing or expired token expiry")
	}
	return OAuthCredentials{Access: data.Access, Refresh: data.Refresh, Expires: expiry.UnixMilli(), AccountID: account}, nil
}

// codexJSONOAuthProvider participates in the same login/storage lifecycle as the
// browser Codex provider, without opening a browser or accepting arbitrary hosts.
type codexJSONOAuthProvider struct{}

// ID returns the isolated storage and routing namespace of imported accounts.
func (codexJSONOAuthProvider) ID() string { return codexJSONProviderID }

// Name returns the login picker label.
func (codexJSONOAuthProvider) Name() string { return codexJSONName }

// IsSubscription enables the normal Codex subscription presentation.
func (codexJSONOAuthProvider) IsSubscription() bool { return true }

// UsesCallbackServer reports that importing JSON needs no local callback listener.
func (codexJSONOAuthProvider) UsesCallbackServer() bool { return false }

// GetAPIKey returns only the request bearer, never the pasted JSON.
func (codexJSONOAuthProvider) GetAPIKey(c OAuthCredentials) string { return c.Access }

// Login supports callback-based hosts; native hosts use the secret prompt below.
func (p codexJSONOAuthProvider) Login(cb OAuthLoginCallbacks) (OAuthCredentials, error) {
	return p.LoginContext(context.Background(), cb)
}

// LoginContext asks once for the complete JSON string and propagates cancellation.
func (codexJSONOAuthProvider) LoginContext(ctx context.Context, cb OAuthLoginCallbacks) (OAuthCredentials, error) {
	if err := ctx.Err(); err != nil {
		return OAuthCredentials{}, err
	}
	prompt := OAuthPrompt{Message: "Paste Codex JSON credentials"}
	var input string
	var err error
	if cb.OnPromptContext != nil {
		input, err = cb.OnPromptContext(ctx, prompt)
	} else if cb.OnPrompt != nil {
		input, err = cb.OnPrompt(prompt)
	} else {
		return OAuthCredentials{}, errors.New("Codex JSON requires an input prompt")
	}
	if err != nil {
		return OAuthCredentials{}, err
	}
	if err := ctx.Err(); err != nil {
		return OAuthCredentials{}, err
	}
	return parseCodexJSON(input, time.Now())
}

// LoginInteraction uses a secret input so native TUI/CLI hosts mask pasted tokens.
func (codexJSONOAuthProvider) LoginInteraction(ctx context.Context, interaction AuthInteraction, _ LoginOptions) (OAuthCredentials, error) {
	if err := ctx.Err(); err != nil {
		return OAuthCredentials{}, err
	}
	input, err := interaction.Prompt(ctx, AuthSecretPrompt{Message: "Paste Codex JSON credentials"})
	if err != nil {
		return OAuthCredentials{}, err
	}
	if err := ctx.Err(); err != nil {
		return OAuthCredentials{}, err
	}
	return parseCodexJSON(input, time.Now())
}

// RefreshToken is the compatibility entry point for hosts without refresh contexts.
func (p codexJSONOAuthProvider) RefreshToken(c OAuthCredentials) (OAuthCredentials, error) {
	return p.RefreshTokenContext(context.Background(), c)
}

// RefreshTokenContext retires only missing or definitively rejected refresh
// tokens. Transport errors, cancellation, rate limits and server errors retain
// credentials. Endpoint bodies are not surfaced because they may contain secrets.
func (codexJSONOAuthProvider) RefreshTokenContext(ctx context.Context, c OAuthCredentials) (OAuthCredentials, error) {
	if err := ctx.Err(); err != nil {
		return OAuthCredentials{}, err
	}
	if c.Refresh == "" {
		return OAuthCredentials{}, errCodexJSONExpired
	}
	refreshed, err := RefreshCodexToken(ctx, c.Refresh)
	if err != nil {
		var rejected *codexTokenRequestError
		if errors.As(err, &rejected) && (rejected.status == 400 || rejected.status == 401) {
			switch rejected.code {
			case "invalid_grant", "refresh_token_expired", "refresh_token_reused", "refresh_token_invalidated", "invalid_refresh_token":
				return OAuthCredentials{}, errCodexJSONExpired
			}
		}
		return OAuthCredentials{}, errors.New("Codex JSON token refresh failed; credentials retained for retry")
	}
	if account := CodexAccountID(refreshed.Access); account != "" && account != c.AccountID {
		return OAuthCredentials{}, errors.New("Codex JSON refresh account mismatch; credentials retained")
	}
	refreshed.AccountID = c.AccountID
	// JSON accounts retain only the minimal import fields, even though the
	// shared token endpoint enriches regular browser/device login profiles.
	refreshed.Extra = nil
	return refreshed, nil
}

// init derives the imported provider's roster from the canonical Codex catalog;
// API implementations, pricing, reasoning capabilities and model IDs stay shared.
func init() {
	builtInOAuthProviders[codexJSONProviderID] = codexJSONOAuthProvider{}
	ompOAuthProviderDisplays[codexJSONProviderID] = struct {
		Name           string
		IsSubscription bool
		LoginLabel     string
	}{Name: codexJSONName, IsSubscription: true}
	catalog, owned := OMPChatCatalog("openai-codex")
	if !owned {
		catalog = GeneratedModels
	}
	for _, model := range slices.Clone(catalog) {
		if model.Provider == "openai-codex" {
			model.Provider = codexJSONProviderID
			// The raw catalog record is also used by RPC/model serialization.
			model.Record = strings.Replace(model.Record, `"provider":"openai-codex"`, `"provider":"codex-json"`, 1)
			GeneratedModels = append(GeneratedModels, model)
		}
	}
	GeneratedProviders = append(GeneratedProviders, codexJSONProviderID)
	slices.Sort(GeneratedProviders)
}

// codexJSONToAuth supplies the required account header even for opaque access
// tokens whose account was provided explicitly in the import.
func codexJSONToAuth(c Credential) (ModelAuth, error) {
	if c.Access == "" || c.AccountID == "" {
		return ModelAuth{}, errors.New("Codex JSON: missing access token or account ID")
	}
	return codexRequestAuth(c), nil
}
