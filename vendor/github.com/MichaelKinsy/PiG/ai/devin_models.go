package ai

// Devin account-scoped model discovery: GetCliModelConfigs against the
// Connect/protobuf endpoint, normalized onto the catalog model shape. The
// credential-scoped roster catalog is account_models.go's, shared with Codex;
// the wire decode lives in devin_proto.go.
//
// Ports packages/catalog/src/discovery/devin.ts (fetchDevinModels,
// normalizeDevinModels). The effort-family collapse (collapseVariants) is a
// deliberate omission: every wire UID lists as its own model.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// WithDevinModelDiscovery installs the live Devin roster on the provider,
// scoped by credential identity and endpoint, with the seed catalog as
// baseline.
func WithDevinModelDiscovery(provider *ModelsProvider) *ModelsProvider {
	if provider == nil || provider.ID != devinProviderName {
		return provider
	}
	catalog := &accountModelCatalog{
		providerID: devinProviderName,
		scopeLabel: "live",
		endpoint:   func(baseURL string) (string, error) { return devinModelEndpoint(baseURL), nil },
		identity:   devinModelIdentity,
		builtin:    func(modelID string) bool { return GetBuiltinModel(devinProviderName, modelID) != nil },
	}
	return withAccountModelDiscovery(provider, catalog, func(ctx context.Context, baseURL string, credential *Credential) ([]*Model, string, error) {
		models, err := fetchDevinModelRoster(ctx, baseURL, credential)
		return models, "", err
	})
}

func devinModelEndpoint(baseURL string) string {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = devinDefaultBaseURL
	}
	return strings.TrimRight(base, "/")
}

// devinModelIdentity is a stable, non-secret roster key: account and org ids
// plus profile claims, never the token itself. The second result reports
// whether the identity came from those stable fields rather than falling back
// to the access token.
func devinModelIdentity(credential Credential) (string, bool) {
	parts := []string{}
	add := func(name, value string) {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, name+"="+value)
		}
	}
	add("account", credential.AccountID)
	add("project", credential.ProjectID)
	for _, key := range []string{"email", "orgId", "orgName", "userId", "apiServerUrl"} {
		var value string
		if raw := credential.Extra[key]; len(raw) != 0 {
			_ = json.Unmarshal(raw, &value)
		}
		add(key, value)
	}
	// JWT subject is a stable account discriminator when present.
	if claims := devinJWTClaims(credential.Access); claims != nil {
		if sub, ok := claims["sub"].(string); ok {
			add("sub", sub)
		}
		if email, ok := claims["email"].(string); ok {
			add("jwt-email", email)
		}
	}
	return strings.Join(parts, "\x00"), len(parts) > 0
}

func devinJWTClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return nil
	}
	return claims
}

// --- Fetch + normalize ---------------------------------------------------------

// fetchDevinModelRoster performs the GetCliModelConfigs unary RPC and
// normalizes the response. A 15-second bound matches omp's discovery timeout.
func fetchDevinModelRoster(ctx context.Context, baseURL string, credential *Credential) ([]*Model, error) {
	if credential == nil || credential.Type != CredentialOAuth {
		return nil, errors.New("Devin model discovery requires OAuth credentials")
	}
	apiKey := devinCredentialToken(UsageCredential{Type: CredentialOAuth, AccessToken: credential.Access})
	if apiKey == "" {
		return nil, errors.New("Devin model discovery requires a session token")
	}
	endpoint := devinModelEndpoint(baseURL)
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	roster, err := devinUnary(reqCtx, http.DefaultClient, endpoint, devinCliModelConfigsPath, devinGetCliModelConfigsRequest(apiKey), decodeDevinCliModelConfigsResponse)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return nil, fmt.Errorf("Devin model discovery failed: %w", err)
	}
	models := normalizeDevinModelConfigs(roster, endpoint)
	if len(models) == 0 {
		// An empty-but-200 roster is the stale-identity failure signature; the
		// seed catalog stays authoritative.
		return nil, errors.New("Devin model discovery returned an empty roster")
	}
	return models, nil
}

// devinImageBlindUIDs never advertise image input even when features claim it.
var devinImageBlindUIDs = map[string]bool{"swe-1-6": true, "swe-1-6-fast": true}

// normalizeDevinModelConfigs ports normalizeDevinModels: skip disabled and
// internal displays, dedupe UIDs, derive capabilities/cost/limits, route fusion
// pairings to their lead, and mark the account default. Effort-family collapse
// (collapseVariants) is deliberately not ported: each wire UID lists itself.
func normalizeDevinModelConfigs(response *devinCliModelConfigsResponse, baseURL string) []*Model {
	if response == nil {
		return nil
	}
	liveConfigs := map[string]devinClientModelConfig{}
	for _, config := range response.configs {
		uid := strings.TrimSpace(config.modelUID)
		if !config.disabled && uid != "" {
			if _, exists := liveConfigs[uid]; !exists {
				liveConfigs[uid] = config
			}
		}
	}
	seen := map[string]bool{}
	models := []*Model{}
	for _, config := range response.configs {
		if config.disabled {
			continue
		}
		var display int
		if config.modelInfo != nil {
			display = config.modelInfo.displayOption
		}
		// Internal displays: QUICK_REVIEW(4) and INTERNAL_DEFAULT(6).
		if display == 4 || display == 6 {
			continue
		}
		uid := strings.TrimSpace(config.modelUID)
		if uid == "" || seen[uid] {
			continue
		}
		seen[uid] = true
		isRouter := display == 3
		var harnessCount int
		if config.modelInfo != nil {
			if config.modelInfo.isModelRouter {
				isRouter = true
			}
			harnessCount = len(config.modelInfo.harnessUIDs)
		}
		// Only harness-less routing slots take AssignModel; harness-backed
		// composites (fusion-*) are valid chat uids themselves.
		isAssignModelRouter := isRouter && harnessCount == 0
		lead, isPairing := devinFusionLeadUID(uid, liveConfigs)
		if isPairing && lead == "" {
			continue // pairing whose lead is not live is never servable
		}
		model := devinModelFromConfig(config, uid, baseURL, isAssignModelRouter)
		if isPairing {
			leadModel := devinModelFromConfig(liveConfigs[lead], lead, baseURL, false)
			devinRouteFusionLead(model, leadModel)
		}
		if response.defaultOverrideUID != "" && uid == response.defaultOverrideUID {
			model.ProviderMeta.Compat.Extra["isProviderDefault"] = json.RawMessage("true")
		}
		models = append(models, model)
	}
	slices.SortFunc(models, func(a, b *Model) int { return strings.Compare(a.ID, b.ID) })
	return models
}

// devinFusionLeadUID is devinFusionLeadUid: for a
// fusion-<lead>[-fast]-sidekick-<sidekick> uid, pairing is true and lead is
// the live lead uid, or "" when no lead is live (never servable). Other uids
// report pairing false.
func devinFusionLeadUID(uid string, liveConfigs map[string]devinClientModelConfig) (lead string, pairing bool) {
	if !strings.HasPrefix(uid, "fusion-") {
		return "", false
	}
	cut := strings.Index(uid, "-sidekick-")
	if cut <= len("fusion-") {
		return "", false
	}
	candidate := uid[len("fusion-"):cut]
	if _, ok := liveConfigs[candidate]; ok {
		return candidate, true
	}
	if strings.HasSuffix(candidate, "-fast") {
		base := strings.TrimSuffix(candidate, "-fast")
		if _, ok := liveConfigs[base+"-priority"]; ok {
			return base + "-priority", true
		}
		if _, ok := liveConfigs[base]; ok {
			return base, true
		}
	}
	return "", true // pairing with no live lead
}

// devinRouteFusionLead is routeDevinFusionLead: the pairing runs only its lead,
// so limits, pricing, and modalities come from the lead's spec.
func devinRouteFusionLead(spec, lead *Model) {
	spec.ProviderMeta.Compat.Extra["requestModelId"] = devinJSON(lead.ID)
	if lead.ProviderMeta.Reasoning {
		spec.ProviderMeta.Reasoning = true
	}
	spec.Input = lead.Input
	spec.Capabilities.SupportsToolUse = lead.Capabilities.SupportsToolUse
	spec.Capabilities.InputCostPer1M = lead.Capabilities.InputCostPer1M
	spec.Capabilities.OutputCostPer1M = lead.Capabilities.OutputCostPer1M
	spec.Capabilities.CacheReadCostPer1M = lead.Capabilities.CacheReadCostPer1M
	spec.Capabilities.CacheWriteCostPer1M = lead.Capabilities.CacheWriteCostPer1M
	spec.Capabilities.ContextWindow = lead.Capabilities.ContextWindow
	spec.Capabilities.MaxOutputTokens = lead.Capabilities.MaxOutputTokens
}

func devinJSON(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

// devinModelFromConfig is devinModelSpec.
func devinModelFromConfig(config devinClientModelConfig, uid, baseURL string, isAssignModelRouter bool) *Model {
	var features *devinModelFeatures
	if config.modelInfo != nil {
		features = config.modelInfo.modelFeatures
	}
	supportsImages := config.supportsImages
	if features != nil {
		supportsImages = features.supportsImages
	}
	if devinImageBlindUIDs[uid] {
		supportsImages = false
	}
	input := []ModelInputModality{ModelInputText}
	if supportsImages {
		input = append(input, ModelInputImage)
	}
	compatExtra := map[string]json.RawMessage{}
	setCompat := func(key string, value any) {
		compatExtra[key] = devinJSON(value)
	}
	if isAssignModelRouter {
		setCompat("modelRouter", true)
	}
	if features != nil && features.supportsParallelToolCalls {
		setCompat("supportsParallelToolCalls", true)
	}
	maxOutputTokens := int64(64000)
	if config.modelInfo != nil && config.modelInfo.maxOutputTokens > 0 {
		maxOutputTokens = int64(config.modelInfo.maxOutputTokens)
	}
	contextWindow := int64(200000)
	if config.maxTokens > 0 {
		contextWindow = int64(config.maxTokens)
	} else if config.modelInfo != nil && config.modelInfo.maxTokens > 0 {
		contextWindow = int64(config.modelInfo.maxTokens)
	}
	supportsTools := true
	if features != nil {
		supportsTools = features.supportsToolCalls
	}
	cost := devinModelCost(config.modelDimensions)
	model := &Model{
		ID:          uid,
		DisplayName: devinModelDisplayName(config),
		Input:       input,
		ProviderMeta: ProviderMetadata{
			API:        APIDevinAgent,
			ProviderID: devinProviderName,
			BaseURL:    baseURL,
			Compat:     &ModelCompat{Extra: compatExtra},
		},
		Capabilities: ModelCapabilities{
			SupportsToolUse:     supportsTools,
			ContextWindow:       int(contextWindow),
			MaxOutputTokens:     int(maxOutputTokens),
			InputCostPer1M:      cost.Input,
			OutputCostPer1M:     cost.Output,
			CacheReadCostPer1M:  cost.CacheRead,
			CacheWriteCostPer1M: cost.CacheWrite,
		},
	}
	if devinSupportsThinking(config) {
		model.ProviderMeta.Reasoning = true
	}
	if description := strings.TrimSpace(config.description); description != "" {
		model.Description = description
	}
	if config.isNew {
		setCompat("isNew", true)
	}
	if config.isBeta {
		setCompat("isBeta", true)
	}
	if config.isRecommended {
		setCompat("isRecommended", true)
	}
	return model
}

var (
	devinReasoningLabelPattern   = regexp.MustCompile(`(?i)think|thinking|minimal|high|medium|low|xhigh|max|reasoning`)
	devinNoReasoningLabelPattern = regexp.MustCompile(`(?i)\bno thinking\b`)
)

// devinSupportsThinking is supportsDevinThinking: model features are
// authoritative; the label heuristic covers configs without features.
func devinSupportsThinking(config devinClientModelConfig) bool {
	if config.modelInfo != nil && config.modelInfo.modelFeatures != nil {
		return config.modelInfo.modelFeatures.supportsThinking
	}
	if devinNoReasoningLabelPattern.MatchString(config.label) {
		return false
	}
	return devinReasoningLabelPattern.MatchString(config.label)
}

func devinModelDisplayName(config devinClientModelConfig) string {
	if name := strings.TrimSpace(config.label); name != "" {
		return name
	}
	return config.modelUID
}

// devinModelCost is devinModelCost: COST(1)/COST_FUZZY(2) dimensions up to the
// "sidekick" marker, labels input/cached input/output, per-million normalized.
func devinModelCost(dimensions []devinModelDimension) ModelCost {
	cost := ModelCost{}
	for _, dimension := range dimensions {
		label := strings.ToLower(strings.TrimSpace(dimension.label))
		if label == "sidekick" {
			break
		}
		if dimension.kind != 1 && dimension.kind != 2 { // COST, COST_FUZZY
			continue
		}
		perMillion := math.Round((dimension.value*1_000_000/devinCostDenominatorTokens(dimension.denominator))*1e6) / 1e6
		switch label {
		case "input":
			cost.Input = perMillion
		case "cached input":
			cost.CacheRead = perMillion
		case "output":
			cost.Output = perMillion
		}
	}
	return cost
}

var devinCostDenominatorPattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*([kmb])?`)

var devinCostDenominatorScale = map[string]float64{"k": 1e3, "m": 1e6, "b": 1e9}

// devinCostDenominatorTokens parses "1M tokens"-style denominators; anything
// unparseable means one million tokens.
func devinCostDenominatorTokens(denominator string) float64 {
	match := devinCostDenominatorPattern.FindStringSubmatch(denominator)
	if match == nil {
		return 1_000_000
	}
	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil || number <= 0 {
		return 1_000_000
	}
	scale := 1.0
	if len(match) > 2 && match[2] != "" {
		scale = devinCostDenominatorScale[strings.ToLower(match[2])]
	}
	tokens := number * scale
	if tokens <= 0 {
		return 1_000_000
	}
	return tokens
}
