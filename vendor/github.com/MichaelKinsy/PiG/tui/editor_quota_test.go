package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

func TestPromptQuotaBorderPreservesStatusAndWidth(t *testing.T) {
	e := NewEditor()
	e.EmbedWorkingStatus = true
	e.SetWorkingStatusIndicator(&StatusIndicator{Kind: "working", Loader: NewLoader(nil, nil, nil, "Working", nil)})
	label := "5h ███████░░░░░ 40% left · reset 2h"
	e.TopRightStatus = func(budget int) string {
		if widthx.VisibleWidth(label) <= budget {
			return label
		}
		return ""
	}
	for _, width := range []int{1, 10, 35, 80, 120} {
		fallback := e.renderTopBorder(width, 3)
		left := e.renderStatusBorder(width, 3, fallback)
		row := e.renderTopRightStatus(width, left)
		if widthx.VisibleWidth(row) != width {
			t.Fatalf("width %d produced %d cells: %q", width, widthx.VisibleWidth(row), row)
		}
		plain := widthx.StripAnsi(row)
		if strings.Contains(widthx.StripAnsi(left), "Working") && !strings.Contains(plain, "Working") {
			t.Fatalf("quota overwrote status: %q", plain)
		}
		if strings.Contains(widthx.StripAnsi(left), "↑ 3 more") && !strings.Contains(plain, "↑ 3 more") {
			t.Fatalf("quota overwrote scroll label: %q", plain)
		}
		if width == 120 && !strings.HasSuffix(plain, label+" ─") {
			t.Fatalf("quota not right aligned: %q", plain)
		}
	}
}

func TestWorkingStatusBorderShowsElapsedTime(t *testing.T) {
	indicator := &StatusIndicator{
		Kind:      "working",
		StartedAt: time.Now().Add(-65 * time.Second),
		Loader:    NewLoader(nil, nil, nil, "Working", nil),
	}
	border := widthx.StripAnsi(indicator.RenderInBorder(80))
	if !strings.Contains(border, "Working 01:05") {
		t.Fatalf("working status lacks elapsed time: %q", border)
	}
}

func TestPromptQuotaBorderUpdatesWhenIdle(t *testing.T) {
	e := NewEditor()
	label := "50% left"
	e.TopRightStatus = func(int) string { return label }
	e.Render(80)
	e.NeedsRedraw()
	if e.IsDirty() {
		t.Fatal("unchanged quota invalidated editor")
	}
	label = "25% left"
	if !e.IsDirty() {
		t.Fatal("quota update was hidden by render cache")
	}
	e.Render(80)
	e.NeedsRedraw()
	e.TopRightStatus = nil
	if !e.IsDirty() {
		t.Fatal("removing quota did not invalidate border")
	}
	row := widthx.StripAnsi(e.Render(80)[0])
	if strings.Contains(row, "left") {
		t.Fatal("removed quota remained visible")
	}
}
