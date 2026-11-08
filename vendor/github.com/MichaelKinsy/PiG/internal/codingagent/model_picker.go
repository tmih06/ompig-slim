package codingagent

import (
	"slices"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/tui"
)

// availableModelItems uses the runtime's available catalog, including account-specific model filters. Each item keeps the enriched
// model it was read from so the selectors can show catalog metric columns and the selected model's fact block.
func (m *InteractiveMode) availableModelItems() []tui.ModelSelectorItem {
	registry := m.modelRegistryOrDefault()
	models := registry.GetAvailableModelData()
	var items []tui.ModelSelectorItem
	for _, model := range models {
		items = append(items, tui.ModelSelectorItem{Provider: model.ProviderMeta.ProviderID, ID: model.ID, Name: model.DisplayName, Model: model})
	}
	return items
}

// scopeBrowserModels restricts chat rows to the Session's explicit scope. Typed
// runners remain available because OMP's --models scope applies only to chat.
func (m *InteractiveMode) scopeBrowserModels(items []tui.ModelSelectorItem) []tui.ModelSelectorItem {
	handle := m.opts.SessionHandle
	if handle == nil || len(handle.ScopedModels()) == 0 {
		return items
	}
	scoped := map[string]bool{}
	for _, entry := range handle.ScopedModels() {
		if entry.Model != nil {
			scoped[modelSpec(entry.Model)] = true
		}
	}
	out := make([]tui.ModelSelectorItem, 0, len(items))
	for _, item := range items {
		if tui.ModelItemKind(item) != "chat" || scoped[item.FQ()] {
			out = append(out, item)
		}
	}
	return out
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

// persistDefaultModel saves the explicit default and adds it to a nonempty model scope.
func (m *InteractiveMode) persistDefaultModel(model *ai.Model) error {
	spec := modelSpec(model)
	provider, _, _ := strings.Cut(spec, "/")
	if sm := m.opts.SettingsManager; sm != nil {
		if err := sm.SetDefaultModelAndProvider(provider, model.ID); err != nil {
			return err
		}
	}
	return m.addPersistedDefaultToNonEmptyScope(model)
}

func (m *InteractiveMode) addPersistedDefaultToNonEmptyScope(model *ai.Model) error {
	spec := modelSpec(model)
	if len(m.scopedModelIDs) == 0 || slices.Contains(m.scopedModelIDs, spec) || slices.Contains(m.scopedModelIDs, model.ID) {
		return nil
	}
	m.scopedModelIDs = append(m.scopedModelIDs, spec)
	if sm := m.opts.SettingsManager; sm != nil {
		enabled := sm.GetEnabledModels()
		if len(enabled) > 0 && !slices.ContainsFunc(enabled, func(pattern string) bool { return jsstring.ToLower(pattern) == jsstring.ToLower(spec) }) {
			enabled = append(enabled, spec)
			return sm.UpdateGlobal(func(settings *Settings) { settings.EnabledModels = enabled })
		}
	}
	return nil
}
