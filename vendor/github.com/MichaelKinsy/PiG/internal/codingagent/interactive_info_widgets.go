package codingagent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// ensureInfoWidgets builds the shared host once. It survives layout and TUI mode
// switches; data and disclosure state belong to the owner loop, not a renderer.
// The first nonempty candidate wins, so a todo list/error takes priority over quota.
func (m *InteractiveMode) ensureInfoWidgets() *tui.InfoWidgets {
	if m.infoWidgets == nil {
		m.infoWidgets = &tui.InfoWidgets{Sections: []tui.InfoWidget{
			{Key: "todo", Title: "Tasks", Render: m.renderTodoWidget},
			{Key: "quota", Title: "Quota", Render: m.renderQuotaWidget},
		}}
	}
	return m.infoWidgets
}

// bindInfoWidgetControl puts the persistent source point beside the existing
// quota label. The editor owns its hit box, so hiding the panel leaves no mouse
// target over responses. Called whenever an editor/layout is constructed.
func (m *InteractiveMode) bindInfoWidgetControl() {
	if m.editor == nil {
		return
	}
	m.editor.TopRightControl = m.ensureInfoWidgets().ControlLabel
	m.editor.OnTopRightControl = m.toggleInfoWidgets
	m.editor.Invalidate()
}

// toggleInfoWidgets shares the prompt-border and keyboard disclosure behavior.
// Invalidate the editor immediately so its cached border and hit box agree.
func (m *InteractiveMode) toggleInfoWidgets() {
	m.ensureInfoWidgets().Toggle()
	if m.editor != nil {
		m.editor.Invalidate()
	}
	m.requestRender()
}

// updateTodoWidget accepts only authoritative mutations. Read-only views and
// failures cannot replace the plan; an error is visible beside the last good
// list. Reusing the registered tool renderer keeps task styling/windowing in
// one place without importing the builtin package (which imports this package).
func (m *InteractiveMode) updateTodoWidget(result agent.AgentToolResult) {
	if result.IsError {
		m.todoWidgetError = result.Text()
		return
	}
	var details struct {
		Op     string             `json:"op"`
		Phases *[]json.RawMessage `json:"phases"`
	}
	raw, err := json.Marshal(result.Details)
	if err != nil || json.Unmarshal(raw, &details) != nil || details.Phases == nil {
		return
	}
	m.todoWidgetError = ""
	if details.Op == "view" {
		return
	}
	if len(*details.Phases) == 0 {
		m.todoWidget = nil
		return
	}
	args := json.RawMessage(`{"op":"view"}`)
	// Only the widget uses the full registered todo renderer. Do not register
	// this private card for transcript rebinding, which uses the compact receipt.
	card := tui.NewToolExecutionComponent("todo", "todo-widget", args, m.toolExecutionOptions(), nil, m.tuiInst, m.opts.CWD)
	if renderers := m.resolveToolRenderers("todo"); renderers != nil {
		card.SetDefinition(m.toolDefinitionRenderers(*renderers, func() *tui.ToolExecutionComponent { return card }, "todo-widget"), args)
	}
	card.SetArgsComplete()
	card.UpdateResult(ToolExecutionResultOf(result, 0), false)
	m.todoWidget = card
}

// restoreTodoWidget resets branch-local presentation and reads only tool-result
// records from the entire live branch. The transcript may load a compacted
// context subset; using that subset would lose a plan from before compaction.
// Supplied entries are the fallback for surfaces with no native Session.
func (m *InteractiveMode) restoreTodoWidget(entries []SessionEntry) {
	m.todoWidget, m.todoWidgetError = nil, ""
	if session := m.currentSession(); session != nil {
		if leaf := session.GetLeafID(); leaf != nil {
			entries = session.branchMessagesWithRole(*leaf, "toolResult")
		} else {
			entries = nil
		}
	}
	for _, entry := range entries {
		if message, ok := asMessage(entry); ok {
			if result := message.Message.ToolResult; result != nil && result.ToolName == "todo" {
				m.updateTodoWidget(result.Result())
			}
		}
	}
}

// renderTodoWidget uses the existing renderer's task styling and windowing;
// the host keeps only its summary row when the section is collapsed.
func (m *InteractiveMode) renderTodoWidget(width int, expanded bool) []string {
	var rows []string
	if m.todoWidget != nil {
		m.todoWidget.SetExpanded(expanded)
		rows = m.todoWidget.Render(width)
		// Tool cards may add outside spacing; the widget supplies its own
		// frame and needs the first row to be the collapsed summary.
		for len(rows) > 0 && strings.TrimSpace(widthx.StripAnsi(rows[0])) == "" {
			rows = rows[1:]
		}
		for len(rows) > 0 && strings.TrimSpace(widthx.StripAnsi(rows[len(rows)-1])) == "" {
			rows = rows[:len(rows)-1]
		}
	}
	if m.todoWidgetError != "" {
		rows = append(rows, tui.NewPaddedText(tui.ActiveTheme().Fg("error", m.todoWidgetError), 0, 0, nil).Render(width)...)
	}
	return rows
}

// renderQuotaWidget reads the existing minute-refreshed, tier-aware quota cache.
// It never reads credentials or probes providers from the paint path. Missing
// or failed reports remain absent instead of inventing quota for API-key models.
func (m *InteractiveMode) renderQuotaWidget(width int, expanded bool) []string {
	if m.opts.Model == nil || m.promptQuota == nil || !m.promptQuota.loaded || m.promptQuota.failed {
		return nil
	}
	provider := m.opts.Model.ProviderID()
	snapshot := m.promptQuota.snapshot
	snapshot.Reports = m.promptQuotaTierReports(snapshot.Reports, provider)
	windows := promptQuotaWindows(snapshot, provider, m.opts.Model.ID)
	if len(windows) == 0 {
		return nil
	}
	if !expanded {
		return []string{fmt.Sprintf("%s · %.0f%% left", windows[0].label, (1-windows[0].used)*100)}
	}
	rows := []string{provider}
	for _, window := range windows {
		status := ai.UsageStatusOK
		if window.used >= 1 {
			status = ai.UsageStatusExhausted
		} else if window.used >= .8 {
			status = ai.UsageStatusWarning
		}
		rows = append(rows, fmt.Sprintf("%s · %.0f%% left", window.label, (1-window.used)*100))
		rows = append(rows, renderPromptQuotaBar(window.used, status, min(20, width)))
		if window.resetsAt > 0 {
			remaining := max(time.Minute, time.Until(time.UnixMilli(window.resetsAt))).Round(time.Minute)
			rows = append(rows, tui.ActiveTheme().Fg("dim", "Resets in "+remaining.String()))
		}
	}
	for _, account := range snapshot.UnavailableAccounts {
		if account.Provider == provider {
			rows = append(rows, "Partial quota · /usage for details")
			break
		}
	}
	// Flatten provider/window labels before they enter fixed-cell frames.
	for i, row := range rows {
		rows[i] = strings.ReplaceAll(strings.ReplaceAll(row, "\n", " "), "\r", " ")
	}
	return rows
}
