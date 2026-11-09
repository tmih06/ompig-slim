package codingagent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// modelCompactionEntries decodes the existing lossless compaction object. Unknown
// keys remain untouched; null entries suppress same-key lower-layer settings.
func modelCompactionEntries(s Settings) map[string]json.RawMessage {
	entries := map[string]json.RawMessage{}
	if s.Compaction != nil {
		_ = json.Unmarshal(s.Compaction.extra["modelThresholds"], &entries)
	}
	return entries
}

// ModelCompactionPolicy returns the model's exact or longest prefix entry plus
// global percentage/fixed policy. Invalid authored entries fail only this model.
func (sm *SettingsManager) ModelCompactionPolicy(model *ai.Model) (point modelroles.CompactionPoint, percent float64, fixed int, err error) {
	s := sm.Get()
	if s.Compaction == nil {
		return
	}
	_ = json.Unmarshal(s.Compaction.extra["thresholdPercent"], &percent)
	_ = json.Unmarshal(s.Compaction.extra["thresholdTokens"], &fixed)
	var enabled *bool
	_ = json.Unmarshal(s.Compaction.extra["modelThresholdsEnabled"], &enabled)
	if enabled != nil && !*enabled {
		return
	}
	if model == nil {
		return
	}
	entries := modelCompactionEntries(s)
	selector := model.ProviderID() + "/" + model.ID
	key := ""
	if _, ok := entries[selector]; ok {
		key = selector
	} else {
		for candidate := range entries {
			if strings.HasSuffix(candidate, "*") && strings.HasPrefix(selector, strings.TrimSuffix(candidate, "*")) && len(candidate) > len(key) {
				key = candidate
			}
		}
	}
	if key != "" {
		point, err = modelroles.ParseCompactionEntry(entries[key])
		if err != nil {
			err = fmt.Errorf("Invalid compaction.modelThresholds.%s: %w", key, err)
		}
	}
	return
}

// SetModelCompactionPoint validates before saving globally and refuses a project
// override that would make the write ineffective. The callback's confirmation
// flag is reserved for models exposing a separately advertised extended tier.
func (sm *SettingsManager) SetModelCompactionPoint(model *ai.Model, input string, confirmed bool) (string, bool, error) {
	point, err := modelroles.ParseCompactionInput(input)
	if err != nil {
		return "", false, err
	}
	key := model.ProviderID() + "/" + model.ID
	if _, ok := modelCompactionEntries(sm.GetProjectSettings())[key]; ok {
		return "", false, fmt.Errorf("%s is set in project config; edit compaction.modelThresholds there", key)
	}
	window := model.Capabilities.ContextWindow
	if window > 0 && (point.Tokens > window || point.Fixed && point.Tokens >= window) {
		return "", false, fmt.Errorf("compaction point must fit the %d-token window", window)
	}
	err = sm.UpdateGlobal(func(s *Settings) {
		if s.Compaction == nil {
			s.Compaction = &CompactionSettingsJSON{}
		}
		if s.Compaction.extra == nil {
			s.Compaction.extra = map[string]json.RawMessage{}
		}
		entries := modelCompactionEntries(*s)
		if raw := point.Raw(); raw != nil {
			entries[key] = raw
		} else {
			delete(entries, key)
		}
		raw, _ := json.Marshal(entries)
		s.Compaction.extra["modelThresholds"] = raw
	})
	if err != nil {
		return "", false, err
	}
	return sm.PreviewModelCompactionPoint(model, input), false, nil
}

// PreviewModelCompactionPoint shares runtime trigger calculation with the hub.
func (sm *SettingsManager) PreviewModelCompactionPoint(model *ai.Model, input string) string {
	point, err := modelroles.ParseCompactionInput(input)
	if err != nil {
		return ""
	}
	cfg, err := sm.GetCompactionSettings(model)
	if err != nil {
		return err.Error()
	}
	if !cfg.Enabled {
		return "auto-compaction is off"
	}
	_, percent, fixed, _ := sm.ModelCompactionPolicy(model)
	tokens := modelroles.CompactionTrigger(model.Capabilities.ContextWindow, cfg.ReserveTokens, percent, fixed, point)
	basis := "window"
	if point.Fixed {
		basis = "fixed"
	} else if point.Tokens > 0 {
		basis = fmt.Sprintf("%d-token base", point.Tokens)
	} else if point.Percent > 0 {
		basis = fmt.Sprintf("%g%% of window", point.Percent)
	}
	return fmt.Sprintf("compacts at %d · %s", tokens, basis)
}
