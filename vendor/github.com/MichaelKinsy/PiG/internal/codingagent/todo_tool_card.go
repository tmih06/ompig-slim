package codingagent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// todoTranscriptRenderers turns the ordinary inline tool shell into a compact
// action receipt: lifecycle icon, operation/target, counts and recorded timing.
// The full list stays exclusively in the widget, including after Ctrl+O. Only
// errors disclose a small diagnostic body. This presentation belongs to the
// interactive host, not the builtin's reusable full-list renderer.
func todoTranscriptRenderers(card *tui.ToolExecutionComponent) *tui.ToolDefinitionRenderers {
	return &tui.ToolDefinitionRenderers{
		Call: func(tui.ToolRenderInput) (tui.Component, bool) {
			return tui.NewPaddedText("", 0, 0, nil), true
		},
		Result: func(input tui.ToolRenderInput) (tui.Component, bool) {
			if input.IsError && input.Expanded {
				return todoCardError{message: card.Output}, true
			}
			return tui.NewPaddedText("", 0, 0, nil), true
		},
	}
}

// todoCardMeta reads only presentation counts from authoritative result details.
// JSON accepts both the native builtin value and a resumed wire snapshot without
// importing the builtin package (which imports codingagent). Error results must
// not claim successful counts; their condensed diagnostic stays visible when
// closed.
func todoCardMeta(result agent.AgentToolResult) []string {
	if result.IsError {
		return []string{widthx.TruncateToWidth(todoErrorSummary(result.Text()), 48, "…", false)}
	}
	var details struct {
		Phases *[]struct {
			Tasks []struct {
				Status string `json:"status"`
			} `json:"tasks"`
		} `json:"phases"`
	}
	raw, err := json.Marshal(result.Details)
	if err != nil || json.Unmarshal(raw, &details) != nil || details.Phases == nil {
		return nil
	}
	total, done, blocked, dropped := 0, 0, 0, 0
	for _, phase := range *details.Phases {
		for _, task := range phase.Tasks {
			total++
			switch task.Status {
			case "completed":
				done++
			case "blocked":
				blocked++
			case "abandoned":
				dropped++
			}
		}
	}
	if total == 0 {
		return []string{"no tasks"}
	}
	meta := []string{fmt.Sprintf("%d/%d done", done, total)}
	if blocked > 0 {
		meta = append(meta, fmt.Sprintf("%d blocked", blocked))
	}
	if dropped > 0 {
		meta = append(meta, fmt.Sprintf("%d dropped", dropped))
	}
	return meta
}

// todoCardError bounds expanded diagnostics to six terminal rows, truncating
// each line in cells. Validation errors may include huge argument objects; an
// expanded receipt must not turn them into another transcript-sized task dump.
type todoCardError struct{ message string }

// Render returns local themed diagnostics without mutating the persisted result.
func (e todoCardError) Render(width int) []string {
	var rows []string
	for _, line := range todoErrorLines(e.message) {
		rows = append(rows, tui.ActiveTheme().Fg(string(tui.ThemeColorError), widthx.TruncateToWidth(line, max(1, width), "…", false)))
		if len(rows) == 6 {
			break
		}
	}
	return rows
}

// todoArgumentsEcho marks the line the harness puts before a rejected call's
// arguments: ai.validateToolArgsSchema and ai.parseToolCallArguments both end
// their message with it, then the payload pretty-printed or verbatim. Everything
// from it on is a copy of what the model sent, so a receipt or a widget row
// spends no space repeating it.
const todoArgumentsEcho = "Received arguments:"

// todoErrorLines returns the diagnostic lines of a failed result, in order, with
// the argument echo removed. Blank lines and ANSI styling are dropped because
// every caller re-styles the text in its own token.
func todoErrorLines(message string) []string {
	var lines []string
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(widthx.StripAnsi(line))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, todoArgumentsEcho) {
			break
		}
		lines = append(lines, line)
	}
	return lines
}

// todoErrorSummary condenses a failed result to the one line a closed receipt or
// a Tasks widget row can hold.
//
// A schema rejection reads `Validation failed for tool "todo":` followed by one
// `- <failure>` bullet per problem, so the header carries no information and the
// first bullet is the reason. The header is recognized by its bullet, not by its
// wording: a tool-level failure whose first line is its own diagnostic (for
// example `Errors: Missing phase name for append operation`) keeps that line.
// Returns "failed" for a result with no text at all, so a row always renders.
func todoErrorSummary(message string) string {
	lines := todoErrorLines(message)
	if len(lines) == 0 {
		return "failed"
	}
	first := lines[0]
	if len(lines) > 1 && strings.HasSuffix(first, ":") && strings.HasPrefix(lines[1], "- ") {
		first = lines[1]
	}
	return strings.TrimPrefix(first, "- ")
}

// Invalidate has no cached rendering to discard.
func (todoCardError) Invalidate() {}
