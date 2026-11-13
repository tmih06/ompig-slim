// Package goalext exposes the goal tracker through PiG's tool and slash-command seams.
package goalext

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"ompig-slim/internal/goal"
)

// Extension returns a hidden inline extension. The goal tool remains available
// to the model while the slash command provides the human lifecycle controls.
func Extension(api extension.API) error {
	tracker := goal.New()
	var mu sync.Mutex
	api.RegisterTool(extension.ToolDefinition{
		Name: "goal", Label: "Goal", Description: "Track one persistent autonomous objective. Use create, get, complete, pause, resume, or drop.",
		PromptSnippet: "Track one persistent autonomous objective",
		Parameters:    json.RawMessage(`{"type":"object","required":["op"],"properties":{"op":{"type":"string","enum":["create","get","complete","pause","resume","drop"]},"objective":{"type":"string"},"token_budget":{"type":"integer","minimum":1}}}`),
		Execute: func(_ context.Context, _ string, raw json.RawMessage, _ extension.AgentToolUpdateCallback) (extension.AgentToolResult, error) {
			var input struct {
				Op          string `json:"op"`
				Objective   string `json:"objective"`
				TokenBudget int    `json:"token_budget"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return extension.AgentToolResult{}, err
			}
			mu.Lock()
			defer mu.Unlock()
			var err error
			switch input.Op {
			case "create":
				err = tracker.Create(input.Objective, input.TokenBudget)
			case "get":
			case "complete":
				err = tracker.Complete()
			case "pause":
				err = tracker.Pause()
			case "resume":
				err = tracker.Resume()
			case "drop":
				err = tracker.Drop()
			default:
				err = fmt.Errorf("unknown goal operation %q", input.Op)
			}
			if err != nil {
				return extension.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: err.Error()}}, IsError: true}, nil
			}
			state := tracker.Get()
			return extension.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: render(state)}}, Details: state}, nil
		},
	})
	api.RegisterCommand("goal", extension.CommandOptions{Description: "Set, inspect, pause, resume, complete, or drop the autonomous goal.", Handler: func(ctx context.Context, args string) error {
		parts := strings.Fields(args)
		op := "show"
		if len(parts) > 0 {
			op, parts = strings.ToLower(parts[0]), parts[1:]
			if op == "show" && len(parts) > 0 {
				// `show` only acts as a command when it is used alone; with
				// arguments it is a perfectly valid prompt.
				parts = append([]string{"show"}, parts...)
				op = "__prompt"
			}
		}
		mu.Lock()
		defer mu.Unlock()
		start := func(objective string) error {
			if err := tracker.Create(objective, 0); err != nil {
				return err
			}
			// Creating the tracker must submit the objective too; otherwise the
			// user waits for an unrelated turn before goal mode starts.
			commandContext := extension.CommandContextFromContext(ctx)
			if commandContext == nil {
				_ = tracker.Drop()
				return fmt.Errorf("goal command context is unavailable")
			}
			// A goal objective is a prompt, not literal text: it reaches the same
			// skill and prompt-template expansion a typed prompt gets, and the same
			// `@path` auto-read. sendUserMessage defaults expansion off for
			// extension-originated messages, so the request is explicit here.
			if err := commandContext.SendUserMessage(objective, &extension.SendUserMessageOptions{ExpandPromptTemplates: new(true)}); err != nil {
				_ = tracker.Drop()
				return err
			}
			return nil
		}
		switch op {
		case "set", "create":
			return start(strings.Join(parts, " "))
		case "show":
			fmt.Println(render(tracker.Get()))
		case "pause":
			if err := tracker.Pause(); err != nil {
				return err
			}
		case "resume":
			if err := tracker.Resume(); err != nil {
				return err
			}
		case "complete":
			if err := tracker.Complete(); err != nil {
				return err
			}
		case "drop":
			if err := tracker.Drop(); err != nil {
				return err
			}
		default:
			// `/goal <prompt>` is the short form of `/goal set <prompt>`.
			objective := strings.Join(append([]string{op}, parts...), " ")
			if op == "__prompt" {
				objective = strings.Join(parts, " ")
			}
			return start(objective)
		}
		return nil
	}})
	api.OnAgentBeforeSettle(func(_ context.Context, _ *extension.AgentBeforeSettleEvent) (extension.AgentBeforeSettleEventResult, error) {
		mu.Lock()
		defer mu.Unlock()
		return continueActiveGoal(tracker, api.SendMessage), nil
	})
	return nil
}

// continueActiveGoal queues the next turn at the settlement boundary. Returning
// continuation rather than triggering a message after settlement keeps observers
// such as agent-state in working state until the goal is complete or paused.
func continueActiveGoal(tracker *goal.Tracker, send func(extension.SendMessagePayload, *extension.SendMessageOptions)) extension.AgentBeforeSettleEventResult {
	state := tracker.Get()
	if state == nil || state.Status != goal.StatusActive {
		return extension.AgentBeforeSettleEventResult{}
	}
	send(extension.SendMessagePayload{
		CustomType: "goal-continuation",
		Content:    fmt.Sprintf("Continue working toward this goal: %s", state.Objective),
		Display:    false,
	}, &extension.SendMessageOptions{DeliverAs: extension.DeliverAsNextTurn})
	continueTurn := true
	return extension.AgentBeforeSettleEventResult{Continue: &continueTurn}
}

// render formats the compact model and command response shared by both surfaces.
func render(state *goal.State) string {
	if state == nil {
		return "No active goal."
	}
	budget := "unbounded"
	if state.TokenBudget > 0 {
		budget = fmt.Sprintf("%d", state.TokenBudget)
	}
	return fmt.Sprintf("Goal: %s\nStatus: %s\nTokens: %d / %s", state.Objective, state.Status, state.TokensUsed, budget)
}
