package codingagent

import (
	"encoding/json"
	"os"
	"os/exec"
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
	// per-segment colors the moment a segment closed. Checked against the raw line:
	// stripping escapes first would remove the very wrapper this looks for.
	if strings.HasPrefix(lines[1], theme.GetFgAnsi("dim")) {
		t.Fatal("line 2 still starts with a dim wrapper")
	}
}

// footerLayoutFixture is a footer with every segment populated at once: a deep
// project path, a branch, a session name, a full set of usage counters and a
// reasoning model. Each layout test shrinks it to the width it cares about, and
// pins HOME to the fixture so the path under test is the abbreviated one a user
// sees rather than the absolute one a machine reports.
func footerLayoutFixture(t *testing.T) footerData {
	t.Helper()
	t.Setenv("HOME", "/home/someone")
	return footerData{
		model:              footerTestModel("claude-sonnet-4", 200000),
		cwd:                "/home/someone/work/clients/acme/platform/project-alpha",
		gitBranch:          "feat-project-dir-collapse",
		sessionName:        "footer layout",
		thinkingLevel:      "high",
		contextTokens:      64000,
		autoCompactEnabled: true,
		usage: footerUsageTotals{
			input: 12500, output: 3400, cacheRead: 900000, cacheWrite: 100, cost: 0.42,
			latestCacheHitRate: new(0.87),
		},
	}
}

// TestFooterPutsProjectAndModelOnOneLine is the reason the model left line 2: a
// phone-width terminal must show what it is working in and what it is working
// with on the row under the prompt, and leave line 2 wholly to the usage stats.
// It also pins right-alignment, which is what makes the model read as the line's
// other end rather than as another trailing stat.
func TestFooterPutsProjectAndModelOnOneLine(t *testing.T) {
	previous := tui.ActiveTheme()
	t.Cleanup(func() { tui.SetThemeInstance(previous) })
	tui.SetThemeInstance(footerTheme(t))

	for _, width := range []int{120, 80, 60, 44, 34} {
		lines := renderFooter(footerLayoutFixture(t), width)
		if len(lines) != 2 {
			t.Fatalf("width %d: footer has %d lines, want 2", width, len(lines))
		}
		project, model := widthx.StripTerminalSequences(lines[0]), widthx.StripTerminalSequences(lines[1])
		if !strings.Contains(project, "project-alpha") {
			t.Fatalf("width %d: line 1 does not name the project: %q", width, project)
		}
		if !strings.Contains(project, "claude-sonnet-4") {
			t.Fatalf("width %d: line 1 does not name the model: %q", width, project)
		}
		if strings.Contains(model, "claude-sonnet-4") {
			t.Fatalf("width %d: line 2 still carries the model: %q", width, model)
		}
		// Truncation may reach the later counters, never the leading pair: those are
		// what a narrow terminal keeps.
		if !strings.Contains(model, "↑13k") || !strings.Contains(model, "↓3.4k") {
			t.Fatalf("width %d: line 2 lost the usage stats: %q", width, model)
		}
		if got := widthx.VisibleWidth(project); got != width {
			t.Fatalf("width %d: line 1 is %d columns wide, want the full width: %q", width, got, project)
		}
		if !strings.HasSuffix(project, "claude-sonnet-4") {
			t.Fatalf("width %d: the model is not right-aligned: %q", width, project)
		}
	}
}

// TestFooterCompactsTheProjectPathInsteadOfTheModel covers the collapse rule: a
// path too long for its half of line 1 is elided in the middle, so the leaf that
// names the project and the home marker that places it both survive, while the
// model keeps its full name.
func TestFooterCompactsTheProjectPathInsteadOfTheModel(t *testing.T) {
	previous := tui.ActiveTheme()
	t.Cleanup(func() { tui.SetThemeInstance(previous) })
	tui.SetThemeInstance(footerTheme(t))

	full := widthx.StripTerminalSequences(renderFooter(footerLayoutFixture(t), 200)[0])
	if !strings.Contains(full, "~/work/clients/acme/platform/project-alpha") {
		t.Fatalf("a wide footer did not show the full path: %q", full)
	}

	compacted := widthx.StripTerminalSequences(renderFooter(footerLayoutFixture(t), 80)[0])
	if !strings.Contains(compacted, "~/…/project-alpha") {
		t.Fatalf("the compacted path lost its anchor or its leaf: %q", compacted)
	}
	if strings.Contains(compacted, "clients") {
		t.Fatalf("the middle segments survived compaction: %q", compacted)
	}
	// The branch, the session name and the model are what the path gave way for.
	for _, want := range []string{"feat-project-dir-collapse", "footer layout", "claude-sonnet-4"} {
		if !strings.Contains(compacted, want) {
			t.Fatalf("compaction dropped %q as well: %q", want, compacted)
		}
	}
}

// TestFooterDropsDecorationsBeforeThePath pins the order the project segment
// gives things up in. The path is what the row is for, so the session name goes
// first and the branch second; a shell prompt's project does not become unreadable
// to keep a conversation's name.
func TestFooterDropsDecorationsBeforeThePath(t *testing.T) {
	previous := tui.ActiveTheme()
	t.Cleanup(func() { tui.SetThemeInstance(previous) })
	tui.SetThemeInstance(footerTheme(t))

	for _, width := range []int{200, 120, 100, 90, 80, 72, 66, 60, 56, 52, 48, 44} {
		line := widthx.StripTerminalSequences(renderFooter(footerLayoutFixture(t), width)[0])
		if !strings.Contains(line, "project-alpha") && !strings.Contains(line, "project-…") {
			t.Fatalf("width %d: line 1 does not name the project: %q", width, line)
		}
		if strings.Contains(line, "claude") && !strings.Contains(line, "claude-sonnet-4") {
			// A cut model name reads as a different model, so it must stay whole on
			// line 1 and leave the row instead.
			t.Fatalf("width %d: line 1 truncated the model name: %q", width, line)
		}
	}

	// The decorations go in order, and each one only goes when the row needs it.
	stages := []struct {
		width        int
		name, branch bool
	}{
		{200, true, true},
		{80, true, true},
		{60, false, true},
		{44, false, false},
	}
	for _, stage := range stages {
		line := widthx.StripTerminalSequences(renderFooter(footerLayoutFixture(t), stage.width)[0])
		if got := strings.Contains(line, "footer layout"); got != stage.name {
			t.Fatalf("width %d: session name shown=%v, want %v: %q", stage.width, got, stage.name, line)
		}
		if got := strings.Contains(line, "feat-project-dir-collapse"); got != stage.branch {
			t.Fatalf("width %d: branch shown=%v, want %v: %q", stage.width, got, stage.branch, line)
		}
	}
}

// TestFooterFallsBackToUpstreamLayoutWhenTheModelCannotShareTheRow keeps the
// contract for terminals too narrow for a shared line 1: rather than lose either
// fact, the footer returns to upstream's two rows. The model keeps its place all
// the way down — it is the fact line 1 could not carry — so line 2 gives up the
// stats before it gives up the model.
func TestFooterFallsBackToUpstreamLayoutWhenTheModelCannotShareTheRow(t *testing.T) {
	previous := tui.ActiveTheme()
	t.Cleanup(func() { tui.SetThemeInstance(previous) })
	tui.SetThemeInstance(footerTheme(t))

	for _, width := range []int{18, 14, 10} {
		line1, line2 := footerLines(footerLayoutFixture(t), width)
		project, model := widthx.StripTerminalSequences(line1), widthx.StripTerminalSequences(line2)
		if !strings.Contains(project, "project") {
			t.Fatalf("width %d: line 1 does not name the project: %q", width, project)
		}
		if strings.Contains(project, "claude") {
			t.Fatalf("width %d: line 1 carries the model that did not fit: %q", width, project)
		}
		if !strings.Contains(model, "claude") {
			t.Fatalf("width %d: line 2 lost the model: %q", width, model)
		}
	}

	// At 18 columns the model still has the row to itself and is right-aligned;
	// below that it is truncated like any other row that cannot fit.
	if _, model := footerLines(footerLayoutFixture(t), 18); !strings.HasSuffix(widthx.StripTerminalSequences(model), "claude-sonnet-4") {
		t.Fatalf("the model is not right-aligned on the fallback row: %q", widthx.StripTerminalSequences(model))
	}
	// Once there is room for both, the stats keep their leading counters beside it.
	if _, model := footerLines(footerLayoutFixture(t), 40); !strings.Contains(widthx.StripTerminalSequences(model), "↑13k") {
		t.Fatalf("the fallback row dropped the stats while they still fit: %q", widthx.StripTerminalSequences(model))
	}
}

// TestFooterRowsNeverExceedTheWidth guards the terminal contract: an over-wide
// row wraps and corrupts every row drawn after it. The sweeps cover both layouts
// and the widths where they meet.
func TestFooterRowsNeverExceedTheWidth(t *testing.T) {
	previous := tui.ActiveTheme()
	t.Cleanup(func() { tui.SetThemeInstance(previous) })
	tui.SetThemeInstance(footerTheme(t))

	for width := 1; width <= 220; width++ {
		for _, line := range renderFooter(footerLayoutFixture(t), width) {
			if got := widthx.VisibleWidth(line); got > width {
				t.Fatalf("width %d: a footer row is %d columns wide: %q", width, got, widthx.StripTerminalSequences(line))
			}
		}
	}
}

// TestFooterReadsGitStatusFromWorkingTree covers the end-to-end status reader:
// staged, unstaged, and untracked files are summarized in the rendered footer.
func TestFooterReadsGitStatusFromWorkingTree(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = dir
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	git("init", "-b", "status-test")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("staged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "tracked.txt")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("unstaged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("untracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	footer := NewFooterComponent(nil, "", nil)
	footer.SetCwd(dir)
	line := widthx.StripTerminalSequences(footer.Render(120)[0])
	if !strings.Contains(line, "status-test *1 +1 ?1") {
		t.Fatalf("git status missing from footer: %q", line)
	}
}

// TestFooterGitStatusShowsWorkingTreeCounts covers the renderer's compact
// indicators independently from git process discovery.
func TestFooterGitStatusShowsWorkingTreeCounts(t *testing.T) {
	line := widthx.StripTerminalSequences(renderFooter(footerData{
		cwd:       "/work/repo",
		gitBranch: "feature/status",
		gitStatus: gitStatusCounts{staged: 2, unstaged: 3, untracked: 1},
	}, 120)[0])
	if !strings.Contains(line, "feature/status *3 +2 ?1") {
		t.Fatalf("git status missing from footer: %q", line)
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

// TestFirstRunHighlightMatchesPreview covers the first-run dialog's theme step: the
// highlighted option must be the theme the dialog previews, which is the appearance
// default. A fixed first entry would show Titanium selected over a light preview on a
// light terminal.
func TestFirstRunHighlightMatchesPreview(t *testing.T) {
	previous := tui.ActiveTheme()
	t.Cleanup(func() {
		tui.SetTerminalColorScheme("")
		tui.SetThemeInstance(previous)
	})

	for _, appearance := range []tui.TerminalTheme{"dark", "light"} {
		tui.SetTerminalColorScheme(appearance)
		component := NewFirstTimeSetupComponent(FirstTimeSetupOptions{})
		want := tui.DefaultThemeName(appearance)
		if got := firstTimeSetupThemeOptions[component.themeIndex].value; got != want {
			t.Fatalf("%s: highlighted %q, want the previewed %q", appearance, got, want)
		}
	}
}
