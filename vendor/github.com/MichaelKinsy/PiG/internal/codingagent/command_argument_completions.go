package codingagent

import (
	"fmt"
	"strings"

	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/tui"
)

// subagentArgumentOptions returns fresh, ordered choices shared by /agents and
// /subagents. Descriptions reflect their common dispatcher, not the bare-command defaults.
func subagentArgumentOptions() []tui.AutocompleteItem {
	return []tui.AutocompleteItem{
		{Value: "on", Label: "on", Description: "Enable built-in subagents"},
		{Value: "off", Label: "off", Description: "Disable subagents and stop active agents"},
		{Value: "status", Label: "status", Description: "Show enabled state, background slots, and agent count"},
		{Value: "hub", Label: "hub", Description: "Toggle the live Agent Hub"},
		{Value: "menu", Label: "menu", Description: "Manage agent types, workflows, and scheduled jobs"},
		{Value: "settings", Label: "settings", Description: "Configure subagent settings"},
	}
}

// builtinArgumentCompletions captures static first-argument choices. Matching is
// case-insensitive prefix matching in declaration order; later arguments are left
// alone rather than replacing the entire argument string with a first-word choice.
func builtinArgumentCompletions(options []tui.AutocompleteItem) func(string) []tui.AutocompleteItem {
	if len(options) == 0 {
		return nil
	}
	return func(prefix string) []tui.AutocompleteItem {
		if strings.ContainsAny(prefix, " \t\r\n") {
			return nil
		}
		var matches []tui.AutocompleteItem
		for _, option := range options {
			if strings.HasPrefix(option.Value, strings.ToLower(prefix)) {
				matches = append(matches, option)
			}
		}
		return matches
	}
}

// callArgumentCompletions converts a Go callback panic into a surfaced callback error.
func callArgumentCompletions(complete extension.ArgumentCompletionsFunc, prefix string) (items []extension.AutocompleteItem, err error) {
	defer func() {
		if value := recover(); value != nil {
			items = nil
			err = fmt.Errorf("autocomplete callback panicked: %v", value)
		}
	}()
	return complete(prefix)
}

// extensionCommandSlashEntry retains the command's awaited callback. Its first result is real, not a cache-miss stand-in followed by a second query.
func (m *InteractiveMode) extensionCommandSlashEntry(command extension.ResolvedCommand) tui.SlashCommand {
	var info *PiSourceInfo
	if command.SourceInfo != (PiSourceInfo{}) {
		converted := command.SourceInfo
		info = &converted
	}
	entry := tui.SlashCommand{Name: strings.TrimPrefix(command.InvocationName, "/"), Description: prefixAutocompleteDescription(command.Description, info)}
	if command.GetArgumentCompletions != nil {
		entry.AwaitArgumentCompletions = func(prefix string) ([]tui.AutocompleteItem, error) {
			items, err := callArgumentCompletions(command.GetArgumentCompletions, prefix)
			if err != nil {
				return nil, err
			}
			converted := make([]tui.AutocompleteItem, len(items))
			for i, item := range items {
				label := item.Label
				if label == "" {
					label = item.Value
				}
				converted[i] = tui.AutocompleteItem{Value: item.Value, Label: label, Description: item.Description}
			}
			return converted, nil
		}
	}
	return entry
}
