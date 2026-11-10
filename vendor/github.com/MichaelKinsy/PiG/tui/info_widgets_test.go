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

// TestInfoWidgetPromptAnchor verifies the panel always meets the prompt border,
// regardless of response occupancy; collapse removes all transcript hit boxes.
func TestInfoWidgetPromptAnchor(t *testing.T) {
	w := widgetFixture()
	wide := strings.Repeat("界", 50)
	background := make([]string, 20)
	for i := 0; i < 8; i++ {
		background[i] = wide
	}
	rows, rect := w.overlayRows(background, 100)
	if rect.X != 57 || rect.Y != 20-len(rows) {
		t.Fatalf("expected bottom-right: %+v", rect)
	}
	for i := range background {
		background[i] = ""
		if i >= 12 {
			background[i] = wide
		}
	}
	_, rect = w.overlayRows(background, 100)
	if rect.X != 57 || rect.Y != 20-len(rows) {
		t.Fatalf("bottom-right anchor moved: %+v", rect)
	}
	for i := range background {
		background[i] = wide
	}
	_, rect = w.overlayRows(background, 100)
	if rect.Y != 20-len(rows) {
		t.Fatal("dense response moved the anchor")
	}
	w.Toggle()
	if rows, _ := w.overlayRows(background, 100); len(rows) != 0 {
		t.Fatal("collapsed panel still covers the response")
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
