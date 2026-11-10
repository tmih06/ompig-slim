package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// scrollingWidgetFixture supplies long prioritized tasks and lower-priority quota
// so scrolling cannot accidentally reveal or toggle the suppressed candidate.
func scrollingWidgetFixture() *InfoWidgets {
	return &InfoWidgets{Sections: []InfoWidget{
		{Key: "tasks", Title: "Tasks", Render: func(int, bool) []string {
			var rows []string
			for i := 0; i < 30; i++ {
				rows = append(rows, fmt.Sprintf("task %02d 界", i))
			}
			return rows
		}},
		{Key: "quota", Title: "Quota", Render: func(int, bool) []string { return []string{"50% left", "reset 2h"} }},
	}}
}

// TestInfoWidgetScrollKeepsEdges checks every scroll offset, including the first
// and last page. Scroll hints belong inside chrome, never in place of an edge or
// a content row, and every task must remain reachable.
func TestInfoWidgetScrollKeepsEdges(t *testing.T) {
	w := scrollingWidgetFixture()
	const width = 40
	initial := w.Render(width)
	top := initial[0]
	seen := map[string]bool{}
	maxOffset := w.lastHeight - w.viewHeight
	for offset := 0; offset <= maxOffset; offset++ {
		rows := w.Render(width)
		if rows[0] != top {
			t.Fatalf("offset %d lost fixed top edge: %q", offset, rows[0])
		}
		bottom := widthx.StripAnsi(rows[len(rows)-1])
		if !strings.HasPrefix(bottom, "╰") || !strings.HasSuffix(bottom, "╯") {
			t.Fatalf("offset %d lost bottom edge: %q", offset, bottom)
		}
		for _, row := range rows {
			if widthx.VisibleWidth(row) != width {
				t.Fatalf("offset %d has broken side edge: %q", offset, row)
			}
			plain := widthx.StripAnsi(row)
			for task := 0; task < 30; task++ {
				label := fmt.Sprintf("task %02d 界", task)
				if strings.Contains(plain, label) {
					seen[label] = true
				}
			}
		}
		if offset < maxOffset {
			w.Scroll(1)
		}
	}
	if len(seen) != 30 {
		t.Fatalf("scrolling lost content rows: reached %d/30 tasks", len(seen))
	}
	w.HandleMouse(TuiMouseEvent{Type: MouseWheel, WheelDelta: -1000, Height: 12})
	if w.offset != 0 {
		t.Fatal("mouse scroll failed to return to top")
	}
	if rows := w.Render(width); rows[0] != top {
		t.Fatal("wheel scrolling moved the top edge")
	}
}

// TestInfoWidgetScrollHeaderHitTargets keeps the selected header pinned and
// ensures neither scrolled body cells nor the footer toggle a hidden candidate.
func TestInfoWidgetScrollHeaderHitTargets(t *testing.T) {
	w := scrollingWidgetFixture()
	w.Render(40)
	w.Scroll(1000)
	rows := w.Render(40)
	if strings.Contains(strings.Join(rows, "\n"), "Quota") {
		t.Fatal("scroll revealed a second widget")
	}
	w.HandleMouse(TuiMouseEvent{Type: MouseClick, Button: MouseButtonLeft, Y: 2})
	w.HandleMouse(TuiMouseEvent{Type: MouseClick, Button: MouseButtonLeft, Y: w.viewHeight - 1})
	if w.collapsed["tasks"] || w.collapsed["quota"] {
		t.Fatal("content/footer toggled a header")
	}
	w.HandleMouse(TuiMouseEvent{Type: MouseClick, Button: MouseButtonLeft, Y: 0})
	if !w.collapsed["tasks"] || w.collapsed["quota"] {
		t.Fatal("fixed top header toggled the wrong widget")
	}
}

// TestInfoWidgetScrollResizeKeepsFrame covers tiny viewports and resizing at the
// end of a long list: show a complete frame or no overlay, never clipped chrome.
func TestInfoWidgetScrollResizeKeepsFrame(t *testing.T) {
	w := scrollingWidgetFixture()
	w.Render(40)
	w.Scroll(1000)
	for _, height := range []int{7, 3, 2, 1, 0, 5, 12, 30} {
		rows, rect := w.overlayRows(make([]string, height), 80)
		if height < 3 {
			if len(rows) != 0 {
				t.Fatalf("height %d painted incomplete widget frame", height)
			}
			continue
		}
		if rect.Y+rect.Height != height {
			t.Fatal("resizing detached widget from prompt")
		}
		top := widthx.StripAnsi(rows[0])
		bottom := widthx.StripAnsi(rows[len(rows)-1])
		if !strings.HasPrefix(top, "╭") || !strings.HasSuffix(top, "╮") || !strings.HasPrefix(bottom, "╰") || !strings.HasSuffix(bottom, "╯") {
			t.Fatalf("height %d clipped frame: %q / %q", height, top, bottom)
		}
		if w.offset > w.lastHeight-w.viewHeight {
			t.Fatal("resize left scroll offset out of range")
		}
	}
}
