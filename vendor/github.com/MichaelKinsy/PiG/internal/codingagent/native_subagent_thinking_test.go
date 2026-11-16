package codingagent

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
	"github.com/MichaelKinsy/PiG/internal/subagents"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

type thinkingChild struct {
	mu       sync.Mutex
	listener func(agent.AgentEvent)
	emit     func(func(agent.AgentEvent))
}

func (c *thinkingChild) Send(ctx context.Context, _ string) ([]agent.AgentMessage, error) {
	c.mu.Lock()
	listener := c.listener
	c.mu.Unlock()
	c.emit(listener)
	<-ctx.Done()
	return nil, ctx.Err()
}
func (c *thinkingChild) QueueSteer(string, []ai.ImageContent) {}
func (c *thinkingChild) Messages() []agent.AgentMessage       { return nil }
func (c *thinkingChild) Path() string                         { return "" }
func (c *thinkingChild) Close() error                         { return nil }
func (c *thinkingChild) Subscribe(fn func(agent.AgentEvent)) func() {
	c.mu.Lock()
	c.listener = fn
	c.mu.Unlock()
	return func() {}
}

func thinkingUpdate(thinking string, redacted bool, signature string) agent.MessageUpdateEvent {
	return agent.MessageUpdateEvent{Message: agent.AgentMessage{Assistant: &agent.AssistantMessage{Role: "assistant", Content: []ai.AssistantContentBlock{ai.ThinkingContent{Thinking: thinking, Redacted: redacted, ThinkingSignature: signature}}}}}
}

func liveThinkingFixture(t *testing.T, updates ...agent.MessageUpdateEvent) (*InteractiveMode, *subagents.Manager) {
	t.Helper()
	created := make(chan struct{})
	manager := subagents.New(subagents.Host{
		CWD: t.TempDir(), AgentDir: t.TempDir(), SessionID: "thinking-test",
		Create: func(context.Context, subagents.Invocation) (subagents.Child, error) {
			return &thinkingChild{emit: func(listener func(agent.AgentEvent)) {
				if listener == nil {
					return
				}
				listener(agent.TurnStartEvent{})
				for _, update := range updates {
					listener(update)
				}
				close(created)
			}}, nil
		},
	})
	if err := manager.Update(map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	if _, err := manager.Spawn(context.Background(), subagents.Params{Prompt: "Think.", Description: "Inspect live thought stream", Role: "@slow", Name: "named-ui-worker", Background: new(true)}, "", ""); err != nil {
		t.Fatal(err)
	}
	<-created
	m := &InteractiveMode{opts: InteractiveModeOptions{SessionHandle: observerSession{manager: manager}}, tuiInst: tui.NewWithOutput(io.Discard, 100, 40), editor: tui.NewEditor()}
	m.tuiInst.SetRenderDeadline(func(time.Time) {})
	m.subagentUI = &subagentUIState{manager: manager}
	t.Cleanup(func() {
		if m.subagentUI.hub != nil {
			m.subagentUI.hub.close()
		}
		for m.subagentUI.viewer != nil {
			m.subagentUI.viewer.close()
		}
	})
	return m, manager
}

func TestSubagentThinkingPreviewFollowsStreamTail(t *testing.T) {
	line := subagentThinkingLine(strings.Repeat("earlier ", 50)+"latest thought", 24)
	if got := widthx.StripTerminalSequences(line); !strings.HasSuffix(got, "latest thought") || widthx.VisibleWidth(line) > 24 {
		t.Fatalf("preview does not follow current thinking: %q", got)
	}
}

func TestSubagentThinkingLineSanitizesUnicodeTail(t *testing.T) {
	line := subagentThinkingLine("gone\x07\n温故而知新的想法 latest", 20)
	plain := widthx.StripTerminalSequences(line)
	if !strings.HasSuffix(plain, "想法 latest") || strings.ContainsRune(plain, '\a') || widthx.VisibleWidth(line) > 20 {
		t.Fatalf("sanitized unicode tail lost its content or width: %q", plain)
	}
}

func TestSubagentThinkingLinesLimitsToTail(t *testing.T) {
	lines := subagentThinkingLines("one\ntwo\nthree\nfour\nfive", 40, 3)
	want := []string{"… 3 earlier lines", "four", "five"}
	if len(lines) != len(want) {
		t.Fatalf("limited thinking kept %d lines, want %d: %q", len(lines), len(want), lines)
	}
	for i, line := range lines {
		if plain := widthx.StripTerminalSequences(line); plain != want[i] {
			t.Fatalf("line %d: got %q, want %q", i, plain, want[i])
		}
	}
	if marker := tui.ActiveTheme().Fg("dim", "… 3 earlier lines"); lines[0] != marker {
		t.Fatalf("hidden-lines marker lost its muted style: %q", lines[0])
	}
}

func TestSubagentWidgetStreamsLiveThinking(t *testing.T) {
	m, _ := liveThinkingFixture(t, thinkingUpdate("THOUGHT_ONE", false, ""), thinkingUpdate("THOUGHT_ONE THOUGHT_TWO", false, ""))
	widget := &nativeAgentWidget{m: m}
	lines := widget.Render(120)
	plain := widthx.StripTerminalSequences(strings.Join(lines, "\n"))
	if !strings.Contains(plain, "THOUGHT_ONE THOUGHT_TWO") {
		t.Fatalf("widget lost the streamed thinking: %q", plain)
	}
	if strings.Count(plain, "THOUGHT_") != 2 {
		t.Fatalf("widget kept a stale thinking snapshot: %q", plain)
	}
	if !strings.Contains(plain, "thinking…") {
		t.Fatalf("widget dropped the existing activity row: %q", plain)
	}
	if !strings.Contains(plain, "@slow named-ui-worker  Inspect live thought stream") {
		t.Fatalf("widget row lost its role, name or task: %q", plain)
	}
	for _, line := range lines {
		if widthx.VisibleWidth(line) > 120 {
			t.Fatalf("thinking preview overflowed the widget: %q", line)
		}
	}
}

func TestSubagentWidgetAlignsContinuationRows(t *testing.T) {
	m, _ := liveThinkingFixture(t, thinkingUpdate("THOUGHT_ALIGNED", false, ""))
	lines := (&nativeAgentWidget{m: m}).Render(120)
	plain := widthx.StripTerminalSequences(strings.Join(lines, "\n"))
	if strings.Contains(plain, "├─") {
		t.Fatalf("last branch was not closed: %q", plain)
	}
	for _, line := range lines[1:] {
		if strings.HasPrefix(widthx.StripTerminalSequences(line), "│") {
			t.Fatalf("a continuation row kept the closed guide: %q", line)
		}
	}
}

func TestSubagentThinkingHidesRedactedAndSignatures(t *testing.T) {
	blocks := []ai.AssistantContentBlock{
		ai.ThinkingContent{Thinking: "secret", Redacted: true},
		ai.ThinkingContent{Thinking: "visible thought", ThinkingSignature: "sig-should-not-render"},
	}
	text := subagentThinkingText(blocks)
	if text != "visible thought" {
		t.Fatalf("redacted or signed thinking leaked: %q", text)
	}
	m, _ := liveThinkingFixture(t, thinkingUpdate("visible thought", false, "sig-should-not-render"))
	rendered := strings.Join((&nativeAgentWidget{m: m}).Render(120), "\n")
	if strings.Contains(widthx.StripAnsi(rendered), "sig-should-not-render") {
		t.Fatal("widget rendered a thinking signature")
	}
}

func TestSubagentViewerShowsThinkingInEveryMarkdownMode(t *testing.T) {
	m, manager := liveThinkingFixture(t, thinkingUpdate("# THOUGHT_VIEWER", false, ""))
	for _, markdown := range []string{"assistant", "all", "off"} {
		m.openSubagentViewer("named-ui-worker", false)
		v := m.subagentUI.viewer
		v.markdown = markdown
		rendered := widthx.StripTerminalSequences(strings.Join(v.Render(100), "\n"))
		if !strings.Contains(rendered, "THOUGHT_VIEWER") {
			t.Fatalf("viewer dropped thinking in markdown mode %q: %q", markdown, rendered)
		}
		v.close()
	}
	record, ok := manager.Find("named-ui-worker")
	if !ok || !record.Active() {
		t.Fatal("fixture child is not still running")
	}
}

func TestHubInspectorShowsLiveThinking(t *testing.T) {
	m, _ := liveThinkingFixture(t, thinkingUpdate("THOUGHT_HUB_LIVE", false, ""))
	m.toggleAgentHub()
	h := m.subagentUI.hub
	h.height = 40
	h.Render(120)
	h.rows[h.index].Activity = "reading…"
	inspector := widthx.StripTerminalSequences(strings.Join(h.details(60), "\n"))
	for _, want := range []string{"THOUGHT_HUB_LIVE", "Current", "reading…", "Task"} {
		if !strings.Contains(inspector, want) {
			t.Fatalf("inspector lost %q: %q", want, inspector)
		}
	}
	if theme := tui.ActiveTheme(); !strings.Contains(strings.Join(h.details(60), "\n"), theme.ThinkingText) {
		t.Fatal("inspector thinking is not drawn in the transcript thinkingText hue")
	}
	if !strings.Contains(inspector, "@slow named-ui-worker") && !strings.Contains(inspector, "named-ui-worker") {
		t.Fatalf("inspector lost the child's name: %q", inspector)
	}
	for _, line := range h.details(60) {
		if widthx.VisibleWidth(line) > 60 {
			t.Fatalf("inspector thinking overflowed: %q", line)
		}
	}
	h.rows[h.index].State = "idle"
	if strings.Contains(widthx.StripTerminalSequences(strings.Join(h.details(60), "\n")), "THOUGHT_HUB_LIVE") {
		t.Fatal("inspector showed a live thought for a non-running child")
	}
}

func TestSubagentRoleUsesCatalogColor(t *testing.T) {
	theme := tui.ModelOverlayTheme()
	settings := Settings{Config: modelroles.Config{
		ModelRoles: map[string]string{"slow": "p/a"},
		ModelTags:  map[string]modelroles.Tag{"review": {Color: "warning"}, "broken": {Color: "not-a-token"}},
	}}
	m := &InteractiveMode{opts: InteractiveModeOptions{Settings: settings}}
	m.opts.SettingsManager = NewInMemorySettingsManager(settings)
	for _, tc := range []struct{ role, wantToken string }{
		{"@task", "success"},
		{"@smol", "warning"},
		{"@slow", "accent"},
		{"@default", "success"},
		{"@review", "warning"},
		{"@broken", "muted"},
		{"@unknown", "muted"},
	} {
		want := theme.Fg(tc.wantToken, tc.role)
		if got := m.subagentRoleSelector(tc.role); got != want {
			t.Fatalf("role %s: got %q, want %s %q", tc.role, got, tc.wantToken, want)
		}
	}
	muted := theme.GetFgAnsi("muted")
	for _, role := range []string{"@task", "@smol", "@slow"} {
		row := m.subagentRow(subagents.Record{Type: "Explore", Params: subagents.Params{Role: role}})
		if strings.HasPrefix(row, muted) {
			t.Fatalf("%s is drawn in the muted hue: %q", role, row)
		}
		if !strings.HasPrefix(row, m.subagentRoleSelector(role)) {
			t.Fatalf("%s row does not start with its role label: %q", role, row)
		}
	}
	m.opts.SettingsManager = NewInMemorySettingsManager(Settings{Config: modelroles.Config{
		ModelTags: map[string]modelroles.Tag{"task": {Color: "error"}},
	}})
	if got, want := m.subagentRoleSelector("@task"), theme.Fg("error", "@task"); got != want {
		t.Fatalf("tag override lost to the built-in task color: got %q, want %q", got, want)
	}
	if got := m.subagentRoleSelector(""); got != "" {
		t.Fatalf("a role-free child rendered a role: %q", got)
	}
	if m.subagentRoleSelector("slow") != m.subagentRoleSelector("@slow") {
		t.Fatal("role selector form changed the identity")
	}
	record := subagents.Record{Type: "Explore", DisplayName: "Explore", Handle: "explore"}
	if got := m.subagentRow(record); got != subagentName(record) {
		t.Fatalf("role-free row changed: %q", got)
	}
	m.opts.SettingsManager = nil
	if got, want := m.subagentRoleSelector("@review"), theme.Fg("warning", "@review"); got != want {
		t.Fatalf("catalog snapshot ignored: got %q, want %q", got, want)
	}
}

func TestSubagentIdentityPrefersNameOverType(t *testing.T) {
	cases := []struct {
		record subagents.Record
		want   string
	}{
		{subagents.Record{Alias: "worker", Handle: "agent-2", DisplayName: "Explore", Type: "Explore"}, "worker"},
		{subagents.Record{Handle: "agent-2", DisplayName: "Explore", Type: "Explore"}, "agent-2"},
		{subagents.Record{DisplayName: "Explore", Type: "Explore"}, "Explore"},
		{subagents.Record{Type: "Explore"}, "Explore"},
		{subagents.Record{ID: "agent_test"}, "agent_test"},
	}
	for _, tc := range cases {
		if got := subagentIdentity(tc.record); got != tc.want {
			t.Fatalf("identity %+v: got %q, want %q", tc.record, got, tc.want)
		}
		if got := hubLabel(subagents.HubAgent{Record: tc.record}); got != tc.want {
			t.Fatalf("hub identity disagrees with the tree: got %q, want %q", got, tc.want)
		}
	}
}

func TestSubagentStatsKeepsOneMutedForeground(t *testing.T) {
	theme := tui.ActiveTheme()
	record := subagents.Record{Turns: 2, ToolUses: 3, Tokens: 12400, ContextWindow: 200000, ContextTokens: 160000, Compactions: 2, StartedAt: time.Now().UnixMilli()}
	stats := subagentStats(record, subagents.DefaultSettings())
	line := theme.Fg("dim", " · "+stats)
	badge := theme.Fg("warning", "80%")
	index := strings.Index(line, badge)
	if index < 0 {
		t.Fatalf("80%% signal lost its warning color: %q", line)
	}
	tail := line[index+len(badge):]
	if !strings.HasPrefix(tail, theme.GetFgAnsi("dim")) {
		t.Fatalf("text after a colored signal was not repainted muted: %q", line)
	}
	if !strings.Contains(tail, " · ⇊2)") {
		t.Fatalf("closing parenthesis is not restored to the muted run: %q", line)
	}
	if strings.Contains(strings.TrimSuffix(tail, "\x1b[39m"), "\x1b[39m") {
		t.Fatalf("a foreground reset after the signal left the line unstyled: %q", line)
	}
	if !strings.Contains(line, "12.4k token (") {
		t.Fatalf("opening parenthesis moved: %q", line)
	}
}

func TestSubagentStatsMatchAcrossRows(t *testing.T) {
	theme := tui.ActiveTheme()
	line := theme.Fg("dim", subagentStats(subagents.Record{Turns: 1, Tokens: 800, Compactions: 3, StartedAt: time.Now().UnixMilli()}, subagents.DefaultSettings()))
	if !strings.Contains(widthx.StripTerminalSequences(line), "(⇊3)") {
		t.Fatalf("plain signals lost: %q", line)
	}
	if strings.Contains(strings.TrimSuffix(line, "\x1b[39m"), "\x1b[39m") {
		t.Fatalf("plain signals reset the muted foreground: %q", line)
	}
}

func TestSubagentRenderCallNamesTheChild(t *testing.T) {
	named := subagentRenderCall(json.RawMessage(`{"name":"named-ui-worker","description":"Inspect"}`), nil, extension.ToolRenderContext{})
	if plain := widthx.StripAnsi(strings.Join(named.Render(80), "\n")); !strings.HasPrefix(plain, "named-ui-worker  Inspect") {
		t.Fatalf("named call card lost its name: %q", plain)
	}
	unnamed := subagentRenderCall(json.RawMessage(`{"name":"  ","description":"Inspect"}`), nil, extension.ToolRenderContext{})
	if plain := widthx.StripAnsi(strings.Join(unnamed.Render(80), "\n")); !strings.HasPrefix(plain, "Agent  Inspect") {
		t.Fatalf("unnamed call card lost its tool name: %q", plain)
	}
}
