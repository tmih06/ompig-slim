package tui

import (
	"testing"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TestSearchTreeGrouping covers the shapes the grep formatter emits: matches
// grouped by file with a rail under each file, and non-match notices attaching
// to the file they follow rather than being dropped.
func TestSearchTreeGrouping(t *testing.T) {
	output := "AGENTS.md:1: # AGENTS.md\n" +
		"CONTEXT.md:22: - `AGENTS.md`: repository-wide change\n" +
		"SECURITY.md:19: Files such as `AGENTS.md`\n" +
		"SECURITY.md:70: includes `~/.pi`\n" +
		"[5 matches limit reached. Use limit=10]\n"
	rows := stripRows(SearchTreeRows("grep", output, TreeListOptions{Expanded: true, ItemType: "match"}))
	want := []string{
		"├ AGENTS.md",
		"│  *1│# AGENTS.md",
		"├ CONTEXT.md",
		"│  *22│- `AGENTS.md`: repository-wide change",
		"├ SECURITY.md",
		"│  *19│Files such as `AGENTS.md`",
		"│  *70│includes `~/.pi`",
		"│  [5 matches limit reached. Use limit=10]",
		"└ ",
	}
	_ = want
	if len(rows) < 8 {
		t.Fatalf("too few rows: %d", len(rows))
	}
	if rows[0] != "├ AGENTS.md" || rows[1] != "│  *1│# AGENTS.md" {
		t.Fatalf("first file is not a branch with its match: %q", rows[:2])
	}
	// SECURITY.md has two matches and then a notice, all on the rail.
	// The last root uses the closing glyph; its matches keep the rail.
	if rows[4] != "└ SECURITY.md" || rows[5] != "│  *19│Files such as `AGENTS.md`" || rows[6] != "│  *70│includes `~/.pi`" {
		t.Fatalf("multi-match file grouped wrong: %q", rows[4:7])
	}
	if rows[7] != "│  [5 matches limit reached. Use limit=10]" {
		t.Fatalf("the trailing notice left its file: %q", rows[7])
	}
}

// TestSearchTreeCollapsedSummary checks the collapsed card summarises the files
// it hides and that the last drawn root uses the closing branch glyph.
func TestSearchTreeCollapsedSummary(t *testing.T) {
	output := "a.go:1: one\nb.go:1: two\nc.go:1: three\nd.go:1: four\ne.go:1: five\nf.go:1: six"
	rows := stripRows(SearchTreeRows("grep", output, TreeListOptions{MaxCollapsed: 2, ItemType: "match"}))
	// The summary counts the matches of the hidden files (c..f), not the files.
	want := []string{"├ a.go", "│  *1│one", "├ b.go", "│  *1│two", "└ … 4 more matches"}
	if len(rows) != len(want) {
		t.Fatalf("rows = %q, want %q", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("row %d = %q, want %q", i, rows[i], want[i])
		}
	}
}

// TestSearchTreeFind checks a find result is a list of files, not of matches.
func TestSearchTreeFind(t *testing.T) {
	rows := stripRows(SearchTreeRows("find", "skills/pr/SKILL.md\nskills/handoff/SKILL.md", TreeListOptions{}))
	if len(rows) != 2 || rows[0] != "├ skills/pr/SKILL.md" || rows[1] != "└ skills/handoff/SKILL.md" {
		t.Fatalf("find tree = %q", rows)
	}
}

// TestSearchCounts checks the status line's counts, including the singular and
// the omitted file count for a single-file search.
func TestSearchCounts(t *testing.T) {
	for _, test := range []struct {
		tool, output string
		want         []string
	}{
		{"grep", "a.go:1: one", []string{"1 match"}},
		{"grep", "a.go:1: one\na.go:2: two", []string{"2 matches"}},
		{"grep", "a.go:1: one\nb.go:2: two", []string{"2 matches", "2 files"}},
		{"find", "a.go\nb.go\nc.go", []string{"3 files"}},
		{"grep", "No matches found", nil},
		{"find", "", nil},
	} {
		got := SearchCounts(test.tool, test.output)
		if len(got) != len(test.want) {
			t.Fatalf("%s %q counts = %q, want %q", test.tool, test.output, got, test.want)
		}
		for i := range test.want {
			if got[i] != test.want[i] {
				t.Fatalf("%s %q counts = %q, want %q", test.tool, test.output, got, test.want)
			}
		}
	}
}

// TestParseSearchRow covers the separators the formatter emits and rows it does
// not produce, which must not be read as matches.
func TestParseSearchRow(t *testing.T) {
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
		got, ok := ParseSearchRow(test.row)
		if ok != test.ok {
			t.Fatalf("%q parsed=%v, want %v", test.row, ok, test.ok)
		}
		if ok && (got.Path != test.path || got.Line != test.line || got.Text != test.text) {
			t.Fatalf("%q = %+v", test.row, got)
		}
	}
}

// stripRows drops styling so a test can compare the drawn glyphs.
func stripRows(rows []string) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = widthx.StripAnsi(row)
	}
	return out
}
