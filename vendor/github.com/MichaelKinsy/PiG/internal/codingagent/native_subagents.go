package codingagent

// Native presentation port of pi-subagents src/ui/{agent-widget,fleet-list,
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
	manager   *subagents.Manager
	focused   bool
	selection int
	viewer    *subagentViewer
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
	turns := fmt.Sprintf("↻%d", r.Turns)
	if r.MaxTurns > 0 {
		turns += fmt.Sprintf("≤%d", r.MaxTurns)
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

type nativeAgentWidget struct {
	m     *InteractiveMode
	below bool
}

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
	if w.below {
		return w.m.renderAgentFleet(width)
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

type fleetEntry struct {
	id, label, description, stats string
	workflow                      bool
}

func (m *InteractiveMode) agentFleet() []fleetEntry {
	manager := m.subagentManager()
	if manager == nil || !manager.Settings().Enabled || !manager.Settings().FleetView {
		return nil
	}
	var rows []fleetEntry
	for _, w := range manager.Workflows() {
		if w.Active() || time.Now().UnixMilli()-w.CompletedAt < 4000 {
			rows = append(rows, fleetEntry{id: w.ID, label: "workflow", description: w.Meta.Name, stats: fmt.Sprintf("%d/%d agents · %ds · ↓ %s tokens", w.DoneCount, w.TotalCount, (time.Now().UnixMilli()-w.StartedAt)/1000, subagentCompact(w.Tokens)), workflow: true})
		}
	}
	for _, r := range manager.Records() {
		if r.TopLevel() && r.SessionFile != "" && (r.Active() || time.Now().UnixMilli()-r.CompletedAt < 4000) {
			rows = append(rows, fleetEntry{id: r.ID, label: r.Type, description: r.Description, stats: subagentElapsed(r) + " · ↓ " + subagentCompact(r.Tokens) + " tokens"})
		}
	}
	if len(rows) > 0 {
		rows = append([]fleetEntry{{label: "main"}}, rows...)
	}
	return rows
}
func (m *InteractiveMode) renderAgentFleet(width int) []string {
	rows := m.agentFleet()
	if len(rows) == 0 {
		return nil
	}
	state := m.subagentUI
	if state == nil {
		return nil
	}
	state.selection = min(state.selection, len(rows)-1)
	lines := []string{tui.ActiveTheme().Fg("dim", "  esc to interrupt · ← for agents · ↓ to manage"), ""}
	start := max(0, state.selection-5)
	end := min(len(rows), start+6)
	for i := start; i < end; i++ {
		r := rows[i]
		icon := "○"
		if i == state.selection {
			icon = "●"
		}
		left := "  " + icon + " " + r.label
		if r.description != "" {
			left += "  " + r.description
		}
		right := r.stats
		available := max(0, width-widthx.VisibleWidth(right)-1)
		line := clampAgentLine(left, available)
		line += strings.Repeat(" ", max(1, width-widthx.VisibleWidth(line)-widthx.VisibleWidth(right))) + right
		if i != state.selection {
			line = tui.ActiveTheme().Fg("dim", line)
		}
		lines = append(lines, clampAgentLine(line, width))
	}
	if end < len(rows) {
		lines = append(lines, tui.ActiveTheme().Fg("dim", fmt.Sprintf("  ↓ %d more", len(rows)-end)))
	}
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
	if m.editor == nil || m.editor.Text() != "" {
		return false
	}
	rows := m.agentFleet()
	if len(rows) == 0 {
		return false
	}
	state := m.subagentUI
	if state == nil {
		return false
	}
	if !state.focused {
		if tui.MatchesKeyID(data, "down") || tui.MatchesKeyID(data, "left") {
			state.focused = true
			state.selection = 0
			m.requestRender()
			return true
		}
		return false
	}
	switch {
	case tui.MatchesKeyID(data, "escape"):
		state.focused = false
	case tui.MatchesKeyID(data, "up"):
		if state.selection == 0 {
			state.focused = false
		} else {
			state.selection--
		}
	case tui.MatchesKeyID(data, "down"):
		state.selection = min(state.selection+1, len(rows)-1)
	case tui.MatchesKeyID(data, "enter"):
		if state.selection == 0 {
			state.focused = false
		} else {
			r := rows[state.selection]
			m.openSubagentViewer(r.id, r.workflow)
		}
	default:
		state.focused = false
		return false
	}
	m.requestRender()
	return true
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
	case "status":
		s := manager.Settings()
		m.showStatus(fmt.Sprintf("Subagents enabled=%t · %d background slots · %d agents", s.Enabled, s.MaxConcurrent, len(manager.Records())))
		return nil
	case "menu", "settings":
	default:
		return fmt.Errorf("usage: /subagents [on|off|status|menu|settings]")
	}
	for {
		options := []string{"Running agents", "Completed agents", "Agent types", "Workflows", "Scheduled jobs", "Settings"}
		choice, ok := m.askSelector(ctx, "Agents", options)
		if args == "settings" {
			choice = "Settings"
			ok = true
		}
		if !ok {
			return nil
		}
		switch choice {
		case "Running agents", "Completed agents":
			var records []subagents.Record
			var labels []string
			for _, r := range manager.Records() {
				if r.TopLevel() && r.Active() == (choice == "Running agents") {
					records = append(records, r)
					labels = append(labels, fmt.Sprintf("%s  @%s · %s · %s", r.DisplayName, r.Handle, r.Description, r.Status))
				}
			}
			if len(labels) == 0 {
				m.showStatus("No " + strings.ToLower(choice))
				continue
			}
			picked, ok := m.askSelector(ctx, choice, labels)
			if ok {
				for i, label := range labels {
					if picked == label {
						m.openSubagentViewer(records[i].ID, false)
						return nil
					}
				}
			}
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
			{"defaultJoinMode", "Join mode", "smart = completions at the parent's next boundary; group = one notification per batch", []string{"smart", "group"}},
			{"backgroundByDefault", "Background by default", "An Agent call that doesn't say runs detached", nil},
			{"schedulingEnabled", "Scheduling", "Schedule agents using cron, intervals, or one-shot timestamps", nil},
			{"workflowsEnabled", "Workflows", "Deterministic scripted workflow orchestration", nil},
			{"packageAgents", "Package agents", "Load agents declared by installed packages; local agents override them", nil},
			{"packageWorkflows", "Package workflows", "Load workflows declared by installed packages", nil},
			{"scopeModels", "Scope models", "Require caller-selected models to match enabledModels", nil},
			{"strictAgentFiles", "Strict agent files", "Refuse activation when a local agent file is invalid", nil},
			{"disableDefaultAgents", "Disable default agents", "Hide embedded agents; your custom files remain available", nil},
			{"fleetView", "Fleet view", "Navigable main and agent list below the editor", nil},
			{"agentMentions", "Agent mentions", "model = clone orchestration; direct = start immediately; off = file mentions only", []string{"model", "direct", "off"}},
			{"rememberAgents", "Remember agents", "Keep completed agents addressable across session reloads", nil},
			{"widgetMode", "Widget", "Which agents appear above the editor", []string{"background", "all", "off"}},
			{"toolDescriptionMode", "Tool description", "Full, compact, or project-authored tool descriptions", []string{"full", "compact", "custom"}},
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
func (m *InteractiveMode) openSubagentViewer(id string, workflow bool) {
	manager := m.subagentManager()
	if manager == nil || m.tuiInst == nil {
		return
	}
	v := &subagentViewer{m: m, manager: manager, id: id, workflow: workflow, follow: true, markdown: manager.Settings().ViewerMarkdown}
	if m.subagentUI != nil {
		v.previous = m.subagentUI.viewer
	}
	options := tui.OverlaySpec{Width: &tui.OverlayValue{Value: 90, Percent: true}, MaxHeight: &tui.OverlayValue{Value: 90, Percent: true}, Anchor: "center"}.Options()
	if m.altScreen != nil {
		options = tui.OverlaySpec{Width: &tui.OverlayValue{Value: 100, Percent: true}, MaxHeight: &tui.OverlayValue{Value: 100, Percent: true}, Margin: tui.OverlayMarginAll(0)}.Options()
		options.Fullscreen = true
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
	title := "Agent " + v.id
	if v.workflow {
		if w, ok := v.manager.Workflow(v.id); ok {
			title = w.Meta.Name + " · " + w.Status
			body = append(body, fmt.Sprintf("%d/%d agents · %s tokens", w.DoneCount, w.TotalCount, subagentCompact(w.Tokens)))
			for _, p := range w.Progress {
				var entry map[string]any
				_ = json.Unmarshal(p, &entry)
				switch entry["type"] {
				case "workflow_phase":
					body = append(body, "", theme.Fg("accent", fmt.Sprint(entry["title"])))
				case "workflow_log":
					body = append(body, fmt.Sprint(entry["message"]))
				case "workflow_agent":
					body = append(body, fmt.Sprintf("  %v · %v · %v", entry["label"], entry["id"], entry["status"]))
				}
			}
			if !w.Active() {
				body = append(body, string(w.Result), w.Error)
			}
		}
	} else {
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
		for _, raw := range messages {
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
	}
	var wrapped []string
	for _, line := range body {
		wrapped = append(wrapped, widthx.WrapTextWithAnsi(line, max(1, width-2))...)
	}
	body = wrapped
	composerRows := []string{}
	if v.composer != nil {
		composerRows = append([]string{theme.Fg("accent", "Steer agent:")}, v.composer.Render(max(1, width-2))...)
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
		if maxScroll > 0 && v.m.altScreen != nil && v.m.opts.SettingsManager.GetFullscreenScrollbar() != "hidden" {
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
	if v.composer != nil {
		if tui.MatchesKeyID(data, "escape") {
			v.composer = nil
		} else if tui.MatchesKeyID(data, "enter") {
			message := strings.TrimSpace(v.composer.Text())
			if message != "" {
				if err := v.manager.SteerObserved(v.id, message); err != nil {
					v.m.showWarning(err.Error())
				}
			}
			v.composer = nil
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

// WithSubagentRenderers installs the native Claude-style tool card.
func WithSubagentRenderers(def extension.ToolDefinition) extension.ToolDefinition {
	if def.Name == "Agent" {
		def.RenderCall = subagentRenderCall
		def.RenderResult = subagentRenderResult
	}
	return def
}
func subagentRenderCall(args json.RawMessage, _ extension.Theme, _ extension.ToolRenderContext) extension.Component {
	var p subagents.Params
	_ = json.Unmarshal(args, &p)
	name := p.Type
	if name == "general-purpose" || name == "" {
		name = "Agent"
	}
	return tui.NewPaddedText("▸ "+tui.ChalkBold(name)+"  "+tui.ActiveTheme().Fg("muted", p.Description), 0, 0, nil)
}
func subagentRenderResult(result extension.AgentToolResult, options extension.ToolRenderResultOptions, _ extension.Theme, _ extension.ToolRenderContext) extension.Component {
	b, _ := json.Marshal(result.Details)
	var r subagents.Record
	_ = json.Unmarshal(b, &r)
	var text string
	if r.ID != "" {
		text = subagentIcon(r) + " " + subagentStats(r, subagents.DefaultSettings())
		if r.Active() {
			text += "\n  ⎿  " + r.Activity
			if !r.Background && time.Now().UnixMilli()-r.StartedAt > 2000 {
				text += " (ctrl+b to run in background)"
			}
		} else if options.Expanded {
			text += "\n" + r.Result
		} else if r.Result != "" {
			lines := strings.Split(r.Result, "\n")
			text += "\n  ⎿  " + lines[0]
			if len(lines) > 1 {
				text += fmt.Sprintf("\n     … (%d more lines, ctrl+o to expand)", len(lines)-1)
			}
		}
	} else {
		for _, content := range result.Content {
			if t, ok := content.(ai.TextContent); ok {
				text += t.Text
			}
		}
	}
	return tui.NewPaddedText(text, 0, 0, nil)
}
