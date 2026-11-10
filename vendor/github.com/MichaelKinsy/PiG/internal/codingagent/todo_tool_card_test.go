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
}
