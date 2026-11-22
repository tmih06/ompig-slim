package ai

// Registration of the Google Cloud Code Assist providers: google-antigravity and google-gemini-cli, their
// model rosters, auth methods, OAuth flows, API implementation and account-scoped model discovery.
//
// Ports packages/ai/src/providers/google-cloud-code.ts, the catalog barrel entries of
// providers/{google-antigravity,google-gemini-cli}.models.ts and the auth wiring of the two providers.
//
// pig additive: the shared tables (provider ids, names, base URLs, env keys, API list, OAuth names and the
// catalog slice) are extended from this file's init rather than by editing the generated catalog or the
// shared registration files, so the transport family owns one registration surface and a sibling feature
// cannot conflict with it.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

// ─── Auth methods ────────────────────────────────────────────────────────────

// googleCloudCodeAPIKeyAuth is envApiKeyAuth(name, []) for a provisioned credential: only a stored
// credential resolves, since Cloud Code has no API-key mode.
func googleCloudCodeAPIKeyAuth(name string) *APIKeyAuth {
	return EnvAPIKeyAuth(name)
}

// googleCloudCodeOAuthAuth is the lazy OAuth method of one Google provider.
func googleCloudCodeOAuthAuth(flow googleOAuthFlow) *OAuthAuth {
	return LazyOAuth(LazyOAuthInput{
		Name:           flow.displayName,
		IsSubscription: true,
		LoginLabel:     "Sign in with Google",
		Load: func() (*OAuthAuth, error) {
			provider := googleCloudCodeOAuthProvider{flow: flow}
			return &OAuthAuth{
				Name:           flow.displayName,
				IsSubscription: true,
				LoginLabel:     "Sign in with Google",
				Login:          oauthNativeLogin(provider),
				Refresh:        oauthRefresh(provider),
				ToAuth: func(credential Credential) (ModelAuth, error) {
					return ModelAuth{APIKey: googleCloudCodeCredentialAPIKey(credential)}, nil
				},
			}, nil
		},
	})
}

// ─── Model rosters ───────────────────────────────────────────────────────────

// googleCloudCodeModelSpec is one baseline row of a provider's roster.
type googleCloudCodeModelSpec struct {
	ID            string
	Name          string
	Provider      string
	BaseURL       string
	Reasoning     bool
	Input         []ModelInputModality
	ContextWindow int
	MaxTokens     int
	// ThinkingMax is the model's highest logical thinking level; empty means the default.
	ThinkingMax string
}

// googleAntigravityBaseline is the set Antigravity always serves. Plan-specific rows come from the
// account roster fetched by RefreshModels, not from this default.
var googleAntigravityBaseline = []googleCloudCodeModelSpec{
	{ID: "gemini-3.8-flash-high", Name: "Gemini 3.8 Flash High", Reasoning: true, Input: []ModelInputModality{ModelInputText, ModelInputImage}},
	{ID: "gemini-3.8-flash-medium", Name: "Gemini 3.8 Flash Medium", Reasoning: true, Input: []ModelInputModality{ModelInputText, ModelInputImage}},
	{ID: "gemini-3.8-flash-low", Name: "Gemini 3.8 Flash Low", Reasoning: true, Input: []ModelInputModality{ModelInputText, ModelInputImage}},
	{ID: "gemini-3.8-flash-tiered", Name: "Gemini 3.8 Flash Tiered", Reasoning: true, Input: []ModelInputModality{ModelInputText, ModelInputImage}},
	{ID: "gemini-3.1-pro", Name: "Gemini 3.1 Pro", Reasoning: true, Input: []ModelInputModality{ModelInputText, ModelInputImage}},
	{ID: "gemini-3-flash", Name: "Gemini 3 Flash", Reasoning: true, Input: []ModelInputModality{ModelInputText, ModelInputImage}},
	{ID: "gemini-3.1-flash-lite", Name: "Gemini 3.1 Flash Lite", Reasoning: true, Input: []ModelInputModality{ModelInputText}},
	{ID: "gemini-3.1-flash-image", Name: "Gemini 3.1 Flash Image", Reasoning: false, Input: []ModelInputModality{ModelInputText, ModelInputImage}},
	{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Reasoning: true, Input: []ModelInputModality{ModelInputText, ModelInputImage}},
	{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Reasoning: true, Input: []ModelInputModality{ModelInputText, ModelInputImage}},
	{ID: "gemini-2.5-flash-thinking", Name: "Gemini 2.5 Flash Thinking", Reasoning: true, Input: []ModelInputModality{ModelInputText, ModelInputImage}},
	{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Reasoning: true, Input: []ModelInputModality{ModelInputText}, ThinkingMax: "max"},
	{ID: "claude-opus-4-6-thinking", Name: "Claude Opus 4.6 Thinking", Reasoning: true, Input: []ModelInputModality{ModelInputText}, ThinkingMax: "max"},
	{ID: "gpt-oss-120b-medium", Name: "GPT-OSS 120B Medium", Reasoning: true, Input: []ModelInputModality{ModelInputText}},
}

// googleGeminiCliBaseline is the Gemini CLI's own roster: Google's models through Cloud Code Assist.
var googleGeminiCliBaseline = []googleCloudCodeModelSpec{
	{ID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro (Preview)", Reasoning: true, Input: []ModelInputModality{ModelInputText, ModelInputImage}, ContextWindow: 1_000_000, MaxTokens: 64_000},
}

// googleCloudCodeImageLimits is the shared resize contract of the Cloud Code Assist image inputs.
func googleCloudCodeImageLimits() *ModelInputLimits {
	return &ModelInputLimits{Images: &ModelImageInputLimits{Resize: &ModelImageResizeOptions{MaxWidth: 2000, MaxHeight: 2000, MaxBytes: 4718592, JPEGQuality: 80}}}
}

// googleCloudCodeRuntimeModel builds the runtime model of one baseline row.
func googleCloudCodeRuntimeModel(spec googleCloudCodeModelSpec) *Model {
	contextWindow := spec.ContextWindow
	if contextWindow == 0 {
		contextWindow = 200_000
	}
	maxTokens := spec.MaxTokens
	if maxTokens == 0 {
		maxTokens = 64_000
	}
	model := &Model{
		Type:        ModelTypeChat,
		ID:          spec.ID,
		DisplayName: spec.Name,
		Input:       slices.Clone(spec.Input),
		ProviderMeta: ProviderMetadata{
			ProviderID: spec.Provider,
			API:        APIGoogleGeminiCLI,
			BaseURL:    spec.BaseURL,
			Reasoning:  spec.Reasoning,
		},
	}
	if spec.ThinkingMax != "" {
		level := spec.ThinkingMax
		model.ThinkingLevelMap = ThinkingLevelMap{ModelThinkingLevel(ThinkingLevelMax): &level}
	}
	if slices.Contains(spec.Input, ModelInputImage) {
		model.InputLimits = googleCloudCodeImageLimits()
	}
	model.Capabilities = ModelCapabilities{
		ContextWindow:   contextWindow,
		MaxOutputTokens: maxTokens,
		SupportsImages:  slices.Contains(spec.Input, ModelInputImage),
		SupportsToolUse: true,
		InputCostPer1M:  0,
		OutputCostPer1M: 0,
	}
	if spec.Reasoning {
		model.Capabilities.MaxThinking = ThinkingLevelHigh
	}
	return model
}

// googleCloudCodeGeneratedRows builds the catalog entries of one provider's baseline roster, so listings,
// --list-models, /model and RPC see the same rows the provider serves.
func googleCloudCodeGeneratedRows(specs []googleCloudCodeModelSpec) []GeneratedModel {
	rows := make([]GeneratedModel, 0, len(specs))
	for _, spec := range specs {
		contextWindow := spec.ContextWindow
		if contextWindow == 0 {
			contextWindow = 200_000
		}
		maxTokens := spec.MaxTokens
		if maxTokens == 0 {
			maxTokens = 64_000
		}
		capabilities := make([]string, len(spec.Input))
		for i, modality := range spec.Input {
			capabilities[i] = string(modality)
		}
		row := GeneratedModel{
			ID:              spec.ID,
			Provider:        spec.Provider,
			DisplayName:     spec.Name,
			API:             APIGoogleGeminiCLI,
			BaseURL:         spec.BaseURL,
			ContextWindow:   contextWindow,
			MaxOutputTokens: maxTokens,
			Reasoning:       spec.Reasoning,
			Capabilities:    capabilities,
		}
		if spec.ThinkingMax != "" {
			level := spec.ThinkingMax
			row.ThinkingLevelMap = map[ModelThinkingLevel]*string{ModelThinkingLevel(ThinkingLevelMax): &level}
		}
		if slices.Contains(spec.Input, ModelInputImage) {
			row.InputLimits = googleCloudCodeImageLimits()
		}
		if record, err := json.Marshal(googleCloudCodeCatalogRecord(spec, contextWindow, maxTokens)); err == nil {
			row.Record = string(record)
		}
		rows = append(rows, row)
	}
	return rows
}

// googleCloudCodeCatalogRecord is the entry's JSON as Pi's catalog data spells it.
func googleCloudCodeCatalogRecord(spec googleCloudCodeModelSpec, contextWindow, maxTokens int) map[string]any {
	input := make([]string, len(spec.Input))
	for i, modality := range spec.Input {
		input[i] = string(modality)
	}
	record := map[string]any{
		"id": spec.ID, "name": spec.Name, "api": string(APIGoogleGeminiCLI), "provider": spec.Provider,
		"baseUrl": spec.BaseURL, "reasoning": spec.Reasoning, "input": input,
		"cost":          map[string]any{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0},
		"contextWindow": contextWindow, "maxTokens": maxTokens, "type": "chat",
	}
	if spec.ThinkingMax != "" {
		record["thinkingLevelMap"] = map[string]any{"max": spec.ThinkingMax}
	}
	if slices.Contains(spec.Input, ModelInputImage) {
		record["inputLimits"] = map[string]any{"images": map[string]any{"resize": map[string]any{
			"maxWidth": 2000, "maxHeight": 2000, "maxBytes": 4718592, "jpegQuality": 80,
		}}}
	}
	return record
}

func googleAntigravityRuntimeModels() []googleCloudCodeModelSpec {
	specs := slices.Clone(googleAntigravityBaseline)
	for i := range specs {
		specs[i].Provider = GoogleAntigravityProviderID
		specs[i].BaseURL = googleAntigravityPrimaryEndpoint
	}
	return specs
}

func googleGeminiCliRuntimeModels() []googleCloudCodeModelSpec {
	specs := slices.Clone(googleGeminiCliBaseline)
	for i := range specs {
		specs[i].Provider = GoogleGeminiCliProviderID
		specs[i].BaseURL = googleCloudCodeAssistEndpoint
	}
	return specs
}

// ─── Providers ───────────────────────────────────────────────────────────────

// GoogleAntigravityProvider is the google-antigravity provider: Cloud Code Assist served from the daily host
// with the sandbox host as failover, and an account-scoped model roster.
func GoogleAntigravityProvider() *ModelsProvider {
	flow := googleAntigravityFlow
	specs := googleAntigravityRuntimeModels()
	baseline := make([]AnyModel, 0, len(specs))
	for _, spec := range specs {
		baseline = append(baseline, googleCloudCodeRuntimeModel(spec))
	}
	return WithGoogleAntigravityModelDiscovery(CreateProvider(CreateProviderOptions{
		ID:      GoogleAntigravityProviderID,
		Name:    ptrString("Google Antigravity"),
		BaseURL: googleAntigravityPrimaryEndpoint,
		Auth: ProviderAuth{
			APIKey: googleCloudCodeAPIKeyAuth("Google Antigravity credential"),
			OAuth:  googleCloudCodeOAuthAuth(flow),
		},
		Models: baseline,
		API:    GoogleCloudCodeAssistAPI(),
	}))
}

// WithGoogleAntigravityModelDiscovery installs the account's live Cloud Code
// Assist roster on the google-antigravity provider, scoped by credential
// identity and endpoint, with the bundled catalog as baseline.
func WithGoogleAntigravityModelDiscovery(provider *ModelsProvider) *ModelsProvider {
	if provider == nil || provider.ID != GoogleAntigravityProviderID {
		return provider
	}
	catalog := &accountModelCatalog{
		providerID: GoogleAntigravityProviderID,
		scopeLabel: "live",
		endpoint:   googleAntigravityCatalogEndpoint,
		identity:   googleAntigravityModelIdentity,
		builtin:    func(modelID string) bool { return GetBuiltinModel(GoogleAntigravityProviderID, modelID) != nil },
	}
	return withAccountModelDiscovery(provider, catalog, fetchGoogleAntigravityModelRoster)
}

func googleAntigravityCatalogEndpoint(baseURL string) (string, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = googleAntigravityPrimaryEndpoint
	}
	return strings.TrimRight(base, "/"), nil
}

// googleAntigravityModelIdentity derives a stable, non-secret roster key from
// account-identifying credential claims (or a digest of a non-redacted refresh
// token). Without an account discriminator it returns false so
// accountModelCatalog falls back to an ephemeral access-token scope.
func googleAntigravityModelIdentity(credential Credential) (string, bool) {
	identity := []string{}
	add := func(name, value string) {
		if value = strings.TrimSpace(value); value != "" {
			identity = append(identity, name, value)
		}
	}
	add("account", credential.AccountID)
	add("email", credential.EnterpriseDomain)
	add("project", credential.ProjectID)
	for _, key := range []string{"email", "userId"} {
		add(key, stringClaim(credential, key))
	}
	if refresh := credential.Refresh; refresh != "" && refresh != RemoteRefreshSentinel {
		add("refresh", accountModelsDigest(GoogleAntigravityProviderID, refresh)[:32])
	}
	if len(identity) == 0 {
		return "", false
	}
	encoded, _ := json.Marshal(identity)
	return string(encoded), true
}

func fetchGoogleAntigravityModelRoster(ctx context.Context, baseURL string, credential *Credential) ([]*Model, string, error) {
	if credential == nil || credential.Type != CredentialOAuth || credential.Access == "" {
		return nil, "", errors.New("Google Antigravity model discovery requires OAuth credentials")
	}
	roster, err := googleDiscoverAntigravityModelsAt(ctx, baseURL, credential)
	if err != nil {
		return nil, "", err
	}
	chat := chatModels(roster)
	if len(chat) == 0 {
		return nil, "", errors.New("Google Antigravity model discovery returned an empty roster")
	}
	return chat, "", nil
}

// GoogleGeminiCliProvider is the google-gemini-cli provider: Google's own roster through Cloud Code Assist.
func GoogleGeminiCliProvider() *ModelsProvider {
	flow := googleGeminiCliFlow
	specs := googleGeminiCliRuntimeModels()
	baseline := make([]AnyModel, 0, len(specs))
	for _, spec := range specs {
		baseline = append(baseline, googleCloudCodeRuntimeModel(spec))
	}
	return CreateProvider(CreateProviderOptions{
		ID:      GoogleGeminiCliProviderID,
		Name:    ptrString("Google Gemini CLI"),
		BaseURL: googleCloudCodeAssistEndpoint,
		Auth: ProviderAuth{
			APIKey: googleCloudCodeAPIKeyAuth("Google Gemini CLI credential"),
			OAuth:  googleCloudCodeOAuthAuth(flow),
		},
		Models: baseline,
		API:    GoogleCloudCodeAssistAPI(),
	})
}

// ─── Antigravity model discovery ─────────────────────────────────────────────

// googleAntigravityDiscoveryDenylist lists roster ids the backend lists but refuses to serve.
var googleAntigravityDiscoveryDenylist = map[string]struct{}{"chat_20706": {}, "chat_23310": {}, "gemini-2.5-pro": {}}

// googleAntigravityModelInfo is one roster row of fetchAvailableModels.
type googleAntigravityModelInfo struct {
	DisplayName      string `json:"displayName"`
	SupportsImages   *bool  `json:"supportsImages"`
	SupportsThinking *bool  `json:"supportsThinking"`
	ThinkingBudget   *int   `json:"thinkingBudget"`
	ThinkingLevel    *int   `json:"thinkingLevel"`
	MaxTokens        *int   `json:"maxTokens"`
	MaxOutputTokens  *int   `json:"maxOutputTokens"`
	Model            string `json:"model"`
	IsInternal       *bool  `json:"isInternal"`
	Disabled         *bool  `json:"disabled"`
}

// googleAntigravityDiscoveryResponse is the fetchAvailableModels payload.
type googleAntigravityDiscoveryResponse struct {
	Models map[string]googleAntigravityModelInfo `json:"models"`
}

// googleDiscoverAntigravityModels reads the account's served roster. A failure returns no roster, so the
// shipped baseline stays in place; a disabled or internal row is dropped, because Cloud Code lists it but
// answers 404 for it.
func googleDiscoverAntigravityModels(ctx context.Context, credential *Credential) ([]AnyModel, error) {
	return googleDiscoverAntigravityModelsAt(ctx, "", credential)
}

func googleAntigravityDiscoveryEndpoints(baseURL string) []string {
	primary := strings.TrimRight(googleAntigravityPrimaryEndpoint, "/")
	sandbox := strings.TrimRight(googleAntigravitySandboxEndpoint, "/")
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base != "" && base != primary && base != sandbox {
		return []string{base}
	}
	return []string{primary, sandbox}
}

func googleDiscoverAntigravityModelsAt(ctx context.Context, baseURL string, credential *Credential) ([]AnyModel, error) {
	if credential == nil || credential.Access == "" {
		return nil, nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, ccaDiscoveryTimeoutMs*time.Millisecond)
	defer cancel()
	endpoints := googleAntigravityDiscoveryEndpoints(baseURL)
	var lastErr error
	for _, endpoint := range endpoints {
		body := "{}"
		if credential.ProjectID != "" {
			encoded, err := json.Marshal(map[string]string{"project": credential.ProjectID})
			if err == nil {
				body = string(encoded)
			}
		}
		request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint+googleCloudCodeAvailableModelsPath, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+credential.Access)
		request.Header.Set("Content-Type", "application/json")
		for name, value := range ccaAntigravityHeaders() {
			request.Header.Set(name, value)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			lastErr = err
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 8<<20))
		_ = response.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			lastErr = errors.New("fetchAvailableModels failed: " + googleStatusText(response.StatusCode))
			continue
		}
		var payload googleAntigravityDiscoveryResponse
		if err := json.Unmarshal(data, &payload); err != nil {
			lastErr = err
			continue
		}
		roster := googleAntigravityModelsFromResponse(payload, endpoint, ccaDiscoveryAccountKey(credential))
		if len(roster) > 0 {
			return roster, nil
		}
	}
	return nil, lastErr
}

// googleAntigravityModelsFromResponse builds the roster rows. It preserves the
// bundled catalog's costs, reasoning, and limits for known ids, falls back to
// the default tier SKU for collapsed families, and records each row's wire
// profile for the request constants.
func googleAntigravityModelsFromResponse(payload googleAntigravityDiscoveryResponse, endpoint, accountKey string) []AnyModel {
	ids := make([]string, 0, len(payload.Models))
	for id := range payload.Models {
		ids = append(ids, id)
	}
	// Effort-tier SKUs collapse into logical families first, exactly as the catalog generator does, so the
	// roster lists logical ids and the transport routes each effort to the wire id that serves it.
	ids, families := ccaCollapseTierFamilies(ids)
	for _, family := range families {
		ccaRecordAccountFamily(accountKey, family)
		for _, wireID := range family.Tiers {
			info, ok := payload.Models[wireID]
			if !ok {
				continue
			}
			if profile := googleAntigravityProfileFromInfo(info); profile != nil {
				ccaRecordAccountProfile(accountKey, wireID, *profile)
			}
		}
	}
	roster := make([]AnyModel, 0, len(ids))
	for _, id := range ids {
		info, direct := payload.Models[id]
		if !direct {
			if family, isFamily := families[id]; isFamily {
				info = payload.Models[family.Default]
			}
		}
		if _, denied := googleAntigravityDiscoveryDenylist[id]; denied {
			continue
		}
		if info.IsInternal != nil && *info.IsInternal {
			continue
		}
		if info.Disabled != nil && *info.Disabled {
			continue
		}
		model := GetBuiltinModel(GoogleAntigravityProviderID, id)
		if model == nil {
			spec := googleCloudCodeModelSpec{ID: id, Name: id, Reasoning: true, Input: []ModelInputModality{ModelInputText, ModelInputImage}}
			for _, baseline := range googleAntigravityBaseline {
				if baseline.ID == id {
					spec = baseline
					break
				}
			}
			spec.Provider = GoogleAntigravityProviderID
			spec.BaseURL = endpoint
			model = googleCloudCodeRuntimeModel(spec)
		} else {
			model.ProviderMeta.BaseURL = endpoint
		}
		if info.DisplayName != "" && (direct || model.DisplayName == "" || model.DisplayName == id) {
			model.DisplayName = info.DisplayName
		}
		if info.SupportsImages != nil {
			if *info.SupportsImages {
				if !slices.Contains(model.Input, ModelInputImage) {
					model.Input = append(model.Input, ModelInputImage)
				}
				model.Capabilities.SupportsImages = true
				if model.InputLimits == nil {
					model.InputLimits = googleCloudCodeImageLimits()
				}
			} else {
				model.Input = []ModelInputModality{ModelInputText}
				model.Capabilities.SupportsImages = false
				model.InputLimits = nil
			}
		}
		if info.SupportsThinking != nil {
			model.ProviderMeta.Reasoning = *info.SupportsThinking
			if !*info.SupportsThinking {
				model.Capabilities.MaxThinking = ""
			} else if model.Capabilities.MaxThinking == "" {
				model.Capabilities.MaxThinking = ThinkingLevelHigh
			}
		}
		if info.MaxTokens != nil && *info.MaxTokens > 0 {
			model.Capabilities.ContextWindow = *info.MaxTokens
		}
		if info.MaxOutputTokens != nil && *info.MaxOutputTokens > 0 {
			model.Capabilities.MaxOutputTokens = *info.MaxOutputTokens
		}
		if profile := googleAntigravityProfileFromInfo(info); profile != nil {
			ccaRecordAccountProfile(accountKey, id, *profile)
			model.ProviderMeta.Compat = withCCAWireProfile(model.ProviderMeta.Compat, id, profile)
		}
		roster = append(roster, model)
	}
	slices.SortFunc(roster, func(a, b AnyModel) int {
		if order := strings.Compare(strings.ToLower(a.(*Model).DisplayName), strings.ToLower(b.(*Model).DisplayName)); order != 0 {
			return order
		}
		return strings.Compare(a.ModelID(), b.ModelID())
	})
	return roster
}

// ccaDiscoveryAccountKey is the key a discovered roster's request constants are stored under: the
// credential's account id, else its email, else its project.
func ccaDiscoveryAccountKey(credential *Credential) string {
	if credential == nil {
		return ""
	}
	for _, value := range []string{credential.AccountID, credential.EnterpriseDomain, credential.ProjectID} {
		if value != "" {
			return value
		}
	}
	return ""
}

// googleAntigravityProfileFromInfo reads the request constants a roster row advertises.
func googleAntigravityProfileFromInfo(info googleAntigravityModelInfo) *ccaAntigravityWireProfile {
	profile := &ccaAntigravityWireProfile{}
	if info.Model != "" {
		profile.ModelEnum = info.Model
	}
	if info.MaxOutputTokens != nil && *info.MaxOutputTokens > 0 {
		profile.MaxOutputTokens = *info.MaxOutputTokens
	}
	if info.ThinkingLevel != nil {
		switch *info.ThinkingLevel {
		case 1:
			profile.ThinkingLevel = "LOW"
		case 2:
			profile.ThinkingLevel = "MEDIUM"
		case 3:
			profile.ThinkingLevel = "HIGH"
		}
	}
	if info.ThinkingBudget != nil {
		profile.ThinkingBudget = info.ThinkingBudget
	}
	if profile.ModelEnum == "" && profile.MaxOutputTokens == 0 && profile.ThinkingLevel == "" && profile.ThinkingBudget == nil {
		return nil
	}
	return profile
}

// withCCAWireProfile keeps the model's compat bag as it is: the model struct carries no field for request
// constants, which live in the per-account table.
func withCCAWireProfile(compat *ModelCompat, wireID string, profile *ccaAntigravityWireProfile) *ModelCompat {
	return compat
}

// ccaStoredWireProfileFor reads a discovered request profile from any account. The serving account's own
// table is consulted first by ccaWireProfileForAccount; this is the fallback for a model built by a caller
// that carries no account identity.
func ccaStoredWireProfileFor(wireID string) (ccaAntigravityWireProfile, bool) {
	ccaAntigravityAccounts.mu.RLock()
	defer ccaAntigravityAccounts.mu.RUnlock()
	for _, account := range ccaAntigravityAccounts.accounts {
		if profile, ok := account.Profiles[wireID]; ok {
			return profile, true
		}
	}
	return ccaAntigravityWireProfile{}, false
}

// ─── Registration ────────────────────────────────────────────────────────────

func init() {
	googleCloudCodeRegister()
}

// googleCloudCodeRegister extends the shared tables with the two providers.
func googleCloudCodeRegister() {
	// Provider ids, display names and base URLs.
	GeneratedProviders = append(GeneratedProviders, GoogleAntigravityProviderID, GoogleGeminiCliProviderID)
	if names := builtinProviderNames(); names != nil {
		names[GoogleAntigravityProviderID] = "Google Antigravity"
		names[GoogleGeminiCliProviderID] = "Google Gemini CLI"
	}
	if urls := builtinProviderBaseURLs(); urls != nil {
		urls[GoogleAntigravityProviderID] = googleAntigravityPrimaryEndpoint
		urls[GoogleGeminiCliProviderID] = googleCloudCodeAssistEndpoint
	}
	// API-key auth display names: the providers accept a provisioned credential only. The methods table is
	// written directly as well, so a table built before this init still resolves the two providers.
	builtinAPIKeyNames()[GoogleAntigravityProviderID] = "Google Antigravity credential"
	builtinAPIKeyNames()[GoogleGeminiCliProviderID] = "Google Gemini CLI credential"
	auths := builtinAPIKeyAuths()
	auths[GoogleAntigravityProviderID] = googleCloudCodeAPIKeyAuth("Google Antigravity credential")
	auths[GoogleGeminiCliProviderID] = googleCloudCodeAPIKeyAuth("Google Gemini CLI credential")
	// OAuth flows.
	builtInOAuthProviders[GoogleAntigravityProviderID] = googleCloudCodeOAuthProvider{flow: googleAntigravityFlow}
	builtInOAuthProviders[GoogleGeminiCliProviderID] = googleCloudCodeOAuthProvider{flow: googleGeminiCliFlow}
	builtinOAuthNames[GoogleAntigravityProviderID] = struct {
		name         string
		subscription bool
	}{name: googleAntigravityFlow.displayName, subscription: true}
	builtinOAuthNames[GoogleGeminiCliProviderID] = struct {
		name         string
		subscription bool
	}{name: googleGeminiCliFlow.displayName, subscription: true}
	builtinOAuthLoginLabels[GoogleAntigravityProviderID] = "Sign in with Google"
	builtinOAuthLoginLabels[GoogleGeminiCliProviderID] = "Sign in with Google"
	// Provider constructors.
	builtinProviderConstructors[GoogleAntigravityProviderID] = GoogleAntigravityProvider
	builtinProviderConstructors[GoogleGeminiCliProviderID] = GoogleGeminiCliProvider
	// API implementation.
	builtInProviders[APIGoogleGeminiCLI] = func(apiKey, model, baseURL string) Provider {
		providerID := GoogleGeminiCliProviderID
		if baseURL != "" && strings.Contains(baseURL, "daily-cloudcode-pa") {
			providerID = GoogleAntigravityProviderID
		}
		return NewGoogleCloudCodeAssistProvider(GoogleCloudCodeAssistConfig{APIKey: apiKey, Model: model, ProviderID: providerID, BaseURL: baseURL})
	}
	builtinAPIs = append(builtinAPIs, struct {
		api     API
		streams func() *ProviderStreams
	}{api: APIGoogleGeminiCLI, streams: GoogleCloudCodeAssistAPI})
	// Catalog rows.
	GeneratedModels = append(GeneratedModels, googleCloudCodeGeneratedRows(googleAntigravityRuntimeModels())...)
	GeneratedModels = append(GeneratedModels, googleCloudCodeGeneratedRows(googleGeminiCliRuntimeModels())...)
}
