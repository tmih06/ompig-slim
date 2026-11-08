package codingagent

// Ports oh-my-pi's grouped search output for the grep and find result cards:
//
//	packages/tui/src/tools/grouped-file-output.ts
//	  groupLineIndicesByBlank, groupsByFile, classifyGroupedLines, isMatchRow
//	packages/tui/src/tools/grep.ts     describeResult (tree of files + matches)
//	packages/tui/src/tools/find.ts     describeResult (tree of hits)
//
// The Go search tools return flat `path:line: text` rows. oh-my-pi draws them
// as a tree: one branch per file, that file's matches hanging under it, and a
// trailing `… N more matches` while collapsed. These helpers recover that
// grouping without changing what the tools return to the model.

import (
	"strconv"
	"strings"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/tui"
)

// searchMatchRow is one `path:line:text` or `path-line-text` row of grep
// output, the shapes grepFormatter emits (grep.ts:198-277).
type searchMatchRow struct {
	Path string
	Line string
	Text string
}

// parseGrepRow splits a grep output row. grepFormatter writes matches as
// `path:line: text` and context rows as `path-line- text`; a row matching
// neither is output the parser leaves alone.
func parseGrepRow(row string) (searchMatchRow, bool) {
	// The separator after the path is the first one whose remainder starts with
	// a line number, so a path containing a colon still parses.
	for i := 0; i < len(row); i++ {
		c := row[i]
		if c != ':' && c != '-' {
			continue
		}
		rest := row[i+1:]
		digits := 0
		for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
			digits++
		}
		if digits == 0 || i == 0 {
			continue
		}
		if digits >= len(rest) || (rest[digits] != ':' && rest[digits] != '-') {
			continue
		}
		number := rest[:digits]
		text := strings.TrimPrefix(rest[digits+1:], " ")
		return searchMatchRow{
			Path: row[:i],
			Line: number,
			Text: text,
		}, true
	}
	return searchMatchRow{}, false
}

// groupGrepRows walks grep output and groups the rows by file, keeping the
// order the tool printed them in. Rows that do not parse (a notice, an empty
// line, an unreadable-file marker) are attached to the file they follow so no
// result is silently dropped.
func groupGrepRows(rows []string) []tui.TreeSearchFile {
	var files []tui.TreeSearchFile
	index := map[string]int{}
	for _, row := range rows {
		if strings.TrimSpace(row) == "" {
			continue
		}
		parsed, ok := parseGrepRow(row)
		if !ok {
			// Unparsed output belongs to the file it follows.
			if len(files) == 0 {
				files = append(files, tui.TreeSearchFile{})
			}
			last := &files[len(files)-1]
			last.Lines = append(last.Lines, outputLineStyle(row))
			continue
		}
		at, seen := index[parsed.Path]
		if !seen {
			at = len(files)
			index[parsed.Path] = at
			files = append(files, tui.TreeSearchFile{Label: fileLabel(parsed.Path)})
		}
		match := matchLineStyle(parsed.Line, parsed.Text)
		files[at].Lines = append(files[at].Lines, match)
	}
	return files
}

// fileLabel is a search result's file row: the path in the tool's title color.
func fileLabel(path string) string {
	return tui.ActiveTheme().Fg("accent", path)
}

// matchLineStyle is one matched row inside a file: `*12│text`, the marker and
// line number dim and the text in the tool-output color, as oh-my-pi's
// code-frame rows read.
func matchLineStyle(line, text string) string {
	theme := tui.ActiveTheme()
	return theme.Fg("dim", "*"+line+"│") + theme.Fg("toolOutput", text)
}

// outputLineStyle styles a row the parser did not recognize.
func outputLineStyle(row string) string {
	return tui.ActiveTheme().Fg("toolOutput", row)
}

// countGrepMatches totals the parsed match rows, for the status line's
// `N matches · M files` meta.
func countGrepMatches(rows []string) (matches, files int) {
	seen := map[string]struct{}{}
	for _, row := range rows {
		parsed, ok := parseGrepRow(row)
		if !ok {
			continue
		}
		matches++
		if _, dup := seen[parsed.Path]; !dup {
			seen[parsed.Path] = struct{}{}
			files++
		}
	}
	return matches, files
}

// searchCounts is the status line's `N matches · M files` for a finished
// search, as grep.ts's head carries. find returns file paths, so its count is
// of files.
func searchCounts(name string, result agent.AgentToolResult) []string {
	rows := strings.Split(shellTextOutput(result.Text()), "\n")
	if name == "find" {
		files := 0
		for _, row := range rows {
			if strings.TrimSpace(row) != "" {
				files++
			}
		}
		if files == 0 {
			return nil
		}
		return []string{countLabel("file", files)}
	}
	matches, files := countGrepMatches(rows)
	if matches == 0 {
		return nil
	}
	meta := []string{countLabel("match", matches)}
	if files > 1 {
		meta = append(meta, countLabel("file", files))
	}
	return meta
}

// countLabel is `N noun`, pluralised: the names the search summaries use take
// `es` when they end in a sibilant, so `1 match` and `21 matches`.
func countLabel(noun string, n int) string {
	suffix := "s"
	switch {
	case strings.HasSuffix(noun, "ch"), strings.HasSuffix(noun, "sh"),
		strings.HasSuffix(noun, "s"), strings.HasSuffix(noun, "x"):
		suffix = "es"
	}
	if n == 1 {
		suffix = ""
	}
	return strconv.Itoa(n) + " " + noun + suffix
}
