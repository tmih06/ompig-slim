package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/MichaelKinsy/PiG/internal/coding/pigidentity"
)

const (
	codexModelsRequestTimeout = 15 * time.Second
	codexModelsResponseLimit  = 4 << 20
	// The Codex endpoint gates roster entries by client version. PiG's own
	// release version is unrelated and 0.7.0 receives an empty roster.
	codexModelsClientVersion = "1.0.0"
)

type codexModelsResponse struct {
	Models json.RawMessage `json:"models"`
}

type codexReasoningLevel struct {
	Effort string `json:"effort"`
}

type codexModelInfo struct {
	Slug                     string                 `json:"slug"`
	DisplayName              string                 `json:"display_name"`
	Description              string                 `json:"description"`
	Visibility               string                 `json:"visibility"`
	SupportedInAPI           bool                   `json:"supported_in_api"`
	InputModalities          json.RawMessage        `json:"input_modalities"`
	ContextWindow            int                    `json:"context_window"`
	MaxContextWindow         int                    `json:"max_context_window"`
	SupportedReasoningLevels *[]codexReasoningLevel `json:"supported_reasoning_levels"`
}

// WithCodexModelDiscovery adds account-scoped live roster refresh to a Codex provider.
// Its existing transport and static catalog remain the trusted baseline.
func WithCodexModelDiscovery(provider *ModelsProvider) *ModelsProvider {
	if provider == nil || !isCodexAccountProvider(provider.ID) {
		return provider
	}
	providerID := provider.ID
	catalog := &accountModelCatalog{
		providerID: providerID,
		scopeLabel: "codex-live",
		endpoint:   codexModelsEndpoint,
		identity:   codexModelIdentity,
		builtin: func(modelID string) bool {
			return GetBuiltinModel(providerID, modelID) != nil || providerID == codexJSONProviderID && GetBuiltinModel("openai-codex", modelID) != nil
		},
	}
	return withAccountModelDiscovery(provider, catalog, func(ctx context.Context, baseURL string, credential *Credential) ([]*Model, string, error) {
		return fetchCodexModelRoster(ctx, providerID, baseURL, credential)
	})
}

func codexModelIdentity(credential Credential) (string, bool) {
	identity := []string{}
	add := func(name, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			identity = append(identity, name, value)
		}
	}
	add("account", credential.AccountID)
	add("project", credential.ProjectID)
	add("enterprise", credential.EnterpriseDomain)
	claims := codexJWTProfile(credential.Access)
	add("token-account", codexClaimString(claims, codexJWTClaimPath, "chatgpt_account_id"))
	add("email", CodexAccountEmail(credential.Access))
	add("subject", func() string { value, _ := claims["sub"].(string); return value }())
	if plan, known := codexAccountPlan(credential); known {
		add("plan", plan)
	}
	for _, key := range []string{"email", "userId", "user_id", "chatgpt_user_id", "chatgptUserId", "orgId", "org_id", "orgName"} {
		var value string
		if raw := credential.Extra[key]; len(raw) != 0 {
			_ = json.Unmarshal(raw, &value)
		}
		add(key, value)
	}
	if len(identity) == 0 {
		return "", false
	}
	encoded, _ := json.Marshal(identity)
	return string(encoded), true
}

func fetchCodexModelRoster(ctx context.Context, providerID, baseURL string, credential *Credential) ([]*Model, string, error) {
	if credential == nil || credential.Access == "" {
		return nil, "", errors.New("Codex model discovery requires OAuth credentials")
	}
	endpoint, err := codexModelsEndpoint(baseURL)
	if err != nil {
		return nil, "", errors.New("Codex model discovery endpoint is invalid")
	}
	requestCtx, cancel := context.WithTimeout(ctx, codexModelsRequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", errors.New("Codex model discovery request could not be created")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+credential.Access)
	request.Header.Set("originator", pigidentity.CodexOriginator)
	if credential.AccountID != "" {
		request.Header.Set("ChatGPT-Account-Id", credential.AccountID)
	}
	client := *http.DefaultClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", context.Cause(ctx)
		}
		return nil, "", errors.New("Codex model discovery request failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, "", errors.New("Codex model discovery returned HTTP " + strconv.Itoa(response.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, codexModelsResponseLimit+1))
	if err != nil || len(body) > codexModelsResponseLimit {
		return nil, "", errors.New("Codex model discovery response could not be read")
	}
	models, err := parseCodexModelRoster(providerID, baseURL, body)
	if err != nil {
		return nil, "", err
	}
	return models, response.Header.Get("ETag"), nil
}

func codexModelsEndpoint(baseURL string) (string, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultCodexBaseURL
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil || (endpoint.Scheme != "https" && endpoint.Scheme != "http") || endpoint.Host == "" || endpoint.User != nil {
		return "", errors.New("invalid Codex base URL")
	}
	path := strings.TrimRight(endpoint.Path, "/")
	switch {
	case strings.HasSuffix(path, "/codex/models"):
	case strings.HasSuffix(path, "/codex"):
		path += "/models"
	default:
		path += "/codex/models"
	}
	endpoint.Path = path
	endpoint.RawPath = ""
	endpoint.Fragment = ""
	query := endpoint.Query()
	query.Set("client_version", codexModelsClientVersion)
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func parseCodexModelRoster(providerID, baseURL string, body []byte) ([]*Model, error) {
	var response codexModelsResponse
	if json.Unmarshal(body, &response) != nil || len(response.Models) == 0 || response.Models[0] != '[' {
		return nil, errors.New("Codex model discovery response is invalid")
	}
	var rows []codexModelInfo
	if json.Unmarshal(response.Models, &rows) != nil {
		return nil, errors.New("Codex model discovery response is invalid")
	}
	seen := make(map[string]struct{}, len(rows))
	models := make([]*Model, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Slug) == "" || row.Slug != strings.TrimSpace(row.Slug) || strings.IndexFunc(row.Slug, unicode.IsControl) >= 0 {
			return nil, errors.New("Codex model discovery response contains an invalid model ID")
		}
		if _, duplicate := seen[row.Slug]; duplicate {
			return nil, errors.New("Codex model discovery response contains duplicate model IDs")
		}
		seen[row.Slug] = struct{}{}
		modalities, err := codexInputModalities(row.InputModalities)
		if err != nil || row.ContextWindow < 0 || row.MaxContextWindow < 0 {
			return nil, errors.New("Codex model discovery response is invalid")
		}
		switch row.Visibility {
		case "list", "hide", "none":
		default:
			return nil, errors.New("Codex model discovery response is invalid")
		}
		if row.Visibility != "list" || !slices.Contains(modalities, ModelInputText) {
			continue
		}
		model := codexModelFromInfo(providerID, baseURL, row, modalities)
		models = append(models, model)
	}
	return models, nil
}

func codexInputModalities(raw json.RawMessage) ([]string, error) {
	if raw == nil {
		return []string{ModelInputText, ModelInputImage}, nil
	}
	var modalities []string
	if err := json.Unmarshal(raw, &modalities); err != nil || modalities == nil {
		return nil, errors.New("invalid input modalities")
	}
	for _, modality := range modalities {
		if modality != ModelInputText && modality != ModelInputImage && modality != "audio" {
			return nil, errors.New("invalid input modality")
		}
	}
	return modalities, nil
}

func codexModelFromInfo(providerID, baseURL string, row codexModelInfo, modalities []string) *Model {
	var model *Model
	if builtin := GetBuiltinModel(providerID, row.Slug); builtin != nil {
		model = builtin
	} else if providerID == codexJSONProviderID {
		model = GetBuiltinModel("openai-codex", row.Slug)
	}
	if model == nil {
		model = &Model{Type: ModelTypeChat, ID: row.Slug, Capabilities: ModelCapabilities{SupportsToolUse: true}}
	}
	model.Type = ModelTypeChat
	model.ID = row.Slug
	if displayName := strings.TrimSpace(row.DisplayName); displayName != "" {
		model.DisplayName = displayName
	} else if model.DisplayName == "" {
		model.DisplayName = row.Slug
	}
	model.ProviderMeta.ProviderID = providerID
	model.ProviderMeta.API = APIOpenAICodexResponses
	model.ProviderMeta.BaseURL = baseURL
	model.Capabilities.SupportsToolUse = true
	model.Capabilities.SupportsImages = slices.Contains(modalities, ModelInputImage)
	model.Input = []string{ModelInputText}
	if model.Capabilities.SupportsImages {
		model.Input = append(model.Input, ModelInputImage)
	}
	contextWindow := row.ContextWindow
	if contextWindow == 0 {
		contextWindow = row.MaxContextWindow
	}
	if contextWindow > 0 {
		model.Capabilities.ContextWindow = contextWindow
	}
	if row.Description != "" {
		model.Description = row.Description
	}
	if row.SupportedReasoningLevels != nil {
		model.ThinkingLevelMap, model.ProviderMeta.Reasoning = codexThinkingLevelMap(*row.SupportedReasoningLevels)
		model.Capabilities.MaxThinking = thinkingMaxLevel(model.ProviderMeta.Reasoning, model.ThinkingLevelMap)
	}
	return model
}

func codexThinkingLevelMap(supported []codexReasoningLevel) (ThinkingLevelMap, bool) {
	levels := ThinkingLevelMap{
		ThinkingOff: nil, ThinkingMinimal: nil, ThinkingLow: nil, ThinkingMedium: nil,
		ThinkingHigh: nil, ThinkingXHigh: nil, ThinkingMax: nil,
	}
	standard := map[string]ModelThinkingLevel{
		"minimal": ThinkingMinimal, "low": ThinkingLow, "medium": ThinkingMedium,
		"high": ThinkingHigh, "xhigh": ThinkingXHigh, "max": ThinkingMax,
	}
	reasoning := false
	for _, item := range supported {
		effort := strings.ToLower(strings.TrimSpace(item.Effort))
		if effort == "none" {
			levels[ThinkingOff] = new(effort)
			continue
		}
		if level, ok := standard[effort]; ok {
			levels[level] = new(effort)
			reasoning = true
		}
	}
	if !reasoning {
		return nil, false
	}
	return levels, true
}
