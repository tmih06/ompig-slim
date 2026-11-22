package codingagent

// ompig-slim: text boards, the in-TUI replacement for printing a long report
// into the transcript. A board is a framed, scrollable page in its own
// fullscreen view, so it neither bloats the transcript nor wraps inside its
// width. It owns the alternate screen like the /usage board and hands it back
// on close. /session and /hotkeys use it.

import (
	"slices"
	"strconv"
	"strings"

	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// textBoardState is the mounted board.
type textBoardState struct {
	handle       *tui.OverlayHandle
	previousMode string
}

// openTextBoard shows board in its own fullscreen view, or focuses the board
// that is already open: one at a time.
func (m *InteractiveMode) openTextBoard(board *textBoard) {
	if m.tuiInst == nil {
		return
	}
	if m.textBoard != nil {
		m.textBoard.handle.Focus()
		return
	}
	previousMode, ok := m.enterBoardScreen(strings.ToLower(board.name))
	if !ok {
		return
	}
	board.height, board.close, board.requestRender = m.tuiInst.Height, m.closeTextBoard, m.requestRender
	handle := m.showBoard(board)
	if handle == nil {
		return
	}
	m.textBoard = &textBoardState{handle: handle, previousMode: previousMode}
	m.requestRender()
}

// closeTextBoard hides the board and restores the chat's previous screen mode.
func (m *InteractiveMode) closeTextBoard() {
	state := m.textBoard
	if state == nil {
		return
	}
	m.textBoard = nil
	m.leaveBoardScreen(state.handle, state.previousMode)
}

// disposeTextBoard drops the board when the mode ends, leaving the renderer,
// which teardown is already stopping, alone.
func (m *InteractiveMode) disposeTextBoard() {
	if state := m.textBoard; state != nil {
		m.textBoard = nil
		state.handle.Hide()
	}
}

// showSessionStats opens the /session board. snapshot reads the session
// afresh; the board calls it now and on refresh.
func (m *InteractiveMode) showSessionStats(snapshot func() sessionReport) {
	report := snapshot()
	m.openTextBoard(&textBoard{
		name: "Session stats",
		lines: func(inner int) []string {
			return strings.Split(strings.TrimRight(report(sessionViewLayout(inner)), "\n"), "\n")
		},
		refresh: func() { report = snapshot() },
	})
}

// textBoard is a framed, scrollable page. The input loop owns it, so it
// holds no lock.
type textBoard struct {
	name string // the page's title, for the warning and a too-narrow frame
	// lines lays the page out for inner columns.
	lines func(inner int) []string
	// refresh re-reads what lines shows; nil when the page has nothing to
	// re-read, which also leaves the refresh key out of the footer.
	refresh func()

	height        func() int
	close         func()
	requestRender func()

	scroll   int
	pageRows int // body rows at the last render
	total    int // page lines at the last render

	// framed holds the page's lines laid out and boxed for framedWidth and
	// framedTheme, and blank the box's empty row. Scrolling and repaints
	// reuse them; a new width or theme, or a refresh, rebuilds them.
	framed      []string
	blank       string
	framedWidth int
	framedTheme *tui.Theme
}

// textBoardChrome is the rows around the body: top border, divider, hint, bottom border.
const textBoardChrome = 4

func (v *textBoard) Invalidate()                 {}
func (v *textBoard) CapturesViewportInput() bool { return true }

func (v *textBoard) Render(width int) []string {
	if width < 6 {
		return []string{widthx.TruncateToWidth(v.name, max(1, width), "…", false)}
	}
	theme := tui.ActiveTheme()
	border := func(s string) string { return theme.Fg("border", s) }
	inner := width - 4
	rows := max(1, max(textBoardChrome+2, v.height())-textBoardChrome)

	row := func(text string) string {
		return border("│ ") + widthx.TruncateToWidth(text, inner, "…", true) + border(" │")
	}
	if v.framed == nil || v.framedWidth != width || v.framedTheme != theme {
		text := v.lines(inner)
		v.framed = make([]string, len(text))
		for i, line := range text {
			v.framed[i] = row(line)
		}
		v.blank = row("")
		v.framedWidth, v.framedTheme = width, theme
	}
	v.pageRows, v.total = rows, len(v.framed)
	v.scroll = min(v.scroll, max(0, v.total-rows))

	lines := make([]string, 0, rows+textBoardChrome)
	lines = append(lines, border("╭"+strings.Repeat("─", width-2)+"╮"))
	visible := v.framed[v.scroll:min(v.total, v.scroll+rows)]
	lines = append(lines, visible...)
	for range rows - len(visible) {
		lines = append(lines, v.blank)
	}
	// The position leads so a narrow screen clips the keys, not where you are.
	hint := textBoardHint(v.refresh != nil)
	if v.total > rows {
		hint = strconv.Itoa(min(v.total, v.scroll+rows)) + "/" + strconv.Itoa(v.total) + " · " + hint
	}
	lines = append(lines,
		border("├"+strings.Repeat("─", width-2)+"┤"),
		row(theme.Fg("dim", hint)),
		border("╰"+strings.Repeat("─", width-2)+"╯"))
	return lines
}

// HandleInput routes keys through the configured actions, as the /usage
// board does: scrolling and closing use the select and alt-screen actions,
// refreshing has its own, and a remap applies here too. j, k, q, home and end
// stay as fixed aliases. Refreshing does nothing on a page with nothing to
// re-read.
func (v *textBoard) HandleInput(data string) {
	keys := tui.GetKeybindings()
	match := func(action string, fallback ...string) bool {
		return tui.ActionMatches(keys, data, action, fallback...)
	}
	switch {
	case match(tui.KBSelectCancel, "escape", "ctrl+c"), data == "q":
		v.close()
		return
	case v.refresh != nil && match(string(AppSessionStatsRefresh), "r"):
		v.refresh()
		v.framed = nil
	case match(tui.KBSelectUp, "up"), data == "k":
		v.scrollBy(-1)
	case match(tui.KBSelectDown, "down"), data == "j":
		v.scrollBy(1)
	case match(tui.KBSelectPageUp, "pageUp"), match(tui.KBAltScreenPageUp, "pageUp"):
		v.scrollBy(-v.pageRows)
	case match(tui.KBSelectPageDown, "pageDown"), match(tui.KBAltScreenPageDown, "pageDown"):
		v.scrollBy(v.pageRows)
	case match(tui.KBAltScreenTop, "ctrl+home"), tui.MatchesKeyID(data, "home"):
		v.scroll = 0
	case match(tui.KBAltScreenBottom, "ctrl+end"), tui.MatchesKeyID(data, "end"):
		v.scroll = v.total
	default:
		return
	}
	v.requestRender()
}

// textBoardHint is the footer's key legend, read from the configured
// bindings; an action the user has unbound drops out of it.
func textBoardHint(refreshable bool) string {
	part := func(label string, keys ...string) string {
		keys = slices.DeleteFunc(keys, func(key string) bool { return key == "" })
		if len(keys) == 0 {
			return ""
		}
		return strings.Join(keys, "/") + " " + label
	}
	parts := []string{
		part("scroll", tui.ActionText(tui.KBSelectUp, "up"), tui.ActionText(tui.KBSelectDown, "down")),
		part("page", tui.ActionText(tui.KBSelectPageUp, "pageUp"), tui.ActionText(tui.KBSelectPageDown, "pageDown")),
	}
	if refreshable {
		parts = append(parts, part("refresh", tui.ActionText(string(AppSessionStatsRefresh), "r")))
	}
	parts = append(parts, part("close", tui.ActionText(tui.KBSelectCancel, "escape")))
	return strings.Join(slices.DeleteFunc(parts, func(p string) bool { return p == "" }), " · ")
}

// HandleMouse scrolls on the wheel and swallows every other mouse event, so
// nothing reaches the chat behind the view.
func (v *textBoard) HandleMouse(event tui.TuiMouseEvent) *tui.TuiMouseDispatchResult {
	if event.Type == tui.MouseWheel && event.WheelDelta != 0 {
		v.scrollBy(3 * min(max(event.WheelDelta, -1), 1))
		v.requestRender()
	}
	return &tui.TuiMouseDispatchResult{TuiMouseEventResult: tui.TuiMouseEventResult{Handled: true}}
}

// scrollBy moves the body by delta rows; Render clamps the position.
func (v *textBoard) scrollBy(delta int) { v.scroll = max(0, v.scroll+delta) }
