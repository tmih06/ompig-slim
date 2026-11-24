package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
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
	// These tests own component mutation and render explicitly, just as the
	// production input loop owns its frame deadline. Do not render on timers.
	m.tuiInst.SetRenderDeadline(func(time.Time) {})
	m.subagentUI = &subagentUIState{manager: manager}
	t.Cleanup(func() {
		if m.subagentUI.expireTimer != nil {
			m.subagentUI.expireTimer.Stop()
		}
		if m.subagentUI.hub != nil {
			m.subagentUI.hub.close()
		}
		for m.subagentUI.viewer != nil {
			m.subagentUI.viewer.close()
		}
	})
	return m, manager
}

// TestFocusedSubagentViewerFillsViewport prevents the centered overlay from
// exposing a row of the parent conversation above the subagent title.
func TestFocusedSubagentViewerFillsViewport(t *testing.T) {
	m, _ := observerFixture(t)
	m.openSubagentViewer("agent_test", false, true)
	v := m.subagentUI.viewer
	m.tuiInst.RenderNow()
	bounds, ok := v.handle.GetBounds()
	if !ok {
		t.Fatal("viewer overlay has no rendered bounds")
	}
	if bounds.Row != 0 || bounds.Height != m.tuiInst.Height() {
		t.Fatalf("viewer leaves parent rows visible: row=%d height=%d, want row=0 height=%d", bounds.Row, bounds.Height, m.tuiInst.Height())
	}
	lines := v.Render(m.tuiInst.Width())
	if !strings.Contains(widthx.StripTerminalSequences(lines[0]), "Find auth files") {
		t.Fatal("subagent title must be the first screen row")
	}
	if v.footerRow != len(lines)-1 {
		t.Fatal("viewer footer is not on the last row")
	}
}

func TestAgentTreeHiddenUntilDownFromEmptyPrompt(t *testing.T) {
	m, _ := observerFixture(t)
	tree := &nativeAgentTree{m: m}
	if lines := tree.Render(100); lines != nil {
		t.Fatalf("tree rendered before Down: %q", lines)
	}
	if !m.handleAgentKey("\x1b[B") {
		t.Fatal("Down on the empty prompt did not open the tree")
	}
	for _, width := range []int{12, 40, 100} {
		lines := tree.Render(width)
		if len(lines) == 0 || !strings.Contains(strings.Join(lines, "\n"), "Agents") {
			t.Fatal("missing agent tree", lines)
		}
		for _, line := range lines {
			if widthx.VisibleWidth(line) > width {
				t.Fatalf("tree overflows %d: %q", width, line)
			}
		}
	}
	if !m.handleAgentKey("\x1ba") || m.subagentUI.hub == nil || m.subagentUI.treeSelected != "" {
		t.Fatal("Alt+A did not open Agent Hub over the tree")
	}
	m.subagentUI.hub.close()
	m.handleAgentKey("\x1b[B")
	m.handleAgentKey("\x1b[D")
	m.handleAgentKey("\x1b[D")
	if m.subagentUI.hub == nil {
		t.Fatal("double-left from the tree did not open Agent Hub")
	}
	m.subagentUI.hub.close()

	m.editor.SetText("normal input")
	if m.handleAgentKey("\x1b[B") {
		t.Fatal("agent navigation consumed arrow inside nonempty prompt")
	}
}

func TestAgentFooterRefreshesWhileTreeHidden(t *testing.T) {
	m, manager := observerFixture(t)
	m.statusLine = NewFooterComponent(nil, "", nil)
	m.statusLine.agents = m.subagentCounts
	m.subagentUI = nil
	m.bindSubagentUI()
	footer := tui.NewContainer(m.statusLine)
	if text := widthx.StripTerminalSequences(strings.Join(footer.Render(100), "\n")); !strings.Contains(text, "1 subagent") {
		t.Fatalf("initial footer omitted the agent: %q", text)
	}
	if err := manager.Update(map[string]any{"widgetMode": "off"}); err != nil {
		t.Fatal(err)
	}
	if text := widthx.StripTerminalSequences(strings.Join(footer.Render(100), "\n")); strings.Contains(text, "subagent") {
		t.Fatalf("cached footer kept stale agent counts: %q", text)
	}
}

func TestAgentTreeNeverOpensOnNonEmptyPrompt(t *testing.T) {
	m, _ := observerFixture(t)
	for _, text := range []string{"x", "   ", "\n", "line one\nline two"} {
		m.editor.SetText(text)
		if m.handleAgentKey("\x1b[B") {
			t.Fatalf("Down opened the tree over %q", text)
		}
		if m.subagentUI.treeSelected != "" {
			t.Fatalf("text %q left a selection", text)
		}
	}
}

func TestAgentTreeOpensOnlyAfterEmptyDraftRestored(t *testing.T) {
	m, _ := observerFixture(t)
	m.editor.AddToHistory("first prompt")
	m.editor.AddToHistory("second prompt")
	m.editor.HandleInput("\x1b[A")
	m.editor.HandleInput("\x1b[A")
	if m.editor.Text() != "first prompt" {
		t.Fatalf("Up Up lost the older entry: %q", m.editor.Text())
	}
	if m.handleAgentKey("\x1b[B") {
		t.Fatal("Down hijacked history navigation")
	}
	m.editor.HandleInput("\x1b[B")
	if m.editor.Text() != "second prompt" {
		t.Fatalf("Down lost the newer entry: %q", m.editor.Text())
	}
	if m.handleAgentKey("\x1b[B") {
		t.Fatal("Down hijacked draft restoration")
	}
	m.editor.HandleInput("\x1b[B")
	if m.editor.Text() != "" {
		t.Fatalf("Down did not restore the draft: %q", m.editor.Text())
	}
	if !m.handleAgentKey("\x1b[B") || m.subagentUI.treeSelected != "agent_test" {
		t.Fatal("Down past the restored empty draft did not open the tree")
	}
	if !m.handleAgentKey("\x1b[A") || m.subagentUI.treeSelected != "" {
		t.Fatal("Up on the first row did not close the tree")
	}
	m.editor.HandleInput("\x1b[A")
	if m.editor.Text() != "second prompt" {
		t.Fatalf("the closing Up browsed history: %q", m.editor.Text())
	}
}

func TestAgentTreePreservesNonEmptyHistoryDraft(t *testing.T) {
	m, _ := observerFixture(t)
	m.editor.AddToHistory("previous prompt")
	m.editor.SetText("draft")
	m.editor.HandleInput("\x1b[H")
	m.editor.HandleInput("\x1b[A")
	if m.editor.Text() != "previous prompt" {
		t.Fatalf("Up did not recall the previous prompt: %q", m.editor.Text())
	}
	if m.handleAgentKey("\x1b[B") {
		t.Fatal("Down hijacked nonempty draft restoration")
	}
	m.editor.HandleInput("\x1b[B")
	if m.handleAgentKey("\x1b[B") || m.editor.Text() != "draft" {
		t.Fatal("Down over the restored draft opened the tree")
	}
	if m.subagentUI.treeSelected != "" {
		t.Fatal("a draft left the tree armed")
	}
}

func treeFixture(t *testing.T, records []subagents.Record) (*InteractiveMode, *subagents.Manager) {
	t.Helper()
	manager := subagents.New(subagents.Host{CWD: t.TempDir(), AgentDir: t.TempDir(), SessionID: "tree-test", Restore: func() []subagents.Record {
		return records
	}})
	if err := manager.Update(map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	m := &InteractiveMode{opts: InteractiveModeOptions{SessionHandle: observerSession{manager: manager}}, tuiInst: tui.NewWithOutput(io.Discard, 100, 24), editor: tui.NewEditor()}
	m.tuiInst.SetRenderDeadline(func(time.Time) {})
	m.subagentUI = &subagentUIState{manager: manager}
	t.Cleanup(func() {
		if m.subagentUI.expireTimer != nil {
			m.subagentUI.expireTimer.Stop()
		}
		if m.subagentUI.hub != nil {
			m.subagentUI.hub.close()
		}
		for m.subagentUI.viewer != nil {
			m.subagentUI.viewer.close()
		}
	})
	return m, manager
}

func TestAgentTreeKeysNavigateSelectAndClose(t *testing.T) {
	records := []subagents.Record{
		{ID: "a", Type: "Explore", Description: "first", Status: "completed", StartedAt: time.Now().UnixMilli(), CompletedAt: time.Now().UnixMilli(), Background: true},
		{ID: "b", Type: "Explore", Description: "second", Status: "completed", StartedAt: time.Now().UnixMilli(), CompletedAt: time.Now().UnixMilli(), Background: true},
	}
	m, _ := treeFixture(t, records)
	if !m.handleAgentKey("\x1b[B") || m.subagentUI.treeSelected != "a" {
		t.Fatal("Down did not open the tree on the first record")
	}
	if !m.handleAgentKey("\x1b[B") || m.subagentUI.treeSelected != "b" {
		t.Fatal("Down did not move the selection")
	}
	if !m.handleAgentKey("\x1b[B") || m.subagentUI.treeSelected != "b" {
		t.Fatal("Down did not clamp on the last row")
	}
	if !m.handleAgentKey("\x1b[A") || m.subagentUI.treeSelected != "a" {
		t.Fatal("Up did not move the selection back")
	}
	if !m.handleAgentKey("\x1b") || m.subagentUI.treeSelected != "" {
		t.Fatal("Escape did not close the tree")
	}
	if m.editor.Text() != "" {
		t.Fatalf("Escape edited the prompt: %q", m.editor.Text())
	}
	if !m.handleAgentKey("\x1b[B") {
		t.Fatal("reopen failed")
	}
	if m.handleAgentKey("q") || m.subagentUI.treeSelected != "" {
		t.Fatal("a printable key did not close the tree")
	}
	m.editor.HandleInput("q")
	if m.editor.Text() != "q" {
		t.Fatalf("the printable key was lost: %q", m.editor.Text())
	}
}

func TestAgentTreeEnterInspectsLikeHub(t *testing.T) {
	records := []subagents.Record{
		{ID: "a", Type: "Explore", Description: "first", Status: "completed", SessionFile: "a.jsonl", StartedAt: time.Now().UnixMilli(), CompletedAt: time.Now().UnixMilli(), Background: true},
		{ID: "b", Type: "Explore", Description: "second", Status: "completed", SessionFile: "b.jsonl", StartedAt: time.Now().UnixMilli(), CompletedAt: time.Now().UnixMilli(), Background: true},
	}
	m, _ := treeFixture(t, records)
	m.handleAgentKey("\x1b[B")
	m.handleAgentKey("\x1b[B")
	if !m.handleAgentKey("\r") {
		t.Fatal("Enter was not consumed")
	}
	v := m.subagentUI.viewer
	if m.subagentUI.treeSelected != "" || v == nil || v.id != "b" || !v.focusSession || v.composer == nil {
		t.Fatalf("Enter did not inspect the selected agent: %+v", v)
	}
	v.close()
	if m.subagentUI.treeSelected != "" {
		t.Fatal("the closed inspector silently reopened the tree")
	}
}

func TestAgentTreeReachesEveryRecordWithinBudget(t *testing.T) {
	var records []subagents.Record
	for i := 0; i < 20; i++ {
		records = append(records, subagents.Record{ID: fmt.Sprintf("agent_%02d", i), DisplayName: fmt.Sprintf("agent_%02d", i), Description: "row", Status: "completed", StartedAt: time.Now().Add(-time.Minute).UnixMilli(), CompletedAt: time.Now().UnixMilli(), Background: true})
	}
	m, _ := treeFixture(t, records)
	tree := &nativeAgentTree{m: m}
	m.handleAgentKey("\x1b[B")
	seen := func() string {
		return widthx.StripTerminalSequences(strings.Join(tree.Render(80), "\n"))
	}
	for i := 0; i < 19; i++ {
		if !m.handleAgentKey("\x1b[B") {
			t.Fatalf("Down stopped being consumed at %d", i)
		}
	}
	if m.subagentUI.treeSelected != "agent_19" {
		t.Fatalf("the last record is unreachable: %q", m.subagentUI.treeSelected)
	}
	text := seen()
	if !strings.Contains(text, "agent_19") || !strings.Contains(text, "above") {
		t.Fatalf("the render window lost the selected row or the overflow marker: %q", text)
	}
	viewport := CreateChatViewport(ChatViewportOptions{
		Document:        tui.NewContainer(),
		PendingMessages: tui.NewContainer(),
		Status:          tui.NewContainer(),
		Editor:          m.editor,
		Footer:          tui.NewContainer(NewFooterComponent(nil, "", nil), tree),
	})
	for _, height := range []int{12, 16, 24} {
		m.tuiInst = tui.NewWithOutput(io.Discard, 80, height)
		m.tuiInst.SetRenderDeadline(func(time.Time) {})
		frame := tui.RenderLayoutFrame(viewport.Root, 80, height, func() {})
		visible := widthx.StripTerminalSequences(strings.Join(frame.Lines, "\n"))
		if !strings.Contains(visible, "›") || !strings.Contains(visible, "agent_19") {
			t.Fatalf("fullscreen layout clipped the selected row at height %d: %q", height, visible)
		}
	}
	for _, line := range tree.Render(30) {
		if widthx.VisibleWidth(line) > 30 {
			t.Fatalf("window overflowed: %q", line)
		}
	}
}

func TestAgentTreeQueuedAndDisabledManagers(t *testing.T) {
	manager := subagents.New(subagents.Host{
		CWD: t.TempDir(), AgentDir: t.TempDir(), SessionID: "queue-test",
		Create: func(context.Context, subagents.Invocation) (subagents.Child, error) {
			return &blockingChild{}, nil
		},
	})
	if err := manager.Update(map[string]any{"enabled": true, "maxConcurrent": 1}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	if _, err := manager.Spawn(context.Background(), subagents.Params{Prompt: "one", Description: "running row", Background: new(true)}, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Spawn(context.Background(), subagents.Params{Prompt: "two", Description: "queued row", Background: new(true)}, "", ""); err != nil {
		t.Fatal(err)
	}
	m := &InteractiveMode{opts: InteractiveModeOptions{SessionHandle: observerSession{manager: manager}}, tuiInst: tui.NewWithOutput(io.Discard, 100, 24), editor: tui.NewEditor()}
	m.tuiInst.SetRenderDeadline(func(time.Time) {})
	m.subagentUI = &subagentUIState{manager: manager}
	rows, _ := m.agentTreeRecords()
	statuses := map[string]string{}
	for _, r := range rows {
		statuses[r.Description] = r.Status
	}
	if statuses["running row"] != "running" || statuses["queued row"] != "queued" {
		t.Fatalf("eligible rows lost the queued record: %v", statuses)
	}
	if !m.handleAgentKey("\x1b[B") {
		t.Fatal("Down did not open the tree")
	}
	if !m.handleAgentKey("\x1b[B") || m.subagentUI.treeSelected != rows[1].ID {
		t.Fatal("the queued row is not selectable")
	}
	text := widthx.StripTerminalSequences(strings.Join((&nativeAgentTree{m: m}).Render(80), "\n"))
	if !strings.Contains(text, "queued row") {
		t.Fatalf("the queued record is not drawn: %q", text)
	}
	total, working := m.subagentCounts()
	if total != 2 || working != 1 {
		t.Fatalf("counts = %d total %d working, want 2 and 1", total, working)
	}
	if err := manager.Update(map[string]any{"enabled": false}); err != nil {
		t.Fatal(err)
	}
	if m.handleAgentKey("\x1b[B") {
		t.Fatal("disabled manager opened the tree")
	}
	if total, working = m.subagentCounts(); total != 0 || working != 0 {
		t.Fatalf("disabled counts = %d %d", total, working)
	}
	if err := manager.Update(map[string]any{"enabled": true, "widgetMode": "off"}); err != nil {
		t.Fatal(err)
	}
	if m.handleAgentKey("\x1b[B") {
		t.Fatal("widgetMode off opened the tree")
	}
	if total, working = m.subagentCounts(); total != 0 || working != 0 {
		t.Fatalf("widgetMode off counts = %d %d", total, working)
	}
}

func TestAgentTreeExpiresFinishedRecords(t *testing.T) {
	now := time.Now().UnixMilli()
	m, _ := treeFixture(t, []subagents.Record{
		{ID: "fresh", Type: "Explore", Description: "fresh", Status: "completed", StartedAt: now - 5000, CompletedAt: now - 1000, Background: true},
		{ID: "old", Type: "Explore", Description: "old", Status: "completed", StartedAt: now - 60000, CompletedAt: now - 31000, Background: true},
		{ID: "err", Type: "Explore", Description: "errored", Status: "error", StartedAt: now - 60000, CompletedAt: now - 31000, Background: true},
	})
	for _, status := range []string{"queued", "scheduled"} {
		if got := agentTreeFinishedAt(subagents.Record{Status: status, StartedAt: now - 60000}); got != 0 {
			t.Fatalf("%s record is not finished: %d", status, got)
		}
	}
	rows, _ := m.agentTreeRecords()
	if len(rows) != 1 || rows[0].ID != "fresh" {
		t.Fatalf("expiry kept the wrong rows: %v", rows)
	}
	if m.subagentUI.expireTimer == nil || m.subagentUI.expireAt.IsZero() {
		t.Fatal("no expiry timer armed for the remaining finished record")
	}
}

func TestAgentTreeCleansUpWithoutInput(t *testing.T) {
	m, _ := treeFixture(t, []subagents.Record{{
		ID: "a", Type: "Explore", Description: "done", Status: "completed",
		CompletedAt: time.Now().Add(-agentTreeFinishedTTL + 600*time.Millisecond).UnixMilli(), Background: true,
	}})
	armed := make(chan time.Time, 8)
	m.tuiInst.SetRenderDeadline(func(due time.Time) {
		if !due.IsZero() {
			armed <- due
		}
	})
	if rows, _ := m.agentTreeRecords(); len(rows) != 1 {
		t.Fatalf("the finishing record was not listed: %d", len(rows))
	}
	select {
	case <-armed:
	case <-time.After(5 * time.Second):
		t.Fatal("the expiry timer did not request a frame")
	}
	if rows, _ := m.agentTreeRecords(); len(rows) != 0 {
		t.Fatalf("the finished record survived its TTL: %v", rows)
	}
	if m.subagentUI.expireTimer != nil || !m.subagentUI.expireAt.IsZero() {
		t.Fatal("the expiry timer stayed armed with nothing left to clean")
	}
	if m.handleAgentKey("\x1b[B") {
		t.Fatal("Down reopened a tree with no eligible records")
	}
}

type blockingChild struct{}

func (c *blockingChild) Send(ctx context.Context, _ string) ([]agent.AgentMessage, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (c *blockingChild) QueueSteer(string, []ai.ImageContent)    {}
func (c *blockingChild) Messages() []agent.AgentMessage          { return nil }
func (c *blockingChild) Path() string                            { return "" }
func (c *blockingChild) Close() error                            { return nil }
func (c *blockingChild) Subscribe(func(agent.AgentEvent)) func() { return func() {} }

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

func TestAgentCardStaysMinimal(t *testing.T) {
	m, _ := observerFixture(t)
	args := json.RawMessage(`{"role":"@default","tool":["read"],"description":"Run sleep loop one"}`)
	card := tui.NewToolExecutionComponent("Agent", "minimal", args, tui.ToolExecutionOptions{}, nil, nil, "")
	m.bindToolCard(card, "minimal", "Agent", args)
	assertMinimal := func(background string) {
		t.Helper()
		var content []string
		lines := card.Render(80)
		if !strings.Contains(strings.Join(lines, "\n"), tui.ActiveTheme().GetBgAnsi(background)) {
			t.Fatalf("missing standard tool background %s", background)
		}
		for _, line := range lines {
			if clean := strings.TrimSpace(widthx.StripTerminalSequences(line)); clean != "" {
				content = append(content, clean)
			}
		}
		if len(content) != 1 || content[0] != "Agent  Run sleep loop one" {
			t.Fatalf("expected one descriptive line, got %q", content)
		}
	}
	assertMinimal("toolPendingBg")
	for _, failed := range []bool{false, true} {
		card.SetResult("tool uses: 99\nprivate result\nctrl+o to expand", failed, time.Hour)
		for _, expanded := range []bool{false, true} {
			card.SetExpanded(expanded)
			background := "toolSuccessBg"
			if failed {
				background = "toolErrorBg"
			}
			assertMinimal(background)
		}
	}
	call := subagentRenderCall(json.RawMessage(`{"description":"long\nlabel\u001b[31m"}`), nil, extension.ToolRenderContext{})
	if lines := call.Render(9); len(lines) != 1 || widthx.VisibleWidth(lines[0]) > 9 {
		t.Fatalf("description must remain one bounded line: %q", lines)
	}
}

func TestSubagentNotificationCardMarksIncompleteOutcomes(t *testing.T) {
	for status, wantMark := range map[string]string{"completed": "✓", "steered": "✓", "error": "✗", "stopped": "✗", "exhausted": "✗"} {
		card := &subagentNotificationCard{text: `<task-notification><task-id>agent_test</task-id><status>` + status + `</status><summary>Audit</summary><result>partial</result></task-notification>`}
		rendered := widthx.StripAnsi(strings.Join(card.Render(50), "\n"))
		if !strings.HasPrefix(rendered, wantMark+" Audit") {
			t.Fatalf("status %q rendered %q, want the %s mark", status, rendered, wantMark)
		}
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
