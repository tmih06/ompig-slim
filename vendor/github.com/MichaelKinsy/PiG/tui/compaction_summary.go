package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/MichaelKinsy/PiG/agent"
)

// customMsgLabelFg is the foreground color for the [compaction] label.
// It is the active theme's customMessageLabel token.
// Fg colors do not need the delta-from-cardBg adjustment (only bg tints do).
func customMsgLabelFg() string { return ActiveTheme().CustomMessageLabel }

// CompactionSummaryMessageComponent renders a collapsible compaction marker. Like upstream's `extends Box`, it embeds the
// Box (padding 1, 1 and the customMessageBg background), so the Box members are inherited; each display update clears the
// Box and adds one mouse region.
//
// upstream: packages/coding-agent/src/modes/interactive/components/compaction-summary-message.ts
type CompactionSummaryMessageComponent struct {
	*Box
	summary       string
	tokensBefore  int
	info          *agent.CompactionInfo
	expanded      bool
	markdownTheme *MarkdownTheme
}

// CompactionSummaryMessage is the part of Pi's CompactionSummaryMessage (coding-agent core/messages.ts:62) that
// [CompactionSummaryMessageComponent] renders: the summary markdown and the context token count before compaction.
type CompactionSummaryMessage struct {
	Summary      string
	TokensBefore int
	// Info is the host-observed record of the compaction that wrote the
	// summary; nil for entries without one.
	Info *agent.CompactionInfo
}

// NewCompactionSummaryMessageComponent is Pi's constructor(message, markdownTheme = getMarkdownTheme(), outputPad = 1)
// (compaction-summary-message.ts:15). The expanded body renders the summary with markdownTheme; nil selects the active
// theme's markdown theme. outputPad is the horizontal padding; absent it is 1.
func NewCompactionSummaryMessageComponent(message CompactionSummaryMessage, markdownTheme *MarkdownTheme, outputPad ...int) *CompactionSummaryMessageComponent {
	padX := 1
	if len(outputPad) > 0 {
		padX = outputPad[0]
	}
	component := &CompactionSummaryMessageComponent{
		Box:           NewPaddedBox(padX, 1, func(text string) string { return ActiveTheme().Bg("customMessageBg", text) }),
		summary:       message.Summary,
		tokensBefore:  message.TokensBefore,
		info:          message.Info,
		markdownTheme: markdownTheme,
	}
	component.updateDisplay()
	return component
}

// SetExpanded opens or collapses the component body. InteractiveMode calls it from the global Ctrl+O toggle.
func (c *CompactionSummaryMessageComponent) SetExpanded(expanded bool) {
	c.expanded = expanded
	c.updateDisplay()
}

// Invalidate invalidates the Box and rebuilds the display, which bakes theme colors into its text.
func (c *CompactionSummaryMessageComponent) Invalidate() {
	c.Box.Invalidate()
	c.updateDisplay()
}

func (c *CompactionSummaryMessageComponent) updateDisplay() {
	c.Clear()
	theme := ActiveTheme()
	content := NewContainer()
	tokenStr := formatThousands(c.tokensBefore)
	content.Add(NewText(theme.Fg("customMessageLabel", "\x1b[1m[compaction]\x1b[22m")))
	content.Add(NewSpacer(1))
	if c.expanded {
		header := "**Compacted from " + tokenStr + " tokens**"
		content.Add(finishedMarkdown(NewMarkdownWithOptions(header, 0, 0, c.markdownTheme, &DefaultTextStyle{
			Color: func(text string) string { return ActiveTheme().Fg("customMessageText", text) },
		}, nil)))
		if info := compactionInfoText(c.info); info != "" {
			content.Add(NewText(theme.Fg("dim", info)))
		}
		content.Add(finishedMarkdown(NewMarkdownWithOptions(c.summary, 0, 0, c.markdownTheme, &DefaultTextStyle{
			Color: func(text string) string { return ActiveTheme().Fg("customMessageText", text) },
		}, nil)))
	} else {
		content.Add(NewText(theme.Fg("customMessageText", "Compacted from "+tokenStr+" tokens (") +
			theme.Fg("dim", AppKeyText("app.tools.expand", "ctrl+o")) +
			theme.Fg("customMessageText", " to expand)")))
		if info := compactionInfoText(c.info); info != "" {
			content.Add(NewText(theme.Fg("dim", info)))
		}
	}
	c.AddChild(NewMouseRegion(content, func(event TuiMouseEvent) *TuiMouseEventResult {
		if event.Type != MouseClick || event.Button != MouseButtonLeft {
			return nil
		}
		c.SetExpanded(!c.expanded)
		return &TuiMouseEventResult{Handled: true}
	}))
}

// compactionInfoText is the one-line provenance of a completed compaction:
// its reason, the distinct model identities that produced the summary, and
// the operation's elapsed time. An absent record or empty members leave them
// unnamed rather than guessed.
func compactionInfoText(info *agent.CompactionInfo) string {
	if info == nil {
		return ""
	}
	reason := info.Reason
	if reason == "" {
		reason = "unknown"
	}
	models := "model unknown"
	if len(info.Models) > 0 {
		models = strings.Join(info.Models, ", ")
	}
	elapsed := ""
	if info.ElapsedMs >= 0 {
		elapsed = compactionElapsedText(info.ElapsedMs)
	}
	if elapsed == "" {
		return reason + " · " + models
	}
	return reason + " · " + models + " · " + elapsed
}

// compactionElapsedText renders a nonnegative millisecond duration as the
// card's compact "Ns" or "M:SS" label.
func compactionElapsedText(elapsedMs int64) string {
	if elapsedMs < 60_000 {
		text := fmt.Sprintf("%.1fs", float64(elapsedMs)/1000)
		if strings.HasSuffix(text, ".0s") {
			text = strings.TrimSuffix(text, ".0s") + "s"
		}
		return text
	}
	total := elapsedMs / 1000
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

// formatThousands is Number.prototype.toLocaleString for an integer: comma thousands separators and a leading minus sign, e.g. 87432 → "87,432" and -1234 → "-1,234".
func formatThousands(n int) string {
	s := strconv.Itoa(n)
	sign := ""
	if n < 0 {
		sign, s = "-", s[1:]
	}
	if len(s) <= 3 {
		return sign + s
	}
	var b strings.Builder
	b.Grow(len(sign) + len(s) + (len(s)-1)/3)
	b.WriteString(sign)
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	return b.String()
}

// SetOutputPad is Pi's setOutputPad(outputPad): the horizontal padding of the Box is the outputPad setting.
func (c *CompactionSummaryMessageComponent) SetOutputPad(outputPad int) {
	c.SetPaddingX(outputPad)
}
