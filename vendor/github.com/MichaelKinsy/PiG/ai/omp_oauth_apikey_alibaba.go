package ai

// Ports the two Alibaba plan logins (node_modules/@oh-my-pi/pi-ai/src/registry/oauth/
// alibaba-coding-plan.ts and alibaba-token-plan.ts).
//
// Both plans ship as regionally separate products with non-interchangeable keys, so each login first
// selects a region (or a custom base URL) and then collects the key that region issued, validating it
// against its own endpoint before storing it.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// Alibaba Coding Plan endpoints (alibaba-coding-plan.ts).
const (
	alibabaCodingPlanAuthURL         = "https://modelstudio.console.alibabacloud.com/"
	alibabaCodingPlanChinaAuthURL    = "https://bailian.console.aliyun.com/?tab=model#/api-key"
	alibabaCodingPlanBaseURL         = "https://coding-intl.dashscope.aliyuncs.com/v1"
	alibabaCodingPlanChinaBaseURL    = "https://coding.dashscope.aliyuncs.com/v1"
	alibabaCodingPlanValidationModel = "qwen3.5-plus"
)

// QwenCloud Token Plan auth pages (alibaba-token-plan.ts).
const (
	alibabaTokenPlanInternationalAuthURL = "https://home.qwencloud.com/billing/subscription/token-plan-individual"
	alibabaTokenPlanChinaAuthURL         = "https://www.aliyun.com/benefit/scene/tokenplan"
)

// alibabaCodingPlanProvider mints an Alibaba Coding Plan credential: the pasted key plus the endpoint
// it belongs to, kept as the credential's enterprise URL so a China or custom endpoint survives into
// requests.
type alibabaCodingPlanProvider struct {
	client *http.Client
}

func newAlibabaCodingPlanOAuthProvider() alibabaCodingPlanProvider {
	return alibabaCodingPlanProvider{client: &http.Client{}}
}

func (alibabaCodingPlanProvider) ID() string               { return "alibaba-coding-plan" }
func (alibabaCodingPlanProvider) Name() string             { return "Alibaba Coding Plan" }
func (alibabaCodingPlanProvider) IsSubscription() bool     { return true }
func (alibabaCodingPlanProvider) UsesCallbackServer() bool { return false }

// GetAPIKey is getOAuthApiKey for a provider whose auth declares `api-key-format "structured"`: the
// request key is the JSON credential, because consumers that only receive key bytes still need the
// endpoint the key belongs to. A credential without an endpoint keeps the bare key.
func (alibabaCodingPlanProvider) GetAPIKey(c OAuthCredentials) string {
	enterpriseURL := ompOAuthExtraString(c, "enterpriseUrl")
	if enterpriseURL == "" {
		return c.Access
	}
	key, err := json.Marshal(struct {
		Token         string `json:"token"`
		EnterpriseURL string `json:"enterpriseUrl"`
	}{Token: c.Access, EnterpriseURL: enterpriseURL})
	if err != nil {
		return c.Access
	}
	return string(key)
}

func (p alibabaCodingPlanProvider) RefreshToken(creds OAuthCredentials) (OAuthCredentials, error) {
	return ompUnchangedRefresh(creds)
}

// Login runs the flow without cancellation. Prefer LoginContext.
func (p alibabaCodingPlanProvider) Login(callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return p.LoginContext(context.Background(), callbacks)
}

// LoginContext asks for the endpoint, collects the key and validates it.
func (p alibabaCodingPlanProvider) LoginContext(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	if err := ompRequirePrompt(callbacks, "Alibaba Coding Plan"); err != nil {
		return OAuthCredentials{}, err
	}
	endpointChoice, err := ompPrompt(ctx, callbacks, "Alibaba Coding Plan",
		"Select Alibaba Coding Plan endpoint: 1=International (default), 2=China, 3=Custom — enter 1, 2, or 3", "1", false)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if err := ompCancelled(ctx); err != nil {
		return OAuthCredentials{}, err
	}

	var baseURL, authURL, instructions string
	choice := trimJSWhitespace(endpointChoice)
	switch choice {
	case "2":
		baseURL = alibabaCodingPlanChinaBaseURL
		authURL = alibabaCodingPlanChinaAuthURL
		instructions = "Copy your API key from the Alibaba Cloud Bailian console (China mainland)"
	case "3":
		customURL, err := ompPrompt(ctx, callbacks, "Alibaba Coding Plan", "Enter custom base URL", "https://your-proxy.com/v1", false)
		if err != nil {
			return OAuthCredentials{}, err
		}
		trimmed := strings.TrimRight(trimJSWhitespace(customURL), "/")
		if trimmed == "" {
			return OAuthCredentials{}, errors.New("Custom URL is required for option 3")
		}
		baseURL = trimmed
		authURL = alibabaCodingPlanAuthURL
		instructions = "Copy your API key from the Alibaba Cloud DashScope console"
	default:
		baseURL = alibabaCodingPlanBaseURL
		authURL = alibabaCodingPlanAuthURL
		instructions = "Copy your API key from the Alibaba Cloud DashScope console (International)"
	}
	ompNotifyAuth(callbacks, authURL, instructions)

	answer, err := ompPrompt(ctx, callbacks, "Alibaba Coding Plan", "Paste your Alibaba Coding Plan API key", "sk-...", false)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if err := ompCancelled(ctx); err != nil {
		return OAuthCredentials{}, err
	}
	apiKey := trimJSWhitespace(answer)
	if apiKey == "" {
		return OAuthCredentials{}, errors.New("API key is required")
	}

	ompNotifyProgress(callbacks, "Validating API key...")
	// A custom endpoint may front any model, so the probe asks its model list instead of naming one.
	if choice == "3" {
		modelsURL := strings.TrimRight(baseURL, "/") + "/models"
		if err := validateAPIKeyAgainstModelsEndpoint(ctx, p.client, "Alibaba Coding Plan", apiKey, modelsURL); err != nil {
			return OAuthCredentials{}, err
		}
	} else if err := validateOpenAICompatibleAPIKey(ctx, p.client, "Alibaba Coding Plan", apiKey, baseURL, alibabaCodingPlanValidationModel, "", 1); err != nil {
		return OAuthCredentials{}, err
	}
	return ompKeyCredentialWithExtra(apiKey, map[string]any{"enterpriseUrl": baseURL})
}

// alibabaTokenPlanProvider mints a QwenCloud Token Plan key: the region-scoped key with the optional
// quota-reporting cookie and, for a non-default region, the endpoint the key authenticates against.
// The composite is stored as the api-key string itself, which the provider's API-key method decodes.
type alibabaTokenPlanProvider struct {
	client *http.Client
}

func newAlibabaTokenPlanOAuthProvider() alibabaTokenPlanProvider {
	return alibabaTokenPlanProvider{client: &http.Client{}}
}

func (alibabaTokenPlanProvider) ID() string                          { return "alibaba-token-plan" }
func (alibabaTokenPlanProvider) Name() string                        { return "QwenCloud Token Plan" }
func (alibabaTokenPlanProvider) IsSubscription() bool                { return true }
func (alibabaTokenPlanProvider) UsesCallbackServer() bool            { return false }
func (alibabaTokenPlanProvider) GetAPIKey(c OAuthCredentials) string { return c.Access }

func (p alibabaTokenPlanProvider) RefreshToken(creds OAuthCredentials) (OAuthCredentials, error) {
	return ompUnchangedRefresh(creds)
}

// Login runs the flow without cancellation. Prefer LoginContext.
func (p alibabaTokenPlanProvider) Login(callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	return p.LoginContext(context.Background(), callbacks)
}

// LoginContext asks for the region, the key and the optional usage cookie, then serializes them.
func (p alibabaTokenPlanProvider) LoginContext(ctx context.Context, callbacks OAuthLoginCallbacks) (OAuthCredentials, error) {
	if err := ompRequirePrompt(callbacks, "QwenCloud Token Plan"); err != nil {
		return OAuthCredentials{}, err
	}
	endpointChoice, err := ompPrompt(ctx, callbacks, "QwenCloud Token Plan",
		"Select QwenCloud Token Plan region: 1=International (default), 2=China (Beijing), 3=Custom — enter 1, 2, or 3", "1", false)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if err := ompCancelled(ctx); err != nil {
		return OAuthCredentials{}, err
	}

	var baseURL, authURL, instructions string
	choice := trimJSWhitespace(endpointChoice)
	switch choice {
	case "2":
		baseURL = alibabaTokenPlanCNBaseURL
		authURL = alibabaTokenPlanChinaAuthURL
		instructions = "Subscribe to the China (Beijing) 百炼 Token Plan and copy its dedicated API key"
	case "3":
		customURL, err := ompPrompt(ctx, callbacks, "QwenCloud Token Plan", "Enter custom Token Plan base URL",
			"https://token-plan.<region>.maas.aliyuncs.com/compatible-mode/v1", false)
		if err != nil {
			return OAuthCredentials{}, err
		}
		trimmed := strings.TrimRight(trimJSWhitespace(customURL), "/")
		if trimmed == "" {
			return OAuthCredentials{}, errors.New("Custom URL is required for option 3")
		}
		baseURL = trimmed
		authURL = alibabaTokenPlanInternationalAuthURL
		instructions = "Copy your Token Plan API key for the custom endpoint"
	default:
		baseURL = alibabaTokenPlanBaseURL
		authURL = alibabaTokenPlanInternationalAuthURL
		instructions = "Subscribe to Token Plan Individual and copy its dedicated API key. Keep this page open; the next prompt explains how to enable optional quota reporting."
	}
	ompNotifyAuth(callbacks, authURL, instructions)

	answer, err := ompPrompt(ctx, callbacks, "QwenCloud Token Plan", "Paste your QwenCloud Token Plan API key", "sk-sp-...", false)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if err := ompCancelled(ctx); err != nil {
		return OAuthCredentials{}, err
	}
	apiKey := trimJSWhitespace(answer)
	if apiKey == "" {
		return OAuthCredentials{}, errors.New("API key is required")
	}

	ompNotifyProgress(callbacks, "Validating API key...")
	if err := validateAPIKeyAgainstModelsEndpoint(ctx, p.client, "QwenCloud Token Plan", apiKey, strings.TrimRight(baseURL, "/")+"/models"); err != nil {
		return OAuthCredentials{}, err
	}

	china := baseURL == alibabaTokenPlanCNBaseURL
	cookieHost := "cs-data.qwencloud.com"
	if china {
		cookieHost = "bailian-cs.console.aliyun.com"
	}
	cookiePrompt := "Optional quota reporting: open browser DevTools → Network, reload the Token Plan page, filter for api.json, and select the " + cookieHost +
		"/data/api.json request whose api query ends in /tokenplan/personal/api/v2/usage. Copy Request Headers → Cookie, then paste the complete name=value; ... value here, or press Enter to skip."
	rawCookie, err := ompPrompt(ctx, callbacks, "QwenCloud Token Plan", cookiePrompt, "name=value; name=value; ...", true)
	if err != nil {
		return OAuthCredentials{}, err
	}
	cookie := trimJSWhitespace(trimJSWhitespace(rawCookie))
	if len(cookie) >= len("Cookie:") && strings.EqualFold(cookie[:len("Cookie:")], "Cookie:") {
		cookie = trimJSWhitespace(cookie[len("Cookie:"):])
	}
	if err := ompCancelled(ctx); err != nil {
		return OAuthCredentials{}, err
	}
	if cookie != "" && !validCookieHeader(cookie) {
		return OAuthCredentials{}, errors.New("Invalid QwenCloud Cookie header. Copy the complete Cookie request header from the " + cookieHost + " usage request, not a single cookie value.")
	}

	// The default international login keeps its existing bare/cookie form; only a diverging region is
	// persisted, so it can override the catalog base URL at inference and discovery time.
	regionURL := ""
	if baseURL != alibabaTokenPlanBaseURL {
		regionURL = baseURL
	}
	return ompKeyCredential(serializeAlibabaTokenPlanCredential(apiKey, cookie, regionURL)), nil
}

// validCookieHeader reports whether a Cookie header carries at least one complete name=value pair
// with a non-blank name and value.
func validCookieHeader(cookie string) bool {
	for segment := range strings.SplitSeq(cookie, ";") {
		if separator := strings.IndexByte(segment, '='); separator > 0 {
			if trimJSWhitespace(segment[:separator]) != "" && trimJSWhitespace(segment[separator+1:]) != "" {
				return true
			}
		}
	}
	return false
}
