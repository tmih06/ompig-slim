package tui

import (
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// richToolLabel identifies the tool's visual role without requiring icon fonts.
// Unknown extension names retain their identity rather than borrowing a built-in role.
func richToolLabel(name string) string {
	switch name {
	case "bash", "powershell":
		return "$ SHELL"
	case "read":
		return "≡ READ"
	case "write":
		return "+ WRITE"
	case "edit":
		return "± EDIT"
	case "grep":
		return "⌕ GREP"
	case "find":
		return "⌕ FIND"
	case "ls":
		return "≡ FILES"
	case "web_search":
		return "⌕ WEB SEARCH"
	case "web_fetch":
		return "↓ WEB FETCH"
	case "codemode":
		return "λ CODEMODE"
	case "Agent", "subagent":
		return "⑂ AGENT"
	default:
		return "◇ " + strings.ToUpper(name)
	}
}

// richStatus returns lifecycle text and a theme role. Only running executions
// consult the clock; restored results never invent elapsed time from wall time.
func (c *ToolExecutionComponent) richStatus(now time.Time) (string, string) {
	status, role := "✓ Done", "success"
	elapsed := c.Elapsed
	switch {
	case c.State == ToolStateRunning:
		status, role = "Preparing", "muted"
		if !c.StartedAt.IsZero() {
			elapsed = max(0, now.Sub(c.StartedAt))
			// Ten braille frames share the session's existing spinner clock.
			frames := [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			status, role = frames[int(elapsed/(80*time.Millisecond))%len(frames)]+" Running", "accent"
		}
	case c.aborted:
		status, role = "■ Cancelled", "warning"
	case c.State == ToolStateError:
		status, role = "✗ Error", "error"
	}
	if c.State != ToolStateRunning && c.DurationMs != nil {
		elapsed = time.Duration(*c.DurationMs) * time.Millisecond
	}
	if elapsed > 0 {
		status += " · " + FormatToolDuration(elapsed)
	}
	return status, role
}

// richSectionLabel draws a width-bounded divider inside the card. Keeping it
// outside mouse regions prevents decorative labels from toggling tool output.
func richSectionLabel(label string, width int) []string {
	line := widthx.TruncateToWidth("─ "+label+" ", max(0, width), "", false)
	line += strings.Repeat("─", max(0, width-widthx.VisibleWidth(line)))
	return []string{ActiveTheme().Fg("muted", line)}
}

// richResultPlaceholder keeps empty, pending, and deliberately hidden results
// distinguishable without overriding a tool's collapsed preview policy.
func (c *ToolExecutionComponent) richResultPlaceholder(width int) []string {
	label := "No text output"
	switch {
	case c.State == ToolStateRunning:
		label = "Waiting for result…"
	case c.Collapsed && c.Output != "":
		label = "Output hidden · " + AppKeyText("app.tools.expand", "ctrl+o") + " to expand"
	case len(c.ImageBlocks) > 0:
		label = "Image output below"
	}
	return NewPaddedText(ActiveTheme().Fg("muted", label), 0, 0, nil).Render(max(1, width))
}

// renderRichFrame surrounds existing renderer rows without interpreting their
// content. Width is at least five terminal cells. Images remain outside this
// frame, and tool renderers keep ownership of previews, syntax, and expansion.
func (c *ToolExecutionComponent) renderRichFrame(width int, content []string) []string {
	theme := ActiveTheme()
	status, role := c.richStatus(time.Now())
	inner := width - 2
	border := func(s string) string { return theme.Fg(role, s) }
	// Truncation uses terminal cells and preserves ANSI/OSC boundaries, not bytes.
	header := " " + richToolLabel(c.Name) + " · " + status + " "
	header = widthx.TruncateToWidth(header, inner, "…", false)
	out := make([]string, 0, len(content)+2)
	out = append(out, border("╭"+header+strings.Repeat("─", max(0, inner-widthx.VisibleWidth(header)))+"╮"))
	for _, line := range content {
		out = append(out, border("│")+widthx.TruncateToWidth(line, inner, "…", true)+border("│"))
	}
	footer := ""
	if c.definitionHasResult() {
		action := "expand"
		if !c.Collapsed {
			action = "collapse"
		}
		footer = " " + AppKeyText("app.tools.expand", "ctrl+o") + " to " + action + " "
	}
	footer = widthx.TruncateToWidth(footer, inner, "…", false)
	out = append(out, border("╰")+theme.Fg("muted", footer)+border(strings.Repeat("─", max(0, inner-widthx.VisibleWidth(footer)))+"╯"))
	return out
}
