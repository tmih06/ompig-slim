package todo

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/internal/sessionentry"
)

// TestNoTodoHistoryDoesNotSerialize pins the no-list path: irrelevant decoded
// assistant answers must not be serialized just to discover they are not todos.
func TestNoTodoHistoryDoesNotSerialize(t *testing.T) {
	entries := make([]extension.SessionEntry, 600)
	for i := range entries {
		entries[i] = sessionentry.MessageEntry{Message: agent.AgentMessage{Assistant: &agent.AssistantMessage{
			Role: "assistant", Content: []ai.AssistantContentBlock{ai.TextContent{Text: strings.Repeat("answer", 700)}},
		}}}
	}
	allocs := testing.AllocsPerRun(3, func() {
		if got := latestPhases(entries); len(got) != 0 {
			t.Fatal("ordinary answer became a todo snapshot")
		}
	})
	if allocs != 0 {
		t.Fatalf("no-todo branch allocated %g objects", allocs)
	}
}
