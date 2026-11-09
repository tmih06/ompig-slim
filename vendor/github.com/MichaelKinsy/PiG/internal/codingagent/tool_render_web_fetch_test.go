package codingagent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/codingagent/tools"
	"github.com/MichaelKinsy/PiG/tui"
)

// webFetchBody is a URL read result as `read` returns it: a header line, a
// title line, and body lines, the whole rendering the model sees.
func webFetchBody(lines int) string {
	var builder strings.Builder
	builder.WriteString("Fetched https://example.com/page (html-native, text/markdown, ")
	builder.WriteString(strings.Repeat("x", 40))
	builder.WriteString(" lines)")
	builder.WriteString("\n\nTitle: Example page")
	for i := range lines {
		builder.WriteString("\nbody line ")
		builder.WriteString(string(rune('a' + i%26)))
		builder.WriteString(strings.Repeat(" word", 6))
	}
	return builder.String()
}

// webFetchCard returns a completed URL-read card for body, wired the way
// InteractiveMode wires one: the per-tool body renderer, then the result.
// `read` owns URL fetching, so the card is a read card whose details mark the
// source as a URL.
func webFetchCard(t *testing.T, body string) *tui.ToolExecutionComponent {
	t.Helper()
	result := agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: body}},
		Details: &tools.ReadURLDetails{Kind: "url", URL: "https://example.com/page"},
	}
	call := ai.ToolCall{Name: "read", Arguments: map[string]any{"path": "https://example.com/page"}}
	comp := tui.NewToolExecutionComponent("read", "call_1", json.RawMessage(`{"path":"https://example.com/page"}`), tui.ToolExecutionOptions{}, nil, nil, "")
	comp.BodyRenderer = toolBodyRendererForCall(call, result, nil)
	comp.UpdateResult(tui.ToolResultUpdate{
		Content: []tui.ToolResultContent{{Type: "text", Text: body}},
		Result:  result,
	}, false)
	return comp
}

// TestWebFetchCardStartsCollapsed is the regression this renderer exists for: a
// fetch returns up to 300 lines, and a card without a body renderer keeps
// upstream's "no definition" default of expanded, so the whole page lands in the
// transcript and the user scrolls past all of it to get back to the chat.
func TestWebFetchCardStartsCollapsed(t *testing.T) {
	comp := webFetchCard(t, webFetchBody(200))
	if !comp.Collapsed {
		t.Fatal("a long URL-read card started expanded")
	}
	const width = 80
	if rows := len(comp.Render(width)); rows > 20 {
		t.Fatalf("collapsed card drew %d rows at width %d, want a short preview", rows, width)
	}
}

// TestWebFetchCardPreviewShowsHeadAndHint checks the collapsed preview and that
// ctrl+o reveals the rest: the head of the page, a count of the hidden lines,
// and every line once expanded.
func TestWebFetchCardPreviewShowsHeadAndHint(t *testing.T) {
	body := webFetchBody(200)
	comp := webFetchCard(t, body)
	const width = 80

	collapsed := strings.Join(comp.Render(width), "\n")
	if !strings.Contains(collapsed, "Fetched https://example.com/page") {
		t.Error("collapsed preview dropped the fetch header")
	}
	if !strings.Contains(collapsed, "Title: Example page") {
		t.Error("collapsed preview dropped the page title")
	}
	if !strings.Contains(collapsed, "more lines") || !strings.Contains(collapsed, "to expand") {
		t.Errorf("collapsed preview has no expand hint:\n%s", collapsed)
	}

	comp.Toggle()
	expanded := comp.Render(width)
	if joined := strings.Join(expanded, "\n"); strings.Contains(joined, "more lines") {
		t.Error("expanded card still shows the collapse hint")
	}
	// The expanded card draws every logical line of the body, wrapped.
	if len(expanded) < strings.Count(body, "\n") {
		t.Fatalf("expanded card drew %d rows for %d logical lines", len(expanded), strings.Count(body, "\n")+1)
	}
}

// TestWebFetchCardPreviewCountsVisualLines keeps one long line from filling the
// screen: the budget is visual rows, so a single minified line is cut too.
func TestWebFetchCardPreviewCountsVisualLines(t *testing.T) {
	comp := webFetchCard(t, "Fetched https://example.com/min.js (raw, 1 lines)\n\n"+strings.Repeat("var a=1;", 400))
	const width = 80
	if rows := len(comp.Render(width)); rows > 20 {
		t.Fatalf("collapsed card drew %d rows for one wrapped line at width %d", rows, width)
	}
}

// TestWebFetchCardErrorStaysVisible checks that a failed fetch is not collapsed
// to a preview: the error text is one line and is the reason to look at the card.
func TestWebFetchCardErrorStaysVisible(t *testing.T) {
	comp := webFetchCard(t, "Error: fetch https://example.com/page: connection refused")
	comp.SetResult("Error: fetch https://example.com/page: connection refused", true, 0)
	const width = 80
	joined := strings.Join(comp.Render(width), "\n")
	if !strings.Contains(joined, "connection refused") {
		t.Errorf("collapsed error card hid its message:\n%s", joined)
	}
}

// TestWebFetchCardFollowsThemeSwitch covers the invariant the renderer's cache
// rests on: the styled rows are baked from the active theme and reused every
// frame, so a theme switch must rebuild them instead of leaving the old colors.
func TestWebFetchCardFollowsThemeSwitch(t *testing.T) {
	dark, light := tui.ThemeByName("dark"), tui.ThemeByName("light")
	if dark == nil || light == nil || dark.ToolOutput == light.ToolOutput {
		t.Skip("the registered themes do not differ in toolOutput")
	}
	t.Cleanup(func() { tui.SetThemeInstance(dark) })

	tui.SetThemeInstance(dark)
	comp := webFetchCard(t, webFetchBody(40))
	const width = 80
	if joined := strings.Join(comp.Render(width), "\n"); !strings.Contains(joined, dark.ToolOutput) {
		t.Fatal("the card did not draw in the active theme")
	}

	tui.SetThemeInstance(light)
	comp.Invalidate()
	joined := strings.Join(comp.Render(width), "\n")
	if !strings.Contains(joined, light.ToolOutput) {
		t.Error("the card kept the previous theme's colors after a theme switch")
	}
	if strings.Contains(joined, dark.ToolOutput) {
		t.Error("the card still carries colors from the previous theme")
	}
}
