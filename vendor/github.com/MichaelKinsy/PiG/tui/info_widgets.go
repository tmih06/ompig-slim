package tui

import (
	"strings"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// InfoWidget is one collapsible candidate for the single visible widget. Render must be local,
// nonblocking and return terminal-cell-safe rows; nil/empty rows omit the section.
type InfoWidget struct {
	Key, Title string
	Render     func(width int, expanded bool) []string
}

// InfoWidgets hosts persistent information outside the transcript. Its state is
// owned by the UI loop. Both renderers open upward from the prompt's right
// border. Global collapse removes every overlay row and its transcript hit box.
// This is a native adaptation of jcode's info-widget margin layout, not its
// provider-specific Rust widgets.
type InfoWidgets struct {
	// Sections are ordered highest priority first; only the first with data renders.
	Sections   []InfoWidget
	activeKey  string
	collapsed  map[string]bool
	hidden     bool
	offset     int
	lastHeight int
	viewHeight int
}

// Invalidate has no cached rows to discard: sections read live owner-loop state.
func (w *InfoWidgets) Invalidate() {}

// Toggle folds the whole overlay into its prompt-border control or reopens it.
// Per-section disclosure is preserved while hidden; no transcript hit box remains.
func (w *InfoWidgets) Toggle() {
	w.hidden = !w.hidden
	w.offset = 0
}

// ControlLabel is the persistent prompt-border source point. It stays available
// even with no data; narrow borders fall back to the disclosure glyph alone. The
// accent token marks it as the panel's control rather than another readout, and
// the marker carries the open/collapsed state on its own.
func (w *InfoWidgets) ControlLabel(width int) string {
	marker := "▾"
	if w.hidden {
		marker = "▸"
	}
	if width < 1 {
		return ""
	}
	label := marker + " Widgets"
	if widthx.VisibleWidth(label) > width {
		label = marker
	}
	return ActiveTheme().Fg(string(ThemeColorAccent), label)
}

// Render bounds standalone previews to twelve rows, just like the overlay.
func (w *InfoWidgets) Render(width int) []string { return w.render(width, 12) }

// render selects the first nonempty candidate and frames only its visible rows.
// Providers retain ownership of their rows; scrolling slices them without copying
// or styling offscreen content. The header/footer stay pinned. Hidden or tiny
// viewports skip providers entirely; switching candidates resets scroll position.
func (w *InfoWidgets) render(width, height int) []string {
	w.lastHeight, w.viewHeight = 0, 0
	if w.hidden || width < 4 || height < 3 {
		return nil
	}
	for _, s := range w.Sections {
		collapsed := w.collapsed[s.Key]
		body := s.Render(max(1, width-4), !collapsed)
		if len(body) == 0 {
			continue
		}
		if w.activeKey != s.Key {
			w.activeKey, w.offset = s.Key, 0
		}
		marker := "▾ "
		if collapsed {
			marker = "▸ "
			body = body[:1]
		}
		w.lastHeight = len(body) + 2
		w.viewHeight = min(12, height, w.lastHeight)
		w.Scroll(0) // Clamp after content changes or viewport resizing.
		rows := make([]string, w.viewHeight)
		// Frame lines take `border`, the title `accent` bold and the keyboard
		// hints `dim`, as every other framed surface does. Segments are colored
		// separately because each Fg closes the foreground it opened: wrapping
		// the whole row would drop the trailing frame back to the default color.
		// The title must not live in `border` either — the bundled palettes aim
		// that token at lines, and titanium's is a near-background gray.
		theme := ActiveTheme()
		border := func(text string) string { return theme.Fg(string(ThemeColorBorder), text) }
		// Truncate the plain label first: the escapes below have no width, and
		// cutting colored text can split a sequence.
		inner := max(0, width-3)
		title, hint := marker+s.Title, " · Alt+W"
		if widthx.VisibleWidth(title)+widthx.VisibleWidth(hint) > inner {
			hint = "" // Drop the shortcut before the widget's own name.
		}
		title = widthx.TruncateToWidth(title, max(0, inner-widthx.VisibleWidth(hint)), "…", false)
		rows[0] = border("╭─") + theme.Bold(theme.Fg(string(ThemeColorAccent), title)) +
			theme.Fg(string(ThemeColorDim), hint) +
			border(strings.Repeat("─", max(0, inner-widthx.VisibleWidth(title)-widthx.VisibleWidth(hint)))+"╮")
		left, right := border("│")+" ", " "+border("│")
		for i, line := range body[w.offset : w.offset+w.viewHeight-2] {
			rows[i+1] = left + widthx.TruncateToWidth(line, width-4, "…", true) + right
		}
		// Navigation hints live inside the bottom border; only a scrolling panel
		// shows them, so a short list keeps a clean edge.
		footer := ""
		if w.viewHeight < w.lastHeight {
			directions := ""
			if w.offset > 0 {
				directions += "↑"
			}
			if w.offset+w.viewHeight < w.lastHeight {
				directions += "↓"
			}
			footer = widthx.TruncateToWidth(" "+directions+" Alt+PgUp/PgDn ", width-2, "…", false)
		}
		rows[len(rows)-1] = border("╰") + theme.Fg(string(ThemeColorDim), footer) +
			border(strings.Repeat("─", max(0, width-2-widthx.VisibleWidth(footer)))+"╯")
		return rows
	}
	w.activeKey, w.offset = "", 0
	return nil
}

// Scroll moves content between the pinned edges, clamped to the last viewport.
// Keyboard page navigation and mouse wheel navigation share the same offset.
func (w *InfoWidgets) Scroll(delta int) {
	w.offset = max(0, min(w.offset+delta, max(0, w.lastHeight-w.viewHeight)))
}

// HandleMouse toggles a header without stealing editor focus. Wheel events scroll
// a height-constrained overlay; full content remains reachable on small terminals.
func (w *InfoWidgets) HandleMouse(e TuiMouseEvent) *TuiMouseEventResult {
	if e.Type == MouseWheel {
		w.viewHeight = e.Height
		w.Scroll(e.WheelDelta)
		return &TuiMouseEventResult{Handled: true}
	}
	if e.Button != MouseButtonLeft {
		return nil
	}
	if e.Type != MouseClick {
		return &TuiMouseEventResult{Handled: true}
	}
	// Only the selected widget's pinned header is a disclosure target.
	if e.Y != 0 || w.activeKey == "" {
		return &TuiMouseEventResult{Handled: true}
	}
	if w.collapsed == nil {
		w.collapsed = map[string]bool{}
	}
	w.collapsed[w.activeKey] = !w.collapsed[w.activeKey]
	w.offset = 0
	return &TuiMouseEventResult{Handled: true}
}

// InfoWidgetLayout keeps widgets in the transcript viewport without adding them
// to its document, scrollback, search index, or selection coordinates.
type InfoWidgetLayout struct {
	Content Component
	Widgets *InfoWidgets
	// Prompt identifies the editor slot in a fullscreen layout. The overlay's
	// lower edge meets its top border even with status/widgets above the editor.
	Prompt Component
}

// Invalidate forwards invalidation without resetting panel disclosure state.
func (c *InfoWidgetLayout) Invalidate() { c.Content.Invalidate(); c.Widgets.Invalidate() }

// Render overlays existing content without appending rows. Fullscreen uses
// LayoutNode so hit boxes remain separate from transcript coordinates.
func (c *InfoWidgetLayout) Render(width int) []string {
	lines := c.Content.Render(width)
	rows, rect := c.Widgets.overlayRows(width, len(lines))
	return compositeInfoWidget(lines, rows, rect, width)
}

// LayoutNode exposes the adaptive region to the fullscreen layout engine.
func (c *InfoWidgetLayout) LayoutNode() LayoutNode { return infoWidgetLayoutNode{c} }

type infoWidgetLayoutNode struct{ layout *InfoWidgetLayout }

func (infoWidgetLayoutNode) isLayoutNode() {}

// overlayRows opens upward at the prompt's right edge. Its source control ends
// three cells before the editor edge, so the panel uses the same right inset.
// Collapse returns no rows at all: transcript selection cannot hit a hidden HUD.
func (w *InfoWidgets) overlayRows(width, height int) ([]string, LayoutRect) {
	panelWidth := min(40, max(0, width-3))
	rows := w.render(panelWidth, height)
	if len(rows) == 0 {
		return nil, LayoutRect{}
	}
	return rows, LayoutRect{X: width - 3 - panelWidth, Y: height - len(rows), Width: panelWidth, Height: len(rows)}
}

// compositeInfoWidget paints into a copy of existing lines. Neither transcript
// rows nor the caller's cached render buffer are changed by a passive overlay.
func compositeInfoWidget(background, rows []string, rect LayoutRect, width int) []string {
	if len(rows) == 0 {
		return background
	}
	result := append([]string(nil), background...)
	for i, row := range rows {
		result[rect.Y+i] = CompositeTuiLine(result[rect.Y+i], row, rect.X, rect.Width, width)
	}
	return result
}

// layoutInfoWidgets paints a higher-layer sibling over the unchanged transcript
// viewport. Prompt identifies the top border to attach to; the overlay never
// changes either the transcript allocation or the prompt geometry.
func layoutInfoWidgets(ctx *layoutContext, c *InfoWidgetLayout, x, y, width int, height *int, clip LayoutRect) *LayoutBox {
	content := layoutComponent(ctx, c.Content, x, y, width, height, clip)
	root := &LayoutBox{Component: c, Rect: content.Rect, Clip: intersect(clip, content.Rect), Children: []*LayoutBox{content}}
	content.Parent = root
	if c.Widgets.hidden {
		return root
	}
	h := content.Rect.Height
	if c.Prompt != nil {
		if prompt := infoWidgetPromptBox(content, c.Prompt); prompt != nil {
			h = max(0, min(h, prompt.Rect.Y-y))
			if prompt.Clip.Height == 0 {
				h = 0
			} // No visible source point on a clipped editor.
		} else {
			h = 0
		} // An absent editor must not anchor over another slot.
	}
	// Placement depends only on geometry. The frame painter composites the
	// sibling panel later; prepainting the transcript here would do it twice.
	rows, rect := c.Widgets.overlayRows(width, h)
	if len(rows) == 0 {
		return root
	}
	rect.X += x
	rect.Y += y
	panel := &LayoutBox{Component: c.Widgets, Rect: rect, Clip: intersect(clip, rect), Lines: rows, Parent: root, Layer: 1}
	root.Children = append(root.Children, panel)
	return root
}

// infoWidgetPromptBox locates the allocated editor slot, rather than estimating
// its position from intrinsic heights that fullscreen flex layout may shrink.
func infoWidgetPromptBox(box *LayoutBox, prompt Component) *LayoutBox {
	if sameComponent(box.Component, prompt) {
		return box
	}
	for _, child := range box.Children {
		if found := infoWidgetPromptBox(child, prompt); found != nil {
			return found
		}
	}
	return nil
}
