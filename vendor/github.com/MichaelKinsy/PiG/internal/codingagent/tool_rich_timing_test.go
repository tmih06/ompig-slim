package codingagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TestRichCardHeaderOwnsTiming ensures shared shell/codemode result renderers
// do not add a second clock or start a redundant timer invalidation goroutine.
func TestRichCardHeaderOwnsTiming(t *testing.T) {
	for _, name := range []string{"bash", "powershell", "codemode"} {
		t.Run(name, func(t *testing.T) {
			card := NewToolRendererCard(context.Background(), name, "call", "", json.RawMessage(`{"command":"echo hi","code":"console.log('hi')"}`), CreateAllToolRenderers()[name], nil)
			defer card.Dispose()
			c := card.Component
			c.MarkExecutionStarted()
			c.StartedAt = time.Now().Add(-5 * time.Second)
			result := agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "hi"}}}
			for _, partial := range []bool{true, false} {
				update := ToolExecutionResultOf(result, time.Second)
				c.UpdateResult(update, partial)
				out := widthx.StripAnsi(strings.Join(c.Render(100), "\n"))
				if strings.Contains(out, "Elapsed ") || strings.Contains(out, "Took ") {
					t.Fatalf("duplicate timing in result: %s", out)
				}
				// The header carries the only timing line for a rich card.
				if !strings.Contains(out, "Bash") && !strings.Contains(out, "Codemode") {
					t.Fatalf("missing header status: %s", out)
				}
				card.state.mu.Lock()
				ticking := card.state.stopTick != nil
				card.state.mu.Unlock()
				if ticking {
					t.Fatal("rich result started redundant timer")
				}
			}
		})
	}
}
