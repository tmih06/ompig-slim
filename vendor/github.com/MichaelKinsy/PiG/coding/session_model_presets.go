package coding

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// ApplyModelPreset validates a preset's prospective default before writing any
// settings, then applies roles and switches the session. Execute off the TUI owner
// loop because auth and extension model_select handlers may await that loop.
// A refused preset leaves settings untouched; a later failure is reported as such.
func (s *Session) ApplyModelPreset(ctx context.Context, name string) (string, error) {
	return s.applyModelPreset(ctx, name, nil)
}

// ApplyModelPresetOnMain dispatches the synchronous model mutation to a host loop
// while auth and extension notifications remain off-loop, like SetModelOnMain.
func (s *Session) ApplyModelPresetOnMain(ctx context.Context, name string, dispatch func(func() error) error) (string, error) {
	return s.applyModelPreset(ctx, name, dispatch)
}

// applyModelPreset owns validation/write/switch ordering for both API entrypoints.
func (s *Session) applyModelPreset(ctx context.Context, name string, dispatch func(func() error) error) (string, error) {
	settings := s.SettingsManager()
	release := settings.AcquireModelRoleMutation()
	defer release()
	if ctx.Err() != nil {
		return "", context.Cause(ctx)
	}
	snapshot := settings.Get()
	preset, err := snapshot.Config.Preset(name)
	if err != nil {
		return "", err
	}
	prospective := snapshot.Config.Clone()
	prospective.ModelRoles = maps.Clone(preset.ModelRoles)
	if snapshot.ModelRoleStorage == "project" {
		prospective = s.SettingsManager().GetGlobalSettings().Config.Merge(prospective)
	}
	legacy := snapshot.DefaultProvider + "/" + snapshot.DefaultModel
	selector := preset.ModelRoles["default"]
	candidates := s.AvailableModelSnapshot()
	if scoped := s.ScopedModels(); len(scoped) > 0 {
		candidates = nil
		for _, entry := range scoped {
			if entry.Model != nil {
				candidates = append(candidates, entry.Model)
			}
		}
	}
	var target *ai.Model
	if selector != "" {
		resolved, e := prospective.Resolve(selector, legacy)
		if e != nil {
			return "", e
		}
		base, _ := modelroles.Split(resolved)
		for _, model := range candidates {
			if providerID(model)+"/"+model.ID == base {
				target = model
				break
			}
		}
	} else {
		for _, model := range candidates {
			if s.checkModelAuth(ctx, model) == nil {
				target = model
				break
			}
		}
	}
	if target == nil {
		return "", fmt.Errorf("Preset %q not applied: default model is not available", name)
	}
	if err = s.checkModelAuth(ctx, target); err != nil {
		return "", fmt.Errorf("Preset %q not applied: %w", name, err)
	}
	shadowed, err := settings.WriteModelPreset(preset)
	if err != nil {
		return "", err
	}
	effective := settings.Get()
	live := effective.ModelRoles["default"]
	if live != "" {
		resolved, e := settings.ResolveModelRole("@default")
		if e != nil {
			return "", fmt.Errorf("Preset %q roles saved, but model was not switched: %w", name, e)
		}
		base, _ := modelroles.Split(resolved)
		target, err = BuildModel(base, s.services)
		if err != nil {
			return "", err
		}
		selector = resolved
	}
	_, level := modelroles.Split(selector)
	if level == "" || level == "inherit" {
		level = preset.DefaultThinkingLevel
	}

	option := ModelMutationOptions{Role: "default", ThinkingLevel: ai.ModelThinkingLevel(level)}
	if err = s.SetModelOnMain(ctx, target, option, dispatch); err != nil {
		return "", fmt.Errorf("Preset %q roles saved, but model was not switched: %w", name, err)
	}
	message := fmt.Sprintf("Switched to preset %q (%s/%s)", name, providerID(target), target.ID)
	if len(shadowed) > 0 {
		message += "; still set elsewhere: " + strings.Join(shadowed, ", ")
	}
	if preset.DefaultThinkingLevel != "" && effective.DefaultThinkingLevel != preset.DefaultThinkingLevel {
		message += "; defaultThinkingLevel still set by project config"
	}
	return message, nil
}
