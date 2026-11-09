package codingagent

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
	"github.com/MichaelKinsy/PiG/tui"
)

// modelHubHandler opens /model management, separate from Alt+P's session picker.
func modelHubHandler(sc *SlashContext) error {
	if sc.ShowModelHub == nil {
		sc.Append("Model management is unavailable in this context.")
		return nil
	}
	sc.ShowModelHub()
	return nil
}

// showModelHub owns the overlay and modal input until closure. The ordinary UI
// task queue keeps running through readModalInputFor, so cancellation and catalog
// notifications cannot deadlock the owner loop. Assignments never switch a busy model.
func (m *InteractiveMode) showModelHub() {
	previousMode := "regular"
	if m.altScreen != nil {
		previousMode = "fullscreen"
	}
	if previousMode == "regular" {
		if !m.switchTuiMode("fullscreen", true, true) {
			m.showWarning("Close active overlays before opening Models.")
			return
		}
		defer m.switchTuiMode(previousMode, true, true)
	}
	sm := m.opts.SettingsManager
	if sm == nil {
		m.showError("Model settings unavailable")
		return
	}
	life, cancel := context.WithCancel(m.runCtx)
	defer cancel()
	registry := m.modelRegistryOrDefault()
	var allSnapshot, availableSnapshot []tui.ModelSelectorItem
	// One composed snapshot serves every role and both browser lists. Discovery
	// and login invalidate both together; role edits never repeat catalog work.
	allItems := func() []tui.ModelSelectorItem {
		if allSnapshot == nil {
			availableSnapshot, allSnapshot = registry.hubCatalogItems(sm.Get().DisabledProviders)
		}
		return allSnapshot
	}
	availableItems := func() []tui.ModelSelectorItem {
		allItems()
		return availableSnapshot
	}
	refreshes := newHubRefreshQueue(m, life, registry, availableItems, func() []tui.ModelSelectorItem { allSnapshot = nil; return allItems() })
	var hub *tui.ModelHub
	hub = tui.NewModelHub(tui.ModelHubConfig{
		Models: availableItems(), AllModels: allItems(), Providers: registry.BrowserProviderIDs(), Current: modelSpec(m.opts.Model), Height: func() int { return m.tuiInst.Height() },
		ScopedConfig: func(scope string) modelroles.Config {
			if scope == "project" {
				return sm.GetProjectSettings().Config
			}
			return sm.GetGlobalSettings().Config
		},
		DefaultThinking: func() string { return string(sm.GetDefaultThinkingLevel()) },
		AssignAsync: func(role, selector, scope string, finish func(error)) {
			if role != "default" {
				finish(sm.SetModelRole(role, selector, scope))
				return
			}
			if m.runStreaming() {
				finish(fmt.Errorf("wait for the current response before changing the default model"))
				return
			}
			global, project := sm.GetGlobalSettings().Config, sm.GetProjectSettings().Config
			layer := &global
			if scope == "project" {
				layer = &project
			}
			layer.ModelRoles = maps.Clone(layer.ModelRoles)
			if layer.ModelRoles == nil {
				layer.ModelRoles = map[string]string{}
			}
			if selector == "" {
				delete(layer.ModelRoles, role)
			} else {
				layer.ModelRoles[role] = selector
			}
			prospective := global.Merge(project)
			// Explicit assignment switches to the chosen layer even if another layer
			// shadows its persisted role. Clear switches only when it exposes a role.
			value := selector
			if value == "" {
				value = prospective.ModelRoles[role]
				if value == "" {
					finish(sm.SetModelRole(role, "", scope))
					return
				}
			}
			pool := []modelroles.Candidate{}
			for _, item := range availableItems() {
				pool = append(pool, modelroles.Candidate{Provider: item.Provider, ID: item.ID, Kind: tui.ModelItemKind(item)})
			}
			resolved, err := prospective.ResolveAvailable(value, "", pool)
			if err != nil {
				finish(err)
				return
			}
			if m.opts.ModelBuilder == nil {
				finish(fmt.Errorf("model switching unavailable"))
				return
			}
			base, _ := modelroles.Split(resolved)
			model, err := m.opts.ModelBuilder(base)
			if err != nil {
				finish(err)
				return
			}
			handle := m.opts.SessionHandle
			m.backgroundTasks.Go(func() {
				release := sm.AcquireModelRoleMutation()
				if life.Err() != nil {
					release()
					return
				}
				stop := context.AfterFunc(life, release)
				settle := func(err error) { stop(); release(); finish(err) }
				var authErr error
				if checker, ok := handle.(interface{ CheckModelAuth(*ai.Model) error }); ok {
					authErr = checker.CheckModelAuth(model)
				}
				m.runOnMain(life, func() {
					if authErr != nil {
						settle(authErr)
						return
					}
					if m.opts.SessionHandle != handle {
						settle(fmt.Errorf("session changed during model assignment"))
						return
					}
					if err := sm.SetModelRole(role, selector, scope); err != nil {
						settle(err)
						return
					}
					m.switchModelForRole(life, resolved, "default", scope == "global" && selector != "", settle)
				})
			})
		},
		Login: func(provider string) {
			sc := m.buildSlashContext(life)
			sc.Args = provider
			if err := handleLoginCommand(sc); err != nil {
				m.showError(err.Error())
			}
			refreshes.request(provider, true, func(models, all []tui.ModelSelectorItem, err error) { hub.UpdateCatalog(models, all, err) })
		},
		Refresh:      refreshes.request,
		ScopeChanged: refreshes.cancelScheduledExcept,
		Compaction: func(selector, input string, confirmed bool) (string, bool, error) {
			provider, id, _ := strings.Cut(selector, "/")
			model := registry.GetProviderModelDataByID(provider, id)
			if model == nil {
				return "", false, fmt.Errorf("model unavailable: %s", selector)
			}
			return sm.SetModelCompactionPoint(model, input, confirmed)
		},
		CompactionPreview: func(selector, input string) string {
			provider, id, _ := strings.Cut(selector, "/")
			model := registry.GetProviderModelDataByID(provider, id)
			if model == nil {
				return ""
			}
			return sm.PreviewModelCompactionPoint(model, input)
		},
		Snapshot: func() (modelroles.Config, map[string][]string) {
			s := sm.Get()
			var chains map[string][]string
			if s.Retry != nil {
				chains = modelroles.CloneChains(s.Retry.FallbackChains)
			}
			return s.Config, chains
		},
		Resolve: func(selector string) (string, error) {
			pool := []modelroles.Candidate{}
			for _, item := range availableItems() {
				pool = append(pool, modelroles.Candidate{Provider: item.Provider, ID: item.ID, Kind: tui.ModelItemKind(item)})
			}
			return sm.ResolveModelRole(selector, pool)
		}, Assign: sm.SetModelRole, DeleteRole: sm.DeleteModelRole, Fallback: sm.SetFallbackChain, Cycle: sm.SetRoleCycleOrder,
	})
	h := m.tuiInst.ShowOverlay(hub, tui.ModelHubOverlayOptions())
	defer h.Close()
	defer hub.Dispose()
	hub.StartRefresh()
	input, release := m.acquireModalInputChannel()
	defer release()
	m.tuiInst.Render()
	for !hub.Done() {
		data, ok := m.readModalInputFor(hub, input)
		if !ok {
			return
		}
		for _, chunk := range m.modalInputChunks(hub, []string{string(data)}) {
			hub.HandleInput(chunk)
			if hub.Done() {
				break
			}
		}
		m.tuiInst.Render()
	}
}
