package codingagent

// OMP Agent Hub styling: active chips and hovered rows use selectedBg; the
// surrounding panels keep the terminal background. Tokens follow the active
// theme rather than introducing a separate Hub palette.
import (
	"strings"

	"github.com/MichaelKinsy/PiG/tui"
)

func hubSurface(line string, width int, background string) string {
	line = hubPad(line, width)
	if background == "" {
		return line
	}
	theme := tui.ActiveTheme()
	open := theme.GetBgAnsi(background)
	line = strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+open)
	line = strings.ReplaceAll(line, "\x1b[m", "\x1b[m"+open)
	line = strings.ReplaceAll(line, "\x1b[49m", open)
	return theme.Bg(background, line)
}

func hubChip(label string, active bool) string {
	theme := tui.ActiveTheme()
	if active {
		return theme.Bg("selectedBg", tui.ChalkBold(theme.Fg("accent", " "+label+" ")))
	}
	return theme.Fg("muted", " "+label+" ")
}

func hubHeading(label string) string {
	return tui.ActiveTheme().Fg("accent", tui.ChalkBold(label))
}

func hubStatus(state, text string) string {
	token := "muted"
	switch state {
	case "running", "queued", "pending":
		token = "accent"
	case "idle", "completed", "steered":
		token = "success"
	case "aborted", "error", "stopped":
		token = "error"
	}
	return tui.ActiveTheme().Fg(token, text)
}

func hubModel(model, thinking string) string {
	theme := tui.ActiveTheme()
	label := theme.Fg("muted", hubSingleLine(model))
	tokens := map[string]string{"minimal": "thinkingMinimal", "low": "thinkingLow", "medium": "thinkingMedium", "high": "thinkingHigh", "xhigh": "thinkingXhigh", "max": "thinkingMax"}
	if token, ok := tokens[thinking]; ok {
		label += " " + theme.Fg(token, hubSingleLine(thinking))
	}
	return label
}
