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
// even with no data; narrow borders fall back to the disclosure glyph alone.
func (w *InfoWidgets) ControlLabel(width int) string {
	marker := "▾"
	if w.hidden {
		marker = "▸"
	}
	label := marker + " Widgets"
	if widthx.VisibleWidth(label) > width {
		label = marker
	}
	if width < 1 {
		return ""
	}
	return label
}

// Render bounds standalone previews. Renderer integration uses overlayRows
// with the available transcript height and never adds these rows to the layout.
func (w *InfoWidgets) Render(width int) []string {
	rows := w.renderRows(width)
	w.lastHeight = len(rows)
	height := min(12, len(rows))
	w.viewHeight = height
	w.offset = min(w.offset, max(0, len(rows)-height))
	return w.windowRows(rows, width, height)
}

// renderRows frames only the highest-priority candidate that has data. Switching
// candidates resets scroll position so a quota viewport cannot hide new tasks.
// The selected renderer chooses its compact summary when its header is collapsed.
func (w *InfoWidgets) renderRows(width int) []string {
	if w.hidden || width < 4 {
		return nil
	}
	var rows []string
	for _, s := range w.Sections {
		body := s.Render(max(1, width-4), !w.collapsed[s.Key])
		if len(body) == 0 {
			continue
		}
		if w.activeKey != s.Key {
			w.activeKey, w.offset = s.Key, 0
		}
		marker := "▾ "
		if w.collapsed[s.Key] {
			marker = "▸ "
			body = body[:1]
		}
		title := widthx.TruncateToWidth(marker+s.Title+" · Alt+W", width-4, "…", false)
		rows = append(rows, ActiveTheme().Fg("border", "╭─"+title+strings.Repeat("─", max(0, width-3-widthx.VisibleWidth(title)))+"╮"))
		for _, line := range body {
			text := widthx.TruncateToWidth(line, width-4, "…", true)
			rows = append(rows, ActiveTheme().Fg("border", "│")+" "+text+" "+ActiveTheme().Fg("border", "│"))
		}
		rows = append(rows, ActiveTheme().Fg("border", "╰"+strings.Repeat("─", width-2)+"╯"))
		return rows
	}
	w.activeKey, w.offset = "", 0
	return rows
}

// Scroll moves the content between the pinned top/bottom edges. Removing two
// chrome rows from both content and viewport leaves the same maximum offset.
// Keyboard page navigation and mouse wheel navigation share that offset.
func (w *InfoWidgets) Scroll(delta int) {
	w.offset = max(0, min(w.offset+delta, max(0, w.lastHeight-w.viewHeight)))
}

// windowRows scrolls only interior rows: the first header and final border stay
// pinned. Navigation hints live in the bottom border rather than replacing an
// edge or hiding the last content row. Fewer than three cells of height cannot
// contain both edges and content, so no partial frame is painted there.
func (w *InfoWidgets) windowRows(rows []string, width, height int) []string {
	if height < 3 {
		return nil
	}
	if len(rows) <= height {
		return rows
	}
	visible := make([]string, height)
	visible[0] = rows[0]
	copy(visible[1:height-1], rows[1+w.offset:1+w.offset+height-2])
	directions := ""
	if w.offset > 0 {
		directions += "↑"
	}
	if w.offset+height < len(rows) {
		directions += "↓"
	}
	hint := widthx.TruncateToWidth(" "+directions+" Alt+PgUp/PgDn ", width-2, "…", false)
	visible[height-1] = ActiveTheme().Fg("border", "╰"+hint+strings.Repeat("─", max(0, width-2-widthx.VisibleWidth(hint)))+"╯")
	return visible
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
	rows, rect := c.Widgets.overlayRows(lines, width)
	return compositeInfoWidget(lines, rows, rect, width)
}

// LayoutNode exposes the adaptive region to the fullscreen layout engine.
func (c *InfoWidgetLayout) LayoutNode() LayoutNode { return infoWidgetLayoutNode{c} }

type infoWidgetLayoutNode struct{ layout *InfoWidgetLayout }

func (infoWidgetLayoutNode) isLayoutNode() {}

// overlayRows opens upward at the prompt's right edge. Its source control ends
// three cells before the editor edge, so the panel uses the same right inset.
// Collapse returns no rows at all: transcript selection cannot hit a hidden HUD.
func (w *InfoWidgets) overlayRows(background []string, width int) ([]string, LayoutRect) {
	panelWidth := min(40, max(0, width-3))
	rows := w.renderRows(panelWidth)
	h := min(12, len(rows), len(background))
	if h < 3 {
		return nil, LayoutRect{}
	}
	w.lastHeight, w.viewHeight = len(rows), h
	w.offset = min(w.offset, max(0, len(rows)-h))
	return w.windowRows(rows, panelWidth, h), LayoutRect{X: width - 3 - panelWidth, Y: len(background) - h, Width: panelWidth, Height: h}
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
	screen := make([]string, max(0, y+content.Rect.Height))
	paintBox(content, screen, max(1, x+width))
	background := make([]string, h)
	for i := range background {
		background[i] = widthx.SliceByColumn(screen[y+i], x, width, true)
	}
	rows, rect := c.Widgets.overlayRows(background, width)
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
