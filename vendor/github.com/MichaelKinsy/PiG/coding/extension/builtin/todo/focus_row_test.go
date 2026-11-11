package todo

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension"
)

// TestTodoFocusRowFallbacks pins selection independently of viewport placement:
// active work wins over earlier pending work; without either, show the tail.
func TestTodoFocusRowFallbacks(t *testing.T) {
	for _, test := range []struct {
		name  string
		tasks []Task
		want  string
	}{
		{"active", []Task{{Content: "pending", Status: StatusPending}, {Content: "active", Status: StatusInProgress}}, "active"},
		{"pending", []Task{{Content: "done", Status: StatusCompleted}, {Content: "pending", Status: StatusPending}, {Content: "later", Status: StatusPending}}, "pending"},
		{"settled", []Task{{Content: "done", Status: StatusCompleted}, {Content: "tail", Status: StatusAbandoned}}, "tail"},
		{"blocked", []Task{{Content: "blocked", Status: StatusBlocked}, {Content: "tail", Status: StatusBlocked}}, "tail"},
	} {
		t.Run(test.name, func(t *testing.T) {
			component := renderResult(extension.AgentToolResult{Details: Details{Phases: []Phase{{Name: "Plan", Tasks: test.tasks}}}}, extension.ToolRenderResultOptions{Expanded: true}, nil, extension.ToolRenderContext{}).(linesComponent)
			rows := component.Render(40)
			row := component.FocusRow(40)
			if row < 0 || row >= len(rows) || !strings.Contains(plain(rows[row]), test.want) {
				t.Fatalf("focus row %d should show %q: %v", row, test.want, rows)
			}
		})
	}
}
