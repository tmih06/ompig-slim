package tui

import (
	"fmt"
	"strings"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// SetWorkingStatusIndicator selects the status displayed by an opted-in editor.
func (e *Editor) SetWorkingStatusIndicator(indicator *StatusIndicator) {
	e.workingStatusIndicator = indicator
	e.Invalidate()
}

// IsDirty reports whether the editor's rendered lines changed since the last
// consume. The right border label can change with its data source. The embedded status indicator is animated and relabelled by its
// owner (spinner ticks, working message, retry countdown) without touching the
// editor, so its own dirty flag counts too; otherwise the per-child render
// cache would keep the old border row until a keystroke dirtied the editor.
func (e *Editor) IsDirty() bool {
	if e.invalidatable.IsDirty() || e.topRightStatusChanged() {
		return true
	}
	indicator := e.workingStatusIndicator
	return e.EmbedWorkingStatus && indicator != nil && indicator.IsDirty()
}

// NeedsRedraw consumes the editor's and embedded indicator's dirty flags and checks the right border label.
func (e *Editor) NeedsRedraw() bool {
	dirty := e.topRightStatusChanged()
	dirty = e.invalidatable.NeedsRedraw() || dirty
	if indicator := e.workingStatusIndicator; e.EmbedWorkingStatus && indicator != nil {
		dirty = indicator.NeedsRedraw() || dirty
	}
	return dirty
}

// pig additive (D91): SurfaceLive reports that the embedded indicator, which changes without
// invalidating the editor, or a dynamic right border label draws in the editor.
func (e *Editor) SurfaceLive() bool {
	return e.TopRightStatus != nil || e.EmbedWorkingStatus && e.workingStatusIndicator != nil
}

// renderStatusBorder mirrors CustomEditor.renderTopBorder, retaining a centered
// overflow label when the complete status or spinner can fit to its left.
func (e *Editor) renderStatusBorder(width, hiddenLineCount int, fallback string) string {
	if !e.EmbedWorkingStatus || e.workingStatusIndicator == nil || width <= 0 {
		return fallback
	}
	indicator := e.workingStatusIndicator
	// Embedded working and compaction indicators take the editor's border color (interactive-mode.ts showWorkingStatusIndicator
	// and the compaction_start colorFn, Pi 1.2.0); branch summary and retry keep their own colors.
	if indicator.Kind == "working" || indicator.Kind == "compaction" {
		loader := &Loader{Message: indicator.Message, Frame: indicator.Frame, Frames: indicator.Frames, IndicatorVerbatim: indicator.IndicatorVerbatim, spinnerColorFn: e.colorEditorBorder, messageColorFn: e.colorEditorBorder}
		indicator = &StatusIndicator{Loader: loader, Kind: indicator.Kind}
	}
	status := indicator.RenderInBorder(max(1, width-5))
	statusWidth := widthx.VisibleWidth(status)
	if statusWidth == 0 {
		return fallback
	}
	overflowLabel := ""
	if hiddenLineCount > 0 {
		overflowLabel = fmt.Sprintf(" ↑ %d more ", hiddenLineCount)
	}
	overflowWidth := widthx.VisibleWidth(overflowLabel)
	overflowStart := (width - overflowWidth) / 2
	canFitOverflow := func() bool {
		return overflowLabel != "" && overflowWidth+2 <= width && overflowStart-(3+statusWidth+1) >= 1
	}
	if overflowLabel != "" && !canFitOverflow() {
		status = indicator.RenderSpinnerInBorder(width)
		statusWidth = widthx.VisibleWidth(status)
	}
	color := e.colorEditorBorder
	if canFitOverflow() {
		leftWidth := 3 + statusWidth + 1
		return color("── ") + status + color(" "+strings.Repeat("─", overflowStart-leftWidth)+overflowLabel+strings.Repeat("─", width-overflowStart-overflowWidth))
	}
	if width >= statusWidth+5 {
		return color("── ") + status + color(" "+strings.Repeat("─", width-statusWidth-4))
	}
	status = indicator.RenderSpinnerInBorder(width)
	statusWidth = widthx.VisibleWidth(status)
	prefixWidth := min(3, max(0, width-statusWidth))
	return color(strings.Repeat("─", prefixWidth)) + status + color(strings.Repeat("─", max(0, width-prefixWidth-statusWidth)))
}

func (e *Editor) topRightStatusChanged() bool {
	return e.topRightStatusLabel() != e.topRightRendered
}

func (e *Editor) topRightStatusLabel() string {
	if e.TopRightStatus == nil || e.topRightBudget <= 0 {
		return ""
	}
	label := e.TopRightStatus(e.topRightBudget)
	if widthx.VisibleWidth(label) > e.topRightBudget {
		return ""
	}
	return label
}

// Only replace trailing border cells, preserving the working label and centered scroll indicator.
func (e *Editor) renderTopRightStatus(width int, border string) string {
	e.topRightRendered = ""
	if e.TopRightStatus == nil {
		e.topRightBudget = 0
		return border
	}
	plain := widthx.StripAnsi(border)
	trailing := len([]rune(plain)) - len([]rune(strings.TrimRight(plain, "─")))
	e.topRightBudget = max(0, min(width-4, trailing-4))
	label := e.topRightStatusLabel()
	labelWidth := widthx.VisibleWidth(label)
	if labelWidth == 0 {
		return border
	}
	e.topRightRendered = label
	left := width - labelWidth - 3
	return widthx.SliceByColumn(border, 0, left, false) + e.colorEditorBorder(" ") + label + e.colorEditorBorder(" ─")
}
