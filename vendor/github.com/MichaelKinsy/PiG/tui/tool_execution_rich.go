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

// richMeta is the status line's trailing facts, joined with the separator:
// an exit code for a shell command, then the duration.
func (c *ToolExecutionComponent) richMeta(now time.Time) []string {
	var meta []string
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
		line += ": " + theme.Fg("muted", widthx.TruncateToWidth(detail, max(0, width/2), "…", false))
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

// richFooterFacts is the tool-owned part of the bottom bar, empty for now:
// OMP fills it with background-job, service and artifact facts the Go runtime
// does not record.
func (c *ToolExecutionComponent) richFooterFacts() []string { return nil }

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

// renderRichFrame draws the output block: top bar with the status line, body
// rows with an optional labelled Output section, and a bottom bar.
func (c *ToolExecutionComponent) renderRichFrame(width int, rows []string, outputStart int) []string {
	theme := ActiveTheme()
	now := time.Now()
	inner := width - 2         // between the vertical borders, including the label bars
	content := max(1, inner-2) // output-block.ts contentPaddingLeft/Right default 1
	run := func(n int) string { return strings.Repeat("─", max(0, n)) }
	out := make([]string, 0, len(rows)+4)

	// renderOutputBlock's renderBar: `╭──` + ` label ` + fill + `╮`. A status
	// line longer than the bar is truncated, never wrapped.
	label := widthx.TruncateToWidth(" "+c.richStatusLine(inner, now)+" ", max(0, inner-richCapWidth-1), "…", false)
	out = append(out, theme.Fg("toolTitle", "╭"+run(richCapWidth))+label+
		theme.Fg("toolTitle", run(inner-widthx.VisibleWidth(label)-richCapWidth-1)+"╮"))

	appendRow := func(line string) {
		// renderContent pads the clipped row back out to the content width, so
		// the right border keeps its column.
		text := widthx.TruncateToWidth(line, content, "…", true)
		text += strings.Repeat(" ", max(0, content-widthx.VisibleWidth(text)))
		out = append(out, theme.Fg("toolTitle", "│ ")+text+theme.Fg("toolTitle", " │"))
	}
	for i, line := range rows {
		if i == outputStart {
			// A labelled section always draws its titled separator bar, from
			// border to border.
			titled := " " + theme.Fg("toolTitle", "Output") + " "
			out = append(out, theme.Fg("toolTitle", "├"+run(richCapWidth))+titled+
				theme.Fg("toolTitle", run(inner-widthx.VisibleWidth(titled)-richCapWidth-1)+"┤"))
		}
		appendRow(line)
	}

	// renderOutputBlock's bottom bar: `╰──` + facts + fill + `╯`.
	footer := ""
	if meta := c.richFooterMeta(); meta != "" {
		footer = " " + theme.Fg("dim", richFlatten(meta)) + " "
	}
	out = append(out, theme.Fg("toolTitle", "╰"+run(richCapWidth))+footer+
		theme.Fg("toolTitle", run(inner-widthx.VisibleWidth(footer)-richCapWidth-1)+"╯"))
	return out
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

// ToolDetail is the status line's description for a built-in tool, decoded
// from the same arguments its call header reads. Unknown tools return "".
func ToolDetail(name string, args json.RawMessage, cwd string) string {
	var fields map[string]any
	if len(args) == 0 || json.Unmarshal(args, &fields) != nil {
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
	var detail string
	switch name {
	case "read", "write", "edit":
		detail = stringOf("file_path", "path")
	case "grep", "find":
		pattern := stringOf("pattern")
		detail = pattern
		if path := stringOf("path"); path != "" {
			if pattern != "" {
				detail = "/" + pattern + "/ in " + path
			} else {
				detail = path
			}
		}
	case "ls":
		detail = stringOf("path")
	case "web_search":
		detail = stringOf("query")
	}
	return text.SanitizeBinaryOutput(text.StripANSIString(detail))
}
