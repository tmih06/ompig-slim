package ai

import (
	"context"
	"math"
	"time"
)

// AccountSessionUsageFreshness bounds the report age accepted by account routing.
const AccountSessionUsageFreshness = 5 * time.Minute

// AccountSessionUsage contains only fresh facts needed to choose a Codex account.
type AccountSessionUsage struct {
	AccountKey    *string                     `json:"accountKey"`
	AccountKeySet bool                        `json:"accountKeySet"`
	FetchedAt     int64                       `json:"fetchedAt"`
	Plan          string                      `json:"plan,omitempty"`
	Windows       []AccountSessionUsageWindow `json:"windows,omitempty"`
}

// AccountSessionUsageWindow is one Codex primary or secondary rolling window.
type AccountSessionUsageWindow struct {
	ID                string   `json:"id"`
	RemainingFraction *float64 `json:"remainingFraction,omitempty"`
	ResetsAt          *int64   `json:"resetsAt,omitempty"`
	DurationMs        *int64   `json:"durationMs,omitempty"`
	Exhausted         bool     `json:"exhausted,omitempty"`
}

// AccountSessionUsageSource supplies cached usage facts before a session reservation.
type AccountSessionUsageSource func(context.Context, string, string) ([]AccountSessionUsage, error)

// SetSessionUsageSource installs the bounded, outside-transaction source used for Codex routing.
func (r *AccountRouter) SetSessionUsageSource(source AccountSessionUsageSource) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.usageSource = source
	r.mu.Unlock()
}

// SetSessionUsageInvalidator installs the cache owner callback used after a reset clears blocks.
func (r *AccountRouter) SetSessionUsageInvalidator(invalidate func(providerID string, accountKey *string)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.sessionUsageInvalidator = invalidate
	r.mu.Unlock()
}

func (r *AccountRouter) sessionUsageSource() AccountSessionUsageSource {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.usageSource
}

func normalizeAccountSessionUsage(fact AccountSessionUsage, now time.Time) (AccountSessionUsage, bool) {
	nowMs := now.UnixMilli()
	if !fact.AccountKeySet || fact.FetchedAt <= 0 || fact.FetchedAt > nowMs || nowMs-fact.FetchedAt > AccountSessionUsageFreshness.Milliseconds() {
		return AccountSessionUsage{}, false
	}
	if normalized, ok := NormalizeCodexAccountPlan(fact.Plan); ok {
		fact.Plan = normalized
	} else {
		fact.Plan = ""
	}
	windows := make([]AccountSessionUsageWindow, 0, len(fact.Windows))
	for _, window := range fact.Windows {
		if window.ID != "primary" && window.ID != "secondary" {
			continue
		}
		if window.ResetsAt != nil {
			if *window.ResetsAt <= nowMs {
				continue
			}
			window.ResetsAt = new(*window.ResetsAt)
		}
		if window.DurationMs != nil {
			if *window.DurationMs <= 0 {
				window.DurationMs = nil
			} else {
				window.DurationMs = new(*window.DurationMs)
			}
		}
		if window.RemainingFraction != nil {
			fraction := *window.RemainingFraction
			if math.IsNaN(fraction) || math.IsInf(fraction, 0) || fraction < 0 || fraction > 1 {
				window.RemainingFraction = nil
			} else {
				window.RemainingFraction = new(fraction)
			}
		}
		if window.Exhausted || window.RemainingFraction != nil || window.ResetsAt != nil {
			windows = append(windows, window)
		}
	}
	fact.Windows = windows
	if fact.Plan == "" && len(fact.Windows) == 0 {
		return AccountSessionUsage{}, false
	}
	if fact.AccountKey != nil {
		fact.AccountKey = new(*fact.AccountKey)
	}
	return fact, true
}

func accountSessionUsageFor(route AccountSessionRoute, accountKey *string, now time.Time) *AccountSessionUsage {
	var selected *AccountSessionUsage
	for _, candidate := range route.Usage {
		if accountKeyOf(candidate.AccountKey) != accountKeyOf(accountKey) {
			continue
		}
		fact, ok := normalizeAccountSessionUsage(candidate, now)
		if !ok {
			continue
		}
		if selected == nil || fact.FetchedAt > selected.FetchedAt {
			selected = &fact
		}
	}
	return selected
}

func accountSessionUsageExhausted(usage *AccountSessionUsage) bool {
	if usage == nil {
		return false
	}
	for _, window := range usage.Windows {
		if window.Exhausted {
			return true
		}
	}
	return false
}

func accountUsageReadyAt(usage *AccountSessionUsage) *int64 {
	if usage == nil {
		return nil
	}
	var latest *int64
	for _, window := range usage.Windows {
		if window.Exhausted && window.ResetsAt != nil && (latest == nil || *window.ResetsAt > *latest) {
			latest = window.ResetsAt
		}
	}
	return latest
}

func shortestWindowRemaining(usage *AccountSessionUsage) float64 {
	if usage == nil {
		return -1
	}
	var shortest *AccountSessionUsageWindow
	for i := range usage.Windows {
		window := &usage.Windows[i]
		if shortest == nil || shorterSessionWindow(window, shortest) {
			shortest = window
		}
	}
	if shortest == nil || shortest.RemainingFraction == nil {
		return -1
	}
	return *shortest.RemainingFraction
}

func shorterSessionWindow(window, current *AccountSessionUsageWindow) bool {
	if window.DurationMs != nil && current.DurationMs != nil && *window.DurationMs != *current.DurationMs {
		return *window.DurationMs < *current.DurationMs
	}
	// Without comparable durations, prefer Codex's primary window.
	return window.ID == "primary" && current.ID != "primary"
}
