package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// ModelHubConfig is the host boundary of OMP's fullscreen model manager. Snapshot
// callbacks must read memory only. Async mutations settle on the UI owner loop;
// the hub blocks duplicate commits until finish and ignores finishes after Dispose.
type ModelHubConfig struct {
	Models            []ModelSelectorItem
	AllModels         []ModelSelectorItem
	Providers         []string
	Scoped            bool
	Current           string
	Height            func() int
	Snapshot          func() (modelroles.Config, map[string][]string)
	ScopedConfig      func(scope string) modelroles.Config
	DefaultThinking   func() string
	Resolve           func(string) (string, error)
	Assign            func(role, selector, scope string) error
	AssignAsync       func(role, selector, scope string, finish func(error))
	Fallback          func(key string, chain []string, scope string) error
	Cycle             func(order []string, scope string) error
	Login             func(provider string)
	ScopeChanged      func(provider string)
	Refresh           func(provider string, force bool, finish func([]ModelSelectorItem, []ModelSelectorItem, error))
	Compaction        func(selector, input string, confirmed bool) (notice string, confirm bool, err error)
	CompactionPreview func(selector, input string) string
}

// hubRoleRow is a stable primary, fallback slot, separator, or creation action.
type hubRoleRow struct {
	key   string
	index int
	label string
	kind  string
}

// hubScope is one sidebar entry; locked providers remain visible for login.
type hubScope struct {
	id, label, provider string
	count               int
	locked              bool
	separator           bool
	annotation          string
}

// hubChip is an inline role/scope/thinking action, including its exact write layer.
type hubChip struct {
	label, action, role, scope, level string
	start, end                        int
}

// hubStrip owns footer action selection or an inline name/compaction input.
type hubStrip struct {
	kind        string
	item        ModelSelectorItem
	role, scope string
	fallback    int
	chips       []hubChip
	index       int
	input       *TextInput
	initial     string
	returnRoles bool
	confirm     string
}

// ModelHub ports model-hub.ts's owner-loop state machine: persistent sidebar,
// shared browser, inline strips, scope-aware writes, and cancel/search ladders.
// No catalog/auth I/O runs from Render. Escape does not roll back committed edits.
type ModelHub struct {
	cfg                                        ModelHubConfig
	config                                     modelroles.Config
	chains                                     map[string][]string
	scope                                      string
	entries                                    []hubScope
	side                                       int
	sidebarFocus                               bool
	sidebarScroll                              int
	roleFollow, sidebarFollow                  bool
	rows                                       []hubRoleRow
	cursor, roleScroll                         int
	roleTab, kindTab                           int
	done, disposed, pending                    bool
	status                                     string
	browser                                    *ModelBrowser
	resolvedRoles                              map[string]string
	target                                     *hubRoleRow
	strip                                      *hubStrip
	refreshing                                 map[string]bool
	bodyRows, sideWidth, renderWidth, stripRow int
	roleVisible                                int
	sidebarHover, roleHover                    int
}

var hubKinds = []string{"all", "chat", "tiny", "image", "tts", "stt", "search", "judge", "embedding", "rerank", "video"}
var hubRoleTabs = []string{"all", "chat", "kind"}

// ModelHubOverlayOptions mounts the already-framed hub without a second modal
// border, preserving its mouse coordinates and full-height footer.
func ModelHubOverlayOptions() OverlayOptions {
	return OverlayOptions{Fullscreen: true, width: overlayPercent(100), maxHeight: overlayPercent(100), anchor: overlayTopLeft}
}

// NewModelHub hydrates synchronously and initially focuses the All-models scope,
// exactly as OMP; cached Enter works before online discovery has settled.
func NewModelHub(cfg ModelHubConfig) *ModelHub {
	h := &ModelHub{cfg: cfg, scope: "global", side: 1, sidebarFocus: true, sidebarHover: -1, roleHover: -1, roleFollow: true, sidebarFollow: true, browser: NewModelBrowser(""), refreshing: map[string]bool{}}
	if cfg.AllModels == nil {
		h.cfg.AllModels = slices.Clone(cfg.Models)
	}
	h.reload()
	if h.config.ModelRoleStorage == "project" {
		h.scope = "project"
	}
	h.applyScope()
	return h
}

// Done reports overlay closure; Dispose also prevents late asynchronous updates.
func (h *ModelHub) Done() bool { return h.done }

// Dispose releases callback ownership; host cancellation stops its I/O workers.
func (h *ModelHub) Dispose() { h.disposed = true }

// Invalidate fulfills Component; the browser owns no stale layout cache.
func (h *ModelHub) Invalidate() { h.browser.Invalidate() }

// reload preserves sidebar identity and row anchors after writes or refreshes.
func (h *ModelHub) reload() {
	active := "all"
	if h.side < len(h.entries) {
		active = h.entries[h.side].id
	}
	h.config, h.chains = h.cfg.Snapshot()
	h.buildRows()
	assigned, total := 0, 0
	for _, role := range h.config.Known(h.chains) {
		if h.config.RoleInfo(role).Hidden {
			continue
		}
		total++
		if h.config.ModelRoles[role] != "" {
			assigned++
		}
	}
	h.entries = []hubScope{{id: "roles", label: "Roles", annotation: fmt.Sprintf("%d/%d", assigned, total)}, {id: "all", label: "All models", count: len(h.cfg.Models)}}
	if len(h.config.ModelRecent) > 0 {
		h.entries = append(h.entries, hubScope{id: "recent", label: "Recently used", count: len(h.config.ModelRecent)})
	}
	available := map[string]int{}
	catalog := map[string]int{}
	for _, item := range h.cfg.Models {
		available[item.Provider]++
	}
	for _, item := range h.cfg.AllModels {
		catalog[item.Provider]++
	}
	for _, provider := range h.cfg.Providers {
		if _, exists := catalog[provider]; !exists {
			catalog[provider] = 0
		}
	}
	providers := []string{}
	for provider := range catalog {
		if !slices.Contains(h.config.DisabledProviders, provider) {
			providers = append(providers, provider)
		}
	}
	slices.Sort(providers)
	// Available providers precede locked providers; search matches float within the
	// available section while both groups preserve deterministic alphabetic order.
	for _, locked := range []bool{false, true} {
		sectionAdded := false
		for _, provider := range providers {
			isLocked := available[provider] == 0
			if isLocked != locked || (h.cfg.Scoped && isLocked) {
				continue
			}
			if !sectionAdded {
				h.entries = append(h.entries, hubScope{id: fmt.Sprintf("separator:%v", locked), separator: true})
				sectionAdded = true
			}
			h.entries = append(h.entries, hubScope{id: "provider:" + provider, label: provider, provider: provider, count: available[provider], locked: isLocked})
		}
	}
	for i, entry := range h.entries {
		if entry.id == active {
			h.side = i
			break
		}
	}
	h.side = min(h.side, len(h.entries)-1)
	resolved := map[string]string{}
	for _, role := range h.config.Known(h.chains) {
		if h.config.ModelRoles[role] == "" && slices.Contains([]string{"plan", "task", "vision", "commit"}, role) {
			continue
		}
		if selector, err := h.cfg.Resolve("@" + role); err == nil {
			resolved[role] = selector
		}
	}
	h.resolvedRoles = resolved
	h.browser.SetRoles(resolved)
	h.browser.roleConfig = h.config
	h.browser.recent = h.config.ModelRecent
}

// kindRole separates non-chat role tabs without coupling them to provider APIs.
func kindRole(role string) bool {
	return slices.Contains([]string{"image", "web", "speech", "dictation", "judge"}, role)
}

// buildRows puts chat roles, kind roles, model-keyed chains, and creation actions
// in OMP's order. Hidden tags stay functional but are absent from this view.
func (h *ModelHub) buildRows() {
	oldKey := ""
	oldIndex := -9
	if h.cursor < len(h.rows) {
		oldKey = h.rows[h.cursor].key
		oldIndex = h.rows[h.cursor].index
	}
	h.rows = nil
	known := h.config.Known(h.chains)
	for _, kind := range []bool{false, true} {
		if h.roleTab == 1 && kind || h.roleTab == 2 && !kind {
			continue
		}
		if kind && len(h.rows) > 0 {
			h.rows = append(h.rows, hubRoleRow{kind: "separator"})
		}
		for _, role := range known {
			if h.config.ModelTags[role].Hidden || kindRole(role) != kind {
				continue
			}
			h.rows = append(h.rows, hubRoleRow{key: role, index: -1, label: role, kind: "role"})
			for i, value := range h.chains[role] {
				h.rows = append(h.rows, hubRoleRow{key: role, index: i, label: value, kind: "fallback"})
			}
		}
	}
	keys := []string{}
	for key := range h.chains {
		if strings.Contains(key, "/") {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	if len(keys) > 0 {
		h.rows = append(h.rows, hubRoleRow{kind: "separator"})
	}
	for _, key := range keys {
		h.rows = append(h.rows, hubRoleRow{key: key, index: -1, label: key, kind: "chain"})
		for i, value := range h.chains[key] {
			h.rows = append(h.rows, hubRoleRow{key: key, index: i, label: value, kind: "fallback"})
		}
	}
	h.rows = append(h.rows, hubRoleRow{kind: "newRole", label: "+ New role…"}, hubRoleRow{kind: "separator"}, hubRoleRow{kind: "newFallback", label: "+ New fallback…"})
	h.cursor = min(h.cursor, len(h.rows)-1)
	for i, row := range h.rows {
		if row.key == oldKey && row.index == oldIndex && row.kind != "separator" {
			h.cursor = i
			break
		}
	}
}

// result reports failures without falsely claiming a save and refreshes live precedence.
func (h *ModelHub) result(err error) bool {
	if err != nil {
		h.status = err.Error()
		return false
	}
	h.status = ""
	h.reload()
	h.applyScope()
	return true
}

// scopedRole resolves the chosen persisted layer, not the winning merged role.
func (h *ModelHub) scopedRole(role, scope string) string {
	if h.cfg.ScopedConfig == nil {
		return h.config.ModelRoles[role]
	}
	config := h.cfg.ScopedConfig(scope)
	value := config.ModelRoles[role]
	if value == "" {
		return ""
	}
	if scope == "project" {
		config = h.cfg.ScopedConfig("global").Merge(config)
	}
	resolved, err := config.Resolve(value, "")
	if err != nil {
		return value
	}
	return resolved
}

// selectedScope picks the layer owning the effective assignment for thinking/clear.
func (h *ModelHub) selectedScope(role string) string {
	if h.cfg.ScopedConfig != nil && h.cfg.ScopedConfig("project").ModelRoles[role] != "" {
		return "project"
	}
	return "global"
}

// find resolves exact literal IDs before routing suffixes and accepts case variants.
func (h *ModelHub) find(selector string) (ModelSelectorItem, string, bool) {
	base, level := modelroles.Split(selector)
	for _, item := range h.cfg.AllModels {
		if strings.EqualFold(item.Provider+"/"+item.ID, base) {
			return item, level, true
		}
	}
	if i := strings.LastIndex(base, "@"); i > strings.Index(base, "/") {
		route := base[i:]
		for _, item := range h.cfg.AllModels {
			if strings.EqualFold(item.Provider+"/"+item.ID, base[:i]) {
				item.ID += route
				return item, level, true
			}
		}
	}
	return ModelSelectorItem{}, level, false
}

// selectedRowItem resolves roles and locked fallback entries through catalog facts.
func (h *ModelHub) selectedRowItem() (ModelSelectorItem, string, bool) {
	if h.cursor >= len(h.rows) {
		return ModelSelectorItem{}, "", false
	}
	row := h.rows[h.cursor]
	if row.kind == "fallback" {
		if strings.HasSuffix(row.label, "/*") {
			return ModelSelectorItem{}, "", false
		}
		return h.find(row.label)
	}
	if row.kind != "role" {
		return ModelSelectorItem{}, "", false
	}
	value := h.scopedRole(row.key, h.selectedScope(row.key))
	if value == "" {
		value, _ = h.cfg.Resolve("@" + row.key)
	}
	return h.find(value)
}

// applyScope retains a provider-local query while allowing global match counts.
func (h *ModelHub) applyScope() {
	if len(h.entries) == 0 {
		return
	}
	entry := h.entries[h.side]
	items := []ModelSelectorItem{}
	for _, item := range h.cfg.Models {
		if h.target != nil && h.target.kind == "role" && !ModelAcceptsRole(h.target.key, item) {
			continue
		}
		if entry.provider != "" && item.Provider != entry.provider {
			continue
		}
		if entry.id == "recent" && !slices.Contains(h.config.ModelRecent, item.Provider+"/"+item.ID) {
			continue
		}
		if h.kindTab > 0 && ModelItemKind(item) != hubKinds[h.kindTab] {
			continue
		}
		items = append(items, item)
	}
	slices.SortStableFunc(items, func(a, b ModelSelectorItem) int {
		ai := slices.Index(h.config.ModelRecent, a.Provider+"/"+a.ID)
		bi := slices.Index(h.config.ModelRecent, b.Provider+"/"+b.ID)
		if ai < 0 {
			ai = 100000
		}
		if bi < 0 {
			bi = 100000
		}
		if ai != bi {
			return ai - bi
		}
		return strings.Compare(a.Provider+"/"+a.ID, b.Provider+"/"+b.ID)
	})
	h.browser.hideProvider = entry.provider != "" && h.target == nil
	h.browser.SetItems(items)
}

// orderSearchScopes floats providers with fuzzy matches while preserving the
// active provider identity and keeping locked providers in their own section.
func (h *ModelHub) orderSearchScopes() {
	if len(h.entries) <= 3 {
		return
	}
	active := h.entries[h.side].id
	for start := 0; start < len(h.entries); {
		if h.entries[start].provider == "" {
			start++
			continue
		}
		end := start + 1
		for end < len(h.entries) && h.entries[end].provider != "" {
			end++
		}
		slices.SortStableFunc(h.entries[start:end], func(a, b hubScope) int {
			if h.browser.Query() != "" {
				am, bm := h.matchCount(a) > 0, h.matchCount(b) > 0
				if am != bm {
					if am {
						return -1
					}
					return 1
				}
			}
			return strings.Compare(a.label, b.label)
		})
		start = end
	}
	for i, entry := range h.entries {
		if entry.id == active {
			h.side = i
			return
		}
	}
}

// setSide changes scope without stealing arrow focus. A role scope cancels assignment.
func (h *ModelHub) setSide(index int) {
	if h.entries[index].separator {
		return
	}
	h.side = index
	h.sidebarFollow = true
	h.roleFollow = true
	if h.cfg.ScopeChanged != nil {
		h.cfg.ScopeChanged(h.entries[index].provider)
	}
	h.sidebarScroll = max(0, min(h.sidebarScroll, h.side))
	if h.entries[index].id == "roles" {
		h.target = nil
	}
	h.applyScope()
	if p := h.entries[index].provider; p != "" && !h.entries[index].locked {
		h.refresh(p, false)
	}
}

// refresh forwards online discovery; the host deduplicates/debounces I/O and owns
// cancellation. F5 requests credential refresh even during an earlier catalog load.
func (h *ModelHub) refresh(provider string, force bool) {
	if h.cfg.Scoped || h.cfg.Refresh == nil {
		return
	}
	h.refreshing[provider] = true
	h.cfg.Refresh(provider, force, func(models, all []ModelSelectorItem, err error) {
		if h.disposed {
			return
		}
		delete(h.refreshing, provider)
		if err != nil {
			h.status = err.Error()
		}
		if models != nil {
			h.cfg.Models = models
		}
		if all != nil {
			h.cfg.AllModels = all
		}
		h.reload()
		h.applyScope()
	})
}

// UpdateCatalog publishes host-owned login/discovery changes on the UI loop.
func (h *ModelHub) UpdateCatalog(models, all []ModelSelectorItem, err error) {
	if h.disposed {
		return
	}
	if models != nil {
		h.cfg.Models = models
	}
	if all != nil {
		h.cfg.AllModels = all
	}
	if err != nil {
		h.status = err.Error()
	}
	h.reload()
	h.applyScope()
}

// StartRefresh starts the opening all-catalog load after the host has attached its
// overlay, so immediate callbacks cannot race constructor ownership.
func (h *ModelHub) StartRefresh() { h.refresh("", false) }

// moveSide skips zero-match providers while searching, including the Roles view.
func (h *ModelHub) moveSide(delta int) {
	for step := 1; step <= len(h.entries); step++ {
		i := (h.side + delta*step + len(h.entries)*step) % len(h.entries)
		entry := h.entries[i]
		if entry.separator {
			continue
		}
		if h.browser.Query() != "" && (entry.id == "roles" || entry.locked || h.matchCount(entry) == 0) {
			continue
		}
		h.setSide(i)
		return
	}
}

// matchCount uses the same haystack/ranker as browser rows, not substring counts.
func (h *ModelHub) matchCount(entry hubScope) int {
	count := 0
	for _, item := range h.cfg.Models {
		if entry.provider != "" && item.Provider != entry.provider {
			continue
		}
		if entry.id == "recent" && !slices.Contains(h.config.ModelRecent, item.FQ()) {
			continue
		}
		if FuzzyMatchScore(h.browser.Query(), BrowserSearchText(item)).Matches {
			count++
		}
	}
	return count
}

// startAssign opens the in-place browser and preserves sidebar navigation.
func (h *ModelHub) startAssign(row hubRoleRow) {
	h.target = &row
	h.sidebarFocus = false
	h.browser.SetQuery("")
	h.kindTab = 0
	h.applyScope()
	if row.kind == "role" {
		if value, err := h.cfg.Resolve("@" + row.key); err == nil {
			base, _ := modelroles.Split(value)
			h.browser.Select(base)
		}
	} else if row.kind == "fallback" {
		base, _ := modelroles.Split(row.label)
		h.browser.Select(base)
	}
}

// nameStrip validates names on submit; invalid input remains editable in place.
func (h *ModelHub) nameStrip(kind string) {
	h.strip = &hubStrip{kind: kind, input: NewInput(InputOptions{}), fallback: -1}
	h.strip.input.Focused = true
}

// assign serializes async commits and offers only supported effort choices after
// success. Selecting the precommitted effort later is confirmation, not a rewrite.
func (h *ModelHub) assign(item ModelSelectorItem, role, scope, level string, returnRoles bool) {
	if scope == "" && h.config.ModelRoleStorage == "project" {
		h.strip = &hubStrip{kind: "scope", item: item, role: role, returnRoles: returnRoles, fallback: -1, chips: []hubChip{{label: "project", scope: "project"}, {label: "global", scope: "global"}}}
		return
	}
	if scope == "" {
		scope = "global"
	}
	if level == "" {
		_, level = modelroles.Split(h.scopedRole(role, scope))
		if !slices.Contains(ModelThinkingOptions(item, false), level) {
			level = "inherit"
		}
	}
	selector := item.Provider + "/" + item.ID
	if level != "" && level != "inherit" {
		selector += ":" + level
	}
	finish := func(err error) {
		if h.disposed {
			return
		}
		h.pending = false
		if !h.result(err) {
			return
		}
		if len(ModelThinkingOptions(item, false)) > 0 {
			h.thinkingStrip(item, role, scope, level, -1, returnRoles)
		} else if returnRoles {
			h.setSide(0)
			h.sidebarFocus = false
		}
	}
	h.strip = nil
	h.pending = true
	if h.cfg.AssignAsync != nil {
		h.cfg.AssignAsync(role, selector, scope, finish)
	} else {
		finish(h.cfg.Assign(role, selector, scope))
	}
}

// thinkingStrip exposes inherit/off/auto/effort chips, scoped to one exact selector.
func (h *ModelHub) thinkingStrip(item ModelSelectorItem, role, scope, level string, index int, back bool) {
	options := ModelThinkingOptions(item, index >= 0)
	if len(options) == 0 {
		return
	}
	if level == "" || level == "auto" && index >= 0 {
		level = "inherit"
	}
	strip := &hubStrip{kind: "thinking", item: item, role: role, scope: scope, initial: level, fallback: index, returnRoles: back}
	for _, option := range options {
		strip.chips = append(strip.chips, hubChip{label: option, level: option})
	}
	strip.index = max(0, slices.Index(options, level))
	h.strip = strip
}

// roleStrip is the model activation menu: role toggles, scoped assignments, and
// model/provider-specific fallback creation. Only compatible role kinds appear.
func (h *ModelHub) roleStrip(item ModelSelectorItem, keyOnly bool) {
	strip := &hubStrip{kind: "roles", item: item, fallback: -1}
	if !keyOnly {
		for _, role := range h.config.Known(h.chains) {
			if h.config.ModelTags[role].Hidden || !ModelAcceptsRole(role, item) {
				continue
			}
			scopes := []string{"global"}
			if h.config.ModelRoleStorage == "project" {
				scopes = []string{"project", "global"}
			}
			for _, scope := range scopes {
				label := role
				if len(scopes) > 1 {
					label = scope + " " + label
				}
				action := "assign"
				base, _ := modelroles.Split(h.scopedRole(role, scope))
				if base == item.FQ() {
					action = "unassign"
					label = "✓ " + label
				}
				strip.chips = append(strip.chips, hubChip{label: label, action: action, role: role, scope: scope})
			}
		}
	}
	strip.chips = append(strip.chips, hubChip{label: "fallbacks:" + item.ID, action: "modelChain"}, hubChip{label: "fallbacks:" + item.Provider + "/*", action: "providerChain"})
	if !keyOnly && ModelAcceptsRole("default", item) {
		strip.chips = append(strip.chips, hubChip{label: "retry-fallback", action: "fallback"})
	}
	h.strip = strip
}

// commitChain deduplicates exact selectors and lands on the changed fallback row.
func (h *ModelHub) commitChain(key string, chain []string, selected string) {
	seen := map[string]bool{}
	dedup := []string{}
	for _, value := range chain {
		if !seen[value] {
			dedup = append(dedup, value)
			seen[value] = true
		}
	}
	if len(dedup) == 0 {
		dedup = nil
	}
	if !h.result(h.cfg.Fallback(key, dedup, "global")) {
		return
	}
	h.target = nil
	h.browser.SetQuery("")
	h.setSide(0)
	h.sidebarFocus = false
	for i, row := range h.rows {
		if row.key == key && row.label == selected {
			h.cursor = i
			break
		}
	}
}

// activateModel performs the second assignment step or opens the inline role strip.
func (h *ModelHub) activateModel() {
	selected := h.browser.Selected()
	if selected == nil {
		return
	}
	item := *selected
	if h.target == nil {
		h.roleStrip(item, false)
		return
	}
	target := *h.target
	h.target = nil
	switch target.kind {
	case "role":
		h.assign(item, target.key, "", "", true)
	case "newFallback":
		h.roleStrip(item, true)
	default:
		chain := slices.Clone(h.chains[target.key])
		if target.index >= 0 && target.index < len(chain) {
			chain[target.index] = item.FQ()
		} else {
			chain = append(chain, item.FQ())
		}
		h.commitChain(target.key, chain, item.FQ())
	}
}

// activateChip applies one inline action, with unchanged-thinking confirmation a no-op.
func (h *ModelHub) activateChip() {
	strip := h.strip
	if strip == nil || len(strip.chips) == 0 {
		return
	}
	chip := strip.chips[strip.index]
	switch strip.kind {
	case "scope":
		h.assign(strip.item, strip.role, chip.scope, "", strip.returnRoles)
	case "thinking":
		h.strip = nil
		if strip.fallback >= 0 {
			chain := slices.Clone(h.chains[strip.role])
			value := strip.item.FQ()
			if chip.level != "inherit" {
				value += ":" + chip.level
			}
			if strip.fallback < len(chain) {
				chain[strip.fallback] = value
				h.commitChain(strip.role, chain, value)
			}
		} else if chip.level != strip.initial {
			h.assignThinking(strip, chip.level)
		} else if strip.returnRoles {
			h.setSide(0)
			h.sidebarFocus = false
		}
	default:
		h.strip = nil
		switch chip.action {
		case "assign":
			h.assign(strip.item, chip.role, chip.scope, "", false)
		case "unassign":
			h.unassign(chip.role, chip.scope)
		case "modelChain":
			h.startAssign(hubRoleRow{key: strip.item.FQ(), index: -1, kind: "append"})
		case "providerChain":
			h.startAssign(hubRoleRow{key: strip.item.Provider + "/*", index: -1, kind: "append"})
		case "fallback":
			h.commitChain("default", append(slices.Clone(h.chains["default"]), strip.item.FQ()), strip.item.FQ())
		}
	}
}

// unassign uses the same async mutation boundary as assignment: clearing a
// default may expose another persisted layer and switch the live session.
func (h *ModelHub) unassign(role, scope string) {
	h.pending = true
	finish := func(err error) {
		if h.disposed {
			return
		}
		h.pending = false
		h.result(err)
	}
	if h.cfg.AssignAsync != nil {
		h.cfg.AssignAsync(role, "", scope, finish)
	} else {
		finish(h.cfg.Assign(role, "", scope))
	}
}

// assignThinking persists a changed chip without reopening its strip after completion.
func (h *ModelHub) assignThinking(strip *hubStrip, level string) {
	selector := strip.item.FQ()
	if level != "inherit" {
		selector += ":" + level
	}
	h.pending = true
	finish := func(err error) {
		if h.disposed {
			return
		}
		h.pending = false
		h.result(err)
		if strip.returnRoles {
			h.setSide(0)
			h.sidebarFocus = false
		}
	}
	if h.cfg.AssignAsync != nil {
		h.cfg.AssignAsync(strip.role, selector, strip.scope, finish)
	} else {
		finish(h.cfg.Assign(strip.role, selector, strip.scope))
	}
}

// moveRole skips separators and clamps wheel navigation instead of wrapping it.
func (h *ModelHub) moveRole(delta int, wrap bool) {
	h.roleFollow = true
	for step := 0; step < len(h.rows); step++ {
		next := h.cursor + delta
		if wrap {
			next = (next + len(h.rows)) % len(h.rows)
		} else {
			next = max(0, min(len(h.rows)-1, next))
		}
		if next == h.cursor {
			return
		}
		h.cursor = next
		if h.rows[next].kind != "separator" {
			return
		}
	}
}

// roleAction edits role assignments, fallback order, effort, cycling, and compaction.
func (h *ModelHub) roleAction(data string) {
	row := h.rows[h.cursor]
	switch data {
	case "n":
		h.nameStrip("role")
	case "k":
		if h.cfg.Compaction != nil {
			if item, _, ok := h.selectedRowItem(); ok {
				h.nameStrip("compaction")
				h.strip.item = item
			}
		}
	case "t":
		if item, level, ok := h.selectedRowItem(); ok {
			index := -1
			if row.kind == "fallback" {
				index = row.index
			}
			h.thinkingStrip(item, row.key, h.selectedScope(row.key), level, index, true)
		}
	case "f", "a":
		if row.key != "" {
			h.startAssign(hubRoleRow{key: row.key, index: -1, kind: "append"})
		}
	case "c":
		if row.kind == "role" {
			order := h.config.Order()
			if i := slices.Index(order, row.key); i >= 0 {
				order = slices.Delete(order, i, i+1)
			} else {
				order = append(order, row.key)
			}
			h.result(h.cfg.Cycle(order, "global"))
		}
	case "[", "]":
		delta := 1
		if data == "[" {
			delta = -1
		}
		if row.kind == "fallback" {
			chain := slices.Clone(h.chains[row.key])
			next := row.index + delta
			if next >= 0 && next < len(chain) {
				chain[row.index], chain[next] = chain[next], chain[row.index]
				h.commitChain(row.key, chain, chain[next])
			}
		} else if row.kind == "role" {
			order := h.config.Order()
			index := slices.Index(order, row.key)
			next := index + delta
			if index >= 0 && next >= 0 && next < len(order) {
				order[index], order[next] = order[next], order[index]
				h.result(h.cfg.Cycle(order, "global"))
			}
		}
	case "x", "d":
		switch row.kind {
		case "fallback":
			chain := slices.Clone(h.chains[row.key])
			chain = slices.Delete(chain, row.index, row.index+1)
			h.commitChain(row.key, chain, "")
		case "chain":
			h.commitChain(row.key, nil, "")
		case "role":
			h.unassign(row.key, h.selectedScope(row.key))
		}
	}
}

// submitName leaves invalid names/limits editable and requires a second unchanged
// submission when the host warns about an extended compaction window.
func (h *ModelHub) submitName() {
	strip := h.strip
	value := strings.TrimSpace(strip.input.Text())
	switch strip.kind {
	case "role":
		if !modelroles.ValidName(value) || slices.Contains(h.config.Known(h.chains), value) {
			return
		}
		h.strip = nil
		h.startAssign(hubRoleRow{key: value, index: -1, kind: "role"})
	case "compaction":
		notice, confirm, err := h.cfg.Compaction(strip.item.FQ(), value, strip.confirm == value)
		if err != nil {
			h.status = err.Error()
			strip.confirm = ""
			return
		}
		if confirm {
			strip.confirm = value
			h.status = notice
			return
		}
		h.strip = nil
		h.status = notice
		h.reload()
	}
}

// cancel follows strip → assignment → query → overlay; it never discards saved settings.
func (h *ModelHub) cancel() {
	if h.pending {
		h.done = true
		return
	}
	if h.strip != nil {
		back := h.strip.returnRoles
		h.strip = nil
		if back {
			h.setSide(0)
			h.sidebarFocus = false
		}
		return
	}
	if h.target != nil {
		h.target = nil
		h.browser.SetQuery("")
		h.setSide(0)
		h.sidebarFocus = false
		return
	}
	if h.browser.Query() != "" {
		h.browser.SetQuery("")
		h.applyScope()
		return
	}
	h.done = true
}

// HandleInput is the keyboard state machine shared by normalized and raw SGR input.
func (h *ModelHub) HandleInput(data string) {
	if h.disposed {
		return
	}
	kb := GetTUIKeybindings()
	if strings.HasPrefix(data, "\x1b[<") {
		if event, ok := parseSgrMouseEvent(data); ok {
			kind := MouseClick
			if event.release {
				kind = MouseRelease
			}
			if event.button&64 != 0 {
				kind = MouseWheel
			}
			delta := 1
			if event.button&1 == 0 {
				delta = -1
			}
			h.HandleMouse(TuiMouseEvent{Type: kind, X: event.x, Y: event.y, WheelDelta: delta, ClickCount: 1})
		}
		return
	}
	if kb.Matches(data, KBSelectCancel) {
		h.cancel()
		return
	}
	if h.pending {
		return
	}
	if h.strip != nil {
		if h.strip.input != nil {
			if kb.Matches(data, KBSelectConfirm) {
				h.submitName()
			} else {
				h.strip.input.HandleInput(data)
				h.status = ""
			}
			return
		}
		delta := 0
		if MatchesKeyID(data, "left") || MatchesKeyID(data, "up") || MatchesKeyID(data, "shift+tab") {
			delta = -1
		}
		if MatchesKeyID(data, "right") || MatchesKeyID(data, "down") || MatchesKeyID(data, "tab") {
			delta = 1
		}
		if delta != 0 {
			h.strip.index = (h.strip.index + delta + len(h.strip.chips)) % len(h.strip.chips)
		} else if kb.Matches(data, KBSelectConfirm) {
			h.activateChip()
		}
		return
	}
	entry := h.entries[h.side]
	roles := entry.id == "roles" && h.target == nil
	if MatchesKeyID(data, "tab") || MatchesKeyID(data, "shift+tab") {
		h.sidebarFocus = !h.sidebarFocus
		return
	}
	if MatchesKeyID(data, "f5") {
		if !entry.locked {
			h.refresh(entry.provider, true)
		}
		return
	}
	delta := 0
	if MatchesKeyID(data, "alt+left") || data == "\x1bb" {
		delta = -1
	}
	if MatchesKeyID(data, "alt+right") || data == "\x1bf" {
		delta = 1
	}
	if delta != 0 {
		if roles {
			h.roleTab = (h.roleTab + delta + 3) % 3
			h.cursor = 0
			h.roleScroll = 0
			h.buildRows()
		} else {
			h.kindTab = (h.kindTab + delta + len(hubKinds)) % len(hubKinds)
			h.applyScope()
		}
		return
	}
	if MatchesKeyID(data, "left") {
		h.sidebarFocus = true
		return
	}
	if MatchesKeyID(data, "right") {
		if !entry.locked {
			h.sidebarFocus = false
		}
		return
	}
	if kb.Matches(data, KBSelectUp) || kb.Matches(data, KBSelectDown) {
		delta = 1
		if kb.Matches(data, KBSelectUp) {
			delta = -1
		}
		if h.sidebarFocus {
			h.moveSide(delta)
		} else if roles {
			h.moveRole(delta, true)
		} else {
			h.browser.Move(delta, true)
		}
		return
	}
	if kb.Matches(data, KBSelectConfirm) {
		if entry.locked && h.target == nil {
			if h.cfg.Login != nil {
				h.cfg.Login(entry.provider)
			}
			return
		}
		if h.sidebarFocus {
			h.sidebarFocus = false
			return
		}
		if roles {
			row := h.rows[h.cursor]
			switch row.kind {
			case "newRole":
				h.nameStrip("role")
			case "newFallback":
				h.startAssign(row)
			case "chain":
				h.startAssign(hubRoleRow{key: row.key, index: -1, kind: "append"})
			case "separator":
			default:
				h.startAssign(row)
			}
		} else {
			h.activateModel()
		}
		return
	}
	if roles && !h.sidebarFocus {
		h.roleAction(data)
		return
	}
	if roles || entry.locked {
		h.setSide(1)
	}
	h.sidebarFocus = false
	h.browser.HandleInput(data)
	h.orderSearchScopes()
	// A query losing every match in its provider scope falls back to All, but a
	// scope with a surviving match stays local even when other providers rank higher.
	if entry.provider != "" && !entry.locked && h.matchCount(entry) == 0 && h.browser.Query() != "" {
		h.setSide(1)
	}
}

// HandleMouse pans independently of selection and uses component-local geometry.
// Hover chooses a row; clicking activates the same path as Enter. Sidebar wheel
// never changes the active scope or starts provider refreshes.
func (h *ModelHub) HandleMouse(event TuiMouseEvent) *TuiMouseDispatchResult {
	if h.pending || h.disposed {
		return nil
	}
	handled := func() *TuiMouseDispatchResult {
		return &TuiMouseDispatchResult{TuiMouseEventResult: TuiMouseEventResult{Handled: true, Focus: true}}
	}
	if event.Y == h.stripRow && h.strip != nil && h.strip.input == nil {
		for i, chip := range h.strip.chips {
			if event.X >= chip.start && event.X < chip.end {
				h.strip.index = i
				if event.Type == MouseClick {
					h.activateChip()
				}
				return handled()
			}
		}
	}
	if event.Y < 1 || event.Y > h.bodyRows {
		return nil
	}
	row := event.Y - 1
	if event.X <= h.sideWidth+2 {
		if event.Type == MouseWheel {
			h.sidebarFollow = false
			h.sidebarScroll = max(0, min(max(0, len(h.entries)-h.bodyRows), h.sidebarScroll+event.WheelDelta))
			return handled()
		}
		index := h.sidebarScroll + row
		if event.Type == MouseMove && index < len(h.entries) {
			h.sidebarHover = index
			return handled()
		}
		if index < len(h.entries) && event.Type == MouseClick {
			h.setSide(index)
			h.sidebarFocus = true
			return handled()
		}
		return nil
	}
	roles := h.entries[h.side].id == "roles" && h.target == nil
	if row == 1 && event.Type == MouseClick && h.target == nil {
		values := hubKinds
		active := h.kindTab
		prefix := len(" Kind: ")
		if roles {
			values = []string{"all", "chat", "kinds"}
			active = h.roleTab
			prefix = len(" Roles: ")
		}
		_, ranges := modelSegmentTrack(values, active)
		column := event.X - h.sideWidth - 5 - prefix
		for i, bounds := range ranges {
			if column >= bounds[0] && column < bounds[1] {
				if roles {
					h.roleTab = i
					h.cursor = 0
					h.roleScroll = 0
					h.buildRows()
				} else {
					h.kindTab = i
					h.applyScope()
				}
				return handled()
			}
		}
	}
	if event.Type == MouseWheel {
		if roles {
			h.roleFollow = false
			h.roleScroll = max(0, min(max(0, len(h.rows)-max(1, h.roleVisible)), h.roleScroll+event.WheelDelta))
		} else {
			h.browser.Pan(event.WheelDelta)
		}
		return handled()
	}
	if roles {
		index := h.roleScroll + row - 2
		if row < 2 || row >= 2+h.roleVisible || index >= len(h.rows) || h.rows[index].kind == "separator" {
			return nil
		}
		if event.Type == MouseClick || event.Type == MouseMove {
			h.cursor = index
			h.roleHover = index
			h.roleFollow = false
			h.sidebarFocus = false
			if event.Type == MouseClick {
				h.HandleInput("\r")
			}
			return handled()
		}
	} else if !h.entries[h.side].locked {
		index := h.browser.scroll + row - 4
		if row >= 4 && index < len(h.browser.filtered) {
			h.browser.cursor = index
			h.sidebarFocus = false
			if event.Type == MouseClick {
				h.activateModel()
			}
			return handled()
		}
	}
	return nil
}

// footer renders horizontally scrolled chips with recorded mouse hit ranges.
func (h *ModelHub) footer(width int) string {
	th := modelOverlayTheme()
	if h.pending {
		return fg(th.Muted, "Applying model assignment…")
	}
	if strip := h.strip; strip != nil {
		if strip.input != nil {
			label := map[string]string{"role": "New role", "compaction": "Compaction limit"}[strip.kind]
			input := strip.input.Render(max(1, width-len(label)-3))
			if len(input) > 0 {
				return label + ": " + input[0]
			}
			return label + ":"
		}
		start := strip.index
		used := 0
		for start > 0 {
			size := len(strip.chips[start-1].label) + 4
			if used+size > width/2 {
				break
			}
			used += size
			start--
		}
		out := ""
		for i := range strip.chips {
			strip.chips[i].start = -1
			strip.chips[i].end = -1
		}
		for i := start; i < len(strip.chips); i++ {
			label := " " + SanitizeModelText(strip.chips[i].label) + " "
			if widthx.VisibleWidth(out)+widthx.VisibleWidth(label)+2 > width {
				break
			}
			strip.chips[i].start = 1 + widthx.VisibleWidth(out)
			strip.chips[i].end = strip.chips[i].start + widthx.VisibleWidth(label)
			if i == strip.index {
				label = fg(th.Accent, "["+strings.TrimSpace(label)+"]")
			}
			out += label + " "
		}
		return out
	}
	if h.target != nil {
		if h.sidebarFocus {
			return fg(th.Dim, "⏎/→ models · ↑/↓ providers · type to search · Alt+←/Alt+→ kind · ⎋ cancel")
		}
		action := "assign"
		switch h.target.kind {
		case "newFallback":
			action = "pick the protected model"
		case "fallback", "append":
			action = "pick fallback"
		}
		return fg(th.Dim, "⏎ "+action+" · ↑/↓ models · ← providers · type to search · Alt+←/Alt+→ kind · ⎋ cancel")
	}
	if h.entries[h.side].id == "roles" && h.target == nil {
		if h.sidebarFocus {
			return fg(th.Dim, "↑/↓ providers · ⏎/→ roles · Alt+←/Alt+→ tabs · ⎋ close")
		}
		thinking, compaction := "", ""
		if item, _, ok := h.selectedRowItem(); ok {
			if len(ModelThinkingOptions(item, false)) > 0 {
				thinking = " · t thinking"
			}
			if h.cfg.Compaction != nil {
				compaction = " · k compaction limit"
			}
		}
		row := h.rows[h.cursor]
		switch row.kind {
		case "fallback":
			return fg(th.Dim, "↑/↓ rows · ⏎ replace · f add another · x remove"+thinking+compaction+" · [/] reorder · ← providers")
		case "chain":
			return fg(th.Dim, "↑/↓ rows · ⏎/f add fallback · x clear chain · ← providers")
		case "newFallback":
			return fg(th.Dim, "↑/↓ rows · ⏎ new model/provider fallback chain · ← providers")
		}
		return fg(th.Dim, "↑/↓ rows · ⏎ pick · f fallback · x clear"+thinking+compaction+" · c cycle · [/] reorder · n new")
	}
	entry := h.entries[h.side]
	if entry.locked {
		return fg(th.Dim, "⏎ log in · ↑/↓ providers · ⎋ close")
	}
	refresh := ""
	if entry.provider != "" {
		refresh = " · F5 refresh"
	}
	if h.sidebarFocus {
		return fg(th.Dim, "⏎/→ models · ↑/↓ providers · type to search · Alt+←/Alt+→ kind"+refresh+" · ⎋ close")
	}
	return fg(th.Dim, "⏎ assign roles · ↑/↓ models · ← providers · type to search · Alt+←/Alt+→ kind"+refresh+" · ⎋ close")
}

// tabs renders OMP's colored track; preset controls are intentionally absent.
// The same track geometry feeds mouse hit testing instead of guessed label sizes.
func (h *ModelHub) tabs(roles bool) string {
	labels := hubKinds
	active := h.kindTab
	caption := "Kind:"
	if roles {
		labels = []string{"all", "chat", "kinds"}
		active = h.roleTab
		caption = "Roles:"
	}
	track, _ := modelSegmentTrack(labels, active)
	return " " + modelOverlayTheme().Fg("dim", caption) + " " + track + "  " + modelOverlayTheme().Fg("dim", "Alt+←/Alt+→")
}

// roleBody ports OMP's role tree: semantic role dots/tags, aligned indented
// fallbacks, right-hand effort/cycle ordinals, overflow hints, and cycle preview.
func (h *ModelHub) roleBody(width, height int) []string {
	th := modelOverlayTheme()
	status := " Model roles — f adds a retry fallback, cleared roles fall back to auto-selection"
	thinking := ""
	if h.cfg.DefaultThinking != nil {
		thinking = h.cfg.DefaultThinking()
	}
	lines := []string{fg(th.Muted, status), h.tabs(true)}
	tagWidth := 0
	for _, row := range h.rows {
		if row.kind != "role" {
			continue
		}
		info := h.config.RoleInfo(row.key)
		tag := info.Tag
		if tag == "" {
			tag = info.Name
		}
		tagWidth = max(tagWidth, widthx.VisibleWidth(SanitizeModelText(tag)))
	}
	// OMP reserves status, tabs, a spacer, and the cycle track before
	// borrowing another row for an overflow indicator on short terminals.
	capacity := max(1, height-4)
	overflow := len(h.rows) > capacity
	visible := capacity
	if overflow {
		visible = max(1, visible-1)
	}
	if h.roleFollow && h.cursor < h.roleScroll {
		h.roleScroll = h.cursor
	}
	if h.roleFollow && h.cursor >= h.roleScroll+visible {
		h.roleScroll = h.cursor - visible + 1
	}
	h.roleScroll = max(0, min(max(0, len(h.rows)-visible), h.roleScroll))
	end := min(len(h.rows), h.roleScroll+visible)
	h.roleVisible = end - h.roleScroll
	for i := h.roleScroll; i < end; i++ {
		row := h.rows[i]
		selected := i == h.cursor
		cursor := " "
		if selected && !h.sidebarFocus {
			cursor = fg(th.Accent, "❯")
		}
		line := ""
		switch row.kind {
		case "separator":
			line = "   " + fg(th.Border, strings.Repeat("─", max(1, width-6)))
		case "newRole", "newFallback":
			color := th.Dim
			if selected {
				color = th.Accent
			}
			line = " " + cursor + " " + fg(color, row.label)
		case "chain":
			line = " " + cursor + " " + fg(th.Dim, "◌ ") + SanitizeModelText(row.key)
		case "fallback":
			color := th.Muted
			if selected {
				color = th.Accent
			}
			line = " " + cursor + " " + fg(th.Dim, strings.Repeat(" ", tagWidth+3)+"↳") + " " + fg(color, SanitizeModelText(row.label))
		case "role":
			info := h.config.RoleInfo(row.key)
			tag := info.Tag
			if tag == "" {
				tag = info.Name
			}
			tag = widthx.TruncateToWidth(SanitizeModelText(tag), tagWidth, "…", true)
			value := h.config.ModelRoles[row.key]
			dot := fg(th.Dim, "○")
			tagStyled := fg(th.Dim, tag)
			effort := ""
			if value != "" {
				dot = th.Fg(info.Color, "●")
				tagStyled = th.Fg(info.Color, tag)
				if resolved := h.resolvedRoles[row.key]; resolved != "" {
					value = resolved
				}
				base, level := modelroles.Split(value)
				provider, id, hasProvider := strings.Cut(base, "/")
				if hasProvider {
					modelColor := th.Text
					if selected {
						modelColor = th.Accent
					}
					value = fg(th.Dim, SanitizeModelText(provider)+"/") + fg(modelColor, SanitizeModelText(id))
				} else {
					value = SanitizeModelText(base)
				}
				if level == "" && row.key == "default" {
					level = thinking
				}
				if level != "" && level != "inherit" {
					effort = fg(th.Dim, browserEffortGlyph(level)+" "+SanitizeModelText(level))
				}
			} else {
				resolved := h.resolvedRoles[row.key]
				if resolved != "" {
					base, _ := modelroles.Split(resolved)
					value = fg(th.Dim, "auto → "+SanitizeModelText(base))
				} else {
					value = fg(th.Dim, "—")
				}
			}
			line = " " + cursor + " " + dot + " " + tagStyled + "  " + value
			right := effort
			if index := slices.Index(h.config.Order(), row.key); index >= 0 {
				if right != "" {
					right += "  "
				}
				right += fg(th.Accent, fmt.Sprintf("↻ %d", index+1))
			}
			if right != "" && widthx.VisibleWidth(line)+widthx.VisibleWidth(right)+2 <= width {
				line += strings.Repeat(" ", width-widthx.VisibleWidth(line)-widthx.VisibleWidth(right)-1) + right
			}
		}
		if i == h.roleHover {
			line = th.Bg("selectedBg", widthx.TruncateToWidth(line, width, "…", true))
		}
		lines = append(lines, line)
	}
	if overflow {
		parts := []string{}
		if h.roleScroll > 0 {
			parts = append(parts, fmt.Sprintf("↑ %d more", h.roleScroll))
		}
		if end < len(h.rows) {
			parts = append(parts, fmt.Sprintf("↓ %d more", len(h.rows)-end))
		}
		lines = append(lines, fg(th.Dim, "   "+strings.Join(parts, "   ")))
	}
	for len(lines) < height-1 {
		lines = append(lines, "")
	}
	cycle := h.config.Order()
	if len(cycle) == 0 {
		lines = append(lines, fg(th.Dim, "  Ctrl+P cycle is empty — press c on a role to add it"))
	} else {
		selectedRole := ""
		if h.cursor < len(h.rows) {
			selectedRole = h.rows[h.cursor].key
		}
		labels := make([]string, len(cycle))
		for i, role := range cycle {
			labels[i] = SanitizeModelText(role)
		}
		track, _ := modelSegmentTrack(labels, slices.Index(cycle, selectedRole))
		lines = append(lines, fg(th.Dim, "  Ctrl+P cycle: ")+track)
	}
	return lines
}

// Render draws OMP's rounded split frame at the live terminal size. The sidebar
// survives phone widths; the browser remains embedded during assignment instead
// of replacing the entire manager with an unrelated PiG modal.
func (h *ModelHub) Render(width int) []string {
	width = max(4, width)
	height := 30
	if h.cfg.Height != nil {
		height = max(8, h.cfg.Height())
	}
	h.bodyRows = height - 4
	h.renderWidth = width
	side := 18
	for _, entry := range h.entries {
		annotation := entry.annotation
		if annotation == "" && !entry.locked && !entry.separator {
			annotation = fmt.Sprint(entry.count)
		}
		side = max(side, min(26, widthx.VisibleWidth(entry.label)+len(annotation)+5))
	}
	side = min(side, max(8, width-18))
	if width < 24 {
		side = max(1, width/3)
	}
	h.sideWidth = side
	bodyWidth := max(1, width-side-7)
	th := modelOverlayTheme()
	body := []string{}
	entry := h.entries[h.side]
	if entry.id == "roles" && h.target == nil {
		body = h.roleBody(bodyWidth, h.bodyRows)
	} else if entry.locked && h.target == nil {
		body = []string{fg(th.Warning, SanitizeModelText(entry.provider)+" · locked"), "", "Sign in to use this provider's models.", "Enter to log in"}
	} else {
		status := entry.label
		switch entry.id {
		case "all":
			status = "All available models"
		case "recent":
			status = "Recently used models"
		default:
			status = fmt.Sprintf("%s · %d models", entry.label, entry.count)
		}
		if h.cfg.Scoped {
			status += " · --models scope"
		}
		statusColor := th.Muted
		if h.target != nil {
			statusColor = th.Accent
			info := h.config.RoleInfo(h.target.key)
			label := info.Tag
			if label == "" {
				label = info.Name
			}
			status = "Assigning " + label + " — ⏎ assigns, ⎋ cancels"
			switch h.target.kind {
			case "newFallback":
				status = "New fallback chain — ⏎ picks the model it protects, ⎋ cancels"
			case "fallback", "append":
				verb := "Adding fallback for"
				if h.target.kind == "fallback" {
					verb = "Replacing fallback of"
				}
				status = verb + " " + label + " — ⏎ picks the fallback model, ⎋ cancels"
			}
		}
		if h.pending {
			status = "Applying model…"
			statusColor = th.Accent
		}
		if entry.provider != "" && h.refreshing[entry.provider] {
			status = entry.label + " · refreshing model list…"
		}
		statusText := " " + SanitizeModelText(status)
		if h.target != nil && h.target.kind != "newFallback" {
			info := h.config.RoleInfo(h.target.key)
			label := info.Tag
			if label == "" {
				label = info.Name
			}
			label = SanitizeModelText(label)
			statusText = strings.Replace(statusText, label, th.Bold(label), 1)
		}
		body = append(body, fg(statusColor, statusText), h.tabs(false))
		h.browser.focused = !h.sidebarFocus
		body = append(body, h.browser.Render(bodyWidth, h.bodyRows-2)...)
	}
	for len(body) < h.bodyRows {
		body = append(body, "")
	}
	if h.sidebarFollow && h.side < h.sidebarScroll {
		h.sidebarScroll = h.side
	}
	if h.sidebarFollow && h.side >= h.sidebarScroll+h.bodyRows {
		h.sidebarScroll = h.side - h.bodyRows + 1
	}
	lines := []string{modelOverlayTop(width, "Models", side, false)}
	for i := 0; i < h.bodyRows; i++ {
		left := ""
		index := h.sidebarScroll + i
		if index < len(h.entries) {
			e := h.entries[index]
			if e.separator {
				left = fg(th.Border, strings.Repeat("─", side))
			} else {
				icon := "●"
				iconColor := th.Success
				if e.locked {
					icon = "○"
					iconColor = th.Dim
				} else {
					switch e.id {
					case "all":
						icon = "⬢"
						iconColor = th.Accent
					case "recent":
						icon = "⏱"
						iconColor = th.Accent
					case "roles":
						icon = "✦"
						iconColor = th.Accent
					}
				}
				cursor := " "
				if index == h.side && h.sidebarFocus {
					cursor = fg(th.Accent, "❯")
				}
				label := SanitizeModelText(e.label)
				if index == h.side {
					label = th.Bold(fg(th.Accent, label))
				} else if e.locked {
					label = fg(th.Dim, label)
				}
				left = cursor + " " + fg(iconColor, icon) + " " + label
				if !e.locked {
					count := e.count
					if h.browser.Query() != "" {
						count = h.matchCount(e)
					}
					annotation := fmt.Sprint(count)
					if e.annotation != "" {
						annotation = e.annotation
					}
					if widthx.VisibleWidth(left)+len(annotation)+1 <= side {
						left += strings.Repeat(" ", side-widthx.VisibleWidth(left)-len(annotation)) + fg(th.Dim, annotation)
					}
				}
				if index == h.sidebarHover {
					left = th.Bg("selectedBg", left)
				}
			}
		}
		lines = append(lines, fg(th.Border, "│ ")+widthx.TruncateToWidth(left, side, "…", true)+fg(th.Border, " │ ")+widthx.TruncateToWidth(body[i], bodyWidth, "…", true)+fg(th.Border, " │"))
	}
	lines = append(lines, modelOverlayDivider(width, side))
	h.stripRow = len(lines)
	footer := h.footer(width - 4)
	if h.status != "" {
		footer = fg(th.Warning, SanitizeModelText(h.status)) + "  " + footer
	}
	lines = append(lines, modelOverlayRow(footer, width, false), fg(th.Border, "╰"+strings.Repeat("─", max(0, width-2))+"╯"))
	for i := range lines {
		lines[i] = widthx.TruncateToWidth(lines[i], width, "…", false)
	}
	return lines
}
