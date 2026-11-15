// Package goalext exposes the goal tracker through PiG's tool and slash-command seams.
package goalext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"ompig-slim/internal/goal"
)

// Extension returns a hidden inline extension. The goal tool remains available
// to the model while the slash command provides the human lifecycle controls.
func Extension(api extension.API) error { return extensionWithTracker(api, goal.New()) }

// extensionWithTracker is Extension over a caller-supplied tracker, so a test
// can observe the state the tool and slash command report.
func extensionWithTracker(api extension.API, tracker *goal.Tracker) error {
	var mu sync.Mutex
	api.RegisterTool(extension.ToolDefinition{
		Name: "goal", Label: "Goal", Description: "Track one persistent autonomous objective. Use create, get, complete, pause, resume, drop, blocked, or progress. Report blocked when the objective needs input or access you do not have, and progress when a turn advanced it without finishing.",
		PromptSnippet: "Track one persistent autonomous objective",
		PromptGuidelines: []string{
			"Report a blocker with the goal tool instead of re-explaining it in prose: `blocked` names the missing input or access, and three reports of the same blocker pause the goal",
			"Report `progress` after a turn that advanced the goal, including read-only research, tests, or verification that changed no file",
		},
		Parameters: json.RawMessage(`{"type":"object","required":["op"],"properties":{"op":{"type":"string","enum":["create","get","complete","pause","resume","drop","blocked","progress"]},"objective":{"type":"string"},"token_budget":{"type":"integer","minimum":1},"reason":{"type":"string","description":"What the goal is missing: the input, access, or decision only the user can supply"}}}`),
		Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ extension.AgentToolUpdateCallback) (extension.AgentToolResult, error) {
			var input struct {
				Op          string `json:"op"`
				Objective   string `json:"objective"`
				TokenBudget int    `json:"token_budget"`
				Reason      string `json:"reason"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return extension.AgentToolResult{}, err
			}
			mu.Lock()
			defer mu.Unlock()
			var err error
			// extra is the one notice a transition shows a human. It is returned
			// once, with the transition that caused it, never on a later read.
			var extra string
			switch input.Op {
			case "create":
				err = tracker.Create(input.Objective, input.TokenBudget)
			case "get":
			case "blocked":
				var state *goal.State
				state, err = tracker.Blocked(input.Reason)
				if err == nil && state.Status == goal.StatusPaused {
					extra = pausedNotice(state)
					notify(ctx, extra, "warning")
				}
			case "progress":
				err = tracker.Progress()
			case "complete":
				err = tracker.Complete()
			case "pause":
				err = tracker.Pause()
			case "resume":
				_, err = tracker.Resume()
			case "drop":
				err = tracker.Drop()
			default:
				err = fmt.Errorf("unknown goal operation %q", input.Op)
			}
			if err != nil {
				return extension.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: err.Error()}}, IsError: true}, nil
			}
			return toolResult(tracker.Get(), extra), nil
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
			if err := resumeGoal(tracker, func(objective string) error {
				commandContext := extension.CommandContextFromContext(ctx)
				if commandContext == nil {
					return fmt.Errorf("goal command context is unavailable")
				}
				return commandContext.SendUserMessage(objective, &extension.SendUserMessageOptions{ExpandPromptTemplates: new(true)})
			}); err != nil {
				return err
			}
			// The streak is cleared, so the working label no longer describes the goal.
			setWorkingMessage(ctx, "")
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
	api.OnAgentBeforeSettle(func(ctx context.Context, _ *extension.AgentBeforeSettleEvent) (extension.AgentBeforeSettleEventResult, error) {
		mu.Lock()
		defer mu.Unlock()
		return continueActiveGoal(ctx, tracker, api.SendMessage), nil
	})
	api.OnAgentSettled(func(ctx context.Context, event extension.AgentSettledEvent) error {
		mu.Lock()
		defer mu.Unlock()
		pauseAbortedGoal(tracker, event.Aborted)
		if state := tracker.Get(); state == nil || state.Status != goal.StatusActive {
			setWorkingMessage(ctx, "")
		}
		return nil
	})
	return nil
}

// resumeGoal reactivates a paused goal and starts its objective, so an explicit
// resume works from an idle session instead of waiting for a turn that would
// reach the settlement boundary. It starts nothing when the goal was already
// active, so repeated resumes cannot duplicate the objective.
func resumeGoal(tracker *goal.Tracker, submit func(objective string) error) error {
	state := tracker.Get()
	if state == nil {
		return errors.New("there is no active goal")
	}
	restart, err := tracker.Resume()
	if err != nil || !restart {
		return err
	}
	if err := submit(state.Objective); err != nil {
		// A resume that cannot start work must not report the goal as running.
		_ = tracker.Pause()
		return err
	}
	return nil
}

// pauseAbortedGoal prevents an Escape/cancellation from being mistaken for a
// completed goal. The next run must be an explicit resume, not an automatic poke.
func pauseAbortedGoal(tracker *goal.Tracker, aborted bool) {
	if aborted {
		_ = tracker.Pause()
	}
}

// setWorkingMessage exposes the poke count beside the live working indicator in
// interactive mode. Other modes bind a no-op UI, so the goal lifecycle remains
// independent of presentation.
func setWorkingMessage(ctx context.Context, message string) {
	if extCtx := extension.FromContext(ctx); extCtx != nil {
		if ui, err := extCtx.UI(); err == nil {
			ui.SetWorkingMessage(message)
		}
	}
}

// continueActiveGoal queues a follow-up at the settlement boundary. Returning
// continuation lets the active run consume it without waiting for a user prompt.
func continueActiveGoal(ctx context.Context, tracker *goal.Tracker, send func(extension.SendMessagePayload, *extension.SendMessageOptions)) extension.AgentBeforeSettleEventResult {
	state := tracker.Get()
	if state == nil || state.Status != goal.StatusActive {
		return extension.AgentBeforeSettleEventResult{}
	}
	pokes := tracker.Poke()
	setWorkingMessage(ctx, fmt.Sprintf("Working · goal pokes: %d", pokes))
	send(extension.SendMessagePayload{
		CustomType: "goal-continuation",
		Content:    fmt.Sprintf("Goal poke %d: Continue working toward this goal: %s", pokes, state.Objective),
		Display:    true,
	}, &extension.SendMessageOptions{DeliverAs: extension.DeliverAsFollowUp})
	continueTurn := true
	return extension.AgentBeforeSettleEventResult{Continue: &continueTurn}
}

// pausedNotice is the one actionable message a threshold pause shows. It names
// what the goal is missing and the exact command that resumes it.
func pausedNotice(state *goal.State) string {
	return fmt.Sprintf("Goal paused after %d reports of the same blocker: %s. Supply it, then run /goal resume.", state.BlockerCount, state.Blocker)
}

// toolResult formats a goal response. extra carries a one-time notice, so the
// transition that paused the goal explains itself without repeating on a read.
func toolResult(state *goal.State, extra string) extension.AgentToolResult {
	text := render(state)
	if extra != "" {
		text += "\n\n" + extra
	}
	return extension.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: text}}, Details: state}
}

// notify shows a human-facing notice. Every other mode binds a no-op UI, so the
// goal lifecycle stays independent of presentation.
func notify(ctx context.Context, message, kind string) {
	if extCtx := extension.FromContext(ctx); extCtx != nil {
		if ui, err := extCtx.UI(); err == nil {
			ui.Notify(message, kind)
		}
	}
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
	out := fmt.Sprintf("Goal: %s\nStatus: %s\nTokens: %d / %s\nPokes: %d", state.Objective, state.Status, state.TokensUsed, budget, state.Pokes)
	if state.Blocker != "" {
		// The text outlives the streak: it is the explanation of why the goal
		// paused, and a resumed goal still needs it in its status.
		out += fmt.Sprintf("\nBlocker: %s", state.Blocker)
		if state.BlockerCount > 0 {
			out += fmt.Sprintf(" (reported %d×)", state.BlockerCount)
		}
	}
	return out
}
