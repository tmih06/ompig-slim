package codingagent

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// AcquireModelRoleMutation acquires the non-reentrant role transaction gate.
// Call off the UI owner loop, keep settings.mu unlocked, and release on every
// completion/cancellation path. The returned release is idempotent.
func (sm *SettingsManager) AcquireModelRoleMutation() func() {
	sm.roleMutation.Lock()
	return sync.OnceFunc(sm.roleMutation.Unlock)
}

// RecordModelUsage updates the bounded MRU list only when its head changes. It
// records physical served models, never credentials or role alias spellings.
func (sm *SettingsManager) RecordModelUsage(selector string) error {
	if selector == "" {
		return nil
	}
	recent := sm.GetGlobalSettings().ModelRecent
	if len(recent) > 0 && recent[0] == selector {
		return nil
	}
	return sm.UpdateGlobal(func(s *Settings) {
		next := []string{selector}
		for _, old := range s.ModelRecent {
			if old != selector && len(next) < 100 {
				next = append(next, old)
			}
		}
		s.ModelRecent = next
	})
}

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
//
// An alias assignment is checked for cycles before mutation. Because an alias now
// comes from the manager's own role list, `default` → `@task` → `@default` is two
// ordinary clicks away, and once saved Config.Resolve reports a cycle for both
// roles, leaving neither resolvable. Refusing the closing assignment keeps every
// saved alias walkable; the pre-existing cycle check in Resolve still covers
// hand-edited settings.
func (sm *SettingsManager) SetModelRole(role, selector, scope string) error {
	if role == "" || strings.ContainsAny(role, "/ @:\t\n") {
		return fmt.Errorf("invalid role name %q", role)
	}
	if alias, ok := modelroles.RoleRef(selector); ok {
		prospective := sm.Get().Config
		prospective.ModelRoles = maps.Clone(prospective.ModelRoles)
		if prospective.ModelRoles == nil {
			prospective.ModelRoles = map[string]string{}
		}
		prospective.ModelRoles[role] = selector
		if _, err := prospective.Resolve("@"+alias, ""); err != nil && strings.Contains(err.Error(), "cycle") {
			return fmt.Errorf("assigning @%s to @%s would form a model role cycle", role, alias)
		}
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
//
// A chain whose role entries would close a reference loop is rejected before
// mutation. A role entry expands into the referenced role's own chain, so a loop
// would leave recovery order undefined rather than merely longer. Because every
// edge of the new loop but one already exists when the write arrives, testing the
// saved chains for a path from each new role entry back to key is enough. The
// check reads the merged authored chains, so a project write is judged against the
// global entries it references and a loop closed across the two layers is refused
// whichever layer completes it. Deletion is never checked: removing an edge can
// only shorten a chain.
func (sm *SettingsManager) SetFallbackChain(key string, chain []string, scope string) error {
	if chain != nil {
		var authored map[string][]string
		if retry := sm.Get().Retry; retry != nil {
			authored = retry.FallbackChains
		}
		for _, entry := range chain {
			role, ok := modelroles.RoleRef(entry)
			if !ok {
				continue
			}
			if loop := modelroles.ChainReaches(authored, role, key); loop != nil {
				return fmt.Errorf("fallback chain for %s would form a role cycle: %s", key, strings.Join(append([]string{key}, loop...), " → "))
			}
		}
	}
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

// SetRoleCycleOrder persists role-based cycling, including an explicit empty
// order to disable it. Role order is the sole source of Ctrl+P membership.
func (sm *SettingsManager) SetRoleCycleOrder(order []string, scope string) error {
	update := func(s *Settings) { s.CycleOrder = append([]string{}, order...) }
	if scope == "project" {
		return sm.UpdateProject(update)
	}
	return sm.UpdateGlobal(update)
}

// ModelRoleCycle returns the effective role cycle without rewriting settings.
// Duplicate models remain distinct logical roles.
func (sm *SettingsManager) ModelRoleCycle() []string {
	s := sm.Get()
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
