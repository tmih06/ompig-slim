package ai

// Ports @oh-my-pi/pi-ai src/utils/stream-markup-healing.ts for the "thinking" pattern and its
// src/dialect/thinking.ts ThinkingInbandScanner: leaked reasoning markup in a provider's visible text
// channel is healed back into thinking deltas, so a model that streams ` thinking` fences instead of
// structured thought parts still reaches the transcript as thinking.

import (
	"strings"
)

// ccaHealKind is the kind of one healing event.
type ccaHealKind int

const (
	ccaHealText ccaHealKind = iota
	ccaHealThinkingDelta
)

// ccaHealEvent is one healed event: visible text or a thinking delta.
type ccaHealEvent struct {
	kind ccaHealKind
	text string
}

// ccaThinkingTag is one dialect's in-band thinking section.
type ccaThinkingTag struct {
	open    string
	close   string
	fenced  bool
	implied bool
}

// ccaThinkingTags are the in-band reasoning idioms the scanner heals, in the source's order.
var ccaThinkingTags = []ccaThinkingTag{
	{open: " thinking", close: "</think>", implied: true},
	{open: "<thinking>", close: "</thinking>"},
	{open: "<scratchpad>", close: "</scratchpad>"},
	{open: "```thinking\n", close: "```", fenced: true},
	{open: "[FILTERED]"},
	{open: "[FILTERED]assistant[FILTERED]analysis[FILTERED]", close: "[FILTERED]"},
	{open: "[FILTERED]analysis[FILTERED]", close: "[FILTERED]"},
}

// ccaHealDedelimiters is every delimiter a hold may be waiting on.
func ccaHealDelimiters() []string {
	delimiters := make([]string, 0, len(ccaThinkingTags)*2)
	for _, tag := range ccaThinkingTags {
		delimiters = append(delimiters, tag.open)
		if tag.close != "" && tag.implied {
			delimiters = append(delimiters, tag.close)
		}
	}
	return delimiters
}

// ccaHealMaxDelimiterLength is the longest delimiter, which bounds a held prefix.
func ccaHealMaxDelimiterLength() int {
	longest := 0
	for _, delimiter := range ccaHealDelimiters() {
		if len(delimiter) > longest {
			longest = len(delimiter)
		}
	}
	return longest
}

// ccaHealBoundaryLead is the set of characters a delimiter or a partial may start on, plus the backtick.
func ccaHealBoundaryLead() [256]bool {
	var lead [256]bool
	for _, delimiter := range ccaHealDelimiters() {
		if delimiter != "" {
			lead[delimiter[0]] = true
		}
	}
	lead['`'] = true
	return lead
}

// ccaThinkingHealer is ThinkingInbandScanner with impliedOpen off: markup outside code spans and fences is
// healed, a partial delimiter at the tail is held, and a fenced or inline code region suppresses healing.
type ccaThinkingHealer struct {
	buffer     string
	closeTag   string
	thinking   string
	fenced     bool
	codeTicks  int
	codeFenced bool
	lineIndent int
	lead       [256]bool
}

func newCCAThinkingHealer() *ccaThinkingHealer {
	return &ccaThinkingHealer{lineIndent: 0, lead: ccaHealBoundaryLead()}
}

// feed consumes a chunk and returns its healed events.
func (healer *ccaThinkingHealer) feed(text string) []ccaHealEvent {
	if text == "" {
		return nil
	}
	healer.buffer += text
	return healer.consume(false)
}

// flush releases everything held back.
func (healer *ccaThinkingHealer) flush() []ccaHealEvent {
	events := healer.consume(true)
	if healer.buffer == "" {
		return events
	}
	if healer.closeTag != "" {
		healer.appendThinking(healer.buffer, &events)
	} else {
		events = append(events, ccaHealEvent{kind: ccaHealText, text: healer.buffer})
	}
	healer.buffer = ""
	healer.closeTag = ""
	return events
}

func (healer *ccaThinkingHealer) appendThinking(delta string, events *[]ccaHealEvent) {
	if delta == "" {
		return
	}
	healer.thinking += delta
	*events = append(*events, ccaHealEvent{kind: ccaHealThinkingDelta, text: delta})
}

func (healer *ccaThinkingHealer) appendText(text string, events *[]ccaHealEvent) {
	if text == "" {
		return
	}
	*events = append(*events, ccaHealEvent{kind: ccaHealText, text: text})
	healer.lineIndent = ccaTrailingLineIndent(text, healer.lineIndent)
}

// consume runs the state machine until it holds or empties the buffer.
func (healer *ccaThinkingHealer) consume(final bool) []ccaHealEvent {
	events := []ccaHealEvent{}
	for {
		if healer.fenced {
			closed, thinking, rest := ccaFencedThinkingFeed(healer.buffer, final)
			healer.buffer = rest
			healer.appendThinking(thinking, &events)
			if closed || final {
				healer.thinking = ""
				healer.closeTag = ""
				healer.fenced = false
			}
			if healer.fenced {
				break
			}
			continue
		}
		if healer.buffer == "" {
			break
		}
		if healer.closeTag != "" {
			closeIndex := strings.Index(healer.buffer, healer.closeTag)
			if closeIndex < 0 {
				hold := 0
				if !final {
					hold = ccaPartialSuffixOverlap(healer.buffer, []string{healer.closeTag})
				}
				healer.appendThinking(healer.buffer[:len(healer.buffer)-hold], &events)
				healer.buffer = healer.buffer[len(healer.buffer)-hold:]
				break
			}
			healer.appendThinking(healer.buffer[:closeIndex], &events)
			healer.buffer = healer.buffer[closeIndex+len(healer.closeTag):]
			healer.thinking = ""
			healer.closeTag = ""
			continue
		}
		if healer.codeTicks > 0 {
			if healer.emitCode(final, &events) {
				continue
			}
			break
		}
		hit := healer.scanVisible(final)
		switch hit.kind {
		case ccaHitNone:
			healer.appendText(healer.buffer, &events)
			healer.buffer = ""
			return events
		case ccaHitHold:
			if hit.index > 0 {
				healer.appendText(healer.buffer[:hit.index], &events)
			}
			healer.buffer = healer.buffer[hit.index:]
			return events
		case ccaHitCode:
			if hit.index > 0 {
				healer.appendText(healer.buffer[:hit.index], &events)
			}
			ticks := hit.ticks
			fenced := ticks >= 3 && healer.lineIndent >= 0 && healer.lineIndent <= 3
			healer.appendText(healer.buffer[hit.index:hit.index+ticks], &events)
			healer.buffer = healer.buffer[hit.index+ticks:]
			healer.codeTicks = ticks
			healer.codeFenced = fenced
			continue
		case ccaHitTag:
			if hit.index > 0 {
				healer.appendText(healer.buffer[:hit.index], &events)
			}
			healer.buffer = healer.buffer[hit.index+len(hit.tag.open):]
			healer.closeTag = hit.tag.close
			healer.thinking = ""
			healer.fenced = hit.tag.fenced
			continue
		}
	}
	return events
}

// ccaVisibleHitKind is one scan outcome.
type ccaVisibleHitKind int

const (
	ccaHitNone ccaVisibleHitKind = iota
	ccaHitTag
	ccaHitCode
	ccaHitHold
)

// ccaVisibleHit is the earliest healing boundary in the idle buffer.
type ccaVisibleHit struct {
	kind  ccaVisibleHitKind
	index int
	tag   ccaThinkingTag
	ticks int
}

// scanVisible walks idle visible text for the earliest reasoning tag, code-span opener or held partial.
func (healer *ccaThinkingHealer) scanVisible(final bool) ccaVisibleHit {
	holdFrom := len(healer.buffer)
	if !final {
		holdFrom = len(healer.buffer) - ccaHealMaxDelimiterLength() + 1
	}
	for index := range len(healer.buffer) {
		code := healer.buffer[index]
		if !healer.lead[code] {
			continue
		}
		for _, tag := range ccaThinkingTags {
			if strings.HasPrefix(healer.buffer[index:], tag.open) {
				return ccaVisibleHit{kind: ccaHitTag, index: index, tag: tag}
			}
		}
		if index >= holdFrom {
			rest := healer.buffer[index:]
			for _, delimiter := range ccaHealDelimiters() {
				if len(delimiter) > len(rest) && strings.HasPrefix(delimiter, rest) {
					return ccaVisibleHit{kind: ccaHitHold, index: index}
				}
			}
		}
		if code == '`' {
			ticks := ccaBacktickRun(healer.buffer, index)
			if !final && index+ticks == len(healer.buffer) {
				return ccaVisibleHit{kind: ccaHitHold, index: index}
			}
			return ccaVisibleHit{kind: ccaHitCode, index: index, ticks: ticks}
		}
	}
	return ccaVisibleHit{kind: ccaHitNone}
}

// emitCode drains a Markdown code region without healing its contents. It returns true when the region
// closed and the caller should continue.
func (healer *ccaThinkingHealer) emitCode(final bool, events *[]ccaHealEvent) bool {
	if healer.codeFenced {
		end := ccaFindFenceCloseEnd(healer.buffer, healer.codeTicks, final)
		if end != -1 {
			healer.appendText(healer.buffer[:end], events)
			healer.buffer = healer.buffer[end:]
			healer.codeTicks = 0
			healer.codeFenced = false
			return true
		}
		if final {
			healer.appendText(healer.buffer, events)
			healer.buffer = ""
			healer.codeTicks = 0
			healer.codeFenced = false
			return false
		}
		if lastNewline := strings.LastIndexByte(healer.buffer, '\n'); lastNewline != -1 {
			healer.appendText(healer.buffer[:lastNewline+1], events)
			healer.buffer = healer.buffer[lastNewline+1:]
		}
		return false
	}
	closeIndex := ccaFindBacktickRun(healer.buffer, 0, healer.codeTicks)
	if closeIndex != -1 && (final || closeIndex+healer.codeTicks < len(healer.buffer)) {
		healer.appendText(healer.buffer[:closeIndex+healer.codeTicks], events)
		healer.buffer = healer.buffer[closeIndex+healer.codeTicks:]
		healer.codeTicks = 0
		return true
	}
	hold := 0
	if !final {
		hold = ccaTrailingBacktickRun(healer.buffer)
	}
	healer.appendText(healer.buffer[:len(healer.buffer)-hold], events)
	healer.buffer = healer.buffer[len(healer.buffer)-hold:]
	if final {
		healer.codeTicks = 0
	}
	return false
}

// ccaBacktickRun is the length of the backtick run at from.
func ccaBacktickRun(buffer string, from int) int {
	end := from
	for end < len(buffer) && buffer[end] == '`' {
		end++
	}
	return end - from
}

// ccaFindBacktickRun finds the first run of exactly ticks backticks at or after from.
func ccaFindBacktickRun(buffer string, from, ticks int) int {
	for index := strings.IndexByte(buffer[from:], '`'); index != -1; index = strings.IndexByte(buffer[from:], '`') {
		index += from
		run := ccaBacktickRun(buffer, index)
		if run == ticks {
			return index
		}
		from = index + run
	}
	return -1
}

// ccaTrailingBacktickRun is the length of the backtick run ending at the tail.
func ccaTrailingBacktickRun(buffer string) int {
	start := len(buffer)
	for start > 0 && buffer[start-1] == '`' {
		start--
	}
	return len(buffer) - start
}

// ccaTrailingLineIndent is the leading-space count of the last line of text.
func ccaTrailingLineIndent(text string, prior int) int {
	lastNewline := strings.LastIndexByte(text, '\n')
	indent := prior
	if lastNewline != -1 {
		indent = 0
	}
	for index := lastNewline + 1; index < len(text); index++ {
		if indent == -1 {
			break
		}
		if text[index] == ' ' {
			indent++
		} else {
			indent = -1
		}
	}
	return indent
}

// ccaFindFenceCloseEnd finds the end of the first closing fence line for a block opened with ticks.
func ccaFindFenceCloseEnd(buffer string, ticks int, final bool) int {
	for start := 0; start <= len(buffer); {
		newline := strings.IndexByte(buffer[start:], '\n')
		terminated := newline != -1
		line := buffer[start:]
		if terminated {
			line = buffer[start : start+newline]
		}
		line = strings.TrimSpace(line)
		if len(line) >= ticks && ccaIsAllBackticks(line) && (terminated || final) {
			if terminated {
				return start + newline + 1
			}
			return len(buffer)
		}
		if !terminated {
			break
		}
		start += newline + 1
	}
	return -1
}

func ccaIsAllBackticks(text string) bool {
	if text == "" {
		return false
	}
	for index := range len(text) {
		if text[index] != '`' {
			return false
		}
	}
	return true
}

// ccaPartialSuffixOverlap is the length of the longest suffix of text that is a proper prefix of one of the
// delimiters.
func ccaPartialSuffixOverlap(text string, delimiters []string) int {
	longest := 0
	for _, delimiter := range delimiters {
		limit := min(len(text), len(delimiter)-1)
		for length := limit; length > longest; length-- {
			if strings.HasSuffix(text, delimiter[:length]) {
				longest = length
				break
			}
		}
	}
	return longest
}

// ccaFencedThinkingFeed feeds the body of a ```thinking block. It returns whether the fence closed, the
// thinking delta to emit, and the rest of the buffer.
func ccaFencedThinkingFeed(buffer string, final bool) (closed bool, thinking, rest string) {
	for start := 0; start <= len(buffer); {
		newline := strings.IndexByte(buffer[start:], '\n')
		terminated := newline != -1
		line := buffer[start:]
		if terminated {
			line = buffer[start : start+newline]
		}
		trimmed := strings.TrimSpace(line)
		if ccaIsAllBackticks(trimmed) && (terminated || final) {
			if terminated {
				return true, buffer[:start], buffer[start+newline+1:]
			}
			return true, buffer[:start], ""
		}
		if !terminated {
			break
		}
		start += newline + 1
	}
	// Stream committed lines; hold only the last, possibly partial, line.
	if lastNewline := strings.LastIndexByte(buffer, '\n'); lastNewline != -1 {
		return false, buffer[:lastNewline+1], buffer[lastNewline+1:]
	}
	return false, "", buffer
}
