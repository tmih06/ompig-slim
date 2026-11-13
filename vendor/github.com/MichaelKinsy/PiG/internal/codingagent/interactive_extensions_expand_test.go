package codingagent

// Regression cover for the extension sendUserMessage seam: a caller that asks
// for prompt expansion must get it, and a caller that does not must stay
// literal. The host read only DeliverAs, so an extension forwarding a user's
// `/skill:` objective reached the model as raw slash text while a typed prompt
// in the same session expanded.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
)

// skillPromptFixture writes a skill file into cwd and returns the definition
// that names it, so an expansion reads real content.
func skillPromptFixture(t *testing.T, cwd string) *SkillDef {
	t.Helper()
	path := filepath.Join(cwd, "review.md")
	if err := os.WriteFile(path, []byte("---\nname: review\ndescription: Review code\n---\nREVIEW BODY\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &SkillDef{Name: "review", Description: "Review code", FilePath: path, BaseDir: cwd}
}

// deliveredPrompt runs one extension delivery through the real host entry
// point while a turn is active and returns the text queued for the model.
//
// The delivery is queued (steer) rather than started, which is the one path a
// fixture without a model can follow; the queued message carries exactly the
// text the next model call would read.
func deliveredPrompt(t *testing.T, options *extension.SendUserMessageOptions) string {
	t.Helper()
	mode := resetWiringTest(t, nil)
	mode.opts.CWD = t.TempDir()
	mode.opts.Skills = []*SkillDef{skillPromptFixture(t, mode.opts.CWD)}
	// No SessionHandle: the delivery then runs the input handlers through the
	// mode's own runner, and a mode with no runner leaves the text untouched.
	// A stub session would panic in RunInputHandlers instead of testing
	// expansion.

	queued, err := agent.NewAgent(agent.AgentOptions{StreamFn: func(context.Context, *ai.Model, ai.TranscriptContext, ai.StreamOptions) (*ai.AssistantMessageEventStream, error) {
		panic("the delivery must not run a model")
	}})
	if err != nil {
		t.Fatal(err)
	}
	mode.agent = queued
	// An active turn is what routes the delivery into the steering queue, and
	// it must be steered: an unspecified delivery during a run is rejected.
	mode.turnActive.Store(true)
	options = &extension.SendUserMessageOptions{ExpandPromptTemplates: options.ExpandPromptTemplates, DeliverAs: extension.DeliverAsSteer}

	if err := mode.deliverUserMessage(ai.UserText("/skill:review fix this"), options); err != nil {
		t.Fatalf("deliverUserMessage: %v", err)
	}
	// The host marshals the delivery onto the owner loop; the loop is what a
	// real mode drains between frames.
	drainOwnerLoop(mode)

	steering, followUps := mode.agent.PendingMessages()
	for _, msg := range append(steering, followUps...) {
		if msg.User == nil {
			continue
		}
		return extractAgentMessageText(msg)
	}
	t.Fatal("the delivery queued no user message")
	return ""
}

// TestDeliverUserMessageHonorsExpandPromptTemplates pins the option the host
// dropped: an extension that forwards a user objective gets the same skill
// expansion a typed prompt gets.
func TestDeliverUserMessageHonorsExpandPromptTemplates(t *testing.T) {
	got := deliveredPrompt(t, &extension.SendUserMessageOptions{ExpandPromptTemplates: new(true)})
	if !strings.Contains(got, `<skill name="review"`) || !strings.Contains(got, "REVIEW BODY") {
		t.Fatalf("the skill was not expanded: %q", got)
	}
	if !strings.HasSuffix(got, "/skill:review fix this") {
		t.Fatalf("the source prompt was not retained: %q", got)
	}
}

// TestDeliverUserMessageKeepsLiteralText covers the default: an agent's own
// message expands nothing, whether the option is absent or explicitly false.
func TestDeliverUserMessageKeepsLiteralText(t *testing.T) {
	for name, expand := range map[string]*bool{
		"nil options":   nil,
		"omitted field": nil,
		"explicit off":  new(false),
	} {
		t.Run(name, func(t *testing.T) {
			// nil options means the caller passed no options at all, which the
			// delivery helper normalizes; both shapes reach the same default.
			got := deliveredPrompt(t, &extension.SendUserMessageOptions{ExpandPromptTemplates: expand})
			if got != "/skill:review fix this" {
				t.Fatalf("delivered %q; want the literal text", got)
			}
		})
	}
}
