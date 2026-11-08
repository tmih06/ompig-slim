package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TestRichToolLifecycle checks all tools share timing without losing tool identity.
func TestRichToolLifecycle(t *testing.T) {
	for _, name := range []string{"bash", "read", "write", "edit", "grep", "find", "ls", "web_search", "web_fetch", "codemode", "Agent", "custom_tool"} {
		t.Run(name, func(t *testing.T) {
			c := NewToolExecutionComponent(name, "call", json.RawMessage(`{}`), ToolExecutionOptions{Rich: true}, nil, nil, "")
			if status, _ := c.richStatus(time.Now()); status != "Preparing" {
				t.Fatalf("before execution: %q", status)
			}
			c.MarkExecutionStarted()
			now := c.StartedAt.Add(1500 * time.Millisecond)
			status, _ := c.richStatus(now)
			if !strings.Contains(status, "Running") || !strings.Contains(status, FormatToolDuration(1500*time.Millisecond)) || !c.IsDirty() {
				t.Fatalf("running status: %q", status)
			}
			c.SetResult("ok", false, time.Second)
			duration := int64(2500)
			c.DurationMs = &duration
			status, _ = c.richStatus(now)
			if !strings.Contains(status, "Done") || !strings.Contains(status, FormatToolDuration(2500*time.Millisecond)) {
				t.Fatalf("recorded duration: %q", status)
			}
			c.SetResult("failed", true, time.Second)
			if status, role := c.richStatus(now); !strings.Contains(status, "Error") || role != "error" {
				t.Fatalf("error status: %q, %q", status, role)
			}
		})
	}
}

// TestRichToolSections verifies input/result ordering for native, fallback, and
// self-rendered tools, including pending output and expansion transitions.
func TestRichToolSections(t *testing.T) {
	for _, mode := range []string{"legacy", "fallback", "definition", "self"} {
		t.Run(mode, func(t *testing.T) {
			name := "read"
			var definition *ToolDefinitionRenderers
			if mode == "fallback" {
				name = "custom"
			}
			if mode == "definition" || mode == "self" {
				definition = &ToolDefinitionRenderers{Self: mode == "self"}
			}
			c := NewToolExecutionComponent(name, "call", json.RawMessage(`{"path":"input.txt"}`), ToolExecutionOptions{Rich: true}, definition, nil, "")
			pending := widthx.StripAnsi(strings.Join(c.Render(80), "\n"))
			if !strings.Contains(pending, "Waiting for result") {
				t.Fatalf("missing pending placeholder: %s", pending)
			}
			c.SetResult("unique-result", false, time.Second)
			for _, expanded := range []bool{false, true} {
				c.SetExpanded(expanded)
				out := widthx.StripAnsi(strings.Join(c.Render(80), "\n"))
				input, result := strings.Index(out, "─ Input"), strings.Index(out, "─ Result")
				if input < 0 || result <= input || strings.Index(out, "input.txt") < input {
					t.Fatalf("bad section order: %s", out)
				}
				if index := strings.Index(out, "unique-result"); index >= 0 && index < result {
					t.Fatalf("result leaked into input: %s", out)
				}
			}
		})
	}
}

// TestRichToolWidths exercises ANSI, wide text, self-renderers, and narrow fallbacks.
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
		if status, role := c.richStatus(time.Now()); !strings.Contains(status, "Cancelled") || role != "warning" {
			t.Fatalf("abort status: %q, %q", status, role)
		}
		for _, width := range []int{1, 3, 5, 12, 40, 100} {
			rows := c.Render(width)
			for _, row := range rows {
				if got := widthx.VisibleWidth(row); got > width {
					t.Fatalf("self=%v width=%d got=%d row=%q", self, width, got, row)
				}
			}
			if width >= 5 && !strings.Contains(strings.Join(rows, "\n"), "╭") {
				t.Fatal("missing frame")
			}
		}
	}
}
