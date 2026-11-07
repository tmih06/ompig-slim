package codingagent

// OMP Agent Hub styling: active chips and hovered rows use selectedBg; the
// surrounding panels keep the terminal background. Tokens follow the active
// theme rather than introducing a separate Hub palette.
import (
	"strings"

	"github.com/MichaelKinsy/PiG/tui"
)

// hubHoverLine fills one roster line to width with the selection background.
// Reopen the fill after nested style resets so hovering covers the whole row.
func hubHoverLine(line string, width int) string {
	line = hubPad(line, width)
	theme := tui.ActiveTheme()
	open := theme.GetBgAnsi("selectedBg")
	line = strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+open)
	line = strings.ReplaceAll(line, "\x1b[m", "\x1b[m"+open)
	line = strings.ReplaceAll(line, "\x1b[49m", open)
	return theme.Bg("selectedBg", line)
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

// hubModel sanitizes the model label and adds a badge only for enabled, known
// thinking levels. The theme owns their colors; off and unknown remain hidden.
func hubModel(model, thinking string) string {
	theme := tui.ActiveTheme()
	label := theme.Fg("muted", hubSingleLine(model))
	switch thinking {
	case "minimal", "low", "medium", "high", "xhigh", "max":
		label += " " + theme.GetThinkingBorderColor(thinking)(thinking)
	}
	return label
}
