package codingagent

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
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

// SaveModelPreset stores only this name in global config; project entries stay
// project-owned. Existing names overwrite atomically, never merge role maps.
func (sm *SettingsManager) SaveModelPreset(name string) error {
	if !modelroles.ValidName(name) {
		return fmt.Errorf("Invalid preset name %q: use a letter, then letters, digits, - or _", name)
	}
	snapshot := sm.Get()
	raw, err := json.Marshal(snapshot.Config.CapturePreset(snapshot.DefaultThinkingLevel))
	if err != nil {
		return err
	}
	return sm.UpdateGlobal(func(s *Settings) {
		if s.ModelPresets == nil {
			s.ModelPresets = map[string]json.RawMessage{}
		}
		s.ModelPresets[name] = raw
	})
}

// DeleteModelPreset removes a global entry. The return value reports a surviving
// higher-layer definition rather than falsely claiming the preset disappeared.
func (sm *SettingsManager) DeleteModelPreset(name string) (string, error) {
	if _, ok := sm.GetGlobalSettings().ModelPresets[name]; ok {
		if err := sm.UpdateGlobal(func(s *Settings) { delete(s.ModelPresets, name) }); err != nil {
			return "", err
		}
		if slices.Contains(sm.Get().Config.PresetNames(), name) {
			return "project", nil
		}
		return "deleted", nil
	}
	if slices.Contains(sm.Get().Config.PresetNames(), name) {
		return "project", nil
	}
	return "missing", nil
}

// WriteModelPreset applies roles to the hub's selected storage scope. The caller
// must validate the effective default and credentials before invoking it. Roles
// absent from the preset are removed only from the target layer; lower/higher
// layers remain authoritative, and shadowed assignments are reported.
func (sm *SettingsManager) WriteModelPreset(p modelroles.Preset) ([]string, error) {
	scope := sm.Get().ModelRoleStorage
	update := func(s *Settings) { s.ModelRoles = maps.Clone(p.ModelRoles) }
	var err error
	if scope == "project" {
		err = sm.UpdateProject(update)
	} else {
		err = sm.UpdateGlobal(update)
	}
	if err != nil {
		return nil, err
	}
	if p.DefaultThinkingLevel != "" {
		if err = sm.UpdateGlobal(func(s *Settings) { s.DefaultThinkingLevel = p.DefaultThinkingLevel }); err != nil {
			return nil, err
		}
	}
	effective := sm.Get()
	var shadowed []string
	for role, value := range effective.ModelRoles {
		if p.ModelRoles[role] != value {
			shadowed = append(shadowed, role)
		}
	}
	for role, value := range p.ModelRoles {
		if _, ok := effective.ModelRoles[role]; !ok && value != "" {
			shadowed = append(shadowed, role)
		}
	}
	slices.Sort(shadowed)
	return shadowed, nil
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
