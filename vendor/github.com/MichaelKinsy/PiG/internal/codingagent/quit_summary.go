package codingagent

import (
	"fmt"
	"strings"

	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/tui"
)

func (m *InteractiveMode) printQuitSummary() {
	if !stdoutIsTTY() {
		return
	}
	session := m.currentSession()
	if session == nil {
		return
	}
	_, _ = fmt.Print(renderQuitSummary(session.Accounting(), m.resumeHintCommand()))
}

func renderQuitSummary(stats SessionAccounting, resume string) string {
	mode := tui.ActiveTheme().GetColorMode()
	var b strings.Builder
	b.WriteByte('\n')
	if supportsPiLogo() {
		top, bottom := logoMark(mode)
		fmt.Fprintf(&b, "%s\n%s\n", top, bottom)
	} else {
		fmt.Fprintf(&b, "%s %s\n", piWordmark(mode), productTag(mode))
	}
	b.WriteString("\nSession summary\n")
	fmt.Fprintf(&b, "Messages  %d user, %d assistant, %d tool calls\n", stats.UserMessages, stats.AssistantMessages, stats.ToolCalls)
	tokens := stats.Tokens
	fmt.Fprintf(&b, "Tokens    ↑ %s (cached %s, cache write %s) · ↓ %s\n",
		formatTokens(tokens.Input+tokens.CacheRead+tokens.CacheWrite), formatTokens(tokens.CacheRead),
		formatTokens(tokens.CacheWrite), formatTokens(tokens.Output))
	if tokens.Cost > 0 {
		// Same three decimals as /session and the footer. A sub-millicent cost
		// rounds to zero there, which would report a paid session as free.
		cost := jsstring.ToFixed(tokens.Cost, 3)
		if cost == "0.000" {
			cost = "<0.001"
		}
		fmt.Fprintf(&b, "Cost      $%s\n", cost)
	}
	if resume != "" {
		fmt.Fprintf(&b, "Resume    %s\n", resume)
	}
	return b.String()
}
