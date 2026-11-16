package ai

// ompig-slim addition: ports the Vibe Code budget read of mistral-vibe's
// vibe/app_server/_billing.py and the allowance view of _account.py, so
// `/usage` and the prompt quota bar show the monthly Vibe Code allowance.

import (
	"context"
	"math"
	"strings"
	"time"
)

const (
	mistralUsageProviderID   = "mistral"
	mistralDefaultAPIBaseURL = "https://api.mistral.ai"
	mistralBudgetPath        = "/v1/billing/budget"
	// The id and label say "credit" on purpose: coding/session_model_usage_health.go
	// modelReportRemaining skips ids and labels containing credit, spend or
	// balance, so a pay-as-you-go account past 100 percent is not rerouted as
	// depleted.
	mistralVibeCreditsID    = "mistral:vibe-credits"
	mistralVibeCreditsLabel = "Vibe Credits"
	mistralPaygOverageNote  = "Usage past the allowance is billed through pay-as-you-go."
)

// normalizeMistralAPIBaseURL strips a trailing `/v1` but keeps a path prefix.
func normalizeMistralAPIBaseURL(baseURL string) string {
	trimmed := trimJSWhitespace(baseURL)
	if trimmed == "" {
		return mistralDefaultAPIBaseURL
	}
	normalized := usageTrimTrailingSlashes(trimmed)
	if strings.HasSuffix(strings.ToLower(normalized), "/v1") {
		normalized = normalized[:len(normalized)-len("/v1")]
	}
	if normalized == "" {
		return mistralDefaultAPIBaseURL
	}
	return normalized
}

// mistralUsageStatus keeps an exhausted allowance a warning while
// pay-as-you-go still serves the requests past it.
func mistralUsageStatus(fraction *float64, payg bool) UsageStatus {
	status := usageStatusFor(fraction)
	if status == UsageStatusExhausted && payg {
		return UsageStatusWarning
	}
	return status
}

// fetchMistralUsage probes the public billing route's Vibe Code budget.
func fetchMistralUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != mistralUsageProviderID {
		return nil, nil
	}
	credential := params.Credential
	token := credential.APIKey
	if credential.Type == CredentialOAuth {
		token = credential.AccessToken
	}
	if token == "" {
		return nil, nil
	}
	endpoint := normalizeMistralAPIBaseURL(params.BaseURL) + mistralBudgetPath
	probeCtx, cancel := usageDeadline(ctx, 3*time.Second)
	defer cancel()
	response, err := usageDo(probeCtx, fctx, usageRejectRedirects, "GET", endpoint, map[string]string{
		"Authorization": "Bearer " + token,
		"Accept":        "application/json",
		"User-Agent":    usageUserAgent(),
	}, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		usageLogWarn(fctx, "Mistral usage fetch error", map[string]any{"error": err.Error()})
		return nil, nil
	}
	if !response.OK() {
		if response.Status == 401 {
			return nil, NewUsageHTTPError("Mistral usage: HTTP 401", 401)
		}
		// A 403 means the member lacks `see_org_inference_usage`, not that the
		// key is bad. A *UsageHTTPError purges the cached report and renders the
		// account as rejected, so only 401 carries one.
		if response.Status == 403 {
			usageLogWarn(fctx, "Mistral billing route is not permitted", map[string]any{"status": response.Status})
			return nil, nil
		}
		usageLogWarn(fctx, "Mistral usage fetch failed", map[string]any{"status": response.Status})
		return nil, nil
	}
	payload, ok := response.JSON()
	record, isRecord := usageRecord(payload)
	if !ok || !isRecord {
		usageLogWarn(fctx, "Mistral usage response was not a JSON object", nil)
		return nil, nil
	}
	budget, ok := usageRecord(record["vibe_budget"])
	if !ok {
		return nil, nil
	}
	percentage, ok := usageNumber(budget["usage_percentage"])
	if !ok || math.IsNaN(percentage) || math.IsInf(percentage, 0) {
		return nil, nil
	}
	allowance, ok := usageNumber(budget["initial_budget"])
	if !ok || math.IsNaN(allowance) || math.IsInf(allowance, 0) || allowance <= 0 {
		return nil, nil
	}
	payg, _ := usageBool(budget, "payg_enabled")
	used := math.Max(0, percentage)
	fraction := used / 100
	remaining := math.Max(0, 100-used)
	remainingFraction := math.Max(0, 1-fraction)
	// Built by hand because usagePercentAmount clamps Used to 100 and would hide
	// an overage.
	amount := UsageAmount{
		Used:              &used,
		Limit:             new(100.0),
		Remaining:         &remaining,
		UsedFraction:      &fraction,
		RemainingFraction: &remainingFraction,
		Unit:              UsageUnitPercent,
	}
	// A calendar month has no fixed span, so the window carries no duration
	// (usage_provider_zai.go's Alibaba monthly bucket for the same reason).
	window := &UsageWindow{ID: "monthly", Label: "Monthly"}
	if resetsAt, ok := usageIsoTimestamp(budget["reset_at"]); ok {
		window.ResetsAt = &resetsAt
	}
	limit := UsageLimit{
		ID:     mistralVibeCreditsID,
		Label:  mistralVibeCreditsLabel,
		Scope:  UsageScope{Provider: params.Provider, WindowID: window.ID, Shared: true},
		Window: window,
		Amount: amount,
		Status: mistralUsageStatus(&fraction, payg),
	}
	if payg && fraction >= 1 {
		limit.Notes = []string{mistralPaygOverageNote}
	}
	return &UsageReport{Provider: params.Provider, FetchedAt: timeNowMillis(), Limits: []UsageLimit{limit}, Raw: payload}, nil
}

// mistralUsageProvider returns the Mistral usage provider.
func mistralUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: mistralUsageProviderID,
		Supports: func(params UsageFetchParams) bool {
			if params.Provider != mistralUsageProviderID {
				return false
			}
			if params.Credential.Type == CredentialOAuth {
				return params.Credential.AccessToken != ""
			}
			return params.Credential.APIKey != ""
		},
		FetchUsage:           fetchMistralUsage,
		ValidatesCredentials: true,
	}
}
