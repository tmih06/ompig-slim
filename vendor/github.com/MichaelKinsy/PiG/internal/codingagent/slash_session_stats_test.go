package codingagent

import (
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/subagents"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

func TestSessionStatsLayout(t *testing.T) {
	session := NewSession("stats-test", t.TempDir())
	now := time.Now().UnixMilli()
	user := agent.AgentMessage{User: &agent.UserMessage{Role: "user", Content: ai.UserText("hi"), Timestamp: now - 60_000}}
	if _, err := session.AppendMessage(user); err != nil {
		t.Fatal(err)
	}
	assistant := agent.AgentMessage{Assistant: &agent.AssistantMessage{
		Role:      "assistant",
		Provider:  "anthropic",
		ModelID:   "claude-test",
		Timestamp: now,
		Usage:     &ai.Usage{Input: 1000, Output: 500, CacheRead: 3000, Cost: ai.UsageCost{Total: 0.01}},
	}}
	if _, err := session.AppendMessage(assistant); err != nil {
		t.Fatal(err)
	}
	other := agent.AgentMessage{Assistant: &agent.AssistantMessage{
		Role:      "assistant",
		Provider:  "openai",
		ModelID:   "gpt-test",
		Timestamp: now,
		Usage:     &ai.Usage{Input: 9000, Output: 1000, Cost: ai.UsageCost{Total: 0.02}},
	}}
	if _, err := session.AppendMessage(other); err != nil {
		t.Fatal(err)
	}

	records := []subagents.Record{{
		ID: "a1", DisplayName: "scout", Status: "completed", StartedAt: now - 90_000, CompletedAt: now,
		Turns: 3, ToolUses: 5, Tokens: 12000, Cost: 0.004,
	}, {
		ID: "a2", DisplayName: "worker", Status: "completed", StartedAt: now - 60_000, CompletedAt: now,
		Turns: 7, ToolUses: 9, Tokens: 8000,
	}}
	sc := &SlashContext{
		CurrentSession: func() *Session { return session },
		SessionSubagents: func() ([]subagents.Record, subagents.Settings) {
			return records, subagents.Settings{ShowModel: true, ShowCost: true}
		},
	}
	out := widthx.StripTerminalSequences(sessionInfoRenderer(sc)())

	for _, want := range []string{
		"Session Stats", "stats-test",
		"Messages", "Tokens",
		"Tokens by model", "openai/gpt-test", "anthropic/claude-test", "69%", "31%",
		"Timeline", "Started:", "Activity:",
		"Subagents", "scout", "3 turns", "2 agents",
		"Cache Warming", "Cost",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	if strings.Count(out, "●") != 2 {
		t.Errorf("expected one bar bullet per model, got:\n%s", out)
	}
}

func TestSessionSparkline(t *testing.T) {
	if got := sessionSparkline(nil, 8); got != "" {
		t.Fatalf("empty series = %q", got)
	}
	if got := sessionSparkline([]int64{1000}, 8); got != "█▁▁▁▁▁▁▁" {
		t.Fatalf("single instant = %q", got)
	}
	got := sessionSparkline([]int64{0, 1, 98, 99, 100}, 4)
	cells := []rune(got)
	if len(cells) != 4 || cells[0] == '▁' || cells[3] == '▁' || cells[1] != '▁' {
		t.Fatalf("bimodal spread = %q", got)
	}
}

func TestSessionDuration(t *testing.T) {
	for ms, want := range map[int64]string{
		45_000:       "45s",
		3_690_000:    "1h 1m",
		180_000_000:  "2d 2h",
		5_000:        "5s",
		61_000:       "1m 1s",
	} {
		if got := sessionDuration(ms); got != want {
			t.Errorf("sessionDuration(%d) = %q, want %q", ms, got, want)
		}
	}
}
