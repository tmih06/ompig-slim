package tui

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// browserNumber follows pi-utils.formatNumber, not the older PiG metric helper:
// one decimal for small leading values, whole thousands above ten thousand.
func browserNumber(n int) string {
	value := float64(n)
	suffix := ""
	switch {
	case n >= 1000000000:
		value /= 1000000000
		suffix = "b"
	case n >= 1000000:
		value /= 1000000
		suffix = "m"
	case n >= 1000:
		value /= 1000
		suffix = "k"
	}
	if value >= 10 {
		value = math.Round(value)
	} else {
		value = math.Round(value*10) / 10
	}
	return strconv.FormatFloat(value, 'f', -1, 64) + suffix
}

// browserMetricCells keeps OMP's browser-only columns separate from the legacy
// generic metric API: intelligence, throughput, context, price; never efforts.
func browserMetricCells(model *ai.Model, width int) [4]string {
	if model == nil {
		return [4]string{}
	}
	cells := GetModelMetricCells(model)
	intelligence, perf := "", ""
	if width >= 76 {
		if cells.Intelligence != "" {
			intelligence = strings.Replace(cells.Intelligence, "int ", "🧠 ", 1)
		}
		perf = cells.Perf
	}
	context := ""
	if model.Capabilities.ContextWindow > 0 {
		context = browserNumber(model.Capabilities.ContextWindow) + " ◫"
	}
	return [4]string{intelligence, perf, context, cells.Cost}
}

// renderBrowserMetrics measures only the visible window, reserves the final
// scrollbar column, and drops price then context to protect a 16-column name.
func renderBrowserMetrics(rows []ModelMetricRow, width int) []string {
	cells := make([][4]string, len(rows))
	widths := [4]int{}
	for i, row := range rows {
		cells[i] = browserMetricCells(row.Model, width)
		for j, value := range cells[i] {
			widths[j] = max(widths[j], widthx.VisibleWidth(value))
		}
	}
	metaWidth := func() int {
		total, count := 0, 0
		for _, w := range widths {
			if w > 0 {
				total += w
				count++
			}
		}
		return total + max(0, count-1)*2
	}
	if width-2-metaWidth() < 16 {
		widths[3] = 0
	}
	if width-2-metaWidth() < 16 {
		widths[2] = 0
	}
	out := make([]string, len(rows))
	for i, row := range rows {
		columns := []string{}
		for j, value := range cells[i] {
			if widths[j] > 0 {
				columns = append(columns, modelOverlayTheme().Fg("dim", strings.Repeat(" ", max(0, widths[j]-widthx.VisibleWidth(value)))+value))
			}
		}
		nameWidth := max(1, width-1-metaWidth())
		if len(columns) > 0 {
			nameWidth--
		}
		left := widthx.TruncateToWidth(row.Left, nameWidth, "…", true)
		if len(columns) > 0 {
			left += " " + strings.Join(columns, "  ")
		}
		out[i] = left
	}
	return out
}

// browserFacts matches OMP's compact two-line detail pane. Cache prices and
// effort ladders belong in model configuration, not this browser's prose row.
func browserFacts(item ModelSelectorItem) string {
	model := item.Model
	if model == nil {
		return SanitizeModelText(item.Name)
	}
	facts := []string{SanitizeModelText(model.DisplayName)}
	if facts[0] == "" {
		facts[0] = SanitizeModelText(item.ID)
	}
	if n := model.Capabilities.ContextWindow; n > 0 {
		facts = append(facts, browserNumber(n)+" ctx")
	}
	if n := model.Capabilities.MaxOutputTokens; n > 0 {
		facts = append(facts, browserNumber(n)+" out")
	}
	cost := GetModelMetricCells(model).Cost
	if cost == "free" {
		facts = append(facts, "free")
	} else {
		facts = append(facts, cost+" per M")
	}
	if model.ProviderMeta.Reasoning {
		facts = append(facts, "reasoning")
	}
	if slices.Contains(model.Input, "image") {
		facts = append(facts, "vision")
	}
	cells := browserMetricCells(model, 120)
	if cells[0] != "" {
		facts = append(facts, cells[0])
	}
	if cells[1] != "" {
		facts = append(facts, cells[1])
	}
	if description := SanitizeModelText(model.Description); description != "" {
		facts = append(facts, description)
	}
	return strings.Join(facts, " · ")
}

// browserScrollbar paints OMP's persistent one-column list scrollbar only when
// the scope overflows. Blank padding rows are not part of the scroll range.
func browserScrollbar(row, visible, total, offset int) string {
	if total <= visible {
		return " "
	}
	thumb := max(1, visible*visible/total)
	start := 0
	if total > visible {
		start = offset * (visible - thumb) / (total - visible)
	}
	if row >= start && row < start+thumb {
		return modelOverlayTheme().Fg("accent", "█")
	}
	return modelOverlayTheme().Fg("muted", "│")
}

// browserEffortGlyph is the configured effort badge used on role chips/rows.
func browserEffortGlyph(level string) string {
	switch level {
	case "off":
		return "○"
	case "minimal":
		return "◔"
	case "low":
		return "◔"
	case "medium":
		return "◑"
	case "high":
		return "◒"
	case "xhigh", "max":
		return "●"
	case "auto":
		return "◈"
	}
	return ""
}

// browserRoleChip retains the assignment's automatic/explicit distinction even
// when several logical roles resolve to the same physical model.
func browserRoleChip(role, level string, explicit bool, color string) string {
	th := modelOverlayTheme()
	if !explicit {
		return fg(th.Dim, "○ "+SanitizeModelText(role))
	}
	label := th.Fg(color, "● "+SanitizeModelText(role))
	if level != "" && level != "inherit" {
		label += " " + fg(th.Dim, browserEffortGlyph(level))
	}
	return label
}

// browserOverflowWarning formats the compact-first fact without suppressing
// selection; the host still owns confirmation and the actual compaction.
func browserOverflowWarning(used, limit int) string {
	return fmt.Sprintf("  ✕ context %s exceeds %s limit · compacts with current model, then switches", browserNumber(used), browserNumber(limit))
}
