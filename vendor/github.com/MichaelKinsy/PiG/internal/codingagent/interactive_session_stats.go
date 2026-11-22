package codingagent

// ompig-slim: the /session report in its own fullscreen view, so the stats
// neither bloat the transcript nor wrap inside its width. The view owns the
// alternate screen like the /usage board and hands it back on close.

import (
	"strconv"
	"strings"

	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// sessionStatsState is the mounted /session view.
type sessionStatsState struct {
	handle       *tui.OverlayHandle
	previousMode string
}

// showSessionStats opens the /session view, or focuses it when it is open.
// snapshot reads the session afresh; the view calls it now and on refresh.
func (m *InteractiveMode) showSessionStats(snapshot func() sessionReport) {
	if m.tuiInst == nil {
		return
	}
	if m.sessionStats != nil {
		m.sessionStats.handle.Focus()
		return
	}
	previousMode, ok := m.enterBoardScreen("session stats")
	if !ok {
		return
	}
	view := &sessionStatsView{
		snapshot:      snapshot,
		report:        snapshot(),
		height:        m.tuiInst.Height,
		close:         m.closeSessionStats,
		requestRender: m.requestRender,
	}
	handle := m.showBoard(view)
	if handle == nil {
		return
	}
	m.sessionStats = &sessionStatsState{handle: handle, previousMode: previousMode}
	m.requestRender()
}

// closeSessionStats hides the view and restores the chat's previous screen mode.
func (m *InteractiveMode) closeSessionStats() {
	state := m.sessionStats
	if state == nil {
		return
	}
	m.sessionStats = nil
	m.leaveBoardScreen(state.handle, state.previousMode)
}

// disposeSessionStats drops the view when the mode ends, leaving the
// renderer, which teardown is already stopping, alone.
func (m *InteractiveMode) disposeSessionStats() {
	if state := m.sessionStats; state != nil {
		m.sessionStats = nil
		state.handle.Hide()
	}
}

// sessionStatsView is a framed, scrollable page of the /session report. The
// input loop owns it, so it holds no lock.
type sessionStatsView struct {
	snapshot      func() sessionReport
	report        sessionReport
	height        func() int
	close         func()
	requestRender func()

	scroll   int
	pageRows int // body rows at the last render
	total    int // report lines at the last render

	// framed holds the report's lines laid out and boxed for framedWidth and
	// framedTheme, and blank the box's empty row. Scrolling and repaints
	// reuse them; a new width or theme, or a refresh, rebuilds them.
	framed      []string
	blank       string
	framedWidth int
	framedTheme *tui.Theme
}

// sessionStatsChrome is the rows around the body: top border, divider, hint, bottom border.
const sessionStatsChrome = 4

func (v *sessionStatsView) Invalidate()                 {}
func (v *sessionStatsView) CapturesViewportInput() bool { return true }

func (v *sessionStatsView) Render(width int) []string {
	if width < 6 {
		return []string{widthx.TruncateToWidth("Session stats", max(1, width), "…", false)}
	}
	theme := tui.ActiveTheme()
	border := func(s string) string { return theme.Fg("border", s) }
	inner := width - 4
	rows := max(1, max(sessionStatsChrome+2, v.height())-sessionStatsChrome)

	// The track fills the room the lane rows leave beside the widest name.
	traceWidth := min(max(inner-sessionMaxLaneNameWidth-sessionLaneChrome, sessionMinTraceWidth), sessionMaxTraceWidth)
	row := func(text string) string {
		return border("│ ") + widthx.TruncateToWidth(text, inner, "…", true) + border(" │")
	}
	if v.framed == nil || v.framedWidth != width || v.framedTheme != theme {
		text := strings.Split(strings.TrimRight(v.report(traceWidth), "\n"), "\n")
		v.framed = make([]string, len(text))
		for i, line := range text {
			v.framed[i] = row(line)
		}
		v.blank = row("")
		v.framedWidth, v.framedTheme = width, theme
	}
	v.pageRows, v.total = rows, len(v.framed)
	v.scroll = min(v.scroll, max(0, v.total-rows))

	lines := make([]string, 0, rows+sessionStatsChrome)
	lines = append(lines, border("╭"+strings.Repeat("─", width-2)+"╮"))
	visible := v.framed[v.scroll:min(v.total, v.scroll+rows)]
	lines = append(lines, visible...)
	for range rows - len(visible) {
		lines = append(lines, v.blank)
	}
	// The position leads so a narrow screen clips the keys, not where you are.
	hint := "↑/↓ scroll · pgup/pgdn page · r refresh · esc close"
	if v.total > rows {
		hint = strconv.Itoa(min(v.total, v.scroll+rows)) + "/" + strconv.Itoa(v.total) + " · " + hint
	}
	lines = append(lines,
		border("├"+strings.Repeat("─", width-2)+"┤"),
		row(theme.Fg("dim", hint)),
		border("╰"+strings.Repeat("─", width-2)+"╯"))
	return lines
}

// HandleInput routes keys through the configured select and alt-screen
// actions, as the /usage board does, so remapped keys work here too.
func (v *sessionStatsView) HandleInput(data string) {
	keys := tui.GetKeybindings()
	switch {
	case keys.Matches(data, tui.KBSelectCancel), data == "q":
		v.close()
		return
	case data == "r":
		v.report, v.framed = v.snapshot(), nil
	case keys.Matches(data, tui.KBSelectUp), data == "k":
		v.scrollBy(-1)
	case keys.Matches(data, tui.KBSelectDown), data == "j":
		v.scrollBy(1)
	case keys.Matches(data, tui.KBSelectPageUp), keys.Matches(data, tui.KBAltScreenPageUp):
		v.scrollBy(-v.pageRows)
	case keys.Matches(data, tui.KBSelectPageDown), keys.Matches(data, tui.KBAltScreenPageDown):
		v.scrollBy(v.pageRows)
	case keys.Matches(data, tui.KBAltScreenTop), tui.MatchesKeyID(data, "home"):
		v.scroll = 0
	case keys.Matches(data, tui.KBAltScreenBottom), tui.MatchesKeyID(data, "end"):
		v.scroll = v.total
	default:
		return
	}
	v.requestRender()
}

// HandleMouse scrolls on the wheel and swallows every other mouse event, so
// nothing reaches the chat behind the view.
func (v *sessionStatsView) HandleMouse(event tui.TuiMouseEvent) *tui.TuiMouseDispatchResult {
	if event.Type == tui.MouseWheel && event.WheelDelta != 0 {
		v.scrollBy(3 * min(max(event.WheelDelta, -1), 1))
		v.requestRender()
	}
	return &tui.TuiMouseDispatchResult{TuiMouseEventResult: tui.TuiMouseEventResult{Handled: true}}
}

// scrollBy moves the body by delta rows; Render clamps the position.
func (v *sessionStatsView) scrollBy(delta int) { v.scroll = max(0, v.scroll+delta) }
