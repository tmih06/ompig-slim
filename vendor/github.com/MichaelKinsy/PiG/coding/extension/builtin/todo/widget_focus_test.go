package todo

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/tui"
)

// TestTodoWidgetReopensAtWorkingTask uses the real card and widget host. It
// covers both disclosures, late phases, wrapped context and work advanced while
// hidden, without relying on theme colors to locate the active task.
func TestTodoWidgetReopensAtWorkingTask(t *testing.T) {
	for _, width := range []int{24, 40} {
		for _, section := range []bool{false, true} {
			t.Run(fmt.Sprintf("width=%d/section=%v", width, section), func(t *testing.T) {
				phases := []Phase{{Name: "Finished"}, {Name: "Working"}}
				for i := 0; i < 20; i++ {
					phases[0].Tasks = append(phases[0].Tasks, Task{Content: fmt.Sprintf("Completed context %02d 界 with wrapping", i), Status: StatusCompleted})
				}
				phases[1].Tasks = []Task{{Content: "Next task", Status: StatusPending}, {Content: "Current work", Status: StatusInProgress}, {Content: "Future work", Status: StatusPending}}
				d := todoDetails{phases: phases}
				def := Definition()
				card := tui.NewToolExecutionComponent("todo", "widget", d.args(), tui.ToolExecutionOptions{}, nil, nil, "/tmp")
				card.SetDefinition(&tui.ToolDefinitionRenderers{
					Unframed: true,
					Call:     func(tui.ToolRenderInput) (tui.Component, bool) { return blankComponent(), true },
					Result: func(input tui.ToolRenderInput) (tui.Component, bool) {
						return def.RenderResult(extension.AgentToolResult{Details: d.value()}, extension.ToolRenderResultOptions{Expanded: input.Expanded}, tui.ActiveTheme(), extension.ToolRenderContext{}), true
					},
				}, d.args())
				card.SetArgsComplete()
				card.SetResult("summary", false, 0)
				w := &tui.InfoWidgets{Sections: []tui.InfoWidget{{Key: "todo", Title: "Tasks", Render: func(width int, expanded bool) []string {
					card.SetExpanded(expanded)
					rows := card.Render(width)
					for len(rows) > 0 && strings.TrimSpace(plain(rows[0])) == "" {
						rows = rows[1:]
					}
					for len(rows) > 0 && strings.TrimSpace(plain(rows[len(rows)-1])) == "" {
						rows = rows[:len(rows)-1]
					}
					return rows
				}, FocusRow: card.ResultFocusRow}}}
				assertVisible := func(task string) {
					t.Helper()
					rows := w.Render(width)
					if !strings.Contains(plain(strings.Join(rows, "\n")), task) {
						t.Fatalf("working task %q not visible:\n%s", task, strings.Join(rows, "\n"))
					}
				}
				toggle := func() {
					if section {
						w.HandleMouse(tui.TuiMouseEvent{Type: tui.MouseClick, Button: tui.MouseButtonLeft, Y: 0})
					} else {
						w.Toggle()
					}
				}
				assertVisible("Current work")
				w.Scroll(-1000)
				before := strings.Join(w.Render(width), "\n")
				if !strings.Contains(before, "Completed") {
					t.Fatal("manual scrolling was overridden")
				}
				toggle()
				w.Render(width)
				toggle()
				assertVisible("Current work")
				toggle()
				w.Render(width)
				phases[1].Tasks[1].Status = StatusCompleted
				phases[1].Tasks[2].Status = StatusInProgress
				card.Invalidate()
				toggle()
				assertVisible("Future work")
			})
		}
	}
}
