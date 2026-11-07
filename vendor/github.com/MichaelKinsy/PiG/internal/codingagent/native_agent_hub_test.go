package codingagent

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/internal/subagents"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

func TestHubResponsiveRosterFilteringAndFocus(t *testing.T) {
	m, _ := observerFixture(t)
	m.toggleAgentHub()
	h := m.subagentUI.hub
	for _, width := range []int{12, 40, 80, 95, 96, 130} {
		lines := h.Render(width)
		if len(lines) > m.tuiInst.Height() {
			t.Fatalf("hub exceeds viewport at %d", width)
		}
		for _, line := range lines {
			if widthx.VisibleWidth(line) > width {
				t.Fatalf("hub overflows %d: %q", width, line)
			}
		}
		if width >= 40 && !strings.HasSuffix(widthx.StripTerminalSequences(lines[0]), "╮") {
			t.Fatal("Hub header lost its closing corner", lines[0])
		}
		if width >= 40 && !strings.Contains(strings.Join(lines, "\n"), "explore") {
			t.Fatal("agent missing from roster", lines)
		}
		selected := h.selected
		h.HandleInput("\t")
		if !h.tree || h.narrow || h.selected != selected {
			t.Fatalf("Tab must switch to parent view without changing focus at width %d", width)
		}
		h.HandleInput("t")
		if !h.tree {
			t.Fatal("t still switches view")
		}
		h.HandleInput("\t")
		if h.tree || h.narrow {
			t.Fatal("Tab did not return to flat view")
		}
	}
	h.Render(80)
	h.HandleInput("\x1b[Z")
	if !h.narrow {
		t.Fatal("Shift+Tab did not open narrow inspector")
	}
	h.HandleInput("\x1b")
	if h.narrow || m.subagentUI.hub == nil {
		t.Fatal("Escape did not return to narrow roster")
	}
	h.HandleInput("/")
	h.HandleInput("missing-agent")
	h.HandleInput("\r")
	if len(h.rows) != 0 {
		h.refresh()
		if len(h.rows) != 0 {
			t.Fatal("roster filter ignored")
		}
	}
	h.HandleInput("\x1b")
	h.refresh()
	if len(h.rows) != 1 || m.subagentUI.hub == nil {
		t.Fatal("Escape did not clear filter before closing")
	}
	h.HandleInput("\r")
	if m.subagentUI.hub != nil || m.subagentUI.viewer == nil || !m.subagentUI.viewer.focusSession {
		t.Fatal("Enter did not focus selected agent")
	}
	m.subagentUI.viewer.HandleInput("\x1b")
	if m.subagentUI.viewer != nil {
		t.Fatal("empty focused editor Escape did not return to main")
	}
}

func TestHubOrderingDoesNotJumpOnStatusChange(t *testing.T) {
	m, manager := observerFixture(t)
	m.toggleAgentHub()
	h := m.subagentUI.hub
	h.refresh()
	before := h.selected
	rank := h.ranks[before]
	if err := manager.KillObserved(before); err != nil {
		t.Fatal(err)
	}
	h.refresh()
	if h.selected != before || h.ranks[before] != rank || h.rows[0].State != "aborted" {
		t.Fatal("roster rank changed with lifecycle", h.selected, h.ranks)
	}
}

func TestHubTreeRetainsOrphansAndCycles(t *testing.T) {
	rows := []subagents.HubAgent{
		{Record: subagents.Record{ID: "child", ParentID: "parent"}},
		{Record: subagents.Record{ID: "solo"}},
		{Record: subagents.Record{ID: "parent"}},
		{Record: subagents.Record{ID: "orphan", ParentID: "missing"}},
		{Record: subagents.Record{ID: "a", ParentID: "b"}},
		{Record: subagents.Record{ID: "b", ParentID: "a"}},
	}
	tree, depth := projectHubTree(rows, map[string]hubRank{})
	if tree[0].ID != "parent" || tree[1].ID != "child" || depth["child"] != 1 {
		t.Fatal("parent-first projection failed", tree, depth)
	}
	seen := map[string]bool{}
	for _, r := range tree {
		if seen[r.ID] {
			t.Fatal("duplicate cyclic row")
		}
		seen[r.ID] = true
	}
	if len(seen) != len(rows) {
		t.Fatal("orphan or cyclic row disappeared")
	}
}

func TestHubLargeTreeProjectionAndPaint(t *testing.T) {
	rows := make([]subagents.HubAgent, 5000)
	for i := range rows {
		parent := ""
		if i > 0 {
			parent = fmt.Sprint(i - 1)
		}
		rows[i] = subagents.HubAgent{Record: subagents.Record{ID: fmt.Sprint(i), ParentID: parent}}
	}
	tree, depth := projectHubTree(rows, map[string]hubRank{})
	if len(tree) != len(rows) || depth["4999"] != 4999 {
		t.Fatal("deep lineage lost")
	}
}

func TestHubDoubleLeftAndOpenWithEmptyRoster(t *testing.T) {
	m, manager := observerFixture(t)
	if !m.handleAgentKey("\x1b[D") || m.subagentUI.hub != nil {
		t.Fatal("first left opened hub")
	}
	if !m.handleAgentKey("\x1b[D") || m.subagentUI.hub == nil {
		t.Fatal("double-left did not open hub")
	}
	m.subagentUI.hub.close()
	if err := manager.Update(map[string]any{"enabled": false}); err != nil {
		t.Fatal(err)
	}
	if m.handleAgentKey("\x1ba") {
		t.Fatal("disabled subagents captured hub key")
	}
}

func TestTurnLimitUsesReadableText(t *testing.T) {
	line := widthx.StripAnsi(subagentStats(subagents.Record{Turns: 0, MaxTurns: 2, StartedAt: time.Now().UnixMilli()}, subagents.DefaultSettings()))
	if !strings.Contains(line, "0/2 turns") || strings.ContainsAny(line, "↻≤") {
		t.Fatal(line)
	}
	line = widthx.StripAnsi(subagentStats(subagents.Record{Turns: 3, StartedAt: time.Now().UnixMilli()}, subagents.DefaultSettings()))
	if !strings.Contains(line, "3 turns") {
		t.Fatal(line)
	}
}

func BenchmarkHubTreeProjection(b *testing.B) {
	rows := make([]subagents.HubAgent, 5000)
	for i := range rows {
		parent := ""
		if i%5 != 0 {
			parent = fmt.Sprint(i - i%5)
		}
		rows[i] = subagents.HubAgent{Record: subagents.Record{ID: fmt.Sprint(i), ParentID: parent}}
	}
	b.ReportAllocs()
	for b.Loop() {
		projectHubTree(rows, map[string]hubRank{})
	}
}

func TestHubMouseSelectionAndFixedInspector(t *testing.T) {
	m, _ := observerFixture(t)
	m.toggleAgentHub()
	h := m.subagentUI.hub
	before := strings.Join(h.Render(120), "\n")
	selected := h.selected
	for _, delta := range []int{1, -1, 20} {
		result := h.HandleMouse(tui.TuiMouseEvent{Type: tui.MouseWheel, X: 110, WheelDelta: delta})
		if !result.Handled || h.selected != selected || strings.Join(h.Render(120), "\n") != before {
			t.Fatal("inspector wheel changed the fixed panel or roster selection")
		}
	}
	for _, key := range []string{"\x1b[6~", "\x1b[5~"} {
		h.HandleInput(key)
		if strings.Join(h.Render(120), "\n") != before {
			t.Fatal("page key scrolled the inspector")
		}
	}
	h.Render(80)
	h.HandleInput("\x1b[Z")
	before = strings.Join(h.Render(80), "\n")
	h.HandleInput("\x1b[6~")
	h.HandleMouse(tui.TuiMouseEvent{Type: tui.MouseWheel, X: 40, WheelDelta: 1})
	if strings.Join(h.Render(80), "\n") != before {
		t.Fatal("narrow inspector scrolled")
	}
	h.HandleInput("2")
	h.HandleInput("f")
	h.HandleInput("s")
	if h.section != 1 || h.filter != 1 || h.scope != 1 {
		t.Fatal("activity controls ignored")
	}
}

func TestHubInspectorCompactRecentActivity(t *testing.T) {
	m, manager := observerFixture(t)
	m.toggleAgentHub()
	h := m.subagentUI.hub
	h.Render(130)
	for range 10 {
		if err := manager.KillObserved(h.selected); err != nil {
			t.Fatal(err)
		}
	}
	h.refresh()
	for _, height := range []int{8, 24, 40, 100} {
		h.height = height
		lines := h.details(50)
		text := widthx.StripTerminalSequences(strings.Join(lines, "\n"))
		if strings.Contains(text, "Last response") || strings.Contains(text, "Found auth.go") {
			t.Fatal("inspector still shows the last response")
		}
		if len(lines) > height-5 {
			t.Fatalf("inspector exceeds available height: %d > %d", len(lines), height-5)
		}
		count := strings.Count(text, "Killed · Agent instance released")
		if count > 3 || (height >= 24 && count != 3) {
			t.Fatalf("expected at most three compact recent events, got %d", count)
		}
		for _, line := range lines {
			if widthx.VisibleWidth(line) > 50 {
				t.Fatal("inspector overflowed its width")
			}
		}
	}
	// Narrow summaries stay one row per event instead of wrapping the log.
	lines := h.details(18)
	text := widthx.StripTerminalSequences(strings.Join(lines, "\n"))
	if strings.Count(text, "Killed") != 3 {
		t.Fatal("narrow inspector lost its three-event summary")
	}
	for _, line := range lines {
		if widthx.VisibleWidth(line) > 18 {
			t.Fatal("recent event was not trimmed to inspector width")
		}
	}
}

func TestHubUsesOMPSelectionAndHoverStyling(t *testing.T) {
	m, _ := observerFixture(t)
	m.toggleAgentHub()
	h := m.subagentUI.hub
	lines := h.Render(130)
	theme := tui.ActiveTheme()
	selected := theme.GetBgAnsi("selectedBg")
	firstRow := -1
	for row, index := range h.hitRows {
		if index != h.index {
			continue
		}
		if firstRow < 0 || row < firstRow {
			firstRow = row
		}
		if strings.Contains(lines[row], selected) {
			t.Fatal("keyboard selection must use an accent cursor, not a filled row")
		}
	}
	if firstRow < 0 || !strings.Contains(lines[firstRow], theme.Fg("accent", "›")) {
		t.Fatal("selected row is missing OMP's accent cursor")
	}
	for _, row := range []int{0, 5, 10, len(lines) - 1} {
		for _, token := range []string{"customMessageBg", "toolPendingBg", "selectedBg"} {
			if strings.Contains(lines[row], theme.GetBgAnsi(token)) {
				t.Fatalf("row %d has an unwanted panel fill", row)
			}
		}
	}
	h.HandleMouse(tui.TuiMouseEvent{Type: tui.MouseMove, X: 5, Y: firstRow})
	lines = h.Render(130)
	count := 0
	for row, index := range h.hitRows {
		if index == h.index {
			if !strings.Contains(lines[row], selected) {
				t.Fatalf("hovered row %d has no hover background", row)
			}
			if widthx.VisibleWidth(lines[row]) != 130 {
				t.Fatal("hover fill overflowed")
			}
			count++
		}
	}
	if count != 3 {
		t.Fatalf("expected three hovered lines, got %d", count)
	}
	h.HandleMouse(tui.TuiMouseEvent{Type: tui.MouseMove, X: 110, Y: firstRow})
	if h.hovered != "" {
		t.Fatal("hover did not clear in the inspector")
	}
	h.narrow = true // Remembered inspector pane after resizing back to a wide terminal.
	h.HandleMouse(tui.TuiMouseEvent{Type: tui.MouseMove, X: 5, Y: firstRow})
	if h.hovered == "" {
		t.Fatal("remembered narrow details blocked hover in a wide Hub")
	}
	h.HandleInput("j")
	if h.hovered != "" {
		t.Fatal("keyboard navigation retained hover fill")
	}
}

func TestHubOMPChipsAndModelBadge(t *testing.T) {
	theme := tui.ActiveTheme()
	if hubChip("Flat", true) != theme.Bg("selectedBg", tui.ChalkBold(theme.Fg("accent", " Flat "))) {
		t.Fatal("active view chip differs from OMP styling")
	}
	if hubChip("By parent", false) != theme.Fg("muted", " By parent ") {
		t.Fatal("inactive chip differs from OMP styling")
	}
	if strings.Contains(widthx.StripTerminalSequences(hubModel("fixture", "off")), "off") {
		t.Fatal("off thinking level should be hidden")
	}
	if !strings.Contains(hubModel("fixture", "high"), theme.Fg("thinkingHigh", "high")) {
		t.Fatal("thinking badge lost its level color")
	}
	if hubStatus("parked", "parked") != theme.Fg("muted", "parked") {
		t.Fatal("parked status must use OMP's muted tone")
	}
}
