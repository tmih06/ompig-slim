package codingagent

import (
	"fmt"
	"strings"
)

// modelPresetHandler ports OMP's save/switch/list/delete commands with failures
// reported separately from a successful live switch and project shadowing.
func modelPresetHandler(sc *SlashContext) error {
	sm := sc.SettingsManager
	if sm == nil {
		return fmt.Errorf("model settings unavailable")
	}
	args := strings.Fields(sc.Args)
	action := "list"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "list":
		s := sm.Get()
		active := s.Config.ActivePreset(s.DefaultThinkingLevel)
		names := s.Config.PresetNames()
		if len(names) == 0 {
			sc.Append("No saved model presets")
			return nil
		}
		for _, name := range names {
			marker := "  "
			if name == active {
				marker = "✓ "
			}
			sc.Append(marker + name)
		}
		return nil
	case "save", "delete", "switch":
		if len(args) != 2 {
			return fmt.Errorf("Usage: /modelpreset %s <name>", action)
		}
		name := args[1]
		if action == "save" {
			if err := sm.SaveModelPreset(name); err != nil {
				return err
			}
			message := fmt.Sprintf("Saved model preset %q", name)
			if _, ok := sm.GetProjectSettings().ModelPresets[name]; ok {
				message += " to global config, but project config defines the same name and takes precedence"
			}
			sc.Append(message)
			return nil
		}
		if action == "delete" {
			result, err := sm.DeleteModelPreset(name)
			if err != nil {
				return err
			}
			switch result {
			case "deleted":
				sc.Append("Deleted model preset " + name)
			case "project":
				sc.Append("Preset " + name + " is still defined by project config; remove it there")
			default:
				sc.Append("Preset not found: " + name)
			}
			return nil
		}
		if sc.ApplyModelPreset == nil {
			return fmt.Errorf("preset switching unavailable in this context")
		}
		sc.ApplyModelPreset(name, func(message string, err error) {
			if err != nil {
				if sc.ShowError != nil {
					sc.ShowError(err.Error())
				}
				return
			}
			if sc.ShowStatus != nil {
				sc.ShowStatus(message)
			}
		})
		return nil
	default:
		return fmt.Errorf("Usage: /modelpreset [list|save|switch|delete] [name]")
	}
}
