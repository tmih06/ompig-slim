package codingagent

import (
	"strings"

	"github.com/MichaelKinsy/PiG/tui"
)

// hubSurface paints a whole cell row, restoring the enclosing surface after
// nested chips or selected rows reset their backgrounds. No terminal-wide
// palette or background changes are needed.
func hubSurface(line string, width int, background string) string {
	theme := tui.ActiveTheme()
	open := theme.GetBgAnsi(background)
	text := theme.GetFgAnsi("text")
	line = hubPad(line, width)
	line = strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+open+text)
	line = strings.ReplaceAll(line, "\x1b[m", "\x1b[m"+open+text)
	line = strings.ReplaceAll(line, "\x1b[49m", open)
	line = strings.ReplaceAll(line, "\x1b[39m", text)
	return theme.Bg(background, theme.Fg("text", line))
}

func hubHeading(label string) string {
	return tui.ActiveTheme().Fg("customMessageLabel", tui.ChalkBold(label))
}

func hubStatus(state, text string) string {
	token := "muted"
	switch state {
	case "running", "queued", "pending":
		token = "accent"
	case "idle", "completed", "steered":
		token = "success"
	case "parked":
		token = "warning"
	case "aborted", "error", "stopped":
		token = "error"
	}
	return tui.ActiveTheme().Fg(token, text)
}

func hubKeys(text string) string {
	var keys []string
	for _, part := range strings.Split(text, "  ") {
		key, label, ok := strings.Cut(part, ":")
		if !ok {
			keys = append(keys, tui.ActiveTheme().Fg("muted", part))
			continue
		}
		keys = append(keys, tui.ActiveTheme().Fg("accent", tui.ChalkBold(key))+tui.ActiveTheme().Fg("muted", ":"+label))
	}
	return strings.Join(keys, "  ")
}

func contextColor(fraction float64) string {
	if fraction >= .85 {
		return "error"
	}
	if fraction >= .7 {
		return "warning"
	}
	return "success"
}
