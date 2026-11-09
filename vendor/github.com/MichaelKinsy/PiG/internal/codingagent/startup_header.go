// Ports packages/coding-agent/src/modes/interactive/interactive-mode.ts.
package codingagent

import (
	"slices"
	"strings"

	"github.com/MichaelKinsy/PiG/extensions/sdk/frontend"
	"github.com/MichaelKinsy/PiG/internal/coding/pigversion"
	"github.com/MichaelKinsy/PiG/internal/pigstrip"
	"github.com/MichaelKinsy/PiG/tui"
)

// headerContainer keeps host spacing outside the replaceable header component, including when a custom header renders no rows. While startup waits for the terminal's colors (startupHeaderPending) it returns one empty container, so the first paint carries no header, as Pi's headerContainer has no children until after that wait (interactive-mode.ts init()); mountStartupHeader fills it.
func (m *InteractiveMode) headerContainer() *tui.Container {
	if m.startupHeaderPending {
		if m.headerBox == nil {
			m.headerBox = tui.NewContainer()
		}
		return m.headerBox
	}
	box := tui.NewContainer()
	m.fillHeaderContainer(box)
	return box
}

func (m *InteractiveMode) fillHeaderContainer(box *tui.Container) {
	if !m.opts.LoginVisible {
		box.Add(m.extHeader)
	} else {
		box.Add(tui.NewSpacer(1))
		box.Add(m.extHeader)
		box.Add(tui.NewSpacer(1))
	}
	if m.altScreen != nil {
	}
}

// mountStartupHeader builds the built-in header and adds it to the header container. Startup calls it once the terminal's colors were reported or the query timed out, because the header bakes theme colors into its text (interactive-mode.ts init(), after waitForTerminalColors).
func (m *InteractiveMode) mountStartupHeader() {
	m.setBuiltInHeader(m.opts.Verbose || m.toolsExpanded)
	if !m.startupHeaderPending {
		return
	}
	m.startupHeaderPending = false
	if m.headerBox != nil {
		m.fillHeaderContainer(m.headerBox)
	}
	m.requestRender()
}

// builtInHeaderKey holds every input that changes the built-in header's lines. A theme carries its color mode, and the theme and the merged keybindings are replaced, never changed in place (tui.SetTheme and RefreshActiveThemeColorMode store a new theme, KeybindingsManager.rebuild a new manager), so their pointers stand for their content. Nil bindings draw the host platform's defaults, which do not change while PiG runs; the version is a constant. showLogo reports whether the terminal renders the half-block mark, which the brand colors follow.
type builtInHeaderKey struct {
	width                 int
	theme                 *tui.Theme
	bindings              *tui.TUIKeybindingsManager
	showLogo              bool
	expanded, showDetails bool
}

// builtInHeaderRender is the last built-in header drawn: its lines and logo area for its key.
type builtInHeaderRender struct {
	key   builtInHeaderKey
	lines []string
	logo  *builtInHeaderLogoArea
}

// renderBuiltInHeader renders the current startup help expansion. Verbose seeds this state only at initialization; tool toggles and header restoration subsequently select it.
// The lines are kept until an input in builtInHeaderKey changes, as Pi's BuiltInHeader keeps its text until it is invalidated (themed-text.ts) and its Text keeps its lines per width (text.ts render); the frame loop calls this on every frame, and wrapping the hints and baking the theme colors is most of a frame's allocations.
func (m *InteractiveMode) renderBuiltInHeader(width int) []string {
	key := m.builtInHeaderInputs(width)
	bindings := key.bindings
	if last := m.builtInHeaderLast.Load(); last != nil && last.key == key {
		m.builtInHeaderLogo.Store(last.logo)
		return last.lines
	}
	if bindings == nil {
		bindings = tui.NewKeybindingsManager(keybindingDefinitionsFor(tui.HostKeybindingPlatform()), nil)
	}
	lines, logo := drawBuiltInHeader(key, bindings)
	m.builtInHeaderLogo.Store(logo)
	m.builtInHeaderLast.Store(&builtInHeaderRender{key: key, lines: lines, logo: logo})
	return lines
}

// builtInHeaderInputs reads every input of the built-in header's lines at width. The mark is the fork's fixed brand art, so no
// sprite state enters the key: only the width, the theme, the keybindings and the startup help expansion change the lines.
func (m *InteractiveMode) builtInHeaderInputs(width int) builtInHeaderKey {
	theme := tui.ActiveTheme()
	var bindings *tui.TUIKeybindingsManager
	if m.keybindings != nil {
		bindings = m.keybindings.merged
	}
	m.toolMu.Lock()
	expanded, showDetails := m.builtInHeaderExpanded, m.builtInHeaderShowDetails
	m.toolMu.Unlock()
	return builtInHeaderKey{
		width: width, theme: theme, bindings: bindings,
		showLogo: supportsPiLogo(), expanded: expanded, showDetails: showDetails,
	}
}

// builtInHeaderUnchanged reports that the built-in header would draw the lines it drew last, whatever the width: a parent
// container draws a child again when the width changes, so only the other inputs are compared. It is the specialLinesComponent's
// clean test, which keeps the frame's window over a long transcript unbroken between frames that change nothing.
func (m *InteractiveMode) builtInHeaderUnchanged() bool {
	last := m.builtInHeaderLast.Load()
	if last == nil {
		return false
	}
	key := m.builtInHeaderInputs(last.key.width)
	return last.key == key
}

// drawBuiltInHeader draws the built-in header for in with bindings, and returns its lines and the logo's clickable area.
func drawBuiltInHeader(in builtInHeaderKey, bindings *tui.TUIKeybindingsManager) ([]string, *builtInHeaderLogoArea) {
	width, theme, expanded, showDetails := in.width, in.theme, in.expanded, in.showDetails
	mode := theme.GetColorMode()
	key := func(action string) string {
		return tui.FormatKeyText(strings.Join(bindings.GetKeys(action), "/"), false)
	}
	rawHint := func(key, description string) string {
		return themeFg(theme.Dim, key) + themeFg(theme.Muted, " "+description)
	}
	hint := func(action, description string) string { return rawHint(key(action), description) }
	// interactive-mode.ts:998-1006: the logo's first line carries the version and its second line the first line of key
	// hints; the fork's product tag follows the mark on the first line. A terminal that cannot render the half blocks
	// (supportsPiLogo) gets the text wordmark on one line instead, with the key hints below it.
	// pig divergence (D63): the startup version is the personal harness release, not the composite PiG+Pi identity.
	version := themeFg(theme.Dim, "v"+pigversion.HarnessVersion)
	tag := productTag(mode)
	withLogo := func(hints string) string {
		if !in.showLogo {
			return piWordmark(mode) + " " + tag + " " + version + "\n" + hints
		}
		top, bottom := piLogoLines(mode)
		inText := tui.ForegroundAnsi(logoWhite, mode) + "in " + logoReset
		return top + "    " + renderGoLine(0, mode) + " " + tag + " " + version + "\n" +
			bottom + " " + inText + renderGoLine(2, mode) + " " + hints
	}
	// pig additive (D92): a hint of a stripped built-in (an action of a stripped command, or `!` of a stripped bash tool) is
	// left out. Stock PiG strips nothing and joins Pi's hints unchanged.
	bashStripped := pigstrip.Has(pigstrip.ListTools, "bash")
	unless := func(stripped bool, hint string) string {
		if stripped {
			return ""
		}
		return hint
	}
	join := func(hints []string, separator string) string {
		return strings.Join(slices.DeleteFunc(hints, func(hint string) bool { return hint == "" }), separator)
	}
	var instructions string
	if expanded {
		instructions = withLogo(join([]string{
			hint("app.interrupt", "to interrupt"),
			hint("app.clear", "to clear"),
			rawHint(key("app.clear")+" twice", "to exit"),
			hint("app.exit", "to exit (empty)"),
			hint("app.suspend", "to suspend"),
			hint("tui.editor.deleteToLineEnd", "to delete to end"),
			unless(appActionStripped("app.thinking.cycle"), hint("app.thinking.cycle", "to cycle thinking level")),
			unless(appActionStripped("app.model.cycleForward"), rawHint(key("app.model.cycleForward")+"/"+key("app.model.cycleBackward"), "to cycle models")),
			unless(appActionStripped("app.model.select"), hint("app.model.select", "to select model")),
			hint("app.tools.expand", "to expand tools"),
			hint("app.thinking.toggle", "to expand thinking"),
			hint("app.editor.external", "for external editor"),
			rawHint("/", "for commands"),
			unless(bashStripped, rawHint("!", "to run bash")),
			unless(bashStripped, rawHint("!!", "to run bash (no context)")),
			hint("app.message.followUp", "to queue follow-up"),
			hint("app.message.dequeue", "to edit all queued messages"),
			hint("app.clipboard.pasteImage", "to paste files on macOS, images, or text"),
			rawHint("drop files", "to attach"),
		}, "\n"))
	} else {
		instructions = join([]string{
			hint("app.interrupt", "interrupt"),
			rawHint(key("app.clear")+"/"+key("app.exit"), "clear/exit"),
			rawHint("/", "commands"),
			unless(bashStripped, rawHint("!", "bash")),
			hint("app.tools.expand", "more"),
		}, themeFg(theme.Muted, " · "))
		// interactive-mode.ts:997, 1044-1048: the loaded resources are mentioned only when the startup details showed as the header was built.
		resources := ""
		if showDetails {
			resources = " and loaded resources"
		}
		instructions = withLogo(instructions) + "\n" + themeFg(theme.Dim, "Press "+key("app.tools.expand")+" to show full startup help"+resources+".")
	}
	// The self-help names this product, the way Pi's names Pi. ompig-slim can explain its own features; without the docs
	// bundle there are no docs to look up (pig additive D92).
	onboarding := themeFg(theme.Dim, AppName+" can explain its own features and look up its docs. Ask it how to use or extend "+AppName+".")
	if pigstrip.Has(pigstrip.ListFeatures, pigstrip.Docs) {
		onboarding = themeFg(theme.Dim, AppName+" can explain its own features. Ask it how to use or extend "+AppName+".")
	}
	text := instructions + "\n\n" + onboarding
	// interactive-mode.ts:1058 sets onLogoClick only when the logo is drawn: Apple Terminal's wordmark takes no click.
	if !in.showLogo {
		return tui.NewPaddedText(text, 1, 0, nil).Render(width), &builtInHeaderLogoArea{}
	}
	// interactive-mode.ts:1058 sets onLogoClick only when the logo is drawn; this header's logo is the whole OM + Pi mark.
	area := builtInHeaderLogoArea{visible: true, column: 1, columns: logoMarkCells, rows: piLogoRows}
	return tui.NewPaddedText(text, 1, 0, nil).Render(width), &area
}

// builtInHeaderLogoArea is the clickable logo of the built-in header as last drawn: the fork's mark, relative to the header's
// first line.
type builtInHeaderLogoArea struct {
	visible       bool
	column, row   int
	columns, rows int
}

// handleBuiltInHeaderMouse plays the logo easter egg when the header's logo is clicked. Mirrors interactive-mode.ts
// BuiltInHeader.handleMouse, which takes a click on the logo's cells (x 1 to 4 of its two lines, after one column of
// padding) and passes the logo's top-left screen cell; Pi sets onLogoClick only when it draws its logo (interactive-mode.ts:1058).
// The mark begins with the fork's OM prefix and then Pi's original four-cell logo, so the clickable area is the whole mark.
func (m *InteractiveMode) handleBuiltInHeaderMouse(event tui.TuiMouseEvent) *tui.TuiMouseDispatchResult {
	area := m.builtInHeaderLogo.Load()
	if event.Type != tui.MouseClick || area == nil || !area.visible ||
		event.X < area.column || event.X >= area.column+area.columns || event.Y < area.row || event.Y >= area.row+area.rows {
		return nil
	}
	left, top := event.ScreenX-event.X+area.column, event.ScreenY-event.Y+area.row
	// The easter egg is PiG's 3D pig, which lifts off the mark: it clears and centers on the clicked mark's cells, the
	// OM + Pi mark the header drew there.
	m.playPigLogoAnimation(left, top, area.columns, area.rows)
	return &tui.TuiMouseDispatchResult{TuiMouseEventResult: tui.TuiMouseEventResult{Handled: true}}
}

// pig additive (D91): builtInHeaderClickArea is the header's logo as a frontend session reports a click on it.
func (m *InteractiveMode) builtInHeaderClickArea() (frontend.Area, bool) {
	area := m.builtInHeaderLogo.Load()
	if area == nil || !area.visible {
		return frontend.Area{}, false
	}
	return frontend.Area{Row: area.row, Column: area.column, Rows: area.rows, Columns: area.columns}, true
}
