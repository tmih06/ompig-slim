package codingagent

// Ports pi-subagents src/agent-color.ts: inverse name badges, palette and contrast.
import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/MichaelKinsy/PiG/tui"
)

func subagentBadge(name, color string) string {
	palette := map[string]string{"red": "#DC2626", "blue": "#6A9BCC", "green": "#16A34A", "yellow": "#CA8A04", "purple": "#827DBD", "orange": "#D97757", "pink": "#C46686", "cyan": "#0891B2", "amber": "#F59E0B", "teal": "#008080", "indigo": "#6366F1", "gold": "#EAB308", "neon-green": "#10B981", "neon-cyan": "#06B6D4", "metallic-blue": "#3B82F6", "violet": "#8B5CF6", "rose": "#F43F5E", "lime": "#84CC16", "gray": "#6B7280", "grey": "#6B7280", "fuchsia": "#D946EF", "slate": "#64748B", "navy": "#1E3A8A"}
	color = strings.ToLower(strings.TrimSpace(color))
	if hex, ok := palette[color]; ok {
		color = hex
	}
	if len(color) != 7 || color[0] != '#' {
		return tui.ChalkBold(name)
	}
	n, err := strconv.ParseUint(color[1:], 16, 24)
	if err != nil {
		return tui.ChalkBold(name)
	}
	r, g, b := int(n>>16), int(n>>8&255), int(n&255)
	background := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
	if string(tui.ActiveTheme().GetColorMode()) == "256color" {
		cube := []int{0, 95, 135, 175, 215, 255}
		nearest := func(channel int) int {
			best := 0
			for i, c := range cube {
				if math.Abs(float64(channel-c)) < math.Abs(float64(channel-cube[best])) {
					best = i
				}
			}
			return best
		}
		ri, gi, bi := nearest(r), nearest(g), nearest(b)
		index := 16 + 36*ri + 6*gi + bi
		r, g, b = cube[ri], cube[gi], cube[bi]
		background = fmt.Sprintf("\x1b[48;5;%dm", index)
	}
	linear := func(channel int) float64 {
		v := float64(channel) / 255
		if v <= .04045 {
			return v / 12.92
		}
		return math.Pow((v+.055)/1.055, 2.4)
	}
	foreground := "\x1b[38;2;255;255;255m"
	if .2126*linear(r)+.7152*linear(g)+.0722*linear(b) > .179 {
		foreground = "\x1b[38;2;0;0;0m"
	}
	return background + foreground + tui.ChalkBold(" "+name+" ") + "\x1b[39m\x1b[49m"
}
