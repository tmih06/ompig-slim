package codingagent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
	"github.com/MichaelKinsy/PiG/tui"
)

// pickModel hosts the compact bottom sheet. Accepted selections remain visible
// until the caller's model change completes; Task-only choices never switch chat.
func (m *InteractiveMode) pickModel(ctx context.Context) (string, bool, func()) {
	items := m.availableModelItems()
	// Explicit CLI models (including keyless local and harness models) can precede
	// catalog discovery. Keep the live model selectable rather than dropping its roles.
	if current := m.opts.Model; current != nil {
		found := false
		for _, item := range items {
			if item.FQ() == modelSpec(current) {
				found = true
				break
			}
		}
		if !found {
			provider, id, _ := strings.Cut(modelSpec(current), "/")
			items = append(items, tui.ModelSelectorItem{Provider: provider, ID: id, Name: current.DisplayName, Model: current})
		}
	}
	items = m.scopeBrowserModels(items)
	var roles []tui.ModelSelectorItem
	roleConfig := modelroles.Config{}
	resolvedRoles := map[string]string{}
	taskSelector := modelSpec(m.opts.Model)
	currentRole := ""
	if h, ok := m.opts.SessionHandle.(interface{ ActiveModelRole() string }); ok {
		currentRole = h.ActiveModelRole()
	}
	if sm := m.opts.SettingsManager; sm != nil {
		config := sm.Get()
		if config.ModelRoles["task"] != "" {
			if value, err := sm.ResolveModelRole("@task"); err == nil {
				taskSelector, _ = modelroles.Split(value)
			}
		}
		roleConfig = config.Config
		pool := []modelroles.Candidate{}
		for _, item := range items {
			pool = append(pool, modelroles.Candidate{Provider: item.Provider, ID: item.ID, Kind: tui.ModelItemKind(item)})
		}
		for _, role := range roleConfig.Known(nil) {
			// Contextual execution inheritance is not an automatic role assignment.
			// OMP leaves unconfigured plan/task/vision/commit chips out of the catalog.
			if roleConfig.ModelRoles[role] == "" && (role == "plan" || role == "task" || role == "vision" || role == "commit") {
				continue
			}
			if selector, err := sm.ResolveModelRole("@"+role, pool); err == nil {
				resolvedRoles[role] = selector
			}
		}
		for _, role := range config.Config.Order() {
			pool := []modelroles.Candidate{}
			for _, item := range items {
				pool = append(pool, modelroles.Candidate{Provider: item.Provider, ID: item.ID})
			}
			selector, err := sm.ResolveModelRole("@"+role, pool)
			if err != nil {
				continue
			}
			base, _ := modelroles.Split(selector)
			for _, item := range items {
				if item.FQ() == base {
					item.Role = role
					roles = append(roles, item)
					break
				}
			}
		}
	}
	tokens := 0
	if m.opts.ContextUsage != nil {
		if used, _ := m.opts.ContextUsage(); used != nil {
			tokens = *used
		}
	}
	picker := tui.NewModelPicker(tui.ModelPickerConfig{Models: items, QuickRoles: roles, Current: modelSpec(m.opts.Model), CurrentRole: currentRole, TaskSelector: taskSelector, ContextTokens: tokens, RoleConfig: roleConfig, ResolvedRoles: resolvedRoles, Recent: roleConfig.ModelRecent, Height: m.tuiInst.Height, TaskKeys: []string{"alt+p", "ctrl+l"}})
	m.disposeActiveSelector()
	overlay := m.tuiInst.ShowOverlay(picker, tui.ModelPickerOverlayOptions())
	token := new(int)
	done := sync.OnceFunc(func() {
		overlay.Close()
		if m.activeSelectorToken == token {
			m.activeSelectorToken = nil
			m.activeSelectorDispose = nil
		}
		m.tuiInst.RequestRender()
	})
	m.activeSelectorToken, m.activeSelectorDispose = token, done
	input, release := m.acquireModalInputChannel()
	defer release()
	m.tuiInst.Render()
	for !picker.Done() {
		data, ok := m.readModalInputFor(picker, input)
		if !ok {
			done()
			return "", false, nil
		}
		for _, chunk := range m.modalInputChunks(picker, []string{string(data)}) {
			picker.HandleInput(chunk)
			if picker.Done() {
				break
			}
		}
		m.tuiInst.Render()
	}
	if picker.Cancelled() {
		done()
		return "", false, nil
	}
	if picker.SelectedTask() {
		if sm := m.opts.SettingsManager; sm != nil {
			sm.ApplyOverrides(Settings{Config: modelroles.Config{ModelRoles: map[string]string{"task": picker.SelectedFQ()}}})
			m.showStatus("Task model: " + picker.SelectedFQ() + " (session only)")
		}
		done()
		return "", false, nil
	}
	if picker.OverContext() {
		handle := m.opts.SessionHandle
		if handle == nil {
			done()
			return "", false, nil
		}
		if !m.askConfirm(ctx, "Compact before switching", "The conversation exceeds this model's context window. Compact before switching?") {
			done()
			return "", false, nil
		}
		// Compaction is asynchronous and drains events through the existing interactive
		// command path, rather than issuing an oversized request to the chosen model.
		selected := picker.SelectedFQ()
		if starter, ok := handle.(interface {
			Compact(context.Context, string) (*CompactionResult, error)
		}); ok {
			m.backgroundTasks.Go(func() {
				_, err := starter.Compact(ctx, "")
				m.runOnMain(ctx, func() {
					if err != nil {
						done()
						m.showError(err.Error())
						return
					}
					m.switchModel(ctx, selected, false, func(err error) {
						done()
						if err != nil {
							m.showError(err.Error())
						}
					})
				})
			})
			return "", false, nil
		}
		done()
		m.showError(fmt.Sprintf("Cannot compact before switching to %s", strings.TrimSpace(selected)))
		return "", false, nil
	}
	return picker.SelectedFQ(), true, done
}
