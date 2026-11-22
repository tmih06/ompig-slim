package codingagent

// ompig-slim: /session report sections upstream does not render: the aligned
// stat block and the session time trace. The data comes from
// SessionAccounting and the subagent manager; each section is written
// through the report's bold/dim stylers so the output follows theme changes
// like the upstream sections do.

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/internal/subagents"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

const (
	// sessionTraceWidth is the track width in the transcript. The fullscreen
	// view widens it to the terminal, within the two bounds.
	sessionTraceWidth       = 40
	sessionMinTraceWidth    = 28
	sessionMaxTraceWidth    = 120
	sessionMaxLaneNameWidth = 24
	sessionMaxSubagentLanes = 12
	// sessionLaneChrome is the width a lane row takes besides its name and
	// track: marker, gaps, token count, and share.
	sessionLaneChrome = 16
)

// sessionBarSlots cycles ANSI palette slots so adjacent lanes keep distinct
// hues in any theme.
var sessionBarSlots = []int{5, 2, 6, 4, 1, 3, 14, 13, 10, 12, 11, 9}

// sessionField is one label/value row of the stat block.
type sessionField struct{ label, value string }

// writeSessionFields renders rows as a dim label column and a value column
// that starts at the same cell on every row.
func writeSessionFields(b *strings.Builder, fields []sessionField, dim func(string) string) {
	labelWidth := 0
	for _, f := range fields {
		labelWidth = max(labelWidth, widthx.VisibleWidth(f.label))
	}
	for _, f := range fields {
		pad := strings.Repeat(" ", labelWidth-widthx.VisibleWidth(f.label))
		fmt.Fprintf(b, "%s%s  %s\n", dim(f.label), pad, f.value)
	}
}

// sessionLane is one row of the trace: who ran, when, and what it used.
type sessionLane struct {
	marker string // styled bullet or status icon
	name   string // plain text
	slot   int
	spans  []SessionSpan
	tokens int
	pct    string // share of session tokens, empty for rows outside the total
}

// sessionModelLanes is one lane per model the main agent used: timed lanes
// first, by start, then the untimed by size.
func sessionModelLanes(stats SessionAccounting, ansi func(int, string) string) []sessionLane {
	if stats.Tokens.Total <= 0 {
		return nil
	}
	rows := slices.Clone(stats.UsageBreakdown)
	slices.SortStableFunc(rows, func(left, right SessionUsageBreakdown) int {
		timedLeft, timedRight := len(left.Spans) > 0, len(right.Spans) > 0
		switch {
		case timedLeft && timedRight:
			return cmp.Compare(left.Spans[0].Start, right.Spans[0].Start)
		case timedLeft:
			return -1
		case timedRight:
			return 1
		default:
			return cmp.Compare(right.Tokens, left.Tokens)
		}
	})
	lanes := make([]sessionLane, len(rows))
	for i, row := range rows {
		slot := sessionBarSlots[i%len(sessionBarSlots)]
		lanes[i] = sessionLane{
			marker: ansi(slot, "●"), name: row.Key, slot: slot, spans: row.Spans, tokens: row.Tokens,
			pct: jsstring.ToFixed(float64(row.Tokens)*100/float64(stats.Tokens.Total), 0) + "%",
		}
	}
	return lanes
}

// sessionSubagentLanes is a lane for each of the first sessionMaxSubagentLanes
// runs, which come sorted by start. A running subagent's span ends at now.
// Slots continue after the model lanes' so every lane keeps its own hue.
func sessionSubagentLanes(runs []subagents.Record, modelLanes int, now time.Time) []sessionLane {
	lanes := make([]sessionLane, 0, min(len(runs), sessionMaxSubagentLanes))
	for i, r := range runs[:cap(lanes)] {
		lane := sessionLane{
			marker: subagentIcon(r), name: strings.Repeat("  ", min(r.Depth, 2)) + subagentIdentity(r),
			slot: sessionBarSlots[(modelLanes+i)%len(sessionBarSlots)], tokens: int(r.Tokens),
		}
		if r.StartedAt > 0 {
			end := r.CompletedAt
			if r.Active() {
				end = now.UnixMilli()
			}
			lane.spans = []SessionSpan{{r.StartedAt, max(end, r.StartedAt)}}
		}
		lanes = append(lanes, lane)
	}
	return lanes
}

// writeSessionTrace renders the session as a time trace. Every lane shares
// one track from the session's first to its last timed activity: a lane per
// model the main agent used, then a lane per subagent, so work that ran at
// the same time lines up in the same column. A cell is filled only where its
// time slice held work, and each cell resolves two slices (▌ ▐ █), so gaps
// and interleaving stay visible. Lanes without a time have an empty track.
// width is the track's length in cells.
func writeSessionTrace(b *strings.Builder, stats SessionAccounting, records []subagents.Record, now time.Time, width int, ansi func(int, string) string, bold, dim func(string) string) {
	models := sessionModelLanes(stats, ansi)
	runs := slices.Clone(records)
	slices.SortStableFunc(runs, func(left, right subagents.Record) int { return cmp.Compare(left.StartedAt, right.StartedAt) })
	agents := sessionSubagentLanes(runs, len(models), now)
	if len(models) == 0 && len(agents) == 0 {
		return
	}
	var agentTokens int64
	var agentCost float64
	for _, r := range runs {
		agentTokens += r.Tokens
		agentCost += r.Cost
	}

	nameWidth := len("Subagents")
	var all []SessionSpan
	for _, lanes := range [][]sessionLane{models, agents} {
		for _, lane := range lanes {
			nameWidth = max(nameWidth, widthx.VisibleWidth(lane.name))
			all = append(all, lane.spans...)
		}
	}
	nameWidth = min(nameWidth, sessionMaxLaneNameWidth)
	axis := newTraceAxis(all, width)
	writeLane := func(lane sessionLane) {
		name := widthx.TruncateToWidth(lane.name, nameWidth, "…", true)
		fmt.Fprintf(b, "%s %s %s  %6s %4s\n", lane.marker, name, sessionTraceTrack(lane, axis, ansi, dim), formatTokens(lane.tokens), lane.pct)
	}

	b.WriteString("\n" + bold("Tokens by model") + "\n")
	if !axis.empty() {
		fmt.Fprintf(b, "  %s %s\n", strings.Repeat(" ", nameWidth), dim(axis.label()))
	}
	for _, lane := range models {
		writeLane(lane)
	}
	if len(agents) > 0 {
		b.WriteString("\n" + bold("Subagents") + "\n")
		for _, lane := range agents {
			writeLane(lane)
		}
		if more := len(runs) - len(agents); more > 0 {
			fmt.Fprintf(b, "  %s\n", dim(fmt.Sprintf("…and %d more", more)))
		}
	}

	b.WriteString(dim(strings.Repeat("─", nameWidth+width+sessionLaneChrome)) + "\n")
	writeTotal := func(label, middle string, tokens int) {
		fmt.Fprintf(b, "  %s %s  %6s %s\n",
			widthx.TruncateToWidth(label, nameWidth, "…", true),
			dim(fmt.Sprintf("%-*s", width, widthx.TruncateToWidth(middle, width, "…", false))),
			formatTokens(tokens), dim("tokens"))
	}
	if len(models) > 0 {
		writeTotal("Total", axis.summary(), stats.Tokens.Total)
	}
	if len(runs) > 0 {
		summary := fmt.Sprintf("%d agents", len(runs))
		if len(runs) == 1 {
			summary = "1 agent"
		}
		if agentCost > 0 {
			summary += " · $" + jsstring.ToFixed(agentCost, 3)
		}
		writeTotal("Subagents", summary, int(agentTokens))
	}
}

// sessionTraceTrack draws one lane's track. The axis's active time is cut
// into two slices per cell; a span marks every slice it touches, and at
// least one so an instant stays visible.
func sessionTraceTrack(lane sessionLane, axis traceAxis, ansi func(int, string) string, dim func(string) string) string {
	halves := axis.width * 2
	busy := make([]bool, halves)
	for _, s := range lane.spans {
		// A span has at least a millisecond on the axis, so active is positive.
		lo := int(axis.offset(s.Start) * int64(halves) / axis.active)
		hi := int((axis.offset(s.End)*int64(halves)+axis.active-1)/axis.active) - 1
		lo = min(max(lo, 0), halves-1)
		hi = min(max(hi, lo), halves-1)
		for i := lo; i <= hi; i++ {
			busy[i] = true
		}
	}
	// Indexed by the left slice (2) and right slice (1) being busy.
	glyphs := [4]string{dim("·"), ansi(lane.slot, "▐"), ansi(lane.slot, "▌"), ansi(lane.slot, "█")}
	var out strings.Builder
	for c := 0; c < axis.width; c++ {
		glyph := 0
		if busy[2*c] {
			glyph |= 2
		}
		if busy[2*c+1] {
			glyph |= 1
		}
		out.WriteString(glyphs[glyph])
	}
	return out.String()
}

// sessionIdleSkip is the shortest idle stretch the trace reports. Shorter
// gaps are still cut from the axis but are not worth a marker.
const sessionIdleSkip = time.Minute

// traceAxis maps wall-clock times onto the trace's axis, which counts
// only the time in which some lane was working. Idle stretches (the user
// away, nothing running) are cut out so the trace reads as one continuous
// run; concurrent lanes still share columns.
type traceAxis struct {
	runs   []traceRun
	active int64 // total length of runs, in ms
	idle   int64 // idle time of at least sessionIdleSkip that was cut, in ms
	width  int   // track length in cells
}

// traceRun is one stretch of continuous work, starting active ms into the axis.
type traceRun struct{ start, end, active int64 }

func newTraceAxis(spans []SessionSpan, width int) traceAxis {
	timed := make([]SessionSpan, 0, len(spans))
	for _, s := range spans {
		if s.Start > 0 {
			// An instant still takes a millisecond, so it has a place on the axis.
			timed = append(timed, SessionSpan{s.Start, max(s.End, s.Start+1)})
		}
	}
	slices.SortFunc(timed, func(left, right SessionSpan) int { return cmp.Compare(left.Start, right.Start) })
	c := traceAxis{width: width}
	for _, s := range timed {
		if n := len(c.runs); n > 0 && s.Start <= c.runs[n-1].end {
			c.runs[n-1].end = max(c.runs[n-1].end, s.End)
			continue
		}
		if n := len(c.runs); n > 0 {
			if gap := s.Start - c.runs[n-1].end; gap >= sessionIdleSkip.Milliseconds() {
				c.idle += gap
			}
			c.active += c.runs[n-1].end - c.runs[n-1].start
		}
		c.runs = append(c.runs, traceRun{start: s.Start, end: s.End, active: c.active})
	}
	if n := len(c.runs); n > 0 {
		c.active += c.runs[n-1].end - c.runs[n-1].start
	}
	return c
}

func (c traceAxis) empty() bool { return len(c.runs) == 0 }

// offset is how much working time precedes t. A time in an idle gap maps to
// the end of the run before it.
func (c traceAxis) offset(t int64) int64 {
	i, _ := slices.BinarySearchFunc(c.runs, t, func(r traceRun, t int64) int {
		if r.start > t {
			return 1
		}
		return -1
	})
	if i == 0 {
		return 0
	}
	r := c.runs[i-1]
	return r.active + min(t, r.end) - r.start
}

// label is the axis line: the wall-clock time at the track's two ends, and
// a ┆ in each column where an idle stretch was cut.
func (c traceAxis) label() string {
	first, last := c.runs[0].start, c.runs[len(c.runs)-1].end
	from, to := time.UnixMilli(first).Local(), time.UnixMilli(last).Local()
	layout := "15:04"
	if from.YearDay() != to.YearDay() || from.Year() != to.Year() {
		layout = "Jan 02 15:04"
	}
	left, right := []rune(from.Format(layout)), []rune(to.Format(layout))
	cells := []rune(strings.Repeat(" ", c.width))
	for i := 1; i < len(c.runs); i++ {
		if c.runs[i].start-c.runs[i-1].end >= sessionIdleSkip.Milliseconds() {
			cells[min(int(c.runs[i].active*int64(c.width)/c.active), c.width-1)] = '┆'
		}
	}
	copy(cells, left)
	if len(c.runs) > 1 || last > first+1 {
		copy(cells[max(len(left)+1, c.width-len(right)):], right)
	}
	return string(cells)
}

// summary is the Total row's note: the working time shown and the idle time cut.
func (c traceAxis) summary() string {
	if c.empty() {
		return ""
	}
	text := tui.FormatDuration(c.active) + " active"
	if c.idle > 0 {
		text += " · " + tui.FormatDuration(c.idle) + " idle cut ┆"
	}
	return text
}
