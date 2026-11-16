package codingagent

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/subagents"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

func subagentThinkingBlock(thinking ai.ThinkingContent) string {
	if thinking.Redacted {
		return ""
	}
	return strings.TrimSpace(thinking.Thinking)
}

func subagentThinkingText(blocks []ai.AssistantContentBlock) string {
	var parts []string
	for _, block := range blocks {
		if thinking, ok := block.(ai.ThinkingContent); ok {
			if text := subagentThinkingBlock(thinking); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func subagentThinkingPreview(manager *subagents.Manager, id string) string {
	if manager == nil || id == "" {
		return ""
	}
	messages, err := manager.Transcript(id)
	if err != nil {
		return ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		var message agent.AgentMessage
		if json.Unmarshal(messages[i], &message) != nil || message.Assistant == nil {
			continue
		}
		return subagentThinkingText(message.Assistant.Content)
	}
	return ""
}

func subagentThinkingSanitize(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, widthx.StripTerminalSequences(text))
}

func subagentThinkingLine(text string, width int) string {
	text = strings.TrimSpace(hubSingleLine(text))
	if text == "" || width < 1 {
		return ""
	}
	if columns := widthx.VisibleWidth(text); columns > width {
		text = "…" + widthx.SliceByColumn(text, columns-width+1, width-1, false)
	}
	return tui.ActiveTheme().Fg("thinkingText", tui.ChalkItalic(text))
}

func subagentThinkingLines(text string, width, limit int) []string {
	text = strings.TrimSpace(subagentThinkingSanitize(text))
	if text == "" || width < 1 {
		return nil
	}
	theme := tui.ActiveTheme()
	var wrapped []string
	for _, line := range strings.Split(text, "\n") {
		wrapped = append(wrapped, widthx.WrapTextWithAnsi(line, width)...)
	}
	hidden := 0
	if limit >= 2 && len(wrapped) > limit {
		hidden = len(wrapped) - limit + 1
		wrapped = wrapped[hidden:]
	}
	out := make([]string, 0, len(wrapped)+1)
	if hidden > 0 {
		out = append(out, theme.Fg("dim", fmt.Sprintf("… %d earlier lines", hidden)))
	}
	for _, line := range wrapped {
		out = append(out, theme.Fg("thinkingText", tui.ChalkItalic(line)))
	}
	return out
}
