package codingagent

import (
	"encoding/json"

	"github.com/MichaelKinsy/PiG/tui"
)

// addCacheWarmingUsage renders one persisted cache-warming refresh when cache
// notices are on. Mirrors upstream interactive-mode.ts addCacheWarmingUsage.
func (m *InteractiveMode) addCacheWarmingUsage(entry UsageEntry) {
	if !m.showCacheMissNotices() {
		return
	}
	m.chatContainer.Add(tui.NewSpacer(1))
	m.chatContainer.Add(themedNotice("dim", FormatCacheWarmingUsage(entry), 1))
}

// decodeUsageEntry decodes a "usage" session entry.
func decodeUsageEntry(raw json.RawMessage) (UsageEntry, bool) {
	var entry UsageEntry
	if json.Unmarshal(raw, &entry) != nil || entry.Type != "usage" {
		return UsageEntry{}, false
	}
	return entry, true
}

func (m *InteractiveMode) handleEntryAppended(raw json.RawMessage) {
	var head struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &head) != nil {
		return
	}
	if head.Type == "custom" {
		var entry CustomEntry
		if json.Unmarshal(raw, &entry) == nil {
			m.addCustomEntryToChat(entry)
			m.tuiInst.RequestRender()
		}
		return
	}
	if head.Type == "context_edit" {
		// Pi's footer asks session.getContextUsage() on every render (footer.ts:143), so an edit that drops a message from the projection (a
		// length-truncated reply, a recovered attempt) changes the percentage at once; PiG's footer holds a stored estimate to refresh.
		m.refreshFooterContextUsage()
		m.tuiInst.RequestRender()
		return
	}
	entry, ok := decodeUsageEntry(raw)
	if !ok {
		return
	}
	m.statusLine.Invalidate()
	if entry.Kind == "cache_warm" {
		m.addCacheWarmingUsage(entry)
	}
	m.tuiInst.RequestRender()
}
