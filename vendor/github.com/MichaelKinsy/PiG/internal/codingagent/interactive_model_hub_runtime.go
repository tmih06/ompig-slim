package codingagent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/configvalue"
	"github.com/MichaelKinsy/PiG/tui"
)

// hubCatalogItem widens typed runtime catalogs into display-only browser rows.
// Image/classifier descriptors never masquerade as executable chat models.
func hubCatalogItem(model ai.AnyModel) tui.ModelSelectorItem {
	item := tui.ModelSelectorItem{Provider: model.ProviderID(), ID: model.ModelID(), Kind: string(model.ModelType())}
	switch typed := model.(type) {
	case *ai.Model:
		item.Name = typed.DisplayName
		item.Model = typed
	case *ai.ImageModel:
		item.Name = typed.Name
		item.Model = &ai.Model{ID: typed.ID, DisplayName: typed.Name, ProviderMeta: ai.ProviderMetadata{ProviderID: typed.Provider}, Input: typed.Input}
	case *ai.ClassifierModel:
		item.Name = typed.Name
		item.Kind = "tiny"
		item.Model = &ai.Model{ID: typed.ID, DisplayName: typed.Name, ProviderMeta: ai.ProviderMetadata{ProviderID: typed.Provider}, Input: typed.Input, Capabilities: ai.ModelCapabilities{ContextWindow: typed.ContextWindow}}
	}
	return item
}

// applyModelPreset runs blocking session mutation on an owned worker; the modal
// keeps draining main-loop tasks and receives its completion on that same loop.
func (m *InteractiveMode) applyModelPreset(ctx context.Context, name string, finish func(string, error)) {
	handle, ok := m.opts.SessionHandle.(interface {
		ApplyModelPreset(context.Context, string) (string, error)
	})
	if !ok {
		finish("", fmt.Errorf("preset switching unavailable"))
		return
	}
	if m.runStreaming() {
		finish("", fmt.Errorf("wait for the current response before switching presets"))
		return
	}
	m.backgroundTasks.Go(func() {
		var message string
		var err error
		if owner, ok := m.opts.SessionHandle.(interface {
			ApplyModelPresetOnMain(context.Context, string, func(func() error) error) (string, error)
		}); ok {
			message, err = owner.ApplyModelPresetOnMain(ctx, name, func(mutate func() error) error { return m.runOnMainAndWait(ctx, mutate) })
		} else {
			message, err = handle.ApplyModelPreset(ctx, name)
		}
		m.runOnMain(ctx, func() {
			if err == nil {
				model := m.opts.SessionHandle.Agent().Model()
				m.opts.Model = model
				m.statusLine.SetModel(model)
				m.refreshThinkingLevel()
				m.tuiInst.RequestRender()
			}
			finish(message, err)
		})
	})
}

// hubAutoRefresh is a process-lifetime once guard, consumed only when a debounce
// actually starts. Separate registries do not share catalogs or credential state.
var hubAutoRefresh sync.Map

// hubRefreshJob lives on the main loop. force requests coalesce into one follow-up
// credential refresh while an ordinary catalog request is already in flight.
type hubRefreshJob struct {
	timer          *time.Timer
	running, force bool
	callbacks      []func([]tui.ModelSelectorItem, []tui.ModelSelectorItem, error)
}

// hubRefreshQueue owns refreshes only for one overlay lifetime. Timers and workers
// publish through runOnMain; cancellation prevents writes into a disposed hub.
type hubRefreshQueue struct {
	m              *InteractiveMode
	ctx            context.Context
	registry       *ModelRegistry
	available, all func() []tui.ModelSelectorItem
	jobs           map[string]*hubRefreshJob
}

// newHubRefreshQueue binds cached snapshots without starting I/O during rendering.
func newHubRefreshQueue(m *InteractiveMode, ctx context.Context, r *ModelRegistry, available, all func() []tui.ModelSelectorItem) *hubRefreshQueue {
	return &hubRefreshQueue{m: m, ctx: ctx, registry: r, available: available, all: all, jobs: map[string]*hubRefreshJob{}}
}

// request debounces hover for 120 ms, deduplicates once-per-process automatic
// requests, and upgrades F5 to a forced refresh even while an earlier fetch runs.
func (q *hubRefreshQueue) request(provider string, force bool, finish func([]tui.ModelSelectorItem, []tui.ModelSelectorItem, error)) {
	if q.ctx.Err() != nil {
		return
	}
	key := struct {
		r *ModelRegistry
		p string
	}{q.registry, provider}
	job := q.jobs[provider]
	if job != nil {
		job.callbacks = append(job.callbacks, finish)
		if force {
			job.force = true
			if job.timer != nil {
				job.timer.Stop()
				job.timer = nil
				q.start(provider, job, true)
			}
		}
		return
	}
	if !force && provider != "" {
		if _, seen := hubAutoRefresh.Load(key); seen {
			finish(q.available(), q.all(), nil)
			return
		}
	}
	job = &hubRefreshJob{force: force, callbacks: []func([]tui.ModelSelectorItem, []tui.ModelSelectorItem, error){finish}}
	q.jobs[provider] = job
	if provider == "" || force {
		q.start(provider, job, force)
		return
	}
	job.timer = time.AfterFunc(120*time.Millisecond, func() { q.m.runOnMain(q.ctx, func() { job.timer = nil; q.start(provider, job, false) }) })
}

// invalidateModelCredentialCommands implements explicit F5 credential refresh.
// Values are copied under registry lock; command invalidation and credential-store
// reads happen outside it. No helper runs until ordinary auth resolution needs it.
func (r *ModelRegistry) invalidateModelCredentialCommands(ctx context.Context, provider string) {
	values := []string{}
	r.mu.RLock()
	if r.config != nil {
		for id, config := range r.config.Providers {
			if provider != "" && provider != id {
				continue
			}
			values = append(values, config.APIKey)
			for _, header := range config.Headers {
				if header != nil {
					values = append(values, *header)
				}
			}
		}
	}
	r.mu.RUnlock()
	if provider != "" {
		if credential, err := r.ReadCredential(ctx, provider); err == nil && credential != nil {
			values = append(values, credential.Key)
		}
	}
	for _, value := range values {
		configvalue.Invalidate(value)
	}
}

// cancelScheduledExcept cancels hover-only debounces when navigation leaves a
// provider. Explicit forced work and in-flight requests continue to settle.
func (q *hubRefreshQueue) cancelScheduledExcept(keep string) {
	for provider, job := range q.jobs {
		if provider == keep || job.timer == nil || job.force {
			continue
		}
		job.timer.Stop()
		delete(q.jobs, provider)
		for _, finish := range job.callbacks {
			finish(nil, nil, nil)
		}
	}
}

// start performs registry discovery off-loop and atomically publishes snapshots
// on completion. Queued forced refreshes outlive navigation, not overlay closure.
func (q *hubRefreshQueue) start(provider string, job *hubRefreshJob, force bool) {
	if q.ctx.Err() != nil || q.jobs[provider] != job {
		return
	}
	job.running = true
	job.force = false
	hubAutoRefresh.Store(struct {
		r *ModelRegistry
		p string
	}{q.registry, provider}, true)
	q.m.backgroundTasks.Go(func() {
		ctx, cancel := context.WithTimeout(q.ctx, 15*time.Second)
		defer cancel()
		options := CatalogRefreshOptions{AllowNetwork: ModelNetworkEnabled(), Force: new(force)}
		if provider != "" {
			options.Providers = []string{provider}
		}
		if force {
			q.registry.invalidateModelCredentialCommands(ctx, provider)
		}
		q.registry.refreshContext(ctx, options.Providers)
		result := q.registry.RefreshCatalogs(ctx, options)
		var err error
		if warning := modelCatalogRefreshError(result, ctx.Err()); warning != "" {
			err = fmt.Errorf("%s", warning)
		}
		q.m.runOnMain(q.ctx, func() {
			if job.force {
				q.start(provider, job, true)
				return
			}
			delete(q.jobs, provider)
			models, all := q.available(), q.all()
			for _, finish := range job.callbacks {
				finish(models, all, err)
			}
			q.m.tuiInst.RequestRender()
		})
	})
}
