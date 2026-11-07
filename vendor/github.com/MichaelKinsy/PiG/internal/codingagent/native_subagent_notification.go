package codingagent

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/MichaelKinsy/PiG/internal/subagents"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// Keep the model's notification envelope in the log while presenting the same
// compact, expandable completion cards as the reference extension.
type subagentNotificationCard struct {
	text     string
	expanded bool
	manager  *subagents.Manager
}

func (c *subagentNotificationCard) Invalidate() {}
func (c *subagentNotificationCard) Render(width int) []string {
	var root struct {
		Tasks []struct {
			ID      string `xml:"task-id"`
			Status  string `xml:"status"`
			Summary string `xml:"summary"`
			Result  string `xml:"result"`
			Text    string `xml:",chardata"`
		} `xml:"task-notification"`
	}
	if xml.Unmarshal([]byte("<notifications>"+c.text+"</notifications>"), &root) != nil || len(root.Tasks) == 0 {
		return widthx.WrapTextWithAnsi(c.text, max(1, width))
	}
	theme := tui.ActiveTheme()
	var lines []string
	for _, task := range root.Tasks {
		label, stats, result := task.Summary, "", task.Result
		if c.manager != nil && task.ID != "" {
			if r, ok := c.manager.Find(task.ID); ok {
				label = subagentName(r) + " · " + r.Description
				stats = subagentStats(r, c.manager.Settings())
				result = r.Result
				if r.Error != "" {
					result += "\n" + r.Error
				}
			}
		}
		if label == "" {
			label = "Workflow completed"
		}
		if result == "" {
			result = strings.TrimSpace(task.Text)
		}
		icon, color := "✓", "success"
		if task.Status == "error" || task.Status == "aborted" {
			icon, color = "✗", "error"
		}
		lines = append(lines, clampAgentLine(theme.Fg(color, icon+" ")+tui.ChalkBold(label), width))
		if stats != "" {
			lines = append(lines, clampAgentLine(theme.Fg("dim", "  "+stats), width))
		}
		var body []string
		for _, line := range strings.Split(strings.TrimSpace(result), "\n") {
			body = append(body, widthx.WrapTextWithAnsi(line, max(1, width-4))...)
		}
		hidden := 0
		if !c.expanded && len(body) > 4 {
			hidden = len(body) - 4
			body = body[:4]
		}
		for _, line := range body {
			lines = append(lines, clampAgentLine(theme.Fg("dim", "  ⎿ ")+line, width))
		}
		if hidden > 0 {
			lines = append(lines, clampAgentLine(theme.Fg("dim", fmt.Sprintf("    … %d more lines (ctrl+o to expand)", hidden)), width))
		}
	}
	return lines
}
