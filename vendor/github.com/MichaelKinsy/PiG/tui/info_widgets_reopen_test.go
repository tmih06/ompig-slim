package tui

import (
	"strings"
	"testing"
)

// TestInfoWidgetReopenKeepsWorkingPoint exercises both disclosure controls after
// the user scrolls to work below the first page, including a paint while folded.
func TestInfoWidgetReopenKeepsWorkingPoint(t *testing.T) {
	for _, section := range []bool{false, true} {
		t.Run(map[bool]string{false: "panel", true: "section"}[section], func(t *testing.T) {
			w := scrollingWidgetFixture()
			w.Render(40)
			w.Scroll(18)
			before := strings.Join(w.Render(40), "\n")
			toggle := func() {
				if section {
					w.HandleMouse(TuiMouseEvent{Type: MouseClick, Button: MouseButtonLeft, Y: 0})
				} else {
					w.Toggle()
				}
			}
			toggle()
			w.Render(40)
			w.Scroll(-1000) // Hidden/folded navigation must not erase the saved offset.
			toggle()
			after := strings.Join(w.Render(40), "\n")
			if after != before {
				t.Fatalf("reopening lost the working point (task 18):\n%s", after)
			}
		})
	}
}
