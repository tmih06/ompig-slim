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
// not claim successful counts; their first diagnostic stays visible when closed.
func todoCardMeta(result agent.AgentToolResult) []string {
	if result.IsError {
		for _, line := range strings.Split(result.Text(), "\n") {
			if line = strings.TrimSpace(widthx.StripAnsi(line)); line != "" {
				return []string{widthx.TruncateToWidth(line, 48, "…", false)}
			}
		}
		return []string{"failed"}
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
	for _, line := range strings.Split(e.message, "\n") {
		line = strings.TrimSpace(widthx.StripAnsi(line))
		if line == "" {
			continue
		}
		rows = append(rows, tui.ActiveTheme().Fg("error", widthx.TruncateToWidth(line, max(1, width), "…", false)))
		if len(rows) == 6 {
			break
		}
	}
	return rows
}

// Invalidate has no cached rendering to discard.
func (todoCardError) Invalidate() {}
