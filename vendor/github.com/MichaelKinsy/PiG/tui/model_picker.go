package tui

import (
	"strings"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// ModelPickerOverlayOptions anchors the frameless host wrapper below the prompt.
func ModelPickerOverlayOptions() OverlayOptions {
	return OverlayOptions{anchor: overlayBottomCenter, width: overlayPercent(90), maxHeight: overlayPercent(50)}
}

// ModelPickerConfig supplies a cached chat catalog and resolved quick roles. It
// owns no credentials or persistence: selection changes only this session or Task.
type ModelPickerConfig struct {
	Models, QuickRoles                 []ModelSelectorItem
	Current, CurrentRole, TaskSelector string
	ContextTokens                      int
	Height                             func() int
	TaskKeys                           []string
}

// ModelPicker is OMP's compact bottom-anchored model switcher. A leading @ query
// switches to quick roles; pressing its shortcut again toggles Task-only mode.
// Escape clears search before closing, and an accepted selector stays immutable.
type ModelPicker struct {
	cfg                ModelPickerConfig
	browser            *ModelBrowser
	roleMode, taskMode bool
	done, cancelled    bool
	selected           string
	selectedTask       bool
	overContext        bool
}

// NewModelPicker hydrates cached rows and preselects the current physical model.
func NewModelPicker(cfg ModelPickerConfig) *ModelPicker {
	p := &ModelPicker{cfg: cfg, browser: NewModelBrowser(cfg.Current)}
	p.browser.SetItems(cfg.Models)
	p.browser.Select(cfg.Current)
	return p
}

// Done reports acceptance or cancellation, never the intermediate query clear.
func (p *ModelPicker) Done() bool { return p.done }

// Cancelled distinguishes Escape closure from an accepted selector.
func (p *ModelPicker) Cancelled() bool { return p.cancelled }

// SelectedFQ is a provider-qualified model or a canonical @role alias.
func (p *ModelPicker) SelectedFQ() string { return p.selected }

// SelectedTask reports a session-only Task override rather than a chat switch.
func (p *ModelPicker) SelectedTask() bool { return p.selectedTask }

// OverContext asks the host to compact before it applies the selected model.
func (p *ModelPicker) OverContext() bool { return p.overContext }

// Invalidate fulfills Component without retaining a stale render generation.
func (p *ModelPicker) Invalidate() { p.browser.Invalidate() }

// SetFilter primes an argument query without mutating the shared catalog.
func (p *ModelPicker) SetFilter(query string) { p.setQuery(query) }

// setQuery swaps the leading-@ role pool while retaining the original query text
// externally. Role names remain searchable independently of their physical model.
func (p *ModelPicker) setQuery(query string) {
	roles := strings.HasPrefix(query, "@")
	if roles != p.roleMode {
		p.roleMode = roles
		if roles {
			p.browser.SetItems(p.cfg.QuickRoles)
		} else {
			p.browser.SetItems(p.cfg.Models)
		}
	}
	p.browser.SetQuery(query)
	if roles && query == "@" && p.cfg.CurrentRole != "" {
		p.browser.Select("@" + p.cfg.CurrentRole)
	}
}

// HandleInput routes Task toggle and acceptance before fuzzy browser navigation.
func (p *ModelPicker) HandleInput(data string) {
	if p.done {
		return
	}
	kb := GetTUIKeybindings()
	if kb.Matches(data, KBSelectCancel) {
		if p.browser.Query() != "" {
			p.setQuery("")
			return
		}
		p.done = true
		p.cancelled = true
		return
	}
	for _, key := range p.cfg.TaskKeys {
		if MatchesKeyID(data, key) && !p.roleMode {
			p.taskMode = !p.taskMode
			selector := p.cfg.Current
			if p.taskMode {
				selector = p.cfg.TaskSelector
			}
			p.browser.Select(selector)
			return
		}
	}
	if kb.Matches(data, KBSelectConfirm) {
		item := p.browser.Selected()
		if item == nil {
			return
		}
		p.selected = item.FQ()
		p.selectedTask = p.taskMode
		p.done = true
		p.overContext = item.Model != nil && item.Model.Capabilities.ContextWindow > 0 && p.cfg.ContextTokens > item.Model.Capabilities.ContextWindow
		return
	}
	query := p.browser.Query()
	p.browser.HandleInput(data)
	if next := p.browser.Query(); next != query {
		p.setQuery(next)
	}
}

// Render mirrors the compact picker card with a shared metric browser and status
// distinguishing session-only, quick-role, and Task-only actions.
func (p *ModelPicker) Render(width int) []string {
	width = max(4, width)
	height := 16
	if p.cfg.Height != nil {
		height = max(12, int(float64(p.cfg.Height())*.4))
	}
	inner := width - 4
	th := ActiveTheme()
	title := "Switch Model"
	status := "Session-only switch — role models stay unchanged"
	footer := "↑↓ models · Enter use for session · @ quick roles · Esc close"
	if p.roleMode {
		status = "Quick role switch — applies its model and thinking for this session"
		footer = "↑↓ roles · Enter apply role · Esc close"
	} else if p.taskMode {
		title = "Switch Task Model"
		status = "Task subagent switch — spawned task agents use this model (session-only)"
		footer = "↑↓ models · Enter use for Task subagents · shortcut returns to session"
	}
	out := []string{fg(th.Accent, "╭─ "+title+" "+strings.Repeat("─", max(0, width-len(title)-5))+"╮"), "│ " + widthx.TruncateToWidth(fg(th.Muted, status), inner, "…", true) + " │"}
	for _, line := range p.browser.Render(inner, height-4) {
		out = append(out, "│ "+widthx.TruncateToWidth(line, inner, "…", true)+" │")
	}
	out = append(out, "│ "+widthx.TruncateToWidth(fg(th.Dim, footer), inner, "…", true)+" │", fg(th.Accent, "╰"+strings.Repeat("─", width-2)+"╯"))
	for i := range out {
		out[i] = widthx.TruncateToWidth(out[i], width, "…", false)
	}
	return out
}
