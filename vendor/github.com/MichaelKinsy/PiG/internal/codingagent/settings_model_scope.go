package codingagent

import (
	"fmt"
	"maps"
	"slices"
)

// ImportLegacyModelScope migrates a saved enabledModels list to ordinary named
// roles exactly once. CLI-only scopes are never passed here. Source-layer
// ownership is preserved, thinking is retained, collisions never overwrite roles,
// and an explicit empty legacy list prevents it filtering future role choices.
func (sm *SettingsManager) ImportLegacyModelScope(choices []ScopedModel) (bool, error) {
	settings := sm.Get()
	if len(settings.EnabledModels) == 0 || len(choices) == 0 {
		return false, nil
	}
	update := sm.UpdateGlobal
	if sm.GetProjectSettings().EnabledModels != nil {
		update = sm.UpdateProject
	}
	err := update(func(s *Settings) {
		s.ModelRoles = maps.Clone(s.ModelRoles)
		if s.ModelRoles == nil {
			s.ModelRoles = map[string]string{}
		}
		// A pre-existing role cycle is not evidence that the legacy filter was
		// cleared. Preserve its order while absorbing any remaining scope rows.
		s.CycleOrder = append([]string{}, settings.CycleOrder...)
		if settings.CycleOrder == nil && len(settings.ModelRoles) > 0 {
			s.CycleOrder = settings.Config.Order()
		}
		for i, item := range choices {
			selector := modelRef(item.Model)
			if item.ThinkingLevel != "" {
				selector += ":" + string(item.ThinkingLevel)
			}
			role := fmt.Sprintf("scoped-%d", i+1)
			for suffix := 2; settings.ModelRoles[role] != "" && settings.ModelRoles[role] != selector; suffix++ {
				role = fmt.Sprintf("scoped-%d-%d", i+1, suffix)
			}
			s.ModelRoles[role] = selector
			if (settings.CycleOrder == nil || len(settings.CycleOrder) > 0) && !slices.Contains(s.CycleOrder, role) {
				s.CycleOrder = append(s.CycleOrder, role)
			}
		}
		s.EnabledModels = []string{}
	})
	return err == nil, err
}
