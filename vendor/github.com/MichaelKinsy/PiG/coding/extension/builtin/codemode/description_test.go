package codemode

import (
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension"
)

// TestCallableToolsExcludesTranscriptStatefulTodo prevents codemode scripts from
// chaining todo mutations whose intermediate results are not yet in the session
// branch. Direct todo calls remain available to the agent loop.
func TestCallableToolsExcludesTranscriptStatefulTodo(t *testing.T) {
	got := callableTools([]extension.AgentTool{
		{Name: "todo"},
		{Name: ToolName},
		{Name: "read"},
	})
	if len(got) != 1 || got[0].Name != "read" {
		t.Fatalf("callable tools = %#v, want only read", got)
	}
}
