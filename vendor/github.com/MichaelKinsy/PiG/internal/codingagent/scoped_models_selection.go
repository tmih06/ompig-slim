package codingagent

// Ports packages/coding-agent/src/modes/interactive/interactive-mode.ts.

import (
	"github.com/MichaelKinsy/PiG/ai"
)

func (m *InteractiveMode) initScopedModels() {
	m.scopedModelIDs = nil
	if m.opts.SessionHandle != nil {
		for _, scoped := range m.opts.SessionHandle.ScopedModels() {
			if scoped.Model != nil {
				m.scopedModelIDs = append(m.scopedModelIDs, scopedModelKey(scoped.Model))
			}
		}
		return
	}
	if len(m.opts.Settings.EnabledModels) == 0 {
		return
	}
	var models []RuntimeModel
	for _, item := range m.availableModelItems() {
		models = append(models, RuntimeModel{Provider: item.Provider, ID: item.ID, Name: item.Name})
	}
	for _, scoped := range ResolveModelScopeFromModels(m.opts.Settings.EnabledModels, models).ScopedModels {
		m.scopedModelIDs = append(m.scopedModelIDs, modelRef(scoped.Model))
	}
}

// scopedModelKey is Pi's `${model.provider}/${model.id}` scope key; unlike modelSpec it never collapses an ID that already starts with the provider.
func scopedModelKey(model *ai.Model) string {
	providerID := model.ProviderMeta.ProviderID
	if providerID == "" && model.Provider != nil {
		providerID = model.Provider.ID()
	}
	return providerID + "/" + model.ID
}
