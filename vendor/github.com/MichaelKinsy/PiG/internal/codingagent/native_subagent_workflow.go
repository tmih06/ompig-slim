package codingagent

import (
	"fmt"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

func (v *subagentViewer) renderWorkflowInspector(width int) []string {
	w, ok := v.manager.Workflow(v.id)
	if !ok {
		return []string{"Workflow no longer exists"}
	}
	height := v.renderHeight()
	theme := tui.ActiveTheme()
	records := v.manager.WorkflowRecords(v.id)
	v.selectedAgent = min(max(0, v.selectedAgent), max(0, len(records)-1))
	leftWidth := min(42, max(18, width/3))
	if width < 60 {
		leftWidth = max(10, width/2)
	}
	rightWidth := max(1, width-leftWidth-3)
	rows := max(1, height-4)
	left := []string{}
	start := max(0, v.selectedAgent-rows+1)
	for i := start; i < len(records) && len(left) < rows; i++ {
		r := records[i]
		cursor := "  "
		if i == v.selectedAgent {
			cursor = "› "
		}
		label := r.Description
		if label == "" {
			label = r.Type
		}
		line := cursor + subagentIcon(r) + " " + label + " · " + r.Status
		left = append(left, clampAgentLine(line, leftWidth))
	}
	if len(records) == 0 {
		left = append(left, theme.Fg("dim", "Waiting for agents…"))
	}
	var right []string
	if len(records) > 0 {
		r := records[v.selectedAgent]
		right = append(right, subagentName(r)+" · "+r.Status, r.Description, subagentStats(r, v.manager.Settings()), "", r.Result)
		if r.Error != "" {
			right = append(right, theme.Fg("error", r.Error))
		}
		if r.Active() {
			right = append(right, r.Activity)
		}
		right = append(right, "", theme.Fg("dim", "Enter opens the live conversation"))
	}
	if len(records) == 0 || v.showWorkflowLog {
		right = nil
		for _, p := range w.Progress {
			right = append(right, string(p))
		}
		if !w.Active() {
			right = append(right, string(w.Result), w.Error)
		}
	}
	var wrapped []string
	for _, line := range right {
		wrapped = append(wrapped, widthx.WrapTextWithAnsi(line, rightWidth)...)
	}
	right = wrapped
	v.maxScroll = max(0, len(right)-rows)
	if v.follow {
		v.scroll = v.maxScroll
	}
	v.scroll = min(max(0, v.scroll), v.maxScroll)
	title := fmt.Sprintf("%s · %s · %d/%d agents · %s tokens · %ds", w.Meta.Name, w.Status, w.DoneCount, w.TotalCount, subagentCompact(w.Tokens), (time.Now().UnixMilli()-w.StartedAt)/1000)
	lines := []string{clampAgentLine(tui.ChalkBold(title), width), theme.Fg("borderMuted", strings.Repeat("─", max(1, width)))}
	for i := 0; i < rows; i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if v.scroll+i < len(right) {
			r = right[v.scroll+i]
		}
		l += strings.Repeat(" ", max(0, leftWidth-widthx.VisibleWidth(l)))
		lines = append(lines, clampAgentLine(l+theme.Fg("borderMuted", " │ ")+r, width))
	}
	lines = append(lines, theme.Fg("borderMuted", strings.Repeat("─", max(1, width))))
	footer := "↑/↓ select · enter view · s skip · r retry · p pause · l log · x stop · esc close"
	if w.Status == "paused" {
		footer = strings.Replace(footer, "p pause", "p resume", 1)
	}
	if v.confirmStop {
		footer = "Press x again to stop · esc cancel"
	}
	v.footerRow = len(lines)
	lines = append(lines, theme.Fg("dim", clampAgentLine(footer, width)))
	return lines
}

func (v *subagentViewer) handleWorkflowInspector(data string) bool {
	records := v.manager.WorkflowRecords(v.id)
	switch {
	case tui.MatchesKeyID(data, "up"):
		v.selectedAgent = max(0, v.selectedAgent-1)
		v.scroll = 0
		v.follow = false
	case tui.MatchesKeyID(data, "down"):
		v.selectedAgent = min(max(0, len(records)-1), v.selectedAgent+1)
		v.scroll = 0
		v.follow = false
	case tui.MatchesKeyID(data, "enter"):
		if len(records) > 0 {
			v.m.openSubagentViewer(records[v.selectedAgent].ID, false)
		}
	case data == "p":
		if err := v.manager.PauseWorkflow(v.id); err != nil {
			v.m.showWarning(err.Error())
		}
	case data == "s":
		if len(records) > 0 {
			if err := v.manager.SkipWorkflowAgent(v.id, records[v.selectedAgent].ID); err != nil {
				v.m.showWarning(err.Error())
			}
		}
	case data == "r":
		if len(records) > 0 {
			if err := v.manager.RetryWorkflowAgent(v.id, records[v.selectedAgent].ID); err != nil {
				v.m.showWarning(err.Error())
			}
		}
	case data == "l":
		v.showWorkflowLog = !v.showWorkflowLog
	default:
		return false
	}
	v.m.requestRender()
	return true
}
