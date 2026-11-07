package ai

// Registration for the four OAuth sign-in flows this harness adds over stock PiG:
// Z.ai (GLM Coding Plan · Sign in), Cursor, Snowflake Cortex and Perplexity.
//
// They are registered the same way the stock flows are — in the built-in OAuth
// provider registry — so /login, `auth login`, the provider selector and the
// model runtime all see them without further wiring. The flows themselves live in
// omp_oauth_zai.go, omp_oauth_cursor.go, omp_oauth_snowflake.go and
// omp_oauth_perplexity.go.
//
// API-key auth is registered beside them because a provider's auth record only
// carries an OAuth method when the provider has an auth record at all: without
// one the selector would list the flow with no method attached. Z.ai already had
// its ZAI_API_KEY method; the other three gain the environment key OMP documents
// (CURSOR_ACCESS_TOKEN, PERPLEXITY_API_KEY, SNOWFLAKE_PAT).

import (
	"context"
	"encoding/json"
	"strings"
)

const (
	cursorAPIKeyName     = "Cursor API key"
	perplexityAPIKeyName = "Perplexity API key"
	snowflakeAPIKeyName  = "Snowflake token or PAT"
	cursorAPIKeyEnv      = "CURSOR_ACCESS_TOKEN"
	perplexityAPIKeyEnv  = "PERPLEXITY_API_KEY"
	snowflakePATEnv      = "SNOWFLAKE_PAT"
	snowflakeAccountEnv  = "SNOWFLAKE_ACCOUNT"
)

// ompOAuthProviderDisplays lists the lazyOAuth metadata of the flows registered
// here. Keeping it beside the flows (rather than in auth_providers.go's stock
// tables) means a regenerated stock table cannot drop them.
var ompOAuthProviderDisplays = map[string]struct {
	Name           string
	IsSubscription bool
	LoginLabel     string
}{
	"zai":        {Name: zaiOAuthName, IsSubscription: true, LoginLabel: zaiOAuthLabel},
	"cursor":     {Name: cursorOAuthName},
	"snowflake":  {Name: snowflakeOAuthName, IsSubscription: true},
	"perplexity": {Name: perplexityOAuthName, IsSubscription: true},
}

// ompOAuthDisplay reports the registered display metadata for one flow.
func ompOAuthDisplay(providerID string) (name string, subscription bool, loginLabel string, ok bool) {
	display, ok := ompOAuthProviderDisplays[providerID]
	if !ok {
		return "", false, "", false
	}
	return display.Name, display.IsSubscription, display.LoginLabel, true
}

func init() {
	for id, provider := range map[string]OAuthProviderInterface{
		"zai":        zaiOAuthProvider{},
		"cursor":     cursorOAuthProvider{},
		"snowflake":  snowflakeOAuthProvider{},
		"perplexity": perplexityOAuthProvider{},
	} {
		builtInOAuthProviders[id] = provider
	}
}

// ompOAuthProviderAPIKey is the API-key auth of a provider whose sign-in flow
// this file registers, or reports false for any other provider.
func ompOAuthProviderAPIKey(providerID string) (*APIKeyAuth, bool) {
	switch providerID {
	case codexJSONProviderID:
		// The JSON importer is OAuth-only, not an ambient API-key login.
		return nil, true
	case "cursor":
		return EnvAPIKeyAuth(cursorAPIKeyName, cursorAPIKeyEnv), true
	case "perplexity":
		return EnvAPIKeyAuth(perplexityAPIKeyName, perplexityAPIKeyEnv), true
	case "snowflake":
		return snowflakeAPIKeyAuth(), true
	}
	return nil, false
}

// snowflakeAPIKeyAuth resolves a Snowflake request credential: a structured OAuth
// key carrying its account, a bare token with SNOWFLAKE_ACCOUNT, or a bare PAT.
func snowflakeAPIKeyAuth() *APIKeyAuth {
	return &APIKeyAuth{
		Name:  snowflakeAPIKeyName,
		Login: snowflakeAPIKeyLogin,
		Resolve: func(ctx context.Context, input APIKeyAuthInput) (*AuthResult, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if input.Credential != nil && input.Credential.Key != "" {
				result, err := snowflakeModelAuth(input.Credential.Key)
				if err != nil {
					return nil, err
				}
				result.Env = input.Credential.Env
				result.Source = "stored credential"
				return result, nil
			}
			token, tokenSet := input.Ctx.Env(snowflakePATEnv)
			token = strings.TrimSpace(token)
			if !tokenSet || token == "" {
				return nil, nil
			}
			if account, set := input.Ctx.Env(snowflakeAccountEnv); set && strings.TrimSpace(account) != "" {
				accountURL, err := NormalizeSnowflakeAccountURL(account)
				if err != nil {
					return nil, err
				}
				return &AuthResult{Auth: ModelAuth{APIKey: token, BaseURL: accountURL}, Source: snowflakePATEnv}, nil
			}
			return &AuthResult{Auth: ModelAuth{APIKey: token}, Source: snowflakePATEnv}, nil
		},
	}
}

// snowflakeAPIKeyLogin asks for a PAT and the account it belongs to, because the
// Cortex endpoint hangs off the account host.
func snowflakeAPIKeyLogin(ctx context.Context, interaction AuthInteraction) (Credential, error) {
	if ctx.Err() != nil {
		return Credential{}, context.Cause(ctx)
	}
	credential, err := promptAPIKey(ctx, interaction, "Enter Snowflake PAT")
	if err != nil {
		return Credential{}, err
	}
	account, err := interaction.Prompt(ctx, AuthTextPrompt{Message: "Enter your Snowflake account identifier or URL", Placeholder: "myorg-myaccount"})
	if err != nil {
		return Credential{}, err
	}
	if trimmed := strings.TrimSpace(account); trimmed != "" {
		if _, err := NormalizeSnowflakeAccountURL(trimmed); err != nil {
			return Credential{}, err
		}
		credential.Env = map[string]string{snowflakeAccountEnv: trimmed}
	}
	return credential, nil
}

// snowflakeModelAuth decodes a Snowflake request credential. A value that starts
// with `{` is the structured JSON a login stores; anything else is a bare bearer
// token, which then depends on the model endpoint or SNOWFLAKE_ACCOUNT for its
// host.
func snowflakeModelAuth(value string) (*AuthResult, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, NewModelsError(ModelsErrorAuth, "Invalid Snowflake credential; run /login snowflake again", nil)
	}
	if !strings.HasPrefix(trimmed, "{") {
		return &AuthResult{Auth: ModelAuth{APIKey: trimmed}}, nil
	}
	var stored struct {
		Token         *string `json:"token"`
		EnterpriseURL *string `json:"enterpriseUrl"`
	}
	if err := json.Unmarshal([]byte(trimmed), &stored); err != nil {
		return nil, NewModelsError(ModelsErrorAuth, "Invalid Snowflake credential; run /login snowflake again", nil)
	}
	if stored.Token == nil || strings.TrimSpace(*stored.Token) == "" ||
		stored.EnterpriseURL == nil || strings.TrimSpace(*stored.EnterpriseURL) == "" {
		// A structured key is an OAuth bearer; without its account it must not get PAT routing.
		return nil, NewModelsError(ModelsErrorAuth, "Invalid Snowflake credential; run /login snowflake again", nil)
	}
	accountURL, err := NormalizeSnowflakeAccountURL(*stored.EnterpriseURL)
	if err != nil {
		return nil, NewModelsError(ModelsErrorAuth, "Invalid Snowflake credential account; run /login snowflake again", err)
	}
	return &AuthResult{Auth: ModelAuth{APIKey: strings.TrimSpace(*stored.Token), BaseURL: accountURL}}, nil
}

// ompOAuthBrowserToAuth is the request auth of the flows registered above for a
// provider whose stored credential is not the bare request bearer. A Snowflake
// login stores its bearer together with the account host, and the request needs
// the bearer as its key while the host replaces the catalog's placeholder origin
// with the model's own path appended (see omp_oauth_snowflake_catalog.go).
func ompOAuthBrowserToAuth(providerID string, credential Credential) (ModelAuth, bool) {
	if providerID != "snowflake" {
		return ModelAuth{}, false
	}
	if account := strings.TrimSpace(credential.EnterpriseDomain); account != "" {
		return ModelAuth{APIKey: strings.TrimSpace(credential.Access)}, true
	}
	// Otherwise the key may be the structured JSON the API-key method writes.
	if result, err := snowflakeModelAuth(credential.Access); err == nil {
		return ModelAuth{APIKey: result.Auth.APIKey}, true
	}
	return ModelAuth{APIKey: credential.Access}, true
}

// OAuthEnv implements ompOAuthCredentialEnv: a stored Snowflake login contributes
// the account host its endpoints hang off.
func (snowflakeOAuthProvider) OAuthEnv(creds OAuthCredentials) map[string]string {
	account := ompOAuthExtraString(creds, "enterpriseUrl")
	if account == "" {
		return nil
	}
	normalized, err := NormalizeSnowflakeAccountURL(account)
	if err != nil {
		return nil
	}
	return map[string]string{snowflakeAccountEnv: normalized}
}

// snowflakeRequestModel materializes the account host in a Snowflake model's base
// URL: the catalog carries the `.invalid` placeholder origin, and the request goes
// to the credential's account with the model's own path appended.
func snowflakeRequestModel(model *Model, env ProviderEnv) *Model {
	account := strings.TrimRight(env[snowflakeAccountEnv], "/")
	if account == "" || model == nil {
		return model
	}
	baseURL := model.ProviderMeta.BaseURL
	if !strings.HasPrefix(baseURL, snowflakeAccountPlaceholder) {
		return model
	}
	resolved := *model
	resolved.ProviderMeta.BaseURL = account + baseURL[len(snowflakeAccountPlaceholder):]
	return &resolved
}

// snowflakeStreams wraps the chat implementations so the account host substitutes
// before dispatch, mirroring how Cloudflare's placeholders materialize
// (cloudflare-streams.ts).
func snowflakeStreams(streams *ProviderStreams) *ProviderStreams {
	var wrapped ProviderStreams
	if streams.Stream != nil {
		stream := streams.Stream
		wrapped.Stream = func(ctx context.Context, model *Model, transcript TranscriptContext, options StreamOptions) (*AssistantMessageEventStream, error) {
			return stream(ctx, snowflakeRequestModel(model, options.Env), transcript, options)
		}
	}
	if streams.StreamSimple != nil {
		streamSimple := streams.StreamSimple
		wrapped.StreamSimple = func(ctx context.Context, model *Model, transcript TranscriptContext, options StreamOptions) (*AssistantMessageEventStream, error) {
			return streamSimple(ctx, snowflakeRequestModel(model, options.Env), transcript, options)
		}
	}
	return &wrapped
}

// ompProviderDisplayName and ompProviderBaseURL are the catalog facts of a provider
// the OAuth ports register. Snowflake and Codex JSON add their own rosters;
// Cursor, Perplexity and Z.ai keep their existing catalog rows.
func ompProviderDisplayName(providerID string) (string, bool) {
	if providerID == codexJSONProviderID {
		return codexJSONName, true
	}
	if providerID != "snowflake" {
		return "", false
	}
	return snowflakeOAuthName, true
}

// ompProviderBaseURL is the provider-level endpoint of a provider the OAuth ports
// register. Codex JSON shares Codex's origin; Snowflake's placeholder is replaced
// with the account host on every request.
func ompProviderBaseURL(providerID string) (string, bool) {
	if providerID == codexJSONProviderID {
		return ProviderBaseURL("openai-codex"), true
	}
	if providerID != "snowflake" {
		return "", false
	}
	return snowflakeAccountPlaceholder, true
}

// ompStreamWrapper is the stream wrapper of a provider the OAuth ports register.
func ompStreamWrapper(providerID string) func(*ProviderStreams) *ProviderStreams {
	if providerID != "snowflake" {
		return nil
	}
	return snowflakeStreams
}

// ompProviderConstructor is the provider constructor of a provider the OAuth ports
// register, for BuiltinProviders.
func ompProviderConstructor(providerID string) (func() *ModelsProvider, bool) {
	if providerID != "snowflake" {
		return nil, false
	}
	return snowflakeProvider, true
}

// SnowflakeProvider is the Snowflake Cortex provider.
func SnowflakeProvider() *ModelsProvider { return snowflakeProvider() }

// ompProviderAPIKeyEnvs is getAPIKeyEnvVars for a provider the OAuth ports
// register. Snowflake's PAT is the ambient key; its account host is separate
// configuration.
func ompProviderAPIKeyEnvs(providerID string) ([]string, bool) {
	switch providerID {
	case "cursor":
		return []string{cursorAPIKeyEnv}, true
	case "perplexity":
		return []string{perplexityAPIKeyEnv}, true
	case "snowflake":
		return []string{snowflakePATEnv}, true
	}
	return nil, false
}
