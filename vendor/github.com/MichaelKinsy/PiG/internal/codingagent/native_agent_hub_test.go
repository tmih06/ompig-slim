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

func TestHubMouseSelectionAndInspectorScroll(t *testing.T) {
	m, _ := observerFixture(t)
	m.toggleAgentHub()
	h := m.subagentUI.hub
	h.Render(120)
	result := h.HandleMouse(tui.TuiMouseEvent{Type: tui.MouseWheel, X: 110, WheelDelta: 1})
	if !result.Handled || h.detailScroll != 3 {
		t.Fatal("inspector wheel was not local")
	}
	h.HandleInput("2")
	h.HandleInput("f")
	h.HandleInput("s")
	if h.section != 1 || h.filter != 1 || h.scope != 1 {
		t.Fatal("activity controls ignored")
	}
}

func TestHubSelectionUsesWholeRowSurfaces(t *testing.T) {
	m, _ := observerFixture(t)
	m.toggleAgentHub()
	h := m.subagentUI.hub
	lines := h.Render(130)
	theme := tui.ActiveTheme()
	selected := theme.GetBgAnsi("selectedBg")
	count := 0
	for row, index := range h.hitRows {
		if index == h.index {
			if !strings.Contains(lines[row], selected) {
				t.Fatalf("selected agent row %d has no selection surface", row)
			}
			if widthx.VisibleWidth(lines[row]) != 130 {
				t.Fatal("surface did not fill the viewport width")
			}
			count++
		}
	}
	if count != 3 {
		t.Fatalf("expected all three selected rows to be highlighted, got %d", count)
	}
	if !strings.Contains(lines[0], theme.GetBgAnsi("customMessageBg")) || !strings.Contains(lines[5], theme.GetBgAnsi("toolPendingBg")) {
		t.Fatal("Hub chrome and roster must use separate surfaces")
	}
	// Nested chips must restore the surrounding panel, including full resets.
	panel := theme.GetBgAnsi("toolPendingBg")
	painted := hubSurface(theme.Bg("selectedBg", "selected")+"\x1b[0m tail", 30, "toolPendingBg")
	if !strings.Contains(painted, "\x1b[0m"+panel) {
		t.Fatal("full reset did not restore enclosing surface")
	}
}
