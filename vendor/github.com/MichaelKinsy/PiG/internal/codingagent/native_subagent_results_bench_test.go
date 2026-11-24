package codingagent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/internal/subagents"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

func BenchmarkSubagentResultTranscript(b *testing.B) {
	for _, expanded := range []bool{false, true} {
		b.Run(fmt.Sprintf("expanded=%t", expanded), func(b *testing.B) {
			m := &InteractiveMode{}
			card := m.newToolCard("get_subagent_result", "bench", []byte(`{"agent_id":"agent_bench"}`))
			card.UpdateResult(toolResultUpdate(agent.AgentToolResult{
				Content: []ai.ToolResultMessageContent{ai.TextContent{Text: strings.Repeat("Unicode λ <result> & exact text. ", 4096)}},
				Details: subagents.OutcomeEnvelope{AgentID: "agent_bench", Outcome: subagents.OutcomeCompleted},
			}))
			card.SetExpanded(expanded)
			card.Render(120)
			var lines []string
			b.ReportAllocs()
			for b.Loop() {
				lines = card.Render(120)
			}
			if !strings.Contains(widthx.StripTerminalSequences(strings.Join(lines, "\n")), "agent_bench") {
				b.Fatal("transcript lost the result card")
			}
		})
	}
}

func BenchmarkSubagentResultCard(b *testing.B) {
	for _, rows := range []int{1, 128, 1024} {
		text := strings.Repeat("Unicode λ <result> & exact text. ", 4) + "\n"
		result := agent.AgentToolResult{
			Content: []ai.ToolResultMessageContent{ai.TextContent{Text: strings.Repeat(text, rows)}},
			Details: subagents.OutcomeEnvelope{AgentID: "agent_bench", Outcome: subagents.OutcomeCompleted},
		}
		for _, expanded := range []bool{false, true} {
			b.Run(fmt.Sprintf("rows=%d/expanded=%t", rows, expanded), func(b *testing.B) {
				options := extension.ToolRenderResultOptions{Expanded: expanded}
				for _, workload := range []string{"cold", "redraw", "resize"} {
					b.Run(workload, func(b *testing.B) {
						component := subagentResultRenderResult(result, options, nil, extension.ToolRenderContext{})
						component.Render(120)
						width := 120
						var lines []string
						b.ReportAllocs()
						for b.Loop() {
							if workload == "cold" {
								component = subagentResultRenderResult(result, options, nil, extension.ToolRenderContext{})
							}
							if workload == "resize" {
								width = 160 - width
							}
							lines = component.Render(width)
						}
						if len(lines) < 2 {
							b.Fatalf("result card lost its header or body: %q", lines)
						}
					})
				}
			})
		}
	}
}
