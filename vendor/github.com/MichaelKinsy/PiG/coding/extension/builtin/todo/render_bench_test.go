package todo

import (
	"encoding/json"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/tui"
)

// benchPhases builds a list shaped like real work: several phases, one active
// task each, the rest open.
func benchPhases(phases, tasks int) []Phase {
	out := make([]Phase, phases)
	for p := range out {
		out[p].Name = "Phase"
		for t := 0; t < tasks; t++ {
			status := StatusPending
			if t == 0 {
				status = StatusInProgress
			}
			out[p].Tasks = append(out[p].Tasks, Task{
				Content: "a representative task description",
				Status:  status,
			})
		}
	}
	return out
}

// BenchmarkRenderResult measures drawing a full card, the work a renderer
// repeats on every expand, progress update and resize. It is the load-bearing
// measurement for the row-joining and details-decoding changes: both are only
// worth their complexity if this stays flat or improves.
func BenchmarkRenderResult(b *testing.B) {
	def := Definition()
	phases := benchPhases(4, 10)
	args := json.RawMessage(`{"op":"done","task":"x"}`)
	result := extension.AgentToolResult{
		Details: Details{Op: OpDone, Phases: phases},
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "done"}},
	}
	renderers := &tui.ToolDefinitionRenderers{
		Unframed: true,
		Call:     func(tui.ToolRenderInput) (tui.Component, bool) { return blankComponent(), true },
		Result: func(input tui.ToolRenderInput) (tui.Component, bool) {
			return def.RenderResult(result, extension.ToolRenderResultOptions{}, tui.ActiveTheme(),
				extension.ToolRenderContext{Cwd: "/tmp", Args: input.Args}), true
		},
	}
	b.ReportAllocs()
	for b.Loop() {
		comp := tui.NewToolExecutionComponent("todo", "call", args,
			tui.ToolExecutionOptions{Rich: true}, nil, nil, "/tmp")
		comp.SetDefinition(renderers, args)
		comp.SetArgsComplete()
		comp.SetResultValue(result)
		comp.SetResult("done", false, 0)
		comp.Render(100)
	}
}

// BenchmarkDecodeDetails pins the fast path for the typed details the tool
// returns. It is a regression guard: decoding them through JSON was ~2300x
// slower and dominated every render.
func BenchmarkDecodeDetails(b *testing.B) {
	result := extension.AgentToolResult{
		Details: Details{Op: OpDone, Phases: benchPhases(4, 10)},
	}
	b.ReportAllocs()
	for b.Loop() {
		decodeDetails(result)
	}
}
