package modelmetadata

// Ports the rest of packages/coding-agent/src/core/model-metadata.ts: catalog patch resolution and
// safe application to a model.

import (
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
)

// Mode selects how much of a row may be inherited.
type Mode string

const (
	// ModeMetrics fills only the model-level scores and the blurb; bundled providers keep their own
	// curated prices, limits and thinking maps.
	ModeMetrics Mode = "metrics"
	// ModeFull fills every missing field.
	ModeFull Mode = "full"
)

// ResolveOptions configure one patch resolution.
type ResolveOptions struct {
	Mode Mode
	// AllowEffortMap reports that the target API actually transmits reasoning effort, so an inherited
	// effort ladder is functional rather than display-only.
	AllowEffortMap bool
}

// ExplicitMetadataFields lists the fields a models.json entry set explicitly; those never come from
// the catalog.
func ExplicitMetadataFields(definition ExplicitDefinition) FieldSet {
	explicit := FieldSet{}
	if definition.Int != nil {
		explicit[FieldInt] = true
	}
	if definition.TPS != nil {
		explicit[FieldTPS] = true
	}
	if definition.Description != nil {
		explicit[FieldDescription] = true
	}
	if definition.Reasoning != nil {
		explicit[FieldReasoning] = true
	}
	if definition.ThinkingLevelMap != nil {
		explicit[FieldThinkingLevelMap] = true
	}
	if definition.PromptCache != nil {
		explicit[FieldPromptCache] = true
	}
	if definition.ContextWindow != nil {
		explicit[FieldContextWindow] = true
	}
	if definition.MaxTokens != nil {
		explicit[FieldMaxTokens] = true
	}
	if definition.Cost != nil {
		if definition.Cost.Input != nil {
			explicit[FieldCostInput] = true
		}
		if definition.Cost.Output != nil {
			explicit[FieldCostOutput] = true
		}
		if definition.Cost.CacheRead != nil {
			explicit[FieldCostCacheRead] = true
		}
		if definition.Cost.CacheWrite != nil {
			explicit[FieldCostCacheWrite] = true
		}
	}
	return explicit
}

// ExplicitDefinition is the fields of a models.json model entry that pin metadata values.
type ExplicitDefinition struct {
	Int              *float64
	TPS              *float64
	Description      *string
	Reasoning        *bool
	ThinkingLevelMap ai.ThinkingLevelMap
	PromptCache      ai.ModelPromptCache
	ContextWindow    *int
	MaxTokens        *int
	Cost             *CostRates
}

// CostRates is a partial rate set as an entry wrote it.
type CostRates struct {
	Input      *float64
	Output     *float64
	CacheRead  *float64
	CacheWrite *float64
}

// ResolvePatch resolves the catalog patch for one model; ok is false when nothing matches.
//
// explicit names the fields the user configured, which always win. Values are only read from the
// best-ranked row; int/tps may use the wider consensus because they score the model rather than one
// reseller's price list.
func (s *Service) ResolvePatch(modelID, modelName string, explicit FieldSet, options ResolveOptions) (Patch, bool) {
	if options.Mode == "" {
		options.Mode = ModeFull
	}
	s.mu.Lock()
	candidates := indexedCandidates(s.index, modelID, modelName)
	source := s.source
	s.mu.Unlock()
	if len(candidates) == 0 {
		if options.Mode == ModeMetrics {
			return Patch{}, false
		}
		reference, hasReference := s.reference(modelID, modelName)
		return s.bundledPatch(reference, hasReference, explicit, options)
	}
	row := candidates[0]
	patch := Patch{
		Source:   SourceCached,
		Provider: row.Provider,
		ModelID:  row.ID,
	}
	if source == SourceNetwork {
		patch.Source = SourceNetwork
	}
	// int/tps score the model, not one host's price list: when the chosen row lacks them, fall back
	// to the consensus of every matching row.
	if !explicit.Has(FieldInt) || !explicit.Has(FieldTPS) {
		consensus := ConsensusCatalogMetrics(candidates)
		if !explicit.Has(FieldInt) && consensus.Int != nil {
			patch.Int = consensus.Int
			patch.Fields = append(patch.Fields, FieldInt)
		}
		if !explicit.Has(FieldTPS) && consensus.TPS != nil {
			patch.TPS = consensus.TPS
			patch.Fields = append(patch.Fields, FieldTPS)
		}
	}
	if row.Description != "" && !explicit.Has(FieldDescription) {
		description := row.Description
		patch.Description = &description
		patch.Fields = append(patch.Fields, FieldDescription)
	}
	// Bundled providers keep their own curated prices and limits; only the model-level scores and
	// the blurb cross over.
	if options.Mode == ModeMetrics {
		if len(patch.Fields) == 0 {
			return Patch{}, false
		}
		return patch, true
	}
	// Bundled references can supply cache policy and reasoning capabilities,
	// never selector metrics. Resolve them only for full custom-model patches.
	reference, hasReference := s.reference(modelID, modelName)
	if row.Cost != nil {
		cost := PatchCost{}
		any := false
		if row.Cost.Input != nil && !explicit.Has(FieldCostInput) {
			cost.Input = row.Cost.Input
			any = true
			patch.Fields = append(patch.Fields, FieldCostInput)
		}
		if row.Cost.Output != nil && !explicit.Has(FieldCostOutput) {
			cost.Output = row.Cost.Output
			any = true
			patch.Fields = append(patch.Fields, FieldCostOutput)
		}
		if row.Cost.CacheRead != nil && !explicit.Has(FieldCostCacheRead) {
			cost.CacheRead = row.Cost.CacheRead
			any = true
			patch.Fields = append(patch.Fields, FieldCostCacheRead)
		}
		if row.Cost.CacheWrite != nil && !explicit.Has(FieldCostCacheWrite) {
			cost.CacheWrite = row.Cost.CacheWrite
			any = true
			patch.Fields = append(patch.Fields, FieldCostCacheWrite)
		}
		if any {
			patch.Cost = &cost
		}
	}
	if row.ContextWindow != nil && !explicit.Has(FieldContextWindow) {
		patch.ContextWindow = row.ContextWindow
		patch.Fields = append(patch.Fields, FieldContextWindow)
	}
	if row.MaxTokens != nil && !explicit.Has(FieldMaxTokens) {
		patch.MaxTokens = row.MaxTokens
		patch.Fields = append(patch.Fields, FieldMaxTokens)
	}
	// The public catalog publishes no cache lifetimes, so the only honest source is a bundled model
	// that declares them.
	if !explicit.Has(FieldPromptCache) && hasReference && reference.PromptCache != nil {
		patch.PromptCache = clonePromptCache(reference.PromptCache)
		patch.Fields = append(patch.Fields, FieldPromptCache)
	}
	// An id-only custom entry would otherwise be stuck at reasoning:false while its effort map is
	// unreadable by the transport. Inherit capability only for a safe match on an API that actually
	// sends effort; an explicit false always wins.
	inheritReasoning := options.AllowEffortMap && options.Mode == ModeFull && !explicit.Has(FieldReasoning)
	referenceReasons := hasReference && reference.Reasoning
	rowReasons := row.Reasoning != nil && *row.Reasoning
	if inheritReasoning && (rowReasons || referenceReasons) {
		reasoning := true
		patch.Reasoning = &reasoning
		patch.Fields = append(patch.Fields, FieldReasoning)
	}
	if options.AllowEffortMap && !explicit.Has(FieldThinkingLevelMap) {
		// The public row's published tiers win; when it publishes none, a bundled canonical model may
		// declare the ladder explicitly (deepseek-flash low/high/max). Neither path guesses.
		var ladder ai.ThinkingLevelMap
		switch {
		case len(row.Efforts) > 0 || row.SupportsOffEffort:
			ladder = EffortMapFromCatalog(row)
		case hasReference && reference.ThinkingLevelMap != nil:
			ladder = InheritanceLadder(reference.ThinkingLevelMap)
		}
		if ladder != nil {
			patch.ThinkingLevelMap = ladder
			patch.Fields = append(patch.Fields, FieldThinkingLevelMap)
		}
	}
	if len(patch.Fields) == 0 {
		return Patch{}, false
	}
	return patch, true
}

// bundledPatch is the patch a bundled declaration alone can supply.
//
// Bundled declarations describe custom endpoints only; a built-in provider already carries its own
// curated values.
func (s *Service) bundledPatch(reference Reference, hasReference bool, explicit FieldSet, options ResolveOptions) (Patch, bool) {
	if !hasReference || options.Mode != ModeFull {
		return Patch{}, false
	}
	patch := Patch{Source: SourceBundled, Provider: reference.Provider, ModelID: reference.ID}
	if options.AllowEffortMap && reference.Reasoning && !explicit.Has(FieldReasoning) {
		reasoning := true
		patch.Reasoning = &reasoning
		patch.Fields = append(patch.Fields, FieldReasoning)
	}
	if options.AllowEffortMap && reference.ThinkingLevelMap != nil && !explicit.Has(FieldThinkingLevelMap) {
		patch.ThinkingLevelMap = InheritanceLadder(reference.ThinkingLevelMap)
		patch.Fields = append(patch.Fields, FieldThinkingLevelMap)
	}
	if reference.PromptCache != nil && !explicit.Has(FieldPromptCache) {
		patch.PromptCache = clonePromptCache(reference.PromptCache)
		patch.Fields = append(patch.Fields, FieldPromptCache)
	}
	if len(patch.Fields) == 0 {
		return Patch{}, false
	}
	return patch, true
}

// reference is the bundled declaration of the same identity, when one exists.
//
// Several hosts bundle the same family; the family's own first-party rows win, then a row whose id
// names the same release date, then the ladder with the most published tiers. Nothing is synthesized
// when no row matches.
func (s *Service) reference(modelID, modelName string) (Reference, bool) {
	target, ok := VariantKeyFor(modelID, modelName)
	if !ok {
		return Reference{}, false
	}
	type candidate struct {
		reference Reference
		key       *VariantKey
	}
	var matches []candidate
	for _, reference := range s.references() {
		// Id and name are merged the same way catalog rows are: a legacy id keeps its version facts
		// when the published name carries them.
		key, ok := VariantKeyFor(reference.ID, reference.Name)
		if !ok || key.Family != target.Family {
			continue
		}
		if !VariantKeysMatch(target, key) {
			continue
		}
		matches = append(matches, candidate{reference: reference, key: key})
	}
	if len(matches) == 0 {
		return Reference{}, false
	}
	tiers := func(reference Reference) int {
		count := 0
		for _, mapped := range reference.ThinkingLevelMap {
			if mapped != nil {
				count++
			}
		}
		return count
	}
	best := matches[0]
	for _, other := range matches[1:] {
		if referenceCandidateLess(other.reference, other.key, best.reference, best.key, target, tiers) {
			best = other
		}
	}
	return best.reference, true
}

// referenceCandidateLess orders two bundled declarations for one target.
func referenceCandidateLess(left Reference, leftKey *VariantKey, right Reference, rightKey *VariantKey, target *VariantKey, tiers func(Reference) int) bool {
	leftCanonical := IsCanonicalProvider(left.Provider, target.Family)
	rightCanonical := IsCanonicalProvider(right.Provider, target.Family)
	if leftCanonical != rightCanonical {
		return leftCanonical
	}
	leftDate := leftKey != nil && leftKey.HasDate && target.HasDate && leftKey.Date == target.Date
	rightDate := rightKey != nil && rightKey.HasDate && target.HasDate && rightKey.Date == target.Date
	if leftDate != rightDate {
		return leftDate
	}
	if leftTiers, rightTiers := tiers(left), tiers(right); leftTiers != rightTiers {
		return leftTiers < rightTiers
	}
	return left.Provider < right.Provider
}

// InheritanceLadder reuses a bundled ladder for a custom endpoint.
//
// The positive tiers are published facts about the model. The off value is the host's own disable
// spelling ("none", null), which this endpoint never declared, so it is dropped: off then sends no
// reasoning parameters, which every OpenAI-compatible host accepts.
func InheritanceLadder(mapping ai.ThinkingLevelMap) ai.ThinkingLevelMap {
	ladder := make(ai.ThinkingLevelMap, len(mapping))
	for level, value := range mapping {
		if level == ai.ThinkingOff {
			continue
		}
		ladder[level] = value
	}
	return ladder
}

// EffortMapFromCatalog turns a row's published tiers into a thinking map; unsupported levels stay
// nil.
func EffortMapFromCatalog(row ai.ModelCatalogMetadataRow) ai.ThinkingLevelMap {
	ladder := ai.ThinkingLevelMap{
		ai.ThinkingMinimal: nil,
		ai.ThinkingLow:     nil,
		ai.ThinkingMedium:  nil,
		ai.ThinkingHigh:    nil,
		ai.ThinkingXHigh:   nil,
		ai.ThinkingMax:     nil,
	}
	for _, effort := range row.Efforts {
		if effort == "none" {
			continue
		}
		level := ai.ModelThinkingLevel(effort)
		if _, known := ladder[level]; !known {
			continue
		}
		value := effort
		ladder[level] = &value
	}
	if row.SupportsOffEffort {
		off := "none"
		ladder[ai.ThinkingOff] = &off
	}
	return ladder
}

// ModelSendsEffort reports whether the API layer actually sends thinking effort for this model, so an
// inherited map is not decoration.
func ModelSendsEffort(api string, supportsReasoningEffort *bool) bool {
	// Only OpenAI-compatible transports read that flag; the others always route effort.
	if strings.HasPrefix(api, "openai-") || api == string(ai.APIAzureOpenAIResponses) || api == "openrouter" {
		if supportsReasoningEffort != nil && !*supportsReasoningEffort {
			return false
		}
	}
	switch ai.API(api) {
	case ai.APIOpenAICompletions,
		ai.APIOpenAIResponses,
		ai.APIAzureOpenAIResponses,
		ai.APIOpenAICodexResponses,
		ai.APIAnthropicMessages,
		ai.APIBedrockConverseStream,
		ai.APIGoogleGenerativeAI,
		ai.APIGoogleVertex,
		"google-gemini-cli",
		ai.APIMistralConversations,
		"openrouter":
		return true
	default:
		return false
	}
}

// PatchableModel is the structural view of the fields a patch reads and writes.
type PatchableModel struct {
	Int              *float64
	TPS              *float64
	Description      *string
	Cost             ai.ModelCost
	ContextWindow    int
	MaxTokens        int
	Reasoning        bool
	ThinkingLevelMap ai.ThinkingLevelMap
	PromptCache      ai.ModelPromptCache
	Catalog          *ai.ModelCatalogProvenance
}

// ApplyMetadataPatch applies a resolved patch to one model without touching identity, routing, auth,
// headers, or explicit values. Callers exclude configured fields from the patch, so an explicit 0,
// false, or partial cost block always survives.
//
// reasoning, input and effort tiers are never taken from a fuzzy identity match: they change what the
// endpoint is asked to do, and a reseller's row is not evidence that a custom deployment accepts
// images or a given effort.
func ApplyMetadataPatch(model PatchableModel, patch Patch, inheritEfforts bool) (PatchableModel, bool) {
	fields := []MetadataField{}
	if patch.Int != nil {
		model.Int = patch.Int
		fields = append(fields, FieldInt)
	}
	if patch.TPS != nil {
		model.TPS = patch.TPS
		fields = append(fields, FieldTPS)
	}
	if patch.Description != nil {
		model.Description = patch.Description
		fields = append(fields, FieldDescription)
	}
	if patch.Cost != nil {
		if patch.Cost.Input != nil {
			model.Cost.Input = *patch.Cost.Input
			fields = append(fields, FieldCostInput)
		}
		if patch.Cost.Output != nil {
			model.Cost.Output = *patch.Cost.Output
			fields = append(fields, FieldCostOutput)
		}
		if patch.Cost.CacheRead != nil {
			model.Cost.CacheRead = *patch.Cost.CacheRead
			fields = append(fields, FieldCostCacheRead)
		}
		if patch.Cost.CacheWrite != nil {
			model.Cost.CacheWrite = *patch.Cost.CacheWrite
			fields = append(fields, FieldCostCacheWrite)
		}
	}
	if patch.ContextWindow != nil {
		model.ContextWindow = *patch.ContextWindow
		fields = append(fields, FieldContextWindow)
	}
	if patch.MaxTokens != nil {
		model.MaxTokens = *patch.MaxTokens
		fields = append(fields, FieldMaxTokens)
	}
	reasoningInjected := false
	if patch.Reasoning != nil && *patch.Reasoning && !model.Reasoning {
		model.Reasoning = true
		reasoningInjected = true
		fields = append(fields, FieldReasoning)
	}
	if patch.PromptCache != nil {
		model.PromptCache = clonePromptCache(patch.PromptCache)
		fields = append(fields, FieldPromptCache)
	}
	if patch.ThinkingLevelMap != nil && (inheritEfforts || reasoningInjected) {
		copied := make(ai.ThinkingLevelMap, len(patch.ThinkingLevelMap))
		for level, value := range patch.ThinkingLevelMap {
			copied[level] = value
		}
		model.ThinkingLevelMap = copied
		fields = append(fields, FieldThinkingLevelMap)
	}
	if len(fields) == 0 {
		return model, false
	}
	// Merge with earlier enrichment: a later metrics-only pass must not erase the full field list an
	// earlier pass recorded.
	applied := []MetadataField{}
	seen := map[MetadataField]bool{}
	if model.Catalog != nil {
		for _, field := range model.Catalog.Fields {
			name := MetadataField(field)
			if seen[name] {
				continue
			}
			seen[name] = true
			applied = append(applied, name)
		}
	}
	for _, field := range fields {
		if seen[field] {
			continue
		}
		seen[field] = true
		applied = append(applied, field)
	}
	names := make([]string, len(applied))
	for i, field := range applied {
		names[i] = string(field)
	}
	model.Catalog = &ai.ModelCatalogProvenance{Source: string(patch.Source), Provider: patch.Provider, ModelID: patch.ModelID, Fields: names}
	return model, true
}

// clonePromptCache copies a lifetime map so a patch cannot alias catalog state.
func clonePromptCache(cache ai.ModelPromptCache) ai.ModelPromptCache {
	if cache == nil {
		return nil
	}
	copied := make(ai.ModelPromptCache, len(cache))
	for tier, lifetime := range cache {
		copied[tier] = lifetime
	}
	return copied
}
