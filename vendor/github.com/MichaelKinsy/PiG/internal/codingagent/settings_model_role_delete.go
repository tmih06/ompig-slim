package codingagent

import (
	"fmt"
	"slices"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// DeleteModelRole removes a role's assignment, display tag, explicit fallback
// chain and cycle entries in one settings-layer write. Built-ins retain a deleted
// tag marker so defaults cannot resurrect them. Invalid scopes/names and incoming
// saved aliases are rejected before mutation. Other authored layers and the live
// session's selected physical model are untouched.
// Repeated deletion is harmless. Trust checks and save failures follow
// UpdateGlobal/UpdateProject: a failed save can leave the in-memory edit applied.
func (sm *SettingsManager) DeleteModelRole(role, scope string) error {
	if !modelroles.ValidName(role) {
		return fmt.Errorf("invalid model role name %q", role)
	}
	if scope != "global" && scope != "project" {
		return fmt.Errorf("invalid model role scope %q", scope)
	}
	// Inspect both authored layers, not just the merged view: removing a role
	// must not silently break an alias currently shadowed by a project override.
	for _, layer := range []Settings{sm.GetGlobalSettings(), sm.GetProjectSettings()} {
		for owner, selector := range layer.ModelRoles {
			base, _ := modelroles.Split(selector)
			if target, ok := modelroles.Role(base); ok && target == role && owner != role {
				return fmt.Errorf("@%s is referenced by @%s; reassign it before deleting", role, owner)
			}
		}
		if layer.Retry != nil {
			for owner, chain := range layer.Retry.FallbackChains {
				if owner == role {
					continue // The role's own chain is removed in the same write.
				}
				for _, selector := range chain {
					base, _ := modelroles.Split(selector)
					if target, ok := modelroles.Role(base); ok && target == role {
						return fmt.Errorf("@%s is referenced by fallback chain %s; edit it before deleting", role, owner)
					}
				}
			}
		}
	}
	update := sm.UpdateGlobal
	if scope == "project" {
		update = sm.UpdateProject
	}
	builtin := slices.Contains(modelroles.Builtins(), role)
	effectiveOrder := sm.Get().Config.Order()
	return update(func(s *Settings) {
		if builtin && s.CycleOrder == nil {
			// Materialize an inherited/default cycle before removing a built-in;
			// otherwise nil would immediately opt it into automatic cycling again.
			s.CycleOrder = s.Config.Order()
			if scope == "project" {
				s.CycleOrder = slices.Clone(effectiveOrder)
			}
		}
		delete(s.ModelRoles, role)
		delete(s.ModelTags, role)
		if builtin {
			if s.ModelTags == nil {
				s.ModelTags = map[string]modelroles.Tag{}
			}
			s.ModelTags[role] = (modelroles.Tag{}).WithDeleted(true)
		}
		// Preserve nil (inherited defaults) versus explicit empty (disabled).
		s.CycleOrder = slices.DeleteFunc(s.CycleOrder, func(value string) bool { return value == role })
		if s.Retry != nil {
			delete(s.Retry.FallbackChains, role)
		}
	})
}
