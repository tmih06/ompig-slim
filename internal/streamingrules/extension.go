package streamingrules

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
)

const customType = "streaming-rule-reminder"

type observedCall struct {
	snapshots []snapshot
	rules     []*rule
	size      int
}

type runState struct {
	rules     []*rule
	observed  map[string]observedCall
	pending   map[string]*rule
	delivered map[string]bool
	pokes     []extension.SessionBoundaryDraft
}

func newRun(rules []*rule) runState {
	return runState{
		rules: rules, observed: map[string]observedCall{},
		pending: map[string]*rule{}, delivered: map[string]bool{},
	}
}

func (s *runState) matching(id, tool string, args map[string]any, streaming bool) []*rule {
	if len(s.rules) == 0 {
		return nil
	}
	snapshots := toolSnapshots(tool, args)
	previous, ok := s.observed[id]
	if ok && slices.Equal(snapshots, previous.snapshots) {
		return previous.rules
	}
	size := 0
	for _, snapshot := range snapshots {
		size += len(snapshot.text)
	}
	if streaming && ok && size > 4096 && size < previous.size+previous.size/4 {
		return nil
	}
	var matched []*rule
	for _, rule := range s.rules {
		for _, snapshot := range snapshots {
			if rule.matches(snapshot) {
				matched = append(matched, rule)
				break
			}
		}
	}
	if streaming {
		s.observed[id] = observedCall{snapshots: snapshots, rules: matched, size: size}
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
		api.OnMessageStart(func(_ context.Context, _ extension.MessageStartEvent) error {
			mu.Lock()
			defer mu.Unlock()
			state.observed = map[string]observedCall{}
			return nil
		})
		api.OnMessageUpdate(func(_ context.Context, event extension.MessageUpdateEvent) error {
			mu.Lock()
			defer mu.Unlock()
			if len(state.rules) == 0 {
				return nil
			}
			switch event := event.AssistantMessageEvent.(type) {
			case ai.StartEvent:
				state.observed = map[string]observedCall{}
			case ai.ToolCallDeltaEvent:
				if event.Partial != nil && event.ContentIndex >= 0 && event.ContentIndex < len(event.Partial.Content) {
					if call, ok := event.Partial.Content[event.ContentIndex].(ai.ToolCall); ok {
						state.matching(call.ID, call.Name, call.Arguments, true)
					}
				}
			case ai.ToolCallEndEvent:
				state.matching(event.ToolCall.ID, event.ToolCall.Name, event.ToolCall.Arguments, true)
			}
			return nil
		})
		api.OnToolCall(func(_ context.Context, event extension.ToolCallEvent) (extension.ToolCallEventResult, error) {
			call, ok := event.(extension.CustomToolCallEvent)
			if !ok {
				return extension.ToolCallEventResult{}, nil
			}
			mu.Lock()
			defer mu.Unlock()
			var blocked []*rule
			for _, rule := range state.matching(call.ToolCallID, call.ToolName, call.Input, false) {
				if rule.interrupt && !state.delivered[rule.name] {
					blocked = append(blocked, rule)
				}
			}
			var reminders []string
			for _, rule := range blocked {
				state.delivered[rule.name] = true
				delete(state.pending, rule.name)
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
			for _, rule := range state.matching(result.ToolCallID, result.ToolName, result.Input, false) {
				if !state.delivered[rule.name] {
					state.pending[rule.name] = rule
				}
			}
			return extension.ToolResultEventResult{}, nil
		})
		extension.OnTurnEndSummary(api, func(_ context.Context, event extension.TurnEndEvent) (extension.TurnEndEventResult, error) {
			mu.Lock()
			defer mu.Unlock()
			defer clear(state.pending)
			pokes := state.pokes
			state.pokes = nil
			var matched []*rule
			var reminders, names []string
			if event.Outcome == extension.AgentActivityCompleted {
				for _, rule := range state.rules {
					if state.pending[rule.name] == nil || state.delivered[rule.name] {
						continue
					}
					state.delivered[rule.name] = true
					matched = append(matched, rule)
					reminders = append(reminders, rule.reminder(false))
					names = append(names, rule.name)
				}
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
