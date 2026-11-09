package tui

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TestModelOverlaySplitGeometry transcribes OMP overlay-box.ts's split formulas.
// The top tee, body divider, and closing tee must share the same display column;
// a rectangular border alone would miss the previous three-column displacement.
func TestModelOverlaySplitGeometry(t *testing.T) {
	for _, width := range []int{40, 80, 120} {
		sidebar := 18
		top := widthx.StripTerminalSequences(modelOverlayTop(width, "Models", sidebar, false))
		divider := widthx.StripTerminalSequences(modelOverlayDivider(width, sidebar))
		wantTop := "╭─ Models " + strings.Repeat("─", sidebar-7) + "┬" + strings.Repeat("─", width-sidebar-5) + "╮"
		wantDivider := "├" + strings.Repeat("─", sidebar+2) + "┴" + strings.Repeat("─", width-sidebar-5) + "┤"
		if top != wantTop || divider != wantDivider {
			t.Fatalf("width %d:\n%s\n%s", width, top, divider)
		}
		for _, line := range []string{top, divider, modelOverlayRow("short", width, false)} {
			if got := widthx.VisibleWidth(line); got != width {
				t.Fatalf("width %d got %d: %q", width, got, line)
			}
		}
	}
}

// TestModelPickerRectangle covers the actual rendered card, not just the host
// overlay. Every short status/detail/footer row must keep its right border at
// the same column, including task mode's warning-colored frame.
func TestModelPickerRectangle(t *testing.T) {
	for _, width := range []int{40, 80, 120} {
		picker := NewModelPicker(ModelPickerConfig{Height: func() int { return 40 }, TaskKeys: []string{"alt+p"}})
		for _, task := range []bool{false, true} {
			picker.taskMode = task
			for i, line := range picker.Render(width) {
				plain := widthx.StripTerminalSequences(line)
				if got := widthx.VisibleWidth(plain); got != width {
					t.Fatalf("task=%v row %d: width %d want %d: %q", task, i, got, width, plain)
				}
			}
		}
	}
}

// TestModelBrowserFrameRows keeps OMP's fixed five-row browser chrome: search,
// blank, list window, blank, detail facts, role chips. Hosts budget around this.
func TestModelBrowserFrameRows(t *testing.T) {
	b := NewModelBrowser("")
	lines := b.Render(80, 12)
	if b.rows != 7 || len(lines) != 12 {
		t.Fatalf("list rows=%d rendered=%d", b.rows, len(lines))
	}
	if widthx.StripTerminalSequences(lines[1]) != "" || widthx.StripTerminalSequences(lines[9]) != "" {
		t.Fatal("browser spacer rows differ")
	}
	if !strings.Contains(widthx.StripTerminalSequences(lines[0]), "🔍") {
		t.Fatal("missing search icon")
	}
}
