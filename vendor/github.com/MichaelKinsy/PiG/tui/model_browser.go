package tui

import (
	"slices"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// ModelBrowser is OMP's frameless shared model list: fuzzy search, windowed metric
// rows, and selected-model facts. The host owns surrounding chrome and activation.
// All methods execute on the TUI owner loop; scrolling never changes selection.
type ModelBrowser struct {
	items    []ModelSelectorItem
	filtered []int
	search   *TextInput
	cursor   int
	scroll   int
	rows     int
	focused  bool
	current  string
	roles    map[string]string
	recent   []string
}

// NewModelBrowser builds an empty browser with no asynchronous work or terminal ownership.
func NewModelBrowser(current string) *ModelBrowser {
	return &ModelBrowser{search: NewInput(InputOptions{}), rows: 10, focused: true, current: current}
}

// ModelItemKind normalizes the legacy chat catalog and classifier kind spelling.
func ModelItemKind(item ModelSelectorItem) string {
	if item.Kind == "" {
		return "chat"
	}
	if item.Kind == "classifier" {
		return "tiny"
	}
	return item.Kind
}

// ModelAcceptsRole is the single kind-compatibility rule for hub and quick picker.
func ModelAcceptsRole(role string, item ModelSelectorItem) bool {
	kind := ModelItemKind(item)
	switch role {
	case "image":
		return kind == "image"
	case "speech":
		return kind == "tts"
	case "dictation":
		return kind == "stt"
	case "web":
		return kind == "search"
	case "judge":
		return kind == "judge" || kind == "tiny" || kind == "chat"
	case "tiny", "memory":
		return kind == "tiny" || kind == "chat"
	default:
		return kind == "chat"
	}
}

// ModelThinkingOptions returns only controls the model can apply. Inherit is
// stored bare, auto is offered only for primary roles, and wildcards have no dial.
func ModelThinkingOptions(item ModelSelectorItem, fallback bool) []string {
	model := item.Model
	if model == nil || (!model.ProviderMeta.Reasoning && model.Capabilities.MaxThinking == "") {
		return nil
	}
	out := []string{"inherit", "off"}
	if !fallback {
		out = append(out, "auto")
	}
	for _, level := range ai.GetSupportedThinkingLevels(model) {
		if level != ai.ThinkingOff {
			out = append(out, string(level))
		}
	}
	return out
}

// BrowserSearchText includes the visible price keyword, matching OMP's free search.
func BrowserSearchText(item ModelSelectorItem) string {
	text := item.Provider + "/" + item.ID
	if item.Role != "" {
		text = "@" + item.Role + " " + text
	}
	if item.Model != nil {
		cost := item.Model.CostRates()
		if cost.Input == 0 && cost.Output == 0 {
			text += " free"
		}
	}
	return text
}

// SetItems replaces scope data while preserving the query and selected identity.
func (b *ModelBrowser) SetItems(items []ModelSelectorItem) {
	selected := ""
	if item := b.Selected(); item != nil {
		selected = item.FQ()
	}
	b.items = slices.Clone(items)
	b.filter()
	if selected != "" {
		b.Select(selected)
	}
}

// SetRoles supplies resolved role selectors for the selected model's detail line.
func (b *ModelBrowser) SetRoles(roles map[string]string) { b.roles = roles }

// Query returns the shared query retained across provider scope changes.
func (b *ModelBrowser) Query() string { return b.search.Text() }

// SetQuery changes fuzzy ranking and resets the viewport to the first match.
func (b *ModelBrowser) SetQuery(query string) { b.search.SetText(query); b.filter() }

// filter ranks a fixed haystack and resets selection only after a query/data change.
func (b *ModelBrowser) filter() {
	indices := make([]int, len(b.items))
	for i := range indices {
		indices[i] = i
	}
	b.filtered = FuzzyFilter(indices, b.Query(), func(i int) string { return BrowserSearchText(b.items[i]) })
	b.cursor = 0
	b.scroll = 0
}

// Selected returns the currently highlighted row, or nil for an empty result set.
func (b *ModelBrowser) Selected() *ModelSelectorItem {
	if b.cursor < 0 || b.cursor >= len(b.filtered) {
		return nil
	}
	return &b.items[b.filtered[b.cursor]]
}

// Select reanchors a provider-qualified model or virtual role without changing its query.
func (b *ModelBrowser) Select(selector string) {
	for i, index := range b.filtered {
		if b.items[index].FQ() == selector {
			b.cursor = i
			b.follow()
			return
		}
	}
}

// follow brings keyboard selection into the viewport without recentering wheel scroll.
func (b *ModelBrowser) follow() {
	if b.cursor < b.scroll {
		b.scroll = b.cursor
	}
	if b.cursor >= b.scroll+b.rows {
		b.scroll = b.cursor - b.rows + 1
	}
	b.scroll = max(0, min(b.scroll, max(0, len(b.filtered)-b.rows)))
}

// Move wraps keyboard arrows; paging clamps. Wheel panning uses Pan instead.
func (b *ModelBrowser) Move(delta int, wrap bool) {
	count := len(b.filtered)
	if count == 0 {
		return
	}
	if wrap {
		b.cursor = (b.cursor + delta%count + count) % count
	} else {
		b.cursor = max(0, min(count-1, b.cursor+delta))
	}
	b.follow()
}

// Pan moves only the viewport, leaving the selected model untouched.
func (b *ModelBrowser) Pan(delta int) {
	b.scroll = max(0, min(max(0, len(b.filtered)-b.rows), b.scroll+delta))
}

// HandleInput processes browser-local search/navigation. Hosts handle Enter and
// pane switching; Escape clears a query first and returns false only when empty.
func (b *ModelBrowser) HandleInput(data string) bool {
	kb := GetTUIKeybindings()
	switch {
	case kb.Matches(data, KBSelectCancel):
		if b.Query() == "" {
			return false
		}
		b.SetQuery("")
	case kb.Matches(data, KBSelectUp):
		b.Move(-1, true)
	case kb.Matches(data, KBSelectDown):
		b.Move(1, true)
	case MatchesKeyID(data, "pageup"):
		b.Move(-b.rows, false)
	case MatchesKeyID(data, "pagedown"):
		b.Move(b.rows, false)
	default:
		b.search.HandleInput(data)
		b.filter()
	}
	return true
}

// Render draws exactly height rows. Selected-model facts remain visible below the
// window; row metrics drop columns using the same width policy as the quick picker.
func (b *ModelBrowser) Render(width, height int) []string {
	height = max(1, height)
	width = max(1, width)
	th := ActiveTheme()
	b.rows = max(1, height-6)
	b.search.Focused = b.focused
	lines := []string{}
	input := b.search.Render(max(1, width-2))
	if len(input) > 0 {
		lines = append(lines, "  "+input[0])
	} else {
		lines = append(lines, "  Search models…")
	}
	lines = append(lines, fg(th.Dim, "  ↑↓ models · type to search"))
	metrics := []ModelMetricRow{}
	end := min(len(b.filtered), b.scroll+b.rows)
	for i := b.scroll; i < end; i++ {
		item := b.items[b.filtered[i]]
		marker := "  "
		if i == b.cursor {
			marker = "› "
		}
		label := SanitizeModelText(item.Provider + "/" + item.ID)
		if item.Role != "" {
			label = "[" + strings.ToUpper(SanitizeModelText(item.Role)) + "] " + label
		}
		if item.FQ() == b.current {
			label += " ✓"
		}
		if i == b.cursor && b.focused {
			label = fg(th.Accent, label)
		}
		metrics = append(metrics, ModelMetricRow{Left: marker + label, Model: item.Model})
	}
	rendered := NewModelMetricRows(metrics).Render(width)
	if len(metrics) == 0 {
		rendered = []string{fg(th.Muted, "  No matching models")}
	}
	for len(rendered) < b.rows {
		rendered = append(rendered, "")
	}
	lines = append(lines, rendered[:min(b.rows, len(rendered))]...)
	if selected := b.Selected(); selected != nil {
		tags := []string{}
		base := selected.Provider + "/" + selected.ID
		for role, selector := range b.roles {
			spec, level := modelroles.Split(selector)
			if spec == base {
				tag := strings.ToUpper(role)
				if level != "" {
					tag += ":" + level
				}
				tags = append(tags, tag)
			}
		}
		slices.Sort(tags)
		lines = append(lines, fg(th.Accent, "  "+SanitizeModelText(selected.Name)+"  "+strings.Join(tags, " · ")))
		facts := ModelDetailFactsText(selected.Model, "metadata unavailable")
		for _, line := range strings.Split(facts, "\n") {
			lines = append(lines, "  "+line)
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	lines = lines[:height]
	for i := range lines {
		lines[i] = widthx.TruncateToWidth(lines[i], width, "…", false)
	}
	return lines
}

// Invalidate fulfills Component without retaining a second render cache.
func (b *ModelBrowser) Invalidate() {}
