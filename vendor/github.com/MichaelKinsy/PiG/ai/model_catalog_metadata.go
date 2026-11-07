package ai

// Ports packages/ai/src/model-catalog-metadata.ts: the shared shape of the public model-metadata
// catalog (catalog.stencil.so/models.json, the source OMP reads through fetchWellKnownModels) and a
// parser that prunes it to the fields model selection displays or inherits.
//
// The payload is a providers map ({ deepseek: { models: { id: row } } }), where each row carries
// int, optional tps, cost, limit, modalities, reasoning, reasoning_options and status. Rows are
// normalized here so the coding agent stores one compact snapshot and matches it without touching
// wire shapes again. The row shape is validated at this trust boundary; a row that does not validate
// is skipped rather than guessed at.

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
)

// ModelCatalogMetadataVersion is the snapshot schema version; bump it when row semantics change so
// old caches are discarded.
const ModelCatalogMetadataVersion = 1

// modelCatalogEffortLevels are the canonical effort tiers a catalog row may publish, in order
// (THINKING_LEVELS of model-schema.ts). "none" is not a tier: it is the explicit disable effort.
var modelCatalogEffortLevels = []string{"minimal", "low", "medium", "high", "xhigh", "max"}

// ModelCatalogCostRates is one published rate set, in USD per million tokens.
type ModelCatalogCostRates struct {
	Input      *float64 `json:"input,omitempty"`
	Output     *float64 `json:"output,omitempty"`
	CacheRead  *float64 `json:"cacheRead,omitempty"`
	CacheWrite *float64 `json:"cacheWrite,omitempty"`
}

// ModelCatalogMetadataRow is one catalog row, pruned to fields Pi can apply to a model.
type ModelCatalogMetadataRow struct {
	// Provider is the provider id from the catalog, for example "deepseek" or "openrouter".
	Provider string `json:"provider"`
	// ID is the model id as the catalog provider publishes it.
	ID string `json:"id"`
	// Name is the published display name, when the row has one.
	Name string `json:"name,omitempty"`
	// Int is the intelligence index. A row without a score omits it; 0 is a real score.
	Int *float64 `json:"int,omitempty"`
	// TPS is the estimated output speed in tokens per second.
	TPS *float64 `json:"tps,omitempty"`
	// Description is the published one-line blurb.
	Description string `json:"description,omitempty"`
	// Input holds the declared input modalities, restricted to the ones Pi supports.
	Input []string `json:"input,omitempty"`
	// Reasoning reports whether the row declares the model reasons.
	Reasoning *bool `json:"reasoning,omitempty"`
	// Efforts are the effort tiers the host accepts, in canonical none-to-max order.
	Efforts []string `json:"efforts,omitempty"`
	// SupportsOffEffort is true when the host publishes an explicit disable effort.
	SupportsOffEffort bool `json:"supportsOffEffort,omitempty"`
	// Cost is the published rate set.
	Cost *ModelCatalogCostRates `json:"cost,omitempty"`
	// ContextWindow is limit.context, else limit.input.
	ContextWindow *int `json:"contextWindow,omitempty"`
	// MaxTokens is limit.output.
	MaxTokens *int `json:"maxTokens,omitempty"`
	// Status is "deprecated", "beta" or "preview" when the source marks the row.
	Status string `json:"status,omitempty"`
}

// ModelCatalogMetadataSnapshot is a durable snapshot of the public metadata catalog.
type ModelCatalogMetadataSnapshot struct {
	Version int `json:"version"`
	// FetchedAt is when the rows were last downloaded from the origin.
	FetchedAt int64 `json:"fetchedAt"`
	// CheckedAt is when the origin was last successfully revalidated (a 304 moves this).
	CheckedAt int64 `json:"checkedAt"`
	// ETag is the origin validator for conditional GETs.
	ETag string                    `json:"etag,omitempty"`
	Rows []ModelCatalogMetadataRow `json:"rows"`
}

// ModelCatalogProvenance records where a model's catalog metadata came from.
type ModelCatalogProvenance struct {
	// Source is "cached"/"network" for remote catalog snapshots and "bundled" for shipped model data.
	Source string `json:"source"`
	// Provider is the catalog provider whose row matched this model, when the match crossed providers.
	Provider string `json:"provider,omitempty"`
	// ModelID is the catalog row id the fields were read from.
	ModelID string `json:"modelId,omitempty"`
	// Fields lists the fields this enrichment filled, in model order.
	Fields []string `json:"fields"`
}

// catalogWireCost is a decoded cost block's raw members.
type catalogWireCost struct {
	input, output, cacheRead, cacheWrite *float64
}

// catalogWireLimit is a decoded limit block's raw members.
type catalogWireLimit struct {
	context, input, output *int
}

// catalogWireRow is a decoded catalog row's raw members; every optional member is nil when absent
// (or JSON null).
type catalogWireRow struct {
	id               *string
	name             *string
	description      *string
	intelligence     *float64
	tps              *float64
	cost             *catalogWireCost
	limit            *catalogWireLimit
	modalities       *[]string
	reasoning        *bool
	reasoningOptions *[]catalogWireEffortOption
	status           *string
}

// catalogWireEffortOption is one reasoning_options entry.
type catalogWireEffortOption struct {
	kind   string
	values []*string
}

// jsonNumber reads raw as a JSON number.
func jsonNumber(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false
	}
	return value, true
}

// jsonTokenCount reads raw as a JSON number of tokens, which the row schema types as a number but a
// model limit holds as a whole count.
func jsonTokenCount(raw json.RawMessage) (int, bool) {
	value, ok := jsonNumber(raw)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return int(value), true
}

func jsonObjectMembers(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, false
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil || members == nil {
		return nil, false
	}
	return members, true
}

func jsonNullOrAbsent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

func jsonString(raw json.RawMessage) (*string, bool) {
	if jsonNullOrAbsent(raw) {
		return nil, true
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false
	}
	return &value, true
}

func jsonBool(raw json.RawMessage) (*bool, bool) {
	if jsonNullOrAbsent(raw) {
		return nil, true
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false
	}
	return &value, true
}

func jsonStringList(raw json.RawMessage) (*[]string, bool) {
	if jsonNullOrAbsent(raw) {
		return nil, true
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, false
	}
	return &values, true
}

// decodeCatalogWireRow validates one row against the catalog row schema (openObject: unknown
// members are allowed). ok is false when a member has the wrong type, so the caller skips the row.
func decodeCatalogWireRow(raw json.RawMessage) (catalogWireRow, bool) {
	members, ok := jsonObjectMembers(raw)
	if !ok {
		return catalogWireRow{}, false
	}
	var row catalogWireRow
	var valid bool
	if value, present := members["id"]; present {
		if row.id, valid = jsonString(value); !valid {
			return catalogWireRow{}, false
		}
	}
	if value, present := members["name"]; present {
		if row.name, valid = jsonString(value); !valid {
			return catalogWireRow{}, false
		}
	}
	if value, present := members["description"]; present {
		if row.description, valid = jsonString(value); !valid {
			return catalogWireRow{}, false
		}
	}
	if value, present := members["int"]; present {
		if jsonNullOrAbsent(value) {
			row.intelligence = nil
		} else if number, ok := jsonNumber(value); ok {
			row.intelligence = &number
		} else {
			return catalogWireRow{}, false
		}
	}
	if value, present := members["tps"]; present {
		// tps is num({ exclusiveMinimum: 0 }): it must be a number greater than zero.
		if jsonNullOrAbsent(value) {
			row.tps = nil
		} else if number, ok := jsonNumber(value); ok && number > 0 {
			row.tps = &number
		} else {
			return catalogWireRow{}, false
		}
	}
	if value, present := members["cost"]; present {
		if !jsonNullOrAbsent(value) {
			costMembers, ok := jsonObjectMembers(value)
			if !ok {
				return catalogWireRow{}, false
			}
			cost := catalogWireCost{}
			for name, target := range map[string]**float64{"input": &cost.input, "output": &cost.output, "cache_read": &cost.cacheRead, "cache_write": &cost.cacheWrite} {
				member, present := costMembers[name]
				if !present || jsonNullOrAbsent(member) {
					continue
				}
				number, ok := jsonNumber(member)
				if !ok || number < 0 {
					return catalogWireRow{}, false
				}
				*target = &number
			}
			row.cost = &cost
		}
	}
	if value, present := members["limit"]; present {
		if !jsonNullOrAbsent(value) {
			limitMembers, ok := jsonObjectMembers(value)
			if !ok {
				return catalogWireRow{}, false
			}
			limit := catalogWireLimit{}
			for name, target := range map[string]**int{"context": &limit.context, "input": &limit.input, "output": &limit.output} {
				member, present := limitMembers[name]
				if !present || jsonNullOrAbsent(member) {
					continue
				}
				number, ok := jsonNumber(member)
				if !ok || number <= 0 {
					return catalogWireRow{}, false
				}
				integer := int(number)
				*target = &integer
			}
			row.limit = &limit
		}
	}
	if value, present := members["modalities"]; present {
		if !jsonNullOrAbsent(value) {
			modalityMembers, ok := jsonObjectMembers(value)
			if !ok {
				return catalogWireRow{}, false
			}
			if input, present := modalityMembers["input"]; present {
				// modalities.input is optional inside the optional block.
				if input, valid := jsonStringList(input); valid {
					row.modalities = input
				} else {
					return catalogWireRow{}, false
				}
			}
		}
	}
	if value, present := members["reasoning"]; present {
		if row.reasoning, valid = jsonBool(value); !valid {
			return catalogWireRow{}, false
		}
	}
	if value, present := members["reasoning_options"]; present {
		if !jsonNullOrAbsent(value) {
			var options []json.RawMessage
			if err := json.Unmarshal(value, &options); err != nil {
				return catalogWireRow{}, false
			}
			decoded := make([]catalogWireEffortOption, 0, len(options))
			for _, option := range options {
				optionMembers, ok := jsonObjectMembers(option)
				if !ok {
					return catalogWireRow{}, false
				}
				entry := catalogWireEffortOption{}
				if kind, present := optionMembers["type"]; present {
					value, valid := jsonString(kind)
					if !valid {
						return catalogWireRow{}, false
					}
					if value != nil {
						entry.kind = *value
					}
				}
				if values, present := optionMembers["values"]; present && !jsonNullOrAbsent(values) {
					var rawValues []json.RawMessage
					if err := json.Unmarshal(values, &rawValues); err != nil {
						return catalogWireRow{}, false
					}
					for _, rawValue := range rawValues {
						value, valid := jsonString(rawValue)
						if !valid {
							return catalogWireRow{}, false
						}
						entry.values = append(entry.values, value)
					}
				}
				decoded = append(decoded, entry)
			}
			row.reasoningOptions = &decoded
		}
	}
	if value, present := members["status"]; present {
		if row.status, valid = jsonString(value); !valid {
			return catalogWireRow{}, false
		}
	}
	return row, true
}

// catalogEfforts is the effort tiers a row publishes, canonical ordering, with "none" reported
// separately (catalogEfforts of model-catalog-metadata.ts).
func catalogEfforts(options *[]catalogWireEffortOption) (efforts []string, supportsOffEffort bool) {
	published := map[string]bool{}
	if options != nil {
		for _, option := range *options {
			if option.kind != "effort" {
				continue
			}
			for _, value := range option.values {
				if value == nil {
					continue
				}
				switch *value {
				case "none":
					published["none"] = true
				case "minimal", "low", "medium", "high", "xhigh", "max":
					published[*value] = true
				}
			}
		}
	}
	if published["none"] {
		delete(published, "none")
		supportsOffEffort = true
	}
	for _, effort := range modelCatalogEffortLevels {
		if published[effort] {
			efforts = append(efforts, effort)
		}
	}
	return efforts, supportsOffEffort
}

func catalogCost(cost *catalogWireCost) *ModelCatalogCostRates {
	if cost == nil {
		return nil
	}
	rates := ModelCatalogCostRates{Input: cost.input, Output: cost.output, CacheRead: cost.cacheRead, CacheWrite: cost.cacheWrite}
	if rates.Input == nil && rates.Output == nil && rates.CacheRead == nil && rates.CacheWrite == nil {
		return nil
	}
	return &rates
}

// catalogRow normalizes one validated provider row; ok is false when it has nothing to contribute.
func catalogRow(provider, fallbackID string, wire catalogWireRow) (ModelCatalogMetadataRow, bool) {
	id := fallbackID
	if wire.id != nil {
		id = *wire.id
	}
	if id == "" {
		return ModelCatalogMetadataRow{}, false
	}
	row := ModelCatalogMetadataRow{Provider: provider, ID: id}
	if wire.name != nil {
		row.Name = *wire.name
	}
	if wire.description != nil && strings.TrimSpace(*wire.description) != "" {
		row.Description = *wire.description
	}
	row.Int = wire.intelligence
	row.TPS = wire.tps
	row.Reasoning = wire.reasoning
	if wire.modalities != nil {
		for _, modality := range *wire.modalities {
			if modality == ModelInputText || modality == ModelInputImage {
				row.Input = append(row.Input, modality)
			}
		}
	}
	row.Efforts, row.SupportsOffEffort = catalogEfforts(wire.reasoningOptions)
	row.Cost = catalogCost(wire.cost)
	if wire.limit != nil {
		switch {
		case wire.limit.context != nil:
			row.ContextWindow = wire.limit.context
		case wire.limit.input != nil:
			row.ContextWindow = wire.limit.input
		}
		row.MaxTokens = wire.limit.output
	}
	if wire.status != nil {
		row.Status = *wire.status
	}
	// Rows contributing only a name or a status are not worth their snapshot bytes (CATALOG_ROW_FIELDS).
	if row.Int == nil && row.TPS == nil && row.Description == "" && row.Input == nil && row.Reasoning == nil &&
		row.Efforts == nil && !row.SupportsOffEffort && row.Cost == nil && row.ContextWindow == nil && row.MaxTokens == nil {
		return ModelCatalogMetadataRow{}, false
	}
	return row, true
}

// ParseModelCatalogMetadata parses a catalog payload into pruned rows. It accepts the stencil
// payload ({ provider: { models: { id: row } } }), a bare array of rows that each name their
// provider, and { models: [...] }. Rows that fail validation are skipped rather than guessed at.
func ParseModelCatalogMetadata(payload []byte) []ModelCatalogMetadataRow {
	rows := []ModelCatalogMetadataRow{}
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return rows
	}
	push := func(provider, fallbackID string, value json.RawMessage) {
		if provider == "" {
			return
		}
		wire, ok := decodeCatalogWireRow(value)
		if !ok {
			return
		}
		if row, ok := catalogRow(provider, fallbackID, wire); ok {
			rows = append(rows, row)
		}
	}
	pushListed := func(value json.RawMessage) {
		members, ok := jsonObjectMembers(value)
		if !ok {
			return
		}
		providerRaw, present := members["provider"]
		if !present {
			return
		}
		provider, valid := jsonString(providerRaw)
		if !valid || provider == nil {
			return
		}
		push(*provider, "", value)
	}
	switch trimmed[0] {
	case '[':
		var listed []json.RawMessage
		if err := json.Unmarshal(trimmed, &listed); err != nil {
			return rows
		}
		for _, value := range listed {
			pushListed(value)
		}
		return rows
	case '{':
	default:
		return rows
	}
	if members, ok := jsonObjectMembers(trimmed); ok {
		if models, present := members["models"]; present {
			var listed []json.RawMessage
			if err := json.Unmarshal(models, &listed); err == nil {
				for _, value := range listed {
					pushListed(value)
				}
				return rows
			}
		}
		for provider, value := range members {
			// The provider map also carries scalar metadata such as id/env; only models entries are
			// rows. Rows are validated one by one so a single malformed entry never costs the whole
			// provider.
			providerMembers, ok := jsonObjectMembers(value)
			if !ok {
				continue
			}
			models, present := providerMembers["models"]
			if !present {
				continue
			}
			modelMembers, ok := jsonObjectMembers(models)
			if !ok {
				continue
			}
			for id, model := range modelMembers {
				push(provider, id, model)
			}
		}
	}
	return rows
}

// Snapshot values are decoded once, rather than copying raw JSON at each level
// and starting another decoder for every row and scalar. UseNumber preserves the
// open-object boundary: an unknown field with an enormous number is still ignored.
func storedCatalogNumber(value any) (float64, bool) {
	if value == nil {
		return 0, true
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	decoded, err := number.Float64()
	return decoded, err == nil
}

func storedCatalogScalar[T string | bool](value any) (*T, bool) {
	if value == nil {
		return nil, true
	}
	decoded, ok := value.(T)
	if !ok {
		return nil, false
	}
	return &decoded, true
}

// decodeStoredCatalogRow validates one pruned row read from disk (CatalogStoredRowSchema).
func decodeStoredCatalogRow(value any) (ModelCatalogMetadataRow, bool) {
	members, ok := value.(map[string]any)
	if !ok {
		return ModelCatalogMetadataRow{}, false
	}
	providerRaw, present := members["provider"]
	if !present {
		return ModelCatalogMetadataRow{}, false
	}
	provider, ok := storedCatalogScalar[string](providerRaw)
	if !ok || provider == nil {
		return ModelCatalogMetadataRow{}, false
	}
	idRaw, present := members["id"]
	if !present {
		return ModelCatalogMetadataRow{}, false
	}
	id, ok := storedCatalogScalar[string](idRaw)
	if !ok || id == nil {
		return ModelCatalogMetadataRow{}, false
	}
	row := ModelCatalogMetadataRow{Provider: *provider, ID: *id}
	if value, present := members["name"]; present {
		name, valid := storedCatalogScalar[string](value)
		if !valid {
			return ModelCatalogMetadataRow{}, false
		}
		if name != nil {
			row.Name = *name
		}
	}
	if value := members["int"]; value != nil {
		number, valid := storedCatalogNumber(value)
		if !valid {
			return ModelCatalogMetadataRow{}, false
		}
		row.Int = &number
	}
	if value := members["tps"]; value != nil {
		number, valid := storedCatalogNumber(value)
		if !valid || number <= 0 {
			return ModelCatalogMetadataRow{}, false
		}
		row.TPS = &number
	}
	if value, present := members["description"]; present {
		description, valid := storedCatalogScalar[string](value)
		if !valid {
			return ModelCatalogMetadataRow{}, false
		}
		if description != nil {
			row.Description = *description
		}
	}
	if value := members["input"]; value != nil {
		modalities, ok := value.([]any)
		if !ok {
			return ModelCatalogMetadataRow{}, false
		}
		for _, value := range modalities {
			modality, ok := value.(string)
			if !ok || modality != ModelInputText && modality != ModelInputImage {
				return ModelCatalogMetadataRow{}, false
			}
			row.Input = append(row.Input, modality)
		}
	}
	if value, present := members["reasoning"]; present {
		reasoning, valid := storedCatalogScalar[bool](value)
		if !valid {
			return ModelCatalogMetadataRow{}, false
		}
		row.Reasoning = reasoning
	}
	if value := members["efforts"]; value != nil {
		efforts, ok := value.([]any)
		if !ok {
			return ModelCatalogMetadataRow{}, false
		}
		for _, effort := range efforts {
			// A null string element decodes as "" and is rejected below.
			effort, valid := effort.(string)
			if !valid {
				return ModelCatalogMetadataRow{}, false
			}
			// An effort outside the canonical tiers would reach a request as an unsupported value.
			if effort == "none" {
				continue
			}
			known := false
			for _, tier := range modelCatalogEffortLevels {
				if effort == tier {
					known = true
					break
				}
			}
			if !known {
				return ModelCatalogMetadataRow{}, false
			}
			row.Efforts = append(row.Efforts, effort)
		}
	}
	if value, present := members["supportsOffEffort"]; present {
		off, valid := storedCatalogScalar[bool](value)
		if !valid {
			return ModelCatalogMetadataRow{}, false
		}
		row.SupportsOffEffort = off != nil && *off
	}
	if value := members["cost"]; value != nil {
		costMembers, ok := value.(map[string]any)
		if !ok {
			return ModelCatalogMetadataRow{}, false
		}
		cost := ModelCatalogCostRates{}
		for name, target := range map[string]**float64{"input": &cost.Input, "output": &cost.Output, "cacheRead": &cost.CacheRead, "cacheWrite": &cost.CacheWrite} {
			member, present := costMembers[name]
			if !present || member == nil {
				continue
			}
			number, valid := storedCatalogNumber(member)
			if !valid {
				return ModelCatalogMetadataRow{}, false
			}
			*target = &number
		}
		row.Cost = &cost
	}
	if value := members["contextWindow"]; value != nil {
		count, ok := storedCatalogNumber(value)
		number := int(count)
		if !ok || number <= 0 {
			return ModelCatalogMetadataRow{}, false
		}
		row.ContextWindow = &number
	}
	if value := members["maxTokens"]; value != nil {
		count, ok := storedCatalogNumber(value)
		number := int(count)
		if !ok || number <= 0 {
			return ModelCatalogMetadataRow{}, false
		}
		row.MaxTokens = &number
	}
	if value, present := members["status"]; present {
		status, valid := storedCatalogScalar[string](value)
		if !valid {
			return ModelCatalogMetadataRow{}, false
		}
		if status != nil {
			row.Status = *status
		}
	}
	return row, true
}

// ParseModelCatalogMetadataSnapshot validates a persisted snapshot. Disk is a trust boundary: a
// truncated or hand-edited file becomes an empty catalog rather than malformed models; rows that no
// longer validate are dropped, not trusted. ok is false when the snapshot itself is unusable.
func ParseModelCatalogMetadataSnapshot(payload []byte) (ModelCatalogMetadataSnapshot, bool) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return ModelCatalogMetadataSnapshot{}, false
	}
	var members map[string]any
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&members); err != nil || members == nil || decoder.InputOffset() != int64(len(trimmed)) {
		return ModelCatalogMetadataSnapshot{}, false
	}
	versionRaw, present := members["version"]
	if !present {
		return ModelCatalogMetadataSnapshot{}, false
	}
	version, ok := storedCatalogNumber(versionRaw)
	if !ok || version != ModelCatalogMetadataVersion {
		return ModelCatalogMetadataSnapshot{}, false
	}
	fetchedRaw, present := members["fetchedAt"]
	if !present {
		return ModelCatalogMetadataSnapshot{}, false
	}
	fetchedAt, ok := storedCatalogNumber(fetchedRaw)
	if !ok {
		return ModelCatalogMetadataSnapshot{}, false
	}
	checkedRaw, present := members["checkedAt"]
	if !present {
		return ModelCatalogMetadataSnapshot{}, false
	}
	checkedAt, ok := storedCatalogNumber(checkedRaw)
	if !ok {
		return ModelCatalogMetadataSnapshot{}, false
	}
	if _, present := members["rows"]; !present {
		return ModelCatalogMetadataSnapshot{}, false
	}
	snapshot := ModelCatalogMetadataSnapshot{Version: ModelCatalogMetadataVersion, FetchedAt: int64(fetchedAt), CheckedAt: int64(checkedAt), Rows: []ModelCatalogMetadataRow{}}
	if etag, ok := members["etag"].(string); ok {
		snapshot.ETag = etag
	}
	if value := members["rows"]; value != nil {
		listed, ok := value.([]any)
		if !ok {
			return ModelCatalogMetadataSnapshot{}, false
		}
		for _, raw := range listed {
			if row, ok := decodeStoredCatalogRow(raw); ok {
				snapshot.Rows = append(snapshot.Rows, row)
			}
		}
	}
	return snapshot, true
}

// MarshalModelCatalogMetadataSnapshot encodes a snapshot the way JSON.stringify writes it.
func MarshalModelCatalogMetadataSnapshot(snapshot ModelCatalogMetadataSnapshot) ([]byte, error) {
	return json.Marshal(snapshot)
}
