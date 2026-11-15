package codemode

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension"
)

func TestDirectOnlyGuidanceSurvivesBudget(t *testing.T) {
	for _, budget := range []int{0, 1, 3000} {
		description, err := CreateDescription([]extension.AgentTool{{Name: "todo"}, {Name: "read"}}, DescriptionOptions{InlineBudget: &budget})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(description, "`todo` is direct-only") || strings.Contains(description, "tools.todo(") {
			t.Fatalf("budget %d lost direct-only guidance: %s", budget, description)
		}
	}
}

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
