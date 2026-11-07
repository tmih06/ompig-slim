package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// StatusIndicator renders a loader either standalone or inside an editor border.
// Its owner advances animation and disposes operation timers on replacement.
type StatusIndicator struct {
	*Loader
	Kind      string
	StartedAt time.Time
}

func (s *StatusIndicator) RenderInBorder(width int) string {
	lines := s.Render(width + 2)
	if len(lines) < 2 {
		return ""
	}
	line := strings.TrimPrefix(lines[1], " ")
	line = strings.TrimRightFunc(line, func(r rune) bool { return r == '\ufeff' || (r != '\u0085' && unicode.IsSpace(r)) })
	if s.Kind == "working" && !s.StartedAt.IsZero() {
		line += " " + formatElapsed(time.Since(s.StartedAt))
	}
	return widthx.TruncateToWidth(line, width, "", false)
}

// formatElapsed uses MM:SS below an hour and H:MM:SS after that, flooring partial seconds.
func formatElapsed(elapsed time.Duration) string {
	seconds := int(elapsed / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	hours := seconds / 3600
	minutes := seconds / 60 % 60
	seconds %= 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func (s *StatusIndicator) RenderSpinnerInBorder(width int) string {
	return widthx.TruncateToWidth(s.renderedIndicator(), width, "", false)
}

// IdleStatus reserves the standalone loader's two rows after clearing it.
type IdleStatus struct{}

func (*IdleStatus) Invalidate() {}
func (*IdleStatus) Render(width int) []string {
	line := strings.Repeat(" ", max(0, width))
	return []string{line, line}
}
