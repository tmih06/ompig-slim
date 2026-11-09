package tui

import (
	"strings"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// ModelPickerOverlayOptions anchors the frameless host wrapper below the prompt.
func ModelPickerOverlayOptions() OverlayOptions {
	return OverlayOptions{anchor: overlayBottomCenter, width: overlayPercent(100), maxHeight: overlayPercent(100)}
}

// ModelPickerConfig supplies a cached chat catalog and resolved quick roles. It
// owns no credentials or persistence: selection changes only this session or Task.
type ModelPickerConfig struct {
	Models, QuickRoles                 []ModelSelectorItem
	Current, CurrentRole, TaskSelector string
	ContextTokens                      int
	Height                             func() int
	TaskKeys                           []string
	RoleConfig                         modelroles.Config
	ResolvedRoles                      map[string]string
	Recent                             []string
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
	if cfg.TaskSelector == "" {
		cfg.TaskSelector = cfg.Current
	}
	p := &ModelPicker{cfg: cfg, browser: NewModelBrowser(cfg.Current)}
	p.browser.contextTokens = cfg.ContextTokens
	p.browser.roleConfig = cfg.RoleConfig
	p.browser.recent = cfg.Recent
	p.browser.SetRoles(cfg.ResolvedRoles)
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
			p.browser.current = "@" + p.cfg.CurrentRole
			if p.cfg.CurrentRole == "" {
				for _, item := range p.cfg.QuickRoles {
					if item.Provider+"/"+item.ID == p.cfg.Current {
						p.browser.current = "@" + item.Role
						break
					}
				}
			}
			p.browser.SetItems(p.cfg.QuickRoles)
		} else {
			p.browser.current = p.cfg.Current
			if p.taskMode {
				p.browser.current = p.cfg.TaskSelector
			}
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
			p.browser.current = selector
			p.browser.SetItems(p.cfg.Models)
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
		height = max(14, int(float64(max(16, p.cfg.Height()))*.4))
	}
	inner := width - 4
	th := modelOverlayTheme()
	title := "Switch Model"
	status := "Session-only switch — role models stay unchanged"
	footer := "↑/↓ models · ⏎ use for this session · type to search · @ quick roles · ⎋ close"
	if p.roleMode {
		status = "Quick role switch — applies its model and thinking for this session"
		footer = "↑/↓ roles · ⏎ apply role model · type to search · ⎋ close"
	} else if p.taskMode {
		title = "Switch Task Model"
		status = "Task subagent switch — spawned task agents use this model (session-only)"
		footer = "↑/↓ models · ⏎ use for Task subagents · type to search · ⎋ close"
	}
	if !p.roleMode && len(p.cfg.TaskKeys) > 0 {
		target := "task model"
		if p.taskMode {
			target = "session model"
		}
		key := p.cfg.TaskKeys[0]
		if key == "alt+p" {
			key = "Alt+P"
		}
		footer += " · " + key + " " + target
	}
	statusColor := th.Muted
	if p.taskMode {
		statusColor = th.Error
	}
	out := []string{modelOverlayTop(width, title, -1, p.taskMode), modelOverlayRow(fg(statusColor, " "+status), width, p.taskMode)}
	for _, line := range p.browser.Render(inner, height-4) {
		out = append(out, modelOverlayRow(line, width, p.taskMode))
	}
	border := th.Border
	if p.taskMode {
		border = th.Error
	}
	out = append(out, modelOverlayRow(fg(th.Dim, footer), width, p.taskMode), fg(border, "╰"+strings.Repeat("─", width-2)+"╯"))
	for i := range out {
		out[i] = widthx.TruncateToWidth(out[i], width, "…", false)
	}
	return out
}
