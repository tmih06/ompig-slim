package tui

import (
	"strings"
	"testing"
)

// TestInfoWidgetSinglePriority ensures a lower-priority provider is neither
// rendered nor drawn while the selected candidate has data, even when collapsed.
func TestInfoWidgetSinglePriority(t *testing.T) {
	w := scrollingWidgetFixture()
	quotaCalls := 0
	w.Sections[1].Render = func(int, bool) []string { quotaCalls++; return []string{"50% left"} }
	rows := strings.Join(w.Render(40), "\n")
	if !strings.Contains(rows, "Tasks") || strings.Contains(rows, "Quota") || quotaCalls != 0 {
		t.Fatalf("multiple candidates rendered: %s", rows)
	}
	w.Scroll(1000)
	w.Sections[0].Render = func(int, bool) []string { return nil }
	rows = strings.Join(w.Render(40), "\n")
	if !strings.Contains(rows, "Quota") || strings.Contains(rows, "Tasks") || w.offset != 0 {
		t.Fatalf("fallback failed or retained previous scroll: %s", rows)
	}
	w.HandleMouse(TuiMouseEvent{Type: MouseClick, Button: MouseButtonLeft, Y: 0})
	rows = strings.Join(w.Render(40), "\n")
	if !strings.Contains(rows, "▸ Quota") {
		t.Fatal("selected widget header lost its disclosure state")
	}
	w.Sections[0].Render = func(int, bool) []string { return []string{"new task"} }
	rows = strings.Join(w.Render(40), "\n")
	if !strings.Contains(rows, "new task") || strings.Contains(rows, "Quota") || w.offset != 0 {
		t.Fatalf("new tasks did not preempt quota: %s", rows)
	}
	w.Sections[0].Render = func(int, bool) []string { return nil }
	w.Sections[1].Render = func(int, bool) []string { return nil }
	if len(w.Render(40)) != 0 || w.activeKey != "" {
		t.Fatal("empty candidates retained a stale widget")
	}
}
