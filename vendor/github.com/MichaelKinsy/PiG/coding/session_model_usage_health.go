package coding

import (
	"context"
	"math"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
)

// SetUsageFallbackConfirmer installs the interactive reserve question. Nil is the
// background-agent policy: reserve may auto-fallback. The caller may replace it
// on session rebind; callbacks run without Session or fallback locks held.
func (s *Session) SetUsageFallbackConfirmer(confirm func(context.Context, string, string, float64) bool) {
	s.retryFallbackMu.Lock()
	s.usageFallbackConfirm = confirm
	s.retryFallbackMu.Unlock()
}

// confirmUsageFallback remembers a declined reserve episode for this primary so
// repeated prompts do not nag; healthy reports clear the episode.
func (s *Session) confirmUsageFallback(ctx context.Context, destination string, remaining float64) bool {
	current := s.fallbackSelector()
	s.retryFallbackMu.Lock()
	confirm := s.usageFallbackConfirm
	approved := s.usageReserveApproved == current
	s.retryFallbackMu.Unlock()
	if approved {
		return false
	}
	if confirm == nil {
		return true
	}
	if destination == "" {
		return false
	}
	if confirm(ctx, current, destination, remaining) {
		return true
	}
	s.retryFallbackMu.Lock()
	s.usageReserveApproved = current
	s.retryFallbackMu.Unlock()
	return false
}

// modelUsageHealth evaluates managed OAuth plan accounts only. Static API keys,
// unmapped usage and failed probes are unknown and fail open. One healthy sibling
// wins; any unknown sibling prevents a reserve/depleted conclusion. Near-limit
// accounts are blocked only when a healthy sibling can take their next request.
func (s *Session) modelUsageHealth(ctx context.Context, model *ai.Model) (string, float64) {
	store, ok := s.services.Credentials().(ai.AccountCredentialStore)
	if !ok {
		return "unknown", 0
	}
	accounts, err := store.ListAccounts(ctx)
	if err != nil {
		return "unknown", 0
	}
	managed := []ai.CredentialAccountInfo{}
	for _, account := range accounts {
		if account.ProviderID == providerID(model) && account.Type == ai.CredentialOAuth {
			managed = append(managed, account)
		}
	}
	if len(managed) == 0 {
		return "unknown", 0
	}
	s.retryFallbackMu.Lock()
	service := s.usageFallbackService
	if service == nil {
		service = icodingagent.NewUsageService(icodingagent.UsageServiceOptions{Credentials: store, Accounts: store, GetAuth: func(ctx context.Context, provider string, overrides ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
			return s.services.Registry().ResolveRegistryProviderAuth(ctx, provider, overrides)
		}})
		s.usageFallbackService = service
	}
	s.retryFallbackMu.Unlock()
	snapshot, err := service.FetchProviderSnapshot(ctx, providerID(model), false)
	if err != nil {
		return "unknown", 0
	}
	reserve := 10.0
	if retry := s.SettingsManager().Get().Retry; retry != nil && retry.UsageReservePct != nil {
		reserve = math.Max(0, math.Min(100, *retry.UsageReservePct))
	}
	healthy, unknown, near := false, false, false
	minimum := 100.0
	bad := []ai.CredentialAccountInfo{}
	for _, account := range managed {
		var report *ai.UsageReport
		for i := range snapshot.Reports {
			candidate := &snapshot.Reports[i]
			if candidate.Provider != providerID(model) {
				continue
			}
			key, _ := candidate.Metadata["accountKey"].(string)
			if account.AccountKey == nil && key == "" || account.AccountKey != nil && key == *account.AccountKey {
				report = candidate
				break
			}
		}
		if report == nil {
			unknown = true
			continue
		}
		remaining, known := modelReportRemaining(*report, model, s.fallbackTime().UnixMilli())
		if !known {
			unknown = true
			continue
		}
		minimum = math.Min(minimum, remaining)
		if remaining > reserve {
			healthy = true
		} else {
			bad = append(bad, account)
			if remaining > 0 {
				near = true
			}
		}
	}
	if healthy {
		for _, account := range bad {
			_ = store.BlockAccount(ctx, providerID(model), account.AccountKey, ai.AccountQuotaScope(providerID(model), model.ID), s.fallbackTime().Add(icodingagent.UsageReportTTL).UnixMilli())
		}
		if len(bad) > 0 {
			if router := s.services.Registry().AccountRouter(); router != nil {
				router.ClearProvider(providerID(model))
			}
		}
		s.retryFallbackMu.Lock()
		s.usageReserveApproved = ""
		s.retryFallbackMu.Unlock()
		return "healthy", minimum
	}
	if unknown {
		return "unknown", minimum
	}
	if near {
		return "reserve", minimum
	}
	return "depleted", minimum
}

// modelReportRemaining chooses the minimum current relevant plan window. Explicit
// model scopes must match; credits/billing amounts are not coding-plan evidence.
func modelReportRemaining(report ai.UsageReport, model *ai.Model, now int64) (float64, bool) {
	remaining := 100.0
	known := false
	for _, limit := range report.Limits {
		if limit.Scope.ModelID != "" && limit.Scope.ModelID != model.ID {
			continue
		}
		if limit.Window != nil && limit.Window.ResetsAt != nil && *limit.Window.ResetsAt <= now && report.FetchedAt < *limit.Window.ResetsAt {
			continue
		}
		id := strings.ToLower(limit.ID + " " + limit.Label)
		if strings.Contains(id, "credit") || strings.Contains(id, "spend") || strings.Contains(id, "balance") {
			continue
		}
		spark := strings.Contains(strings.ToLower(model.ID), "spark")
		if strings.Contains(id, "spark") != spark && providerID(model) == "openai-codex" {
			continue
		}
		value := 0.0
		valid := true
		switch {
		case limit.Amount.RemainingFraction != nil:
			value = *limit.Amount.RemainingFraction * 100
		case limit.Amount.UsedFraction != nil:
			value = (1 - *limit.Amount.UsedFraction) * 100
		case limit.Amount.Remaining != nil && limit.Amount.Limit != nil && *limit.Amount.Limit > 0:
			value = *limit.Amount.Remaining / *limit.Amount.Limit * 100
		case limit.Amount.Used != nil && limit.Amount.Limit != nil && *limit.Amount.Limit > 0:
			value = (1 - *limit.Amount.Used / *limit.Amount.Limit) * 100
		case limit.Status == ai.UsageStatusExhausted:
			value = 0
		default:
			valid = false
		}
		if valid && !math.IsNaN(value) && !math.IsInf(value, 0) {
			known = true
			remaining = math.Min(remaining, math.Max(0, value))
		}
	}
	return remaining, known
}
