package tui

// Ports oh-my-pi's tree list rendering for search results:
//
//	packages/tui/src/render/tree-list.ts   renderTreeList
//	packages/tui/src/render/utils.ts       getTreeBranch, getTreeContinuePrefix
//
// A search result reads as a tree of files, each file's matched lines hanging
// under it:
//
//	├─ skills/code-review/SKILL.md
//	│  *2│name: code-review
//	├─ skills/codebase-design/SKILL.md
//	│  *2│name: codebase-design
//	└─ … 19 more matches
//
// The branch glyph is a continuation marker, not a nesting level: the first
// N-1 roots draw `├`, the last draws `└`, and a multi-row item prefixes its
// continuation rows with the vertical rail so the entries stay aligned.

import (
	"fmt"
	"strings"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TreeItem is one root of a tree list: a label row and the rows that hang
// under it.
type TreeItem struct {
	// Label is the item's first row, drawn after the branch glyph.
	Label string
	// Children are the rows under the label, each prefixed with the
	// continuation rail so a multi-row item reads as one entry.
	Children []string
}

// TreeListOptions controls a tree list's collapse policy.
type TreeListOptions struct {
	Expanded bool
	// MaxCollapsed caps the roots drawn while collapsed.
	MaxCollapsed int
	// ItemType names the hidden roots in the trailing summary ("match",
	// "file", "hit"). Empty disables the summary.
	ItemType string
}

// RenderTreeList draws a tree list with themed branches and an optional
// trailing `… N more <itemType>` row while collapsed.
//
// The budget is per root, not per row: a root and all its children are kept
// or dropped together, so a file never appears without its matches. The
// summary replaces the last drawn root when it would otherwise be the only
// one — matching oh-my-pi, which renders the summary as the final root and
// leaves the preceding roots on `├`.
func RenderTreeList(items []TreeItem, options TreeListOptions) []string {
	theme := ActiveTheme()
	branch := theme.Fg("dim", "├ ")
	last := theme.Fg("dim", "└ ")
	rail := theme.Fg("dim", "│  ")

	shown := items
	hidden := 0
	if !options.Expanded && options.MaxCollapsed > 0 && len(items) > options.MaxCollapsed {
		shown = items[:options.MaxCollapsed]
		hidden = len(items) - options.MaxCollapsed
	}

	type root struct {
		item TreeItem
		// summary marks the trailing row, which draws `└` and takes no rail.
		summary bool
	}
	roots := make([]root, 0, len(shown)+1)
	for _, item := range shown {
		roots = append(roots, root{item: item})
	}
	if hidden > 0 && options.ItemType != "" {
		roots = append(roots, root{
			item: TreeItem{Label: theme.Fg("muted", fmt.Sprintf("… %d more %s%s",
				hidden, options.ItemType, pluralItems(options.ItemType, hidden)))},
			summary: true,
		})
	}

	out := make([]string, 0, len(roots)*2)
	for i, entry := range roots {
		isLast := i == len(roots)-1
		prefix := branch
		if isLast {
			prefix = last
		}
		out = append(out, prefix+entry.item.Label)
		if entry.summary {
			continue
		}
		for _, child := range entry.item.Children {
			out = append(out, rail+child)
		}
	}
	return out
}

// pluralItems pluralises the item names the search summaries use: the ones
// ending in `ch`/`sh`/`s` take `es`, the rest take `s`.
func pluralItems(itemType string, n int) string {
	if n == 1 {
		return ""
	}
	switch {
	case strings.HasSuffix(itemType, "ch"), strings.HasSuffix(itemType, "sh"),
		strings.HasSuffix(itemType, "s"), strings.HasSuffix(itemType, "x"):
		return "es"
	default:
		return "s"
	}
}

// TreeSearchFile is one file of a search result: its label and the matched
// lines under it.
type TreeSearchFile struct {
	Label string
	// Lines are the matched rows, already styled.
	Lines []string
}

// RenderSearchTree draws search results as oh-my-pi's grouped tree: one root
// per file, its matched lines under it, and a trailing summary naming the
// hidden matches while collapsed. It returns the rows and whether anything
// was drawn.
func RenderSearchTree(files []TreeSearchFile, options TreeListOptions) []string {
	items := make([]TreeItem, len(files))
	for i, file := range files {
		items[i] = TreeItem{Label: file.Label, Children: file.Lines}
	}
	return RenderTreeList(items, options)
}

// wrapTreeRows breaks a styled row to width, keeping the given prefix on the
// first row and aligning its continuation beneath the content.
func wrapTreeRows(prefix, indent, row string, width int) []string {
	available := max(1, width-widthx.VisibleWidth(prefix))
	wrapped := widthx.WrapTextWithAnsi(row, available)
	if len(wrapped) <= 1 {
		return []string{prefix + row}
	}
	out := make([]string, 0, len(wrapped))
	out = append(out, prefix+wrapped[0])
	for _, rest := range wrapped[1:] {
		out = append(out, indent+rest)
	}
	return out
}

// TruncateRow clips one already-styled row to width in terminal cells, keeping
// its escape sequences intact.
func TruncateRow(row string, width int) string {
	return widthx.TruncateToWidth(row, max(0, width), "…", true)
}
