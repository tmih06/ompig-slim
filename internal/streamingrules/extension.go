package streamingrules

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/coding/extension"
)

const customType = "streaming-rule-reminder"

type deliveryState uint8

const (
	pending deliveryState = iota + 1
	delivered
)

type runState struct {
	rules    []*rule
	delivery map[*rule]deliveryState
	pokes    []extension.SessionBoundaryDraft
}

func newRun(rules []*rule) runState {
	return runState{rules: rules, delivery: map[*rule]deliveryState{}}
}

func (s *runState) matching(tool string, args map[string]any, blockingOnly bool) []*rule {
	var snapshots []snapshot
	var matched []*rule
	for _, rule := range s.rules {
		if (blockingOnly && !rule.interrupt) || s.delivery[rule] == delivered {
			continue
		}
		if !slices.ContainsFunc(rule.scopes, func(scope scope) bool { return scope.tool == "" || scope.tool == tool }) {
			continue
		}
		if snapshots == nil {
			snapshots = toolSnapshots(tool, args)
		}
		for _, snapshot := range snapshots {
			if rule.matches(snapshot) {
				matched = append(matched, rule)
				break
			}
		}
	}
	return matched
}

// Extension installs tool-focused streaming rules. State and once-per-run delivery belong to this factory invocation.
func Extension(agentDir string) func(extension.API) error {
	return func(api extension.API) error {
		api.RegisterEntryRenderer(pokeType, renderPoke)
		var mu sync.Mutex
		state := newRun(nil)
		api.OnAgentStart(func(ctx context.Context, _ extension.AgentStartEvent) error {
			cwd, err := extension.FromContext(ctx).CWD()
			if err != nil {
				return err
			}
			rules, warnings := loadRules(cwd, agentDir, readSettings(api.GetSettings()))
			for _, warning := range warnings {
				fmt.Fprintln(os.Stderr, "streaming rules:", warning)
			}
			mu.Lock()
			defer mu.Unlock()
			state = newRun(rules)
			return nil
		})
		api.OnToolCall(func(_ context.Context, event extension.ToolCallEvent) (extension.ToolCallEventResult, error) {
			call, ok := event.(extension.CustomToolCallEvent)
			if !ok {
				return extension.ToolCallEventResult{}, nil
			}
			mu.Lock()
			defer mu.Unlock()
			blocked := state.matching(call.ToolName, call.Input, true)
			var reminders []string
			for _, rule := range blocked {
				state.delivery[rule] = delivered
				reminders = append(reminders, rule.reminder(true))
			}
			if len(blocked) > 0 {
				state.pokes = append(state.pokes, pokeDraft(blocked, true))
			}
			return extension.ToolCallEventResult{Block: len(blocked) > 0, Reason: strings.Join(reminders, "\n\n")}, nil
		})
		api.OnToolResult(func(_ context.Context, event extension.ToolResultEvent) (extension.ToolResultEventResult, error) {
			result, ok := event.(extension.CustomToolResultEvent)
			if !ok || result.IsError {
				return extension.ToolResultEventResult{}, nil
			}
			mu.Lock()
			defer mu.Unlock()
			for _, rule := range state.matching(result.ToolName, result.Input, false) {
				state.delivery[rule] = pending
			}
			return extension.ToolResultEventResult{}, nil
		})
		extension.OnTurnEndSummary(api, func(_ context.Context, event extension.TurnEndEvent) (extension.TurnEndEventResult, error) {
			mu.Lock()
			defer mu.Unlock()
			pokes := state.pokes
			state.pokes = nil
			var matched []*rule
			var reminders, names []string
			for _, rule := range state.rules {
				if state.delivery[rule] != pending {
					continue
				}
				if event.Outcome != extension.AgentActivityCompleted {
					delete(state.delivery, rule)
					continue
				}
				state.delivery[rule] = delivered
				matched = append(matched, rule)
				reminders = append(reminders, rule.reminder(false))
				names = append(names, rule.name)
			}
			if len(pokes) == 0 && len(matched) == 0 {
				return extension.TurnEndEventResult{}, nil
			}
			entries := append([]extension.SessionBoundaryDraft(nil), event.Entries...)
			entries = append(entries, pokes...)
			if len(matched) > 0 {
				entries = append(entries, extension.SessionBoundaryDraft{
					Type: extension.SessionBoundaryDraftCustomMessage, CustomType: customType,
					Content: strings.Join(reminders, "\n\n"), Details: map[string]any{"rules": names},
				}, pokeDraft(matched, false))
			}
			return extension.TurnEndEventResult{Entries: &entries}, nil
		})
		return nil
	}
}
