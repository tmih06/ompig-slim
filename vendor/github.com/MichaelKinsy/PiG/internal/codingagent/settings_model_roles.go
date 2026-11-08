package codingagent

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// ResolveModelRole expands OMP role aliases against a consistent merged settings snapshot.
func (sm *SettingsManager) ResolveModelRole(selector string, pools ...[]modelroles.Candidate) (string, error) {
	s := sm.Get()
	legacy := s.DefaultProvider + "/" + s.DefaultModel
	if len(pools) > 0 {
		return s.Config.ResolveAvailable(selector, legacy, pools[0])
	}
	return s.Config.Resolve(selector, legacy)
}

// GetAutoThinkingMaxEffort reads OMP's provider-level ceiling without changing
// unrelated provider settings preserved by the lossless settings object.
func (sm *SettingsManager) GetAutoThinkingMaxEffort() string {
	s := sm.Get()
	var providers struct {
		Max string `json:"autoThinkingMaxEffort"`
	}
	_ = json.Unmarshal(s.extra["providers"], &providers)
	if providers.Max == "max" {
		return "max"
	}
	return "xhigh"
}

// SetModelRole writes an assignment in the requested scope. Empty removes that
// scope's override, revealing a lower layer or the role's automatic inheritance.
// Project writes use the existing trusted-settings boundary; errors are not swallowed.
func (sm *SettingsManager) SetModelRole(role, selector, scope string) error {
	if role == "" || strings.ContainsAny(role, "/ @:\t\n") {
		return fmt.Errorf("invalid role name %q", role)
	}
	update := func(s *Settings) {
		if s.ModelRoles == nil {
			s.ModelRoles = map[string]string{}
		}
		if selector == "" {
			delete(s.ModelRoles, role)
		} else {
			s.ModelRoles[role] = selector
		}
	}
	if scope == "project" {
		return sm.UpdateProject(update)
	}
	return sm.UpdateGlobal(update)
}

// SetFallbackChain replaces one ordered chain. A non-nil empty chain disables
// inherited fallbacks; nil removes this scope's key and restores inheritance.
func (sm *SettingsManager) SetFallbackChain(key string, chain []string, scope string) error {
	update := func(s *Settings) {
		if s.Retry == nil {
			s.Retry = &RetrySettingsJSON{}
		}
		if s.Retry.FallbackChains == nil {
			s.Retry.FallbackChains = map[string][]string{}
		}
		if chain == nil {
			delete(s.Retry.FallbackChains, key)
		} else {
			s.Retry.FallbackChains[key] = append([]string{}, chain...)
		}
	}
	if scope == "project" {
		return sm.UpdateProject(update)
	}
	return sm.UpdateGlobal(update)
}

// SetRoleCycleOrder persists role-based cycling. The legacy enabledModels list is
// retained for compatibility, but explicit role order becomes authoritative.
func (sm *SettingsManager) SetRoleCycleOrder(order []string, scope string) error {
	update := func(s *Settings) { s.CycleOrder = append([]string{}, order...) }
	if scope == "project" {
		return sm.UpdateProject(update)
	}
	return sm.UpdateGlobal(update)
}

// ModelRoleCycle merges legacy model scope with the role system without rewriting
// settings during reads. Explicit cycle order wins; duplicate models remain distinct roles.
func (sm *SettingsManager) ModelRoleCycle() []string {
	s := sm.Get()
	if s.CycleOrder == nil && len(s.ModelRoles) == 0 && len(s.EnabledModels) > 0 {
		return nil
	}
	return slices.Clone(s.Config.Order())
}

// ModelFallbackChains returns a detached map, expanding OMP's default chain to
// chat roles only. Explicit empty entries remain authoritative opt-outs.
func (sm *SettingsManager) ModelFallbackChains() map[string][]string {
	s := sm.Get()
	if s.Retry == nil {
		return nil
	}
	chains := modelroles.CloneChains(s.Retry.FallbackChains)
	if chain, ok := chains["default"]; ok {
		for _, role := range s.Config.Known(chains) {
			if slices.Contains([]string{"image", "web", "speech", "dictation", "judge"}, role) {
				continue
			}
			if _, exists := chains[role]; !exists {
				chains[role] = slices.Clone(chain)
			}
		}
	}
	return chains
}
