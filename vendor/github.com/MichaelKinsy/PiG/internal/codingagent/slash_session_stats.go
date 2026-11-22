package codingagent

// ompig-slim: /session report sections upstream does not render: the
// per-model token bar chart, the activity timeline sparkline, and the
// subagent rollup. The data comes from SessionAccounting and the subagent
// manager; each section is written through the report's bold/dim stylers so
// the output follows theme changes like the upstream sections do.

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/internal/subagents"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

const sessionBarWidth = 20

// sessionBarSlots cycles ANSI palette slots so adjacent model rows keep
// distinct hues in any theme.
var sessionBarSlots = []int{5, 2, 6, 4, 1, 3, 14, 13, 10, 12, 11, 9}

const sessionMaxModelNameWidth = 28

// writeSessionModelBars renders "Tokens by model": one colored bar per usage
// key, scaled to the largest row, with the share of the session's total
// tokens. Rows sort by tokens descending.
func writeSessionModelBars(b *strings.Builder, stats SessionAccounting, ansi func(int, string) string, bold, dim func(string) string) {
	if len(stats.UsageBreakdown) == 0 || stats.Tokens.Total <= 0 {
		return
	}
	rows := slices.Clone(stats.UsageBreakdown)
	slices.SortStableFunc(rows, func(left, right SessionUsageBreakdown) int {
		return right.Tokens - left.Tokens
	})
	maxTokens := rows[0].Tokens
	nameWidth := 0
	for _, row := range rows {
		nameWidth = max(nameWidth, widthx.VisibleWidth(row.Key))
	}
	nameWidth = min(nameWidth, sessionMaxModelNameWidth)

	b.WriteString("\n" + bold("Tokens by model") + "\n")
	for i, row := range rows {
		slot := sessionBarSlots[i%len(sessionBarSlots)]
		fill := 1
		if maxTokens > 0 {
			fill = max(1, int(math.Round(float64(row.Tokens)*sessionBarWidth/float64(maxTokens))))
		}
		name := widthx.TruncateToWidth(row.Key, nameWidth, "…", true)
		bar := ansi(slot, strings.Repeat("█", fill)) + dim(strings.Repeat("·", sessionBarWidth-fill))
		pct := jsstring.ToFixed(float64(row.Tokens)*100/float64(stats.Tokens.Total), 0) + "%"
		fmt.Fprintf(b, "%s %s %s %6s %4s\n", ansi(slot, "●"), name, bar, formatTokens(row.Tokens), pct)
	}
	fmt.Fprintf(b, "  %s%s %6s %s\n",
		widthx.TruncateToWidth("Total", nameWidth, "…", true),
		strings.Repeat(" ", sessionBarWidth+1),
		formatTokens(stats.Tokens.Total), dim("tokens"))
}

// writeSessionTimeline renders "Timeline": when the session started and a
// sparkline of user/assistant message activity across its active span.
func writeSessionTimeline(b *strings.Builder, stats SessionAccounting, headerTimestamp string, ansi func(int, string) string, bold, dim func(string) string) {
	started, hasStarted := sessionHeaderTime(headerTimestamp)
	activity := slices.Clone(stats.Activity)
	slices.Sort(activity)
	if !hasStarted && len(activity) == 0 {
		return
	}
	b.WriteString("\n" + bold("Timeline") + "\n")
	if hasStarted {
		ago := ""
		if age := sessionAge(started); age != "now" {
			ago = " " + dim("("+age+" ago)")
		}
		fmt.Fprintf(b, "%s %s%s\n", dim("Started:"), started.Local().Format("2006-01-02 15:04"), ago)
	}
	if len(activity) < 2 {
		return
	}
	first, last := time.UnixMilli(activity[0]), time.UnixMilli(activity[len(activity)-1])
	span := sessionSpanLabel(first, last)
	fmt.Fprintf(b, "%s %s  %s %s\n",
		dim("Activity:"), ansi(sessionBarSlots[0], sessionSparkline(activity, 24)), dim(span), dim("· "+sessionDuration(activity[len(activity)-1]-activity[0])))
}

// sessionSparkline buckets timestamps into width cells and maps each count
// onto the ▁…█ block ramp, the busiest bucket filling the top block.
func sessionSparkline(times []int64, width int) string {
	if len(times) == 0 || width <= 0 {
		return ""
	}
	lo, hi := times[0], times[len(times)-1]
	span := hi - lo
	buckets := make([]int, width)
	for _, t := range times {
		idx := 0
		if span > 0 {
			idx = int((t - lo) * int64(width) / (span + 1))
		}
		buckets[idx]++
	}
	peak := 1
	for _, count := range buckets {
		peak = max(peak, count)
	}
	var ramp = []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	var out strings.Builder
	for _, count := range buckets {
		out.WriteString(ramp[count*(len(ramp)-1)/peak])
	}
	return out.String()
}

// sessionSpanLabel renders the "15:04 – 18:32" or "Nov 20 – Nov 22" range a
// sparkline covers.
func sessionSpanLabel(first, last time.Time) string {
	first, last = first.Local(), last.Local()
	if first.YearDay() == last.YearDay() && first.Year() == last.Year() {
		return first.Format("15:04") + " – " + last.Format("15:04")
	}
	return first.Format("Jan 02") + " – " + last.Format("Jan 02")
}

// sessionDuration renders an active span as "45s", "1m 5s", "1h 23m", or
// "2d 4h", matching the compact style the subagent rows use.
func sessionDuration(ms int64) string {
	d := max(0, ms)
	switch d := time.Duration(d) * time.Millisecond; {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

// sessionHeaderTime parses the session header's millisecond ISO timestamp.
func sessionHeaderTime(timestamp string) (time.Time, bool) {
	ms := ai.DateParse(timestamp)
	if math.IsNaN(ms) {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(ms)), true
}

// writeSessionSubagents renders the "Subagents" rollup: every recorded agent
// with its status icon, name and the hub row's stats string, plus a totals
// line when more than one ran. The list is capped so a long session's report
// stays bounded.
func writeSessionSubagents(b *strings.Builder, records []subagents.Record, settings subagents.Settings, bold, dim func(string) string) {
	if len(records) == 0 {
		return
	}
	const maxRows = 10
	b.WriteString("\n" + bold("Subagents") + "\n")
	var totalTokens int64
	var totalCost float64
	for _, r := range records {
		totalTokens += r.Tokens
		totalCost += r.Cost
	}
	shown := min(len(records), maxRows)
	for _, r := range records[:shown] {
		indent := strings.Repeat("  ", min(r.Depth, 4))
		fmt.Fprintf(b, "%s %s%s  %s\n", subagentIcon(r), indent, subagentName(r), dim(subagentStats(r, settings)))
	}
	if more := len(records) - shown; more > 0 {
		fmt.Fprintf(b, "  %s\n", dim(fmt.Sprintf("…and %d more", more)))
	}
	if len(records) > 1 {
		summary := fmt.Sprintf("%d agents · %s tokens", len(records), formatTokens(int(totalTokens)))
		if totalCost > 0 {
			summary += fmt.Sprintf(" · $%s", jsstring.ToFixed(totalCost, 3))
		}
		fmt.Fprintf(b, "%s %s\n", dim("Total:"), dim(summary))
	}
}
