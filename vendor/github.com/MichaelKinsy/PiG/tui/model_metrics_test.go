package tui

// Behavioral tests for the catalog metric columns and the selected-model fact block. Mirrors
// packages/coding-agent/test/model-metrics.test.ts at the boundaries that stay uncertain in the port:
// price/token formatting, per-window column planning and its drop order, right alignment, and fact
// wrapping without truncation.

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// metricTestModel builds a chat model with the given catalog metrics.
func metricTestModel(id string, mutate func(*ai.Model)) *ai.Model {
	model := &ai.Model{
		ID:          id,
		DisplayName: id,
		ProviderMeta: ai.ProviderMetadata{
			ProviderID: "deploy",
			API:        ai.APIOpenAICompletions,
			BaseURL:    "https://example.com",
		},
		Input: []string{"text"},
		Capabilities: ai.ModelCapabilities{
			ContextWindow:   128_000,
			MaxOutputTokens: 16_384,
		},
	}
	if mutate != nil {
		mutate(model)
	}
	return model
}

func floatPointer(value float64) *float64 { return new(value) }

func stringPointer(value string) *string { return new(value) }

func TestModelMetricCellsFormatCatalogValues(t *testing.T) {
	model := metricTestModel("deepseek-ai/deepseek-v4.1-flash-1017", func(model *ai.Model) {
		model.DisplayName = "DeepSeek V4.1 Flash 1017"
		model.ProviderMeta.Reasoning = true
		model.Capabilities.ContextWindow = 1_000_000
		model.Capabilities.MaxOutputTokens = 384_000
		model.Capabilities.InputCostPer1M = 0.3
		model.Capabilities.OutputCostPer1M = 1.2
		model.Capabilities.CacheReadCostPer1M = 0.006
		low, high, maxLevel := "low", "high", "max"
		model.ThinkingLevelMap = ai.ThinkingLevelMap{ai.ThinkingMinimal: nil, ai.ThinkingLow: &low, ai.ThinkingMedium: nil, ai.ThinkingHigh: &high, ai.ThinkingMax: &maxLevel}
		model.Int = floatPointer(62.4)
		model.TPS = floatPointer(118.4)
	})

	cells := GetModelMetricCells(model)
	if cells.Intelligence != "int 62" {
		t.Errorf("intelligence = %q, want %q", cells.Intelligence, "int 62")
	}
	if cells.Perf != "~118t/s" {
		t.Errorf("perf = %q, want %q", cells.Perf, "~118t/s")
	}
	if cells.Context != "1M ctx" {
		t.Errorf("context = %q, want %q", cells.Context, "1M ctx")
	}
	if cells.Cost != "$0.3/1.2" {
		t.Errorf("cost = %q, want %q", cells.Cost, "$0.3/1.2")
	}
	// A ladder with a gap is listed rather than ranged.
	if cells.Efforts != "low/high/max" {
		t.Errorf("efforts = %q, want %q", cells.Efforts, "low/high/max")
	}
}

func TestModelMetricRowsAlignColumnsInOneWindow(t *testing.T) {
	wide := metricTestModel("wide-model", func(model *ai.Model) {
		model.DisplayName = "Wide Model"
		model.Capabilities.ContextWindow = 200_000
		model.Capabilities.InputCostPer1M = 3
		model.Capabilities.OutputCostPer1M = 15
		low, medium, high := "low", "medium", "high"
		model.ThinkingLevelMap = ai.ThinkingLevelMap{ai.ThinkingLow: &low, ai.ThinkingMedium: &medium, ai.ThinkingHigh: &high}
		model.Int = floatPointer(71)
	})
	narrow := metricTestModel("narrow-model", func(model *ai.Model) {
		model.DisplayName = "Narrow Model"
		model.Capabilities.ContextWindow = 8_192
		model.Int = floatPointer(9)
	})

	lines := NewModelMetricRows([]ModelMetricRow{
		{Left: "→ wide-model [deploy]", Model: wide},
		{Left: "  narrow-model [deploy]", Model: narrow},
	}).Render(160)
	if len(lines) != 2 {
		t.Fatalf("rendered %d lines, want 2", len(lines))
	}
	first, second := widthx.StripTerminalSequences(lines[0]), widthx.StripTerminalSequences(lines[1])
	for _, want := range []string{"wide-model [deploy]", "int 71", "200k ctx", "$3/15", "low-high"} {
		if !strings.Contains(first, want) {
			t.Errorf("wide row %q lacks %q", first, want)
		}
	}
	if !strings.Contains(second, "int 9") {
		t.Errorf("narrow row %q lacks its intelligence", second)
	}
	// Metric cells are right-aligned inside one window-wide column: every row ends at the same column.
	if widthx.VisibleWidth(first) != widthx.VisibleWidth(second) {
		t.Errorf("row widths differ: %d and %d", widthx.VisibleWidth(first), widthx.VisibleWidth(second))
	}
	if strings.Index(first, "int 71") == strings.Index(second, "int 9") {
		t.Errorf("intelligence cells start at the same column: %q", first)
	}
}

func TestModelMetricColumnsDropLowPriorityFirst(t *testing.T) {
	model := metricTestModel("deepseek-ai/deepseek-v4.1-flash-1017", func(model *ai.Model) {
		model.DisplayName = "DeepSeek V4.1 Flash 1017"
		model.Capabilities.ContextWindow = 1_000_000
		model.Capabilities.InputCostPer1M = 0.3
		model.Capabilities.OutputCostPer1M = 1.2
		high := "high"
		model.ThinkingLevelMap = ai.ThinkingLevelMap{ai.ThinkingHigh: &high}
		model.Int = floatPointer(62)
		model.TPS = floatPointer(118)
	})
	cells := GetModelMetricCells(model)

	wide := PlanModelMetricColumns(140, []ModelMetricCells{cells})
	if wide.Intelligence == 0 || wide.Perf == 0 || wide.Context == 0 || wide.Cost == 0 || wide.Efforts == 0 {
		t.Fatalf("wide plan dropped a column: %+v", wide)
	}

	// Space runs out: cost disappears first, then context; intelligence is the last column kept.
	cramped := PlanModelMetricColumns(30, []ModelMetricCells{cells})
	if cramped.Intelligence == 0 {
		t.Errorf("intelligence dropped at width 30: %+v", cramped)
	}
	for name, columnWidth := range map[string]int{"cost": cramped.Cost, "context": cramped.Context, "perf": cramped.Perf, "efforts": cramped.Efforts} {
		if columnWidth != 0 {
			t.Errorf("%s survived at width 30: %d", name, columnWidth)
		}
	}
	row := widthx.StripTerminalSequences(strings.Join(NewModelMetricRows([]ModelMetricRow{
		{Left: "→ deepseek-v4.1-flash-1017", Model: model},
	}).Render(30), "\n"))
	if !strings.Contains(row, "int 62") {
		t.Errorf("narrow row %q lacks the intelligence column", row)
	}
	if strings.Contains(row, "ctx") {
		t.Errorf("narrow row %q kept the context column", row)
	}
	if visible := widthx.VisibleWidth(row); visible > 30 {
		t.Errorf("narrow row is %d cells wide, want at most 30", visible)
	}

	// Too narrow for any aligned block: the identity text alone, still truncated to the width.
	empty := PlanModelMetricColumns(20, []ModelMetricCells{cells})
	if empty != (ModelMetricWidths{}) {
		t.Errorf("width 20 plan = %+v, want no columns", empty)
	}
	emptyRow := widthx.StripTerminalSequences(strings.Join(NewModelMetricRows([]ModelMetricRow{
		{Left: "→ deepseek-v4.1-flash-1017", Model: model},
	}).Render(20), "\n"))
	if !strings.HasPrefix(emptyRow, "→ deepseek") {
		t.Errorf("identity row = %q", emptyRow)
	}
	if visible := widthx.VisibleWidth(emptyRow); visible > 20 {
		t.Errorf("identity row is %d cells wide, want at most 20", visible)
	}
}

func TestModelMetricCellsHideUnsuppliedValues(t *testing.T) {
	model := metricTestModel("plain-model", func(model *ai.Model) {
		model.Capabilities.InputCostPer1M = 0.5
		model.Capabilities.OutputCostPer1M = 1
	})
	cells := GetModelMetricCells(model)
	if cells.Intelligence != "" || cells.Perf != "" || cells.Efforts != "" {
		t.Errorf("cells invented values: %+v", cells)
	}
	if cells.Context != "128k ctx" {
		t.Errorf("context = %q", cells.Context)
	}
	if cells.Cost != "$0.5/1" {
		t.Errorf("cost = %q", cells.Cost)
	}
	row := widthx.StripTerminalSequences(strings.Join(NewModelMetricRows([]ModelMetricRow{
		{Left: "  plain-model", Model: model},
	}).Render(120), "\n"))
	if strings.Contains(row, "int ") || strings.Contains(row, "t/s") {
		t.Errorf("row %q shows metrics the catalog never supplied", row)
	}
}

func TestModelDetailFactsKeepDroppedColumns(t *testing.T) {
	model := metricTestModel("deepseek-ai/deepseek-v4.1-flash-1017", func(model *ai.Model) {
		model.DisplayName = "DeepSeek V4.1 Flash 1017"
		model.ProviderMeta.Reasoning = true
		model.Input = []string{"text", "image"}
		model.Capabilities.ContextWindow = 1_000_000
		model.Capabilities.MaxOutputTokens = 384_000
		model.Capabilities.InputCostPer1M = 0.3
		model.Capabilities.OutputCostPer1M = 1.2
		model.Capabilities.CacheReadCostPer1M = 0.006
		model.Capabilities.CacheWriteCostPer1M = 0.012
		low, high := "low", "high"
		model.ThinkingLevelMap = ai.ThinkingLevelMap{ai.ThinkingLow: &low, ai.ThinkingHigh: &high, ai.ThinkingXHigh: nil}
		model.Int = floatPointer(62)
		model.Description = "Flagship latency-first model"
	})

	facts := GetModelDetailFacts(model)
	for _, want := range []string{"1M ctx", "384k out", "$0.3 in", "$1.2 out", "$0.006 cache read", "$0.01 cache write", "reasoning", "input: text · image", "int 62", "Flagship latency-first model"} {
		if !strings.Contains(facts.Summary, want) {
			t.Errorf("summary %q lacks %q", facts.Summary, want)
		}
	}
	if facts.Efforts != "low · high" {
		t.Errorf("efforts = %q, want %q", facts.Efforts, "low · high")
	}
	if len(facts.Facts) == 0 || facts.Facts[len(facts.Facts)-1] != "Efforts: low · high" {
		t.Errorf("facts = %v", facts.Facts)
	}

	// The detail block still lists the metrics at a width where every column is gone.
	rendered := widthx.StripTerminalSequences(strings.Join(NewModelDetailFacts(model, "").Render(40), "\n"))
	if !strings.Contains(rendered, "1M ctx") || !strings.Contains(rendered, "int 62") {
		t.Errorf("detail block %q lost a fact", rendered)
	}
}

func TestModelDetailFactsWrapInsteadOfTruncating(t *testing.T) {
	description := "A very long provider blurb that must survive narrow terminals without losing its tail"
	longName := "Extremely detailed model name that does not fit"
	model := metricTestModel("short-id", func(model *ai.Model) {
		model.DisplayName = longName
		model.Description = description
	})
	rendered := strings.Join(NewModelDetailFacts(model, "").Render(24), "\n")
	rendered = widthx.StripTerminalSequences(rendered)
	flattened := strings.Join(strings.Fields(rendered), " ")
	if !strings.Contains(flattened, longName) {
		t.Errorf("wrapped block %q lost the name", flattened)
	}
	if !strings.Contains(flattened, "without losing its tail") {
		t.Errorf("wrapped block %q lost the blurb tail", flattened)
	}
	if strings.Contains(flattened, "...") {
		t.Errorf("wrapped block %q truncated a value", flattened)
	}
}

func TestSanitizeModelTextAndUnavailableDetail(t *testing.T) {
	if got := SanitizeModelText("  Fast\x1b[31m model \n v2\t"); got != "Fast model v2" {
		t.Errorf("sanitize = %q, want %q", got, "Fast model v2")
	}
	if got := SanitizeModelText(""); got != "" {
		t.Errorf("sanitize of empty = %q", got)
	}
	if efforts := GetSupportedEfforts(metricTestModel("no-thinking", nil)); len(efforts) != 0 {
		t.Errorf("efforts = %v, want none", efforts)
	}
	off := "off"
	high := "high"
	withThinking := metricTestModel("with-thinking", func(model *ai.Model) {
		model.ThinkingLevelMap = ai.ThinkingLevelMap{ai.ThinkingOff: &off, ai.ThinkingHigh: &high}
	})
	if efforts := strings.Join(GetSupportedEfforts(withThinking), ","); efforts != "off,high" {
		t.Errorf("efforts = %q, want %q", efforts, "off,high")
	}
	detail := widthx.StripTerminalSequences(strings.Join(NewModelDetailFacts(nil, "Model unavailable").Render(80), "\n"))
	if !strings.Contains(detail, "Model unavailable") {
		t.Errorf("unavailable detail = %q", detail)
	}
}

func TestFormatTokenCountAndFreeCost(t *testing.T) {
	cases := map[float64]string{999: "999", 8_192: "8.2k", 1_000_000: "1M", 1_500_000: "1.5M"}
	for count, want := range cases {
		if got := FormatTokenCount(count); got != want {
			t.Errorf("FormatTokenCount(%v) = %q, want %q", count, got, want)
		}
	}
	free := metricTestModel("free-model", nil)
	if cells := GetModelMetricCells(free); cells.Cost != "free" {
		t.Errorf("free model cost = %q, want %q", cells.Cost, "free")
	}
	if facts := GetModelDetailFacts(free); !strings.Contains(facts.Summary, "free") {
		t.Errorf("free model summary = %q", facts.Summary)
	}
}

func TestFormatCostLegMicroPrices(t *testing.T) {
	// A micro price keeps its significant digits instead of rounding to zero.
	if got := formatCostLeg(0.0012); got != "0.0012" {
		t.Errorf("micro price = %q, want %q", got, "0.0012")
	}
	if got := formatCostLeg(0.25); got != "0.25" {
		t.Errorf("price = %q, want %q", got, "0.25")
	}
	if got := formatCostLeg(12.5); got != "12.5" {
		t.Errorf("price = %q, want %q", got, "12.5")
	}
	if got := formatCostLeg(120.5); got != "121" {
		t.Errorf("price = %q, want %q", got, "121")
	}
	if got := formatCostLeg(-1); got != "?" {
		t.Errorf("negative price = %q, want %q", got, "?")
	}
}
