package codingagent

import (
	"fmt"
	"slices"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// DeleteModelRole removes a custom role's assignment, tag, explicit fallback
// chain and cycle entries in one settings-layer write. Built-ins, invalid scopes
// and roles referenced by another saved assignment/chain are rejected before
// mutation. Other layers and the live session's selected model are untouched.
// Repeated deletion is harmless. Trust checks and save failures follow
// UpdateGlobal/UpdateProject: a failed save can leave the in-memory edit applied.
func (sm *SettingsManager) DeleteModelRole(role, scope string) error {
	if !modelroles.ValidName(role) || slices.Contains(modelroles.Builtins(), role) {
		return fmt.Errorf("only custom model roles can be deleted: @%s", role)
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
					if target, ok := modelroles.RoleRef(selector); ok && target == role {
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
	return update(func(s *Settings) {
		delete(s.ModelRoles, role)
		delete(s.ModelTags, role)
		// Preserve nil (inherited defaults) versus explicit empty (disabled).
		s.CycleOrder = slices.DeleteFunc(s.CycleOrder, func(value string) bool { return value == role })
		if s.Retry != nil {
			delete(s.Retry.FallbackChains, role)
		}
	})
}
