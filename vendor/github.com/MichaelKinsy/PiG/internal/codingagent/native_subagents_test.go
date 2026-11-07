package codingagent

import (
	"context"
	"encoding/json"
	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/internal/subagents"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

type observerSession struct {
	InteractiveSessionHandle
	manager *subagents.Manager
}

func (s observerSession) Subagents() *subagents.Manager { return s.manager }

func observerFixture(t *testing.T) (*InteractiveMode, *subagents.Manager) {
	t.Helper()
	manager := subagents.New(subagents.Host{CWD: t.TempDir(), AgentDir: t.TempDir(), SessionID: "viewer-test", Restore: func() []subagents.Record {
		return []subagents.Record{{ID: "agent_test", Type: "Explore", DisplayName: "Explore", Handle: "explore", Description: "Find auth files", Status: "completed", Result: "Found auth.go", StartedAt: time.Now().Add(-time.Second).UnixMilli(), CompletedAt: time.Now().UnixMilli(), Tokens: 12400, Turns: 3, ToolUses: 3, Background: true, SessionFile: "fixture.jsonl", Config: subagents.Config{Name: "Explore"}}}
	}})
	if err := manager.Update(map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	m := &InteractiveMode{opts: InteractiveModeOptions{SessionHandle: observerSession{manager: manager}}, tuiInst: tui.NewWithOutput(io.Discard, 100, 24), editor: tui.NewEditor()}
	m.subagentUI = &subagentUIState{manager: manager}
	return m, manager
}

func TestNativeAgentWidgetAndFleetWidth(t *testing.T) {
	m, _ := observerFixture(t)
	for _, width := range []int{12, 40, 100} {
		widget := &nativeAgentWidget{m: m}
		lines := widget.Render(width)
		if len(lines) == 0 || !strings.Contains(strings.Join(lines, "\n"), "Agents") {
			t.Fatal("missing live widget", lines)
		}
		for _, line := range lines {
			if widthx.VisibleWidth(line) > width {
				t.Fatalf("widget overflows %d: %q", width, line)
			}
		}
		fleet := m.renderAgentFleet(width)
		for _, line := range fleet {
			if widthx.VisibleWidth(line) > width {
				t.Fatalf("fleet overflows %d: %q", width, line)
			}
		}
	}
	if !m.handleAgentKey("\x1b[B") || !m.subagentUI.focused {
		t.Fatal("empty prompt down did not enter fleet")
	}
	m.editor.SetText("normal input")
	if m.handleAgentKey("\x1b[B") {
		t.Fatal("fleet consumed arrow inside nonempty prompt")
	}
}

func TestAgentCompletionPreservesFileProvider(t *testing.T) {
	m, manager := observerFixture(t)
	provider := &subagentAutocomplete{CombinedProvider: tui.NewCombinedProvider(nil, t.TempDir(), ""), m: m}
	prefix, task, ok := provider.SuggestionTask([]string{"@ex"}, 0, 3, false)
	if !ok || prefix != "@ex" {
		t.Fatalf("missing asynchronous mention completion: %q %t", prefix, ok)
	}
	items, err := task(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.Value == "@explore" && strings.Contains(item.Description, "resume") {
			found = true
		}
	}
	if !found {
		t.Fatal(items)
	}
	if err := manager.Update(map[string]any{"agentMentions": "off"}); err != nil {
		t.Fatal(err)
	}
	rows := provider.merge(nil, []string{"@ex"}, 0, 3)
	if rows != nil {
		t.Fatal("disabled mentions still complete", rows)
	}
}

func TestObserverMouseAndComposerStayLocal(t *testing.T) {
	m, manager := observerFixture(t)
	v := &subagentViewer{m: m, manager: manager, id: "agent_test", follow: false, markdown: "assistant", scroll: 20, maxScroll: 100, viewport: 10, width: 80, dividerRow: 12, footerRow: 13}
	result := v.HandleMouse(tui.TuiMouseEvent{Type: tui.MouseWheel, WheelDelta: -1})
	if !result.Handled || v.scroll != 17 {
		t.Fatalf("wheel escaped or did not scroll: %+v %d", result, v.scroll)
	}
	v.HandleMouse(tui.TuiMouseEvent{Type: tui.MouseClick, Y: 12})
	if !v.follow {
		t.Fatal("jump indicator did not resume following")
	}
	v.composer = tui.NewEditor()
	v.composer.SetText("draft")
	m.editor.SetText("main prompt")
	v.HandleInput("\x1b")
	if v.composer != nil || m.editor.Text() != "main prompt" {
		t.Fatal("cancel steering changed main editor")
	}
	v.HandleInput("x")
	if !v.confirmStop {
		t.Fatal("first stop not confirmed")
	}
	v.HandleInput("\x1b")
	if v.confirmStop {
		t.Fatal("escape did not cancel stop confirmation")
	}
}

func TestAgentCardRetainsErrorAndCollapsedResult(t *testing.T) {
	raw := json.RawMessage(`{"subagent_type":"Explore","description":"Find auth files"}`)
	call := subagentRenderCall(raw, nil, extension.ToolRenderContext{})
	if !strings.Contains(strings.Join(call.Render(100), "\n"), "Find auth files") {
		t.Fatal("call description missing")
	}
	r := subagents.Record{ID: "test", Status: "completed", Result: "line one\nline two", StartedAt: 1, CompletedAt: 1001}
	card := subagentRenderResult(agent.AgentToolResult{Details: r}, extension.ToolRenderResultOptions{}, nil, extension.ToolRenderContext{})
	rendered := strings.Join(card.Render(100), "\n")
	if !strings.Contains(rendered, "line one") || !strings.Contains(rendered, "ctrl+o") {
		t.Fatal(rendered)
	}
}

func TestSubagentNotificationCardsHideEnvelopeAndExpand(t *testing.T) {
	card := &subagentNotificationCard{text: `<task-notification><task-id>agent_test</task-id><status>completed</status><summary>Audit &amp; report</summary><result>one
two
three
four
five
six</result></task-notification>`}
	collapsed := strings.Join(card.Render(50), "\n")
	if strings.Contains(collapsed, "task-notification") || !strings.Contains(collapsed, "Audit & report") || strings.Contains(collapsed, "six") {
		t.Fatal(collapsed)
	}
	card.expanded = true
	expanded := strings.Join(card.Render(50), "\n")
	if !strings.Contains(expanded, "six") {
		t.Fatal(expanded)
	}
	for _, line := range card.Render(16) {
		if widthx.VisibleWidth(line) > 16 {
			t.Fatal("notification overflow", line)
		}
	}
}
