
package codingagent

import (
	"slices"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
)

const transcriptPageSize = 200

type transcriptHistory struct {
	boundaryID string
	shown map[string]struct{}
	units []transcriptHistoryUnit
	baseCount int
	pendingToolResults map[string]*agent.ToolResultMessage
}

type transcriptHistoryUnit struct {
	startID string
	more bool
	firstComp tui.Component
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
		boundaryID:         boundaryID,
		shown:              shown,
		units:              []transcriptHistoryUnit{{startID: boundaryID, more: true}},
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
	for pages := 0; pages < 3; pages++ {
		j := -1
		for k := len(h.units) - 1; k >= 0; k-- {
			if h.units[k].more {
				j = k
				break
			}
		}
		if j < 0 {
			return
		}
		page, more, found := sess.BranchEntriesBefore(h.units[j].startID, h.shown, transcriptPageSize)
		if !found {
			m.transcriptHistory = nil
			return
		}
		if len(page) == 0 {
			h.units[j].more = false
			return
		}
		for _, e := range page {
			h.shown[e.Base().ID] = struct{}{}
		}
		box := m.renderTranscriptPage(page)
		kids := box.Children()
		unit := transcriptHistoryUnit{startID: page[0].Base().ID, more: more}
		if len(kids) > 0 {
			unit.firstComp = kids[0]
		}
		h.units[j].more = false
		h.units = slices.Insert(h.units, j, unit)
		if len(kids) == 0 {
			continue
		}
		if m.altScreen != nil {
			m.altScreen.ResetTextSelection()
		}
		pos := m.spliceHistoryChildren(h.historyAnchor(j), kids)
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

func (h *transcriptHistory) historyAnchor(j int) tui.Component {
	for k := j + 1; k < len(h.units); k++ {
		if h.units[k].firstComp != nil {
			return h.units[k].firstComp
		}
	}
	return nil
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
