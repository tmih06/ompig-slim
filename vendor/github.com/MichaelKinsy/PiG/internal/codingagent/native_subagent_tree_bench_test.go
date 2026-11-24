package codingagent

import (
	"fmt"
	"hash/crc32"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/internal/subagents"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

func BenchmarkAgentTree(b *testing.B) {
	for _, count := range []int{1, 20, 200, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			manager := subagents.New(subagents.Host{
				CWD: b.TempDir(), AgentDir: b.TempDir(), SessionID: "tree-benchmark", Restore: func() []subagents.Record {
					records := make([]subagents.Record, count)
					for i := range records {
						name := fmt.Sprintf("worker_%04d", i)
						records[i] = subagents.Record{
							ID: name, Handle: name, Description: "Inspect the implementation and report findings",
							Status: "completed", Background: true, StartedAt: 1, CompletedAt: time.Now().UnixMilli(),
							ModelName: "fixture-model", Thinking: "high", Turns: 5, ToolUses: 12,
							Tokens: 12400, ContextTokens: 50000, ContextWindow: 200000, Cost: 0.02,
							Params: subagents.Params{Role: "@task"},
						}
					}
					return records
				},
			})
			b.Cleanup(manager.Close)
			if err := manager.Update(map[string]any{"enabled": true}); err != nil {
				b.Fatal(err)
			}
			selected := manager.Records()[count/2].ID
			m := &InteractiveMode{
				opts:       InteractiveModeOptions{SessionHandle: observerSession{manager: manager}},
				tuiInst:    tui.NewWithOutput(io.Discard, 120, 40),
				editor:     tui.NewEditor(),
				subagentUI: &subagentUIState{manager: manager, treeSelected: selected},
			}
			m.tuiInst.SetRenderDeadline(func(time.Time) {})
			b.Cleanup(func() {
				if m.subagentUI.expireTimer != nil {
					m.subagentUI.expireTimer.Stop()
				}
			})
			tree := &nativeAgentTree{m: m}
			footer := NewFooterComponent(nil, "", nil)
			footer.agents = m.subagentCounts
			viewport := CreateChatViewport(ChatViewportOptions{
				Document:        tui.NewPaddedText(strings.Repeat("Conversation line\n", 100), 0, 0, nil),
				PendingMessages: tui.NewContainer(),
				Status:          tui.NewContainer(),
				Editor:          m.editor,
				Footer:          tui.NewContainer(footer, tree),
			})
			check := func(b *testing.B, lines []string) {
				b.Helper()
				text := widthx.StripTerminalSequences(strings.Join(lines, "\n"))
				if !strings.Contains(text, "›") || !strings.Contains(text, selected) {
					b.Fatalf("render omitted selected agent %s: %q", selected, text)
				}
				b.ReportMetric(float64(crc32.ChecksumIEEE([]byte(strings.Join(lines, "\n")))), "frame-hash")
			}
			b.Run("render", func(b *testing.B) {
				b.ReportAllocs()
				var lines []string
				for b.Loop() {
					lines = tree.Render(120)
				}
				check(b, lines)
			})
			b.Run("counts", func(b *testing.B) {
				b.ReportAllocs()
				var total, working int
				for b.Loop() {
					total, working = m.subagentCounts()
				}
				if total != count || working != 0 {
					b.Fatalf("counts = %d %d, want %d 0", total, working, count)
				}
			})
			b.Run("layout", func(b *testing.B) {
				b.ReportAllocs()
				var frame tui.LayoutFrame
				for b.Loop() {
					footer.Invalidate()
					frame = tui.RenderLayoutFrame(viewport.Root, 120, 40, func() {})
				}
				check(b, frame.Lines)
			})
			b.Run("hidden", func(b *testing.B) {
				m.subagentUI.treeSelected = ""
				b.ReportAllocs()
				var lines []string
				for b.Loop() {
					lines = tree.Render(120)
				}
				if lines != nil {
					b.Fatalf("hidden tree rendered: %q", lines)
				}
			})
		})
	}
}
