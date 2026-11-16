package ai

import (
	"slices"
	"testing"
)

// TestOpenCodeFreeEffortsMatchPublishedCapabilities pins each free model's thinking levels and
// wire efforts to the effort list https://opencode.ai/inference/v1/models publishes. A model that
// publishes a narrower list must not offer the levels it omits, or the request sends an effort the
// route rejects. A model that publishes none always reasons and sends no effort at all.
func TestOpenCodeFreeEffortsMatchPublishedCapabilities(t *testing.T) {
	published := map[string][]string{
		"big-pickle":                  nil,
		"mimo-v2.6-flash-free":        nil,
		"deepseek-v4.1-flash-free":    {"low", "high", "max"},
		"glm-5.3-flash-free":          {"low", "high", "max"},
		"longcat-2.5-free":            nil,
		"iceman-free":                 nil,
		"inkling-2-small-free":        {"none", "minimal", "low", "medium", "high", "max"},
		"beam-free":                   {"low", "medium", "high", "xhigh", "max"},
		"flint-40b-free":              {"low", "medium", "high", "xhigh", "max"},
		"nemotron-3.5-super-vl-free":  {"none", "minimal", "low", "medium", "high", "xhigh"},
		"nemotron-3.5-lightning-free": nil,
		"minimax-m3.1-flash-free":     {"low", "medium", "high", "xhigh", "max"},
		"hy4-flash-free":              {"none", "high"},
		"nemotron-3-ultra-free":       nil,
	}
	models := opencodeFreeChatModels()
	if len(models) != len(published) {
		t.Fatalf("catalog has %d models, want %d", len(models), len(published))
	}
	for _, generated := range models {
		efforts, known := published[generated.ID]
		if !known {
			t.Fatalf("catalog model %s is not in the published list", generated.ID)
		}
		model := generated.ToModel()
		supported := GetSupportedThinkingLevels(model)
		if len(efforts) == 0 {
			// Without an effort control the levels are inert: selectable, but nothing to send.
			if supportsReasoningEffort(generated) {
				t.Errorf("%s advertises an effort control it does not publish", generated.ID)
			}
			for _, level := range supported {
				if mapped, present := model.ThinkingLevelMap[level]; present && mapped != nil {
					t.Errorf("%s maps inert level %q to %q", generated.ID, level, *mapped)
				}
			}
			continue
		}
		if !supportsReasoningEffort(generated) {
			t.Errorf("%s publishes efforts but does not send reasoning effort", generated.ID)
		}
		want := make([]ModelThinkingLevel, 0, len(efforts))
		for _, effort := range efforts {
			if level := effortLevel(effort); level != ThinkingOff {
				want = append(want, level)
			}
		}
		got := slices.DeleteFunc(slices.Clone(supported), func(level ModelThinkingLevel) bool { return level == ThinkingOff })
		if !slices.Equal(got, want) {
			t.Errorf("%s supports %v, want %v", generated.ID, got, want)
		}
		for _, effort := range efforts {
			level := effortLevel(effort)
			mapped, present := model.ThinkingLevelMap[level]
			if !present || mapped == nil || *mapped != effort {
				t.Errorf("%s level %q maps to %v, want %q", generated.ID, level, mapped, effort)
			}
		}
	}
}

func supportsReasoningEffort(model GeneratedModel) bool {
	return model.Compat != nil && model.Compat.SupportsReasoningEffort != nil && *model.Compat.SupportsReasoningEffort
}

// TestOpenCodeFreeClampsUnpublishedEffort proves the user-visible consequence: a level a model does
// not publish resolves to one it does, so the request never carries an effort the route rejects.
func TestOpenCodeFreeClampsUnpublishedEffort(t *testing.T) {
	for _, generated := range opencodeFreeChatModels() {
		model := generated.ToModel()
		supported := GetSupportedThinkingLevels(model)
		for _, requested := range extendedThinkingLevels {
			clamped := ClampThinkingLevel(model, requested)
			if !slices.Contains(supported, clamped) {
				t.Fatalf("%s clamps %q to unsupported %q", generated.ID, requested, clamped)
			}
			if !supportsReasoningEffort(generated) {
				continue
			}
			mapped, present := model.ThinkingLevelMap[clamped]
			if !present || mapped == nil {
				t.Errorf("%s sends no effort for clamped level %q", generated.ID, clamped)
			}
		}
	}
}
