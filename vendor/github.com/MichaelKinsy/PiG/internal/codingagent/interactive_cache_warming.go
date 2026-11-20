package codingagent

import (
	"encoding/json"

	"github.com/MichaelKinsy/PiG/tui"
)

type cacheWarmingUsageNotice struct {
	entry     UsageEntry
	notice    *tui.ThemedText
	noteMixed bool
}

// addCacheWarmingUsage groups adjacent cache refreshes into one cumulative notice.
func (m *InteractiveMode) addCacheWarmingUsage(entry UsageEntry) {
	if !m.showCacheMissNotices() {
		m.cacheWarmingUsage = nil
		return
	}
	if current := m.cacheWarmingUsage; current != nil {
		current.entry.Usage.Cost.Total += entry.Usage.Cost.Total
		if !current.noteMixed && current.entry.Note != entry.Note {
			current.entry.Note = ""
			current.noteMixed = true
		}
		current.notice.Invalidate()
		return
	}
	current := &cacheWarmingUsageNotice{entry: entry}
	current.notice = tui.NewThemedText(func() string {
		return tui.ActiveTheme().Fg("dim", FormatCacheWarmingUsage(current.entry))
	}, 1, 0)
	m.cacheWarmingUsage = current
	m.chatContainer.Add(tui.NewSpacer(1))
	m.chatContainer.Add(current.notice)
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
	if head.Type != "usage" {
		m.cacheWarmingUsage = nil
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
		m.cacheWarmingUsage = nil
		return
	}
	m.statusLine.Invalidate()
	if entry.Kind == "cache_warm" {
		m.addCacheWarmingUsage(entry)
	} else {
		m.cacheWarmingUsage = nil
	}
	m.tuiInst.RequestRender()
}
