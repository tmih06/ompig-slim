package ai

// Ports the 16 OpenAI-compatible providers omp-slim adds over stock Pi
// (packages/ai/src/providers/<id>.ts, commit 4b87440d7). Every one is a
// models.dev catalog served on one OpenAI-compatible endpoint, so the only
// provider-level state is its display name, endpoint, environment key and the
// api-key auth method; the catalog itself lives in providers_added_catalog.go.
//
// The registration is additive: provider_names.go, auth_env_keys.go,
// auth_providers.go and builtin_providers.go each look in an extra table here
// before their stock one, so a stock PiG rebuild that regenerates those files
// keeps these providers (see addedProvider* maps and init below).

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
)

// addedProviderInfo is one added provider's static description: the upstream
// provider fields this harness consumes.
type addedProviderInfo struct {
	// Name is provider.name, which /login and login status labels render.
	Name string
	// BaseURL is provider.baseUrl, the provider-level default.
	BaseURL string
	// APIKeyEnv is the environment variable getApiKeyEnvVars maps the provider to.
	APIKeyEnv string
	// APIKeyName is the envApiKeyAuth display name of the provider's API-key method.
	APIKeyName string
	// OAuthName and OAuthLoginLabel are the provider's lazyOAuth name and loginLabel,
	// empty when the provider advertises no OAuth method.
	OAuthName       string
	OAuthLoginLabel string
}

// addedProviders is the static description of each provider this harness adds.
var addedProviders = map[string]addedProviderInfo{
	"aiand": {
		Name:       "ai&",
		BaseURL:    "https://api.aiand.com/v1",
		APIKeyEnv:  "AIAND_API_KEY",
		APIKeyName: "ai& API key",
	},
	"aimlapi": {
		Name:       "AI/ML API",
		BaseURL:    "https://api.aimlapi.com/v1",
		APIKeyEnv:  "AIMLAPI_API_KEY",
		APIKeyName: "AI/ML API API key",
	},
	"alibaba-coding-plan": {
		Name:            "Alibaba Coding Plan",
		BaseURL:         "https://coding-intl.dashscope.aliyuncs.com/v1",
		APIKeyEnv:       "ALIBABA_CODING_PLAN_API_KEY",
		APIKeyName:      "Alibaba Coding Plan API key",
		OAuthName:       "Alibaba Coding Plan",
		OAuthLoginLabel: "Sign in with Alibaba",
	},
	"alibaba-token-plan": {
		Name:            "Alibaba Token Plan",
		BaseURL:         "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1",
		APIKeyEnv:       "ALIBABA_TOKEN_PLAN_API_KEY",
		APIKeyName:      "Alibaba Token Plan API key",
		OAuthName:       "QwenCloud Token Plan",
		OAuthLoginLabel: "Sign in with QwenCloud",
	},
	"cline-pass": {
		Name:       "ClinePass",
		BaseURL:    "https://api.cline.bot/api/v1",
		APIKeyEnv:  "CLINE_API_KEY",
		APIKeyName: "ClinePass API key",
	},
	"commandcode": {
		Name:       "CommandCode",
		BaseURL:    "https://api.commandcode.ai/provider/v1",
		APIKeyEnv:  "CMD_API_KEY",
		APIKeyName: "CommandCode API key",
	},
	"coralbricks": {
		Name:       "CoralBricks",
		BaseURL:    "https://inference.coralbricks.ai/v1",
		APIKeyEnv:  "CORAL_API_KEY",
		APIKeyName: "CoralBricks API key",
	},
	"deepinfra": {
		Name:       "Deep Infra",
		BaseURL:    "https://api.deepinfra.com/v1/openai",
		APIKeyEnv:  "DEEPINFRA_API_KEY",
		APIKeyName: "Deep Infra API key",
	},
	"inception": {
		Name:       "Inception",
		BaseURL:    "https://api.inceptionlabs.ai/v1",
		APIKeyEnv:  "INCEPTION_API_KEY",
		APIKeyName: "Inception API key",
	},
	"kilo": {
		Name:            "Kilo Gateway",
		BaseURL:         "https://api.kilo.ai/api/gateway",
		APIKeyEnv:       "KILO_API_KEY",
		APIKeyName:      "Kilo Gateway API key",
		OAuthName:       "Kilo Gateway",
		OAuthLoginLabel: "Sign in with Kilo",
	},
	"modular": {
		Name:       "Modular",
		BaseURL:    "https://api.modular.com/v1",
		APIKeyEnv:  "MODULAR_API_KEY",
		APIKeyName: "Modular API key",
	},
	"ollama-cloud": {
		Name:       "Ollama Cloud",
		BaseURL:    "https://ollama.com/v1",
		APIKeyEnv:  "OLLAMA_API_KEY",
		APIKeyName: "Ollama Cloud API key",
	},
	"sambanova": {
		Name:       "SambaNova",
		BaseURL:    "https://api.sambanova.ai/v1",
		APIKeyEnv:  "SAMBANOVA_API_KEY",
		APIKeyName: "SambaNova API key",
	},
	"scaleway": {
		Name:       "Scaleway",
		BaseURL:    "https://api.scaleway.ai/v1",
		APIKeyEnv:  "SCALEWAY_API_KEY",
		APIKeyName: "Scaleway API key",
	},
	"synthetic": {
		Name:       "Synthetic",
		BaseURL:    "https://api.synthetic.new/openai/v1",
		APIKeyEnv:  "SYNTHETIC_API_KEY",
		APIKeyName: "Synthetic API key",
	},
	"venice": {
		Name:       "Venice AI",
		BaseURL:    "https://api.venice.ai/api/v1",
		APIKeyEnv:  "VENICE_API_KEY",
		APIKeyName: "Venice AI API key",
	},
}

// addedProvider displays a provider this harness adds, or reports false.
func addedProvider(providerID string) (addedProviderInfo, bool) {
	info, ok := addedProviders[providerID]
	return info, ok
}

// addedProviderDisplayName is ProviderDisplayName for an added provider.
func addedProviderDisplayName(providerID string) (string, bool) {
	info, ok := addedProviders[providerID]
	if !ok {
		return "", false
	}
	return info.Name, true
}

// addedProviderBaseURL is ProviderBaseURL for an added provider.
func addedProviderBaseURL(providerID string) (string, bool) {
	info, ok := addedProviders[providerID]
	if !ok {
		return "", false
	}
	return info.BaseURL, true
}

// addedProviderAPIKeyEnv is getAPIKeyEnvVars for an added provider.
func addedProviderAPIKeyEnv(providerID string) (string, bool) {
	info, ok := addedProviders[providerID]
	if !ok {
		return "", false
	}
	return info.APIKeyEnv, true
}

// addedProviderOAuth is builtinOAuthNames and builtinOAuthLoginLabels for an
// added provider that advertises an OAuth method.
func addedProviderOAuth(providerID string) (name string, subscription bool, label string, ok bool) {
	info, found := addedProviders[providerID]
	if !found || info.OAuthName == "" {
		return "", false, "", false
	}
	return info.OAuthName, true, info.OAuthLoginLabel, true
}

// alibabaTokenPlanBaseURL is the international (Singapore) Token Plan endpoint,
// the default region of a QwenCloud Token Plan key.
const alibabaTokenPlanBaseURL = "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1"

// alibabaTokenPlanCNBaseURL is the China (Beijing) Token Plan endpoint. Keys are
// region-locked: a Beijing key is rejected by the international endpoint and the
// reverse (pi-catalog wire/alibaba-token-plan.ts).
const alibabaTokenPlanCNBaseURL = "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1"

// alibabaTokenPlanToken is the bearer-token grammar a Token Plan key matches
// (`sk-[A-Za-z0-9._~+/-]+={0,2}`).
func validAlibabaTokenPlanToken(token string) bool {
	rest, ok := strings.CutPrefix(token, "sk-")
	if !ok || rest == "" {
		return false
	}
	// Base64 padding may only trail, and at most twice.
	body := strings.TrimRight(rest, "=")
	if len(rest)-len(body) > 2 {
		return false
	}
	for _, c := range body {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '.', c == '_', c == '~', c == '+', c == '/', c == '-':
		default:
			return false
		}
	}
	return true
}

// alibabaTokenPlanCredential is a parsed QwenCloud Token Plan api key: the
// bearer token and the region endpoint it authenticates against, which is absent
// for the default international endpoint.
type alibabaTokenPlanCredential struct {
	Token   string
	Cookie  string
	BaseURL string
}

// parseAlibabaTokenPlanCredential reads a stored or ambient QwenCloud Token Plan
// key. Pi stores a key together with its optional quota cookie and a non-default
// region as JSON `{token, cookie?, baseUrl?}` and a plain key as itself; both
// forms are accepted here, and a value that matches neither is not a credential
// (parseAlibabaTokenPlanCredential, @oh-my-pi/pi-catalog wire/alibaba-token-plan).
func parseAlibabaTokenPlanCredential(value string) (alibabaTokenPlanCredential, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return alibabaTokenPlanCredential{}, false
	}
	if !strings.HasPrefix(trimmed, "{") {
		if !validAlibabaTokenPlanToken(trimmed) {
			return alibabaTokenPlanCredential{}, false
		}
		return alibabaTokenPlanCredential{Token: trimmed}, true
	}
	var stored struct {
		Token   string          `json:"token"`
		Cookie  json.RawMessage `json:"cookie"`
		BaseURL json.RawMessage `json:"baseUrl"`
	}
	if err := json.Unmarshal([]byte(trimmed), &stored); err != nil || !validAlibabaTokenPlanToken(strings.TrimSpace(stored.Token)) {
		return alibabaTokenPlanCredential{}, false
	}
	credential := alibabaTokenPlanCredential{Token: strings.TrimSpace(stored.Token)}
	// Omitted optional members are valid; an explicit null is not a string.
	if len(stored.Cookie) > 0 {
		if stored.Cookie[0] != '"' || json.Unmarshal(stored.Cookie, &credential.Cookie) != nil {
			return alibabaTokenPlanCredential{}, false
		}
		credential.Cookie = strings.TrimSpace(credential.Cookie)
	}
	if len(stored.BaseURL) > 0 {
		if stored.BaseURL[0] != '"' || json.Unmarshal(stored.BaseURL, &credential.BaseURL) != nil {
			return alibabaTokenPlanCredential{}, false
		}
		credential.BaseURL = strings.TrimSpace(credential.BaseURL)
	}
	return credential, true
}

// serializeAlibabaTokenPlanCredential is the stored form of a Token Plan key. A
// key with neither a cookie nor a non-default region stores as the bare token
// (serializeAlibabaTokenPlanCredential).
func serializeAlibabaTokenPlanCredential(token, cookie, baseURL string) string {
	token = strings.TrimSpace(token)
	cookie = strings.TrimSpace(cookie)
	baseURL = strings.TrimSpace(baseURL)
	if cookie == "" && baseURL == "" {
		return token
	}
	stored := struct {
		Token   string `json:"token"`
		Cookie  string `json:"cookie,omitempty"`
		BaseURL string `json:"baseUrl,omitempty"`
	}{Token: token, Cookie: cookie, BaseURL: baseURL}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return token
	}
	return string(encoded)
}

// alibabaTokenPlanAPIKeyAuth is the api-key method of alibaba-token-plan. It
// decodes the stored key at request time so the raw serialized credential never
// becomes bearer material: the token is the bearer and a stored region replaces
// the catalog endpoint. An unparsable value is a configuration error, as in
// upstream's request setup, instead of a request sent with a malformed key.
func alibabaTokenPlanAPIKeyAuth() *APIKeyAuth {
	return &APIKeyAuth{
		Name:  "Alibaba Token Plan API key",
		Login: envAPIKeyLogin("Alibaba Token Plan API key"),
		Resolve: func(ctx context.Context, input APIKeyAuthInput) (*AuthResult, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if input.Credential != nil && input.Credential.Key != "" {
				auth, err := alibabaTokenPlanModelAuth(input.Credential.Key)
				if err != nil {
					return nil, err
				}
				auth.Env = input.Credential.Env
				auth.Source = "stored credential"
				return auth, nil
			}
			if value := alibabaTokenPlanEnvValue(input.Ctx); value != "" {
				auth, err := alibabaTokenPlanModelAuth(value)
				if err != nil {
					return nil, err
				}
				auth.Source = addedProviders["alibaba-token-plan"].APIKeyEnv
				return auth, nil
			}
			return nil, nil
		},
	}
}

// alibabaTokenPlanEnvValue reads the Token Plan key from its environment
// variables, in lookup order.
func alibabaTokenPlanEnvValue(ctx AuthContext) string {
	for _, name := range getAPIKeyEnvVars("alibaba-token-plan") {
		if value, ok := ctx.Env(name); ok && value != "" {
			return value
		}
	}
	return ""
}

// alibabaTokenPlanModelAuth decodes one Token Plan key into request auth.
func alibabaTokenPlanModelAuth(value string) (*AuthResult, error) {
	credential, ok := parseAlibabaTokenPlanCredential(value)
	if !ok {
		return nil, NewModelsError(ModelsErrorAuth, "Invalid QwenCloud Token Plan credential", nil)
	}
	return &AuthResult{Auth: ModelAuth{APIKey: credential.Token, BaseURL: credential.BaseURL}}, nil
}

// alibabaCodingPlanAPIKeyAuth is the api-key method of alibaba-coding-plan. A
// stored or ambient key may be the bare key or a structured JSON credential
// `{token, enterpriseUrl}`: the token is the bearer and enterpriseUrl replaces
// the catalog endpoint, which is how a login for the China or a custom region is
// remembered (auth/alibaba-coding-plan's `api-key-format "structured"`). A value
// that is not such JSON is used as the key itself, as upstream's parse does.
func alibabaCodingPlanAPIKeyAuth() *APIKeyAuth {
	return &APIKeyAuth{
		Name:  "Alibaba Coding Plan API key",
		Login: envAPIKeyLogin("Alibaba Coding Plan API key"),
		Resolve: func(ctx context.Context, input APIKeyAuthInput) (*AuthResult, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if input.Credential != nil && input.Credential.Key != "" {
				auth := alibabaCodingPlanModelAuth(input.Credential.Key)
				auth.Env = input.Credential.Env
				auth.Source = "stored credential"
				return auth, nil
			}
			envVar := addedProviders["alibaba-coding-plan"].APIKeyEnv
			if value, ok := input.Ctx.Env(envVar); ok && value != "" {
				auth := alibabaCodingPlanModelAuth(value)
				auth.Source = envVar
				return auth, nil
			}
			return nil, nil
		},
	}
}

// alibabaCodingPlanModelAuth decodes one Alibaba Coding Plan key into request auth.
func alibabaCodingPlanModelAuth(value string) *AuthResult {
	auth := ModelAuth{APIKey: value}
	trimmed := strings.TrimSpace(value)
	if body, ok := strings.CutPrefix(trimmed, "{"); ok {
		var stored struct {
			Token         *string `json:"token"`
			EnterpriseURL *string `json:"enterpriseUrl"`
		}
		if json.Unmarshal([]byte("{"+body), &stored) == nil {
			auth.APIKey = value
			if stored.Token != nil {
				auth.APIKey = *stored.Token
			}
			if stored.EnterpriseURL != nil {
				auth.BaseURL = *stored.EnterpriseURL
			}
		}
	}
	return &AuthResult{Auth: auth}
}

// addedProviderAPIKeyAuths holds one API-key method per added provider, shared
// like the stock methods in builtinAPIKeyAuths: the selectors and the model
// registry compare the method they were handed, and constructing one per call
// would hand out a different object each time.
var addedProviderAPIKeyAuths = sync.OnceValue(func() map[string]*APIKeyAuth {
	auths := make(map[string]*APIKeyAuth, len(addedProviders))
	for providerID, info := range addedProviders {
		if providerID == "alibaba-token-plan" {
			// A stored Token Plan key is structured JSON; decode it per request.
			auths[providerID] = alibabaTokenPlanAPIKeyAuth()
			continue
		}
		if providerID == "alibaba-coding-plan" {
			// A stored Coding Plan key may remember its region alongside the token.
			auths[providerID] = alibabaCodingPlanAPIKeyAuth()
			continue
		}
		auths[providerID] = EnvAPIKeyAuth(info.APIKeyName, info.APIKeyEnv)
	}
	return auths
})

// addedProviderAPIKeyAuth is builtinAPIKeyAuth for an added provider, or
// reports false when the provider is not one this harness adds.
func addedProviderAPIKeyAuth(providerID string) (*APIKeyAuth, bool) {
	auth, ok := addedProviderAPIKeyAuths()[providerID]
	return auth, ok
}

// AddedProvider is the ModelsProvider of a provider this harness adds, assembled
// from its catalog exactly as a stock built-in provider is.
func AddedProvider(providerID string) *ModelsProvider {
	if _, ok := addedProvider(providerID); !ok {
		return nil
	}
	return builtinProvider(providerID)
}

// addedProviderConstructor is the builtinProviderConstructors entry of an added
// provider.
func addedProviderConstructor(providerID string) (func() *ModelsProvider, bool) {
	if _, ok := addedProvider(providerID); !ok {
		return nil, false
	}
	return func() *ModelsProvider { return AddedProvider(providerID) }, true
}

// Provider constructors, one per added provider, mirroring
// packages/ai/src/providers/<id>.ts. Each is the provider's own entry of
// providers/all.ts builtinModels.

// AiandProvider is the "aiand" provider.
func AiandProvider() *ModelsProvider { return AddedProvider("aiand") }

// AimlapiProvider is the "aimlapi" provider.
func AimlapiProvider() *ModelsProvider { return AddedProvider("aimlapi") }

// AlibabaCodingPlanProvider is the "alibaba-coding-plan" provider.
func AlibabaCodingPlanProvider() *ModelsProvider { return AddedProvider("alibaba-coding-plan") }

// AlibabaTokenPlanProvider is the "alibaba-token-plan" provider.
func AlibabaTokenPlanProvider() *ModelsProvider { return AddedProvider("alibaba-token-plan") }

// ClinePassProvider is the "cline-pass" provider.
func ClinePassProvider() *ModelsProvider { return AddedProvider("cline-pass") }

// CommandCodeProvider is the "commandcode" provider.
func CommandCodeProvider() *ModelsProvider { return AddedProvider("commandcode") }

// CoralBricksProvider is the "coralbricks" provider.
func CoralBricksProvider() *ModelsProvider { return AddedProvider("coralbricks") }

// DeepInfraProvider is the "deepinfra" provider.
func DeepInfraProvider() *ModelsProvider { return AddedProvider("deepinfra") }

// InceptionProvider is the "inception" provider.
func InceptionProvider() *ModelsProvider { return AddedProvider("inception") }

// KiloProvider is the "kilo" provider.
func KiloProvider() *ModelsProvider { return AddedProvider("kilo") }

// ModularProvider is the "modular" provider.
func ModularProvider() *ModelsProvider { return AddedProvider("modular") }

// OllamaCloudProvider is the "ollama-cloud" provider.
func OllamaCloudProvider() *ModelsProvider { return AddedProvider("ollama-cloud") }

// SambaNovaProvider is the "sambanova" provider.
func SambaNovaProvider() *ModelsProvider { return AddedProvider("sambanova") }

// ScalewayProvider is the "scaleway" provider.
func ScalewayProvider() *ModelsProvider { return AddedProvider("scaleway") }

// SyntheticProvider is the "synthetic" provider.
func SyntheticProvider() *ModelsProvider { return AddedProvider("synthetic") }

// VeniceProvider is the "venice" provider.
func VeniceProvider() *ModelsProvider { return AddedProvider("venice") }

// Chat catalog accessors, one per added provider, mirroring the
// <ID>_MODELS, <ID>_IMAGE_MODELS and <ID>_CLASSIFIER_MODELS exports of
// packages/ai/src/providers/<id>.models.ts. These providers carry chat models
// only, so their image and classifier catalogs are empty.

// AiandModels is AIAND_MODELS.
func AiandModels() ChatModelCatalog { return chatModelCatalog("aiand") }

// AimlapiModels is AIMLAPI_MODELS.
func AimlapiModels() ChatModelCatalog { return chatModelCatalog("aimlapi") }

// AlibabaCodingPlanModels is ALIBABA_CODING_PLAN_MODELS.
func AlibabaCodingPlanModels() ChatModelCatalog { return chatModelCatalog("alibaba-coding-plan") }

// AlibabaTokenPlanModels is ALIBABA_TOKEN_PLAN_MODELS.
func AlibabaTokenPlanModels() ChatModelCatalog { return chatModelCatalog("alibaba-token-plan") }

// ClinePassModels is CLINE_PASS_MODELS.
func ClinePassModels() ChatModelCatalog { return chatModelCatalog("cline-pass") }

// CommandCodeModels is COMMANDCODE_MODELS.
func CommandCodeModels() ChatModelCatalog { return chatModelCatalog("commandcode") }

// CoralBricksModels is CORALBRICKS_MODELS.
func CoralBricksModels() ChatModelCatalog { return chatModelCatalog("coralbricks") }

// DeepInfraModels is DEEPINFRA_MODELS.
func DeepInfraModels() ChatModelCatalog { return chatModelCatalog("deepinfra") }

// InceptionModels is INCEPTION_MODELS.
func InceptionModels() ChatModelCatalog { return chatModelCatalog("inception") }

// KiloModels is KILO_MODELS.
func KiloModels() ChatModelCatalog { return chatModelCatalog("kilo") }

// ModularModels is MODULAR_MODELS.
func ModularModels() ChatModelCatalog { return chatModelCatalog("modular") }

// OllamaCloudModels is OLLAMA_CLOUD_MODELS.
func OllamaCloudModels() ChatModelCatalog { return chatModelCatalog("ollama-cloud") }

// SambaNovaModels is SAMBANOVA_MODELS.
func SambaNovaModels() ChatModelCatalog { return chatModelCatalog("sambanova") }

// ScalewayModels is SCALEWAY_MODELS.
func ScalewayModels() ChatModelCatalog { return chatModelCatalog("scaleway") }

// SyntheticModels is SYNTHETIC_MODELS.
func SyntheticModels() ChatModelCatalog { return chatModelCatalog("synthetic") }

// VeniceModels is VENICE_MODELS.
func VeniceModels() ChatModelCatalog { return chatModelCatalog("venice") }
