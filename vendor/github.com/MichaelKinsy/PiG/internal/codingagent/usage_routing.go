package codingagent

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// FetchAccountSessionUsage reads the shared Codex usage cache for one model. It
// propagates store and caller errors, while unavailable endpoint reports yield
// no facts and therefore leave credential-plan routing unchanged.
func (service *UsageService) FetchAccountSessionUsage(ctx context.Context, provider, modelID string) ([]ai.AccountSessionUsage, error) {
	if provider != "openai-codex" && provider != "codex-json" {
		return nil, nil
	}
	snapshot, err := service.fetchSnapshot(ctx, false, nil, provider, true, true)
	if err != nil {
		return nil, err
	}
	now := service.now()
	facts := make([]ai.AccountSessionUsage, 0, len(snapshot.Reports))
	for _, report := range snapshot.Reports {
		if report.Provider != provider || report.FetchedAt <= 0 || report.FetchedAt > now.UnixMilli() || now.Sub(time.UnixMilli(report.FetchedAt)) > UsageReportTTL {
			continue
		}
		fact, ok := accountSessionUsageFromReport(report, provider, modelID)
		if ok {
			facts = append(facts, fact)
		}
	}
	return facts, nil
}

func (r *ModelRegistry) accountSessionUsageService() (*UsageService, uint64) {
	generation := r.credentialStoreGeneration.Load()
	r.accountRouterMu.Lock()
	defer r.accountRouterMu.Unlock()
	if r.accountUsageService == nil || r.accountUsageServiceGeneration != generation {
		r.accountUsageService = NewUsageService(UsageServiceOptions{
			Credentials:      registryCredentials{registry: r},
			Accounts:         registryCredentialAccounts{registry: r},
			AccountRouter:    r.accountRouter,
			DisableAutoReset: true,
			GetAuth: func(ctx context.Context, providerID string, overrides ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
				return r.ResolveRegistryProviderAuth(ctx, providerID, overrides)
			},
			BaseURLFor: func(providerID string) string {
				if provider := r.GetProvider(providerID); provider != nil {
					return provider.BaseURL
				}
				return ""
			},
		})
		r.accountUsageServiceGeneration = generation
	}
	return r.accountUsageService, generation
}

func (r *ModelRegistry) invalidateAccountSessionUsage(providerID string, accountKey *string) {
	r.accountRouterMu.Lock()
	service := r.accountUsageService
	r.accountRouterMu.Unlock()
	if service != nil {
		service.invalidateUsageAfterReset(providerID, ai.CredentialAccountInfo{ProviderID: providerID, AccountKey: accountKey})
	}
}

func (r *ModelRegistry) accountSessionUsage(ctx context.Context, providerID, modelID string) ([]ai.AccountSessionUsage, error) {
	if !ModelNetworkEnabled() {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	service, generation := r.accountSessionUsageService()
	if generation != r.credentialStoreGeneration.Load() {
		return nil, nil
	}
	// Reuse the existing usage-probe budget so routing adds no new timeout configuration.
	preflight, cancel := context.WithTimeout(ctx, UsageRequestTimeout)
	defer cancel()
	facts, err := service.FetchAccountSessionUsage(preflight, providerID, modelID)
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	if errors.Is(err, context.DeadlineExceeded) && context.Cause(preflight) != nil {
		return nil, nil
	}
	if err != nil {
		// Usage is advisory for routing. A failed probe must not make the
		// account preview disappear or block a request; the router falls back
		// to the credential's saved plan and selected-account policy.
		return nil, nil
	}
	if context.Cause(preflight) != nil || generation != r.credentialStoreGeneration.Load() {
		return nil, nil
	}
	return facts, nil
}

func accountSessionUsageFromReport(report ai.UsageReport, provider, modelID string) (ai.AccountSessionUsage, bool) {
	if !report.AccountKeySet {
		return ai.AccountSessionUsage{}, false
	}
	fact := ai.AccountSessionUsage{AccountKey: report.AccountKey, AccountKeySet: true, FetchedAt: report.FetchedAt}
	if plan, ok := report.Metadata["planType"].(string); ok {
		if normalized, recognized := ai.NormalizeCodexAccountPlan(plan); recognized {
			fact.Plan = normalized
		}
	}
	for _, limit := range report.Limits {
		window, ok := accountSessionUsageWindow(limit, provider, modelID)
		if ok {
			fact.Windows = append(fact.Windows, window)
		}
	}
	return fact, fact.Plan != "" || len(fact.Windows) > 0
}

func accountSessionUsageWindow(limit ai.UsageLimit, provider, modelID string) (ai.AccountSessionUsageWindow, bool) {
	if limit.Scope.Provider != provider || limit.Window == nil || limit.Amount.Unit != ai.UsageUnitPercent || !accountSessionLimitMatchesModel(limit, provider, modelID) {
		return ai.AccountSessionUsageWindow{}, false
	}
	windowID := limit.ID[strings.LastIndex(limit.ID, ":")+1:]
	if windowID != "primary" && windowID != "secondary" {
		return ai.AccountSessionUsageWindow{}, false
	}
	window := ai.AccountSessionUsageWindow{ID: windowID, Exhausted: ai.IsUsageLimitExhausted(limit)}
	if limit.Amount.RemainingFraction != nil {
		fraction := *limit.Amount.RemainingFraction
		if !math.IsNaN(fraction) && !math.IsInf(fraction, 0) && fraction >= 0 && fraction <= 1 {
			window.RemainingFraction = new(fraction)
		}
	} else if limit.Amount.UsedFraction != nil {
		fraction := 1 - *limit.Amount.UsedFraction
		if !math.IsNaN(fraction) && !math.IsInf(fraction, 0) && fraction >= 0 && fraction <= 1 {
			window.RemainingFraction = new(fraction)
		}
	}
	if limit.Window.ResetsAt != nil {
		window.ResetsAt = new(*limit.Window.ResetsAt)
	}
	if limit.Window.DurationMs != nil && *limit.Window.DurationMs > 0 {
		window.DurationMs = new(*limit.Window.DurationMs)
	}
	return window, window.Exhausted || window.RemainingFraction != nil || window.ResetsAt != nil
}

func accountSessionLimitMatchesModel(limit ai.UsageLimit, provider, modelID string) bool {
	if limit.Scope.Tier == "credits" {
		return false
	}
	requestedModel := ai.AccountPolicyModelID(modelID)
	limitModel := ai.AccountPolicyModelID(limit.Scope.ModelID)
	if limitModel != "" && limitModel != requestedModel {
		return false
	}
	if ai.AccountQuotaScope(provider, modelID) == "spark" {
		return limit.Scope.Tier == "spark" || (limitModel != "" && limitModel == requestedModel)
	}
	if limit.Scope.Tier == "spark" {
		return false
	}
	return limit.Scope.Tier == "" || limitModel == requestedModel
}
