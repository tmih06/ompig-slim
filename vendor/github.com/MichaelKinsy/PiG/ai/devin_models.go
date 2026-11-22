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
		project:    devinCollapseFamilies,
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
	models = devinCollapseFamilies(models)
	slices.SortFunc(models, func(a, b *Model) int { return strings.Compare(a.ID, b.ID) })
	return models
}

// devinServiceSuffix maps a service-tier uid suffix to its family lane: the
// fast lane (`-lightning`, `-priority`, `-fast`) and the 1M-context lane.
var devinServiceSuffix = []struct{ suffix, lane string }{
	{"-lightning", "-fast"},
	{"-priority", "-fast"},
	{"-fast", "-fast"},
	{"-1m", "-1m"},
}

var devinEffortSuffixes = []string{
	"-none", "-minimal", "-low", "-medium", "-high", "-xhigh", "-max",
	"-thinking", "-no-thinking", "-off",
}

// devinEffortOrder ranks lane names when no wire default survives a stored
// roster; the native client defaults to the strongest tier.
var devinEffortOrder = map[string]int{
	"max": 7, "xhigh": 6, "high": 5, "medium": 4, "low": 3,
	"minimal": 2, "thinking": 1, "none": 0, "no-thinking": 0, "off": 0,
}

// devinLaneParts splits a wire uid into family base, effort name, and service
// lane. swe-2-medium-lightning is the medium lane of family swe-2-fast;
// gpt-6-sol-high-priority is the high lane of gpt-6-sol-fast.
func devinLaneParts(uid string) (base, effort, service string) {
	rest := uid
	for _, candidate := range devinServiceSuffix {
		if strings.HasSuffix(rest, candidate.suffix) {
			service = candidate.lane
			rest = rest[:len(rest)-len(candidate.suffix)]
			break
		}
	}
	for _, suffix := range devinEffortSuffixes {
		if strings.HasSuffix(rest, suffix) {
			effort = suffix[1:]
			rest = rest[:len(rest)-len(suffix)]
			break
		}
	}
	return rest, effort, service
}

// devinCollapseFamilies projects the wire-level roster onto the logical model
// set upstream's collapseVariants produces: one model per effort/service
// family plus standalone rows. Lane members keep their models (selections like
// devin/swe-2-max and request routing still resolve to the wire uid) but carry
// the effortLane compat marker so pickers show the family once. A family whose
// logical id has no wire uid synthesizes a model from its default member and
// routes through requestModelId, the same mechanism Fusion pairings use.
func devinCollapseFamilies(models []*Model) []*Model {
	byID := make(map[string]*Model, len(models))
	for _, model := range models {
		byID[model.ID] = model
	}
	type family struct {
		id      string
		members map[string]*Model // effort name ("" for plain service lanes) -> uid's model
	}
	families := map[string]*family{}
	var order []*family
	for _, model := range models {
		if strings.HasPrefix(model.ID, "fusion-") {
			continue
		}
		base, effort, service := devinLaneParts(model.ID)
		if effort == "" && service == "" {
			continue
		}
		familyID := base + service
		entry := families[familyID]
		if entry == nil {
			entry = &family{id: familyID, members: map[string]*Model{}}
			families[familyID] = entry
			order = append(order, entry)
		}
		if _, exists := entry.members[effort]; !exists {
			entry.members[effort] = model
		}
	}
	for _, entry := range order {
		// A uid that heads its own family (nemotron-3-5-lightning has its own
		// effort lanes) is a face, not a member of a -fast lane family.
		for effort, member := range entry.members {
			if _, headsFamily := families[member.ID]; headsFamily && member.ID != entry.id {
				delete(entry.members, effort)
			}
		}
		if face := byID[entry.id]; face != nil {
			// The bare family uid is the face, not a lane of itself.
			delete(entry.members, "")
			for effort, member := range entry.members {
				if member == face {
					delete(entry.members, effort)
				}
			}
		}
		if len(entry.members) < 2 && byID[entry.id] == nil {
			// A lone suffix-shaped uid is not a family: upstream collapses
			// only wire-declared ladders, so one orphan lane stays itself.
			continue
		}
		routing := map[string]string{}
		for effort, member := range entry.members {
			if member.ProviderMeta.Compat == nil {
				member.ProviderMeta.Compat = &ModelCompat{Extra: map[string]json.RawMessage{}}
			} else if member.ProviderMeta.Compat.Extra == nil {
				member.ProviderMeta.Compat.Extra = map[string]json.RawMessage{}
			}
			member.ProviderMeta.Compat.Extra["effortLane"] = devinJSON(entry.id)
			routing[effort] = member.ID
		}
		if byID[entry.id] != nil {
			continue
		}
		defaultMember := devinDefaultLane(entry.members)
		if defaultMember == nil {
			continue
		}
		family := *defaultMember
		family.ID = entry.id
		family.DisplayName = devinFamilyName(entry.id, defaultMember.DisplayName)
		compat := &ModelCompat{Extra: map[string]json.RawMessage{}}
		if defaultMember.ProviderMeta.Compat != nil {
			for key, value := range defaultMember.ProviderMeta.Compat.Extra {
				if key == "effortLane" {
					continue
				}
				compat.Extra[key] = value
			}
		}
		compat.Extra["requestModelId"] = devinJSON(defaultMember.ID)
		compat.Extra["effortRouting"] = devinJSON(routing)
		family.ProviderMeta.Compat = compat
		models = append(models, &family)
	}
	return models
}

// devinDefaultLane picks the member the family serves when no effort is
// requested: the strongest advertised tier, matching the native client's
// default.
func devinDefaultLane(members map[string]*Model) *Model {
	best := ""
	var pick *Model
	for effort, member := range members {
		if pick == nil || devinEffortOrder[effort] > devinEffortOrder[best] || pick.ID < member.ID && devinEffortOrder[effort] == devinEffortOrder[best] {
			best, pick = effort, member
		}
	}
	return pick
}

// devinFamilyName trims the lane word off a member's label ("SWE-2 High
// Lightning" -> "SWE-2 Fast"); it falls back to the family id.
func devinFamilyName(familyID, memberName string) string {
	fields := strings.Fields(strings.TrimSpace(memberName))
	for len(fields) > 0 {
		last := strings.ToLower(fields[len(fields)-1])
		if last == "lightning" || last == "priority" || last == "fast" || last == "1m" || last == "thinking" || devinEffortOrder[last] > 0 || last == "none" || last == "off" || last == "minimal" {
			fields = fields[:len(fields)-1]
			continue
		}
		break
	}
	trimmed := strings.Join(fields, " ")
	if trimmed == "" {
		return familyID
	}
	switch {
	case strings.HasSuffix(familyID, "-fast"):
		return trimmed + " Fast"
	case strings.HasSuffix(familyID, "-1m"):
		return trimmed + " 1M"
	}
	return trimmed
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
	cost, priced := devinModelCost(config.modelDimensions)
	if !priced {
		if card, ok := devinEffectiveCostCard(uid, time.Now()); ok {
			cost.Input, cost.Output, cost.CacheRead, cost.CacheWrite = card.Input, card.Output, card.CacheRead, card.CacheWrite
		}
	}
	if cost.CacheWrite == 0 {
		if rate := devinCacheWriteRate(uid); rate > 0 {
			cost.CacheWrite = cost.Input * rate
		}
	}
	if patch, ok := devinCostPatchCard(uid, time.Now()); ok {
		cost.Input, cost.Output, cost.CacheRead = patch.Input, patch.Output, patch.CacheRead
	}
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
// The second result reports whether the wire carried any token price at all;
// compat/rules/providers/devin.kdl seeds documented rates only when it did not.
func devinModelCost(dimensions []devinModelDimension) (ModelCost, bool) {
	cost := ModelCost{}
	priced := false
	for _, dimension := range dimensions {
		label := strings.ToLower(strings.TrimSpace(dimension.label))
		if label == "sidekick" {
			break
		}
		if dimension.kind != 1 && dimension.kind != 2 { // COST, COST_FUZZY
			continue
		}
		priced = true
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
	return cost, priced
}

// devinCostCard is one dated or standing price card (dollars per 1M tokens).
type devinCostCard struct {
	effectiveFrom time.Time // zero: always in effect
	Input         float64
	Output        float64
	CacheRead     float64
	CacheWrite    float64
}

// devinCostFallback is the cost-fallback policy of
// compat/rules/providers/devin.kdl: the documented enterprise (ACU) rates that
// apply only when GetCliModelConfigs reports no token price for the uid. Rows
// name each logical id plus its raw effort uids, matching the KDL model lists.
// Later cards override earlier ones once effectiveFrom passes.
var devinCostFallback = map[string][]devinCostCard{
	// SWE-2 promo until 2027-01-01T08:00Z, then enterprise list.
	"swe-2":        {{Input: 0.5, Output: 2.5, CacheRead: 0.05}, {effectiveFrom: time.Date(2027, 1, 1, 8, 0, 0, 0, time.UTC), Input: 3, Output: 15, CacheRead: 0.3}},
	"swe-2-medium": {{Input: 0.5, Output: 2.5, CacheRead: 0.05}, {effectiveFrom: time.Date(2027, 1, 1, 8, 0, 0, 0, time.UTC), Input: 3, Output: 15, CacheRead: 0.3}},
	"swe-2-high":   {{Input: 0.5, Output: 2.5, CacheRead: 0.05}, {effectiveFrom: time.Date(2027, 1, 1, 8, 0, 0, 0, time.UTC), Input: 3, Output: 15, CacheRead: 0.3}},
	"swe-2-max":    {{Input: 0.5, Output: 2.5, CacheRead: 0.05}, {effectiveFrom: time.Date(2027, 1, 1, 8, 0, 0, 0, time.UTC), Input: 3, Output: 15, CacheRead: 0.3}},
	// SWE-2 Fast: no published end date or list price.
	"swe-2-fast":             {{Input: 1, Output: 5, CacheRead: 0.1}},
	"swe-2-medium-lightning": {{Input: 1, Output: 5, CacheRead: 0.1}},
	"swe-2-high-lightning":   {{Input: 1, Output: 5, CacheRead: 0.1}},
	"swe-2-max-lightning":    {{Input: 1, Output: 5, CacheRead: 0.1}},
	"swe-2-flash":            {{Input: 0.075, Output: 0.375, CacheRead: 0.0075, CacheWrite: 0.09375}, {effectiveFrom: time.Date(2026, 11, 18, 18, 0, 0, 0, time.UTC), Input: 0.1, Output: 0.5, CacheRead: 0.01, CacheWrite: 0.125}},
	"swe-2-flash-medium":     {{Input: 0.075, Output: 0.375, CacheRead: 0.0075, CacheWrite: 0.09375}, {effectiveFrom: time.Date(2026, 11, 18, 18, 0, 0, 0, time.UTC), Input: 0.1, Output: 0.5, CacheRead: 0.01, CacheWrite: 0.125}},
	"swe-2-flash-high":       {{Input: 0.075, Output: 0.375, CacheRead: 0.0075, CacheWrite: 0.09375}, {effectiveFrom: time.Date(2026, 11, 18, 18, 0, 0, 0, time.UTC), Input: 0.1, Output: 0.5, CacheRead: 0.01, CacheWrite: 0.125}},
	"swe-2-flash-max":        {{Input: 0.075, Output: 0.375, CacheRead: 0.0075, CacheWrite: 0.09375}, {effectiveFrom: time.Date(2026, 11, 18, 18, 0, 0, 0, time.UTC), Input: 0.1, Output: 0.5, CacheRead: 0.01, CacheWrite: 0.125}},
	"swe-1-7":                {{Input: 0.5, Output: 2.5, CacheRead: 0.2}},
	"swe-1-7-medium":         {{Input: 0.5, Output: 2.5, CacheRead: 0.2}},
	"glm-5-2":                {{Input: 1.4, Output: 4.4, CacheRead: 0.26}},
}

// devinCostPatch is the KDL cost-patch policy: dated list cards that replace a
// wire or fallback price once the promo ends. Each uid list names the logical
// id plus its raw effort uids, as upstream's KDL does.
var devinCostPatch = map[string]devinCostCard{
	"gemini-4-argon":         {effectiveFrom: time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC), Input: 4, Output: 20, CacheRead: 0.2},
	"gemini-4-argon-low":     {effectiveFrom: time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC), Input: 4, Output: 20, CacheRead: 0.2},
	"gemini-4-argon-medium":  {effectiveFrom: time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC), Input: 4, Output: 20, CacheRead: 0.2},
	"gemini-4-argon-high":    {effectiveFrom: time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC), Input: 4, Output: 20, CacheRead: 0.2},
	"gemini-4-carbon":        {effectiveFrom: time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC), Input: 4, Output: 20, CacheRead: 0.2},
	"gemini-4-carbon-low":    {effectiveFrom: time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC), Input: 4, Output: 20, CacheRead: 0.2},
	"gemini-4-carbon-medium": {effectiveFrom: time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC), Input: 4, Output: 20, CacheRead: 0.2},
	"gemini-4-carbon-high":   {effectiveFrom: time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC), Input: 4, Output: 20, CacheRead: 0.2},
}

// devinFlashListCard is the class-wide Gemini Flash 3.7–3.9 list card.
var devinFlashListCard = devinCostCard{effectiveFrom: time.Date(2027, 1, 1, 8, 0, 0, 0, time.UTC), Input: 1.5, Output: 7.5, CacheRead: 0.15}

// devinCostPatchCard resolves the dated override for uid at now, or false.
// Lane suffixes (effort, then service tier) strip before the class check so
// gemini-3-7-flash-high-lightning prices like the rest of its revision.
func devinCostPatchCard(uid string, now time.Time) (devinCostCard, bool) {
	if card, ok := devinCostPatch[uid]; ok && !card.effectiveFrom.After(now) {
		return card, true
	}
	base := uid
	for _, suffix := range []string{"-lightning", "-priority", "-fast", "-1m"} {
		base = strings.TrimSuffix(base, suffix)
	}
	for _, suffix := range []string{"-minimal", "-low", "-medium", "-high", "-xhigh", "-max", "-none", "-thinking", "-off"} {
		base = strings.TrimSuffix(base, suffix)
	}
	if (base == "gemini-3-7-flash" || base == "gemini-3-8-flash") && !devinFlashListCard.effectiveFrom.After(now) {
		return devinFlashListCard, true
	}
	return devinCostCard{}, false
}

// devinEffectiveCostCard resolves the fallback card in force at now.
func devinEffectiveCostCard(uid string, now time.Time) (devinCostCard, bool) {
	cards, ok := devinCostFallback[uid]
	if !ok {
		return devinCostCard{}, false
	}
	card := cards[0]
	for _, candidate := range cards[1:] {
		if !candidate.effectiveFrom.After(now) {
			card = candidate
		}
	}
	return card, true
}

// devinCacheWriteRate is the KDL cache-write-rate rule: Devin's published card
// bills cache writes at a multiple of input for Claude and GPT-5.6+ models;
// `adaptive` bills them at the input rate itself. Other lineages publish none.
func devinCacheWriteRate(uid string) float64 {
	switch {
	case uid == "adaptive":
		return 1.0
	case strings.HasPrefix(uid, "claude-"):
		return 1.25
	default:
		base := uid
		if cut := strings.Index(base, "-fast"); cut > 0 {
			base = base[:cut]
		}
		if cut := strings.Index(base, "-priority"); cut > 0 {
			base = base[:cut]
		}
		if devinGPTRevision(base) >= 5.6 && devinGPTRevision(base) < 7 {
			return 1.25
		}
		return 0
	}
}

// devinGPTRevision extracts the gpt revision from uids like gpt-6-2-astra-low
// or gpt-5-6-sol-priority; zero when the uid is not a gpt lane.
func devinGPTRevision(uid string) float64 {
	if !strings.HasPrefix(uid, "gpt-") {
		return 0
	}
	parts := strings.Split(strings.TrimPrefix(uid, "gpt-"), "-")
	if len(parts) == 0 {
		return 0
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	minor := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	return float64(major) + float64(minor)/10
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
