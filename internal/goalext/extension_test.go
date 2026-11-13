package goalext

import (
	"context"
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/coding/extension/factoryload"
	"github.com/MichaelKinsy/PiG/coding/extension/host/inproc"

	"ompig-slim/internal/goal"
)

// TestActiveGoalRequestsContinuationBeforeSettlement pins the boundary contract:
// an unmet goal keeps the run working instead of first reporting idle.
func TestActiveGoalRequestsContinuationBeforeSettlement(t *testing.T) {
	tracker := goal.New()
	if err := tracker.Create("finish the work", 0); err != nil {
		t.Fatal(err)
	}
	var sent extension.SendMessagePayload
	result := continueActiveGoal(context.Background(), tracker, func(payload extension.SendMessagePayload, options *extension.SendMessageOptions) {
		sent = payload
		if options == nil || options.DeliverAs != extension.DeliverAsNextTurn {
			t.Fatalf("continuation options = %+v, want nextTurn", options)
		}
	})
	if result.Continue == nil || !*result.Continue {
		t.Fatal("an unmet active goal must continue before agent_settled")
	}
	if sent.CustomType != "goal-continuation" {
		t.Fatalf("custom message type = %q, want goal-continuation", sent.CustomType)
	}
	if !sent.Display {
		t.Fatal("goal continuation is hidden from the transcript")
	}
	if sent.Content != "Goal poke 1: Continue working toward this goal: finish the work" {
		t.Fatalf("continuation content = %q", sent.Content)
	}
	if got := tracker.Get().Pokes; got != 1 {
		t.Fatalf("goal pokes = %d, want 1", got)
	}
}

func TestAbortedGoalPausesBeforeAnotherPoke(t *testing.T) {
	tracker := goal.New()
	if err := tracker.Create("finish the work", 0); err != nil {
		t.Fatal(err)
	}
	pauseAbortedGoal(tracker, true)
	if state := tracker.Get(); state == nil || state.Status != goal.StatusPaused {
		t.Fatalf("aborted goal state = %+v, want paused", state)
	}
	result := continueActiveGoal(context.Background(), tracker, func(extension.SendMessagePayload, *extension.SendMessageOptions) {
		t.Fatal("paused goal was poked after interruption")
	})
	if result.Continue != nil && *result.Continue {
		t.Fatal("paused goal requested continuation")
	}
}

// TestGoalSubmitsSkillObjective exercises the real command runner. Goal must
// forward slash skill syntax to the host's normal prompt/resource expansion.
func TestGoalSubmitsSkillObjective(t *testing.T) {
	for _, args := range []string{"/skill:review fix this", "set /skill:review fix this", "create /skill:review fix this"} {
		t.Run(args, func(t *testing.T) {
			root := t.TempDir()
			loaded, err := factoryload.LoadExtensionFromFactory(Extension, root, extension.CreateEventBus(), extension.CreateExtensionRuntime(), "<inline:goal>")
			if err != nil {
				t.Fatal(err)
			}
			runner := inproc.NewRunner([]extension.Extension{loaded}, root)
			var submitted any
			var expand *bool
			runner.BindCore(extension.ExtensionActions{SendUserMessage: func(content any, opts *extension.SendUserMessageOptions) error {
				submitted = content
				if opts != nil {
					expand = opts.ExpandPromptTemplates
				}
				return nil
			}}, extension.ContextActions{}, nil)
			if !runner.ExecuteCommand(context.Background(), "goal", args) {
				t.Fatal("goal command not handled")
			}
			if submitted != "/skill:review fix this" {
				t.Fatalf("submitted = %q; want skill objective", submitted)
			}
			// A goal objective is a prompt, so the host must run the same
			// skill/template expansion a typed prompt gets. Without the explicit
			// request, an extension-originated message defaults to no expansion
			// and a `/skill:` objective reaches the model as literal text.
			if expand == nil || !*expand {
				t.Fatalf("ExpandPromptTemplates = %v; want an explicit true", expand)
			}
		})
	}
}
