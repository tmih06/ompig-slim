package codingagent

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
// path too long for its half of line 1 is elided in the middle, so the segments
// that name the project survive, while the model keeps its full name.
func TestFooterCompactsTheProjectPathInsteadOfTheModel(t *testing.T) {
	previous := tui.ActiveTheme()
	t.Cleanup(func() { tui.SetThemeInstance(previous) })
	tui.SetThemeInstance(footerTheme(t))

	full := widthx.StripTerminalSequences(renderFooter(footerLayoutFixture(t), 200)[0])
	if !strings.Contains(full, "…/acme/platform/project-alpha") {
		t.Fatalf("a wide footer did not show the path's tail: %q", full)
	}

	compacted := widthx.StripTerminalSequences(renderFooter(footerLayoutFixture(t), 80)[0])
	if !strings.Contains(compacted, "…/project-alpha") {
		t.Fatalf("the compacted path lost its leaf: %q", compacted)
	}
	if strings.Contains(compacted, "clients") {
		t.Fatalf("the elided segments survived compaction: %q", compacted)
	}
	// The branch, the session name and the model are what the path gave way for.
	for _, want := range []string{"feat-project-dir-collapse", "footer layout", "claude-sonnet-4"} {
		if !strings.Contains(compacted, want) {
			t.Fatalf("compaction dropped %q as well: %q", want, compacted)
		}
	}
}

// TestFooterShortensTheProjectPathToItsTail pins the standing cap: a checkout
// deeper than the last few segments shows where it is rather than the whole
// ancestry, which is what makes two checkouts of the same project distinguishable
// within a footer row. The cap holds at every width, so it is not compaction.
func TestFooterShortensTheProjectPathToItsTail(t *testing.T) {
	previous := tui.ActiveTheme()
	t.Cleanup(func() { tui.SetThemeInstance(previous) })
	tui.SetThemeInstance(footerTheme(t))

	t.Setenv("HOME", "/home/someone")
	// A worktree checkout is the case that motivates the cap: the fixed prefix is
	// most of the path, and only its tail says which checkout this is.
	d := footerData{
		cwd:       "/home/someone/.herdr/worktrees/ompig-slim/feat-git-status-show-both",
		gitBranch: "feat-git-status-show-both",
	}
	line := widthx.StripTerminalSequences(renderFooter(d, 200)[0])
	if !strings.Contains(line, "…/worktrees/ompig-slim/feat-git-status-show-both") {
		t.Fatalf("the path was not shortened to its last three segments: %q", line)
	}
	if strings.Contains(line, ".herdr") || strings.Contains(line, "ubuntu") {
		t.Fatalf("the path kept the segments before its tail: %q", line)
	}

	// Short paths keep every segment: the cap only removes what a reader does not
	// need, so a shallow checkout still reads as the home-relative path it had.
	for _, tc := range []struct{ cwd, want string }{
		{"/home/someone/ompig-slim", "~/ompig-slim"},
		{"/home/someone", "~"},
		{"/tmp/work", "/tmp/work"},
		{"/home/someone/a/b/c", "…/a/b/c"},
	} {
		short := shortenFooterPath(formatCwdForFooter(tc.cwd, "/home/someone"))
		if short != tc.want {
			t.Fatalf("shortenFooterPath(%q) = %q, want %q", tc.cwd, short, tc.want)
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

// TestParseGitStatusPorcelainV2 covers the reader's parsing of the shapes git
// emits, using the exact lines a repository produces. The header is the part the
// working-tree counts cannot stand in for: it is where an unpushed commit appears,
// and where a branch with no upstream reports nothing rather than zero.
func TestParseGitStatusPorcelainV2(t *testing.T) {
	for _, tc := range []struct {
		name  string
		out   string
		count gitStatusCounts
	}{
		{
			// git writes the sides signed, so behind arrives as "-3". The
			// magnitudes are what the footer draws, on the sides the fields name.
			name:  "diverged both ways",
			out:   "# branch.oid 5ea4\n# branch.head main\n# branch.upstream origin/main\n# branch.ab +2 -3\n",
			count: gitStatusCounts{ahead: 2, behind: 3},
		},
		{
			name:  "in sync",
			out:   "# branch.ab +0 -0\n",
			count: gitStatusCounts{},
		},
		{
			// No upstream, an upstream whose ref is gone, and a detached HEAD all
			// omit branch.ab, so all three read as no divergence.
			name:  "no divergence header",
			out:   "# branch.oid d78d\n# branch.head (detached)\n",
			count: gitStatusCounts{},
		},
		{
			// The XY columns, not the file columns: a staged rename edited again is
			// one staged and one unstaged, and an untracked path is neither.
			name: "entry kinds",
			out: "# branch.ab +1 -0\n" +
				"1 M. N... 100644 100644 100644 b51e 79b3 b.txt\n" +
				"1 .M N... 100644 100644 100644 b51e 79b3 c.txt\n" +
				"2 RM N... 100644 100644 100644 9382 9382 R100 sp ace.txt\ta.txt\n" +
				"? un tracked.txt\n" +
				"fatal: not a status line\n",
			count: gitStatusCounts{ahead: 1, staged: 2, unstaged: 2, untracked: 1},
		},
		{
			// A header this reader cannot parse contributes nothing rather than
			// failing the whole summary; the working-tree counts still stand.
			name:  "unparseable header",
			out:   "# branch.ab nonsense\n? a.txt\n",
			count: gitStatusCounts{untracked: 1},
		},
	} {
		if got := parseGitStatusPorcelainV2(tc.out); got != tc.count {
			t.Fatalf("%s: parsed %+v, want %+v", tc.name, got, tc.count)
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

// TestFooterShowsCommitsTheUpstreamDoesNotHave covers the divergence segment: a
// clean tree with unpushed commits is a state the working-tree counts cannot show,
// and it is the one that costs work if the checkout is lost.
func TestFooterShowsCommitsTheUpstreamDoesNotHave(t *testing.T) {
	previous := tui.ActiveTheme()
	t.Cleanup(func() { tui.SetThemeInstance(previous) })
	tui.SetThemeInstance(footerTheme(t))

	for _, tc := range []struct {
		name   string
		status gitStatusCounts
		want   string
	}{
		// Only the non-zero side is drawn, so the common cases stay one glyph.
		{"ahead only", gitStatusCounts{ahead: 2}, "feature/x ⇡2)"},
		{"behind only", gitStatusCounts{behind: 1}, "feature/x ⇣1)"},
		// The divergence is the branch's, so it precedes the working tree's counts
		// and the pull request that reviews the pushed commit.
		{"diverged and dirty", gitStatusCounts{ahead: 2, behind: 3, staged: 1, unstaged: 4, untracked: 5}, "feature/x ⇡2 ⇣3 *4 +1 ?5)"},
		// In sync, or no upstream at all: nothing to say about a distance of zero.
		{"in sync", gitStatusCounts{}, "feature/x)"},
	} {
		line := widthx.StripTerminalSequences(renderFooter(footerData{
			cwd:       "/work/repo",
			gitBranch: "feature/x",
			gitStatus: tc.status,
		}, 120)[0])
		if !strings.Contains(line, tc.want) {
			t.Fatalf("%s: footer shows %q, want %q", tc.name, line, tc.want)
		}
	}
}

// TestFooterDivergenceKeepsTheUsageArrowsDistinct guards the glyph choice: line 2
// spends ↑ and ↓ on the input and output counters, so divergence uses ⇡ and ⇣. A
// repeated arrow on the row under the prompt would read as one more usage stat.
func TestFooterDivergenceKeepsTheUsageArrowsDistinct(t *testing.T) {
	divergence := widthx.StripTerminalSequences(footerDivergence(gitStatusCounts{ahead: 1, behind: 1}))
	if strings.ContainsAny(divergence, "↑↓") {
		t.Fatalf("divergence reuses the usage arrows: %q", divergence)
	}
	if want := "⇡1 ⇣1"; divergence != want {
		t.Fatalf("divergence = %q, want %q", divergence, want)
	}
}

// TestFooterReadsUpstreamDivergenceFromGit covers the end-to-end divergence
// reader against a real repository with a real upstream: a commit that was made
// and not pushed is invisible to the working-tree counts, so this is the state the
// porcelain v2 header exists to report.
func TestFooterReadsUpstreamDivergenceFromGit(t *testing.T) {
	dir, remote := t.TempDir(), t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = dir
		// Fixture identity, so the test does not depend on a global git config.
		command.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=footer", "GIT_AUTHOR_EMAIL=footer@example.com",
			"GIT_COMMITTER_NAME=footer", "GIT_COMMITTER_EMAIL=footer@example.com")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	git("init", "-b", "divergence-test")
	git("init", "--bare", remote)
	git("remote", "add", "origin", remote)
	write("tracked.txt", "one\n")
	git("add", "tracked.txt")
	git("commit", "-m", "one")
	git("push", "-u", "origin", "divergence-test")

	// A clean tree with nothing to push reports no divergence at all.
	footer := NewFooterComponent(nil, "", nil)
	footer.SetCwd(dir)
	if line := widthx.StripTerminalSequences(footer.Render(120)[0]); strings.ContainsAny(line, "⇡⇣") {
		t.Fatalf("a branch in sync reported divergence: %q", line)
	}

	// Two commits made locally and never pushed: the tree is still clean.
	write("tracked.txt", "two\n")
	git("commit", "-am", "two")
	write("tracked.txt", "three\n")
	git("commit", "-am", "three")

	// The provider caches for a second, so wait it out rather than reach inside.
	time.Sleep(1100 * time.Millisecond)
	line := widthx.StripTerminalSequences(footer.Render(120)[0])
	if !strings.Contains(line, "divergence-test ⇡2") {
		t.Fatalf("two unpushed commits were not reported: %q", line)
	}
	// A clean tree keeps the working-tree counts quiet, which is what makes the
	// divergence the only thing on the branch.
	if !strings.Contains(line, "(divergence-test ⇡2)") {
		t.Fatalf("the working-tree counts drew on a clean tree: %q", line)
	}
}

// TestFooterPullRequestShowsCISummary keeps the PR indicator compact and omits zero CI buckets.
func TestFooterPullRequestShowsCISummary(t *testing.T) {
	line := widthx.StripTerminalSequences(renderFooter(footerData{
		cwd:         "/work/repo",
		gitBranch:   "feature/status",
		pullRequest: pullRequestStatus{number: 42, green: 3, red: 1, pending: 2},
	}, 120)[0])
	if !strings.Contains(line, "feature/status ↗#42 ✓3 ✗1 …2") {
		t.Fatalf("pull request status missing from footer: %q", line)
	}
	clean := widthx.StripTerminalSequences(footerPullRequest(pullRequestStatus{number: 7, green: 2}))
	if clean != "↗#7 ✓2" {
		t.Fatalf("zero CI buckets were not omitted: %q", clean)
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
