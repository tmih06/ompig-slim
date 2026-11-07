package codingagent

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// Personal runtime presentation: the prompt shows aggregate subscription quota across accounts.
const promptQuotaInterval = time.Minute

type promptQuotaState struct {
	snapshot       ai.UsageSnapshot
	activeAccounts map[string]ai.UsageActiveAccount
	selection      *promptQuotaSelection
	fetching       bool
	loaded         bool
	failed         bool
}

type promptQuotaSelection struct {
	provider, model   string
	key               *string
	resolved, present bool
}

// Model changes invalidate the border immediately; credential-store reads stay off the UI loop.
func (m *InteractiveMode) promptQuotaModelChanged() {
	if m.editor != nil {
		m.editor.Invalidate()
	}
	if m.backgroundCtx != nil {
		m.refreshPromptQuotaAccount(m.backgroundCtx)
	}
}

func (m *InteractiveMode) refreshPromptQuotaAccount(ctx context.Context) {
	resolver, ok := m.opts.SessionHandle.(interface {
		ResolveUsageAccount(context.Context, *ai.Model) (*string, bool, error)
	})
	if !ok || m.opts.Model == nil || ctx.Err() != nil {
		return
	}
	if m.promptQuota == nil {
		m.promptQuota = &promptQuotaState{}
	}
	state, model := m.promptQuota, m.opts.Model
	selection := &promptQuotaSelection{provider: model.ProviderID(), model: model.ID}
	state.selection = selection
	m.backgroundTasks.Go(func() {
		resolveCtx, cancel := context.WithTimeout(ctx, UsageRequestTimeout)
		defer cancel()
		key, present, err := resolver.ResolveUsageAccount(resolveCtx, model)
		_ = m.postToMain(ctx, func() {
			if m.promptQuota != state || state.selection != selection || ctx.Err() != nil {
				return
			}
			selection.key, selection.present, selection.resolved = key, present, err == nil
			if m.editor != nil {
				m.editor.Invalidate()
			}
			m.requestRender()
		})
	})
}

// Polling and network work belong to the run's joined background task group. Only the owner loop reads or publishes presentation state.
func (m *InteractiveMode) startPromptQuota(ctx context.Context) {
	m.refreshPromptQuota(ctx)
	m.backgroundTasks.Go(func() {
		ticker := time.NewTicker(promptQuotaInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.postToMain(ctx, func() { m.refreshPromptQuota(ctx) }); err != nil {
					return
				}
			}
		}
	})
}

func (m *InteractiveMode) refreshPromptQuota(ctx context.Context) {
	m.refreshPromptQuotaAccount(ctx)
	service := m.opts.UsageService
	if service == nil {
		service = m.usageService
	}
	if service == nil || ctx.Err() != nil {
		return
	}
	if m.promptQuota == nil {
		m.promptQuota = &promptQuotaState{}
	}
	state := m.promptQuota
	if state.fetching {
		return
	}
	state.fetching = true
	m.backgroundTasks.Go(func() {
		fetchCtx, cancel := context.WithTimeout(ctx, 2*UsageRequestTimeout)
		defer cancel()
		snapshot, err := service.FetchSnapshot(fetchCtx, true)
		activeAccounts := make(map[string]ai.UsageActiveAccount)
		seen := make(map[string]bool)
		for _, report := range snapshot.Reports {
			if seen[report.Provider] {
				continue
			}
			seen[report.Provider] = true
			if account, ok := service.ActiveAccount(fetchCtx, report.Provider); ok {
				activeAccounts[report.Provider] = account
			}
		}
		_ = m.postToMain(ctx, func() {
			// A replacement session owns a new quota service and state.
			if m.promptQuota != state || ctx.Err() != nil {
				return
			}
			state.snapshot, state.loaded, state.failed, state.fetching = snapshot, true, err != nil, false
			state.activeAccounts = activeAccounts
			if m.editor != nil {
				m.editor.Invalidate()
			}
			m.requestRender()
		})
	})
}

func (m *InteractiveMode) promptQuotaLabel(maxWidth int) string {
	if m.opts.Model == nil || maxWidth <= 0 {
		return ""
	}
	provider := m.opts.Model.ProviderID()
	state := m.promptQuota
	if state == nil || !state.loaded || state.failed {
		return ""
	}
	snapshot := state.snapshot
	snapshot.Reports = m.promptQuotaTierReports(snapshot.Reports, provider)
	windows := promptQuotaWindows(snapshot, provider, m.opts.Model.ID)
	if len(windows) == 0 {
		return ""
	}
	window := windows[0]
	for _, candidate := range windows[1:] {
		if candidate.durationMs > 0 && (window.durationMs == 0 || candidate.durationMs < window.durationMs) {
			window = candidate
		}
	}
	remaining := 1 - window.used
	reset := "reset unknown"
	if window.resetsAt > 0 {
		delta := max(time.Minute, time.Until(time.UnixMilli(window.resetsAt)))
		minutes := int(math.Ceil(delta.Minutes()))
		if minutes < 60 {
			reset = fmt.Sprintf("reset %dm", minutes)
		} else if minutes < 24*60 {
			reset = fmt.Sprintf("reset %dh%02dm", minutes/60, minutes%60)
		} else {
			reset = fmt.Sprintf("reset %dd%dh", minutes/(24*60), minutes/60%24)
		}
	}
	suffix := fmt.Sprintf(" %.0f%% left · %s", remaining*100, reset)
	partial := false
	for _, account := range state.snapshot.UnavailableAccounts {
		partial = partial || account.Provider == provider
	}
	if partial {
		suffix += " (partial)"
	}
	prefix := window.label + " "
	barWidth := min(24, maxWidth-widthx.VisibleWidth(prefix+suffix))
	if barWidth < 12 {
		return ""
	}
	quotaStatus := ai.UsageStatusOK
	if window.used >= 1 {
		quotaStatus = ai.UsageStatusExhausted
	} else if window.used >= .8 {
		quotaStatus = ai.UsageStatusWarning
	}
	limit := ai.UsageLimit{Amount: ai.UsageAmount{UsedFraction: &window.used}, Status: quotaStatus}
	status := prefix + tui.RenderUsageBar(tui.ActiveTheme(), limit, barWidth) + suffix
	if widthx.VisibleWidth(status) <= maxWidth {
		return status
	}
	return ""
}

// Follow the routed account's tier, falling back to the store's active account as the usage board does. Store reads happen during background refresh, never while rendering.
func (m *InteractiveMode) promptQuotaTierReports(reports []ai.UsageReport, provider string) []ai.UsageReport {
	key, routed := m.usageAccountKey(provider)
	if !routed {
		if m.promptQuota == nil {
			return nil
		}
		selection := m.promptQuota.selection
		if selection != nil {
			// A default account can belong to a different tier than the selected model.
			// Do not show that tier while its model-specific route is pending or failed.
			if selection.provider != provider || m.opts.Model == nil || selection.model != m.opts.Model.ID || !selection.resolved {
				return nil
			}
			if selection.present {
				key, routed = selection.key, true
			}
		}
		if !routed {
			account, ok := m.promptQuota.activeAccounts[provider]
			if !ok {
				return nil
			}
			key = account.AccountKey
		}
	}
	for _, group := range tui.GroupUsageReportsByPlan(reports, nil) {
		if group.Provider != provider {
			continue
		}
		for _, report := range group.Reports {
			accountKey, keyed := report.Metadata["accountKey"].(string)
			if key != nil && keyed && accountKey == *key || key == nil && !keyed {
				return group.Reports
			}
		}
	}
	return nil
}

type promptQuotaWindow struct {
	label      string
	used       float64
	resetsAt   int64
	durationMs int64
}

// Sum capacities where the provider supplies absolute values; percentage-only accounts contribute equal capacity. Different windows remain separate.
func promptQuotaWindows(snapshot ai.UsageSnapshot, provider, model string) []promptQuotaWindow {
	type bucket struct {
		label               string
		sum, used, capacity float64
		count               int
		weighted            bool
		unit                ai.UsageUnit
		resetsAt            int64
		durationMs          int64
	}
	var buckets []*bucket
	byKey := map[string]*bucket{}
	for _, report := range tui.CollapseSharedUsageReports(snapshot.Reports) {
		if report.Provider != provider {
			continue
		}
		for _, limit := range report.Limits {
			if limit.Scope.ModelID != "" && limit.Scope.ModelID != model {
				continue
			}
			fraction, ok := ai.UsageUsedFraction(limit)
			if !ok || math.IsNaN(fraction) || math.IsInf(fraction, 0) {
				continue
			}
			windowID, label := limit.Scope.WindowID, limit.Label
			if limit.Window != nil {
				windowID = limit.Window.ID
				label = windowID
			}
			if label == "" {
				label = limit.ID
			}
			key := limit.ID + "|" + windowID
			if limit.Scope.SharedGroup != "" {
				key = limit.Scope.SharedGroup + "|" + windowID
			}
			b := byKey[key]
			if b == nil {
				b = &bucket{label: tui.SanitizeDisplayLine(label), weighted: true, unit: limit.Amount.Unit}
				byKey[key] = b
				buckets = append(buckets, b)
			}
			if limit.Window != nil && limit.Window.DurationMs != nil && *limit.Window.DurationMs > 0 {
				b.durationMs = *limit.Window.DurationMs
			}
			if limit.Window != nil && limit.Window.ResetsAt != nil && *limit.Window.ResetsAt > time.Now().UnixMilli() && (b.resetsAt == 0 || *limit.Window.ResetsAt < b.resetsAt) {
				b.resetsAt = *limit.Window.ResetsAt
			}
			b.sum += fraction
			b.count++
			capacity := limit.Amount.Limit
			if capacity == nil || *capacity <= 0 || math.IsNaN(*capacity) || math.IsInf(*capacity, 0) || b.unit != limit.Amount.Unit {
				b.weighted = false
			} else {
				b.used += fraction * *capacity
				b.capacity += *capacity
			}
		}
	}
	result := make([]promptQuotaWindow, 0, len(buckets))
	for _, b := range buckets {
		used := b.sum / float64(b.count)
		if b.weighted && b.capacity > 0 {
			used = b.used / b.capacity
		}
		result = append(result, promptQuotaWindow{label: b.label, used: max(0, min(1, used)), resetsAt: b.resetsAt, durationMs: b.durationMs})
	}
	return result
}
