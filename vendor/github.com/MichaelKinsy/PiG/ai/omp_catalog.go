package ai

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"sync"
)

// ompCatalogFiles is OMP 18.14.2's complete bundled catalog, imported from the
// published MIT-licensed pi-catalog package. Providers decompress independently;
// exact-model startup must not parse thousands of unrelated catalog entries.
//
//go:embed omp_catalog_data/*.json.gz omp_catalog_data/provider-auth.json
var ompCatalogFiles embed.FS

type ompCatalogMetric struct {
	Provider string   `json:"provider"`
	ID       string   `json:"id"`
	Int      *float64 `json:"int,omitempty"`
	TPS      *float64 `json:"tps,omitempty"`
}

var ompCatalogLoads sync.Map
var ompMetricAliases = sync.OnceValue(func() map[string][]ompCatalogMetric {
	compressed, err := ompCatalogFiles.ReadFile("omp_catalog_data/metric-aliases.json.gz")
	if err != nil {
		panic(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		panic(err)
	}
	data, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		panic(err)
	}
	var index map[string][]ompCatalogMetric
	if err = json.Unmarshal(data, &index); err != nil {
		panic(err)
	}
	return index
})
var ompCatalogEnvKeys = sync.OnceValue(func() map[string][]string {
	raw, err := ompCatalogFiles.ReadFile("omp_catalog_data/provider-auth.json")
	if err != nil {
		panic(err)
	}
	var keys map[string][]string
	if err = json.Unmarshal(raw, &keys); err != nil {
		panic(err)
	}
	return keys
})

// OMPProviderEnvKeys returns the published provider credential names, not guessed
// provider-to-environment conversions. Returned slices are detached from catalog state.
func OMPProviderEnvKeys(provider string) []string { return slices.Clone(ompCatalogEnvKeys()[provider]) }

// ompCatalog contains immutable rows shared by registry readers. Chat transport
// descriptors remain separate from non-chat catalog descriptors, so a speech or
// embedding entry can never accidentally become a chat completion model.
type ompCatalog struct {
	chat     []GeneratedModel
	chatByID map[string]GeneratedModel
	other    []*Model
	models   map[string]*Model
}

// ompCatalogRow is the current OMP wire schema at the import boundary. Unknown
// fields remain in Record for lossless extension/RPC catalog serialization.
type ompCatalogRow struct {
	ID, Name, Provider, API, BaseURL, Kind, Description string
	Input                                               []string
	Headers                                             map[string]string
	Reasoning                                           bool
	LongContextWindowUncapped                           bool
	ContextWindow, MaxTokens                            int
	Int, TPS, Priority                                  *float64
	Compat                                              *ModelCompat
	InputLimits                                         *ModelInputLimits
	SamplingParams                                      map[string]any
	Cost                                                struct {
		ModelCost
		LongContext *struct {
			ModelCost
			InputThreshold float64
		}
	}
	Thinking         struct{ Efforts []string }
	ThinkingLevelMap ThinkingLevelMap
}

// OMPCatalogProviders returns bundled provider IDs without decoding model rows.
func OMPCatalogProviders() []string {
	entries, _ := ompCatalogFiles.ReadDir("omp_catalog_data")
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json.gz") && entry.Name() != "metric-aliases.json.gz" {
			out = append(out, strings.TrimSuffix(entry.Name(), ".json.gz"))
		}
	}
	return out
}

// HasOMPCatalog reports ownership without creating caches for unknown IDs.
func HasOMPCatalog(provider string) bool {
	if strings.ContainsAny(provider, "/\\") || provider == "" || provider == "metric-aliases" {
		return false
	}
	file, err := ompCatalogFiles.Open("omp_catalog_data/" + provider + ".json.gz")
	if err != nil {
		return false
	}
	file.Close()
	return true
}

// loadOMPCatalog decodes a trusted, generated provider member exactly once.
// Invalid checked-in data is an invariant violation and fails loudly, rather
// than silently falling back to a different catalog and hiding missing models.
func loadOMPCatalog(provider string) *ompCatalog {
	if !HasOMPCatalog(provider) {
		return nil
	}
	loader, _ := ompCatalogLoads.LoadOrStore(provider, sync.OnceValue(func() *ompCatalog {
		compressed, err := ompCatalogFiles.ReadFile("omp_catalog_data/" + provider + ".json.gz")
		if err != nil {
			panic(err)
		}
		reader, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			panic(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			panic(err)
		}
		var records []json.RawMessage
		if err = json.Unmarshal(data, &records); err != nil {
			panic(err)
		}
		catalog := &ompCatalog{models: map[string]*Model{}, chatByID: map[string]GeneratedModel{}}
		for _, record := range records {
			var row ompCatalogRow
			if err = json.Unmarshal(record, &row); err != nil {
				panic(err)
			}
			if row.Provider != provider || row.ID == "" {
				panic("invalid OMP catalog identity")
			}
			kind := row.Kind
			if kind == "" {
				kind = "chat"
			}
			api := API(row.API)
			if api == "openrouter" {
				api = APIOpenAICompletions
			}
			levels := row.ThinkingLevelMap
			if len(row.Thinking.Efforts) > 0 {
				levels = ThinkingLevelMap{}
				for _, effort := range []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"} {
					if slices.Contains(row.Thinking.Efforts, effort) {
						value := effort
						levels[ModelThinkingLevel(effort)] = &value
					} else {
						levels[ModelThinkingLevel(effort)] = nil
					}
				}
			}
			tiers := row.Cost.Tiers
			if long := row.Cost.LongContext; long != nil {
				tiers = append(slices.Clone(tiers), CostTier{InputTokensAbove: long.InputThreshold, InputCostPer1M: long.Input, OutputCostPer1M: long.Output, CacheReadCostPer1M: long.CacheRead, CacheWriteCostPer1M: long.CacheWrite})
			}
			// Match OMP's default standard-pricing window. The full advertised
			// window remains in Record for explicit extended-context policy.
			if long := row.Cost.LongContext; long != nil && !row.LongContextWindowUncapped && long.InputThreshold > 0 && row.ContextWindow > int(long.InputThreshold) {
				row.ContextWindow = int(long.InputThreshold)
			}
			generated := GeneratedModel{ID: row.ID, Provider: provider, DisplayName: row.Name, API: api, BaseURL: row.BaseURL, Headers: row.Headers, Compat: row.Compat, Reasoning: row.Reasoning, Capabilities: row.Input, ContextWindow: row.ContextWindow, MaxOutputTokens: row.MaxTokens, InputCostPerMTokens: row.Cost.Input, OutputCostPerMTokens: row.Cost.Output, CacheReadCost: row.Cost.CacheRead, CacheWriteCost: row.Cost.CacheWrite, Tiers: tiers, ThinkingLevelMap: levels, InputLimits: row.InputLimits, SamplingParams: row.SamplingParams, Record: string(record)}
			// Do not call ToModel here: that method also projects OMP display metadata.
			model := &Model{Type: ModelType(kind), ID: row.ID, DisplayName: row.Name, Input: row.Input, Capabilities: generated.ToCapabilities(), ThinkingLevelMap: levels, Int: row.Int, TPS: row.TPS, Priority: row.Priority, Description: row.Description, ProviderMeta: ProviderMetadata{ProviderID: provider, API: api, BaseURL: row.BaseURL, Headers: row.Headers, Compat: row.Compat, Reasoning: row.Reasoning}, catalogRecord: string(record)}
			catalog.models[row.ID] = model
			if kind == "chat" {
				catalog.chat = append(catalog.chat, generated)
				catalog.chatByID[row.ID] = generated
			} else {
				catalog.other = append(catalog.other, model)
			}
		}
		catalog.chat = slices.Clip(catalog.chat)
		catalog.other = slices.Clip(catalog.other)
		return catalog
	}))
	return loader.(func() *ompCatalog)()
}

// LookupOMPChatModel resolves one exact OMP-owned provider/id in constant time.
// It returns a detached row value; nested catalog members remain immutable.
func LookupOMPChatModel(provider, id string) (GeneratedModel, bool) {
	catalog := loadOMPCatalog(provider)
	if catalog == nil {
		return GeneratedModel{}, false
	}
	model, ok := catalog.chatByID[id]
	return model, ok
}

var ompChatByID = sync.OnceValue(func() map[string][]GeneratedModel {
	index := map[string][]GeneratedModel{}
	for _, provider := range OMPCatalogProviders() {
		catalog := loadOMPCatalog(provider)
		for _, model := range catalog.chat {
			index[model.ID] = append(index[model.ID], model)
		}
	}
	return index
})

// LookupOMPChatModelByID resolves a bare ID in OMP provider order, matching
// the generated registry's deterministic first-provider behavior.
func LookupOMPChatModelByID(id string) (GeneratedModel, bool) {
	rows := ompChatByID()[id]
	if len(rows) == 0 {
		return GeneratedModel{}, false
	}
	return rows[0], true
}

// OMPChatCatalog returns immutable generated chat rows and whether OMP owns the
// provider. An owned empty catalog intentionally suppresses legacy chat entries.
func OMPChatCatalog(provider string) ([]GeneratedModel, bool) {
	catalog := loadOMPCatalog(provider)
	if catalog == nil {
		return nil, false
	}
	return catalog.chat, true
}

// OMPOtherCatalog returns fresh non-chat descriptors. They carry kind identity
// for catalog/role management, never a chat backend. Existing native typed
// runners remain authoritative when they implement the same provider/model ID.
func OMPOtherCatalog(provider string) []AnyModel {
	catalog := loadOMPCatalog(provider)
	if catalog == nil {
		return nil
	}
	out := make([]AnyModel, 0, len(catalog.other))
	for _, model := range catalog.other {
		out = append(out, new(*model))
	}
	return out
}

// OMPCatalogModel returns a fresh descriptor for a bundled model. Nested catalog
// metadata remains shared and read-only. Provider configuration and extension
// overrides still apply after this base.
func OMPCatalogModel(provider, id string) *Model {
	catalog := loadOMPCatalog(provider)
	if catalog == nil {
		return nil
	}
	model := catalog.models[id]
	if model == nil {
		return nil
	}
	return new(*model)
}

// projectOMPModelMetrics fills fields omitted by the old generated Go type.
// Date-stamped releases may inherit missing scores from the same base model,
// preferring the provider's own row before other catalog observations. Provider
// identity and the date-stripped model family are both required; this avoids
// borrowing scores across a distinct model variant.
func projectOMPModelMetrics(model *Model) *Model {
	source := OMPCatalogModel(model.ProviderID(), model.ID)
	if source == nil {
		return model
	}
	if source.Priority != nil {
		model.Priority = new(*source.Priority)
	}
	model.Description = source.Description
	provenance := map[string]string{}
	if source.Int != nil {
		model.Int = new(*source.Int)
		provenance["int"] = source.ProviderID() + "/" + source.ID
	}
	if source.TPS != nil {
		model.TPS = new(*source.TPS)
		provenance["tps"] = source.ProviderID() + "/" + source.ID
	}
	if source.Description != "" {
		provenance["description"] = source.ProviderID() + "/" + source.ID
	}
	// OMP's final metrics pass shares complete exact-ID observations before
	// trying a date-stripped alias. Keep the pair together: an observed speed
	// and intelligence score describe the same catalog entry.
	if (model.Int == nil || model.TPS == nil) && stripCatalogReleaseDate(model.ID) == "" {
		for _, candidate := range ompMetricAliases()[ompMetricAliasKey(model.ID)] {
			if !strings.EqualFold(candidate.ID, model.ID) || candidate.Provider == model.ProviderID() || candidate.Int == nil || candidate.TPS == nil || *candidate.TPS <= 0 {
				continue
			}
			model.Int = new(*candidate.Int)
			model.TPS = new(*candidate.TPS)
			provenance["int"] = candidate.Provider + "/" + candidate.ID
			provenance["tps"] = candidate.Provider + "/" + candidate.ID
			break
		}
	}
	if baseID := stripCatalogReleaseDate(model.ID); baseID != "" && source.Int == nil && source.TPS == nil {
		if base := OMPCatalogModel(model.ProviderID(), baseID); base != nil {
			if model.Int == nil && base.Int != nil {
				model.Int = new(*base.Int)
				provenance["int"] = base.ProviderID() + "/" + base.ID
			}
			if model.TPS == nil && base.TPS != nil {
				model.TPS = new(*base.TPS)
				provenance["tps"] = base.ProviderID() + "/" + base.ID
			}
		}
		for _, candidate := range ompMetricAliases()[ompMetricAliasKey(baseID)] {
			if candidate.Provider == model.ProviderID() {
				continue
			}
			if model.Int == nil && candidate.Int != nil {
				model.Int = new(*candidate.Int)
				provenance["int"] = candidate.Provider + "/" + candidate.ID
			}
			if model.TPS == nil && candidate.TPS != nil {
				model.TPS = new(*candidate.TPS)
				provenance["tps"] = candidate.Provider + "/" + candidate.ID
			}
			if model.Int != nil && model.TPS != nil {
				break
			}
		}
	}
	if len(provenance) > 0 {
		fields := []string{}
		for _, field := range []string{"int", "tps", "description"} {
			if provenance[field] != "" {
				fields = append(fields, field)
			}
		}
		first := provenance[fields[0]]
		provider, sourceID, _ := strings.Cut(first, "/")
		model.Catalog = &ModelCatalogProvenance{Source: "bundled", Provider: provider, ModelID: sourceID, Fields: fields}
	}
	return model
}

// ompMetricAliasKey groups only the unqualified model name; callers still require
// an exact date-stripped alias before sharing a score.
func ompMetricAliasKey(id string) string {
	if slash := strings.LastIndexByte(id, '/'); slash >= 0 {
		id = id[slash+1:]
	}
	return strings.ToLower(id)
}

// stripCatalogReleaseDate recognizes the two date suffixes in OMP model IDs and
// returns the exact base ID. Other numeric variants remain distinct.
func stripCatalogReleaseDate(id string) string {
	dash := strings.LastIndexByte(id, '-')
	if dash < 0 {
		return ""
	}
	suffix := id[dash+1:]
	if len(suffix) == 8 && allASCIIDigits(suffix) {
		return id[:dash]
	}
	if dash >= 8 && len(suffix) == 2 && allASCIIDigits(suffix) && id[dash-3] == '-' && id[dash-8] == '-' && allASCIIDigits(id[dash-7:dash-3]) && allASCIIDigits(id[dash-2:dash]) {
		return id[:dash-8]
	}
	return ""
}

// allASCIIDigits rejects empty and non-ASCII date parts; numeric model suffixes
// are stripped only after their length and surrounding date geometry agree.
func allASCIIDigits(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

// OMPKeylessCatalog identifies OMP's built-in local and search runners. Listing
// their typed catalogs does not execute inference, download weights, or search.
func OMPKeylessCatalog(provider string) bool { return provider == "local" || provider == "web" }
