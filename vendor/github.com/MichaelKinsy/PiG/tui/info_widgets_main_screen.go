package tui

// SetInfoWidgets attaches a passive HUD without mounting a focus-owning modal or
// adding a layout child. dock measures the prompt and every row below it on
// each frame, so the overlay meets the prompt border across resize/multiline input.
// Call on the UI owner loop; nil widgets removes the HUD.
func (t *TuiMainScreen) SetInfoWidgets(widgets *InfoWidgets, dock Component) {
	t.mu.Lock()
	t.infoWidgets, t.infoWidgetDock = widgets, dock
	t.mu.Unlock()
}

// composeInfoWidgetFrame paints over existing visible rows above the prompt.
// It preserves the line count and cached root lines, keeps the prompt untouched,
// and returns a terminal-relative mouse target beneath normal modal overlays.
func (t *TuiMainScreen) composeInfoWidgetFrame(root []string, width, height int) ([]string, *renderedOverlayLayout) {
	if t.infoWidgets == nil {
		return root, nil
	}
	dockHeight := 0
	if t.infoWidgetDock != nil {
		dockHeight = len(t.infoWidgetDock.Render(width))
	}
	viewportStart := max(0, len(root)-height)
	end := max(0, len(root)-dockHeight)
	if end <= viewportStart {
		return root, nil
	}
	rows, rect := t.infoWidgets.overlayRows(root[viewportStart:end], width)
	if len(rows) == 0 {
		return root, nil
	}
	bounds := OverlayBounds{Row: rect.Y, Col: rect.X, Width: rect.Width, Height: rect.Height}
	rect.Y += viewportStart
	lines := compositeInfoWidget(root, rows, rect, width)
	return lines, &renderedOverlayLayout{component: t.infoWidgets, bounds: bounds}
}
