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
			{Key: "todo", Title: "Tasks", Render: m.renderTodoWidget, FocusRow: m.todoWidgetFocusRow},
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

// restoreTodoWidget resets branch-local presentation and reads only todo-result
// records from the entire live branch. The transcript may load a compacted
// context subset; using that subset would lose a plan from before compaction.
// Supplied entries are the fallback for surfaces with no native Session.
func (m *InteractiveMode) restoreTodoWidget(entries []SessionEntry) {
	m.todoWidget, m.todoWidgetError = nil, ""
	if session := m.currentSession(); session != nil {
		if leaf := session.GetLeafID(); leaf != nil {
			entries = session.branchMessagesWithRole(*leaf, "toolResult", "todo")
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
//
// A failed call contributes exactly one row, placed directly under the plan's
// summary and above its tasks. The panel is a capped overlay beside the prompt,
// and a schema rejection's full text is a header, a bullet and a pretty-printed
// copy of the model's own arguments: condensing it keeps the panel's row count
// stable and still says what to fix, and the position keeps both facts — which
// plan, and why the last call did not change it — in view above a long task list.
// With no plan yet the reason is simply the first row. The untouched result stays
// in the transcript and the session log.
func (m *InteractiveMode) renderTodoWidget(width int, expanded bool) []string {
	var rows, reason []string
	if m.todoWidgetError != "" {
		// Errors own the error token; nothing else in the panel uses it, so a
		// failed call stays distinguishable from a warning-styled caveat.
		// Truncation happens on the plain text: cutting styled text can split
		// an escape sequence.
		line := widthx.TruncateToWidth(todoErrorSummary(m.todoWidgetError), max(1, width), "…", false)
		reason = []string{tui.ActiveTheme().Fg(string(tui.ThemeColorError), line)}
	}
	if m.todoWidget != nil {
		m.todoWidget.SetExpanded(expanded)
		body := m.todoWidget.Render(width)
		// Tool cards may add outside spacing; the widget supplies its own
		// frame and needs the first row to be the collapsed summary.
		for len(body) > 0 && strings.TrimSpace(widthx.StripAnsi(body[0])) == "" {
			body = body[1:]
		}
		for len(body) > 0 && strings.TrimSpace(widthx.StripAnsi(body[len(body)-1])) == "" {
			body = body[:len(body)-1]
		}
		if len(body) > 0 {
			rows = append(rows, body[0])
			body = body[1:]
		}
		rows = append(rows, reason...)
		rows = append(rows, body...)
		return rows
	}
	return append(rows, reason...)
}

// todoWidgetFocusRow delegates task selection and wrapped-row measurement to
// the canonical renderer. A rejected call takes priority so its reason is visible.
func (m *InteractiveMode) todoWidgetFocusRow(width int) int {
	if m.todoWidgetError != "" {
		return 0
	}
	if m.todoWidget != nil {
		return m.todoWidget.ResultFocusRow(width)
	}
	return -1
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
		// One summary row: window label muted, pressure colored, as the usage
		// dashboard reads. The percentage stays in a single span so its text is
		// still contiguous for callers that match on it.
		return []string{fmt.Sprintf("%s · %s", tui.ActiveTheme().Fg(string(tui.ThemeColorMuted), windows[0].label),
			quotaPercentFg(windows[0].used, (1-windows[0].used)*100))}
	}
	theme := tui.ActiveTheme()
	rows := []string{theme.Fg(string(tui.ThemeColorAccent), provider)}
	for _, window := range windows {
		status := ai.UsageStatusOK
		if window.used >= 1 {
			status = ai.UsageStatusExhausted
		} else if window.used >= .8 {
			status = ai.UsageStatusWarning
		}
		rows = append(rows, theme.Fg(string(tui.ThemeColorMuted), window.label)+" · "+quotaPercentFg(window.used, (1-window.used)*100))
		rows = append(rows, renderPromptQuotaBar(window.used, status, min(20, width)))
		if window.resetsAt > 0 {
			remaining := max(time.Minute, time.Until(time.UnixMilli(window.resetsAt))).Round(time.Minute)
			rows = append(rows, theme.Fg(string(tui.ThemeColorDim), "Resets in "+remaining.String()))
		}
	}
	for _, account := range snapshot.UnavailableAccounts {
		if account.Provider == provider {
			// Partial data is a caveat, not a failure: warning, not error.
			rows = append(rows, theme.Fg(string(tui.ThemeColorWarning), "Partial quota · /usage for details"))
			break
		}
	}
	// Flatten provider/window labels before they enter fixed-cell frames.
	for i, row := range rows {
		rows[i] = strings.ReplaceAll(strings.ReplaceAll(row, "\n", " "), "\r", " ")
	}
	return rows
}

// quotaPercentFg colors a remaining percentage by pressure, using the same
// thresholds and tokens as the quota bar and the usage dashboard: exhausted is
// an error, 80% used a warning, and a healthy window follows its bar's success
// color. The text keeps `% left` so it matches the border label's wording.
func quotaPercentFg(used, remaining float64) string {
	theme := tui.ActiveTheme()
	token := tui.ThemeColorSuccess
	switch {
	case used >= 1:
		token = tui.ThemeColorError
	case used >= .8:
		token = tui.ThemeColorWarning
	}
	return theme.Fg(string(token), fmt.Sprintf("%.0f%% left", remaining))
}
