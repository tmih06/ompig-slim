package tui

// Ports packages/coding-agent/src/modes/interactive/model-metrics.ts: the catalog metadata columns
// shared by the /model and /scoped-models selectors.
//
// Rows keep a readable model identity on the left and a right-aligned block of metric columns:
// intelligence index, catalog speed, context window, per-million price, and supported thinking
// efforts. Columns are measured per visible window and dropped lowest-priority-first so narrow
// terminals keep the name readable; the selected model's full detail block always lists every fact.

import (
	"math"
	"strconv"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

const (
	// modelMetricMinNameWidth is the narrowest model-identity cell (cursor, id, name, provider) kept
	// before columns are dropped.
	modelMetricMinNameWidth = 20
	// modelMetricPerfMinWidth is the row width from which the catalog speed column appears.
	modelMetricPerfMinWidth = 76
	// modelMetricEffortsMinWidth is the row width from which the supported-efforts column appears.
	modelMetricEffortsMinWidth = 56
	// modelMetricColumnGap is the gap between the identity cell and the first metric column.
	modelMetricColumnGap = 1
	// modelMetricColumnSpacing is the gap between metric columns.
	modelMetricColumnSpacing = 2
)

// effortOrder is the thinking ladder in ascending intensity (EFFORT_ORDER of model-metrics.ts).
var effortOrder = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

var effortShortLabels = map[string]string{
	"off":     "off",
	"minimal": "min",
	"low":     "low",
	"medium":  "med",
	"high":    "high",
	"xhigh":   "xhigh",
	"max":     "max",
}

// SanitizeModelText removes terminal control sequences and flattens whitespace in provider-supplied
// text (sanitizeModelText of model-metrics.ts). Catalog ids, names and blurbs are remote input.
func SanitizeModelText(value string) string {
	if value == "" {
		return ""
	}
	// Most catalog strings are already plain: skip the scanning passes on them.
	unsafe := false
	for _, r := range value {
		if (r <= 0x1f) || (r >= 0x7f && r <= 0x9f) {
			unsafe = true
			break
		}
	}
	if !unsafe {
		return jsstring.Trim(value)
	}
	cleaned := widthx.ReplaceControlCharacters(widthx.StripTerminalSequences(value))
	return strings.Join(strings.Fields(cleaned), " ")
}

// ModelMetricCells are the plain (unstyled) metric cells of one model row; an empty string hides
// that column for the row.
type ModelMetricCells struct {
	Intelligence string
	Perf         string
	Context      string
	Cost         string
	Efforts      string
}

// ModelMetricWidths are the column widths of one visible window; zero hides the column.
type ModelMetricWidths struct {
	Intelligence int
	Perf         int
	Context      int
	Cost         int
	Efforts      int
}

// hiddenModelMetricWidths hides every column.
var hiddenModelMetricWidths = ModelMetricWidths{}

// formatCostLeg renders one per-million price leg: "3", "0.25", "12.5"; "?" when the catalog value
// is unusable.
func formatCostLeg(rate float64) string {
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 {
		return "?"
	}
	if rate > 0 && rate < 0.01 {
		// JavaScript's toLocaleString with maximumFractionDigits 20 keeps the significant digits of a
		// micro price instead of rounding it to zero.
		return strconv.FormatFloat(rate, 'f', -1, 64)
	}
	var formatted string
	switch {
	case rate >= 100:
		formatted = strconv.FormatFloat(math.Round(rate), 'f', -1, 64)
	case rate >= 10:
		formatted = strconv.FormatFloat(rate, 'f', 1, 64)
	default:
		formatted = strconv.FormatFloat(rate, 'f', 2, 64)
	}
	return trimTrailingZeros(formatted)
}

// trimTrailingZeros is the upstream `formatted.replace(/\.?0+$/, "")` for a decimal string.
func trimTrailingZeros(formatted string) string {
	if !strings.Contains(formatted, ".") {
		return formatted
	}
	trimmed := strings.TrimRight(formatted, "0")
	return strings.TrimSuffix(trimmed, ".")
}

// FormatTokenCount renders a compact token count: "1M", "1.5M", "384k", "999".
func FormatTokenCount(count float64) string {
	leg := func(value float64) string {
		formatted := strconv.FormatFloat(value, 'f', 1, 64)
		return strings.TrimSuffix(formatted, ".0")
	}
	if count >= 1_000_000 {
		return leg(count/1_000_000) + "M"
	}
	if count >= 1_000 {
		return leg(count/1_000) + "k"
	}
	return strconv.FormatFloat(math.Round(count), 'f', -1, 64)
}

// formatTps renders "118t/s" average output speed; one decimal below 10 t/s.
func formatTps(tps float64) string {
	if tps >= 10 {
		return strconv.FormatFloat(math.Round(tps), 'f', -1, 64) + "t/s"
	}
	return strconv.FormatFloat(tps, 'f', 1, 64) + "t/s"
}

// GetSupportedEfforts lists the thinking efforts the model's ThinkingLevelMap declares, least
// intensive first (getSupportedEfforts of model-metrics.ts).
func GetSupportedEfforts(model *ai.Model) []string {
	if model == nil || model.ThinkingLevelMap == nil {
		return nil
	}
	efforts := []string{}
	for _, level := range effortOrder {
		mapped, present := model.ThinkingLevelMap[ai.ModelThinkingLevel(level)]
		if present && mapped != nil {
			efforts = append(efforts, level)
		}
	}
	return efforts
}

func formatEffortRange(efforts []string) string {
	if len(efforts) == 0 {
		return ""
	}
	if len(efforts) == 1 {
		return effortShortLabels[efforts[0]]
	}
	firstIndex, lastIndex := -1, -1
	for i, level := range effortOrder {
		if level == efforts[0] && firstIndex < 0 {
			firstIndex = i
		}
		if level == efforts[len(efforts)-1] {
			lastIndex = i
		}
	}
	contiguous := firstIndex >= 0 && lastIndex-firstIndex == len(efforts)-1
	if contiguous {
		return effortShortLabels[efforts[0]] + "-" + effortShortLabels[efforts[len(efforts)-1]]
	}
	labels := make([]string, len(efforts))
	for i, level := range efforts {
		labels[i] = effortShortLabels[level]
	}
	return strings.Join(labels, "/")
}

// GetModelMetricCells builds the plain metric cells of a model, using only values the catalog
// actually supplied (getModelMetricCells of model-metrics.ts).
func GetModelMetricCells(model *ai.Model) ModelMetricCells {
	if model == nil {
		return ModelMetricCells{}
	}
	cells := ModelMetricCells{}
	// Catalog fields are wire input from remote snapshots: range-check before display.
	if model.Int != nil && !math.IsNaN(*model.Int) && !math.IsInf(*model.Int, 0) {
		cells.Intelligence = "int " + strconv.FormatFloat(math.Round(*model.Int), 'f', -1, 64)
	}
	if model.TPS != nil && !math.IsNaN(*model.TPS) && !math.IsInf(*model.TPS, 0) && *model.TPS > 0 {
		cells.Perf = "~" + formatTps(*model.TPS)
	}
	contextWindow := float64(model.Capabilities.ContextWindow)
	if !math.IsNaN(contextWindow) && contextWindow > 0 {
		cells.Context = FormatTokenCount(contextWindow) + " ctx"
	}
	// Upstream model browser behavior: an all-zero cost pair renders "free".
	input := float64(model.Capabilities.InputCostPer1M)
	output := float64(model.Capabilities.OutputCostPer1M)
	if input == 0 && output == 0 {
		cells.Cost = "free"
	} else {
		cells.Cost = "$" + formatCostLeg(input) + "/" + formatCostLeg(output)
	}
	cells.Efforts = formatEffortRange(GetSupportedEfforts(model))
	return cells
}

// modelMetricColumnsWidth is the total width of the visible columns including their gaps.
func modelMetricColumnsWidth(widths ModelMetricWidths) int {
	total, count := 0, 0
	for _, columnWidth := range []int{widths.Intelligence, widths.Perf, widths.Context, widths.Cost, widths.Efforts} {
		if columnWidth <= 0 {
			continue
		}
		total += columnWidth
		count++
	}
	if count == 0 {
		return 0
	}
	return total + modelMetricColumnSpacing*(count-1)
}

// PlanModelMetricColumns measures and fits the metadata columns of one rendered window. The identity
// cell keeps at least modelMetricMinNameWidth cells.
func PlanModelMetricColumns(width int, rows []ModelMetricCells) ModelMetricWidths {
	measured := ModelMetricWidths{}
	for _, cells := range rows {
		measured.Intelligence = max(measured.Intelligence, widthx.VisibleWidth(cells.Intelligence))
		measured.Perf = max(measured.Perf, widthx.VisibleWidth(cells.Perf))
		measured.Context = max(measured.Context, widthx.VisibleWidth(cells.Context))
		measured.Cost = max(measured.Cost, widthx.VisibleWidth(cells.Cost))
		measured.Efforts = max(measured.Efforts, widthx.VisibleWidth(cells.Efforts))
	}
	if width < modelMetricPerfMinWidth {
		measured.Perf = 0
	}
	if width < modelMetricEffortsMinWidth {
		measured.Efforts = 0
	}
	widths := measured
	// Lowest priority first; intelligence stays longest because it has no other home in the rows.
	for _, key := range []string{"efforts", "perf", "cost", "context", "intelligence"} {
		columns := modelMetricColumnsWidth(widths)
		gap := 0
		if columns > 0 {
			gap = modelMetricColumnGap
		}
		if width-2-columns-gap >= modelMetricMinNameWidth {
			break
		}
		switch key {
		case "efforts":
			widths.Efforts = 0
		case "perf":
			widths.Perf = 0
		case "cost":
			widths.Cost = 0
		case "context":
			widths.Context = 0
		case "intelligence":
			widths.Intelligence = 0
		}
	}
	return widths
}

// ModelMetricRow is a selector row: its already-styled identity cell plus the model whose facts it
// shows.
type ModelMetricRow struct {
	Left  string
	Model *ai.Model
}

// ModelMetricDetailText is the one-line metric text of a model for surfaces that lay out their own
// rows, such as a generic frontend's selector item detail: the non-empty columns in row order, plain
// (unstyled) and already sanitized. An empty string means the catalog supplied nothing.
func ModelMetricDetailText(model *ai.Model) string {
	cells := GetModelMetricCells(model)
	parts := make([]string, 0, 5)
	for _, cell := range []string{cells.Intelligence, cells.Perf, cells.Context, cells.Cost, cells.Efforts} {
		if cell != "" {
			parts = append(parts, cell)
		}
	}
	return strings.Join(parts, " · ")
}

// ModelDetailFactsText is the selected model's whole fact set as one plain line, for surfaces that
// wrap their own text; unavailableText replaces it for a nil model.
func ModelDetailFactsText(model *ai.Model, unavailableText string) string {
	if model == nil {
		if unavailableText == "" {
			return "Model unavailable"
		}
		return unavailableText
	}
	return GetModelDetailFacts(model).Summary
}

// ModelMetricRows renders metadata rows at the current terminal width. Identity text is truncated
// (ANSI aware) so metric columns stay aligned instead of wrapping.
type ModelMetricRows struct {
	invalidatable
	rows []ModelMetricRow
}

// NewModelMetricRows builds the metric row block of one visible window.
func NewModelMetricRows(rows []ModelMetricRow) *ModelMetricRows {
	return &ModelMetricRows{rows: rows}
}

// SetRows replaces the rows and invalidates the cached render.
func (m *ModelMetricRows) SetRows(rows []ModelMetricRow) {
	m.rows = rows
	m.Invalidate()
}

// Render draws every row with its right-aligned metric columns.
func (m *ModelMetricRows) Render(width int) []string {
	if len(m.rows) == 0 {
		return nil
	}
	renderWidth := max(1, width)
	cells := make([]ModelMetricCells, len(m.rows))
	for i, row := range m.rows {
		cells[i] = GetModelMetricCells(row.Model)
	}
	widths := PlanModelMetricColumns(renderWidth, cells)
	dim := ActiveTheme().Dim
	lines := make([]string, 0, len(m.rows))
	for index, row := range m.rows {
		rowCells := cells[index]
		if rowCells.Intelligence == "" && rowCells.Perf == "" && rowCells.Context == "" && rowCells.Cost == "" && rowCells.Efforts == "" {
			lines = append(lines, widthx.TruncateToWidth(row.Left, renderWidth, "", false))
			continue
		}
		var columns []string
		push := func(text string, columnWidth int) {
			if columnWidth <= 0 {
				return
			}
			missing := columnWidth - widthx.VisibleWidth(text)
			if missing > 0 {
				text = strings.Repeat(" ", missing) + text
			}
			columns = append(columns, fg(dim, text))
		}
		push(rowCells.Intelligence, widths.Intelligence)
		push(rowCells.Perf, widths.Perf)
		push(rowCells.Context, widths.Context)
		push(rowCells.Cost, widths.Cost)
		push(rowCells.Efforts, widths.Efforts)
		if len(columns) == 0 {
			lines = append(lines, widthx.TruncateToWidth(row.Left, renderWidth, "", false))
			continue
		}
		metaWidth := modelMetricColumnsWidth(widths)
		available := max(1, renderWidth-metaWidth-modelMetricColumnGap)
		left := widthx.TruncateToWidth(row.Left, available, "", false)
		gap := max(0, available-widthx.VisibleWidth(left))
		lines = append(lines, left+strings.Repeat(" ", gap)+" "+strings.Join(columns, strings.Repeat(" ", modelMetricColumnSpacing)))
	}
	return lines
}

// ModelDetailFacts is the full fact set of one selected model.
type ModelDetailFacts struct {
	Facts   []string
	Summary string
	Efforts string
}

// GetModelDetailFacts lists every fact of a model, in the order the upstream model browser shows
// them: identity, limits, price, capabilities, catalog metrics, blurb.
func GetModelDetailFacts(model *ai.Model) ModelDetailFacts {
	if model == nil {
		return ModelDetailFacts{}
	}
	facts := []string{}
	identity := SanitizeModelText(model.DisplayName)
	if identity == "" {
		identity = SanitizeModelText(model.ID)
	}
	facts = append(facts, identity)
	if model.Capabilities.ContextWindow > 0 {
		facts = append(facts, FormatTokenCount(float64(model.Capabilities.ContextWindow))+" ctx")
	}
	if model.Capabilities.MaxOutputTokens > 0 {
		facts = append(facts, FormatTokenCount(float64(model.Capabilities.MaxOutputTokens))+" out")
	}
	if model.Capabilities.InputCostPer1M == 0 && model.Capabilities.OutputCostPer1M == 0 {
		facts = append(facts, "free")
	} else {
		// Cache legs are shown even at zero: a published 0 means cache tokens are free.
		facts = append(facts,
			"$"+formatCostLeg(model.Capabilities.InputCostPer1M)+" in",
			"$"+formatCostLeg(model.Capabilities.OutputCostPer1M)+" out",
			"$"+formatCostLeg(model.Capabilities.CacheReadCostPer1M)+" cache read",
			"$"+formatCostLeg(model.Capabilities.CacheWriteCostPer1M)+" cache write",
			"per M",
		)
	}
	if model.ProviderMeta.Reasoning {
		facts = append(facts, "reasoning")
	}
	// Input modalities already state image support; no separate "vision" marker.
	if len(model.Input) > 0 {
		facts = append(facts, "input: "+strings.Join(model.Input, " · "))
	}
	if model.Int != nil && !math.IsNaN(*model.Int) && !math.IsInf(*model.Int, 0) {
		facts = append(facts, "int "+strconv.FormatFloat(math.Round(*model.Int), 'f', -1, 64))
	}
	if model.TPS != nil && !math.IsNaN(*model.TPS) && !math.IsInf(*model.TPS, 0) && *model.TPS > 0 {
		facts = append(facts, "~"+formatTps(*model.TPS))
	}
	if description := SanitizeModelText(model.Description); description != "" {
		facts = append(facts, description)
	}
	efforts := GetSupportedEfforts(model)
	if len(efforts) > 0 {
		facts = append(facts, "Efforts: "+strings.Join(efforts, " · "))
	}
	return ModelDetailFacts{Facts: facts, Summary: strings.Join(facts, " · "), Efforts: strings.Join(efforts, " · ")}
}

// packFacts packs facts into intact lines; a fact longer than the width wraps instead of being cut.
func packFacts(facts []string, width, indent int) []string {
	available := max(1, width-indent)
	lines := []string{}
	current := ""
	for _, fact := range facts {
		candidate := fact
		if current != "" {
			candidate = current + " · " + fact
		}
		if widthx.VisibleWidth(candidate) <= available {
			current = candidate
			continue
		}
		if current != "" {
			lines = append(lines, current)
		}
		current = ""
		if widthx.VisibleWidth(fact) <= available {
			current = fact
			continue
		}
		// Long single values (ids, descriptions) wrap to whole lines rather than truncating.
		wrapped := widthx.WrapTextWithAnsi(fact, available)
		if len(wrapped) > 1 {
			lines = append(lines, wrapped[:len(wrapped)-1]...)
		}
		if len(wrapped) > 0 {
			current = wrapped[len(wrapped)-1]
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	muted := ActiveTheme().Muted
	padded := make([]string, len(lines))
	for i, line := range lines {
		padded[i] = fg(muted, strings.Repeat(" ", indent)+line)
	}
	return padded
}

// ModelDetailFactsComponent renders the detail block of the selected model: every catalog fact,
// wrapped so no value is lost.
type ModelDetailFactsComponent struct {
	invalidatable
	model           *ai.Model
	unavailableText string
}

// NewModelDetailFacts builds the detail block of model, or the unavailable line when model is nil.
func NewModelDetailFacts(model *ai.Model, unavailableText string) *ModelDetailFactsComponent {
	return &ModelDetailFactsComponent{model: model, unavailableText: unavailableText}
}

// SetModel replaces the model whose facts are shown.
func (m *ModelDetailFactsComponent) SetModel(model *ai.Model) {
	m.model = model
	m.Invalidate()
}

// Render draws the wrapped fact block.
func (m *ModelDetailFactsComponent) Render(width int) []string {
	renderWidth := max(1, width)
	if m.model == nil {
		message := m.unavailableText
		if message == "" {
			message = "Model unavailable"
		}
		return []string{widthx.TruncateToWidth(fg(ActiveTheme().Muted, "  "+message), renderWidth, "", false)}
	}
	// Each fact stays intact so values such as "1M ctx" are never split mid-value.
	return packFacts(GetModelDetailFacts(m.model).Facts, renderWidth, 2)
}
