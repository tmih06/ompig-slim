package todo

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/tui"
)

// TestCardIsUnframed pins the presentation contract: the todo card draws OMP's
// content (the `Todo N tasks` line and the phase tree) and nothing else. No
// border, no status bar and no lifecycle background, because the rows already
// carry the structure a reader needs and the frame only adds a box and a
// repeated title.
//
// The assertions read the raw rows rather than the stripped ones: a frame is
// made of box-drawing glyphs and a background tint, and both are only visible
// before styling is removed.
func TestCardIsUnframed(t *testing.T) {
	def := Definition()
	if def.RenderShell != extension.ToolRenderShellUnframed {
		t.Fatalf("todo must opt out of the tool-card frame, got renderShell %q", def.RenderShell)
	}

	d := todoDetails{phases: []Phase{
		{Name: "Foundation", Tasks: []Task{
			{Content: "port the model", Status: StatusCompleted},
			{Content: "port the reminder", Status: StatusInProgress},
		}},
		{Name: "Verify", Tasks: []Task{{Content: "run tests", Status: StatusPending}}},
	}}
	result := extension.AgentToolResult{Details: d.value(), Content: d.content()}
	comp := tui.NewToolExecutionComponent("todo", "call-1", d.args(),
		tui.ToolExecutionOptions{Rich: true}, nil, nil, "/tmp")
	comp.SetDefinition(&tui.ToolDefinitionRenderers{
		Unframed: true,
		Call: func(input tui.ToolRenderInput) (tui.Component, bool) {
			return def.RenderCall(input.Args, tui.ActiveTheme(), extension.ToolRenderContext{Cwd: "/tmp", IsPartial: false}), true
		},
		Result: func(input tui.ToolRenderInput) (tui.Component, bool) {
			return def.RenderResult(result, extension.ToolRenderResultOptions{}, tui.ActiveTheme(),
				extension.ToolRenderContext{Cwd: "/tmp", Args: input.Args}), true
		},
	}, d.args())
	comp.SetArgsComplete()
	comp.SetResultValue(result)
	comp.SetResult("summary", false, 0)

	rows := comp.Render(70)
	if len(rows) == 0 {
		t.Fatal("card drew no rows")
	}

	// No frame: the rounded bar glyphs and the vertical border.
	for _, row := range rows {
		for _, chrome := range []string{"╭", "╮", "╰", "╯", "│"} {
			if strings.Contains(row, chrome) {
				t.Errorf("unframed card must not draw the card border, found %q in %q", chrome, row)
			}
		}
	}

	// No lifecycle background: the tint opens with a 48;2 (truecolor) or 48;5
	// (256-color) SGR, and the frame is the only place the card applies it.
	for _, row := range rows {
		if strings.Contains(row, "\x1b[48;2;") || strings.Contains(row, "\x1b[48;5;") {
			t.Errorf("unframed card must not paint a background, got %q", row)
		}
	}

	// The content itself is intact.
	joined := strings.Join(rows, "\n")
	for _, want := range []string{"☑", "Todo", "3 tasks", "I. Foundation", "1/2", "port the reminder", "II. Verify"} {
		if !strings.Contains(joined, want) {
			t.Errorf("unframed card should still draw %q, got:\n%s", want, joined)
		}
	}
}

// TestCardKeepsContentWhenExpanded pins that dropping the frame did not drop the
// expand behaviour: the collapsed card windows the list and Ctrl+O shows it all.
func TestCardKeepsContentWhenExpanded(t *testing.T) {
	many := make([]Task, 0, collapsedItems+4)
	for i := 0; i < collapsedItems+4; i++ {
		many = append(many, Task{Content: "task number " + strings.Repeat("x", i+1), Status: StatusPending})
	}
	d := todoDetails{phases: []Phase{{Name: "Tasks", Tasks: many}}}

	collapsed := renderCard(t, d, false)
	expanded := renderCard(t, d, true)
	if len(collapsed) >= len(expanded) {
		t.Fatalf("expanding should disclose more rows: collapsed %d, expanded %d", len(collapsed), len(expanded))
	}
	// Collapsed windows the tail with a "more" marker; expanded lists every task.
	if !strings.Contains(strings.Join(collapsed, "\n"), "more") {
		t.Errorf("collapsed card should summarize the hidden tasks, got:\n%s", strings.Join(collapsed, "\n"))
	}
}
