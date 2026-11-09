package codingagent

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// footerTheme is a theme whose every token is a distinct color, so a test can
// tell which token a segment was drawn with. NewTheme requires every token a
// component draws with, so the whole table is supplied rather than a subset.
func footerTheme(t *testing.T) *tui.Theme {
	t.Helper()
	var foregrounds, backgrounds []tui.ThemeTokenValue
	brightness := 0
	add := func(name string) tui.ThemeColorValue {
		brightness++
		return tui.ColorValueOf("#" + twoHex(brightness) + twoHex(brightness) + twoHex(brightness))
	}
	for _, token := range tui.ThemeColorTokens() {
		foregrounds = append(foregrounds, tui.ThemeTokenValue{Token: token.Name, Value: add(token.Name)})
	}
	for _, token := range []string{"selectedBg", "searchMatchBg", "userMessageBg", "customMessageBg", "toolPendingBg", "toolSuccessBg", "toolErrorBg"} {
		backgrounds = append(backgrounds, tui.ThemeTokenValue{Token: token, Value: add(token)})
	}
	theme, err := tui.NewTheme(foregrounds, backgrounds, tui.TerminalColorModeTrueColor, tui.ThemeOptions{Name: "footer-test"})
	if err != nil {
		t.Fatalf("build theme: %v", err)
	}
	return theme
}

// footerTestModel is a model whose context window is the number the footer
// reports, which is what the context column reads (footer.ts:162).
func footerTestModel(id string, window int) *ai.Model {
	model := &ai.Model{ID: id}
	model.Capabilities.ContextWindow = window
	return model
}

func twoHex(value int) string {
	digits := "0123456789abcdef"
	return string([]byte{digits[value/16%16], digits[value%16]})
}

// TestFooterSegmentsUseStatusLineTokens covers the footer's coloring: each
// segment is drawn in its own statusLine* token rather than the whole footer in
// one dim wash, which is what made it read as plain terminal text.
func TestFooterSegmentsUseStatusLineTokens(t *testing.T) {
	previous := tui.ActiveTheme()
	t.Cleanup(func() { tui.SetThemeInstance(previous) })
	theme := footerTheme(t)
	tui.SetThemeInstance(theme)

	lines := renderFooter(footerData{
		model:              footerTestModel("claude-sonnet-4", 128000),
		sessionName:        "testing",
		cwd:                "/tmp",
		gitBranch:          "main",
		contextTokens:      64000,
		autoCompactEnabled: true,
		usage: footerUsageTotals{
			input: 12500, output: 3400, cacheRead: 900000, cost: 0.42,
			latestCacheHitRate: new(0.87),
		},
	}, 100)
	if len(lines) != 2 {
		t.Fatalf("footer has %d lines", len(lines))
	}
	joined := strings.Join(lines, "\n")
	for token, want := range map[string]string{
		"statusLinePath":  "/tmp",
		"statusLineModel": "claude-sonnet-4",
		// The counters go through formatTokens, which abbreviates 12500 as 13k.
		"statusLineSpend":  "↑13k",
		"statusLineOutput": "↓3.4k",
		"statusLineCost":   "$0.420",
	} {
		if !strings.Contains(joined, theme.GetFgAnsi(token)+want) {
			t.Fatalf("%q is not drawn with %s:\n%q", want, token, joined)
		}
	}
	// The path and the model must not share a color; that is the whole point.
	if theme.GetFgAnsi("statusLinePath") == theme.GetFgAnsi("statusLineModel") {
		t.Fatal("path and model share a color")
	}
	// The footer must not be wrapped in one outer color, which would reset the
	// per-segment colors the moment a segment closed.
	if strings.HasPrefix(widthx.StripAnsi(lines[1]), theme.GetFgAnsi("dim")) {
		t.Fatal("line 2 still starts with a dim wrapper")
	}
}

// TestFooterContextThresholds covers the context window's color levels: the
// window is statusLineContext below 50%, warning above 50%, thinkingHigh above
// 70%, and error above 90%.
func TestFooterContextThresholds(t *testing.T) {
	previous := tui.ActiveTheme()
	t.Cleanup(func() { tui.SetThemeInstance(previous) })
	theme := footerTheme(t)
	tui.SetThemeInstance(theme)

	const window = 1000
	for _, want := range []struct {
		tokens int
		token  string
		pct    string
	}{
		{100, "statusLineContext", "10.0%"},
		{600, "warning", "60.0%"},
		{800, "thinkingHigh", "80.0%"},
		{950, "error", "95.0%"},
	} {
		lines := renderFooter(footerData{
			model:         footerTestModel("m", window),
			cwd:           "/tmp",
			contextTokens: want.tokens,
		}, 100)
		display := want.pct + "/1.0k"
		found := false
		for _, line := range lines {
			if strings.Contains(line, theme.GetFgAnsi(want.token)+display) {
				found = true
			}
		}
		if !found {
			t.Fatalf("%d/%d tokens did not draw the context window with %s:\n%q", want.tokens, window, want.token, strings.Join(lines, "\n"))
		}
	}
}
