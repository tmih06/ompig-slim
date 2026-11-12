package tui

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// widgetFixture keeps panel content deterministic while exercising disclosure.
func widgetFixture() *InfoWidgets {
	return &InfoWidgets{Sections: []InfoWidget{{Key: "tasks", Title: "Tasks", Render: func(width int, expanded bool) []string {
		if !expanded {
			return []string{"task summary"}
		}
		return []string{"first task", "second task", "third task"}
	}}}}
}

// TestInfoWidgetPlacement checks real layout cells and hit boxes, not just the
// placement solver: wide text, scrollbars, resize and ANSI/Unicode all matter.
func TestInfoWidgetPlacement(t *testing.T) {
	widgets := widgetFixture()
	doc := NewPaddedText(strings.Repeat("short response\n", 30), 0, 0, nil)
	scroll := NewScrollView(doc, ScrollViewOptions{Follow: "end", Primary: true, Scrollbar: ScrollbarAlways})
	root := &InfoWidgetLayout{Content: scroll, Widgets: widgets}
	frame := RenderLayoutFrame(root, 100, 15, nil)
	panel := frame.Root.Children[1]
	if panel.Rect.X != 57 || panel.Rect.Y+panel.Rect.Height != 15 {
		t.Fatalf("not floating in right margin: %+v", panel.Rect)
	}
	hits := GetLayoutBoxesAt(frame, panel.Rect.X, panel.Rect.Y)
	if hits[0].Component != widgets {
		t.Fatal("floating panel has no topmost mouse target")
	}
	anchor := panel.Rect
	doc.SetText(strings.Repeat("changed\n", 30))
	frame = RenderLayoutFrame(root, 100, 15, nil)
	if frame.Root.Children[1].Rect != anchor {
		t.Fatal("safe anchor jumped")
	}
	doc.SetText(strings.Repeat("界", 49) + "\n" + strings.Repeat("short\n", 30))
	// Scrolling and wide Unicode responses must not move the prompt anchor.
	scroll.ScrollToStart()
	frame = RenderLayoutFrame(root, 100, 15, nil)
	if frame.Root.Children[1].Rect != anchor {
		t.Fatal("response text moved the prompt-anchored panel")
	}
	doc.SetText(strings.Repeat(strings.Repeat("x", 98)+"\n", 30))
	frame = RenderLayoutFrame(root, 100, 15, nil)
	panel = frame.Root.Children[1]
	if panel.Rect.X != 57 || panel.Rect.Y+panel.Rect.Height != 15 {
		t.Fatalf("dense content did not stay in bottom-right overlay: %+v", panel.Rect)
	}
	if frame.Root.Children[0].Rect.Height != 15 || scroll.ViewportHeight() != 15 {
		t.Fatal("overlay consumed transcript space")
	}
	frame = RenderLayoutFrame(root, 30, 9, nil)
	if frame.Root.Children[1].Rect.Width != 27 {
		t.Fatal("narrow overlay escaped viewport")
	}
	for _, line := range frame.Lines {
		if widthx.VisibleWidth(line) > 30 {
			t.Fatalf("overflow: %q", line)
		}
	}
}

// TestInfoWidgetDisclosureAndScroll proves independent mouse disclosure, global
// keyboard toggle, and access to tasks below the bounded overlay preview.
func TestInfoWidgetDisclosureAndScroll(t *testing.T) {
	w := widgetFixture()
	w.Sections = append(w.Sections, InfoWidget{Key: "quota", Title: "Quota", Render: func(int, bool) []string { return []string{"50% left"} }})
	expanded := w.Render(40)
	result := w.HandleMouse(TuiMouseEvent{Type: MouseClick, Button: MouseButtonLeft, Y: 0})
	if result == nil || !result.Handled || result.Focus {
		t.Fatal("header click must handle without taking focus")
	}
	compact := w.Render(40)
	if len(compact) >= len(expanded) || w.collapsed["quota"] {
		t.Fatal("disclosure affected another section")
	}
	w.Toggle()
	if !w.hidden || len(w.Render(40)) != 0 {
		t.Fatal("global collapse left overlay rows")
	}
	w.Toggle()
	if w.hidden || !w.collapsed["tasks"] || w.collapsed["quota"] {
		t.Fatal("reopen lost per-section disclosure state")
	}
	w.Sections = []InfoWidget{{Key: "long", Title: "Long plan", Render: func(int, bool) []string {
		var rows []string
		for i := 0; i < 40; i++ {
			rows = append(rows, fmt.Sprint("task ", i))
		}
		return rows
	}}}
	rows := w.Render(40)
	if len(rows) != 12 {
		t.Fatalf("unbounded preview: %d", len(rows))
	}
	w.HandleMouse(TuiMouseEvent{Type: MouseWheel, WheelDelta: 100, Height: 12})
	if !strings.Contains(strings.Join(w.Render(40), "\n"), "task 39") {
		t.Fatal("last task inaccessible")
	}
}

// TestInfoWidgetPromptAnchor checks bottom-right geometry across viewport sizes.
// The overlay needs only dimensions, never a painted copy of the transcript.
func TestInfoWidgetPromptAnchor(t *testing.T) {
	w := widgetFixture()
	for _, width := range []int{30, 80, 100} {
		for _, height := range []int{3, 5, 20} {
			rows, rect := w.overlayRows(width, height)
			if rect.X+rect.Width != width-3 || rect.Y+len(rows) != height {
				t.Fatalf("detached bottom-right anchor at %dx%d: %+v", width, height, rect)
			}
		}
	}
	w.Toggle()
	if rows, _ := w.overlayRows(100, 20); len(rows) != 0 {
		t.Fatal("collapsed panel still covers the response")
	}
}

// TestInfoWidgetSkipsInvisibleProviders protects the hidden/tiny fast path from
// rendering data that cannot be painted, including resetting stale scroll bounds.
func TestInfoWidgetSkipsInvisibleProviders(t *testing.T) {
	w := widgetFixture()
	w.Render(40)
	w.Sections[0].Render = func(int, bool) []string {
		t.Fatal("invisible widget invoked provider")
		return nil
	}
	for _, size := range [][2]int{{3, 20}, {40, 2}, {40, 0}} {
		if rows := w.render(size[0], size[1]); len(rows) != 0 || w.lastHeight != 0 || w.viewHeight != 0 {
			t.Fatalf("invisible widget retained viewport at %v", size)
		}
	}
	w.Toggle()
	if len(w.Render(40)) != 0 {
		t.Fatal("hidden widget retained rows")
	}
}

// TestInfoWidgetPreservesProviderRows allows providers to return cached slices:
// framing, scrolling and disclosure must never overwrite the provider's content.
func TestInfoWidgetPreservesProviderRows(t *testing.T) {
	body := strings.Split(strings.Repeat("☐ task 界\n", 30), "\n")
	original := strings.Join(body, "\n")
	w := &InfoWidgets{Sections: []InfoWidget{{Key: "tasks", Title: "Tasks", Render: func(int, bool) []string { return body }}}}
	w.Render(40)
	w.Scroll(1000)
	w.Render(40)
	w.HandleMouse(TuiMouseEvent{Type: MouseClick, Button: MouseButtonLeft})
	w.Render(40)
	if strings.Join(body, "\n") != original {
		t.Fatal("widget mutated cached provider rows")
	}
}

// TestMainScreenInfoWidgetOverlay protects regular-mode scrollback, prompt rows,
// root caches, modal ownership and passive mouse hit testing from regressions.
func TestMainScreenInfoWidgetOverlay(t *testing.T) {
	r := NewWithOutput(io.Discard, 80, 20)
	w := widgetFixture()
	doc := NewPaddedText(strings.Repeat("response\n", 24), 0, 0, nil)
	dock := NewPaddedText("PROMPT\nFOOTER", 0, 0, nil)
	root := NewContainer(doc, dock)
	original := root.Render(80)
	originalCopy := append([]string(nil), original...)
	r.Add(root)
	r.SetInfoWidgets(w, dock)
	r.doRender()
	if len(r.prevLines) != len(original) {
		t.Fatal("HUD added rows to regular-mode layout")
	}
	if r.HasOverlay() {
		t.Fatal("passive HUD claimed modal ownership")
	}
	if strings.Join(original, "\n") != strings.Join(originalCopy, "\n") {
		t.Fatal("HUD mutated cached transcript rows")
	}
	for _, label := range []string{"PROMPT", "FOOTER"} {
		if !strings.Contains(strings.Join(r.prevLines[len(r.prevLines)-2:], "\n"), label) {
			t.Fatal("HUD covered prompt or footer")
		}
	}
	if len(r.renderedOverlayLayouts) != 1 {
		t.Fatal("missing passive mouse bounds")
	}
	bounds := r.renderedOverlayLayouts[0].bounds
	hit := r.dispatchMouseToOverlay(TuiMouseEvent{Type: MouseClick, Button: MouseButtonLeft, ScreenX: bounds.Col, ScreenY: bounds.Row})
	if !hit.hit || !w.collapsed["tasks"] {
		t.Fatal("overlay header click did not collapse section")
	}
	w.Toggle()
	r.doRender()
	if len(r.renderedOverlayLayouts) != 0 {
		t.Fatal("collapsed HUD left transcript mouse bounds")
	}
	if strings.Join(root.Render(80), "\n") != strings.Join(originalCopy, "\n") {
		t.Fatal("collapse did not restore the original response")
	}
	r.SetInfoWidgets(nil, nil)
	r.doRender()
	if len(r.renderedOverlayLayouts) != 0 {
		t.Fatal("removed HUD left stale mouse bounds")
	}
}

// TestInfoWidgetEmptyAndTiny keeps empty state height-free and protects editors
// on tiny terminals without changing the transcript allocation.
func TestInfoWidgetEmptyAndTiny(t *testing.T) {
	for width := 1; width < 90; width++ {
		for height := 1; height < 8; height++ {
			root := &InfoWidgetLayout{Content: NewPaddedText("response", 0, 0, nil), Widgets: widgetFixture()}
			frame := RenderLayoutFrame(root, width, height, nil)
			if len(frame.Lines) != height {
				t.Fatal("wrong frame height")
			}
			for _, box := range frame.Root.Children {
				if box.Rect.Y+box.Rect.Height > height {
					t.Fatalf("out of bounds: %+v", box.Rect)
				}
			}
		}
	}
	w := &InfoWidgets{}
	if len(w.Render(80)) != 0 {
		t.Fatal("empty host reserved rows")
	}
}

// TestInfoWidgetPaletteTokens pins the widget frame to the theme tokens the rest
// of the TUI uses after the palette port: border for lines, accent bold for the
// title, dim for the keyboard hints. It matters because the token names are easy
// to get subtly wrong — titanium's border is #2a3038, so a title drawn in border
// would nearly vanish on a dark terminal while every width assertion still held.
func TestInfoWidgetPaletteTokens(t *testing.T) {
	theme := optionalPalette(t, "titanium")
	if theme == nil {
		t.Skip("titanium palette unavailable")
	}
	swapTheme(t, theme)
	accent, border, dim := theme.GetFgAnsi("accent"), theme.GetFgAnsi("border"), theme.GetFgAnsi("dim")
	if accent == border || accent == dim {
		t.Fatal("fixture theme aliases its tokens; the test would prove nothing")
	}

	w := &InfoWidgets{Sections: []InfoWidget{{Key: "tasks", Title: "Tasks", Render: func(int, bool) []string {
		var rows []string
		for i := 0; i < 30; i++ {
			rows = append(rows, fmt.Sprint("task ", i))
		}
		return rows
	}}}}
	rows := w.Render(40)
	if len(rows) != 12 {
		t.Fatalf("want a bounded viewport, got %d rows", len(rows))
	}
	top, bottom := rows[0], rows[len(rows)-1]
	if !strings.Contains(top, accent) || !strings.Contains(top, "\x1b[1m") {
		t.Fatalf("title is not accent bold: %q", top)
	}
	if !strings.Contains(top, border) || !strings.Contains(bottom, border) {
		t.Fatal("frame lines are not drawn in border")
	}
	// The title must not borrow the border color; that is the visible regression
	// this pins.
	if strings.Contains(top, border+accent) && !strings.Contains(top, accent) {
		t.Fatal("title lost its accent color")
	}
	if !strings.Contains(bottom, dim) {
		t.Fatalf("scroll hints are not dim: %q", bottom)
	}
	for _, row := range rows {
		if widthx.VisibleWidth(row) != 40 {
			t.Fatalf("styled frame broke the width: %q", row)
		}
	}

	// The border control is the panel's one interactive affordance, so it takes
	// accent rather than the quota label's default color.
	label := w.ControlLabel(40)
	if !strings.Contains(label, accent) || !strings.Contains(widthx.StripAnsi(label), "▾ Widgets") {
		t.Fatalf("control label is not accent: %q", label)
	}
	if got := w.ControlLabel(1); widthx.VisibleWidth(got) != 1 {
		t.Fatalf("narrow border did not fall back to the glyph alone: %q", got)
	}
}
