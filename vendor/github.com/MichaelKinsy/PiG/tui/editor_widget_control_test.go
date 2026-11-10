package tui

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TestPromptWidgetControl keeps the source point beside quota and outside the
// response. Toggling it changes neither input text nor focus/selection ownership.
func TestPromptWidgetControl(t *testing.T) {
	e := NewEditor()
	w := widgetFixture()
	e.TopRightStatus = func(int) string { return "50% left" }
	e.TopRightControl = w.ControlLabel
	e.OnTopRightControl = w.Toggle
	e.SetText("keep this prompt")
	row := widthx.StripAnsi(e.Render(100)[0])
	if !strings.Contains(row, "50% left · ▾ Widgets") {
		t.Fatalf("control not beside quota: %s", row)
	}
	x := e.topRightControlStart
	for _, kind := range []TuiMouseEventType{MousePress, MouseRelease, MouseClick} {
		result := e.HandleMouse(TuiMouseEvent{Type: kind, Button: MouseButtonLeft, X: x, Y: 0})
		if result == nil || !result.Handled || result.Focus {
			t.Fatal("control stole editor focus or missed click")
		}
	}
	if !w.hidden || len(w.Render(40)) != 0 || e.Text() != "keep this prompt" {
		t.Fatal("collapse left response overlay or modified prompt")
	}
	if !e.IsDirty() {
		t.Fatal("disclosure state did not invalidate border")
	}
	row = widthx.StripAnsi(e.Render(100)[0])
	if !strings.Contains(row, "50% left · ▸ Widgets") {
		t.Fatalf("collapsed source missing: %s", row)
	}
	// Quota remains a status label, not an enlarged disclosure hit target.
	e.HandleMouse(TuiMouseEvent{Type: MouseClick, Button: MouseButtonLeft, X: x - 4, Y: 0})
	if !w.hidden {
		t.Fatal("quota click reopened panel")
	}
	e.HandleMouse(TuiMouseEvent{Type: MouseClick, Button: MouseButtonLeft, X: x, Y: 0})
	if w.hidden {
		t.Fatal("source point could not reopen panel")
	}
}

// TestPromptWidgetControlNarrow preserves working/overflow labels and terminal
// width. Missing quota still leaves the source point reachable on the border.
func TestPromptWidgetControlNarrow(t *testing.T) {
	e := NewEditor()
	w := widgetFixture()
	e.TopRightControl = w.ControlLabel
	e.OnTopRightControl = w.Toggle
	e.EmbedWorkingStatus = true
	e.SetWorkingStatusIndicator(&StatusIndicator{Kind: "working", Loader: NewLoader(nil, nil, nil, "Working", nil)})
	for _, width := range []int{1, 5, 10, 20, 40, 100} {
		fallback := e.renderStatusBorder(width, 3, e.renderTopBorder(width, 3))
		row := e.renderTopRightStatus(width, fallback)
		if widthx.VisibleWidth(row) != width {
			t.Fatalf("control overflow at %d: %q", width, row)
		}
		plain := widthx.StripAnsi(row)
		for _, label := range []string{"Working", "↑ 3 more"} {
			if strings.Contains(widthx.StripAnsi(fallback), label) && !strings.Contains(plain, label) {
				t.Fatalf("control overwrote %s", label)
			}
		}
	}
	e.SetWorkingStatusIndicator(nil)
	row := widthx.StripAnsi(e.Render(100)[0])
	if !strings.Contains(row, "▾ Widgets") {
		t.Fatal("no control without quota/status")
	}
	e.NeedsRedraw()
	w.Toggle()
	if !e.IsDirty() {
		t.Fatal("keyboard toggle was hidden by editor cache")
	}
}

// TestInfoWidgetMeetsPromptBorder uses allocated fullscreen geometry, including
// intermediate status rows and multiline prompts. Hidden panels have no layout
// box at all, so normal response selection receives every transcript cell.
func TestInfoWidgetMeetsPromptBorder(t *testing.T) {
	w := widgetFixture()
	e := NewEditor()
	e.TopRightControl = w.ControlLabel
	e.OnTopRightControl = w.Toggle
	doc := NewScrollView(NewPaddedText(strings.Repeat("response\n", 30), 0, 0, nil), ScrollViewOptions{Follow: "end", Primary: true})
	root := NewVStack([]StackEntry{
		{Component: doc, StackEntryOptions: StackEntryOptions{Basis: new(0), Grow: new(1), MinSize: new(1)}},
		{Component: NewPaddedText("status above prompt", 0, 0, nil)},
		{Component: e, StackEntryOptions: StackEntryOptions{MinSize: new(3)}},
		{Component: NewPaddedText("footer", 0, 0, nil)},
	}, StackOptions{})
	layout := &InfoWidgetLayout{Content: root, Widgets: w, Prompt: e}
	for _, text := range []string{"single line", "one\ntwo\nthree"} {
		e.SetText(text)
		frame := RenderLayoutFrame(layout, 100, 24, nil)
		editor := infoWidgetPromptBox(frame.Root.Children[0], e)
		panel := frame.Root.Children[1]
		if panel.Rect.Y+panel.Rect.Height != editor.Rect.Y {
			t.Fatalf("panel not touching prompt: %+v, editor %+v", panel.Rect, editor.Rect)
		}
		if panel.Rect.X+panel.Rect.Width != editor.Rect.X+editor.Rect.Width-3 {
			t.Fatal("panel and source point are not right-aligned")
		}
		if !strings.Contains(widthx.StripAnsi(frame.Lines[editor.Rect.Y]), "▾ Widgets") {
			t.Fatal("source control not on prompt border")
		}
	}
	w.Toggle()
	frame := RenderLayoutFrame(layout, 100, 24, nil)
	if len(frame.Root.Children) != 1 {
		t.Fatal("collapsed overlay still has a transcript hit box")
	}
	editor := infoWidgetPromptBox(frame.Root.Children[0], e)
	if !strings.Contains(widthx.StripAnsi(frame.Lines[editor.Rect.Y]), "▸ Widgets") {
		t.Fatal("collapse did not leave a border control")
	}
	for _, box := range GetLayoutBoxesAt(frame, 80, editor.Rect.Y-1) {
		if box.Component == w {
			t.Fatal("collapsed panel intercepts response selection")
		}
	}
	w.Toggle()
	// Flex minimums can push the editor outside an extremely short viewport.
	// An invisible source point must not produce an out-of-bounds overlay.
	for height := 1; height < 9; height++ {
		frame := RenderLayoutFrame(layout, 40, height, nil)
		if len(frame.Lines) != height {
			t.Fatal("tiny viewport changed height")
		}
		if len(frame.Root.Children) > 1 {
			panel := frame.Root.Children[1]
			if panel.Rect.Y+panel.Rect.Height > height {
				t.Fatal("panel escaped tiny viewport")
			}
		}
	}
}
