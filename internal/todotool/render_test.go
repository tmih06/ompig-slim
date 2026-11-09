package todotool

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/tui"

	"ompig-slim/internal/todo"
)

// plain strips SGR escapes so an assertion can talk about layout rather than
// colors, which are asserted separately where they carry meaning.
func plain(line string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range line {
		switch {
		case r == 0x1b:
			inEscape = true
		case inEscape:
			if r == 'm' {
				inEscape = false
			}
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// renderCard draws rows through the real card component, so the layout under
// test is the one a user sees: the call region, the result region, and the tree
// gutters between them.
func renderCard(t *testing.T, details todoDetails, expanded bool) []string {
	t.Helper()
	def := Definition()
	result := extension.AgentToolResult{Details: details.value(), Content: details.content()}
	comp := tui.NewToolExecutionComponent("todo", "call-1", details.args(),
		tui.ToolExecutionOptions{Rich: true}, nil, nil, "/tmp")
	comp.SetDefinition(&tui.ToolDefinitionRenderers{
		Unframed: def.RenderShell == extension.ToolRenderShellUnframed,
		Call: func(input tui.ToolRenderInput) (tui.Component, bool) {
			return def.RenderCall(input.Args, tui.ActiveTheme(), extension.ToolRenderContext{
				Cwd: "/tmp", Args: input.Args, IsPartial: input.IsPartial, Expanded: input.Expanded,
			}), true
		},
		Result: func(input tui.ToolRenderInput) (tui.Component, bool) {
			return def.RenderResult(result, extension.ToolRenderResultOptions{Expanded: expanded}, tui.ActiveTheme(),
				extension.ToolRenderContext{Cwd: "/tmp", Args: input.Args, Expanded: expanded, IsPartial: false}), true
		},
	}, details.args())
	comp.SetArgsComplete()
	comp.SetResultValue(result)
	comp.SetResult("summary", false, 0)
	if expanded {
		comp.SetExpanded(true)
	}
	rows := comp.Render(72)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if trimmed := plain(row); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// TestRenderCardLayout pins the card's shape against OMP's todoToolRenderer: one
// `Todo N tasks` line, roman-numeral phase headers when there are several phases,
// and `├─`/`└─` tree gutters on task rows.
func TestRenderCardLayout(t *testing.T) {
	d := todoDetails{phases: []todo.Phase{
		{Name: "Foundation", Tasks: []todo.Task{
			{Content: "port the model", Status: todo.StatusCompleted},
			{Content: "port the reminder", Status: todo.StatusInProgress},
		}},
		{Name: "Verify", Tasks: []todo.Task{
			{Content: "run tests", Status: todo.StatusPending},
		}},
	}}
	rows := renderCard(t, d, true)

	// Exactly one header line: OMP merges call and result, so a settled card must
	// not keep a stale `● Todo` line above the task list.
	headers := 0
	for _, row := range rows {
		if strings.Contains(row, "Todo") && strings.Contains(row, "tasks") {
			headers++
		}
	}
	if headers != 1 {
		t.Fatalf("expected exactly one `Todo N tasks` header, got %d in:\n%s", headers, strings.Join(rows, "\n"))
	}
	joined := strings.Join(rows, "\n")
	for _, want := range []string{"I. Foundation", "II. Verify", "├─", "└─", "3 tasks"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("card should contain %q, got:\n%s", want, joined)
		}
	}
}

// TestRenderCardCollapsesUntouchedPhases checks the behavior a single task flip
// depends on: phases this update did not touch collapse to a one-line summary
// instead of redrawing every task.
func TestRenderCardCollapsesUntouchedPhases(t *testing.T) {
	d := todoDetails{
		phases: []todo.Phase{
			{Name: "Foundation", Tasks: []todo.Task{
				{Content: "active work", Status: todo.StatusInProgress},
			}},
			{Name: "Verify", Tasks: []todo.Task{
				{Content: "run tests", Status: todo.StatusPending},
				{Content: "check output", Status: todo.StatusPending},
			}},
		},
		op: todo.OpDone,
	}
	rows := renderCard(t, d, false)
	joined := strings.Join(rows, "\n")

	// The untouched phase keeps its header plus progress and draws no task rows.
	if !strings.Contains(joined, "II. Verify") || !strings.Contains(joined, "0/2") {
		t.Fatalf("untouched phase should keep a progress summary, got:\n%s", joined)
	}
	if strings.Contains(joined, "run tests") {
		t.Fatalf("untouched phase should not list its tasks, got:\n%s", joined)
	}
	// The touched phase still lists its work.
	if !strings.Contains(joined, "active work") {
		t.Fatalf("the active phase should list its tasks, got:\n%s", joined)
	}
}

// TestRenderCardStatusMarkers checks that each status is visually distinct: a
// dropped task must not look finished, and a blocked task must carry its reason.
func TestRenderCardStatusMarkers(t *testing.T) {
	d := todoDetails{phases: []todo.Phase{{Name: "Tasks", Tasks: []todo.Task{
		{Content: "did it", Status: todo.StatusCompleted},
		{Content: "doing it", Status: todo.StatusInProgress},
		{Content: "waiting", Status: todo.StatusBlocked, Blocker: "needs a decision"},
		{Content: "gave up", Status: todo.StatusAbandoned},
		{Content: "todo", Status: todo.StatusPending},
	}}}}
	rows := renderCard(t, d, true)
	joined := strings.Join(rows, "\n")

	for _, want := range []string{
		symCheckboxChecked + " did it",     // completed: checked box
		symCheckboxUnchecked + " doing it", // in progress
		symCheckboxUnchecked + " waiting (blocked: needs a decision)",
		symCheckboxUnchecked + " gave up", // dropped: unchecked, struck through
		symCheckboxUnchecked + " todo",    // pending
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("card should contain %q, got:\n%s", want, joined)
		}
	}
	// A dropped task must not read as completed, so it may not use the checked box.
	for _, row := range rows {
		if strings.Contains(row, "gave up") && strings.Contains(row, symCheckboxChecked) {
			t.Fatalf("dropped task must not use the checked box: %q", row)
		}
	}
}

// TestRenderCardError checks the error card: an error header and the failure
// text, with no task tree.
func TestRenderCardError(t *testing.T) {
	def := Definition()
	comp := tui.NewToolExecutionComponent("todo", "c", nil, tui.ToolExecutionOptions{Rich: true}, nil, nil, "/tmp")
	comp.SetDefinition(&tui.ToolDefinitionRenderers{
		Unframed: def.RenderShell == extension.ToolRenderShellUnframed,
		Call:     func(tui.ToolRenderInput) (tui.Component, bool) { return blankComponent(), true },
		Result: func(tui.ToolRenderInput) (tui.Component, bool) {
			return def.RenderResult(extension.AgentToolResult{
				Content: []ai.ToolResultMessageContent{ai.TextContent{Text: `Errors: Task "x" not found`}},
				IsError: true,
			}, extension.ToolRenderResultOptions{}, tui.ActiveTheme(), extension.ToolRenderContext{}), true
		},
	}, nil)
	comp.SetArgsComplete()
	comp.SetResult("err", true, 0)
	rows := comp.Render(72)
	var lines []string
	for _, row := range rows {
		if trimmed := plain(row); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Todo") {
		t.Fatalf("error card should still be titled Todo, got:\n%s", joined)
	}
	if !strings.Contains(joined, `Task "x" not found`) {
		t.Fatalf("error card should carry the failure text, got:\n%s", joined)
	}
}

// TestRomanNumeral pins the display numbering OMP uses for phase headers.
func TestRomanNumeral(t *testing.T) {
	cases := map[int]string{0: "", 1: "I", 2: "II", 3: "III", 4: "IV", 5: "V", 9: "IX", 14: "XIV"}
	for input, want := range cases {
		if got := romanNumeral(input); got != want {
			t.Fatalf("romanNumeral(%d) = %q, want %q", input, got, want)
		}
	}
}

// TestSelectCollapsedTodos pins OMP's walking viewport: the last closed task
// leads as context, the active task always survives, and the summary counts what
// did not fit.
func TestSelectCollapsedTodos(t *testing.T) {
	many := make([]todo.Task, 0, 12)
	for i := 0; i < 12; i++ {
		status := todo.StatusPending
		if i == 0 {
			status = todo.StatusCompleted
		}
		if i == 1 {
			status = todo.StatusInProgress
		}
		many = append(many, todo.Task{Content: string(rune('a' + i)), Status: status})
	}
	selection := selectCollapsedTodos(many, collapsedItems)
	if len(selection.items) == 0 {
		t.Fatal("selection dropped every row")
	}
	// The active task must survive any window.
	found := false
	for _, task := range selection.items {
		if task.Status == todo.StatusInProgress {
			found = true
		}
	}
	if !found {
		t.Fatal("the in-progress task must never be dropped from the collapsed view")
	}
	// The closed lead is context, so the completed task leads the rows.
	if selection.items[0].Status != todo.StatusCompleted {
		t.Fatalf("expected the last closed task to lead, got %q", selection.items[0].Status)
	}
	if selection.summary == "" {
		t.Fatal("a list longer than the cap should report a summary")
	}
}

// TestFormatTodoLineBlockedNote checks the blocked row's note, which is the one
// place a task explains why it stopped.
func TestFormatTodoLineBlockedNote(t *testing.T) {
	withReason := formatTodoLine(todo.Task{Content: "wait", Status: todo.StatusBlocked, Blocker: "needs a decision"})
	if !strings.Contains(plain(withReason), "(blocked: needs a decision)") {
		t.Fatalf("blocked row should carry its reason, got %q", plain(withReason))
	}
	withoutReason := formatTodoLine(todo.Task{Content: "wait", Status: todo.StatusBlocked})
	if !strings.Contains(plain(withoutReason), "(blocked)") {
		t.Fatalf("blocked row without a reason should still say blocked, got %q", plain(withoutReason))
	}
}
