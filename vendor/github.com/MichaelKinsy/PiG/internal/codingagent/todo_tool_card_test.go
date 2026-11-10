package codingagent

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// compactTodoRows removes only card separation so assertions count visible rows.
func compactTodoRows(card *tui.ToolExecutionComponent, width int) []string {
	var rows []string
	for _, row := range card.Render(width) {
		if strings.TrimSpace(widthx.StripAnsi(row)) != "" {
			rows = append(rows, widthx.StripAnsi(row))
		}
	}
	return rows
}

// TestTodoTranscriptCardStaysCompact covers pending/running/success/expanded and
// renderer rebinding. Only operation/target/counts belong in the action receipt.
func TestTodoTranscriptCardStaysCompact(t *testing.T) {
	m := &InteractiveMode{}
	args := json.RawMessage(`{"op":"init","list":[{"phase":"Plan","items":["private task body"]}]}`)
	card := m.newToolCard("todo", "receipt", args)
	for _, running := range []bool{false, true} {
		if running {
			card.MarkExecutionStarted()
		}
		rows := compactTodoRows(card, 100)
		if len(rows) != 1 || !strings.Contains(rows[0], "Todo: init") || strings.Contains(rows[0], "private task body") {
			t.Fatalf("verbose pending card: %v", rows)
		}
	}
	result := widgetTodoResult("init", "private task body")
	card.SetArgsComplete()
	card.SetResultMeta(todoCardMeta(result))
	final := ToolExecutionResultOf(result, 0)
	final.DurationMs = new(int64(1200))
	card.UpdateResult(final, false)
	for _, expanded := range []bool{false, true} {
		card.SetExpanded(expanded)
		rows := compactTodoRows(card, 100)
		if len(rows) != 1 || !strings.Contains(rows[0], "✔") || !strings.Contains(rows[0], "0/1 done") || !strings.Contains(rows[0], "1.2s") {
			t.Fatalf("missing compact lifecycle facts: %v", rows)
		}
		if strings.Contains(rows[0], "private task body") || strings.Contains(rows[0], "╭") {
			t.Fatalf("list/frame leaked into receipt: %v", rows)
		}
	}
	for _, width := range []int{1, 5, 9} {
		rows := compactTodoRows(card, width)
		if len(rows) != 1 || widthx.VisibleWidth(rows[0]) > width {
			t.Fatalf("tiny receipt vanished or overflowed at %d: %v", width, rows)
		}
	}
	m.bindToolCard(card, "receipt", "todo", args)
	if rows := compactTodoRows(card, 100); len(rows) != 1 || strings.Contains(rows[0], "private task body") {
		t.Fatalf("rebind restored full list: %v", rows)
	}
}

// TestTodoTranscriptFailure keeps an immediate reason on the closed status row
// and bounds expanded diagnostics, including very wide or multiline messages.
func TestTodoTranscriptFailure(t *testing.T) {
	m := &InteractiveMode{}
	card := m.newToolCard("todo", "failure", json.RawMessage(`{"op":"done","task":"named target"}`))
	message := "Task not found\n" + strings.Repeat(strings.Repeat("界", 80)+"\n", 20)
	result := agent.AgentToolResult{IsError: true, Content: []ai.ToolResultMessageContent{ai.TextContent{Text: message}}}
	card.SetResultMeta(todoCardMeta(result))
	card.UpdateResult(ToolExecutionResultOf(result, 0), false)
	rows := compactTodoRows(card, 80)
	if len(rows) != 1 || !strings.Contains(rows[0], "✘") || !strings.Contains(rows[0], "Task not found") || !strings.Contains(rows[0], "named target") {
		t.Fatalf("error receipt lost state/target/reason: %v", rows)
	}
	card.SetExpanded(true)
	rows = compactTodoRows(card, 80)
	if len(rows) != 7 {
		t.Fatalf("error preview should be status plus six rows: %d", len(rows))
	}
	for _, row := range rows {
		if widthx.VisibleWidth(row) > 80 {
			t.Fatalf("unbounded error row: %q", row)
		}
	}
}

// TestTodoErrorSummary pins the one line a closed receipt and a Tasks widget row
// can show, against every failure shape the sessions actually logged.
//
// A schema rejection arrives as a header, one `- path: reason` bullet per
// problem, then a pretty-printed copy of the call's own arguments. Showing its
// first line alone names no problem at all (`Validation failed for tool "todo":`),
// and showing all of it turns a two-row widget into a scrollable dump of the
// model's payload. The bullet is the diagnostic, and the echo is never one.
func TestTodoErrorSummary(t *testing.T) {
	validation := "Validation failed for tool \"todo\":\n" +
		"  - op: expected one of [\"init\",\"start\",\"done\",\"drop\",\"block\",\"unblock\",\"rm\",\"append\",\"view\"]\n\n" +
		"Received arguments:\n{\n  \"list\": [\n    {\n      \"phase\": \"P\"\n    }\n  ]\n}"
	cases := map[string]struct{ message, want string }{
		"a schema rejection reports its first bullet, not its header": {
			message: validation,
			want:    `op: expected one of ["init","start","done","drop","block","unblock","rm","append","view"]`,
		},
		"only the first problem is kept": {
			message: "Validation failed for tool \"todo\":\n  - op: missing\n  - list.0.items: missing\n\nReceived arguments:\n{}",
			want:    "op: missing",
		},
		"a rejection without an argument echo is unchanged": {
			message: "Validation failed for tool \"todo\":\n  - op: missing",
			want:    "op: missing",
		},
		"an unparseable call keeps the text that failed to parse": {
			message: "Invalid arguments for tool \"todo\": Unexpected token 'l', \"{\"init\": list\" /><p\"... is not valid JSON\n\nReceived arguments:\n{\"init\": list\" /><parameter name=\"phase\">investigate}",
			want:    `Invalid arguments for tool "todo": Unexpected token 'l', "{"init": list" /><p"... is not valid JSON`,
		},
		"a tool rejection keeps its own first line": {
			message: "Errors: Missing phase name for append operation\nRemaining items (2):\n  - A [in_progress] (P)\n",
			want:    "Errors: Missing phase name for append operation",
		},
		"a tool rejection that names a task keeps the hint": {
			message: "Errors: Task not found (the todo list is empty - was it replaced, or not created yet?)\nRemaining items: none.",
			want:    "Errors: Task not found (the todo list is empty - was it replaced, or not created yet?)",
		},
		"a single-line result is its own summary": {
			message: "Missing phase name for append operation",
			want:    "Missing phase name for append operation",
		},
		"an empty result still names a failure": {
			message: "\n\n   \n",
			want:    "failed",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := todoErrorSummary(c.message); got != c.want {
				t.Fatalf("todoErrorSummary = %q, want %q", got, c.want)
			}
		})
	}
}

// TestTodoTranscriptValidationFailure drives a real schema rejection through the
// receipt: the closed row carries the actionable bullet and the expanded body
// stops before the copy of the model's arguments.
func TestTodoTranscriptValidationFailure(t *testing.T) {
	m := &InteractiveMode{}
	card := m.newToolCard("todo", "invalid", json.RawMessage(`{"list":[{"phase":"P"}]}`))
	message := "Validation failed for tool \"todo\":\n  - list.0.items: missing\n\nReceived arguments:\n" +
		"{\n  \"list\": [\n    {\n      \"phase\": \"P\"\n    }\n  ]\n}"
	result := agent.AgentToolResult{IsError: true, Content: []ai.ToolResultMessageContent{ai.TextContent{Text: message}}}
	card.SetResultMeta(todoCardMeta(result))
	card.UpdateResult(ToolExecutionResultOf(result, 0), false)
	rows := compactTodoRows(card, 100)
	if len(rows) != 1 || !strings.Contains(rows[0], "list.0.items: missing") {
		t.Fatalf("closed receipt must name the rejected member: %v", rows)
	}
	if strings.Contains(rows[0], "Validation failed") || strings.Contains(rows[0], "Received arguments") {
		t.Fatalf("closed receipt kept the header or the argument echo: %v", rows)
	}
	card.SetExpanded(true)
	joined := strings.Join(compactTodoRows(card, 100), "\n")
	if !strings.Contains(joined, "list.0.items: missing") {
		t.Fatalf("expanded receipt lost the diagnostic: %s", joined)
	}
	if strings.Contains(joined, "Received arguments") || strings.Contains(joined, `"phase": "P"`) {
		t.Fatalf("expanded receipt echoed the arguments: %s", joined)
	}
}

// TestTodoTranscriptResume rebuilds the actual transcript path: the persisted
// call gets a compact receipt while its full result hydrates the private widget.
func TestTodoTranscriptResume(t *testing.T) {
	m := &InteractiveMode{chatContainer: tui.NewContainer(), tuiInst: tui.NewWithOutput(io.Discard, 100, 24), toolByID: map[string]*tui.ToolExecutionComponent{}}
	records := ParseSessionEntries(`{"type":"message","id":"call","message":{"role":"assistant","content":[{"type":"toolCall","id":"a","name":"todo","arguments":{"op":"init"}}],"stopReason":"toolUse"}}
{"type":"message","id":"result","message":{"role":"toolResult","toolCallId":"a","toolName":"todo","content":[{"type":"text","text":"widget-only task"}],"details":{"op":"init","phases":[{"name":"Plan","tasks":[{"content":"widget-only task","status":"pending"}]}]}}}`)
	var entries []SessionEntry
	for _, record := range records {
		if entry, ok := record.(SessionEntry); ok {
			entries = append(entries, entry)
		}
	}
	m.renderSessionEntryList(entries, false)
	if len(m.toolOrder) != 1 || m.todoWidget == nil {
		t.Fatal("resume lost receipt or widget")
	}
	receipt := strings.Join(compactTodoRows(m.toolOrder[0], 100), "\n")
	if !strings.Contains(receipt, "Todo: init") || !strings.Contains(receipt, "0/1 done") || strings.Contains(receipt, "widget-only task") {
		t.Fatalf("resumed receipt is not compact: %s", receipt)
	}
	if !strings.Contains(strings.Join(m.renderTodoWidget(100, true), "\n"), "widget-only task") {
		t.Fatal("resume lost widget task content")
	}
}

// TestTodoCardMeta uses persisted status names without treating abandoned work
// as completed, and accepts absent details from interrupted or legacy calls.
func TestTodoCardMeta(t *testing.T) {
	result := agent.AgentToolResult{Details: map[string]any{"phases": []any{map[string]any{"tasks": []any{
		map[string]any{"status": "completed"}, map[string]any{"status": "pending"}, map[string]any{"status": "in_progress"}, map[string]any{"status": "blocked"}, map[string]any{"status": "abandoned"},
	}}}}}
	if got := strings.Join(todoCardMeta(result), " · "); got != "1/5 done · 1 blocked · 1 dropped" {
		t.Fatalf("wrong receipt counts: %s", got)
	}
	if len(todoCardMeta(agent.AgentToolResult{})) != 0 {
		t.Fatal("missing details invented task counts")
	}
	// A rejected call claims no counts of its own; the list it could not change
	// is unchanged, and the status row says only why it failed.
	failed := agent.AgentToolResult{IsError: true, Details: result.Details, Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "Errors: Task \"x\" not found"}}}
	if got := strings.Join(todoCardMeta(failed), " · "); got != `Errors: Task "x" not found` {
		t.Fatalf("an error receipt reported counts: %s", got)
	}
}
