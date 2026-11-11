package codingagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TestDequeueSkillsRestoresSource exercises the queue-to-editor boundary used
// by Alt+Up: instructions stay model-only, tokens stay editable, and resubmitting
// an edited prompt loads the skills exactly once again.
func TestDequeueSkillsRestoresSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review.md")
	if err := os.WriteFile(path, []byte("---\nname: review\n---\nPRIVATE INSTRUCTIONS"), 0600); err != nil {
		t.Fatal(err)
	}
	skills := []*SkillDef{{Name: "review", FilePath: path, BaseDir: dir}}
	source := "please /skill:review fix this /skill:review"
	expanded, ok, err := ExpandSkillCommand(source, skills)
	if err != nil || !ok {
		t.Fatalf("expansion: %v %v", ok, err)
	}
	for _, followUp := range []bool{false, true} {
		mode := resetWiringTest(t, nil)
		queuedAgent, agentErr := agent.NewAgent(agent.AgentOptions{StreamFn: func(context.Context, *ai.Model, ai.TranscriptContext, ai.StreamOptions) (*ai.AssistantMessageEventStream, error) {
			panic("queue restoration must not run a model")
		}})
		if agentErr != nil {
			t.Fatal(agentErr)
		}
		mode.agent = queuedAgent
		mode.editor.SetAutocomplete(tui.NewCombinedProvider([]tui.SlashCommand{{Name: "skill:review"}}, dir, ""))
		msg := agent.AgentMessage{User: &agent.UserMessage{Role: agent.RoleUser, Content: ai.UserContentBlocks{ai.TextContent{Text: expanded}}}}
		if followUp {
			mode.agent.FollowUp(msg)
		} else {
			mode.agent.Steer(msg)
		}
		mode.compactionQueue = []compactionQueuedMessage{{text: "after compaction"}}
		mode.editor.SetText("current draft")
		if n := mode.restoreQueuedMessagesToEditor(false); n != 2 {
			t.Fatalf("restored %d prompts", n)
		}
		want := source + "\n\nafter compaction\n\ncurrent draft"
		if followUp {
			want = "after compaction\n\n" + source + "\n\ncurrent draft"
		}
		if got := mode.editor.Text(); got != want {
			t.Fatalf("Alt+Up restored skill body instead of source: %q", got)
		}
		display := widthx.StripAnsi(strings.Join(mode.editor.Render(120), "\n"))
		if strings.Contains(display, "PRIVATE INSTRUCTIONS") || !strings.Contains(display, "◆ review") {
			t.Fatalf("bad editor display: %q", display)
		}
		steering, followUps := mode.agent.PendingMessages()
		if len(steering)+len(followUps)+len(mode.compactionQueue) != 0 {
			t.Fatal("restored prompts still queued")
		}
		edited, ok, err := ExpandSkillCommand(mode.editor.Text()+" edited", skills)
		if err != nil || !ok || strings.Count(edited, "PRIVATE INSTRUCTIONS") != 1 {
			t.Fatalf("resubmission: %q %v", edited, err)
		}
	}
}
