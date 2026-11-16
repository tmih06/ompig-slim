package codingagent

import (
	"fmt"
	"strings"

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
		fmt.Fprintf(&b, "Cost      $%.3f\n", tokens.Cost)
	}
	if resume != "" {
		fmt.Fprintf(&b, "Resume    %s\n", resume)
	}
	return b.String()
}
