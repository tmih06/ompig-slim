package codingagent

import "github.com/MichaelKinsy/PiG/ai"

// RuntimeModels returns composed selection metadata without resolving request authentication or copying unused request capabilities.
func (r *ModelRegistry) RuntimeModels() []RuntimeModel {
	providerIDs := r.modelProviderIDs()
	// Size the result for the bundled catalogs, which are most of it, so it is not regrown model by model.
	overlays := make([]bool, len(providerIDs))
	builtins := 0
	for i, id := range providerIDs {
		overlays[i] = !r.isPlainGenerated(id)
		builtins += len(builtinChatCatalog(id))
	}
	result := make([]RuntimeModel, 0, builtins)
	for i, id := range providerIDs {
		if overlays[i] {
			for _, model := range r.GetProviderModelData(id) {
				result = append(result, RuntimeModel{Provider: model.ProviderMeta.ProviderID, ID: model.ID, Name: model.DisplayName, Reasoning: model.ProviderMeta.Reasoning || model.Capabilities.MaxThinking != "", Headers: ai.ProviderHeadersFromStrings(model.ProviderMeta.Headers)})
			}
			continue
		}
		// Unmodified built-ins already contain every field the model resolver reads. Preserve fresh header ownership without materializing the request-only fields of ai.Model.
		for _, model := range builtinChatCatalog(id) {
			result = append(result, RuntimeModel{Provider: model.Provider, ID: model.ID, Name: model.DisplayName, Reasoning: model.Reasoning, Headers: ai.ProviderHeadersFromStrings(model.Headers)})
		}
	}
	return result
}
