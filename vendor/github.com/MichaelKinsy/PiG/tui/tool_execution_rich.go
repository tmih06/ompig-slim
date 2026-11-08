package tui

// Ports OMP's tool card rendering:
//
//	packages/tui/src/render/status-line.ts   renderStatusLine
//	packages/tui/src/render/output-block.ts  renderOutputBlock
//	packages/tui/src/render/render-utils.ts  formatStatusIcon, formatExpandHint, formatMoreItems
//	packages/tui/src/tools/bash.ts           createShellRenderer
//
// A card is an output block: a rounded frame whose top bar carries the tool's
// status line, optional labelled sections, and a bottom bar carrying facts.
//
//	╭── ✔ Bash: go test ./... · 0.4s ────────────╮
//	│ $ go test ./...                            │
//	├── Output ─────────────────────────────────┤
//	│ ok                                         │
//	╰────────────────────────────────────────────╯
//
// Tools whose body is content rather than a result stream (read, write, edit,
// grep, find, ls, search) have no labelled section: their body follows the
// status line directly, matching OMP's inline renderers.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/internal/text"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

const (
	richCapWidth = 3  // output-block.ts `cap`: the horizontal run after a corner
	richMinWidth = 10 // narrower panes fall back to the plain card
)

// richToolName is OMP's status-line title for a tool.
func richToolName(name string) string {
	switch name {
	case "bash", "powershell":
		return "Bash"
	case "read":
		return "Read"
	case "write":
		return "Write"
	case "edit":
		return "Edit"
	case "grep":
		return "Grep"
	case "find":
		return "Find"
	case "ls":
		return "List"
	case "web_search":
		return "Web Search"
	case "codemode":
		return "Codemode"
	case "Agent", "subagent":
		return "Agent"
	default:
		return name
	}
}

// richInline reports whether a tool is OMP's `inline: true`: its card is the
// status line plus the disclosed body, with no frame. These are the renderers
// whose collapsed form is the one-row head — read.ts, grep.ts, find.ts and
// glob.ts. write.ts and edit.ts return framed boxes, so they keep the block.
func richInline(name string) bool {
	switch name {
	case "read", "grep", "find", "ls":
		return true
	}
	return false
}

// richOutputSection reports whether a tool draws a labelled Output section.
// Only the shell keeps its command in one section and its result stream in
// another; every other tool's body follows its status line directly, as OMP's
// inline renderers draw them.
func richOutputSection(name string) bool {
	return name == "bash" || name == "powershell"
}

// richStatusIcon is formatStatusIcon: a spinner while the tool runs, then the
// settled glyph for its outcome.
func (c *ToolExecutionComponent) richStatusIcon(now time.Time) (string, string) {
	if c.State == ToolStateRunning {
		if c.StartedAt.IsZero() {
			return "⏳", "muted" // status.pending
		}
		// theme.spinnerFrames, indexed by the frame counter at 80ms.
		frames := [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		return frames[int(max(0, now.Sub(c.StartedAt))/(80*time.Millisecond))%len(frames)], "accent"
	}
	switch {
	case c.aborted:
		return "✘", "error" // status.aborted
	case c.State == ToolStateError:
		return "✘", "error" // status.error
	default:
		return "✔", "success" // status.success
	}
}

// richElapsed is the settled or live duration the status line shows. A running
// card reads its own clock; a settled one prefers the recorded duration, which
// survives a session reload.
func (c *ToolExecutionComponent) richElapsed(now time.Time) time.Duration {
	if c.State == ToolStateRunning {
		if c.StartedAt.IsZero() {
			return 0
		}
		return max(0, now.Sub(c.StartedAt))
	}
	if c.DurationMs != nil {
		return time.Duration(*c.DurationMs) * time.Millisecond
	}
	return c.Elapsed
}

// richMeta is the status line's trailing facts, joined with the separator: the
// search result counts oh-my-pi's grep head carries, an exit code for a shell
// command, then the duration.
func (c *ToolExecutionComponent) richMeta(now time.Time) []string {
	var meta []string
	// Counts describe a finished search, so they appear with its result.
	if c.State != ToolStateRunning {
		if searchMeta := c.searchResultMeta(); searchMeta != nil {
			meta = append(meta, searchMeta...)
		} else {
			meta = append(meta, c.resultMeta...)
		}
	}
	if c.ExitCode != nil {
		meta = append(meta, "exit "+fmt.Sprint(*c.ExitCode))
	}
	if elapsed := c.richElapsed(now); elapsed >= time.Second {
		meta = append(meta, FormatToolDuration(elapsed))
	}
	return meta
}

// richStatusLine is renderStatusLine: icon, title, a muted `: description`,
// then dim meta. Fragments are flattened so a stray newline cannot grow the
// border row into several rows.
func (c *ToolExecutionComponent) richStatusLine(width int, now time.Time) string {
	theme := ActiveTheme()
	icon, iconRole := c.richStatusIcon(now)
	line := theme.Fg(iconRole, icon) + " " + theme.Fg("toolTitle", richFlatten(richToolName(c.Name)))
	if detail := strings.TrimSpace(richFlatten(c.Detail)); detail != "" {
		// OMP's read card reads `Read <path>:<range>`; the search tools read
		// `<Tool>: <subject>`, so only they use a colon.
		sep := ": "
		if c.Name == "read" || c.Name == "write" || c.Name == "edit" {
			sep = " "
		}
		line += sep + theme.Fg("muted", widthx.TruncateToWidth(detail, max(0, width/2), "…", false))
	}
	if meta := c.richMeta(now); len(meta) > 0 {
		// Without a description the facts follow the title directly, so the
		// bar reads `Bash · exit 0 · 1.2s` rather than losing the separator.
		sep := " "
		if strings.TrimSpace(c.Detail) == "" {
			sep = " · "
		}
		line += sep + theme.Fg("dim", strings.Join(meta, " · "))
	}
	return line
}

// richFlatten collapses CR/LF runs so a caller-supplied fragment stays one row.
func richFlatten(value string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(value)
}

// RichMoreLines is render-utils.ts formatMoreItems with formatExpandHint: the
// dim marker a collapsed body appends after the rows it hid. It is exported
// for the tool renderers that live outside this package.
func RichMoreLines(hidden int) string {
	return richMoreLines(hidden)
}

// ToolOutputRows styles one output line in the tool-output color and wraps it
// to width, as OMP's output pane does.
func ToolOutputRows(line string, width int) []string {
	theme := ActiveTheme()
	color := theme.ToolOutput
	if color == "" {
		color = "\x1b[38;2;128;128;128m"
	}
	wrapped := WrapText(line, max(1, width))
	out := make([]string, len(wrapped))
	for i, row := range wrapped {
		out[i] = color + row + SGRFgReset
	}
	return out
}

// ToolDiffRows styles one diff row — a `+` or `-` marker and its text — in the
// theme's added/removed color and wraps it to width. Edit and write previews
// that have no computed diff still show a change, so they use this.
func ToolDiffRows(marker, line string, width int) []string {
	theme := ActiveTheme()
	token := "toolDiffAdded"
	if marker == "-" {
		token = "toolDiffRemoved"
	}
	wrapped := WrapText(line, max(1, width-2))
	out := make([]string, len(wrapped))
	for i, row := range wrapped {
		prefix := "  "
		if i == 0 {
			prefix = marker + " "
		}
		out[i] = theme.Fg(token, prefix+row)
	}
	return out
}

// richFooterMeta is the bottom bar's facts: OMP keeps the expansion hint and
// the tool's own notices there rather than inside the body.
func (c *ToolExecutionComponent) richFooterMeta() string {
	meta := c.richFooterFacts()
	if c.richExpandable() && !c.Collapsed {
		meta = append(meta, AppKeyText("app.tools.expand", "Ctrl+O")+": Collapse")
	}
	if len(meta) == 0 {
		return ""
	}
	return strings.Join(meta, " · ")
}

// richFooterFacts is the tool-owned part of the bottom bar: write.ts labels a
// call whose file body is still arriving `streaming`, and clears the label once
// argsComplete makes the content final.
func (c *ToolExecutionComponent) richFooterFacts() []string {
	if c.Name == "write" && !c.argsComplete {
		return []string{"streaming"}
	}
	return nil
}

// ArgsComplete reports whether the call's arguments finished streaming.
func (c *ToolExecutionComponent) ArgsComplete() bool { return c.argsComplete }

// richExpandable reports whether Ctrl+O opens or closes anything.
func (c *ToolExecutionComponent) richExpandable() bool {
	return c.definitionHasResult() || c.Output != "" || len(c.ImageBlocks) > 0
}

// richMoreLines is render-utils.ts formatMoreItems plus formatExpandHint: the
// dim marker a collapsed body appends after the rows it hid.
func richMoreLines(hidden int) string {
	label := fmt.Sprintf("… %d more line%s", hidden, plural(hidden))
	label += " ⟨" + AppKeyText("app.tools.expand", "Ctrl+O") + ": Expand⟩"
	return ActiveTheme().Fg("dim", label)
}

// renderRichFrame is render-output-block.ts renderOutputBlock. Every bar and
// every body row is padded out to the card's full width, so the right border
// sits in the same column on every line and the block's background tint covers
// the borders too. `outputStart` is the first body row after the labelled
// Output section, or -1 for a card with no such section.
func (c *ToolExecutionComponent) renderRichFrame(width int, rows []string, outputStart int) []string {
	theme := ActiveTheme()
	now := time.Now()
	content := richContentWidth(width)
	tone := c.richTone()
	border := func(s string) string { return theme.Fg(tone, s) }
	// The block's background tint covers the borders and the body padding, and
	// survives the SGR resets a renderer's own rows carry, so a colored body
	// line cannot punch a hole in the card.
	bgOpen := c.richBgOpen()
	paint := func(s string) string {
		if bgOpen == "" {
			return s
		}
		return bgOpen + strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+bgOpen) + SGRBgReset
	}
	// A bar is `left + cap + label + fill + right`, always exactly width cells.
	bar := func(left, right, label string) string {
		lead := left + strings.Repeat("─", richCapWidth)
		tail := right
		maxLabel := max(0, width-widthx.VisibleWidth(lead)-widthx.VisibleWidth(tail))
		label = widthx.TruncateToWidth(label, maxLabel, "…", false)
		fill := strings.Repeat("─", max(0, width-widthx.VisibleWidth(lead)-widthx.VisibleWidth(label)-widthx.VisibleWidth(tail)))
		return paint(border(lead) + label + border(fill+tail))
	}

	out := make([]string, 0, len(rows)+4)
	// Missing-args cards draw no header at all, as OMP's renderBar does.
	header := ""
	if label := c.richStatusLine(content, now); label != "" {
		header = " " + label + " "
	}
	out = append(out, bar("╭", "╮", header))

	appendRow := func(line string) {
		text := widthx.TruncateToWidth(line, content, "…", true)
		text += strings.Repeat(" ", max(0, content-widthx.VisibleWidth(text)))
		out = append(out, paint(border("│")+" "+text+" "+border("│")))
	}
	for i, line := range rows {
		if i == outputStart {
			out = append(out, bar("├", "┤", " "+theme.Fg("toolTitle", "Output")+" "))
		}
		appendRow(line)
	}

	footer := ""
	if meta := c.richFooterMeta(); meta != "" {
		footer = " " + theme.Fg("dim", richFlatten(meta)) + " "
	}
	out = append(out, bar("╰", "╯", footer))
	return out
}

// searchTreeTool reports whether a tool's result is drawn as a grouped tree.
func searchTreeTool(name string) bool { return name == "grep" || name == "find" }

// searchTreeCollapsed is grep.ts NATIVE_COLLAPSED_FILES: the files a collapsed
// search tree shows before summarising the rest.
const searchTreeCollapsed = 4

// SetSearchOutput records a search result for the card to draw as a tree. The
// card owns the transform so the tree appears however the tool's renderers
// resolved. It is optional: the card falls back to its own output, so a path
// that does not call this still draws the tree rather than nothing.
func (c *ToolExecutionComponent) SetSearchOutput(output string) {
	c.searchOutput = output
	c.Invalidate()
}

// richSearchRows draws the search result as oh-my-pi's grouped tree. The
// output comes from SetSearchOutput when a caller recorded it, else from the
// card's own result text, so the tree never depends on which plumbing ran.
func (c *ToolExecutionComponent) richSearchRows(width int) []string {
	output := c.searchOutput
	if output == "" {
		output = c.Output
	}
	if strings.TrimSpace(output) == "" {
		return nil
	}
	itemType := "match"
	if c.Name == "find" {
		itemType = "file"
	}
	rows := SearchTreeRows(c.Name, output, TreeListOptions{
		Expanded:     !c.Collapsed,
		MaxCollapsed: searchTreeCollapsed,
		ItemType:     itemType,
	})
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, TruncateRow(row, width))
	}
	return out
}

// searchResultMeta is the status line's counts for a search, from the output the
// tree was built from.
func (c *ToolExecutionComponent) searchResultMeta() []string {
	if !searchTreeTool(c.Name) {
		return nil
	}
	output := c.searchOutput
	if output == "" {
		output = c.Output
	}
	return SearchCounts(c.Name, output)
}

// richTone is the card's border color: the lifecycle state's color role, as
// output-block.ts picks the border from the block state.
func (c *ToolExecutionComponent) richTone() string {
	switch {
	case c.State == ToolStateError, c.aborted:
		return "error"
	case c.State == ToolStateRunning:
		return "accent"
	default:
		return "dim"
	}
}

// richBgOpen is the block's background tint: render/tool-card.ts
// getStateBgColor over the card phase. A shell card opts out with applyBg:false
// and keeps a neutral segmented look in every phase, so only the other tools
// are washed. The tint covers the borders and the padding, as renderOutputBlock
// pads the whole line before applying it.
func (c *ToolExecutionComponent) richBgOpen() string {
	theme := ActiveTheme()
	switch {
	case c.State == ToolStateRunning:
		return theme.GetBgAnsi("toolPendingBg")
	case c.State == ToolStateError, c.aborted:
		return theme.GetBgAnsi("toolErrorBg")
	default:
		return theme.GetBgAnsi("toolSuccessBg")
	}
}

// trimBlankEdges drops blank rows a renderer adds as its own separators. The
// frame already separates the status line from the body.
func trimBlankEdges(rows []string) []string {
	start, end := 0, len(rows)
	for start < end && strings.TrimSpace(widthx.StripAnsi(rows[start])) == "" {
		start++
	}
	for end > start && strings.TrimSpace(widthx.StripAnsi(rows[end-1])) == "" {
		end--
	}
	return rows[start:end]
}

// richContentWidth is output-block.ts outputBlockContentWidth: the width two
// round borders and the block's default one-cell padding leave for a child.
func richContentWidth(width int) int {
	return max(1, width-2-2)
}

// richResultPlaceholder is the body of a tool that produced no text: OMP shows
// `(no output)` rather than leaving the block empty.
func (c *ToolExecutionComponent) richResultPlaceholder(width int) []string {
	label := "(no output)"
	if c.State == ToolStateRunning {
		label = "Waiting for result…"
	}
	return NewPaddedText(ActiveTheme().Fg("muted", label), 0, 0, nil).Render(max(1, width))
}

// IsReadableURLPath is read.ts isReadableUrlPath: an http(s) or `www.` target.
// A path that is not one stays a filesystem path.
func IsReadableURLPath(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "www.")
}

// FormatReadURLHeader is fetch.ts formatReadUrlDescription for a plain card:
// `read <domain><path>`, with the target linked when the terminal supports it.
func FormatReadURLHeader(raw json.RawMessage, _ string) string {
	path, _ := decodeToolArgs(raw)["path"].(string)
	if path == "" {
		path, _ = decodeToolArgs(raw)["url"].(string)
	}
	target := path
	display := target
	if !strings.Contains(display, "://") {
		display = "https://" + display
	}
	if parsed, err := url.Parse(display); err == nil && parsed.Host != "" {
		display = parsed.Host + parsed.Path
	}
	return toolTitleText("read") + " " + fg(ActiveTheme().Accent, display)
}

// ToolDetail is the status line's subject for a tool, decoded from the same
// arguments its call header reads. Each follows OMP:
//
//	read/write/edit  <path>[:<start>-<end>]      read.ts formatReadPathLink
//	grep             <pattern> in <scope>        grep.ts formatScopedSubject
//	globs            "<query>"                   find.ts quoteQuery
//	web_search       "<query>"                   web-search.ts renderSearchCall
//	ls               <path>
//
// Unknown tools return "" and keep the bare title.
func ToolDetail(name string, args json.RawMessage, cwd string) string {
	var fields map[string]any
	if len(args) == 0 || json.Unmarshal(args, &fields) != nil {
		return ""
	}
	return ToolDetailFields(name, fields)
}

// ToolDetailFields is ToolDetail over arguments a streaming parser already
// decoded, so a card whose arguments are still arriving still names its
// subject in the status line.
func ToolDetailFields(name string, fields map[string]any) string {
	if fields == nil {
		return ""
	}
	stringOf := func(keys ...string) string {
		for _, key := range keys {
			if value, ok := fields[key].(string); ok && value != "" {
				return value
			}
		}
		return ""
	}
	numberOf := func(key string) (int, bool) {
		value, ok := fields[key].(float64)
		return int(value), ok
	}
	detail := ""
	switch name {
	case "read", "write", "edit":
		detail = stringOf("file_path", "path")
		if name == "read" && detail != "" {
			// read.ts formatReadPathLink appends the 1-based inclusive range.
			offset, hasOffset := numberOf("offset")
			limit, hasLimit := numberOf("limit")
			if hasOffset || hasLimit {
				start := 1
				if hasOffset {
					start = max(offset, 1)
				}
				end := ""
				if hasLimit {
					end = fmt.Sprint(start + max(limit, 0) - 1)
				}
				detail += ":" + fmt.Sprint(start) + "-" + end
				detail = strings.TrimSuffix(detail, "-")
			}
		}
	case "grep", "find":
		// grep.ts grepSubject: `<pattern> in <scope>`, or the scope alone. find
		// takes the same members: its `pattern` is the glob it searches for.
		pattern, scope := stringOf("pattern"), stringOf("path")
		switch {
		case pattern != "" && scope != "":
			detail = pattern + " in " + scope
		case pattern != "":
			detail = pattern
		default:
			detail = scope
		}
	case "web_search":
		// web-search.ts renderSearchCall puts the query in the description, so
		// it reads `Web Search: BTC price` on the top bar.
		detail = stringOf("query")
	case "ls":
		detail = stringOf("path")
	}
	return text.SanitizeBinaryOutput(text.StripANSIString(detail))
}
