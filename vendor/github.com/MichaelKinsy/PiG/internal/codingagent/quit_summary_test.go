package codingagent

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

func TestQuitSummary(t *testing.T) {
	previous := supportsPiLogo
	t.Cleanup(func() { supportsPiLogo = previous })
	for _, logo := range []bool{true, false} {
		supportsPiLogo = func() bool { return logo }
		stats := SessionAccounting{
			UserMessages: 2, AssistantMessages: 3, ToolCalls: 1,
			Tokens: SessionTokenStats{Input: 1000, Output: 500, CacheRead: 2000, CacheWrite: 3000, Cost: 0.123},
		}
		got := widthx.StripTerminalSequences(renderQuitSummary(stats, "ompig-slim --session example"))
		for _, want := range []string{
			"-slim", "Session summary", "Messages  2 user, 3 assistant, 1 tool calls",
			"Tokens    ↑ 6.0k (cached 2.0k, cache write 3.0k) · ↓ 500",
			"Cost      $0.123", "Resume    ompig-slim --session example",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("summary missing %q:\n%s", want, got)
			}
		}
		if logo {
			top, bottom := logoMark(tui.ActiveTheme().GetColorMode())
			for _, line := range []string{top, bottom} {
				if !strings.Contains(got, widthx.StripTerminalSequences(line)) {
					t.Fatalf("summary missing startup logo:\n%s", got)
				}
			}
		} else if !strings.Contains(got, "OMPi") {
			t.Fatalf("summary missing text logo:\n%s", got)
		}
	}
}

func TestQuitSummaryWithoutPersistenceOrUsage(t *testing.T) {
	got := widthx.StripTerminalSequences(renderQuitSummary(SessionAccounting{}, ""))
	for _, want := range []string{"Session summary", "0 user, 0 assistant, 0 tool calls", "↑ 0", "↓ 0"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"Resume", "Cost", "NaN", "Inf"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("summary contains %q:\n%s", unwanted, got)
		}
	}
}
