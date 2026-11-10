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

// TestTodoWidgetErrorStaysOneRow pins what a rejected call does to the panel.
//
// The panel is an overlay capped at twelve rows beside the prompt, so a schema
// rejection's header, bullets and pretty-printed argument echo used to fill it
// and push the last good plan out of view. A failed call now contributes one
// truncated row carrying the actionable reason, while the list it could not
// change stays intact and the full text stays in the transcript.
func TestTodoWidgetErrorStaysOneRow(t *testing.T) {
	m := &InteractiveMode{}
	m.updateTodoWidget(widgetTodoResult("init", "kept task"))
	base := len(m.renderTodoWidget(40, true))
	message := "Validation failed for tool \"todo\":\n" +
		"  - op: expected one of [\"init\",\"start\",\"done\",\"drop\",\"block\",\"unblock\",\"rm\",\"append\",\"view\"]\n\n" +
		"Received arguments:\n" + strings.Repeat("{\n  \"op\": \"bogus\"\n}\n", 20)
	m.updateTodoWidget(agent.AgentToolResult{IsError: true, Content: []ai.ToolResultMessageContent{ai.TextContent{Text: message}}})

	for _, expanded := range []bool{false, true} {
		rows := m.renderTodoWidget(40, expanded)
		if len(rows) != base+1 {
			t.Fatalf("expanded=%v: a failed call grew the widget by %d rows: %v", expanded, len(rows)-base, rows)
		}
		// The plan's summary row stays first so the panel still names the list;
		// the reason follows it so it is never below the fold.
		if !strings.Contains(widthx.StripAnsi(rows[0]), "Todo") {
			t.Fatalf("expanded=%v: the panel lost the plan summary: %q", expanded, rows[0])
		}
		reason := widthx.StripAnsi(rows[1])
		if !strings.Contains(reason, "op: expected one of") {
			t.Fatalf("expanded=%v: the panel did not show the reason below the summary: %q", expanded, reason)
		}
		if strings.Contains(reason, "Received arguments") || strings.Contains(reason, "Validation failed") {
			t.Fatalf("expanded=%v: error row kept the header or echo: %q", expanded, reason)
		}
		if widthx.VisibleWidth(strings.TrimRight(reason, " ")) > 40 {
			t.Fatalf("expanded=%v: error row overflowed the width: %q", expanded, reason)
		}
		if joined := strings.Join(rows, "\n"); !strings.Contains(joined, "kept task") {
			t.Fatalf("expanded=%v: a failed call hid the last good list: %v", expanded, rows)
		}
	}

	// The panel frames the same rows, so its height stays inside the twelve-row
	// cap and the reason is on a visible interior row, not below the fold.
	panel := m.ensureInfoWidgets().Render(40)
	if len(panel) > 12 {
		t.Fatalf("panel exceeded its row bound: %d", len(panel))
	}
	if joined := widthx.StripAnsi(strings.Join(panel, "\n")); !strings.Contains(joined, "op: expected one of") || !strings.Contains(joined, "kept task") {
		t.Fatalf("panel lost the reason or the last good list: %s", joined)
	}

	// A tool-level rejection whose first line is its diagnostic keeps that line.
	// Rendered wide enough that truncation cannot be mistaken for the wrong line.
	m.updateTodoWidget(agent.AgentToolResult{IsError: true, Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "Errors: Missing phase name for append operation\nRemaining items (2):\n  - A [in_progress] (P)"}}})
	rows := m.renderTodoWidget(80, true)
	if last := widthx.StripAnsi(rows[1]); !strings.Contains(last, "Missing phase name for append operation") || strings.Contains(last, "Remaining items") {
		t.Fatalf("a tool rejection was condensed to the wrong line: %q", last)
	}

	// A successful call clears the failure: the panel must not keep claiming a
	// problem the next mutation already resolved.
	m.updateTodoWidget(widgetTodoResult("done", "next task"))
	if m.todoWidgetError != "" {
		t.Fatalf("a successful call kept the old failure: %q", m.todoWidgetError)
	}
	if rows := m.renderTodoWidget(80, true); strings.Contains(strings.Join(rows, "\n"), "Missing phase name") {
		t.Fatalf("a successful call left the failure row in the panel: %v", rows)
	}

	// A failure with no list yet is still the panel's only content: the reason
	// must not be dropped for want of a plan to sit above.
	fresh := &InteractiveMode{}
	fresh.updateTodoWidget(agent.AgentToolResult{IsError: true, Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "Validation failed for tool \"todo\":\n  - op: missing\n\nReceived arguments:\n{\n  \"list\": []\n}"}}})
	rows = fresh.renderTodoWidget(40, false)
	if len(rows) != 1 || !strings.Contains(widthx.StripAnsi(rows[0]), "op: missing") {
		t.Fatalf("a failure with no plan was dropped: %v", rows)
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
