package codingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// footerTheme is a theme whose every token is a distinct color, so a test can
// tell which token a segment was drawn with. It is built from theme JSON, the
// public path, so the fixture cannot drift from how a real theme is resolved.
func footerTheme(t *testing.T) *tui.Theme {
	t.Helper()
	brightness := 0
	color := func() string {
		brightness++
		return "#" + twoHex(brightness) + twoHex(brightness) + twoHex(brightness)
	}
	document := map[string]any{"name": "footer-test", "colors": map[string]any{}}
	colors := document["colors"].(map[string]any)
	for _, token := range tui.ThemeColorTokens() {
		colors[token.Name] = color()
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode fixture theme: %v", err)
	}
	path := filepath.Join(t.TempDir(), "footer-test.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write fixture theme: %v", err)
	}
	// LoadThemeFromPath with an explicit mode, not LoadThemeFile: the latter reads
	// GetTerminalColorMode(), so this fixture would resolve in truecolor on a
	// developer's terminal and in 256-color on a CI runner with no COLORTERM. The
	// #NNNNNN grey ramp below is only distinct in truecolor -- 256-color spaces its
	// grey ramp 10 levels apart, so 63 steps collapse onto ~24 entries and tokens a
	// test expects to differ share an escape sequence.
	theme, err := tui.LoadThemeFromPath(path, tui.TerminalColorModeTrueColor)
	if err != nil {
		t.Fatalf("load fixture theme: %v", err)
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
