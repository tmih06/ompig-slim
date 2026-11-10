package codingagent

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// widgetTodoResult supplies the persisted wire shape without importing the todo
// builtin, whose registration imports codingagent and would create a cycle.
func widgetTodoResult(op, task string) agent.AgentToolResult {
	return agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: task}}, Details: map[string]any{
		"op": op, "phases": []any{map[string]any{"name": "Plan", "tasks": []any{map[string]any{"content": task, "status": "pending"}}}},
	}}
}

// TestTodoWidgetSnapshots pins the same authoritative-result rules as the tool:
// views, errors and malformed details cannot replace state; clearing can.
func TestTodoWidgetSnapshots(t *testing.T) {
	m := &InteractiveMode{}
	m.updateTodoWidget(widgetTodoResult("init", "original"))
	original := m.todoWidget
	if original == nil {
		t.Fatal("no widget after init")
	}
	m.updateTodoWidget(widgetTodoResult("view", "not authoritative"))
	if m.todoWidget != original {
		t.Fatal("view replaced state")
	}
	failed := widgetTodoResult("done", "failure")
	failed.IsError = true
	m.updateTodoWidget(failed)
	if m.todoWidget != original || m.todoWidgetError != "failure" {
		t.Fatal("error lost last good plan or was hidden")
	}
	m.updateTodoWidget(agent.AgentToolResult{Details: map[string]any{"op": "init"}})
	if m.todoWidget != original {
		t.Fatal("malformed details replaced state")
	}
	m.updateTodoWidget(agent.AgentToolResult{Details: map[string]any{"op": "init", "phases": []any{}}})
	if m.todoWidget != nil || m.todoWidgetError != "" {
		t.Fatal("empty plan failed to clear widget")
	}
}

// TestTodoEventsUseCompactCards covers direct receipts and nested calls, which
// update the widget without duplicating their existing parent transcript card.
func TestTodoEventsUseCompactCards(t *testing.T) {
	m := &InteractiveMode{chatContainer: tui.NewContainer(), tuiInst: tui.NewWithOutput(io.Discard, 80, 24), toolByID: map[string]*tui.ToolExecutionComponent{}, toolStarts: map[string]time.Time{}}
	for _, parent := range []string{"", "codemode-parent"} {
		m.handleAgentEvent(agent.ToolExecutionStartEvent{ToolName: "todo", ToolCallID: "call", ParentToolCallID: parent, Args: json.RawMessage(`{"op":"init"}`)})
		m.handleAgentEvent(agent.ToolExecutionEndEvent{ToolName: "todo", ToolCallID: "call", ParentToolCallID: parent, Result: widgetTodoResult("init", "visible task")})
		if m.chatContainer.IsEmpty() || len(m.toolOrder) != 1 || m.todoWidget == nil {
			t.Fatal("missing direct receipt/widget, or nested call duplicated receipt")
		}
		transcript := widthx.StripAnsi(strings.Join(m.toolOrder[0].Render(100), "\n"))
		if !strings.Contains(transcript, "Todo: init") || !strings.Contains(transcript, "0/1 done") || strings.Contains(transcript, "visible task") {
			t.Fatalf("receipt duplicated list or lost action summary: %s", transcript)
		}
	}
}

// TestTodoWidgetBranchRestore verifies resume and rewinds use branch results,
// not a sticky side table. Restored widget state is independent of receipts.
func TestTodoWidgetBranchRestore(t *testing.T) {
	m := &InteractiveMode{}
	var entries []SessionEntry
	for _, record := range ParseSessionEntries(`{"type":"message","id":"one","message":{"role":"toolResult","toolCallId":"a","toolName":"todo","content":[{"type":"text","text":"first"}],"details":{"op":"init","phases":[{"name":"Plan","tasks":[{"content":"first","status":"pending"}]}]}}}
{"type":"message","id":"two","message":{"role":"toolResult","toolCallId":"b","toolName":"todo","content":[{"type":"text","text":"second"}],"details":{"op":"done","phases":[{"name":"Plan","tasks":[{"content":"second","status":"completed"}]}]}}}`) {
		if entry, ok := record.(SessionEntry); ok {
			entries = append(entries, entry)
		}
	}
	if len(entries) != 2 {
		t.Fatal("bad session fixture")
	}
	m.restoreTodoWidget(entries)
	if text := strings.Join(m.renderTodoWidget(80, true), "\n"); !strings.Contains(text, "second") {
		t.Fatalf("latest state missing: %s", text)
	}
	m.restoreTodoWidget(entries[:1])
	if text := strings.Join(m.renderTodoWidget(80, true), "\n"); !strings.Contains(text, "first") || strings.Contains(text, "second") {
		t.Fatalf("rewind retained future state: %s", text)
	}
	m.restoreTodoWidget(nil)
	if m.todoWidget != nil {
		t.Fatal("new branch retained old plan")
	}
}

// TestQuotaWidgetUsesCachedWindows ensures all applicable quota windows are
// available, while compact and failed states follow the existing quota policy.
func TestQuotaWidgetUsesCachedWindows(t *testing.T) {
	m := &InteractiveMode{opts: InteractiveModeOptions{Model: &ai.Model{ID: "test", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai-codex"}}}, promptQuota: &promptQuotaState{loaded: true, snapshot: ai.UsageSnapshot{Reports: []ai.UsageReport{{Provider: "openai-codex", Limits: []ai.UsageLimit{promptTestLimit("5h", .2, 0), promptTestLimit("7d", .5, 0)}}}}}}
	m.opts.UsageAccountKey = func(string) (*string, bool) { return nil, true }
	expanded := strings.Join(m.renderQuotaWidget(36, true), "\n")
	if !strings.Contains(expanded, "5h") || !strings.Contains(expanded, "7d") || !strings.Contains(expanded, "80% left") {
		t.Fatalf("missing quota windows: %s", expanded)
	}
	if len(m.renderQuotaWidget(36, false)) != 1 {
		t.Fatal("quota did not collapse")
	}
	m.updateTodoWidget(widgetTodoResult("init", "priority task"))
	selected := strings.Join(m.ensureInfoWidgets().Render(40), "\n")
	if !strings.Contains(selected, "Tasks") || strings.Contains(selected, "Quota") {
		t.Fatalf("todo did not take sole widget priority: %s", selected)
	}
	m.updateTodoWidget(agent.AgentToolResult{Details: map[string]any{"op": "init", "phases": []any{}}})
	selected = strings.Join(m.ensureInfoWidgets().Render(40), "\n")
	if !strings.Contains(selected, "Quota") || strings.Contains(selected, "Tasks") {
		t.Fatalf("clearing todo did not restore quota: %s", selected)
	}
	m.promptQuota.failed = true
	if len(m.renderQuotaWidget(36, true)) != 0 {
		t.Fatal("failed quota presented as current")
	}
}
