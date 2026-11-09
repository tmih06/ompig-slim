package codingagent

import (
	"slices"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
)

// availableModelItems uses the runtime's available catalog, including account-specific model filters. Each item keeps the enriched
// model it was read from so the selectors can show catalog metric columns and the selected model's fact block.
func (m *InteractiveMode) availableModelItems() []tui.ModelSelectorItem {
	registry := m.modelRegistryOrDefault()
	models := registry.GetAvailableModelData()
	var items []tui.ModelSelectorItem
	disabled := []string{}
	if m.opts.SettingsManager != nil {
		disabled = m.opts.SettingsManager.Get().DisabledProviders
	}
	for _, model := range models {
		if slices.Contains(disabled, model.ProviderID()) {
			continue
		}
		items = append(items, tui.ModelSelectorItem{Provider: model.ProviderMeta.ProviderID, ID: model.ID, Name: model.DisplayName, Model: model})
	}
	return items
}

// modelRegistryOrDefault is the mode's model registry, or the fallback registry over the agent
// directory's credentials when the mode has none. The fallback is built once per mode and released
// by Stop, so repeated picker passes do not open a new database handle each time. An external
// registry (and, by extension, a caller-supplied credential store) is never closed here.
func (m *InteractiveMode) modelRegistryOrDefault() *ModelRegistry {
	if m == nil {
		return NewModelRegistry("")
	}
	if registry := m.opts.ModelRegistry; registry != nil {
		return registry
	}
	m.fallbackMu.Lock()
	defer m.fallbackMu.Unlock()
	if m.fallbackRegistry != nil {
		return m.fallbackRegistry
	}
	agentDir := m.opts.AgentDir
	// Stop already released the mode's fallback. A later call (teardown races, or a picker after
	// Stop) gets an unauthenticated registry rather than a new handle nothing would close.
	if m.fallbackClosed {
		return NewModelRegistry(agentDir)
	}
	registry := NewModelRegistry(agentDir)
	if agentDir != "" {
		if credentials, err := ai.OpenCredentialStore(agentDir); err == nil {
			registry.SetCredentialStore(credentials)
			m.fallbackStore = credentials
		}
	}
	m.fallbackRegistry = registry
	return registry
}

// closeFallbackRegistry releases the fallback registry's model tasks and the credential store the
// mode opened for it. It never touches the mode's external registry or a caller-supplied store, and
// it is idempotent so repeated Stop calls and a Stop after a failed Init release the handle once.
func (m *InteractiveMode) closeFallbackRegistry() {
	if m == nil {
		return
	}
	m.fallbackMu.Lock()
	registry, store := m.fallbackRegistry, m.fallbackStore
	m.fallbackRegistry, m.fallbackStore = nil, nil
	m.fallbackClosed = true
	m.fallbackMu.Unlock()
	if registry != nil {
		// Drain the reads that registry started before its database handle goes away.
		registry.CloseModelTasks()
	}
	if closer, ok := store.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

// persistDefaultModel saves the explicit physical default without changing roles
// or the role cycle.
func (m *InteractiveMode) persistDefaultModel(model *ai.Model) error {
	spec := modelSpec(model)
	provider, _, _ := strings.Cut(spec, "/")
	if sm := m.opts.SettingsManager; sm != nil {
		if err := sm.SetDefaultModelAndProvider(provider, model.ID); err != nil {
			return err
		}
	}
	return nil
}
