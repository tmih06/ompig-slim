package tui

import (
	"iter"
	"strings"

	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// IsKnownSkill reports whether a name has a registered slash invocation.
func (p *CombinedProvider) IsKnownSkill(name string) bool { return p.slash.IsKnownSkill(name) }

// IsKnownSkill reports whether a name has a registered slash invocation.
func (p *SlashOnlyProvider) IsKnownSkill(name string) bool {
	for _, command := range p.Commands {
		if command.commandName() == "skill:"+name {
			return true
		}
	}
	return false
}

// skillSpans recognizes only registered invocations in prompts, never shell or command arguments.
func (e *Editor) skillSpans(text string) []SkillToken {
	known, ok := e.autocomplete.(interface{ IsKnownSkill(string) bool })
	if !ok || !strings.Contains(text, "/skill:") || !AllowsSkillTokens(e.Text()) {
		return nil
	}
	var spans []SkillToken
	for _, span := range SkillTokens(text) {
		if known.IsKnownSkill(span.Name) {
			spans = append(spans, span)
		}
	}
	return spans
}

// segmentSkills merges known skill tokens into atomic edit/navigation segments.
// Source text stays unchanged so undo, history and submission retain slash syntax.
func (e *Editor) segmentSkills(text string, base iter.Seq[SegmentData]) iter.Seq[SegmentData] {
	spans := e.skillSpans(text)
	if len(spans) == 0 {
		return base
	}
	return func(yield func(SegmentData) bool) {
		i := 0
		for segment := range base {
			for i < len(spans) && jsstring.Length(text[:spans[i].End]) <= segment.Index {
				i++
			}
			if i < len(spans) {
				span := spans[i]
				start, end := jsstring.Length(text[:span.Start]), jsstring.Length(text[:span.End])
				if segment.Index >= start && segment.Index < end {
					if segment.Index == start && !yield(SegmentData{Segment: text[span.Start:span.End], Index: start, Input: text}) {
						return
					}
					continue
				}
			}
			if !yield(segment) {
				return
			}
		}
	}
}

// skillLabel is the compact display text, including one cell of pill padding per side.
func skillLabel(name string) string { return " ◆ " + name + " " }

// skillColumn maps a source UTF-16 column to its compact display column.
// Interior token positions snap to the start because skills are atomic.
func (e *Editor) skillColumn(text string, col int) int {
	removed := 0
	for _, span := range e.skillSpans(text) {
		start, end := jsstring.Length(text[:span.Start]), jsstring.Length(text[:span.End])
		if col < start {
			break
		}
		if col < end {
			return start - removed
		}
		removed += end - start - jsstring.Length(skillLabel(span.Name))
	}
	return col - removed
}

// skillSourceColumn maps a compact display column back to source UTF-16 units.
// A position inside a badge selects its start, matching mouse and arrow navigation.
func (e *Editor) skillSourceColumn(text string, col int) int {
	removed := 0
	for _, span := range e.skillSpans(text) {
		start, end := jsstring.Length(text[:span.Start]), jsstring.Length(text[:span.End])
		labelLength := jsstring.Length(skillLabel(span.Name))
		if col < start-removed {
			break
		}
		if col < start-removed+labelLength {
			return start
		}
		removed += end - start - labelLength
	}
	return col + removed
}

// skillVisibleWidth measures the same compact, atomic segments used by wrapping and mouse input.
func (e *Editor) skillVisibleWidth(text string) int {
	width := widthx.VisibleWidth(text)
	for _, span := range e.skillSpans(text) {
		width -= widthx.VisibleWidth(text[span.Start:span.End]) - widthx.VisibleWidth(skillLabel(span.Name))
	}
	return width
}

// skillDisplay renders compact badges while retaining source columns for cursor placement.
// No hidden-prefix padding is emitted; subsequent text follows the pill immediately.
func (e *Editor) skillDisplay(text string, col int, emit bool) string {
	spans := e.skillSpans(text)
	if len(spans) == 0 {
		if col < 0 {
			return text
		}
		return e.renderCursorAt(text, col, emit)
	}
	var out strings.Builder
	offset := 0
	// Each part owns its starting cursor position; the end-of-line cursor is emitted last.
	appendPart := func(part string, start int) {
		length := jsstring.Length(part)
		if col >= start && col < start+length {
			out.WriteString(e.renderCursorAt(part, col-start, emit))
		} else {
			out.WriteString(part)
		}
	}
	theme := ActiveTheme()
	for _, span := range spans {
		appendPart(text[offset:span.Start], jsstring.Length(text[:offset]))
		out.WriteString(theme.GetBgAnsi("customMessageBg") + theme.GetFgAnsi("customMessageLabel") + "\x1b[1m")
		label := skillLabel(span.Name)
		start, end := jsstring.Length(text[:span.Start]), jsstring.Length(text[:span.End])
		if col >= start && col < end {
			col = start
		}
		appendPart(label, start)
		out.WriteString("\x1b[22m\x1b[39m\x1b[49m")
		offset = span.End
	}
	appendPart(text[offset:], jsstring.Length(text[:offset]))
	if col >= jsstring.Length(text) {
		out.WriteString(e.renderCursorAt("", 0, emit))
	}
	return out.String()
}

// skillSlashContext keeps slash completion active after prose or a previous skill.
func (e *Editor) skillSlashContext(before string) bool {
	_, ok := trailingSkillSlash(before)
	return ok && AllowsSkillTokens(e.Text())
}
