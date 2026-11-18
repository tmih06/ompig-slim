package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
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

type codexModelCatalog struct {
	providerID string
	mu         sync.RWMutex
	rosters    map[string]codexModelRoster
}

type codexModelRoster struct {
	endpoint string
	models   []*Model
}

// WithCodexModelDiscovery adds account-scoped live roster refresh to a Codex provider.
// Its existing transport and static catalog remain the trusted baseline.
func WithCodexModelDiscovery(provider *ModelsProvider) *ModelsProvider {
	if provider == nil || !isCodexAccountProvider(provider.ID) {
		return provider
	}
	baseGetModels := provider.GetModels
	baseGetAllModels := provider.GetAllModels
	baseFilter := provider.FilterModels
	baseFilterAll := provider.FilterAllModels
	catalog := &codexModelCatalog{providerID: provider.ID, rosters: map[string]codexModelRoster{}}
	provider.ModelCatalog = catalog
	provider.GetModels = func() ([]*Model, error) {
		rosters := catalog.Snapshots(provider.BaseURL)
		if len(rosters) == 0 {
			if baseGetModels == nil {
				return nil, nil
			}
			return baseGetModels()
		}
		return codexRosterUnion(rosters), nil
	}
	provider.GetAllModels = func() ([]AnyModel, error) {
		var all []AnyModel
		if baseGetAllModels != nil {
			var err error
			all, err = baseGetAllModels()
			if err != nil {
				return nil, err
			}
		} else if baseGetModels != nil {
			chat, err := baseGetModels()
			if err != nil {
				return nil, err
			}
			all = AnyModels(chat)
		}
		rosters := catalog.Snapshots(provider.BaseURL)
		if len(rosters) == 0 {
			return all, nil
		}
		all = slices.DeleteFunc(all, func(model AnyModel) bool { return IsModelType(model, ModelTypeChat) })
		return append(all, AnyModels(codexRosterUnion(rosters))...), nil
	}
	provider.FilterModels = func(models []*Model, credential *Credential) []*Model {
		scope := catalog.Scope(provider.BaseURL, credential)
		for _, roster := range catalog.Snapshots(provider.BaseURL) {
			if roster.ScopeID == scope.ID {
				if baseFilter != nil {
					return baseFilter(roster.Models, credential)
				}
				return roster.Models
			}
		}
		if baseGetModels != nil {
			if baseline, err := baseGetModels(); err == nil {
				if baseFilter != nil {
					return baseFilter(baseline, credential)
				}
				return baseline
			}
		}
		if baseFilter != nil {
			return baseFilter(models, credential)
		}
		return models
	}
	provider.FilterAllModels = func(models []AnyModel, credential *Credential) []AnyModel {
		if baseFilterAll != nil {
			models = baseFilterAll(models, credential)
		}
		chat, err := provider.GetModels()
		if err != nil {
			chat = nil
		}
		if provider.FilterModels != nil {
			chat = provider.FilterModels(chat, credential)
		}
		filtered := make([]AnyModel, 0, len(models)+len(chat))
		for _, model := range models {
			if !IsModelType(model, ModelTypeChat) {
				filtered = append(filtered, model)
			}
		}
		return append(filtered, AnyModels(chat)...)
	}
	provider.RefreshModels = func(refresh RefreshModelsContext) error {
		signal := refresh.Signal
		if signal == nil {
			signal = context.Background()
		}
		scope := catalog.Scope(refresh.BaseURL, refresh.Credential)
		if scope.ID == "" {
			return nil
		}
		publishRoster := func(models []*Model, persist *ModelsStoreEntry) error {
			update := func() { catalog.store(refresh.BaseURL, scope.ID, models) }
			if refresh.Publish == nil {
				update()
				return nil
			}
			_, err := refresh.Publish(ModelsPublication{Persist: persist, Update: update})
			return err
		}
		if refresh.Stored != nil {
			models, err := decodeModelsCatalog(refresh.Stored.Models, provider.ID)
			if err != nil {
				return err
			}
			if err := publishRoster(chatModels(models), nil); err != nil {
				return err
			}
		}
		if !refresh.AllowNetwork || signal.Err() != nil || refresh.Credential.Type != CredentialOAuth {
			return nil
		}
		models, etag, err := fetchCodexModelRoster(signal, provider.ID, refresh.BaseURL, refresh.Credential)
		if err != nil {
			return err
		}
		if signal.Err() != nil {
			return context.Cause(signal)
		}
		entry := &ModelsStoreEntry{Models: AnyModels(models), CheckedAt: new(float64(time.Now().UnixMilli())), ETag: etag}
		return publishRoster(models, entry)
	}
	return provider
}

func (c *codexModelCatalog) Scope(baseURL string, credential *Credential) ModelsCatalogScope {
	if credential == nil || credential.Type != CredentialOAuth {
		return ModelsCatalogScope{}
	}
	endpoint, err := codexModelsEndpoint(baseURL)
	if err != nil {
		return ModelsCatalogScope{}
	}
	identity, stable := codexModelIdentity(*credential)
	if !stable {
		if credential.Access == "" {
			return ModelsCatalogScope{}
		}
		identity, _ = json.Marshal([]string{"ephemeral-access-token", credential.Access})
	}
	endpointID := codexModelsDigest(c.providerID, endpoint)
	id := c.providerID + ":codex-live:" + codexModelsDigest(c.providerID, endpointID+"\x00"+string(identity))
	scope := ModelsCatalogScope{ID: id}
	if stable {
		scope.StoreKey = id
	}
	return scope
}

func codexModelIdentity(credential Credential) ([]byte, bool) {
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
		return nil, false
	}
	encoded, _ := json.Marshal(identity)
	return encoded, true
}

func (c *codexModelCatalog) Snapshots(baseURL string) []ModelsCatalogSnapshot {
	endpoint, err := codexModelsEndpoint(baseURL)
	if err != nil {
		return nil
	}
	endpointID := codexModelsDigest(c.providerID, endpoint)
	c.mu.RLock()
	var snapshots []ModelsCatalogSnapshot
	for scopeID, roster := range c.rosters {
		if roster.endpoint == endpointID {
			snapshots = append(snapshots, ModelsCatalogSnapshot{ScopeID: scopeID, Models: cloneCodexModels(roster.models)})
		}
	}
	c.mu.RUnlock()
	slices.SortFunc(snapshots, func(a, b ModelsCatalogSnapshot) int { return strings.Compare(a.ScopeID, b.ScopeID) })
	return snapshots
}

func (c *codexModelCatalog) RetainScopes(baseURL string, scopeIDs []string) {
	endpoint, err := codexModelsEndpoint(baseURL)
	if err != nil {
		return
	}
	endpointID := codexModelsDigest(c.providerID, endpoint)
	keep := make(map[string]struct{}, len(scopeIDs))
	for _, scopeID := range scopeIDs {
		keep[scopeID] = struct{}{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for scopeID, roster := range c.rosters {
		if roster.endpoint == endpointID {
			if _, ok := keep[scopeID]; !ok {
				delete(c.rosters, scopeID)
			}
		}
	}
}

func (c *codexModelCatalog) ModelAvailability(baseURL string, credential *Credential, modelID string) (known, available bool) {
	scope := c.Scope(baseURL, credential)
	if scope.ID == "" {
		return false, true
	}
	endpoint, err := codexModelsEndpoint(baseURL)
	if err != nil {
		return false, true
	}
	endpointID := codexModelsDigest(c.providerID, endpoint)
	c.mu.RLock()
	if roster, ok := c.rosters[scope.ID]; ok && roster.endpoint == endpointID {
		available := containsCodexModel(roster.models, modelID)
		c.mu.RUnlock()
		return true, available
	}
	discovered := false
	for _, roster := range c.rosters {
		if roster.endpoint == endpointID && containsCodexModel(roster.models, modelID) {
			discovered = true
			break
		}
	}
	c.mu.RUnlock()
	if discovered && !c.builtinModel(modelID) {
		return true, false
	}
	return false, true
}

func (c *codexModelCatalog) builtinModel(modelID string) bool {
	return GetBuiltinModel(c.providerID, modelID) != nil || c.providerID == codexJSONProviderID && GetBuiltinModel("openai-codex", modelID) != nil
}

func (c *codexModelCatalog) store(baseURL, scopeID string, models []*Model) {
	endpoint, err := codexModelsEndpoint(baseURL)
	if err != nil || scopeID == "" {
		return
	}
	endpointID := codexModelsDigest(c.providerID, endpoint)
	c.mu.Lock()
	c.rosters[scopeID] = codexModelRoster{endpoint: endpointID, models: cloneCodexModels(models)}
	c.mu.Unlock()
}

func cloneCodexModels(models []*Model) []*Model {
	copied := ModelsStoreEntry{Models: AnyModels(models)}.Clone()
	return chatModels(copied.Models)
}

func codexRosterUnion(rosters []ModelsCatalogSnapshot) []*Model {
	models := []*Model{}
	seen := map[string]struct{}{}
	for _, roster := range rosters {
		for _, model := range roster.Models {
			if _, exists := seen[model.ID]; exists {
				continue
			}
			seen[model.ID] = struct{}{}
			models = append(models, model)
		}
	}
	return models
}

func containsCodexModel(models []*Model, modelID string) bool {
	return slices.ContainsFunc(models, func(model *Model) bool { return model.ID == modelID })
}

func codexModelsDigest(providerID, identity string) string {
	digest := sha256.Sum256([]byte(providerID + "\x00" + identity))
	return hex.EncodeToString(digest[:])
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
