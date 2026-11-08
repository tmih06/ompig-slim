package codingagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// searchCard renders a finished search card the way the interactive mode wires
// one, so the test sees the rows a user sees.
func searchCard(t *testing.T, name, args, output string, expanded bool) []string {
	t.Helper()
	card := NewToolRendererCard(context.Background(), name, "c", "", json.RawMessage(args), CreateAllToolRenderers()[name], nil)
	t.Cleanup(card.Dispose)
	c := card.Component
	c.MarkExecutionStarted()
	c.StartedAt = time.Now().Add(-time.Second)
	c.SetArgsComplete()
	result := agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: output}}}
	final := ToolExecutionResultOf(result, time.Second)
	final.DurationMs = new(int64(900))
	c.SetResultValue(result)
	c.UpdateResult(final, false)
	c.SetExpanded(expanded)
	return c.Render(90)
}

// grepFixture is grepFormatter's output shape: one `path:line: text` row per
// match, with context rows using dashes.
const grepFixture = "skills/code-review/SKILL.md:2: name: code-review\n" +
	"skills/codebase-design/SKILL.md:2: name: codebase-design\n" +
	"skills/pr/SKILL.md:2: name: pr\n" +
	"skills/simplify/SKILL.md:2: name: simplify\n" +
	"skills/optimize/SKILL.md:2: name: optimize\n" +
	"skills/handoff/SKILL.md:2: name: handoff"

// TestGrepCardDrawsGroupedTree checks oh-my-pi's grouped search tree: one
// branch per file with its match under it, the counts on the status line, and a
// trailing summary of what a collapsed card hides.
func TestGrepCardDrawsGroupedTree(t *testing.T) {
	out := widthx.StripAnsi(strings.Join(searchCard(t, "grep", `{"pattern":"frontmatter|^name:","path":"skills"}`, grepFixture, false), "\n"))
	for _, want := range []string{
		"Grep: frontmatter|^name: in skills",
		"6 matches · 6 files",
		"├ skills/code-review/SKILL.md",
		"│  *2│name: code-review",
		"└ … 2 more matches",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	// The collapsed cap keeps the first files and summarises the rest.
	if strings.Contains(out, "skills/optimize/SKILL.md") {
		t.Fatalf("collapsed card drew a file past its cap:\n%s", out)
	}
}

// TestGrepCardExpandedShowsEveryMatch checks the expanded card draws every file
// and drops the summary.
func TestGrepCardExpandedShowsEveryMatch(t *testing.T) {
	out := widthx.StripAnsi(strings.Join(searchCard(t, "grep", `{"pattern":"frontmatter|^name:","path":"skills"}`, grepFixture, true), "\n"))
	for _, want := range []string{"skills/optimize/SKILL.md", "skills/handoff/SKILL.md"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expanded card is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "more match") {
		t.Fatalf("expanded card kept the summary:\n%s", out)
	}
	// The last root draws `└`.
	if !strings.Contains(out, "└ skills/handoff/SKILL.md") {
		t.Fatalf("the final root is not the last branch:\n%s", out)
	}
}

// TestFindCardDrawsFileTree checks find's card: quoted subject, the glob on the
// status line, and one branch per file with no child rows.
func TestFindCardDrawsFileTree(t *testing.T) {
	output := "skills/optimize/SKILL.md\nskills/simplify/SKILL.md\nskills/pr/SKILL.md\nskills/implement-spec/SKILL.md\nskills/to-tickets/SKILL.md\nskills/writing-for-agents/SKILL.md"
	out := widthx.StripAnsi(strings.Join(searchCard(t, "find", `{"pattern":"skills/**/SKILL.md","path":"skills"}`, output, false), "\n"))
	for _, want := range []string{
		"Find: skills/**/SKILL.md in skills",
		"6 files",
		"├ skills/optimize/SKILL.md",
		"└ … 2 more files",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
}

// TestParseGrepRow covers the separators grepFormatter emits and the rows it
// does not produce, which must not be mistaken for matches.
func TestParseGrepRow(t *testing.T) {
	for _, test := range []struct {
		row              string
		path, line, text string
		ok               bool
	}{
		{"a.go:12: TODO fix", "a.go", "12", "TODO fix", true},
		{"a.go-12- context", "a.go", "12", "context", true},
		{"dir:x/a.go:3: hit", "dir:x/a.go", "3", "hit", true},
		{"[Showing lines 1-10 of 20.]", "", "", "", false},
		{"No matches found", "", "", "", false},
	} {
		got, ok := parseGrepRow(test.row)
		if ok != test.ok {
			t.Fatalf("%q parsed=%v, want %v", test.row, ok, test.ok)
		}
		if !ok {
			continue
		}
		if got.Path != test.path || got.Line != test.line || got.Text != test.text {
			t.Fatalf("%q = %+v", test.row, got)
		}
	}
}

// TestRenderTreeListGeometry covers the branch rails: every root but the last
// draws `├`, the last draws `└`, and children keep the vertical rail.
func TestRenderTreeListGeometry(t *testing.T) {
	rows := tui.RenderTreeList([]tui.TreeItem{
		{Label: "a", Children: []string{"a1", "a2"}},
		{Label: "b"},
	}, tui.TreeListOptions{})
	want := []string{"├ a", "│  a1", "│  a2", "└ b"}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d", len(rows), len(want))
	}
	for i, row := range rows {
		if got := widthx.StripAnsi(row); got != want[i] {
			t.Fatalf("row %d = %q, want %q", i, got, want[i])
		}
	}
}

// TestCountLabel covers the pluralisation the summaries use.
func TestCountLabel(t *testing.T) {
	for _, test := range []struct {
		noun string
		n    int
		want string
	}{
		{"match", 1, "1 match"}, {"match", 21, "21 matches"},
		{"file", 1, "1 file"}, {"file", 20, "20 files"},
		{"hit", 3, "3 hits"},
	} {
		if got := countLabel(test.noun, test.n); got != test.want {
			t.Fatalf("countLabel(%q, %d) = %q, want %q", test.noun, test.n, got, test.want)
		}
	}
}
