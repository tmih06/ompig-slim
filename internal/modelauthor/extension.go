package modelauthor

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/MichaelKinsy/PiG/coding/extension"
)

// authorEnabled reads modelAuthor.enabled from the effective settings; it is off
// unless the setting is explicitly true.
func authorEnabled(settings extension.Settings) bool {
	values, ok := settings["modelAuthor"].(map[string]any)
	if !ok {
		return false
	}
	enabled, ok := values["enabled"].(bool)
	return ok && enabled
}

// Extension returns the hidden inline extension. Its bash tool_call handler
// edits the command in place, which the harness then runs, so attribution never
// depends on the model remembering a rule.
func Extension(api extension.API) error {
	// override is the process's /model-author choice; nil defers to the setting.
	var override atomic.Pointer[bool]
	enabled := func() bool {
		if choice := override.Load(); choice != nil {
			return *choice
		}
		return authorEnabled(api.GetSettings())
	}
	api.RegisterCommand("model-author", extension.CommandOptions{
		Description:            "Attribute the agent's git commits to its model (off by default).",
		GetArgumentCompletions: commandCompletions,
		Handler: func(ctx context.Context, args string) error {
			switch arg := strings.ToLower(strings.TrimSpace(args)); arg {
			case "on", "off":
				choice := arg == "on"
				override.Store(&choice)
				notify(ctx, statusText(choice), "info")
			case "", "status":
				notify(ctx, statusText(enabled()), "info")
			default:
				notify(ctx, "Usage: /model-author [on|off|status]", "error")
				return fmt.Errorf("model-author: unknown argument %q", arg)
			}
			return nil
		},
	})
	api.OnToolCall(func(ctx context.Context, event extension.ToolCallEvent) (extension.ToolCallEventResult, error) {
		call, ok := event.(extension.CustomToolCallEvent)
		if !ok || call.ToolName != "bash" {
			return extension.ToolCallEventResult{}, nil
		}
		command, ok := call.Input["command"].(string)
		if !ok || !strings.Contains(command, "commit") || !enabled() {
			return extension.ToolCallEventResult{}, nil
		}
		model, err := extension.FromContext(ctx).Model()
		if err != nil || model == nil {
			return extension.ToolCallEventResult{}, err
		}
		identity, ok := authorFor(model)
		if !ok {
			return extension.ToolCallEventResult{}, nil
		}
		call.Input["command"] = rewriteCommits(command, identity)
		return extension.ToolCallEventResult{}, nil
	})
	return nil
}

func statusText(on bool) string {
	if on {
		return "Model author is on: git commit in bash is attributed to the model."
	}
	return "Model author is off. Run /model-author on, or set modelAuthor.enabled to true to enable it by default."
}

func commandCompletions(prefix string) ([]extension.AutocompleteItem, error) {
	var items []extension.AutocompleteItem
	for _, word := range []string{"on", "off", "status"} {
		if strings.HasPrefix(word, strings.ToLower(prefix)) {
			items = append(items, extension.AutocompleteItem{Value: word, Label: word})
		}
	}
	return items, nil
}

// notify shows a human-facing notice; modes without a UI bind a no-op one.
func notify(ctx context.Context, message, kind string) {
	if extCtx := extension.FromContext(ctx); extCtx != nil {
		if ui, err := extCtx.UI(); err == nil {
			ui.Notify(message, kind)
		}
	}
}
