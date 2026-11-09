package codingagent

// Native presentation port of pi-subagents src/ui/{agent-widget,
// conversation-viewer}.ts. All component mutation runs on the TUI owner loop.
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/internal/subagents"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

type subagentUIState struct {
	manager     *subagents.Manager
	hub         *agentHub
	lastLeftTap time.Time
	viewer      *subagentViewer
}

func (m *InteractiveMode) subagentManager() *subagents.Manager {
	if host, ok := m.opts.SessionHandle.(interface{ Subagents() *subagents.Manager }); ok {
		return host.Subagents()
	}
	return nil
}
func (m *InteractiveMode) bindSubagentUI() {
	manager := m.subagentManager()
	if m.subagentUI != nil && m.subagentUI.manager == manager {
		return
	}
	if m.subagentUI != nil {
		if m.subagentUI.manager != nil {
			m.subagentUI.manager.SetChanged(nil)
		}
		if m.subagentUI.hub != nil {
			m.subagentUI.hub.close()
		}
		for m.subagentUI.viewer != nil {
			m.subagentUI.viewer.close()
		}
	}
	m.subagentUI = &subagentUIState{manager: manager}
	if manager != nil {
		manager.SetChanged(func() { m.wakeSpinner(); m.requestRender() })
	}
}
func (m *InteractiveMode) subagentsActive() bool {
	manager := m.subagentManager()
	if manager == nil || !manager.Settings().Enabled {
		return false
	}
	now := time.Now().UnixMilli()
	for _, r := range manager.Records() {
		if r.TopLevel() && (r.Active() || now-r.CompletedAt < 4000) {
			return true
		}
	}
	for _, w := range manager.Workflows() {
		if w.Active() {
			return true
		}
	}
	return false
}

func subagentCompact(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprint(n)
}
func subagentElapsed(r subagents.Record) string {
	end := r.CompletedAt
	if end == 0 {
		end = time.Now().UnixMilli()
	}
	d := time.Duration(max(0, end-r.StartedAt)) * time.Millisecond
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

var subagentSpinners = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func subagentIcon(r subagents.Record) string {
	theme := tui.ActiveTheme()
	switch r.Status {
	case "running":
		return theme.Fg("accent", subagentSpinners[time.Now().UnixMilli()/100%10])
	case "queued":
		return theme.Fg("muted", "◦")
	case "completed":
		return theme.Fg("success", "✓")
	case "steered":
		return theme.Fg("warning", "✓")
	case "stopped":
		return theme.Fg("dim", "■")
	}
	return theme.Fg("error", "✗")
}
func subagentName(r subagents.Record) string {
	name := r.DisplayName
	if name == "" {
		name = r.Type
	}
	if r.Color != "" {
		return subagentBadge(name, r.Color)
	}
	return tui.ChalkBold(name)
}
func subagentStats(r subagents.Record, s subagents.Settings) string {
	var parts []string
	if s.ShowModel && r.ModelName != "" {
		parts = append(parts, r.ModelName, "thinking: "+r.Thinking)
	}
	turns := fmt.Sprintf("%d turns", r.Turns)
	if r.MaxTurns > 0 {
		turns = fmt.Sprintf("%d/%d turns", r.Turns, r.MaxTurns)
	}
	parts = append(parts, turns, fmt.Sprintf("%d tool uses", r.ToolUses))
	if r.Tokens > 0 {
		tokens := subagentCompact(r.Tokens) + " token"
		var signals []string
		if r.ContextWindow > 0 && r.ContextTokens > 0 {
			percent := float64(r.ContextTokens) * 100 / float64(r.ContextWindow)
			color := "dim"
			if percent >= 85 {
				color = "error"
			} else if percent >= 70 {
				color = "warning"
			}
			signals = append(signals, tui.ActiveTheme().Fg(color, fmt.Sprintf("%.0f%%", percent)))
		}
		if r.Compactions > 0 {
			signals = append(signals, fmt.Sprintf("⇊%d", r.Compactions))
		}
		if len(signals) > 0 {
			tokens += " (" + strings.Join(signals, " · ") + ")"
		}
		parts = append(parts, tokens)
	}
	if s.ShowCost && r.Cost > 0 {
		parts = append(parts, fmt.Sprintf("$%.4f", r.Cost))
	}
	return strings.Join(append(parts, subagentElapsed(r)), " · ")
}
func clampAgentLine(s string, width int) string {
	return widthx.TruncateToWidth(s, max(1, width), "…", false)
}

type nativeAgentWidget struct{ m *InteractiveMode }

func (w *nativeAgentWidget) Invalidate() {}
func (w *nativeAgentWidget) Render(width int) []string {
	manager := w.m.subagentManager()
	if manager == nil {
		return nil
	}
	s := manager.Settings()
	if !s.Enabled {
		return nil
	}
	if s.WidgetMode == "off" {
		return nil
	}
	theme := tui.ActiveTheme()
	lines := []string{}
	queued := 0
	overflow := 0
	for _, r := range manager.Records() {
		if !r.TopLevel() || s.WidgetMode == "background" && !r.Background {
			continue
		}
		if !r.Active() && time.Now().UnixMilli()-r.CompletedAt > 4000 {
			continue
		}
		if r.Status == "queued" {
			queued++
			continue
		}
		if len(lines) > 8 {
			overflow++
			continue
		}
		lines = append(lines, theme.Fg("dim", "├─ ")+subagentIcon(r)+" "+subagentName(r)+"  "+theme.Fg("muted", r.Description)+theme.Fg("dim", " · "+subagentStats(r, s)))
		if r.Active() {
			activity := r.Activity
			if activity == "" {
				activity = "thinking…"
			}
			lines = append(lines, theme.Fg("dim", "│    ⎿  "+activity))
		}
	}
	if queued > 0 {
		lines = append(lines, theme.Fg("dim", fmt.Sprintf("├─ ◦ %d queued", queued)))
	}
	if overflow > 0 {
		lines = append(lines, theme.Fg("dim", fmt.Sprintf("├─ … %d more", overflow)))
	}
	if len(lines) == 0 {
		return nil
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "├─") {
			lines[i] = strings.Replace(lines[i], "├─", "└─", 1)
			if i+1 < len(lines) {
				lines[i+1] = strings.Replace(lines[i+1], "│", " ", 1)
			}
			break
		}
	}
	lines = append([]string{theme.Fg("accent", "● Agents")}, lines...)
	for i := range lines {
		lines[i] = clampAgentLine(lines[i], width)
	}
	return lines
}

func (m *InteractiveMode) handleAgentKey(data string) bool {
	manager := m.subagentManager()
	if manager == nil || !manager.Settings().Enabled {
		return false
	}
	if tui.MatchesKeyID(data, "ctrl+b") && manager.Background("") {
		m.requestRender()
		return true
	}
	matches := func(action, fallback string) bool {
		if m.keybindings != nil {
			return m.keybindings.Matches(data, action)
		}
		return tui.MatchesKeyID(data, fallback)
	}
	if matches("app.agents.hub", "alt+a") || matches("app.session.observe", "ctrl+s") {
		m.toggleAgentHub()
		return true
	}
	if m.editor == nil || m.subagentUI == nil {
		return false
	}
	if m.editor.Text() != "" {
		m.subagentUI.lastLeftTap = time.Time{}
		return false
	}
	if tui.MatchesKeyID(data, "left") && len(manager.Records()) > 0 {
		now := time.Now()
		if !m.subagentUI.lastLeftTap.IsZero() && now.Sub(m.subagentUI.lastLeftTap) <= 500*time.Millisecond {
			m.subagentUI.lastLeftTap = time.Time{}
			m.toggleAgentHub()
		} else {
			m.subagentUI.lastLeftTap = now
		}
		return true
	}
	m.subagentUI.lastLeftTap = time.Time{}
	return false
}

func (m *InteractiveMode) subagentsCommand(ctx context.Context, args string) error {
	manager := m.subagentManager()
	if manager == nil {
		return fmt.Errorf("subagents are unavailable")
	}
	args = strings.TrimSpace(args)
	switch args {
	case "", "on", "off":
		enabled := !manager.Settings().Enabled
		if args == "on" {
			enabled = true
		}
		if args == "off" {
			enabled = false
		}
		if err := manager.Update(map[string]any{"enabled": enabled}); err != nil {
			return err
		}
		m.showStatus("Subagents " + map[bool]string{true: "enabled", false: "disabled"}[enabled])
		m.requestRender()
		return nil
	case "hub":
		m.toggleAgentHub()
		return nil
	case "status":
		s := manager.Settings()
		m.showStatus(fmt.Sprintf("Subagents enabled=%t · %d background slots · %d agents", s.Enabled, s.MaxConcurrent, len(manager.Records())))
		return nil
	case "menu", "settings":
	default:
		return fmt.Errorf("usage: /subagents [on|off|status|menu|settings]")
	}
	for {
		options := []string{"Agent Hub", "Agent types", "Workflows", "Scheduled jobs", "Settings"}
		choice, ok := m.askSelector(ctx, "Agents", options)
		if args == "settings" {
			choice = "Settings"
			ok = true
		}
		if !ok {
			return nil
		}
		switch choice {
		case "Agent Hub":
			m.toggleAgentHub()
			return nil
		case "Workflows":
			var labels []string
			workflows := manager.Workflows()
			for _, w := range workflows {
				labels = append(labels, w.Meta.Name+" · "+w.Status+" · "+w.ID)
			}
			picked, ok := m.askSelector(ctx, "Workflows", labels)
			if ok {
				for i, label := range labels {
					if label == picked {
						m.openSubagentViewer(workflows[i].ID, true)
						return nil
					}
				}
			}
		case "Scheduled jobs":
			jobs := manager.Jobs()
			var labels []string
			for _, j := range jobs {
				if !j.Cancelled {
					labels = append(labels, j.ID+" · "+j.Schedule+" · "+j.Params.Description)
				}
			}
			picked, ok := m.askSelector(ctx, "Scheduled jobs (select to cancel)", labels)
			if ok {
				if err := manager.CancelJob(strings.Split(picked, " · ")[0]); err != nil {
					return err
				}
			}
		case "Agent types":
			configs := manager.Configs()
			var labels []string
			for _, c := range configs {
				labels = append(labels, c.Name+" · "+c.Source+" · "+c.Description)
			}
			labels = append(labels, "Create agent")
			picked, ok := m.askSelector(ctx, "Agent types", labels)
			if !ok {
				continue
			}
			if picked == "Create agent" {
				name, ok := m.askEditor(ctx, "Agent name", "")
				if !ok {
					continue
				}
				path := filepath.Join(m.opts.CWD, ".pi", "agents", name+".md")
				body := "---\nname: " + name + "\ndescription: Custom agent\n---\nYou are a helpful specialist."
				if _, err := subagents.ParseConfig(path, []byte(body)); err != nil {
					return err
				}
				if _, err := os.Stat(path); err == nil {
					return fmt.Errorf("agent file already exists")
				}
				edited, ok := m.askEditor(ctx, "Create agent", body)
				if !ok {
					continue
				}
				if _, err := subagents.ParseConfig(path, []byte(edited)); err != nil {
					return err
				}
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					return err
				}
				if err := os.WriteFile(path, []byte(edited), 0600); err != nil {
					return err
				}
				if err := manager.Update(map[string]any{}); err != nil {
					return err
				}
			} else {
				for i, label := range labels[:len(labels)-1] {
					if picked != label {
						continue
					}
					c := configs[i]
					if c.Source == "package" {
						action, ok := m.askSelector(ctx, c.Name+" · installed package (read-only)", []string{"View", "Eject to project", "Disable for project"})
						if !ok {
							continue
						}
						if action == "View" {
							m.showStatus(c.Description + " · " + c.SourcePath + "\n" + c.SystemPrompt)
						} else {
							path, err := manager.EjectConfig(c, action == "Disable for project")
							if err != nil {
								return err
							}
							m.showStatus("Project override saved: " + path)
						}
						continue
					}
					if c.SourcePath == "" {
						m.showStatus(c.Description + " · define .pi/agents/" + c.Name + ".md to override")
						continue
					}
					b, err := os.ReadFile(c.SourcePath)
					if err != nil {
						return err
					}
					edited, ok := m.askEditor(ctx, c.Name, string(b))
					if !ok {
						continue
					}
					if _, err := subagents.ParseConfig(c.SourcePath, []byte(edited)); err != nil {
						return err
					}
					if err := os.WriteFile(c.SourcePath, []byte(edited), 0600); err != nil {
						return err
					}
					if err := manager.Update(map[string]any{}); err != nil {
						return err
					}
				}
			}
		case "Settings":
			if err := m.subagentSettings(ctx, manager); err != nil {
				return err
			}
			if args == "settings" {
				return nil
			}
		}
	}
}
func (m *InteractiveMode) subagentSettings(ctx context.Context, manager *subagents.Manager) error {
	selectedID := ""
	for {
		settings := manager.Settings()
		b, _ := json.Marshal(settings)
		var raw map[string]any
		_ = json.Unmarshal(b, &raw)
		specs := []struct {
			id, label, description string
			values                 []string
		}{
			{"enabled", "Enabled", "Built-in subagents. Turning off stops and drains active agents.", nil},
			{"maxConcurrent", "Max concurrency", "Max concurrent background agents (Enter to type)", nil},
			{"maxConcurrentForeground", "Max foreground concurrency", "Max concurrent foreground agents (0 = unlimited, Enter to type)", nil},
			{"defaultMaxTurns", "Default max turns", "Default turns before wrap-up (0 = unlimited, Enter to type)", nil},
			{"graceTurns", "Grace turns", "Grace turns after wrap-up steer (Enter to type)", nil},
			{"maxSubagentDepth", "Nested depth", "Hard cap on nested delegation; main is 0 (Enter to type)", nil},
			{"defaultJoinMode", "Join mode", "smart = batch siblings while parent is idle; group = always batch (up to 30s)", []string{"smart", "group"}},
			{"backgroundByDefault", "Background by default", "An Agent call that doesn't say runs detached", nil},
			{"schedulingEnabled", "Scheduling", "Schedule agents using cron, intervals, or one-shot timestamps", nil},
			{"workflowsEnabled", "Workflows", "Deterministic scripted workflow orchestration", nil},
			{"packageAgents", "Package agents", "Load agents declared by installed packages; local agents override them", nil},
			{"packageWorkflows", "Package workflows", "Load workflows declared by installed packages", nil},
			{"strictAgentFiles", "Strict agent files", "Refuse activation when a local agent file is invalid", nil},
			{"disableDefaultAgents", "Disable default agents", "Hide embedded agents; your custom files remain available", nil},
			{"agentMentions", "Agent mentions", "direct = start immediately without an orchestration model call; off = file mentions only", []string{"direct", "off"}},
			{"rememberAgents", "Remember agents", "Keep completed agents addressable across session reloads", nil},
			{"widgetMode", "Widget", "Which agents appear above the editor", []string{"background", "all", "off"}},
			{"toolDescriptionMode", "Tool description", "Compact by default; full or project-authored descriptions are opt-in", []string{"full", "compact", "custom"}},
			{"outputTranscript", "Output transcript", "Write streaming JSONL .output files", nil},
			{"worktreeIsolation", "Worktree isolation", "Allow temporary git worktrees for parallel edits", nil},
			{"reportUsage", "Report usage", "Include subagent token usage in tool results", nil},
			{"showCost", "Show cost", "Show estimated cost in agent displays", nil},
			{"showModel", "Show model", "Show model and thinking level in agent displays", nil},
			{"viewerMarkdown", "Viewer markdown", "Render assistant text or all messages as Markdown", []string{"assistant", "all", "off"}},
			{"fallbackSubagent", "Fallback agent", "Agent used when a type is unknown, disabled, or ambiguous", nil},
		}
		var items []tui.SettingItem
		for _, spec := range specs {
			value := fmt.Sprint(raw[spec.id])
			values := spec.values
			if v, ok := raw[spec.id].(bool); ok {
				value = map[bool]string{true: "on", false: "off"}[v]
				values = []string{"on", "off"}
			}
			if spec.id == "packageAgents" || spec.id == "packageWorkflows" {
				value = "on"
				if b, ok := raw[spec.id].(bool); ok && !b {
					value = "off"
				}
				values = []string{"on", "off"}
			}
			if spec.id == "fallbackSubagent" {
				for _, c := range manager.Configs() {
					if c.Enabled == nil || *c.Enabled {
						values = append(values, c.Name)
					}
				}
				values = append(values, "none")
			}
			if len(values) == 0 {
				values = []string{value}
			}
			items = append(items, tui.SettingItem{ID: spec.id, Label: spec.label, Description: spec.description, CurrentValue: value, Values: values})
		}
		done := false
		id, newValue := "", ""
		list := tui.NewSettingsList(items, 12, tui.GetSettingsListTheme(), func(key, value string) { id, newValue = key, value; done = true }, func() { done = true })
		if selectedID != "" {
			list.SelectItem(selectedID)
		}
		component := tui.NewContainer(tui.NewPaddedText("Subagent settings", 0, 0, nil), tui.NewSpacer(1), list)
		m.runEditorSlotComponent(component, list.HandleInput, func() bool { return done })
		if id == "" {
			return nil
		}
		selectedID = id
		var next any = newValue
		if old, ok := raw[id].(float64); ok {
			answer, accepted := m.askEditor(ctx, id+" (0 = unlimited where supported)", strconv.Itoa(int(old)))
			if !accepted {
				continue
			}
			n, err := strconv.Atoi(strings.TrimSpace(answer))
			if err != nil {
				m.showWarning("Enter an integer")
				continue
			}
			next = n
		} else if _, ok := raw[id].(bool); ok {
			next = newValue == "on"
		} else if id == "packageAgents" || id == "packageWorkflows" {
			next = newValue == "on"
		}
		if err := manager.Update(map[string]any{id: next}); err != nil {
			m.showWarning(err.Error())
		}
	}
}

type subagentViewer struct {
	focusSession    bool
	anchorMessage   int
	seekAnchor      bool
	lastLeftTap     time.Time
	m               *InteractiveMode
	manager         *subagents.Manager
	id              string
	workflow        bool
	handle          *tui.OverlayHandle
	scroll          int
	follow          bool
	expanded        bool
	markdown        string
	confirmStop     bool
	composer        *tui.Editor
	messageCount    int
	newMessages     int
	maxScroll       int
	viewport        int
	bodyHeight      int
	footerRow       int
	dividerRow      int
	width           int
	dragging        bool
	cards           map[string]*tui.ToolExecutionComponent
	cardResults     map[string]string
	selectedAgent   int
	showWorkflowLog bool
	previous        *subagentViewer
}

func (v *subagentViewer) Invalidate()                 {}
func (v *subagentViewer) SetFocused(bool)             {}
func (v *subagentViewer) CapturesViewportInput() bool { return true }
func (v *subagentViewer) close() {
	if v.handle != nil {
		v.handle.Hide()
	}
	if v.m.subagentUI != nil {
		v.m.subagentUI.viewer = v.previous
	}
	v.m.requestRender()
}
func (m *InteractiveMode) openSubagentViewer(id string, workflow bool, focus ...bool) {
	manager := m.subagentManager()
	if manager == nil || m.tuiInst == nil {
		return
	}
	v := &subagentViewer{m: m, manager: manager, id: id, workflow: workflow, follow: true, markdown: manager.Settings().ViewerMarkdown}
	if len(focus) > 0 && focus[0] {
		v.focusSession = true
		v.composer = tui.NewEditor()
	}
	if m.subagentUI != nil {
		v.previous = m.subagentUI.viewer
	}
	options := tui.OverlaySpec{Width: &tui.OverlayValue{Value: 90, Percent: true}, MaxHeight: &tui.OverlayValue{Value: 90, Percent: true}, Anchor: "center"}.Options()
	if m.altScreen != nil || v.focusSession {
		options = tui.OverlaySpec{Width: &tui.OverlayValue{Value: 100, Percent: true}, MaxHeight: &tui.OverlayValue{Value: 100, Percent: true}, Margin: tui.OverlayMarginAll(0)}.Options()
		options.Fullscreen = m.altScreen != nil
	}
	v.handle = m.tuiInst.ShowOverlay(v, options)
	if m.subagentUI != nil {
		m.subagentUI.viewer = v
	}
	m.requestRender()
}
func (v *subagentViewer) Render(width int) []string {
	if v.workflow {
		return v.renderWorkflowInspector(width)
	}
	height := max(8, v.m.tuiInst.Height()-2)
	theme := tui.ActiveTheme()
	var body []string
	anchorOffset := -1
	title := "Agent " + v.id

	record, ok := v.manager.Observe(v.id)
	if ok {
		title = subagentName(record) + "  " + record.Description + " · " + record.Status + " · " + subagentStats(record, v.manager.Settings())
	}
	messages, err := v.manager.Transcript(v.id)
	if err != nil {
		body = append(body, err.Error())
	}
	if v.messageCount > 0 && len(messages) > v.messageCount && !v.follow {
		v.newMessages += len(messages) - v.messageCount
	}
	v.messageCount = len(messages)
	toolResults := map[string]agent.AgentMessage{}
	for _, raw := range messages {
		var msg agent.AgentMessage
		if json.Unmarshal(raw, &msg) == nil && msg.ToolResult != nil {
			toolResults[msg.ToolResult.ToolCallID] = msg
		}
	}
	for messageIndex, raw := range messages {
		if v.seekAnchor && messageIndex == v.anchorMessage {
			anchorOffset = len(body)
		}
		var msg agent.AgentMessage
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		switch msg.Role() {
		case "user":
			if msg.User != nil {
				text := extractAgentMessageText(msg)
				body = append(body, "", theme.Fg("accent", "❯ "+text))
			}
		case "assistant":
			if msg.Assistant != nil {
				for _, block := range msg.Assistant.Content {
					switch b := block.(type) {
					case ai.TextContent:
						if v.markdown != "off" {
							md := tui.NewMarkdown(b.Text)
							md.FinishStreaming()
							body = append(body, md.Render(max(1, width-2))...)
						} else {
							body = append(body, widthx.WrapTextWithAnsi(b.Text, max(1, width-2))...)
						}
					case ai.ToolCall:
						args, _ := json.Marshal(b.Arguments)
						if v.cards == nil {
							v.cards = map[string]*tui.ToolExecutionComponent{}
							v.cardResults = map[string]string{}
						}
						card := v.cards[b.ID]
						if card == nil {
							card = v.m.newToolCard(b.Name, b.ID, args)
							v.cards[b.ID] = card
						}
						card.SetExpanded(v.expanded)
						if resultMsg, ok := toolResults[b.ID]; ok {
							encoded, _ := json.Marshal(resultMsg.ToolResult)
							if v.cardResults[b.ID] != string(encoded) {
								r := resultMsg.ToolResult
								result := agent.AgentToolResult{Content: r.Content, Details: r.Details, IsError: r.IsError}
								var content []tui.ToolResultContent
								for _, c := range r.Content {
									if t, ok := c.(ai.TextContent); ok {
										content = append(content, tui.ToolResultContent{Type: "text", Text: t.Text})
									}
								}
								card.UpdateResult(tui.ToolResultUpdate{Content: content, Details: r.Details, IsError: r.IsError, Result: result})
								v.cardResults[b.ID] = string(encoded)
							}
						}
						body = append(body, card.Render(max(1, width-2))...)
					}
				}
			}
		case "toolResult":
			if msg.ToolResult != nil && v.cards[msg.ToolResult.ToolCallID] == nil {
				var parts []string
				for _, c := range msg.ToolResult.Content {
					if text, ok := c.(ai.TextContent); ok {
						parts = append(parts, text.Text)
					}
				}
				text := strings.Join(parts, "\n")
				lines := strings.Split(text, "\n")
				if !v.expanded && len(lines) > 10 {
					lines = append(lines[:10], fmt.Sprintf("… (%d more lines, ctrl+o to expand)", len(lines)-10))
				}
				if v.markdown == "all" {
					md := tui.NewMarkdown(strings.Join(lines, "\n"))
					md.FinishStreaming()
					body = append(body, md.Render(max(1, width-2))...)
				} else {
					body = append(body, theme.Fg("dim", strings.Join(lines, "\n")))
				}
			}
		}
	}
	var wrapped []string
	for index, line := range body {
		if v.seekAnchor && index == anchorOffset {
			v.scroll = len(wrapped)
			v.seekAnchor = false
			v.follow = false
		}
		wrapped = append(wrapped, widthx.WrapTextWithAnsi(line, max(1, width-2))...)
	}
	body = wrapped
	composerRows := []string{}
	if v.composer != nil {
		label := "Steer agent:"
		if v.focusSession {
			label = "Message agent · Enter sends · Esc on empty returns to main"
		}
		composerRows = append([]string{theme.Fg("accent", label)}, v.composer.Render(max(1, width-2))...)
	}
	viewport := max(1, height-4-len(composerRows))
	maxScroll := max(0, len(body)-viewport)
	v.maxScroll = maxScroll
	v.viewport = viewport
	v.bodyHeight = len(body)
	v.width = width
	if v.follow {
		v.scroll = maxScroll
		v.newMessages = 0
	}
	v.scroll = min(max(0, v.scroll), maxScroll)
	if v.scroll == maxScroll {
		v.follow = true
		v.newMessages = 0
	}
	lines := []string{clampAgentLine(title, width), theme.Fg("borderMuted", strings.Repeat("─", max(1, width)))}
	for i := 0; i < viewport; i++ {
		line := ""
		if idx := v.scroll + i; idx < len(body) {
			line = body[idx]
		}
		if maxScroll > 0 && v.m.altScreen != nil && (v.m.opts.SettingsManager == nil || v.m.opts.SettingsManager.GetFullscreenScrollbar() != "hidden") {
			thumbHeight := max(1, viewport*viewport/max(1, len(body)))
			thumbStart := v.scroll * (viewport - thumbHeight) / max(1, maxScroll)
			rail := theme.Fg("scrollbarTrack", "│")
			if i >= thumbStart && i < thumbStart+thumbHeight {
				rail = theme.Fg("scrollbarThumb", "┃")
			}
			line = widthx.TruncateToWidth(line, max(1, width-1), "…", true) + rail
		}
		lines = append(lines, clampAgentLine(line, width))
	}
	divider := strings.Repeat("─", max(1, width))
	if !v.follow {
		if v.newMessages > 0 {
			divider = fmt.Sprintf("[ ↓ %d new messages · Ctrl+End ]", v.newMessages)
		} else {
			divider = "[ ↓ Jump to latest message · Ctrl+End ]"
		}
	}
	v.dividerRow = len(lines)
	lines = append(lines, theme.Fg("borderMuted", clampAgentLine(divider, width)))
	lines = append(lines, composerRows...)
	footer := "enter steer · x stop · m markdown · ctrl+o expand · esc close"
	if v.focusSession {
		footer = "enter:send · alt+a:Agent Hub · page up/down:scroll · esc:main"
	}
	if v.workflow {
		footer = "x stop · ↑/↓ scroll · esc close"
	}
	if v.confirmStop {
		footer = "Press x again to stop · esc cancel"
	}
	v.footerRow = len(lines)
	lines = append(lines, theme.Fg("dim", clampAgentLine(footer, width)))
	return lines
}
func (v *subagentViewer) HandleInput(data string) {
	if v.focusSession {
		if tui.MatchesKeyID(data, "alt+a") || tui.MatchesKeyID(data, "ctrl+s") {
			v.close()
			v.m.toggleAgentHub()
			return
		}
		if tui.MatchesKeyID(data, "ctrl+o") {
			v.expanded = !v.expanded
			v.m.requestRender()
			return
		}
		if tui.MatchesKeyID(data, "pageUp") || tui.MatchesKeyID(data, "pageDown") {
			direction := 1
			if tui.MatchesKeyID(data, "pageUp") {
				direction = -1
			}
			v.follow = false
			v.scroll = max(0, v.scroll+direction*10)
			v.m.requestRender()
			return
		}
		if v.composer == nil {
			v.composer = tui.NewEditor()
		}
		if v.composer.Text() == "" && tui.MatchesKeyID(data, "escape") {
			v.close()
			return
		}
		if v.composer.Text() == "" && tui.MatchesKeyID(data, "left") {
			now := time.Now()
			if !v.lastLeftTap.IsZero() && now.Sub(v.lastLeftTap) < 500*time.Millisecond {
				v.close()
				return
			}
			v.lastLeftTap = now
			return
		}
		v.lastLeftTap = time.Time{}
	}
	if v.composer != nil {
		if tui.MatchesKeyID(data, "escape") {
			if v.focusSession {
				v.composer.SetText("")
			} else {
				v.composer = nil
			}
		} else if tui.MatchesKeyID(data, "enter") {
			message := strings.TrimSpace(v.composer.Text())
			if message != "" {
				var err error
				if v.focusSession {
					err = v.manager.PromptObserved(v.id, message)
				} else {
					err = v.manager.SteerObserved(v.id, message)
				}
				if err != nil {
					v.m.showWarning(err.Error())
					return
				}
				v.follow = true
			}
			if v.focusSession {
				v.composer.SetText("")
			} else {
				v.composer = nil
			}
		} else {
			v.composer.HandleInput(data)
		}
		v.m.requestRender()
		return
	}

	if v.workflow && v.handleWorkflowInspector(data) {
		return
	}
	switch {
	case tui.MatchesKeyID(data, "escape"):
		if v.confirmStop {
			v.confirmStop = false
		} else {
			v.close()
		}
	case tui.MatchesKeyID(data, "up"):
		v.follow = false
		v.scroll = max(0, v.scroll-1)
	case tui.MatchesKeyID(data, "pageUp"):
		v.follow = false
		v.scroll = max(0, v.scroll-10)
	case tui.MatchesKeyID(data, "down"):
		v.follow = false
		v.scroll++
	case tui.MatchesKeyID(data, "pageDown"):
		v.follow = false
		v.scroll += 10
	case tui.MatchesKeyID(data, "end"), tui.MatchesKeyID(data, "ctrl+end"):
		v.follow = true
	case tui.MatchesKeyID(data, "home"):
		v.follow = false
		v.scroll = 0
	case data == "x":
		if v.confirmStop {
			if v.workflow {
				_ = v.manager.StopWorkflow(v.id)
			} else {
				_ = v.manager.StopObserved(v.id)
			}
			v.confirmStop = false
		} else {
			v.confirmStop = true
		}
	case data == "b":
		v.manager.Background(v.id)
	case data == "m":
		switch v.markdown {
		case "off":
			v.markdown = "assistant"
		case "assistant":
			v.markdown = "all"
		default:
			v.markdown = "off"
		}
	case tui.MatchesKeyID(data, "ctrl+o"):
		v.expanded = !v.expanded
	case tui.MatchesKeyID(data, "enter"):
		if r, ok := v.manager.Observe(v.id); ok && r.Active() {
			v.composer = tui.NewEditor()
		}
	}
	v.m.requestRender()
}
func (v *subagentViewer) HandleMouse(event tui.TuiMouseEvent) *tui.TuiMouseDispatchResult {
	capture := false
	if event.Type == tui.MouseWheel {
		v.follow = false
		v.scroll = max(0, v.scroll+event.WheelDelta*3)
	}
	if event.Type == tui.MouseRelease {
		v.dragging = false
	}
	if event.X >= v.width-1 && event.Y >= 2 && event.Y < 2+v.viewport && v.maxScroll > 0 && (event.Type == tui.MousePress || event.Type == tui.MouseClick || event.Type == tui.MouseDrag || v.dragging) {
		v.follow = false
		v.dragging = event.Type != tui.MouseRelease
		capture = v.dragging
		v.scroll = min(v.maxScroll, max(0, (event.Y-2)*v.maxScroll/max(1, v.viewport-1)))
	}
	if event.Type == tui.MouseClick && event.Y == v.dividerRow {
		v.follow = true
		v.newMessages = 0
	}
	if event.Type == tui.MouseClick && event.Y == v.footerRow {
		switch {
		case event.X < 12:
			v.HandleInput("\r")
		case event.X < 21:
			v.HandleInput("x")
		case event.X < 34:
			v.HandleInput("m")
		case event.X < 51:
			v.HandleInput("\x0f")
		default:
			v.HandleInput("\x1b")
		}
	}
	v.m.requestRender()
	render := true
	return &tui.TuiMouseDispatchResult{TuiMouseEventResult: tui.TuiMouseEventResult{Handled: true, Capture: capture, Render: &render}}
}

func (m *InteractiveMode) handleAgentMention(prompt string) bool {
	manager := m.subagentManager()
	if manager == nil || !manager.Settings().Enabled || manager.Settings().AgentMentions == "off" || !strings.HasPrefix(prompt, "@") {
		return false
	}
	parts := strings.SplitN(prompt, " ", 2)
	if len(parts) != 2 {
		return false
	}
	handle := strings.TrimPrefix(parts[0], "@")
	if strings.EqualFold(handle, "main") {
		return false
	}
	if _, ok := manager.Find(handle); ok {
		err := manager.Mention(handle, parts[1])
		if err != nil {
			m.showWarning(err.Error())
		} else {
			m.showStatus("Sent to @" + handle)
		}
		return true
	}
	for _, c := range manager.Configs() {
		if strings.EqualFold(c.Name, handle) {
			err := manager.Mention(handle, parts[1])
			if err != nil {
				m.showWarning(err.Error())
			}
			return true
		}
	}
	return false
}

// WithSubagentRenderers keeps delegation calls to one descriptive line. The
// agent tree and Hub own progress, results, and session navigation.
func WithSubagentRenderers(def extension.ToolDefinition) extension.ToolDefinition {
	if def.Name == "Agent" {
		def.RenderShell = extension.ToolRenderShellDefault
		def.RenderCall = subagentRenderCall
		def.RenderResult = subagentRenderResult
	}
	return def
}
func subagentRenderCall(args json.RawMessage, _ extension.Theme, _ extension.ToolRenderContext) extension.Component {
	var p subagents.Params
	_ = json.Unmarshal(args, &p)
	text := tui.ChalkBold("Agent")
	if description := hubSingleLine(p.Description); description != "" {
		text += "  " + tui.ActiveTheme().Fg("muted", description)
	}
	return linesComponent{render: func(width int) []string {
		if width <= 0 {
			return nil
		}
		return []string{clampAgentLine(text, width)}
	}}
}
func subagentRenderResult(_ extension.AgentToolResult, _ extension.ToolRenderResultOptions, _ extension.Theme, _ extension.ToolRenderContext) extension.Component {
	// A non-nil, empty component suppresses the generic result fallback.
	return linesComponent{render: func(int) []string { return nil }}
}
