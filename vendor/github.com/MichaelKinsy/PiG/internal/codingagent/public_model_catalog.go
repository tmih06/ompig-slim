package codingagent

import (
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelmetadata"
)

// publicModelCatalog memoizes a projection of the durable public-source snapshot.
// The metadata service owns network/cache policy and atomic persistence; this
// projection never performs I/O. Published models and their nested data are
// immutable. A replacement service or generation invalidates every provider.
type publicModelCatalog struct {
	mu         sync.Mutex
	service    *modelmetadata.Service
	generation uint64
	bySource   map[string][]ai.ModelCatalogMetadataRow
	byProvider map[string][]*ai.Model
}

// publicCatalogAdditions makes OMP's shared source additive to the curated seed.
// Known IDs keep their provider-authored transport/limits, as OMP's additiveOnly
// shared layer does; native endpoint discovery remains authoritative above this
// layer. Explicit models/overrides are composed afterwards. Unknown providers
// and account-scoped rosters cannot be authorized by this unauthenticated feed.
func (r *ModelRegistry) publicCatalogAdditions(provider string) []*ai.Model {
	policy, known := ai.OMPProviderCatalogPolicy(provider)
	if !known || policy.AccountScoped {
		return nil
	}
	service := r.modelMetadata()
	generation := service.Generation()
	if generation == 0 {
		return nil
	}
	cache := &r.publicCatalog
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.service != service || cache.generation != generation {
		cache.service = service
		cache.generation = generation
		cache.bySource = map[string][]ai.ModelCatalogMetadataRow{}
		cache.byProvider = map[string][]*ai.Model{}
		for _, row := range service.Rows() {
			cache.bySource[row.Provider] = append(cache.bySource[row.Provider], row)
		}
	}
	if models, ok := cache.byProvider[provider]; ok {
		return models
	}
	seed, _ := ai.OMPChatCatalog(provider)
	var template *ai.Model
	if generated, ok := ai.LookupOMPChatModel(provider, policy.DefaultModel); ok {
		template = generated.ToModel()
	}
	if template == nil && len(seed) > 0 {
		template = seed[0].ToModel()
	}
	// Empty/dynamic-only providers need their native manager, not a guessed API.
	if template == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, row := range seed {
		seen[row.ID] = true
	}
	for _, model := range ai.OMPOtherCatalog(provider) {
		seen[model.ModelID()] = true
	}
	var added []*ai.Model
	for _, source := range policy.SourceKeys {
		for _, row := range cache.bySource[source] {
			if seen[row.ID] || !publicChatRowEligible(row) {
				continue
			}
			model := publicChatModel(provider, template, row)
			seen[row.ID] = true
			added = append(added, model)
		}
	}
	sort.Slice(added, func(i, j int) bool { return added[i].ID < added[j].ID })
	cache.byProvider[provider] = slices.Clip(added)
	return cache.byProvider[provider]
}

// publicChatRowEligible requires affirmative tool support and usable positive
// limits. Neither an unknown kind nor metadata-only rows can create a runnable
// chat model. Deprecated/beta/preview status is not a deletion instruction: OMP
// retains such catalog rows unless the provider's own policy excludes them.
func publicChatRowEligible(row ai.ModelCatalogMetadataRow) bool {
	return row.ID != "" && strings.TrimSpace(row.ID) == row.ID && !strings.ContainsAny(row.ID, "\x00\r\n") &&
		(row.Kind == "" || row.Kind == "chat") && row.ToolCall != nil && *row.ToolCall &&
		row.ContextWindow != nil && *row.ContextWindow > 0 && row.MaxTokens != nil && *row.MaxTokens > 0 && slices.Contains(row.Input, ai.ModelInputText)
}

// publicChatModel projects only data onto a trusted provider transport. The
// public feed cannot set API/baseURL, headers, auth, upstream routing or compat.
// Per-model seed fields (priority, signed-thinking policy, effort mappings) are
// deliberately not cloned onto a different ID. Unsupported provider protocols
// still require code; new IDs on the existing protocol do not.
func publicChatModel(provider string, template *ai.Model, row ai.ModelCatalogMetadataRow) *ai.Model {
	model := &ai.Model{Type: ai.ModelTypeChat, ID: row.ID, DisplayName: row.Name,
		ProviderMeta: ai.ProviderMetadata{ProviderID: provider, API: template.ProviderMeta.API, BaseURL: template.ProviderMeta.BaseURL},
		Input:        slices.Clone(row.Input), Capabilities: ai.ModelCapabilities{ContextWindow: *row.ContextWindow, MaxOutputTokens: *row.MaxTokens},
		Description: row.Description, Catalog: &ai.ModelCatalogProvenance{Source: "cached", Provider: row.Provider, ModelID: row.ID}}
	if model.DisplayName == "" {
		model.DisplayName = row.ID
	}
	if row.Int != nil {
		model.Int = new(*row.Int)
		model.Catalog.Fields = append(model.Catalog.Fields, "int")
	}
	if row.TPS != nil {
		model.TPS = new(*row.TPS)
		model.Catalog.Fields = append(model.Catalog.Fields, "tps")
	}
	if row.Description != "" {
		model.Catalog.Fields = append(model.Catalog.Fields, "description")
	}
	if cost := row.Cost; cost != nil {
		if cost.Input != nil {
			model.Capabilities.InputCostPer1M = *cost.Input
		}
		if cost.Output != nil {
			model.Capabilities.OutputCostPer1M = *cost.Output
		}
		if cost.CacheRead != nil {
			model.Capabilities.CacheReadCostPer1M = *cost.CacheRead
		}
		if cost.CacheWrite != nil {
			model.Capabilities.CacheWriteCostPer1M = *cost.CacheWrite
		}
	}
	if row.Reasoning != nil && *row.Reasoning {
		model.ProviderMeta.Reasoning = true
		model.ThinkingLevelMap = modelmetadata.EffortMapFromCatalog(row)
		for _, level := range []ai.ModelThinkingLevel{"minimal", "low", "medium", "high", "xhigh", "max"} {
			if value, ok := model.ThinkingLevelMap[level]; ok && value != nil {
				model.Capabilities.MaxThinking = ai.ThinkingLevel(level)
			}
		}
	}
	return model
}

// withPublicCatalog lists shared-source additions through the same provider
// used by typed lookup and request binding. No new Stream or auth implementation
// is created, and user models still override these rows during composition.
func (r *ModelRegistry) withPublicCatalog(provider string, base *ai.ModelsProvider) *ai.ModelsProvider {
	if base == nil {
		return nil
	}
	wrapped := *base
	wrapped.GetModels = func() ([]*ai.Model, error) {
		models, err := base.GetModels()
		if err != nil {
			return nil, err
		}
		return append(slices.Clone(models), r.publicCatalogAdditions(provider)...), nil
	}
	wrapped.GetAllModels = func() ([]ai.AnyModel, error) {
		models, err := allProviderModels(base)
		if err != nil {
			return nil, err
		}
		return append(slices.Clone(models), ai.AnyModels(r.publicCatalogAdditions(provider))...), nil
	}
	return &wrapped
}
