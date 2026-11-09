package coding

import (
	"slices"

	"github.com/MichaelKinsy/PiG/ai"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
)

// ResolveCliModelOptions selects one catalog model. InlineApiKey permits first-
// time setup before stored authentication exists; ModelRuntime owns the catalog.
type ResolveCliModelOptions struct {
	CliProvider  string
	CliModel     string
	CliThinking  ai.ThinkingLevel
	InlineApiKey bool
	ModelRuntime *ModelRuntime
}

// ResolveCliModelResult keeps diagnostics separate from model construction.
// Error is suitable for CLI display; when set, Model is nil.
type ResolveCliModelResult struct {
	Model         *ai.Model
	ThinkingLevel ai.ThinkingLevel
	Warning       string
	Error         string
}

// runtimeModelOf projects native selection metadata without cloning the model.
func runtimeModelOf(model *ai.Model) icodingagent.RuntimeModel {
	return icodingagent.RuntimeModel{NativeModel: model, Provider: model.ProviderMeta.ProviderID, ID: model.ID, Name: model.DisplayName, Reasoning: model.ProviderMeta.Reasoning || model.Capabilities.MaxThinking != "", Headers: ai.ProviderHeadersFromStrings(model.ProviderMeta.Headers)}
}

// cliResolverRuntime lists identities without constructing every bundled model;
// ResolveCliModel materializes only the selected row through SourceModel.
type cliResolverRuntime struct{ runtime *ModelRuntime }

// GetModels returns all identities, or only the requested provider's identities.
func (r cliResolverRuntime) GetModels(providerID string) []icodingagent.RuntimeModel {
	models := r.runtime.runtimeModels()
	if providerID != "" {
		models = slices.DeleteFunc(models, func(model icodingagent.RuntimeModel) bool { return model.Provider != providerID })
	}
	return models
}

// HasConfiguredAuth reads configured auth without performing request auth.
func (r cliResolverRuntime) HasConfiguredAuth(providerID string) bool {
	return r.runtime.HasConfiguredAuth(providerID)
}

// ResolveCliModel resolves one model against the complete catalog, not only
// authenticated rows, so --api-key works for first-time setup. A thinking suffix
// is returned separately and never mutates the shared catalog model.
func ResolveCliModel(options ResolveCliModelOptions) ResolveCliModelResult {
	resolved := icodingagent.ResolveCliModel(options.CliProvider, options.CliModel, string(options.CliThinking), options.InlineApiKey, cliResolverRuntime{options.ModelRuntime})
	result := ResolveCliModelResult{ThinkingLevel: resolved.ThinkingLevel, Warning: resolved.Warning, Error: resolved.Error}
	if resolved.Model != nil {
		result.Model = resolved.Model.SourceModel()
	}
	return result
}
