package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TestRichStatusLine covers OMP's status line: spinner/pending glyph, the
// `Title: description` form, exit-code and duration meta, and error states.
func TestRichStatusLine(t *testing.T) {
	for _, name := range []string{"bash", "read", "write", "edit", "grep", "find", "ls", "web_search", "web_fetch", "codemode", "Agent", "custom_tool"} {
		t.Run(name, func(t *testing.T) {
			c := NewToolExecutionComponent(name, "call", json.RawMessage(`{}`), ToolExecutionOptions{Rich: true}, nil, nil, "")
			if line := widthx.StripAnsi(c.richStatusLine(80, time.Now())); !strings.HasPrefix(line, "⏳ ") {
				t.Fatalf("pending status: %q", line)
			}
			c.MarkExecutionStarted()
			c.StartedAt = time.Now().Add(-1500 * time.Millisecond)
			line := widthx.StripAnsi(c.richStatusLine(80, time.Now()))
			if strings.Contains(line, "⏳") || !strings.Contains(line, FormatToolDuration(1500*time.Millisecond)) || !c.IsDirty() {
				t.Fatalf("running status: %q", line)
			}
			c.SetResult("ok", false, time.Second)
			duration := int64(2500)
			c.DurationMs = &duration
			line = widthx.StripAnsi(c.richStatusLine(80, time.Now()))
			if !strings.HasPrefix(line, "✔ ") || !strings.Contains(line, FormatToolDuration(2500*time.Millisecond)) {
				t.Fatalf("success status: %q", line)
			}
			c.SetResult("failed", true, time.Second)
			if line := widthx.StripAnsi(c.richStatusLine(80, time.Now())); !strings.HasPrefix(line, "✘ ") {
				t.Fatalf("error status: %q", line)
			}
			aborted := NewToolExecutionComponent(name, "call", json.RawMessage(`{}`), ToolExecutionOptions{Rich: true}, nil, nil, "")
			aborted.MarkExecutionStarted()
			aborted.FinalizeAborted(time.Second)
			if line := widthx.StripAnsi(aborted.richStatusLine(80, time.Now())); !strings.HasPrefix(line, "✘ ") {
				t.Fatalf("abort status: %q", line)
			}
		})
	}
}

// TestRichBashCard checks the shell card's Output section and its exit-code and
// duration facts, drawn on the status line as OMP's head does.
func TestRichBashCard(t *testing.T) {
	c := NewToolExecutionComponent("bash", "call", json.RawMessage(`{"command":"go test ./..."}`), ToolExecutionOptions{Rich: true}, nil, nil, "")
	c.MarkExecutionStarted()
	c.BodyRenderer = func(width int, expanded bool) []string { return []string{"ok", "PASS"} }
	code := 0
	c.SetExitCode(&code)
	c.SetResult("ok\nPASS", false, time.Second)
	out := widthx.StripAnsi(strings.Join(c.Render(70), "\n"))
	for _, want := range []string{"✔ Bash", "✔ Bash", "· exit 0 ·", "├─── Output ", "$ go test ./...", "ok"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	for _, row := range strings.Split(out, "\n") {
		if strings.Contains(row, " Output ") && (!strings.HasPrefix(row, "├") || !strings.HasSuffix(row, "┤")) {
			t.Fatalf("divider does not span the frame: %q", row)
		}
	}
}

// TestRichInlineTools checks that every non-shell tool draws its body directly
// under the status line, with no labelled Output section.
func TestRichInlineTools(t *testing.T) {
	for _, name := range []string{"read", "write", "edit", "grep", "find", "ls", "web_search", "web_fetch", "custom_tool"} {
		t.Run(name, func(t *testing.T) {
			c := NewToolExecutionComponent(name, "call", json.RawMessage(`{"path":"a.go"}`), ToolExecutionOptions{Rich: true}, nil, nil, "")
			c.MarkExecutionStarted()
			c.BodyRenderer = func(width int, expanded bool) []string { return []string{"unique-body"} }
			c.SetResult("unique-body", false, time.Second)
			out := widthx.StripAnsi(strings.Join(c.Render(70), "\n"))
			if !strings.Contains(out, "unique-body") {
				t.Fatalf("body missing: %s", out)
			}
			if strings.Contains(out, "Output") {
				t.Fatalf("inline tool drew an Output section: %s", out)
			}
			top := strings.Split(strings.TrimSpace(out), "\n")[0]
			if !strings.HasPrefix(top, "╭") || !strings.HasSuffix(top, "╮") {
				t.Fatalf("bad top bar: %s", out)
			}
		})
	}
}

// TestRichToolWidths exercises ANSI, wide text, self-renderers and narrow panes.
func TestRichToolWidths(t *testing.T) {
	for _, self := range []bool{false, true} {
		c := NewToolExecutionComponent("edit", "call", nil, ToolExecutionOptions{Rich: true}, &ToolDefinitionRenderers{
			Self: self,
			Call: func(ToolRenderInput) (Component, bool) {
				return NewPaddedText("\x1b[32m界界 long tool content\x1b[0m", 0, 0, nil), true
			},
		}, nil, "")
		c.MarkExecutionStarted()
		c.FinalizeAborted(time.Second)
		for _, width := range []int{1, 3, 9, 12, 40, 100} {
			rows := c.Render(width)
			for _, row := range rows {
				if got := widthx.VisibleWidth(row); got > width {
					t.Fatalf("self=%v width=%d got=%d row=%q", self, width, got, row)
				}
			}
			if width < richMinWidth && strings.Contains(strings.Join(rows, "\n"), "╭") {
				t.Fatalf("frame in a %d-cell pane", width)
			}
		}
	}
}
