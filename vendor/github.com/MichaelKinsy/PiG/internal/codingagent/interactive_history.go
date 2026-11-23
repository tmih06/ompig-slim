package codingagent

import (
	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
)

const transcriptPageSize = 200

type transcriptHistory struct {
	// oldestID and oldestComp track the oldest painted unit: the paging
	// frontier sits just above it, so the next page splices before
	// oldestComp. A unit that rendered no children (undisplayed entries)
	// still advances oldestID while oldestComp keeps pointing at the first
	// rendered row below the frontier.
	oldestID   string
	oldestComp tui.Component
	more       bool
	shown      map[string]struct{}
	// baseCount anchors page insertion at the first transcript child, so
	// pre-transcript children (loaded resources, notices) stay above the
	// paged history.
	baseCount          int
	pendingToolResults map[string]*agent.ToolResultMessage
}

func (m *InteractiveMode) initTranscriptHistory(boundaryID string, baseCount int, seedShown []string) {
	if boundaryID == "" {
		return
	}
	shown := make(map[string]struct{}, len(seedShown)+8)
	for _, id := range seedShown {
		if id != "" {
			shown[id] = struct{}{}
		}
	}
	m.transcriptHistory = &transcriptHistory{
		oldestID:           boundaryID,
		more:               true,
		shown:              shown,
		baseCount:          baseCount,
		pendingToolResults: make(map[string]*agent.ToolResultMessage),
	}
}

// Scroll callbacks may run off the owner loop. Only post work here.
func (m *InteractiveMode) onTranscriptScrolled(delta int) {
	if delta >= 0 {
		return
	}
	sv := m.transcriptScrollView
	if sv == nil {
		return
	}
	if sv.ScrollTop() > sv.ViewportHeight() {
		return
	}
	m.postUITask(m.pageTranscriptHistory)
}

func (m *InteractiveMode) offerTranscriptHistoryPage(data string) bool {
	if m.altScreen != nil || m.transcriptHistory == nil || m.chatContainer == nil {
		return false
	}
	keys := tui.GetKeybindings()
	match := tui.ActionMatches(keys, data, tui.KBAltScreenPageUp, "pageup") ||
		tui.ActionMatches(keys, data, tui.KBAltScreenTop, "ctrl+home")
	if !match || (m.editor != nil && m.editor.Text() != "") {
		return false
	}
	m.pageTranscriptHistory()
	return true
}

func (m *InteractiveMode) pageTranscriptHistory() {
	h := m.transcriptHistory
	sess := m.currentSession()
	if h == nil || sess == nil {
		return
	}
	// One call may cross several undisplayed intervals (usage entries and
	// friends render nothing) before it reaches a page that paints.
	for pages := 0; pages < 3; pages++ {
		if !h.more {
			return
		}
		page, more, found := sess.BranchEntriesBefore(h.oldestID, h.shown, transcriptPageSize)
		if !found {
			// The boundary left the active path (leaf moved without a
			// repaint): a page would splice against the wrong window.
			m.transcriptHistory = nil
			return
		}
		h.more = more
		if len(page) == 0 {
			return
		}
		for _, e := range page {
			h.shown[e.Base().ID] = struct{}{}
		}
		box := m.renderTranscriptPage(page)
		kids := box.Children()
		anchor := h.oldestComp
		h.oldestID = page[0].Base().ID
		if len(kids) > 0 {
			h.oldestComp = kids[0]
		}
		if len(kids) == 0 {
			continue
		}
		if m.altScreen != nil {
			m.altScreen.ResetTextSelection()
		}
		pos := m.spliceHistoryChildren(anchor, kids)
		m.compensateHistoryPrepend(box, pos)
		if m.tuiInst != nil {
			if m.altScreen != nil {
				m.tuiInst.Render()
			} else {
				m.tuiInst.ForceFullRender()
			}
		}
		return
	}
}

func (m *InteractiveMode) spliceHistoryChildren(anchor tui.Component, kids []tui.Component) int {
	pos := m.chatContainer.ChildCount()
	if anchor != nil {
		for i, ch := range m.chatContainer.Children() {
			if ch == anchor {
				pos = i
				break
			}
		}
	}
	if pos == m.chatContainer.ChildCount() {
		for _, kid := range kids {
			m.chatContainer.Add(kid)
		}
		return pos
	}
	for _, kid := range kids {
		m.chatContainer.InsertBefore(anchor, kid)
	}
	return pos
}

func (m *InteractiveMode) compensateHistoryPrepend(box *tui.Container, pos int) {
	sv := m.transcriptScrollView
	if sv == nil {
		return
	}
	width := sv.RenderedContentWidth()
	if width <= 0 {
		return
	}
	// A splice below the viewport must not shift it: only compensate when
	// the insertion point's row offset is at or above the scroll offset.
	beforeRows := 0
	for _, component := range []*tui.Container{m.headerBox, m.loadedResourcesContainer} {
		if component != nil {
			beforeRows += len(component.Render(width))
		}
	}
	for _, component := range m.chatContainer.Children()[:pos] {
		beforeRows += len(component.Render(width))
	}
	if sv.ScrollTop() >= beforeRows {
		sv.CompensatePrepend(len(box.Render(width)))
	}
}

func (m *InteractiveMode) renderTranscriptPage(page []SessionEntry) *tui.Container {
	box := tui.NewContainer()
	// Render into a scratch container with live chrome detached: the
	// warming notice belongs to the live tail, not the historical page.
	prev, warming := m.chatContainer, m.cacheWarmingUsage
	m.chatContainer, m.cacheWarmingUsage = box, nil
	defer func() {
		m.chatContainer, m.cacheWarmingUsage = prev, warming
	}()
	sess := m.currentSession()
	pass := transcriptRenderPass{pendingCalls: make(map[string]ai.ToolCall)}
	for _, e := range page {
		pe := parsedEntry{entry: e}
		if e.Base().Type == "message" {
			pe.agent, pe.hasAgent = sessionEntryAgentMessage(sess, e)
		}
		m.renderParsedSessionEntry(pe, nil, pass)
	}
	return box
}
