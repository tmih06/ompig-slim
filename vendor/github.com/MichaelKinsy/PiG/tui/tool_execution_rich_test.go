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

// TestRichInlineTools checks OMP's inline tools: the collapsed card is the
// status-line row alone, with no frame, and the body appears under it.
func TestRichInlineTools(t *testing.T) {
	for _, name := range []string{"read", "grep", "find", "ls"} {
		t.Run(name, func(t *testing.T) {
			c := NewToolExecutionComponent(name, "call", json.RawMessage(`{"path":"a.go"}`), ToolExecutionOptions{Rich: true}, nil, nil, "")
			c.MarkExecutionStarted()
			c.BodyRenderer = func(width int, expanded bool) []string { return []string{"unique-body"} }
			c.SetResult("unique-body", false, time.Second)

			collapsed := widthx.StripAnsi(strings.Join(c.Render(70), "\n"))
			if strings.ContainsAny(collapsed, "╭│╰") {
				t.Fatalf("inline tool drew a frame: %s", collapsed)
			}
			if !strings.Contains(collapsed, "unique-body") || !strings.Contains(collapsed, "✔ "+richToolName(name)) {
				t.Fatalf("inline card is missing its row or body: %s", collapsed)
			}
			c.SetExpanded(true)
			expanded := widthx.StripAnsi(strings.Join(c.Render(70), "\n"))
			if strings.ContainsAny(expanded, "╭│╰") {
				t.Fatalf("expanded inline tool drew a frame: %s", expanded)
			}
		})
	}
}

// TestRichFramedTools checks that the stream tools keep OMP's output block and
// that their every row is the card width.
func TestRichFramedTools(t *testing.T) {
	// write and edit return framed boxes in oh-my-pi (write.ts, edit.ts); the
	// rest are either framed streams or unknown tools on the generic card.
	for _, name := range []string{"bash", "web_search", "codemode", "write", "edit", "custom_tool"} {
		t.Run(name, func(t *testing.T) {
			c := NewToolExecutionComponent(name, "call", json.RawMessage(`{}`), ToolExecutionOptions{Rich: true}, nil, nil, "")
			c.MarkExecutionStarted()
			c.BodyRenderer = func(width int, expanded bool) []string { return []string{"unique-body"} }
			c.SetResult("unique-body", false, time.Second)
			rows := c.Render(70)
			for len(rows) > 0 && strings.TrimSpace(widthx.StripAnsi(rows[0])) == "" {
				rows = rows[1:]
			}
			out := widthx.StripAnsi(strings.Join(rows, "\n"))
			if !strings.Contains(out, "unique-body") || !strings.Contains(out, richToolName(name)) {
				t.Fatalf("framed card is missing its title or body: %s", out)
			}
			for _, row := range rows {
				if got := widthx.VisibleWidth(row); got != 70 {
					t.Fatalf("%s: row is %d cells: %q", name, got, row)
				}
			}
		})
	}
}

// TestRichStatusSubjects checks each tool's subject on the top bar, following
// oh-my-pi's per-renderer heads: a search names its query, grep its pattern and
// scope, read its path and range, find its quoted query.
func TestRichStatusSubjects(t *testing.T) {
	for _, test := range []struct {
		name, args, want string
	}{
		{"web_search", `{"query":"BTC price"}`, "Web Search: BTC price"},
		{"grep", `{"pattern":"frontmatter|^name:","path":"skills"}`, "Grep: frontmatter|^name: in skills"},
		{"find", `{"pattern":"**/*.go","path":"src"}`, "Find: **/*.go in src"},
		{"read", `{"path":"skills/optimize/SKILL.md","offset":70,"limit":26}`, "Read skills/optimize/SKILL.md:70-95"},
		{"read", `{"path":"skills/optimize/SKILL.md"}`, "Read skills/optimize/SKILL.md"},
		{"write", `{"path":"a.txt"}`, "Write a.txt"},
		{"edit", `{"path":"main.go"}`, "Edit main.go"},
	} {
		t.Run(test.name+"/"+test.want, func(t *testing.T) {
			c := NewToolExecutionComponent(test.name, "call", json.RawMessage(test.args), ToolExecutionOptions{Rich: true}, nil, nil, "")
			out := widthx.StripAnsi(strings.Join(c.Render(80), "\n"))
			if !strings.Contains(out, test.want) {
				t.Fatalf("missing %q:\n%s", test.want, out)
			}
		})
	}
}

// TestRichStreamedSubject checks that the top bar names the subject while the
// arguments are still arriving, not only once they are complete.
func TestRichStreamedSubject(t *testing.T) {
	c := NewToolExecutionComponent("web_search", "call", nil, ToolExecutionOptions{Rich: true}, nil, nil, "")
	c.SetStreamedArgs(streamedTestArgs{fields: map[string]any{"query": "BTC price"}})
	if out := widthx.StripAnsi(strings.Join(c.Render(80), "\n")); !strings.Contains(out, "Web Search: BTC price") {
		t.Fatalf("streamed subject missing:\n%s", out)
	}
}

// streamedTestArgs is the streaming parser's view of partial arguments.
type streamedTestArgs struct{ fields map[string]any }

func (s streamedTestArgs) Object() map[string]any { return s.fields }
func (s streamedTestArgs) JSON() json.RawMessage {
	raw, _ := json.Marshal(s.fields)
	return raw
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

// TestRichFrameGeometry checks every row is exactly the card width and that the
// background tint covers border cells, at several widths and content shapes.
func TestRichFrameGeometry(t *testing.T) {
	bodies := map[string][]string{
		"plain":    {"short", "a longer body row that has to be truncated somewhere near the end of the line"},
		"wide":     {"界界界 wide text", "code: if (x) { return 1; }"},
		"styled":   {"\x1b[31mred\x1b[0m", "\x1b[1mbold\x1b[0m text"},
		"empty":    {},
		"trailing": {"line with trailing spaces   "},
	}
	for name, body := range bodies {
		for _, width := range []int{richMinWidth, 11, 20, 40, 80, 121} {
			c := NewToolExecutionComponent("bash", "call", json.RawMessage(`{"command":"x"}`), ToolExecutionOptions{Rich: true}, nil, nil, "")
			c.BodyRenderer = func(int, bool) []string { return body }
			c.SetResult(strings.Join(body, "\n"), false, time.Second)
			rows := c.Render(width)
			bg := c.richBgOpen()
			// Render opens with the card's spacer row, which is not part of the frame.
			for len(rows) > 0 && strings.TrimSpace(widthx.StripAnsi(rows[0])) == "" {
				rows = rows[1:]
			}
			if len(rows) == 0 {
				t.Fatalf("%s width=%d: no frame rows", name, width)
			}
			for _, row := range rows {
				if got := widthx.VisibleWidth(row); got != width {
					t.Fatalf("%s width=%d: row is %d cells: %q", name, width, got, row)
				}
				if bg != "" && !strings.HasPrefix(row, bg) {
					t.Fatalf("%s width=%d: row is not tinted: %q", name, width, row)
				}
				if bg != "" && !strings.Contains(row, SGRBgReset) {
					t.Fatalf("%s width=%d: tint is not closed: %q", name, width, row)
				}
			}
		}
	}
}

// TestRichMouseToggle checks a left click anywhere on a rich card expands and
// collapses it, whether the card is framed or inline, and that a click outside
// the borders does nothing. This is the regression the frame's row offsets can
// silently break.
func TestRichMouseToggle(t *testing.T) {
	for _, name := range []string{"bash", "read", "grep", "write", "edit"} {
		t.Run(name, func(t *testing.T) {
			c := NewToolExecutionComponent(name, "call", json.RawMessage(`{"command":"ls","path":"a.go"}`), ToolExecutionOptions{Rich: true}, nil, nil, "")
			// A collapsed body draws one row and an expanded one draws three, so
			// the rendered height proves which state the card is in.
			c.BodyRenderer = func(width int, expanded bool) []string {
				if expanded {
					return []string{"body one", "body two", "body three"}
				}
				return []string{"body one"}
			}
			if richOutputSection(name) {
				c.Output = ""
			}
			c.SetResult("body", false, time.Second)
			collapsedRows := len(c.Render(60))

			click := func(x, y int) bool {
				return c.HandleMouse(TuiMouseEvent{Type: MouseClick, Button: MouseButtonLeft, X: x, Y: y, Width: 60}) != nil
			}
			if !click(2, 0) || c.Collapsed {
				t.Fatalf("%s: clicking the status row did not expand", name)
			}
			expandedRows := len(c.Render(60))
			if expandedRows <= collapsedRows {
				t.Fatalf("%s: expanding drew no extra rows (%d -> %d)", name, collapsedRows, expandedRows)
			}
			// A body row toggles back.
			if !click(3, min(1, expandedRows-1)) || !c.Collapsed {
				t.Fatalf("%s: clicking the body did not collapse", name)
			}
			// Outside the card is not a click on it.
			if click(0, 0) && c.Collapsed {
				if c.Rich && !richInline(name) {
					t.Fatalf("%s: a click on the border toggled the card", name)
				}
			}
		})
	}
}

// TestRichMouseToggleSurvivesResize checks the recorded geometry is refreshed
// for a new width before the click is mapped.
func TestRichMouseToggleSurvivesResize(t *testing.T) {
	c := NewToolExecutionComponent("grep", "call", json.RawMessage(`{"pattern":"x"}`), ToolExecutionOptions{Rich: true}, nil, nil, "")
	c.BodyRenderer = func(width int, expanded bool) []string { return []string{"body"} }
	c.SetResult("body", false, time.Second)
	for _, width := range []int{40, 120, 60} {
		c.Render(width)
		if c.HandleMouse(TuiMouseEvent{Type: MouseClick, Button: MouseButtonLeft, X: 2, Y: 0, Width: width}) == nil {
			t.Fatalf("width %d: click did not expand", width)
		}
		if c.Collapsed {
			t.Fatalf("width %d: card stayed collapsed", width)
		}
		c.SetExpanded(false)
	}
}
