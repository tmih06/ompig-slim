package codingagent

// file_mentions_card.go renders the rows an auto-read of `@path` mentions
// produced, directly under the submitted prompt:
//
//	├─ Read README.md (123 lines)
//	└─ Read src/ (12 entries)
//
// It is a native replacement for oh-my-pi's `fileMention` transcript block
// (packages/tui/src/chat/transcript-render-helpers.ts buildFileMentionBlock),
// which this runtime has no message role for: the mentions travel beside the
// prompt as a developer message, so the card owns its own rendering.

import (
	"fmt"

	"github.com/MichaelKinsy/PiG/internal/codingagent/tools"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// fileMentionCard draws one mentioned path per row as a tree, matching the
// read tool's status rows so the auto-read and a manual read look the same.
type fileMentionCard struct {
	mentions []FileMention
}

// Render draws the tree. A single row uses `└─`; several use `├─` and end with
// `└─`. Rows never wrap: the tree's rail only stays aligned when every entry is
// one row, so a path too wide for the terminal is truncated with `…`.
func (c *fileMentionCard) Render(width int) []string {
	if len(c.mentions) == 0 {
		return nil
	}
	theme := tui.ActiveTheme()
	rows := make([]string, 0, len(c.mentions))
	for i, mention := range c.mentions {
		branch := "├─ "
		if i == len(c.mentions)-1 {
			branch = "└─ "
		}
		detail := fileMentionDetail(mention)
		row := theme.Fg("dim", branch) +
			theme.Fg("muted", "Read ") +
			tui.ChalkBold(mention.Path) +
			theme.Fg("dim", " ("+detail+")")
		rows = append(rows, widthx.TruncateToWidth(row, max(1, width), "…", false))
	}
	return rows
}

// Invalidate is a no-op: the rows bake in the theme colors of the render that
// built them, as other native cards do.
func (c *fileMentionCard) Invalidate() {}

// fileMentionDetail is the parenthesised row detail: the line count for a file,
// the entry count for a directory, or why the mention was not read.
func fileMentionDetail(mention FileMention) string {
	switch mention.Skipped {
	case "tooLarge":
		return "skipped: " + tools.FormatSize(int(mention.ByteSize))
	case "binary":
		return "skipped: binary, " + tools.FormatSize(int(mention.ByteSize))
	}
	if mention.IsDirectory {
		switch mention.LineCount {
		case 0:
			return "empty"
		case 1:
			return "1 entry"
		}
		return fmt.Sprintf("%d entries", mention.LineCount)
	}
	if mention.LineCount == 1 {
		return "1 line"
	}
	return fmt.Sprintf("%d lines", mention.LineCount)
}
