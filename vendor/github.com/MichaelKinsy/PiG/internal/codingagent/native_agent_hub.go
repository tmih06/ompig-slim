package codingagent

// Ports oh-my-pi packages/tui/src/overlays/agent-hub.ts,
// agent-hub-projection.ts and agent-hub-renderer.ts at
// f2b795ba747b23d42052184d822b41e3d01357cc (MIT). Native sessions supply the
// roster and activity; no transcript or filesystem scan runs on the input loop.
import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/MichaelKinsy/PiG/internal/subagents"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

const hubSplitWidth = 96

type hubRank struct {
	batch, status     int
	activity, started int64
}
type agentHub struct {
	m                        *InteractiveMode
	manager                  *subagents.Manager
	handle                   *tui.OverlayHandle
	done                     chan struct{}
	closeOnce                sync.Once
	agents                   []subagents.HubAgent
	rows                     []subagents.HubAgent
	events                   []subagents.HubActivity
	ranks                    map[string]hubRank
	depth                    map[string]int
	selected                 string
	hovered                  string
	index, activityIndex     int
	section                  int
	tree, narrow, following  bool
	query, activityQuery     string
	editing                  bool
	input                    *tui.Editor
	filter, scope            int
	width, height, leftWidth int
	hitRows                  map[int]int
	notice                   string
	batch                    int
	lastLeft                 time.Time
}

func (m *InteractiveMode) toggleAgentHub() {
	if m.tuiInst == nil || m.subagentManager() == nil {
		return
	}
	if m.subagentUI == nil {
		m.bindSubagentUI()
	}
	if m.subagentUI.hub != nil {
		m.subagentUI.hub.close()
		return
	}
	for m.subagentUI.viewer != nil {
		m.subagentUI.viewer.close()
	}
	h := &agentHub{m: m, manager: m.subagentManager(), done: make(chan struct{}), ranks: map[string]hubRank{}, depth: map[string]int{}, following: true}
	m.subagentUI.hub = h
	options := tui.OverlaySpec{Width: &tui.OverlayValue{Value: 100, Percent: true}, MaxHeight: &tui.OverlayValue{Value: 100, Percent: true}, Margin: tui.OverlayMarginAll(0)}.Options()
	options.Fullscreen = m.altScreen != nil
	h.handle = m.tuiInst.ShowOverlay(h, options)
	// Relative ages and active durations keep moving even while the parent is idle.
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-h.done:
				return
			case <-ticker.C:
				m.wakeSpinner()
				m.requestRender()
			}
		}
	}()
	m.requestRender()
}
func (h *agentHub) Invalidate()                 {}
func (h *agentHub) SetFocused(bool)             {}
func (h *agentHub) CapturesViewportInput() bool { return true }
func (h *agentHub) close() {
	h.closeOnce.Do(func() {
		if h.done != nil {
			close(h.done)
		}
		if h.handle != nil {
			h.handle.Hide()
		}
		if h.m.subagentUI != nil && h.m.subagentUI.hub == h {
			h.m.subagentUI.hub = nil
		}
		h.m.requestRender()
	})
}

func hubSingleLine(text string) string {
	text = widthx.StripTerminalSequences(text)
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}
func hubLabel(r subagents.HubAgent) string {
	if r.Alias != "" {
		return r.Alias
	}
	if r.Handle != "" {
		return r.Handle
	}
	if r.DisplayName != "" {
		return r.DisplayName
	}
	return r.ID
}
func hubStatusOrder(status string) int {
	switch status {
	case "running":
		return 0
	case "idle":
		return 1
	case "parked":
		return 2
	}
	return 3
}
func hubGlyph(status string) string {
	theme := tui.ActiveTheme()
	switch status {
	case "running":
		return theme.Fg("accent", subagentSpinners[time.Now().UnixMilli()/100%10])
	case "idle":
		return theme.Fg("success", "●")
	case "parked":
		return theme.Fg("muted", "○")
	}
	return theme.Fg("error", "✕")
}
func hubDuration(milliseconds int64) string {
	seconds := max(0, milliseconds) / 1000
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm%ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh%dm", seconds/3600, seconds/60%60)
}
func hubMetrics(r subagents.HubAgent) string {
	if !r.Measured {
		return "usage —"
	}
	end := r.CompletedAt
	if end == 0 {
		end = time.Now().UnixMilli()
	}
	return fmt.Sprintf("$%.4f · %s span · %d req · %d tools · %s tok", r.Usage.Cost.Total, hubDuration(end-r.StartedAt), r.Requests, r.ToolUses, subagentCompact(int64(r.Usage.TotalTokens)))
}

func (h *agentHub) refresh() {
	agents := h.manager.HubSnapshot()
	initial := len(h.ranks) == 0
	newBatch := h.batch - 1
	added := false
	for _, r := range agents {
		rank, exists := h.ranks[r.ID]
		if !exists || rank.started != r.StartedAt {
			batch := 0
			if !initial {
				batch = newBatch
				added = true
			}
			h.ranks[r.ID] = hubRank{batch: batch, status: hubStatusOrder(r.State), activity: r.LastActive, started: r.StartedAt}
		}
	}
	if added {
		h.batch = newBatch
	}
	slices.SortStableFunc(agents, func(a, b subagents.HubAgent) int {
		x, y := h.ranks[a.ID], h.ranks[b.ID]
		if x.batch != y.batch {
			if x.batch < y.batch {
				return -1
			}
			return 1
		}
		if x.status != y.status {
			return x.status - y.status
		}
		if x.activity != y.activity {
			if x.activity > y.activity {
				return -1
			}
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	h.agents = agents
	rows := agents
	h.depth = map[string]int{}
	if h.tree {
		rows, h.depth = projectHubTree(agents, h.ranks)
	}
	h.rows = nil
	q := strings.ToLower(h.query)
	for _, r := range rows {
		if q == "" || strings.Contains(strings.ToLower(r.ID+" "+hubLabel(r)+" "+r.DisplayName), q) {
			h.rows = append(h.rows, r)
		}
	}
	found := false
	for i, r := range h.rows {
		if r.ID == h.selected {
			h.index = i
			found = true
			break
		}
	}
	if !found {
		h.index = min(max(0, h.index), max(0, len(h.rows)-1))
		if len(h.rows) > 0 {
			h.selected = h.rows[h.index].ID
		} else {
			h.selected = ""
		}
	}
	h.refreshActivity()
}

// Parent-first projection is stable while data changes, retains orphaned and
// cyclic persisted rows, and lets a new sibling appear without moving its parent.
func projectHubTree(rows []subagents.HubAgent, ranks map[string]hubRank) ([]subagents.HubAgent, map[string]int) {
	byID := map[string]subagents.HubAgent{}
	order := map[string]int{}
	children := map[string][]string{}
	for i, r := range rows {
		byID[r.ID] = r
		order[r.ID] = i
	}
	for _, r := range rows {
		parent := r.ParentID
		if _, ok := byID[parent]; !ok || parent == r.ID {
			parent = ""
		}
		children[parent] = append(children[parent], r.ID)
	}
	priority := map[string]int{}
	visiting, ranked := map[string]bool{}, map[string]bool{}
	type rankEntry struct {
		id       string
		expanded bool
	}
	for _, r := range rows {
		if ranked[r.ID] {
			continue
		}
		stack := []rankEntry{{id: r.ID}}
		for len(stack) > 0 {
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if current.expanded {
				own := order[current.id]
				best := own
				for _, child := range children[current.id] {
					if ranks[child].batch < ranks[current.id].batch && order[child] < own {
						continue
					}
					if p, ok := priority[child]; ok {
						best = min(best, p)
					}
				}
				priority[current.id] = best
				delete(visiting, current.id)
				ranked[current.id] = true
				continue
			}
			if ranked[current.id] || visiting[current.id] {
				continue
			}
			visiting[current.id] = true
			stack = append(stack, rankEntry{id: current.id, expanded: true})
			for _, child := range children[current.id] {
				if !ranked[child] && !visiting[child] {
					stack = append(stack, rankEntry{id: child})
				}
			}
		}
	}

	for parent := range children {
		sort.SliceStable(children[parent], func(i, j int) bool {
			a, b := children[parent][i], children[parent][j]
			if priority[a] != priority[b] {
				return priority[a] < priority[b]
			}
			return order[a] < order[b]
		})
	}
	var out []subagents.HubAgent
	depth := map[string]int{}
	seen := map[string]bool{}
	type entry struct {
		id    string
		level int
	}
	visit := func(root string) {
		stack := []entry{{root, 0}}
		for len(stack) > 0 {
			e := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[e.id] {
				continue
			}
			seen[e.id] = true
			depth[e.id] = e.level
			out = append(out, byID[e.id])
			desc := children[e.id]
			for i := len(desc) - 1; i >= 0; i-- {
				stack = append(stack, entry{desc[i], e.level + 1})
			}
		}
	}
	for _, id := range children[""] {
		visit(id)
	}
	for _, r := range rows {
		visit(r.ID)
	}
	return out, depth
}

func (h *agentHub) refreshActivity() {
	previous := uint64(0)
	if h.activityIndex < len(h.events) {
		previous = h.events[h.activityIndex].ID
	}
	h.events = nil
	query := strings.ToLower(h.activityQuery)
	subtree := map[string]bool{}
	if h.scope == 2 {
		children := map[string][]string{}
		for _, r := range h.agents {
			children[r.ParentID] = append(children[r.ParentID], r.ID)
		}
		pending := []string{h.selected}
		for len(pending) > 0 {
			id := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if subtree[id] {
				continue
			}
			subtree[id] = true
			pending = append(pending, children[id]...)
		}
	}
	for _, e := range h.manager.HubActivity() {
		if h.filter == 1 && e.Status != "error" && e.Status != "aborted" {
			continue
		}
		if h.filter == 2 && e.Kind != "response" {
			continue
		}
		if h.filter == 3 && e.Kind != "tool" {
			continue
		}
		if h.scope == 1 && e.AgentID != h.selected {
			continue
		}
		if h.scope == 2 && !subtree[e.AgentID] {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(e.AgentID+" "+e.Title+" "+e.Summary), query) {
			continue
		}
		h.events = append(h.events, e)
	}
	h.activityIndex = min(max(0, h.activityIndex), max(0, len(h.events)-1))
	if h.following {
		h.activityIndex = max(0, len(h.events)-1)
	} else {
		for i, e := range h.events {
			if e.ID == previous {
				h.activityIndex = i
				break
			}
		}
	}
}

func (h *agentHub) summary() []string {
	counts := map[string]int{}
	measured, requests, tools, tokens := 0, 0, 0, 0
	cost := 0.0
	for _, r := range h.rows {
		counts[r.State]++
		if r.Measured {
			measured++
			requests += r.Requests
			tools += r.ToolUses
			tokens += r.Usage.TotalTokens
			cost += r.Usage.Cost.Total
		}
	}
	var status []string
	for _, state := range []string{"running", "idle", "parked", "aborted"} {
		if counts[state] > 0 {
			status = append(status, hubStatus(state, fmt.Sprintf("%s %d %s", hubGlyph(state), counts[state], state)))
		}
	}
	view := hubChip("Flat", !h.tree) + tui.ActiveTheme().Fg("dim", "/") + hubChip("By parent", h.tree)
	header := tui.ChalkBold("Roster") + tui.ActiveTheme().Fg("dim", " · ") + view
	if len(status) > 0 {
		header += " · " + strings.Join(status, " · ")
	}
	usage := fmt.Sprintf("Usage — · 0/%d measured", len(h.rows))
	if measured > 0 {
		usage = fmt.Sprintf("$%.4f · %d req · %d tools · %s tok · %d/%d measured", cost, requests, tools, subagentCompact(int64(tokens)), measured, len(h.rows))
	}
	lines := []string{header, tui.ActiveTheme().Fg("dim", usage)}
	if h.editing || h.query != "" {
		lines = append(lines, tui.ActiveTheme().Fg("accent", "/"+h.query))
	}
	return append(lines, "")
}
func (h *agentHub) roster(width, height int) []string {
	lines := h.summary()
	for i := range lines {
		h.hitRows[2+i] = -1
	}
	if len(h.rows) == 0 {
		text := "No agents in this session"
		if h.query != "" {
			text = "No matching agents"
		}
		return append(lines, tui.ChalkBold(text), tui.ActiveTheme().Fg("dim", "Spawn an agent here, or resume a session with saved agents."))
	}
	budget := max(1, height-len(lines))
	visible := max(1, budget/3)
	start := max(0, min(h.index-visible/2, len(h.rows)-visible))
	end := min(len(h.rows), start+visible)
	for i := start; i < end; i++ {
		r := h.rows[i]
		cursor := " "
		if r.ID == h.selected {
			cursor = tui.ActiveTheme().Fg("accent", "›")
		}
		indent := ""
		if h.tree && h.depth[r.ID] > 0 {
			indent = strings.Repeat("  ", min(12, h.depth[r.ID]-1)) + "└─ "
		}
		label := hubSingleLine(hubLabel(r))
		if r.ID == h.selected {
			label = tui.ActiveTheme().Fg("accent", tui.ChalkBold(label))
		} else {
			label = tui.ChalkBold(label)
		}
		title := cursor + " " + indent + hubGlyph(r.State) + " " + label
		if !h.tree && r.ParentID != "" {
			title += "  ↳ " + hubSingleLine(h.labelFor(r.ParentID))
		}
		model := hubModel(r.ModelName, r.Thinking)
		if r.ModelName != "" && widthx.VisibleWidth(title)+widthx.VisibleWidth(model)+2 <= width {
			title += strings.Repeat(" ", width-widthx.VisibleWidth(title)-widthx.VisibleWidth(model)) + model
		}
		detail := r.Description
		if detail == "" {
			detail = r.Activity
		}
		metrics := hubMetrics(r) + " · " + hubDuration(time.Now().UnixMilli()-r.LastActive) + " ago"
		for _, line := range []string{title, tui.ActiveTheme().Fg("muted", "    "+hubSingleLine(detail)), tui.ActiveTheme().Fg("dim", "    "+metrics)} {
			if len(lines) >= height {
				break
			}
			h.hitRows[2+len(lines)] = i
			if r.ID == h.hovered {
				line = hubSurface(line, width, "selectedBg")
			}
			lines = append(lines, line)
		}
	}
	if end < len(h.rows) && len(lines) < height {
		lines = append(lines, tui.ActiveTheme().Fg("dim", fmt.Sprintf("… %d more", len(h.rows)-end)))
	}
	return lines
}
func (h *agentHub) labelFor(id string) string {
	if id == "" {
		return "main"
	}
	for _, r := range h.agents {
		if r.ID == id {
			return hubLabel(r)
		}
	}
	return id
}
func (h *agentHub) details(width int) []string {
	if len(h.rows) == 0 {
		return []string{"Select an agent to inspect"}
	}
	r := h.rows[h.index]
	lines := []string{hubGlyph(r.State) + " " + tui.ChalkBold(hubSingleLine(hubLabel(r))), tui.ActiveTheme().Fg("dim", r.ID), hubStatus(r.State, r.State) + " · last active " + hubDuration(time.Now().UnixMilli()-r.LastActive) + " ago"}
	if r.ModelName != "" {
		lines = append(lines, hubModel(r.ModelName, r.Thinking))
	}
	lines = append(lines, "", hubHeading("Task"), hubSingleLine(r.Description))
	if r.Params.Prompt != "" {
		lines = append(lines, hubSingleLine(r.Params.Prompt))
	}
	if r.Activity != "" {
		lines = append(lines, "", hubHeading("Current"), hubSingleLine(r.Activity))
	}
	lines = append(lines, "", hubHeading("Usage"), hubMetrics(r))
	if r.ContextWindow > 0 {
		fraction := min(1.0, max(0.0, float64(r.ContextTokens)/float64(r.ContextWindow)))
		cells := min(10, max(1, width-4))
		filled := int(fraction * float64(cells))
		lines = append(lines, tui.ActiveTheme().Fg("accent", strings.Repeat("━", filled))+tui.ActiveTheme().Fg("dim", strings.Repeat("─", cells-filled))+fmt.Sprintf(" %s/%s %.0f%%", subagentCompact(r.ContextTokens), subagentCompact(int64(r.ContextWindow)), fraction*100))
	}
	lines = append(lines, "", hubHeading("Lineage"), "Spawned by "+hubSingleLine(h.labelFor(r.ParentID)))
	var children []string
	for _, child := range h.agents {
		if child.ParentID == r.ID {
			children = append(children, hubLabel(child))
		}
	}
	if len(children) > 0 {
		display := children[:min(12, len(children))]
		line := strings.Join(display, ", ")
		if len(children) > 12 {
			line += fmt.Sprintf(" … +%d", len(children)-12)
		}
		lines = append(lines, "Children: "+hubSingleLine(line))
	}
	if r.WorkflowID != "" {
		lines = append(lines, "Workflow: "+r.WorkflowID)
	}
	lines = append(lines, "Registered: "+time.UnixMilli(r.StartedAt).Format(time.RFC3339))
	for _, path := range []struct{ label, value string }{{"Session", r.SessionFile}, {"Output", r.OutputFile}, {"Worktree", r.WorktreePath}, {"Branch", r.WorktreeBranch}} {
		if path.value != "" {
			lines = append(lines, "", hubHeading(path.label), hubSingleLine(path.value))
		}
	}
	if r.Error != "" {
		lines = append(lines, "", tui.ActiveTheme().Fg("error", hubSingleLine(r.Error)))
	}
	// The inspector is a fixed summary, not a second transcript. Fit metadata
	// to the viewport and use only the remaining rows for compact recent events.
	height := max(1, h.height-5)
	var wrapped []string
	for _, line := range lines {
		wrapped = append(wrapped, widthx.WrapTextWithAnsi(line, max(1, width))...)
	}
	// Reserve the compact activity summary so saved session/output paths do
	// not push every event off-screen when inspecting a resumed agent.
	budget := min(3, max(0, height-5))
	metadataRows := height
	if budget > 0 {
		metadataRows -= budget + 2
	}
	wrapped = wrapped[:min(len(wrapped), metadataRows)]
	if budget == 0 {
		return wrapped
	}
	wrapped = append(wrapped, "", hubHeading("Recent activity"))
	events := h.manager.HubActivity()
	count := 0
	for i := len(events) - 1; i >= 0 && count < budget; i-- {
		if events[i].AgentID == r.ID {
			count++
			wrapped = append(wrapped, clampAgentLine(hubSingleLine(events[i].Title+" · "+events[i].Summary), width))
		}
	}
	if count == 0 {
		wrapped = append(wrapped, clampAgentLine(tui.ActiveTheme().Fg("dim", "No response or tool activity yet"), width))
	}
	return wrapped
}
func (h *agentHub) activityLines(width, height int) []string {
	filters := []string{"All", "Errors", "Responses", "Tools"}
	scopes := []string{"all agents", h.labelFor(h.selected), h.labelFor(h.selected) + " subtree"}
	follow := "paused"
	if h.following {
		follow = "following"
	}
	lines := []string{tui.ActiveTheme().Fg("dim", scopes[h.scope]+" · "+filters[h.filter]+" · "+follow)}
	if h.editing || h.activityQuery != "" {
		lines = append(lines, tui.ActiveTheme().Fg("accent", "/"+h.activityQuery))
	}
	lines = append(lines, "")
	if len(h.events) == 0 {
		return append(lines, "No agent activity recorded yet")
	}
	budget := max(1, height-len(lines))
	start := max(0, min(h.activityIndex-budget/2, len(h.events)-budget))
	for i := start; i < len(h.events) && len(lines) < height; i++ {
		e := h.events[i]
		cursor := " "
		if i == h.activityIndex {
			cursor = tui.ActiveTheme().Fg("accent", "›")
		}
		icon := "○"
		switch e.Kind {
		case "tool":
			icon = tui.ActiveTheme().Fg("warning", "→")
		case "response":
			icon = tui.ActiveTheme().Fg("success", "◆")
		}
		if e.Status == "error" || e.Status == "aborted" {
			icon = tui.ActiveTheme().Fg("error", "✕")
		}
		line := fmt.Sprintf("%s %s %s %s %s · %s", cursor, tui.ActiveTheme().Fg("dim", time.UnixMilli(e.Timestamp).Format("15:04:05")), icon, tui.ChalkBold(hubSingleLine(h.labelFor(e.AgentID))), tui.ActiveTheme().Fg(map[bool]string{true: "success", false: "muted"}[e.Kind == "response"], hubSingleLine(e.Title)), hubSingleLine(e.Summary))
		h.hitRows[2+len(lines)] = i
		lines = append(lines, line)
	}
	return lines
}

func (h *agentHub) Render(width int) []string {
	h.refresh()
	h.width = width
	h.height = max(6, h.m.tuiInst.Height())
	h.hitRows = map[int]int{}
	if width < 6 {
		return []string{clampAgentLine("Agent Hub", width)}
	}
	theme := tui.ActiveTheme()
	inner := width - 4
	bodyRows := max(1, h.height-5)
	tabs := []string{hubChip("1 Agents", h.section == 0), hubChip("2 Activity", h.section == 1)}
	border := func(text string) string { return theme.Fg("border", text) }
	lines := []string{
		hubSurface(border("╭─ ")+hubHeading("Agent Hub")+border(" "+strings.Repeat("─", max(0, width-14))+"╮"), width, ""),
		hubSurface(border("│ ")+hubPad(strings.Join(tabs, theme.Fg("dim", " · ")), inner)+border(" │"), width, ""),
	}
	split := width >= hubSplitWidth && h.section == 0
	h.leftWidth = inner
	var left, right []string
	if h.section == 1 {
		left = h.activityLines(inner, bodyRows)
	} else if split {
		h.leftWidth = max(48, (width-7)*3/5)
		rightWidth := width - 7 - h.leftWidth
		left = h.roster(h.leftWidth, bodyRows)
		right = h.details(rightWidth)
	} else if h.narrow {
		left = h.details(inner)
	} else {
		left = h.roster(inner, bodyRows)
	}
	for i := 0; i < bodyRows; i++ {
		l := ""
		if i < len(left) {
			l = left[i]
		}
		if split {
			r := ""
			if i < len(right) {
				r = right[i]
			}
			lines = append(lines, hubSurface(border("│ ")+hubPad(l, h.leftWidth)+border(" │ ")+hubSurface(r, width-7-h.leftWidth, "")+border(" │"), width, ""))
		} else {
			lines = append(lines, hubSurface(border("│ ")+hubPad(l, inner)+border(" │"), width, ""))
		}
	}
	footer := "j/k:select  enter:open  tab:by parent  /:filter  r:revive  x:kill  esc:close"
	if h.tree {
		footer = strings.Replace(footer, "tab:by parent", "tab:flat", 1)
	}
	if !split && h.section == 0 {
		footer = "tab:view  shift+tab:details  enter:open  /:filter  r/x:manage  esc:close"
	}
	if h.section == 1 {
		footer = "1:agents  j/k:select  enter:transcript  space:follow  f:filter  s:scope  /:search  esc:close"
	}
	if h.editing {
		footer = "/" + h.input.Text() + "  · enter:apply  esc:clear"
	}
	if h.notice != "" {
		footer = h.notice + " · " + footer
	}
	lines = append(lines, hubSurface(border("├"+strings.Repeat("─", width-2)+"┤"), width, ""), hubSurface(border("│ ")+hubPad(theme.Fg("dim", footer), inner)+border(" │"), width, ""), hubSurface(border("╰"+strings.Repeat("─", width-2)+"╯"), width, ""))
	for i := range lines {
		lines[i] = clampAgentLine(lines[i], width)
	}
	return lines
}
func hubPad(line string, width int) string {
	line = clampAgentLine(line, width)
	return line + strings.Repeat(" ", max(0, width-widthx.VisibleWidth(line)))
}

func (h *agentHub) selectRow(index int) {
	h.index = min(max(0, index), max(0, len(h.rows)-1))
	if len(h.rows) > 0 {
		h.selected = h.rows[h.index].ID
	}
	h.notice = ""
}
func (h *agentHub) activate() {
	id := h.selected
	anchor := -1
	if h.section == 1 {
		if len(h.events) == 0 {
			return
		}
		event := h.events[h.activityIndex]
		id = event.AgentID
		anchor = event.MessageIndex
	}
	if id == "" {
		return
	}
	h.close()
	h.m.openSubagentViewer(id, false, true)
	if v := h.m.subagentUI.viewer; v != nil {
		v.focusSession = true
		v.composer = tui.NewEditor()
		if anchor >= 0 {
			v.anchorMessage = anchor
			v.seekAnchor = true
			v.follow = false
		}
	}
}
func (h *agentHub) HandleInput(data string) {
	h.hovered = ""
	h.refresh()
	if tui.MatchesKeyID(data, "alt+a") || tui.MatchesKeyID(data, "ctrl+s") {
		h.close()
		return
	}
	if h.editing {
		if tui.MatchesKeyID(data, "escape") {
			h.editing = false
			h.input = nil
			if h.section == 0 {
				h.query = ""
			} else {
				h.activityQuery = ""
			}
		} else if tui.MatchesKeyID(data, "enter") {
			h.editing = false
			h.input = nil
		} else {
			h.input.HandleInput(data)
			if h.section == 0 {
				h.query = h.input.Text()
			} else {
				h.activityQuery = h.input.Text()
			}
		}
		h.refresh()
		h.m.requestRender()
		return
	}
	switch {
	case tui.MatchesKeyID(data, "escape"), tui.MatchesKeyID(data, "ctrl+c"):
		if h.section == 0 && h.query != "" {
			h.query = ""
		} else if h.section == 1 && h.activityQuery != "" {
			h.activityQuery = ""
		} else if h.section == 0 && h.narrow && h.width < hubSplitWidth {
			h.narrow = false
		} else {
			h.close()
		}
	case data == "1":
		h.section = 0
	case data == "2":
		h.section = 1
	case data == "/":
		h.editing = true
		h.input = tui.NewEditor()
		if h.section == 0 {
			h.input.SetText(h.query)
		} else {
			h.input.SetText(h.activityQuery)
		}
	case tui.MatchesKeyID(data, "enter"):
		h.activate()
	case data == "j" || tui.MatchesKeyID(data, "down"):
		if h.section == 1 {
			h.following = false
			h.activityIndex = min(max(0, len(h.events)-1), h.activityIndex+1)
		} else {
			h.selectRow(h.index + 1)
		}
	case data == "k" || tui.MatchesKeyID(data, "up"):
		if h.section == 1 {
			h.following = false
			h.activityIndex = max(0, h.activityIndex-1)
		} else {
			h.selectRow(h.index - 1)
		}
	case tui.MatchesKeyID(data, "shift+tab") && h.section == 0 && h.width < hubSplitWidth:
		h.narrow = !h.narrow
	case tui.MatchesKeyID(data, "tab") && h.section == 0:
		h.tree = !h.tree
	case data == "r" && h.section == 0:
		if err := h.manager.ReviveObserved(h.selected); err != nil {
			h.notice = err.Error()
		}
	case data == "x" && h.section == 0:
		if err := h.manager.KillObserved(h.selected); err != nil {
			h.notice = err.Error()
		}
	case data == "f" && h.section == 1:
		h.filter = (h.filter + 1) % 4
	case data == "s" && h.section == 1:
		h.scope = (h.scope + 1) % 3
	case data == " " && h.section == 1:
		h.following = !h.following
	case tui.MatchesKeyID(data, "left"):
		if h.narrow && h.width < hubSplitWidth {
			h.narrow = false
		} else {
			now := time.Now()
			if !h.lastLeft.IsZero() && now.Sub(h.lastLeft) < 500*time.Millisecond {
				h.close()
			}
			h.lastLeft = now
		}
	}
	h.m.requestRender()
}
func (h *agentHub) HandleMouse(event tui.TuiMouseEvent) *tui.TuiMouseDispatchResult {
	if event.Type == tui.MouseMove {
		hovered := ""
		if h.section == 0 && (h.width >= hubSplitWidth || !h.narrow) && event.X <= h.leftWidth+2 {
			if index, ok := h.hitRows[event.Y]; ok && index >= 0 && index < len(h.rows) {
				hovered = h.rows[index].ID
			}
		}
		if h.hovered != hovered {
			h.hovered = hovered
			h.m.requestRender()
		}
		return &tui.TuiMouseDispatchResult{TuiMouseEventResult: tui.TuiMouseEventResult{Handled: true}}
	}
	h.hovered = ""
	if event.Type == tui.MouseWheel {
		if h.section == 0 && ((h.width >= hubSplitWidth && event.X > h.leftWidth+2) || (h.width < hubSplitWidth && h.narrow)) {
			// Consume wheel input without scrolling the fixed inspector or roster.
		} else if h.section == 1 {
			h.following = false
			h.activityIndex = min(max(0, h.activityIndex+event.WheelDelta), max(0, len(h.events)-1))
		} else {
			h.selectRow(h.index + event.WheelDelta)
		}
	} else if event.Type == tui.MouseClick {
		if event.Y == 1 {
			if event.X < 15 {
				h.section = 0
			} else {
				h.section = 1
			}
		} else if index, ok := h.hitRows[event.Y]; ok && index >= 0 && event.X <= h.leftWidth+2 {
			if h.section == 1 {
				h.activityIndex = index
			} else {
				h.selectRow(index)
			}
			h.activate()
		}
	}
	h.m.requestRender()
	return &tui.TuiMouseDispatchResult{TuiMouseEventResult: tui.TuiMouseEventResult{Handled: true}}
}
