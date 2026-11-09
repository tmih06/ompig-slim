package codingagent

// Ports packages/coding-agent/src/core/model-metadata-provider.ts and the runtime wiring of
// packages/coding-agent/src/core/model-runtime.ts (createModelRuntime's metadata service, refresh()
// refreshing it, and the withModelMetadata wrapper on every composed provider).
//
// Provider wrapper that attaches public catalog metadata (int, tps, description, plus missing
// cost/limit fields and effort ladders) to every model a provider lists, without touching identity,
// routing, auth, defaults, or explicit configuration.
//
// Enrichment runs at list time because a provider's model list is the one place every consumer reads
// from (selectors, --list-models, the runtime snapshot, model resolution).
//
// Bundled providers only receive int/tps/description: their prices, limits, and thinking maps are
// curated with the release and outrank a public catalog scrape. Custom models.json entries are
// enriched fully where the composer builds them, where an explicitly configured value can be
// excluded field by field.

import (
	"context"
	"sync"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelmetadata"
)

// newRegistryModelMetadata builds the registry's metadata service and restores its durable snapshot
// before any model is composed or listed. The snapshot lives beside models-store.json so a pinned
// models path keeps its metadata with it, and a registry without an agent directory (tests, one-shot
// runs) never touches the user's snapshot.
//
// The restore is synchronous on purpose: model-runtime.ts:329-330 awaits metadata.load() before it
// constructs the runtime, so even a runtime created with refreshOnCreate:false (or offline) lists
// enriched models on its very first read. A background restore would let the first listing miss the
// snapshot, with no later publish to repaint it.
func newRegistryModelMetadata(agentDir string) *modelmetadata.Service {
	var store modelmetadata.Store = modelmetadata.NewInMemoryStore()
	if path := modelmetadata.DefaultPath(agentDir); path != "" {
		store = modelmetadata.NewFileStore(path)
	}
	service := modelmetadata.New(modelmetadata.Options{
		Store:      store,
		References: builtinMetadataReferences,
	})
	// A missing or corrupt snapshot is an empty catalog, never a startup failure.
	_ = service.Load(context.Background())
	return service
}

// builtinMetadataReferences lists the bundled declarations the public catalog does not publish: cache
// lifetimes and effort ladders. It is built once, lazily, from the shipped catalog; nothing is inferred
// from absence (model-runtime.ts builtinMetadataReferences).
var builtinMetadataReferences = sync.OnceValue(func() []modelmetadata.Reference {
	references := []modelmetadata.Reference{}
	for _, providerID := range ai.ListProviders() {
		for _, generated := range builtinChatCatalog(providerID) {
			if generated.PromptCache == nil && generated.ThinkingLevelMap == nil {
				continue
			}
			references = append(references, modelmetadata.Reference{
				Provider:         generated.Provider,
				ID:               generated.ID,
				Name:             generated.DisplayName,
				Reasoning:        generated.Reasoning,
				ThinkingLevelMap: generated.ThinkingLevelMap,
				PromptCache:      generated.PromptCache,
			})
		}
	}
	return references
})

// modelMetadata returns the metadata service, installing the in-memory one a zero registry starts with.
func (r *ModelRegistry) modelMetadata() *modelmetadata.Service {
	if service := r.metadata.Load(); service != nil {
		return service
	}
	created := newRegistryModelMetadata(r.agentDir)
	r.metadata.CompareAndSwap(nil, created)
	return r.metadata.Load()
}

// ModelMetadata returns the public model-metadata service of this registry.
func (r *ModelRegistry) ModelMetadata() *modelmetadata.Service { return r.modelMetadata() }

// SetModelMetadata replaces the metadata service (tests and hosts that own the snapshot).
//
// The memo is dropped so a replacement never serves enrichment resolved against the previous rows.
// The store happens first and the memo is keyed by the service pointer as well as its generation, so a
// read already in flight against the old service cannot leave an entry the new one accepts; forget
// then releases the old entries promptly.
func (r *ModelRegistry) SetModelMetadata(service *modelmetadata.Service) {
	r.metadata.Store(service)
	r.enricher.forget()
}

// LoadModelMetadata restores the durable snapshot once.
func (r *ModelRegistry) LoadModelMetadata(ctx context.Context) error {
	return r.modelMetadata().Load(ctx)
}

// ModelMetadataResult is the outcome of the last metadata refresh; ok is false before one runs.
func (r *ModelRegistry) ModelMetadataResult() (modelmetadata.RefreshResult, bool) {
	// Both fields are written together under r.mu; reading only one of them inside the lock would race.
	r.mu.RLock()
	result, refreshed := r.metadataResult, r.metadataRefreshed
	r.mu.RUnlock()
	return result, refreshed
}

// RefreshModelMetadata refreshes the public model-metadata catalog and re-derives enrichment.
//
// The rows actually available are returned: a failed fetch keeps the previous snapshot, so callers
// never lose labels or prices because the network was down.
func (r *ModelRegistry) RefreshModelMetadata(ctx context.Context, allowNetwork, force bool) (modelmetadata.RefreshResult, error) {
	service := r.modelMetadata()
	generation, source := service.Generation(), service.Source()
	result, err := service.Refresh(ctx, modelmetadata.RefreshOptions{AllowNetwork: &allowNetwork, Force: force})
	if err != nil {
		return result, err
	}
	r.mu.Lock()
	r.metadataResult, r.metadataRefreshed = result, true
	r.mu.Unlock()
	// Offline, fresh, failed, and unchanged refreshes keep the same rows. Do not
	// throw away their enrichment or publish a spurious model-list change.
	if generation != service.Generation() || source != service.Source() {
		r.enricher.forget()
		r.notifyModelDataChanged()
	}
	return result, nil
}

// notifyModelDataChanged publishes a catalog change to the registry's observers.
func (r *ModelRegistry) notifyModelDataChanged() {
	r.mu.RLock()
	listener := r.onChange
	observers := append([]*modelRegistryChangeListener(nil), r.observers...)
	r.mu.RUnlock()
	for _, observer := range observers {
		observer.publish()
	}
	listener.publish()
}

// modelMetadataExplicit is the field set an enriched model still leaves to the catalog.
//
// A model that carries a value without the catalog provenance naming it was configured explicitly
// (models.json, an extension definition, or a curated bundled value), and enrichment never overwrites
// it — an explicit 0 or 0 tps included.
func modelMetadataExplicit(model *ai.Model) modelmetadata.FieldSet {
	if model == nil {
		return nil
	}
	enriched := map[string]bool{}
	if model.Catalog != nil {
		for _, field := range model.Catalog.Fields {
			enriched[field] = true
		}
	}
	explicit := modelmetadata.FieldSet{}
	if model.Int != nil && !enriched[string(modelmetadata.FieldInt)] {
		explicit[modelmetadata.FieldInt] = true
	}
	if model.TPS != nil && !enriched[string(modelmetadata.FieldTPS)] {
		explicit[modelmetadata.FieldTPS] = true
	}
	if model.Description != "" && !enriched[string(modelmetadata.FieldDescription)] {
		explicit[modelmetadata.FieldDescription] = true
	}
	if len(explicit) == 0 {
		return nil
	}
	return explicit
}

// enrichChatModel attaches the catalog metrics of one bundled or composed chat model. A model without
// a catalog match is returned untouched.
func (r *ModelRegistry) enrichChatModel(service *modelmetadata.Service, generation uint64, model *ai.Model) *ai.Model {
	if model == nil || !ai.IsModelType(model, ai.ModelTypeChat) || generation == 0 {
		return model
	}
	// Image/classifier entries carry no catalog metrics; only chat rows are scored.
	patch, ok := service.ResolvePatch(model.ID, model.DisplayName, modelMetadataExplicit(model), modelmetadata.ResolveOptions{Mode: modelmetadata.ModeMetrics})
	if !ok {
		return model
	}
	enriched := new(*model)
	applyModelMetadataPatch(enriched, patch)
	return enriched
}

// metadataModelCacheEntry is one memoized enrichment of a base model object at a catalog generation.
type metadataModelCacheEntry struct {
	// service identifies the catalog the entry was resolved against. Two services can report the same
	// generation, so the pointer is part of the key, not just the generation.
	service    *modelmetadata.Service
	generation uint64
	model      *ai.Model
}

// metadataListCacheEntry is one memoized enrichment of a source model list at a catalog generation.
//
// It mirrors model-metadata-provider.ts's lastList: a provider usually rebuilds its list once per
// snapshot and then hands out the same array for every read, so the enriched list is reused instead of
// walked per call. Only one entry is kept, which bounds the cache by one provider's list.
type metadataListCacheEntry struct {
	// head identifies the source list's backing array; length is its length. A list of length zero has
	// no identity and is never cached.
	head       *ai.Model
	length     int
	service    *modelmetadata.Service
	generation uint64
	enriched   []*ai.Model
}

// metadataCacheMaxModels bounds the per-model memo, which serves providers whose list array is rebuilt
// per read while the model objects stay the same. It is cleared once it outgrows the bound, so the
// cache is bounded by a catalog's worth of models rather than by every model ever listed.
const metadataCacheMaxModels = 4096

// metadataEnricher memoizes catalog enrichment per base model object and per source list, both keyed by
// the catalog generation, so the steady-state cost of a listing is one map lookup per list and no
// allocation.
type metadataEnricher struct {
	mu     sync.Mutex
	models map[*ai.Model]metadataModelCacheEntry
	// lists holds the recently used source lists, at most metadataRecentLists.
	lists []*metadataListCacheEntry
}

// metadataRecentLists bounds how many source lists a registry remembers. Providers in one snapshot
// each hand out a stable list, so this covers the catalogs that are read repeatedly.
const metadataRecentLists = 8

// enrichChatModelCached is enrichChatModel with the per-model memo.
func (e *metadataEnricher) enrichChatModelCached(r *ModelRegistry, service *modelmetadata.Service, generation uint64, model *ai.Model) *ai.Model {
	if model == nil {
		return model
	}
	e.mu.Lock()
	cached, ok := e.models[model]
	e.mu.Unlock()
	if ok && cached.service == service && cached.generation == generation {
		return cached.model
	}
	next := r.enrichChatModel(service, generation, model)
	e.mu.Lock()
	if e.models == nil || len(e.models) >= metadataCacheMaxModels {
		e.models = make(map[*ai.Model]metadataModelCacheEntry, 64)
	}
	e.models[model] = metadataModelCacheEntry{service: service, generation: generation, model: next}
	e.mu.Unlock()
	return next
}

// listEntry returns the memoized enrichment of models at generation, if one is recorded.
func (e *metadataEnricher) listEntry(models []*ai.Model, service *modelmetadata.Service, generation uint64) []*ai.Model {
	if len(models) == 0 {
		return nil
	}
	head := models[0]
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, entry := range e.lists {
		if entry.head == head && entry.length == len(models) && entry.service == service && entry.generation == generation {
			return entry.enriched
		}
	}
	return nil
}

// recordList remembers the enrichment of models at generation.
func (e *metadataEnricher) recordList(models []*ai.Model, service *modelmetadata.Service, generation uint64, enriched []*ai.Model) {
	if len(models) == 0 {
		return
	}
	entry := &metadataListCacheEntry{head: models[0], length: len(models), service: service, generation: generation, enriched: enriched}
	e.mu.Lock()
	defer e.mu.Unlock()
	// Drop an older entry for the same source list, then keep the newest entries bounded.
	kept := make([]*metadataListCacheEntry, 0, len(e.lists)+1)
	for _, candidate := range e.lists {
		if candidate.head == entry.head && candidate.length == entry.length {
			continue
		}
		kept = append(kept, candidate)
	}
	if len(kept) == metadataRecentLists {
		kept = kept[1:]
	}
	e.lists = append(kept, entry)
}

// forget drops every memoized enrichment; the next read re-resolves against the current rows.
func (e *metadataEnricher) forget() {
	e.mu.Lock()
	e.models = nil
	e.lists = nil
	e.mu.Unlock()
}

// applyModelMetadataPatch writes the metrics-only fields of patch onto model and records provenance.
func applyModelMetadataPatch(model *ai.Model, patch modelmetadata.Patch) {
	fields := []string{}
	existing := map[string]bool{}
	if model.Catalog != nil {
		for _, field := range model.Catalog.Fields {
			if existing[field] {
				continue
			}
			existing[field] = true
			fields = append(fields, field)
		}
	}
	add := func(field modelmetadata.MetadataField) {
		if existing[string(field)] {
			return
		}
		existing[string(field)] = true
		fields = append(fields, string(field))
	}
	if patch.Int != nil {
		model.Int = patch.Int
		add(modelmetadata.FieldInt)
	}
	if patch.TPS != nil {
		model.TPS = patch.TPS
		add(modelmetadata.FieldTPS)
	}
	if patch.Description != nil {
		model.Description = *patch.Description
		add(modelmetadata.FieldDescription)
	}
	model.Catalog = &ai.ModelCatalogProvenance{Source: string(patch.Source), Provider: patch.Provider, ModelID: patch.ModelID, Fields: fields}
}

// enrichModelData applies the metrics enrichment to a provider's chat models, leaving every other
// model type untouched.
//
// The list and every model in it are memoized by catalog generation, so a repeated read returns the
// same slice and allocates nothing while the rows are unchanged (model-metadata-provider.ts
// withModelMetadata).
func (r *ModelRegistry) enrichModelData(models []*ai.Model) []*ai.Model {
	if len(models) == 0 {
		return models
	}
	service := r.modelMetadata()
	generation := service.Generation()
	if generation == 0 {
		return models
	}
	if enriched := r.enricher.listEntry(models, service, generation); enriched != nil {
		return enriched
	}
	// Allocate the enriched copy only once a model actually changes, so an unchanged list is returned
	// as the caller handed it over.
	var enriched []*ai.Model
	for i, model := range models {
		next := r.enricher.enrichChatModelCached(r, service, generation, model)
		if next == model {
			continue
		}
		if enriched == nil {
			enriched = make([]*ai.Model, len(models))
			copy(enriched, models)
		}
		enriched[i] = next
	}
	if enriched == nil {
		r.enricher.recordList(models, service, generation, models)
		return models
	}
	r.enricher.recordList(models, service, generation, enriched)
	return enriched
}

// enrichAnyModelData applies the metrics enrichment to the chat models of a mixed list.
func (r *ModelRegistry) enrichAnyModelData(models []ai.AnyModel) []ai.AnyModel {
	if len(models) == 0 {
		return models
	}
	service := r.modelMetadata()
	generation := service.Generation()
	if generation == 0 {
		return models
	}
	var enriched []ai.AnyModel
	for i, model := range models {
		chat, ok := model.(*ai.Model)
		if !ok || !ai.IsModelType(chat, ai.ModelTypeChat) {
			continue
		}
		next := r.enricher.enrichChatModelCached(r, service, generation, chat)
		if next == chat {
			continue
		}
		if enriched == nil {
			enriched = make([]ai.AnyModel, len(models))
			copy(enriched, models)
		}
		enriched[i] = next
	}
	if enriched == nil {
		return models
	}
	return enriched
}

// enrichDefinitionEntry applies the full catalog patch to one models.json model entry that the
// composer just built, excluding every field the entry set itself (provider-composer.ts
// modelFromJson). Resolve even without fetched rows: bundled references supply
// declared thinking/cache capabilities on a first offline launch. Explicit
// endpoint effort opt-outs still prevent inheriting a nonfunctional ladder.
func (r *ModelRegistry) enrichDefinitionEntry(entry *ModelEntry, definition modelDefinition) {
	// Every registry path that builds an entry must publish the same values, so a registry that was
	// constructed without a service gets one here rather than listing unenriched models.
	service := r.modelMetadata()
	explicit := modelmetadata.ExplicitMetadataFields(modelmetadata.ExplicitDefinition{
		Int:              definition.Int,
		TPS:              definition.TPS,
		Description:      definition.Description,
		Reasoning:        definition.Reasoning,
		ThinkingLevelMap: definition.ThinkingLevelMap,
		ContextWindow:    definition.ContextWindow,
		MaxTokens:        definition.MaxTokens,
		Cost:             definitionCost(definition),
	})
	allowEffortMap := (definition.Reasoning == nil || *definition.Reasoning) && modelSendsReasoningEffort(entry.API, entry.Compat)
	patch, ok := service.ResolvePatch(definition.ID, definition.DisplayName(), explicit, modelmetadata.ResolveOptions{Mode: modelmetadata.ModeFull, AllowEffortMap: allowEffortMap})
	if !ok {
		return
	}
	applyModelMetadataPatchToEntry(entry, patch, allowEffortMap)
}

// DisplayName returns the definition's display name, falling back to its id.
func (d modelDefinition) DisplayName() string {
	if d.Name != "" {
		return d.Name
	}
	return d.ID
}

// definitionCost reads the rates an entry declared, so explicit rates are never overwritten.
func definitionCost(definition modelDefinition) *modelmetadata.CostRates {
	if definition.Cost == nil {
		return nil
	}
	// A models.json cost block writes all four rates, so every rate counts as explicit.
	input, output := definition.Cost.Input, definition.Cost.Output
	cacheRead, cacheWrite := definition.Cost.CacheRead, definition.Cost.CacheWrite
	return &modelmetadata.CostRates{Input: &input, Output: &output, CacheRead: &cacheRead, CacheWrite: &cacheWrite}
}

// modelSendsReasoningEffort reports whether an API actually transmits reasoning effort for a model
// with the given compat, so an inherited map is functional rather than display-only.
func modelSendsReasoningEffort(api string, compat *ai.OpenAICompat) bool {
	var supports *bool
	if compat != nil {
		supports = compat.SupportsReasoningEffort
	}
	return modelmetadata.ModelSendsEffort(api, supports)
}

// applyModelMetadataPatchToEntry writes a full patch onto a composed model entry, keeping entries
// (and their rounded JSON records) free of fields the catalog does not improve.
func applyModelMetadataPatchToEntry(entry *ModelEntry, patch modelmetadata.Patch, allowEffortMap bool) {
	fields := []string{}
	existing := map[string]bool{}
	if entry.Catalog != nil {
		for _, field := range entry.Catalog.Fields {
			if existing[field] {
				continue
			}
			existing[field] = true
			fields = append(fields, field)
		}
	}
	add := func(field modelmetadata.MetadataField) {
		if existing[string(field)] {
			return
		}
		existing[string(field)] = true
		fields = append(fields, string(field))
	}
	if patch.Int != nil {
		entry.Int = patch.Int
		add(modelmetadata.FieldInt)
	}
	if patch.TPS != nil {
		entry.TPS = patch.TPS
		add(modelmetadata.FieldTPS)
	}
	if patch.Description != nil {
		entry.Description = *patch.Description
		add(modelmetadata.FieldDescription)
	}
	if patch.Cost != nil {
		if patch.Cost.Input != nil {
			entry.InputCost = *patch.Cost.Input
			add(modelmetadata.FieldCostInput)
		}
		if patch.Cost.Output != nil {
			entry.OutputCost = *patch.Cost.Output
			add(modelmetadata.FieldCostOutput)
		}
		if patch.Cost.CacheRead != nil {
			entry.CacheReadCost = *patch.Cost.CacheRead
			add(modelmetadata.FieldCostCacheRead)
		}
		if patch.Cost.CacheWrite != nil {
			entry.CacheWriteCost = *patch.Cost.CacheWrite
			add(modelmetadata.FieldCostCacheWrite)
		}
	}
	if patch.ContextWindow != nil {
		entry.ContextWindow = *patch.ContextWindow
		add(modelmetadata.FieldContextWindow)
	}
	if patch.MaxTokens != nil {
		entry.MaxTokens = *patch.MaxTokens
		add(modelmetadata.FieldMaxTokens)
	}
	if patch.Reasoning != nil && *patch.Reasoning && !entry.Reasoning {
		entry.Reasoning = true
		add(modelmetadata.FieldReasoning)
	}
	if patch.PromptCache != nil {
		entry.PromptCache = patch.PromptCache
		add(modelmetadata.FieldPromptCache)
	}
	if patch.ThinkingLevelMap != nil && (allowEffortMap || (patch.Reasoning != nil && *patch.Reasoning)) {
		entry.ThinkingLevelMap = patch.ThinkingLevelMap
		add(modelmetadata.FieldThinkingLevelMap)
	}
	entry.Catalog = &ai.ModelCatalogProvenance{Source: string(patch.Source), Provider: patch.Provider, ModelID: patch.ModelID, Fields: fields}
}
